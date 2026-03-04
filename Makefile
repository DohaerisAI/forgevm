.PHONY: build build-agent test lint clean serve dev release-build

VERSION ?= $(shell git describe --tags --always 2>/dev/null || echo dev)

# Build the server/CLI binary
build:
	go build -ldflags="-s -w -X main.version=$(VERSION)" -o forgevm ./cmd/forgevm

# Build the guest agent (static, linux/amd64)
build-agent:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/forgevm-agent ./cmd/forgevm-agent

# Build everything
build-all: build build-agent

# Run all tests
test:
	go test ./...

# Run tests with verbose output
test-v:
	go test -v ./...

# Build the web frontend
web:
	cd web && npm install && npm run build

# Start the server
serve: build
	./forgevm serve

# Clean build artifacts
clean:
	rm -f forgevm forgevm-agent
	rm -rf bin/ web/dist/

# Run go vet
lint:
	go vet ./...

# Build static release binaries + checksums
release-build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w -X main.version=$(VERSION)" -o forgevm-linux-amd64 ./cmd/forgevm
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o forgevm-agent-linux-amd64 ./cmd/forgevm-agent
	sha256sum forgevm-linux-amd64 forgevm-agent-linux-amd64 > checksums.txt
