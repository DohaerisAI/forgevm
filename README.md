<h1 align="center">ForgeVM</h1>

<p align="center">
  <b>Your AI agent just got its own computer.</b>
</p>

<p align="center">
  ForgeVM lets any LLM spawn isolated microVMs, run code, manage files, and destroy everything when done.<br>
  Self-hosted. Single binary. ~28ms to boot. No cloud required.
</p>

<p align="center">
  <img src="https://img.shields.io/badge/go-1.25+-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go 1.25+">
  <img src="https://img.shields.io/badge/license-MIT-green?style=flat-square" alt="MIT License">
  <img src="https://img.shields.io/badge/platform-linux-blue?style=flat-square&logo=linux&logoColor=white" alt="Linux">
</p>

---

## The Problem

You're building an AI agent. It needs to run code. You have two options:

1. **Docker** -- shares the host kernel. One escape and your machine is owned.
2. **Cloud sandboxes (E2B, etc.)** -- sends your data to someone else's servers. Costs money. Adds latency.

Neither is great when your agent is running **untrusted, AI-generated code** on **your own machine**.

## The Solution

ForgeVM gives your AI agent its own **real virtual machine** -- hardware-level KVM isolation via Firecracker microVMs. Not containers. Not namespaces. Actual separate kernels.

```
Your LLM  -->  ForgeVM API  -->  Firecracker microVM (~28ms)
                                  |-- exec("python3 sketch.py")
                                  |-- read/write files
                                  |-- stream stdout in real-time
                                  \-- destroy --> gone. forever.
```

One binary. One command. Your hardware. Your data.

---

## Install

### Option 1: Download binary (fastest)

```bash
# Download latest release
curl -fsSL https://github.com/mainadwitiya/forgevm/releases/latest/download/forgevm-linux-amd64 -o forgevm
chmod +x forgevm
sudo mv forgevm /usr/local/bin/
```

### Option 2: Build from source

**Prerequisites:** Go 1.25+ ([install](https://go.dev/dl/))

```bash
git clone https://github.com/mainadwitiya/forgevm
cd forgevm
make build
sudo mv forgevm /usr/local/bin/
```

### Option 3: Docker

```bash
docker run -p 7423:7423 ghcr.io/mainadwitiya/forgevm
```

### Verify

```bash
forgevm version
```

---

## Start the server

```bash
forgevm serve
# ForgeVM listening on http://localhost:7423
```

That's it. The **mock provider** is enabled by default -- it runs commands on the host using temp directories. No VMs, no KVM, no extra setup. Perfect for trying things out.

> For real VM isolation, see [Firecracker setup](#firecracker-production) below.

### Quick test

```bash
# Spawn
curl -s -X POST localhost:7423/api/v1/sandboxes \
  -H "Content-Type: application/json" \
  -d '{"image":"alpine:latest"}' | jq .id
# "sb-a1b2c3d4"

# Run code
curl -s -X POST localhost:7423/api/v1/sandboxes/sb-a1b2c3d4/exec \
  -H "Content-Type: application/json" \
  -d '{"command":"echo hello from ForgeVM"}'
# {"exit_code":0,"stdout":"hello from ForgeVM\n"}

# Destroy
curl -s -X DELETE localhost:7423/api/v1/sandboxes/sb-a1b2c3d4
```

---

## Use with Python

```bash
pip install forgevm
```

```python
from forgevm import Client

client = Client("http://localhost:7423")

# Spawn a sandbox and run code
sandbox = client.spawn(image="alpine:latest")
result = sandbox.exec("echo hello world")
print(result.stdout)  # "hello world\n"

# File operations
sandbox.write_file("/app/main.py", 'print("built with ForgeVM")')
result = sandbox.exec("python3 /app/main.py")
print(result.stdout)  # "built with ForgeVM\n"

# Extend TTL if you need more time
sandbox.extend_ttl("30m")

# Stream output in real-time
for chunk in sandbox.exec_stream("ping -c 3 localhost"):
    print(chunk.data, end="")

# Clean up
sandbox.destroy()
```

**Context manager** -- auto-destroys on exit:

```python
with client.spawn(image="python:3.12") as sb:
    sb.exec("pip install requests")
    sb.exec("python3 -c 'import requests; print(requests.get(\"https://httpbin.org/ip\").text)'")
# sandbox destroyed automatically
```

**Async support:**

```python
from forgevm import AsyncClient

async with AsyncClient("http://localhost:7423") as client:
    sandbox = await client.spawn(image="alpine:latest")
    result = await sandbox.exec("whoami")
    await sandbox.destroy()
```

---

## Use with TypeScript

```bash
npm install forgevm
```

```typescript
import { Client } from "forgevm";

const client = new Client("http://localhost:7423");

// Spawn and execute
const sandbox = await client.spawn({ image: "alpine:latest" });
const result = await sandbox.exec("echo hello world");
console.log(result.stdout); // "hello world\n"

// Files
await sandbox.writeFile("/app/index.js", 'console.log("hi")');
await sandbox.exec("node /app/index.js");

// Extend TTL
await sandbox.extendTtl("30m");

// Stream output
for await (const chunk of sandbox.execStream("ping -c 3 localhost")) {
  process.stdout.write(chunk.data);
}

await sandbox.destroy();
```

---

## CLI

```bash
forgevm serve                                     # start server
forgevm spawn --image alpine:latest --ttl 1h      # spawn a sandbox
forgevm list                                       # list sandboxes
forgevm exec sb-a1b2c3d4 -- echo hello world      # run a command
forgevm kill sb-a1b2c3d4                           # destroy
forgevm tui                                        # interactive dashboard
```

---

## REST API

Base: `http://localhost:7423/api/v1`

| Method | Endpoint | What it does |
|--------|----------|-------------|
| `POST` | `/sandboxes` | Spawn a sandbox |
| `GET` | `/sandboxes` | List all |
| `GET` | `/sandboxes/:id` | Get details |
| `DELETE` | `/sandboxes/:id` | Destroy |
| `POST` | `/sandboxes/:id/extend` | Extend TTL |
| `POST` | `/sandboxes/:id/exec` | Run a command |
| `GET` | `/sandboxes/:id/exec/ws` | Run via WebSocket |
| `POST` | `/sandboxes/:id/files` | Write a file |
| `GET` | `/sandboxes/:id/files?path=` | Read a file |
| `GET` | `/sandboxes/:id/files/list` | List files |
| `GET` | `/sandboxes/:id/logs` | Console logs |
| `GET` | `/health` | Health check |
| `GET` | `/events` | SSE event stream |
| `POST` | `/templates` | Create template |
| `POST` | `/templates/:name/spawn` | Spawn from template |

---

## Providers

ForgeVM uses a **provider interface**. Swap backends without changing your code.

### Mock (default -- zero setup)

Runs commands with `os/exec` in temp directories on the host. No VMs. Ships enabled by default for development and testing.

### Firecracker (production)

Real Firecracker microVMs with KVM hardware isolation. Each sandbox gets its own kernel, rootfs, and network namespace.

**Prerequisites:**
- Linux with KVM support (`ls /dev/kvm`)
- [Firecracker binary](https://github.com/firecracker-microvm/firecracker/releases)

```bash
# 1. Setup kernel + rootfs
./scripts/setup-kernel.sh
sudo ./scripts/build-rootfs.sh alpine:latest

# 2. Build guest agent
make build-agent

# 3. Enable in config
cat > forgevm.yaml <<EOF
providers:
  default: "firecracker"
  firecracker:
    enabled: true
    firecracker_path: "/usr/local/bin/firecracker"
    kernel_path: "/var/lib/forgevm/vmlinux.bin"
    agent_path: "./bin/forgevm-agent"
    data_dir: "/var/lib/forgevm"
EOF

# 4. Start
forgevm serve
```

First spawn cold-boots (~1s) and creates a snapshot. Every spawn after that restores from snapshot in **~28ms**.

### E2B (cloud)

Forwards operations to [E2B](https://e2b.dev) cloud API.

### Custom HTTP

Point ForgeVM at any HTTP endpoint that implements the provider protocol.

---

## Web Dashboard

Built-in React dashboard with sandbox management, live terminal, file browser, and log viewer.

```bash
make web          # build frontend
forgevm serve     # open http://localhost:7423
```

---

## Configuration

```yaml
# forgevm.yaml (optional -- sane defaults without it)
server:
  host: "0.0.0.0"
  port: 7423

providers:
  default: "mock"

defaults:
  ttl: "30m"
  image: "alpine:latest"
  memory_mb: 512
  vcpus: 1

auth:
  enabled: false
  api_key: "your-secret-key"
```

Config: `./forgevm.yaml` > `~/.forgevm/config.yaml` > env vars (`FORGEVM_SERVER_PORT=8080`)

---

## How it Compares

| | **ForgeVM** | E2B | Daytona | microsandbox |
|---|---|---|---|---|
| **Self-hosted** | Single binary | Terraform | Yes | Yes |
| **Isolation** | Firecracker KVM | Firecracker KVM | Docker | libkrun |
| **Dependencies** | Just Go | Cloud infra | Docker + k8s | Rust toolchain |
| **Spawn speed** | ~28ms (snapshot) | ~150ms | ~27ms | Unknown |
| **SDKs** | Python, TypeScript | Python, JS/TS | Multiple | Python, JS/TS |
| **Web dashboard** | Built-in | No | No | No |
| **TUI** | Built-in | No | No | No |
| **Templates + Pools** | Built-in | Paid | Config | No |
| **License** | MIT | Apache-2.0 | Apache-2.0 | Apache-2.0 |
| **Cloud required** | Never | Optional | Optional | Never |

---

## Security

- **KVM isolation** -- each sandbox = its own kernel, rootfs, network
- **No shared kernel** -- guest exploits can't reach the host
- **Ephemeral rootfs** -- destroyed on teardown, nothing persists
- **vsock only** -- host/guest communicate over virtio-vsock, zero network exposure
- **API key auth** -- optional, on all endpoints
- **Auto-expiry** -- sandboxes destroyed after TTL

See [SECURITY.md](SECURITY.md) for vulnerability reporting.

---

## Architecture

```
cmd/forgevm/              CLI + server
cmd/forgevm-agent/        Guest agent (runs inside VMs)
internal/
  agentproto/             Wire protocol (host <-> guest)
  api/                    REST API + WebSocket + SSE
  orchestrator/           Lifecycle, events, pools, templates
  providers/              Firecracker, mock, E2B, custom
  store/                  SQLite persistence
sdk/
  python/                 Python SDK
  js/                     TypeScript SDK
web/                      React dashboard
tui/                      Terminal dashboard
```

---

## Development

```bash
make build        # build server binary
make build-agent  # build guest agent (static linux/amd64)
make test         # run all tests
make web          # build frontend
make lint         # go vet
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[MIT](LICENSE)
