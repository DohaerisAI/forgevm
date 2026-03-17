#!/bin/bash
set -euo pipefail

export PATH="$HOME/.local/go/bin:$HOME/go/bin:$PATH"

echo "Building forgevm..."
go build -o forgevm ./cmd/forgevm

echo "Building forgevm-agent (static linux/amd64)..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/forgevm-agent ./cmd/forgevm-agent

echo "Done. Run with: ./forgevm serve"
