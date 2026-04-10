#!/bin/bash
# Builds a minimal Alpine aarch64 rootfs for the PRoot provider.
# Contains Python 3, Node.js, Bash, and common dev tools.
#
# Usage: ./scripts/build-rootfs-arm64.sh [output-path]
# Example: ./scripts/build-rootfs-arm64.sh ./alpine-rootfs-aarch64.tar.gz
#
# Requirements:
#   - qemu-user-static (for cross-arch chroot on x86_64 host)
#     Install: sudo apt install qemu-user-static binfmt-support
#   - Root access (sudo) for chroot
#
# On a native aarch64 host (RPi, Graviton), qemu is not needed.
set -euo pipefail

ALPINE_VERSION="3.21"
ARCH="aarch64"
OUTPUT="${1:-./alpine-rootfs-${ARCH}.tar.gz}"
ROOTFS_DIR="$(mktemp -d)/rootfs-${ARCH}"

trap 'rm -rf "$(dirname "${ROOTFS_DIR}")"' EXIT

echo "==> Bootstrapping Alpine ${ALPINE_VERSION} ${ARCH}"
mkdir -p "${ROOTFS_DIR}"
MINIROOTFS_URL="https://dl-cdn.alpinelinux.org/alpine/v${ALPINE_VERSION}/releases/${ARCH}/alpine-minirootfs-${ALPINE_VERSION}.3-${ARCH}.tar.gz"
wget -q "${MINIROOTFS_URL}" -O /tmp/minirootfs.tar.gz
tar xzf /tmp/minirootfs.tar.gz -C "${ROOTFS_DIR}"
rm /tmp/minirootfs.tar.gz

# Copy qemu binary if cross-compiling
if [[ "$(uname -m)" != "aarch64" ]]; then
    echo "    Cross-arch detected, using qemu-aarch64-static"
    QEMU_BIN=$(command -v qemu-aarch64-static 2>/dev/null || true)
    if [[ -z "$QEMU_BIN" ]]; then
        echo "ERROR: qemu-aarch64-static not found."
        echo "Install: sudo apt install qemu-user-static binfmt-support"
        exit 1
    fi
    cp "$QEMU_BIN" "${ROOTFS_DIR}/usr/bin/"
fi

echo "==> Installing core packages"
sudo chroot "${ROOTFS_DIR}" /bin/sh -c '
    apk update && apk add --no-cache \
        python3 py3-pip \
        nodejs npm \
        bash coreutils \
        curl wget jq \
        git openssh-client \
        sqlite \
        gcc musl-dev python3-dev \
    && pip install --break-system-packages --no-cache-dir \
        requests beautifulsoup4 pandas \
        pdfplumber flask fastapi uvicorn \
    && npm install -g typescript tsx \
    && rm -rf /var/cache/apk/* /root/.cache /tmp/*
'

# Clean up qemu binary if we copied it
if [[ "$(uname -m)" != "aarch64" ]]; then
    rm -f "${ROOTFS_DIR}/usr/bin/qemu-aarch64-static"
fi

# Create workspace mount point
mkdir -p "${ROOTFS_DIR}/workspace"

echo "==> Packaging rootfs"
tar czf "${OUTPUT}" -C "${ROOTFS_DIR}" .

echo "==> Done: ${OUTPUT} ($(du -h "${OUTPUT}" | cut -f1))"
echo "    Python: $(sudo chroot "${ROOTFS_DIR}" python3 --version 2>/dev/null || echo 'check in rootfs')"
echo "    Node:   $(sudo chroot "${ROOTFS_DIR}" node --version 2>/dev/null || echo 'check in rootfs')"
