#!/bin/bash
# ForgeVM Installer — download pre-built binaries and get running in one command
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/DohaerisAI/forgevm/main/scripts/install.sh | bash
#
# Environment variables:
#   FORGEVM_VERSION       — specific version to install (default: latest)
#   FORGEVM_INSTALL_DIR   — where to put binaries (default: /usr/local/bin)
#
set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BOLD='\033[1m'
NC='\033[0m'

info()  { echo -e "${GREEN}[+]${NC} $1"; }
warn()  { echo -e "${YELLOW}[!]${NC} $1"; }
fail()  { echo -e "${RED}[x]${NC} $1"; exit 1; }

INSTALL_DIR="${FORGEVM_INSTALL_DIR:-/usr/local/bin}"
DATA_DIR="/var/lib/forgevm"
KERNEL_URL="https://s3.amazonaws.com/spec.ccfc.min/img/quickstart_guide/x86_64/kernels/vmlinux.bin"
REPO="DohaerisAI/forgevm"

echo ""
echo "  ╔═══════════════════════════════════╗"
echo "  ║       ForgeVM Installer           ║"
echo "  ╚═══════════════════════════════════╝"
echo ""

# ── Check OS + arch ──────────────────────────────────────
if [[ "$(uname -s)" != "Linux" ]]; then
    fail "ForgeVM requires Linux. You're on $(uname -s)."
fi

ARCH=$(uname -m)
case "$ARCH" in
    x86_64)  ARCH_SUFFIX="amd64" ;;
    aarch64|arm64) ARCH_SUFFIX="arm64" ;;
    *) fail "ForgeVM supports x86_64 and aarch64. You're on $ARCH." ;;
esac
info "Detected architecture: $ARCH ($ARCH_SUFFIX)"

# ── Detect version ───────────────────────────────────────
if [[ -n "${FORGEVM_VERSION:-}" ]]; then
    VERSION="$FORGEVM_VERSION"
    info "Using specified version: $VERSION"
else
    info "Detecting latest version..."
    VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | cut -d'"' -f4)
    if [[ -z "$VERSION" ]]; then
        fail "Could not detect latest version. Set FORGEVM_VERSION manually."
    fi
    info "Latest version: $VERSION"
fi

RELEASE_URL="https://github.com/${REPO}/releases/download/${VERSION}"

# ── Download binaries ────────────────────────────────────
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

info "Downloading forgevm ${VERSION} (${ARCH_SUFFIX})..."
curl -fSL -o "$TMPDIR/forgevm" "${RELEASE_URL}/forgevm-linux-${ARCH_SUFFIX}"
curl -fSL -o "$TMPDIR/forgevm-agent" "${RELEASE_URL}/forgevm-agent-linux-${ARCH_SUFFIX}"
curl -fSL -o "$TMPDIR/checksums.txt" "${RELEASE_URL}/checksums.txt"

# ── Verify checksums ────────────────────────────────────
info "Verifying checksums..."
cd "$TMPDIR"
# checksums.txt has names like forgevm-linux-amd64, rename for verification
sha256sum -c checksums.txt --ignore-missing 2>/dev/null || {
    # Try manual verification with renamed files
    EXPECTED_VM=$(grep "forgevm-linux-${ARCH_SUFFIX}" checksums.txt | grep -v agent | awk '{print $1}')
    EXPECTED_AGENT=$(grep "forgevm-agent-linux-${ARCH_SUFFIX}" checksums.txt | awk '{print $1}')
    ACTUAL_VM=$(sha256sum forgevm | awk '{print $1}')
    ACTUAL_AGENT=$(sha256sum forgevm-agent | awk '{print $1}')

    if [[ "$EXPECTED_VM" != "$ACTUAL_VM" ]]; then
        fail "Checksum mismatch for forgevm binary!"
    fi
    if [[ "$EXPECTED_AGENT" != "$ACTUAL_AGENT" ]]; then
        fail "Checksum mismatch for forgevm-agent binary!"
    fi
}
info "Checksums verified"
cd - > /dev/null

# ── Install binaries ─────────────────────────────────────
info "Installing to ${INSTALL_DIR}..."
chmod +x "$TMPDIR/forgevm" "$TMPDIR/forgevm-agent"

if [[ -w "$INSTALL_DIR" ]]; then
    cp "$TMPDIR/forgevm" "$INSTALL_DIR/forgevm"
    cp "$TMPDIR/forgevm-agent" "$INSTALL_DIR/forgevm-agent"
else
    sudo cp "$TMPDIR/forgevm" "$INSTALL_DIR/forgevm"
    sudo cp "$TMPDIR/forgevm-agent" "$INSTALL_DIR/forgevm-agent"
fi
info "Installed forgevm and forgevm-agent to ${INSTALL_DIR}"

# ── Install Firecracker if missing ───────────────────────
if command -v firecracker &>/dev/null; then
    FC_VER=$(firecracker --version 2>&1 | head -1)
    info "Firecracker already installed: $FC_VER"
elif [[ "$ARCH_SUFFIX" == "arm64" ]]; then
    warn "Firecracker auto-install not supported on ARM64."
    warn "On ARM64, consider using the PRoot or Docker provider instead."
    warn "Set FORGEVM_PROVIDERS_DEFAULT=proot or FORGEVM_PROVIDERS_DEFAULT=docker"
else
    warn "Firecracker not found. Installing latest release..."
    FC_VERSION=$(curl -fsSL https://api.github.com/repos/firecracker-microvm/firecracker/releases/latest | grep '"tag_name"' | cut -d'"' -f4)
    if [[ -z "$FC_VERSION" ]]; then
        warn "Could not fetch Firecracker version. Install manually: https://github.com/firecracker-microvm/firecracker/releases"
    else
        FC_ARCH="x86_64"
        FC_URL="https://github.com/firecracker-microvm/firecracker/releases/download/${FC_VERSION}/firecracker-${FC_VERSION}-${FC_ARCH}.tgz"
        FC_TMP=$(mktemp -d)
        curl -fSL -o "$FC_TMP/firecracker.tgz" "$FC_URL"
        tar -xzf "$FC_TMP/firecracker.tgz" -C "$FC_TMP"
        FC_BIN=$(find "$FC_TMP" -name "firecracker-${FC_VERSION}-${FC_ARCH}" -type f | head -1)
        if [[ -n "$FC_BIN" ]]; then
            sudo cp "$FC_BIN" /usr/local/bin/firecracker
            sudo chmod +x /usr/local/bin/firecracker
            info "Firecracker ${FC_VERSION} installed"
        else
            warn "Could not find firecracker binary in release archive"
        fi
        rm -rf "$FC_TMP"
    fi
fi

# ── Data directory + kernel ──────────────────────────────
if [[ ! -d "$DATA_DIR" ]]; then
    info "Creating ${DATA_DIR}..."
    sudo mkdir -p "$DATA_DIR"
    sudo chown "$(whoami)" "$DATA_DIR"
fi

if [[ "$ARCH_SUFFIX" == "arm64" ]]; then
    info "Skipping kernel download on ARM64 (Firecracker kernel not needed for PRoot/Docker)"
elif [[ -f "$DATA_DIR/vmlinux.bin" ]]; then
    info "Kernel already exists at $DATA_DIR/vmlinux.bin"
else
    info "Downloading Firecracker kernel..."
    curl -fSL -o "$DATA_DIR/vmlinux.bin" "$KERNEL_URL" || sudo curl -fSL -o "$DATA_DIR/vmlinux.bin" "$KERNEL_URL"
    chmod 644 "$DATA_DIR/vmlinux.bin" 2>/dev/null || sudo chmod 644 "$DATA_DIR/vmlinux.bin"
    info "Kernel downloaded"
fi

# ── Check KVM (warn only) ───────────────────────────────
if [[ -e /dev/kvm ]]; then
    if [[ -r /dev/kvm ]] && [[ -w /dev/kvm ]]; then
        info "KVM is accessible"
    else
        warn "KVM exists but no permission. Fix with: sudo chmod 666 /dev/kvm"
        warn "ForgeVM still works in mock mode without KVM."
    fi
else
    warn "KVM not available. ForgeVM will work in mock mode (no hardware VM isolation)."
    warn "For microVM support, enable virtualization in BIOS."
fi

# ── Check Docker (advisory) ─────────────────────────────
if command -v docker &>/dev/null; then
    info "Docker found (needed for custom environment images)"
else
    warn "Docker not found. Optional — needed only for custom Docker images."
    warn "Install: https://docs.docker.com/engine/install/"
fi

# ── Done ─────────────────────────────────────────────────
echo ""
echo -e "${GREEN}══════════════════════════════════════${NC}"
echo -e "${GREEN}  ForgeVM ${VERSION} installed!${NC}"
echo -e "${GREEN}══════════════════════════════════════${NC}"
echo ""
echo "  Start the server:"
echo -e "    ${BOLD}forgevm serve${NC}"
echo ""
echo "  Test it:"
echo "    curl -s -X POST localhost:7423/api/v1/sandboxes \\"
echo "      -H 'Content-Type: application/json' \\"
echo "      -d '{\"image\":\"alpine:latest\"}' | jq .id"
echo ""
echo "  For XFS reflink support (faster snapshots), run:"
echo "    ./scripts/setup.sh from the source repo"
echo ""
