#!/bin/bash
# ForgeVM — one-command setup
# Usage: ./scripts/setup.sh
set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info()  { echo -e "${GREEN}[+]${NC} $1"; }
warn()  { echo -e "${YELLOW}[!]${NC} $1"; }
fail()  { echo -e "${RED}[x]${NC} $1"; exit 1; }

DATA_DIR="/var/lib/forgevm"
DATA_IMG="$DATA_DIR.img"
DATA_IMG_SIZE="8192"  # MB
KERNEL_URL="https://s3.amazonaws.com/spec.ccfc.min/img/quickstart_guide/x86_64/kernels/vmlinux.bin"

echo ""
echo "  ╔═══════════════════════════════════╗"
echo "  ║       ForgeVM Setup               ║"
echo "  ╚═══════════════════════════════════╝"
echo ""

# ── Check OS ──────────────────────────────────────────────
if [[ "$(uname -s)" != "Linux" ]]; then
    fail "ForgeVM requires Linux. You're on $(uname -s)."
fi

ARCH=$(uname -m)
if [[ "$ARCH" != "x86_64" ]]; then
    fail "ForgeVM requires x86_64. You're on $ARCH."
fi

# ── Check Go ──────────────────────────────────────────────
info "Checking Go..."
if command -v go &>/dev/null; then
    GO_VER=$(go version | grep -oP '\d+\.\d+')
    info "Go $GO_VER found"
else
    fail "Go not found. Install Go 1.25+: https://go.dev/dl/"
fi

# ── Check Docker ──────────────────────────────────────────
info "Checking Docker..."
if command -v docker &>/dev/null; then
    if docker info &>/dev/null; then
        info "Docker is running"
    else
        fail "Docker is installed but not running, or you need to be in the docker group.\n      Try: sudo usermod -aG docker \$(whoami) && newgrp docker"
    fi
else
    fail "Docker not found. Install Docker: https://docs.docker.com/engine/install/"
fi

# ── Check KVM ─────────────────────────────────────────────
info "Checking KVM..."
if [[ -e /dev/kvm ]]; then
    if [[ -r /dev/kvm ]] && [[ -w /dev/kvm ]]; then
        info "KVM is accessible"
    else
        warn "KVM exists but no permission. Fixing..."
        sudo chmod 666 /dev/kvm
        info "KVM permissions fixed"
    fi
else
    fail "KVM not available. Enable virtualization in BIOS or check your hypervisor settings."
fi

# ── Check Firecracker ─────────────────────────────────────
info "Checking Firecracker..."
if command -v firecracker &>/dev/null; then
    FC_VER=$(firecracker --version 2>&1 | head -1)
    info "Firecracker found: $FC_VER"
else
    warn "Firecracker not found. Installing latest release..."
    FC_VERSION=$(curl -s https://api.github.com/repos/firecracker-microvm/firecracker/releases/latest | grep '"tag_name"' | cut -d'"' -f4)
    if [[ -z "$FC_VERSION" ]]; then
        fail "Could not fetch latest Firecracker version. Install manually: https://github.com/firecracker-microvm/firecracker/releases"
    fi
    FC_ARCH="x86_64"
    FC_URL="https://github.com/firecracker-microvm/firecracker/releases/download/${FC_VERSION}/firecracker-${FC_VERSION}-${FC_ARCH}.tgz"
    TMPDIR=$(mktemp -d)
    curl -fSL -o "$TMPDIR/firecracker.tgz" "$FC_URL"
    tar -xzf "$TMPDIR/firecracker.tgz" -C "$TMPDIR"
    FC_BIN=$(find "$TMPDIR" -name "firecracker-${FC_VERSION}-${FC_ARCH}" -type f | head -1)
    if [[ -z "$FC_BIN" ]]; then
        fail "Could not find firecracker binary in release archive"
    fi
    sudo cp "$FC_BIN" /usr/local/bin/firecracker
    sudo chmod +x /usr/local/bin/firecracker
    rm -rf "$TMPDIR"
    info "Firecracker ${FC_VERSION} installed to /usr/local/bin/firecracker"
fi

# ── Data directory with XFS reflink ──────────────────────
info "Setting up data directory..."
sudo mkdir -p "$DATA_DIR"

if mountpoint -q "$DATA_DIR" 2>/dev/null; then
    FS_TYPE=$(df -T "$DATA_DIR" | tail -1 | awk '{print $2}')
    info "$DATA_DIR already mounted ($FS_TYPE)"
else
    # Check if XFS tools are available
    if ! command -v mkfs.xfs &>/dev/null; then
        info "Installing XFS tools for fast COW rootfs copies..."
        sudo apt-get install -y xfsprogs >/dev/null 2>&1 || true
    fi

    if command -v mkfs.xfs &>/dev/null; then
        if [[ ! -f "$DATA_IMG" ]]; then
            info "Creating ${DATA_IMG_SIZE}MB XFS volume at $DATA_IMG..."
            sudo dd if=/dev/zero of="$DATA_IMG" bs=1M count="$DATA_IMG_SIZE" status=progress
            sudo mkfs.xfs -f -m reflink=1 "$DATA_IMG"
            info "XFS volume created with reflink support"
        fi

        sudo mount -o loop "$DATA_IMG" "$DATA_DIR" 2>/dev/null && {
            info "Mounted XFS volume at $DATA_DIR (reflink = instant rootfs copies)"

            # Add to fstab for auto-mount
            if ! grep -q "forgevm" /etc/fstab 2>/dev/null; then
                sudo bash -c "echo '$DATA_IMG $DATA_DIR xfs loop,rw,relatime 0 0' >> /etc/fstab"
                info "Added to /etc/fstab for auto-mount on boot"
            fi
        } || {
            warn "XFS mount failed (WSL2 kernel may not support XFS). Using ext4 fallback."
            warn "Rootfs copies will be slower (~300ms vs ~1ms). Still fully functional."
        }
    else
        warn "XFS tools not available. Using ext4 (rootfs copies will be slower)."
    fi
fi

sudo chown "$(whoami)" "$DATA_DIR"
info "$DATA_DIR ready"

# ── Download kernel ───────────────────────────────────────
info "Setting up kernel..."
if [[ -f "$DATA_DIR/vmlinux.bin" ]]; then
    info "Kernel already exists at $DATA_DIR/vmlinux.bin"
else
    info "Downloading Firecracker kernel..."
    curl -fSL -o "$DATA_DIR/vmlinux.bin" "$KERNEL_URL"
    chmod 644 "$DATA_DIR/vmlinux.bin"
    info "Kernel downloaded ($(du -h "$DATA_DIR/vmlinux.bin" | cut -f1))"
fi

# ── Build binaries ────────────────────────────────────────
info "Building ForgeVM..."
make build-all
info "Built ./forgevm and ./bin/forgevm-agent"

# ── Done ──────────────────────────────────────────────────
echo ""
echo -e "${GREEN}══════════════════════════════════════${NC}"
echo -e "${GREEN}  ForgeVM is ready!${NC}"
echo -e "${GREEN}══════════════════════════════════════${NC}"
echo ""
echo "  Start the server:"
echo "    ./forgevm serve"
echo ""
echo "  Test it:"
echo "    curl -s -X POST localhost:7423/api/v1/sandboxes \\"
echo "      -H 'Content-Type: application/json' \\"
echo "      -d '{\"image\":\"alpine:latest\"}' | jq .id"
echo ""

FS_TYPE=$(df -T "$DATA_DIR" | tail -1 | awk '{print $2}')
if [[ "$FS_TYPE" == "xfs" ]]; then
    echo -e "  ${GREEN}⚡ XFS reflink enabled — snapshot restores ~35ms${NC}"
else
    echo -e "  ${YELLOW}ℹ  ext4 detected — snapshot restores ~300ms${NC}"
    echo -e "  ${YELLOW}   For ~35ms restores, install xfsprogs and re-run setup${NC}"
fi
echo ""
