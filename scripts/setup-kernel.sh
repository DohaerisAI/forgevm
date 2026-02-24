#!/bin/bash
# Downloads a prebuilt Firecracker-compatible vmlinux kernel.
# Usage: ./scripts/setup-kernel.sh [output-dir]
set -euo pipefail

KERNEL_URL="https://s3.amazonaws.com/spec.ccfc.min/img/quickstart_guide/x86_64/kernels/vmlinux.bin"
OUTPUT_DIR="${1:-/var/lib/forgevm}"
OUTPUT_FILE="${OUTPUT_DIR}/vmlinux.bin"

echo "==> Setting up Firecracker kernel"
echo "    Output: ${OUTPUT_FILE}"

mkdir -p "${OUTPUT_DIR}"

if [ -f "${OUTPUT_FILE}" ]; then
    echo "    Kernel already exists, skipping download"
    exit 0
fi

echo "    Downloading kernel..."
curl -fSL -o "${OUTPUT_FILE}" "${KERNEL_URL}"
chmod 644 "${OUTPUT_FILE}"

echo "==> Kernel downloaded successfully"
echo "    Path: ${OUTPUT_FILE}"
echo "    Size: $(du -h "${OUTPUT_FILE}" | cut -f1)"
