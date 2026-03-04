<p align="center">
  <img src="assets/logo.png" alt="ForgeVM" width="500">
</p>

<p align="center">
  <b>Your AI agent's computer. On YOUR machine.</b>
</p>

<p align="center">
  ForgeVM lets any LLM spawn isolated microVMs, run code, manage files, and destroy everything when done.<br>
  Self-hosted. Single binary. ~28ms to boot. No cloud required.
</p>

<p align="center">
  <a href="https://github.com/DohaerisAI/forgevm/releases/latest"><img src="https://img.shields.io/github/v/release/DohaerisAI/forgevm?style=flat-square&label=release" alt="Release"></a>
  <a href="https://github.com/DohaerisAI/forgevm/stargazers"><img src="https://img.shields.io/github/stars/DohaerisAI/forgevm?style=flat-square" alt="Stars"></a>
  <a href="https://github.com/DohaerisAI/forgevm/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/DohaerisAI/forgevm/ci.yml?style=flat-square&label=CI" alt="CI"></a>
  <img src="https://img.shields.io/badge/go-1.25+-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go 1.25+">
  <img src="https://img.shields.io/badge/license-MIT-green?style=flat-square" alt="MIT License">
  <img src="https://img.shields.io/badge/platform-linux-blue?style=flat-square&logo=linux&logoColor=white" alt="Linux">
</p>

---

## Why ForgeVM

|  | **ForgeVM** | **E2B** |
|--|-------------|---------|
| **Hosting** | Self-hosted (your machine) | Cloud only |
| **Isolation** | KVM microVM (hardware) | Firecracker microVM |
| **Boot time** | ~28ms (snapshot restore) | ~500ms |
| **Pricing** | Free forever | Pay per use |
| **Data privacy** | Data never leaves your machine | Data on their servers |
| **Setup** | One command | Sign up + API key |
| **Multi-user pools** | Built-in | No |
| **Python SDK** | Yes (sync + async) | Yes |
| **TypeScript SDK** | Yes | Yes |

**Key advantages:**

- **Your data stays on your machine.** No cloud API calls. No vendor lock-in. No usage billing.
- **~28ms boot** — snapshot/restore, not cold boot. Faster than every cloud alternative.
- **Drop-in E2B replacement** — same API shape, same SDKs, point at localhost instead of `api.e2b.dev`.
- **Real VM isolation** — KVM hardware virtualization, not containers. AI-generated code can't escape.

---

## Install

### One command (recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/DohaerisAI/forgevm/main/scripts/install.sh | bash
```

Downloads pre-built binaries, installs Firecracker, downloads the kernel. No Go required.

### Build from source

```bash
git clone https://github.com/DohaerisAI/forgevm && cd forgevm
./scripts/setup.sh
```

Requires Go 1.25+, Docker, and KVM. Sets up XFS reflink for fastest possible snapshot restores.

---

## Quick Start

```bash
# 1. Install
curl -fsSL https://raw.githubusercontent.com/DohaerisAI/forgevm/main/scripts/install.sh | bash

# 2. Start the server
forgevm serve

# 3. Spawn a sandbox, run code, destroy it
SANDBOX=$(curl -s -X POST localhost:7423/api/v1/sandboxes \
  -H "Content-Type: application/json" \
  -d '{"image":"alpine:latest"}' | jq -r .id)

curl -s -X POST localhost:7423/api/v1/sandboxes/$SANDBOX/exec \
  -H "Content-Type: application/json" \
  -d '{"command":"echo hello from ForgeVM"}'
# {"exit_code":0,"stdout":"hello from ForgeVM\n"}

curl -s -X DELETE localhost:7423/api/v1/sandboxes/$SANDBOX
```

---

## Give Your LLM a Computer

ForgeVM turns any LLM into a coding agent. Define `execute_code` and `write_file` as tools, and let the model call them — ForgeVM handles sandboxed execution.

### OpenAI function calling

```python
import openai
import requests

FORGEVM = "http://localhost:7423/api/v1"

# Spawn a sandbox for this conversation
sb = requests.post(f"{FORGEVM}/sandboxes", json={"image": "python:3.12"}).json()
sandbox_id = sb["id"]

tools = [
    {
        "type": "function",
        "function": {
            "name": "execute_code",
            "description": "Execute a shell command in a sandboxed VM",
            "parameters": {
                "type": "object",
                "properties": {"command": {"type": "string"}},
                "required": ["command"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "write_file",
            "description": "Write a file in the sandbox",
            "parameters": {
                "type": "object",
                "properties": {
                    "path": {"type": "string"},
                    "content": {"type": "string"},
                },
                "required": ["path", "content"],
            },
        },
    },
]


def handle_tool_call(name, args):
    if name == "execute_code":
        r = requests.post(f"{FORGEVM}/sandboxes/{sandbox_id}/exec", json=args)
        return r.json()
    elif name == "write_file":
        r = requests.post(
            f"{FORGEVM}/sandboxes/{sandbox_id}/files",
            json={"path": args["path"], "content": args["content"]},
        )
        return r.json()


client = openai.OpenAI()
messages = [{"role": "user", "content": "Write a Python script that prints the first 10 primes, then run it"}]

while True:
    response = client.chat.completions.create(model="gpt-4o", messages=messages, tools=tools)
    msg = response.choices[0].message
    messages.append(msg)

    if msg.tool_calls:
        for tc in msg.tool_calls:
            import json
            result = handle_tool_call(tc.function.name, json.loads(tc.function.arguments))
            messages.append({"role": "tool", "tool_call_id": tc.id, "content": json.dumps(result)})
    else:
        print(msg.content)
        break

# Clean up
requests.delete(f"{FORGEVM}/sandboxes/{sandbox_id}")
```

### Claude tool_use

```python
import anthropic
import requests
import json

FORGEVM = "http://localhost:7423/api/v1"
sb = requests.post(f"{FORGEVM}/sandboxes", json={"image": "python:3.12"}).json()
sandbox_id = sb["id"]

tools = [
    {
        "name": "execute_code",
        "description": "Execute a shell command in a sandboxed VM",
        "input_schema": {
            "type": "object",
            "properties": {"command": {"type": "string"}},
            "required": ["command"],
        },
    },
    {
        "name": "write_file",
        "description": "Write a file in the sandbox",
        "input_schema": {
            "type": "object",
            "properties": {"path": {"type": "string"}, "content": {"type": "string"}},
            "required": ["path", "content"],
        },
    },
]

client = anthropic.Anthropic()
messages = [{"role": "user", "content": "Write a Python script that calculates fibonacci numbers, then run it"}]

while True:
    response = client.messages.create(model="claude-sonnet-4-20250514", max_tokens=4096, tools=tools, messages=messages)

    # Collect assistant response
    messages.append({"role": "assistant", "content": response.content})

    if response.stop_reason == "tool_use":
        tool_results = []
        for block in response.content:
            if block.type == "tool_use":
                if block.name == "execute_code":
                    r = requests.post(f"{FORGEVM}/sandboxes/{sandbox_id}/exec", json=block.input)
                    result = r.json()
                elif block.name == "write_file":
                    r = requests.post(f"{FORGEVM}/sandboxes/{sandbox_id}/files", json={"path": block.input["path"], "content": block.input["content"]})
                    result = r.json()
                tool_results.append({"type": "tool_result", "tool_use_id": block.id, "content": json.dumps(result)})
        messages.append({"role": "user", "content": tool_results})
    else:
        for block in response.content:
            if hasattr(block, "text"):
                print(block.text)
        break

requests.delete(f"{FORGEVM}/sandboxes/{sandbox_id}")
```

**That's it.** Any LLM that supports tool calling can now write and execute code safely on your machine.

---

## Use with Python

```bash
pip install forgevm
```

```python
from forgevm import Client

client = Client("http://localhost:7423")

# Spawn and execute
sandbox = client.spawn(image="alpine:latest")
result = sandbox.exec("echo hello world")
print(result.stdout)  # "hello world\n"

# File operations
sandbox.write_file("/app/main.py", 'print("built with ForgeVM")')
result = sandbox.exec("python3 /app/main.py")
print(result.stdout)  # "built with ForgeVM\n"

# Extended file operations
content = sandbox.read_file("/etc/hostname")
files = sandbox.list_files("/app")
sandbox.move_file("/app/main.py", "/app/app.py")
sandbox.chmod_file("/app/app.py", "755")
info = sandbox.stat_file("/app/app.py")
matches = sandbox.glob_files("/app/*.py")
sandbox.delete_file("/app/app.py")

# Extend TTL
sandbox.extend_ttl("30m")

# Stream output in real-time
for chunk in sandbox.exec_stream("ping -c 3 localhost"):
    print(chunk.data, end="")

# Pool status (multi-user mode)
status = client.pool_status()
print(status)

# Clean up
sandbox.destroy()
```

**Context manager** — auto-destroys on exit:

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

// File operations
await sandbox.writeFile("/app/index.js", 'console.log("hi")');
await sandbox.exec("node /app/index.js");

// Extended file operations
const content = await sandbox.readFile("/etc/hostname");
const files = await sandbox.listFiles("/app");
await sandbox.moveFile("/app/index.js", "/app/app.js");
await sandbox.chmodFile("/app/app.js", "755");
const info = await sandbox.statFile("/app/app.js");
const matches = await sandbox.globFiles("/app/*.js");
await sandbox.deleteFile("/app/app.js");

// Extend TTL
await sandbox.extendTtl("30m");

// Stream output
for await (const chunk of sandbox.execStream("ping -c 3 localhost")) {
  process.stdout.write(chunk.data);
}

await sandbox.destroy();
```

---

## Multi-User Pools

Run a shared pool of VMs that multiple users (or agents) share. Each user gets an isolated session within a pre-warmed VM.

```yaml
# forgevm.yaml
pool:
  enabled: true
  max_vms: 10
  max_users_per_vm: 5
  image: "python:3.12"
  memory_mb: 2048
  vcpus: 2
  overflow: "reject"  # or "queue"
```

```bash
# Check pool status
curl -s localhost:7423/api/v1/pool/status | jq
```

```json
{
  "total_vms": 10,
  "active_users": 23,
  "available_slots": 27,
  "vms": [
    {"id": "sb-a1b2c3d4", "users": 3, "max_users": 5, "memory_mb": 2048}
  ]
}
```

VMs are pre-warmed from snapshots (~28ms). When a user connects, they get assigned to a VM with available capacity. When `overflow` is `"reject"`, new requests fail with 503 if all slots are full. With `"queue"`, they wait for a slot.

---

## CLI

```bash
forgevm serve                                     # start server
forgevm spawn --image alpine:latest --ttl 1h      # spawn a sandbox
forgevm list                                       # list sandboxes
forgevm exec sb-a1b2c3d4 -- echo hello world      # run a command
forgevm kill sb-a1b2c3d4                           # destroy
forgevm build-image python:3.12                    # pre-build rootfs from Docker image
forgevm tui                                        # interactive dashboard
forgevm version                                    # print version
```

---

## REST API

Base: `http://localhost:7423/api/v1`

### Sandboxes

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/sandboxes` | Create a sandbox |
| `GET` | `/sandboxes` | List all sandboxes |
| `DELETE` | `/sandboxes` | Prune expired sandboxes |
| `GET` | `/sandboxes/:id` | Get sandbox details |
| `DELETE` | `/sandboxes/:id` | Destroy a sandbox |
| `POST` | `/sandboxes/:id/extend` | Extend TTL |
| `POST` | `/sandboxes/:id/exec` | Execute a command |
| `GET` | `/sandboxes/:id/exec/ws` | Execute via WebSocket |
| `POST` | `/sandboxes/:id/files` | Write a file |
| `GET` | `/sandboxes/:id/files` | Read a file |
| `DELETE` | `/sandboxes/:id/files` | Delete a file |
| `GET` | `/sandboxes/:id/files/list` | List directory |
| `POST` | `/sandboxes/:id/files/move` | Move/rename file |
| `POST` | `/sandboxes/:id/files/chmod` | Change permissions |
| `GET` | `/sandboxes/:id/files/stat` | File stat info |
| `GET` | `/sandboxes/:id/files/glob` | Glob pattern match |
| `GET` | `/sandboxes/:id/logs` | Console logs |

### Templates

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/templates` | Create a template |
| `GET` | `/templates` | List templates |
| `GET` | `/templates/:name` | Get template |
| `PUT` | `/templates/:name` | Update template |
| `DELETE` | `/templates/:name` | Delete template |
| `POST` | `/templates/:name/spawn` | Spawn from template |

### Providers

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/providers` | List providers |
| `POST` | `/providers/test` | Test provider health |
| `GET` | `/providers/:name` | Provider details |

### Snapshots

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/snapshots` | List snapshots |

### Environments

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/environments/specs` | Create environment spec |
| `GET` | `/environments/specs` | List specs |
| `GET` | `/environments/specs/:id` | Get spec |
| `GET` | `/environments/specs/:id/suggestions` | Package suggestions |
| `POST` | `/environments/builds` | Start build |
| `GET` | `/environments/builds` | List builds |
| `GET` | `/environments/builds/:id` | Get build |
| `POST` | `/environments/builds/:id/cancel` | Cancel build |
| `GET` | `/environments/builds/:id/spawn-config` | Spawn config |
| `POST` | `/environments/registry-connections` | Save registry credentials |
| `GET` | `/environments/registry-connections` | List connections |
| `DELETE` | `/environments/registry-connections/:id` | Delete connection |

### System

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/health` | Health check |
| `GET` | `/metrics` | Runtime metrics |
| `GET` | `/events` | SSE event stream |
| `GET` | `/pool/status` | VM pool status |

---

## Providers

ForgeVM uses a **provider interface** — swap backends without changing your code.

### Mock (default — zero setup)

Runs commands with `os/exec` in temp directories on the host. No VMs. Ships enabled by default for development and testing. Supports custom Docker images via `build-image`.

### Firecracker (production)

Real Firecracker microVMs with KVM hardware isolation. Each sandbox gets its own kernel, rootfs, and network namespace.

```bash
# Full setup (builds from source + XFS reflink)
./scripts/setup.sh

# Or with pre-built binaries
curl -fsSL https://raw.githubusercontent.com/DohaerisAI/forgevm/main/scripts/install.sh | bash
sudo chmod 666 /dev/kvm

# Enable in config
cat > forgevm.yaml <<EOF
providers:
  default: "firecracker"
  firecracker:
    enabled: true
EOF
```

First spawn cold-boots (~1s) and creates a snapshot. Every spawn after that restores from snapshot in **~28ms**.

### E2B (cloud)

Forwards operations to [E2B](https://e2b.dev) cloud API.

### Custom HTTP

Point ForgeVM at any HTTP endpoint that implements the provider protocol.

---

## Configuration

```yaml
# forgevm.yaml (optional — sane defaults without it)
server:
  host: "0.0.0.0"
  port: 7423

providers:
  default: "firecracker"    # or "mock", "e2b", "custom"
  firecracker:
    enabled: true
    firecracker_path: "/usr/local/bin/firecracker"
    kernel_path: "/var/lib/forgevm/vmlinux.bin"
    agent_path: "./bin/forgevm-agent"
    data_dir: "/var/lib/forgevm"

defaults:
  ttl: "30m"
  image: "alpine:latest"
  memory_mb: 1024
  vcpus: 1

pool:
  enabled: false
  max_vms: 10
  max_users_per_vm: 5
  image: "python:3.12"
  memory_mb: 2048
  vcpus: 2
  overflow: "reject"

auth:
  enabled: false
  api_key: "your-secret-key"
```

Config priority: `./forgevm.yaml` > `~/.forgevm/config.yaml` > env vars (`FORGEVM_SERVER_PORT=8080`)

---

## Web Dashboard

Built-in React dashboard with sandbox management, live terminal, file browser, and log viewer.

```bash
make web          # build frontend
forgevm serve     # open http://localhost:7423
```

---

## Security

### Why VMs > Containers for AI Agents

AI agents generate and run **untrusted code**. Containers share the host kernel — a single kernel exploit means host compromise. ForgeVM uses Firecracker microVMs: each sandbox gets its own kernel with KVM hardware isolation. Even if AI-generated code exploits a kernel vulnerability, it only affects that sandbox's kernel, not the host.

- **KVM isolation** — each sandbox = its own kernel, rootfs, network
- **No shared kernel** — guest exploits can't reach the host
- **Ephemeral rootfs** — destroyed on teardown, nothing persists
- **vsock only** — host/guest communicate over virtio-vsock, zero network exposure
- **API key auth** — optional, on all endpoints
- **Auto-expiry** — sandboxes destroyed after TTL

See [SECURITY.md](SECURITY.md) for vulnerability reporting.

---

## vs E2B

| | **ForgeVM** | **E2B** |
|--|-------------|---------|
| Cost | Free forever | $0.000075/s per sandbox |
| Data location | Your machine | Their cloud |
| Latency | ~28ms (local) | ~500ms + network RTT |
| Internet required | No | Yes |
| Max sandboxes | Your hardware limit | Plan-dependent |
| Custom images | Any Docker image | E2B templates only |
| Open source | MIT | Partial |

---

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                        Your LLM / Agent                      │
│                   (Python SDK / TS SDK / curl)                │
└──────────────────────┬───────────────────────────────────────┘
                       │ HTTP / WebSocket / SSE
                       ▼
┌──────────────────────────────────────────────────────────────┐
│                      ForgeVM Server                          │
│                                                              │
│  ┌─────────┐  ┌──────────────┐  ┌─────────┐  ┌──────────┐  │
│  │ REST API │  │ Orchestrator │  │  Store   │  │  Events  │  │
│  │ Chi +    │  │ Lifecycle,   │  │ SQLite   │  │  SSE     │  │
│  │ WebSocket│  │ TTL, Pools,  │  │ WAL mode │  │  bus     │  │
│  │ + SSE    │  │ Templates    │  │          │  │          │  │
│  └────┬─────┘  └──────┬───────┘  └──────────┘  └──────────┘  │
│       │               │                                      │
│       └───────┬───────┘                                      │
│               ▼                                              │
│  ┌─────────────────────────────────────────────────────────┐ │
│  │              Provider Interface                          │ │
│  │              ┌───────────────┐                           │ │
│  │              │  Firecracker   │                           │ │
│  │              │  KVM microVMs  │                           │ │
│  │              └───────┬───────┘                           │ │
│  └──────────────────────┼──────────────────────────────────┘ │
└───────────┼──────────────────────────────────────────────────┘
            │ vsock (virtio)
            ▼
┌──────────────────────────────────────────────────────────────┐
│                   Firecracker microVM                        │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐   │
│  │                  forgevm-agent (PID 1)                │   │
│  │                                                       │   │
│  │  • Exec commands (/bin/sh -c "...")                   │   │
│  │  • Read/write files                                   │   │
│  │  • Stream stdout/stderr                               │   │
│  │  • Length-prefixed JSON protocol over vsock            │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                              │
│  Dedicated kernel · Ephemeral rootfs · KVM isolated          │
└──────────────────────────────────────────────────────────────┘
```

**Key flow:** SDK -> REST API -> Orchestrator -> Provider -> Firecracker -> vsock -> Guest Agent -> execute -> response back

**Snapshot/restore:** First spawn cold-boots (~1s) and snapshots. Every spawn after restores in ~28ms.

---

## Development

```bash
make build        # build server binary
make build-agent  # build guest agent (static linux/amd64)
make build-all    # build both
make test         # run all tests
make web          # build frontend
make lint         # go vet
make release-build # static release binaries + checksums
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[MIT](LICENSE)
