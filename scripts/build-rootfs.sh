#!/bin/bash
# Builds a Firecracker rootfs from a Docker image with forgevm-agent baked in.
# Usage: sudo ./scripts/build-rootfs.sh <docker-image> [output-path] [disk-size-mb]
# Example: sudo ./scripts/build-rootfs.sh alpine:latest ./rootfs.ext4 512
set -euo pipefail

IMAGE="${1:?Usage: $0 <docker-image> [output-path] [disk-size-mb]}"
OUTPUT="${2:-./rootfs.ext4}"
DISK_SIZE_MB="${3:-512}"
AGENT_PATH="${AGENT_PATH:-./bin/forgevm-agent}"

if [ ! -f "${AGENT_PATH}" ]; then
    echo "ERROR: Agent binary not found at ${AGENT_PATH}"
    echo "Build it first: GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags='-s -w' -o bin/forgevm-agent ./cmd/forgevm-agent"
    exit 1
fi

TMPDIR="$(mktemp -d)"
trap "sudo umount ${TMPDIR}/mnt 2>/dev/null || true; rm -rf ${TMPDIR}" EXIT

echo "==> Building rootfs from ${IMAGE} (${DISK_SIZE_MB}MB)"

# Export Docker image.
echo "    Exporting Docker image..."
CONTAINER_ID=$(docker create "${IMAGE}")
docker export "${CONTAINER_ID}" -o "${TMPDIR}/rootfs.tar"
docker rm "${CONTAINER_ID}" >/dev/null

# Create ext4 image.
echo "    Creating ext4 image..."
dd if=/dev/zero of="${TMPDIR}/rootfs.ext4" bs=1M count="${DISK_SIZE_MB}" status=none
mkfs.ext4 -F "${TMPDIR}/rootfs.ext4" >/dev/null 2>&1

# Mount and populate.
echo "    Populating rootfs..."
mkdir -p "${TMPDIR}/mnt"
mount -o loop "${TMPDIR}/rootfs.ext4" "${TMPDIR}/mnt"

tar xf "${TMPDIR}/rootfs.tar" -C "${TMPDIR}/mnt"

# Copy agent.
mkdir -p "${TMPDIR}/mnt/usr/local/bin"
cp "${AGENT_PATH}" "${TMPDIR}/mnt/usr/local/bin/forgevm-agent"
chmod 755 "${TMPDIR}/mnt/usr/local/bin/forgevm-agent"

# Write init script (remove symlink first — Alpine links /sbin/init to busybox).
mkdir -p "${TMPDIR}/mnt/sbin"
rm -f "${TMPDIR}/mnt/sbin/init"
cat > "${TMPDIR}/mnt/sbin/init" <<'INITEOF'
#!/bin/sh
mount -t proc proc /proc
mount -t sysfs sysfs /sys
mount -t devtmpfs devtmpfs /dev
exec /usr/local/bin/forgevm-agent
INITEOF
chmod 755 "${TMPDIR}/mnt/sbin/init"

umount "${TMPDIR}/mnt"

# Shrink ext4 to minimum size.
echo "    Shrinking rootfs..."
e2fsck -fy "${TMPDIR}/rootfs.ext4" >/dev/null 2>&1 || true
resize2fs -M "${TMPDIR}/rootfs.ext4" >/dev/null 2>&1

# Move to output.
cp "${TMPDIR}/rootfs.ext4" "${OUTPUT}"

echo "==> Rootfs built successfully"
echo "    Output: ${OUTPUT}"
echo "    Size: $(du -h "${OUTPUT}" | cut -f1)"
