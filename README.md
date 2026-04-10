<p align="center">
  <img src="assets/logo.png" alt="ForgeVM" width="200"/>
</p>

<h3 align="center"><b>Your AI agent just got its own computer.</b></h3>

<p align="center">
Like E2B but self-hosted. Like Docker but actually isolated. Like Daytona but one binary.
</p>

<p align="center">
One tool. Every isolation level. Every platform.<br><br>
On a Mac? Docker provider, no KVM needed.<br>
On bare metal? Firecracker microVMs in ~28ms.<br>
On Kubernetes? gVisor or Kata containers.<br>
Need 100 sandboxes but only have 20 VMs? Pool mode.<br><br>
Self-hosted. Single binary. Python &amp; TypeScript SDKs. MIT licensed. No cloud required.
</p>

<p align="center">
  <a href="https://github.com/DohaerisAI/forgevm/stargazers"><img src="https://img.shields.io/github/stars/DohaerisAI/forgevm?style=flat-square" alt="Stars"/></a>
  <a href="https://github.com/DohaerisAI/forgevm/network/members"><img src="https://img.shields.io/github/forks/DohaerisAI/forgevm?style=flat-square" alt="Forks"/></a>
  <a href="https://github.com/DohaerisAI/forgevm/issues"><img src="https://img.shields.io/github/issues/DohaerisAI/forgevm?style=flat-square" alt="Issues"/></a>
  <img src="https://img.shields.io/badge/go-1.25+-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go"/>
  <img src="https://img.shields.io/badge/license-MIT-green?style=flat-square" alt="MIT"/>
  <img src="https://img.shields.io/badge/platform-linux%20%7C%20mac%20%7C%20windows-blue?style=flat-square" alt="Platform"/>
</p>

<p align="center">
  <a href="#quick-start-30-seconds">Quick Start</a> •
  <a href="#why-forgevm">Why ForgeVM</a> •
  <a href="#pick-your-isolation-level">Providers</a> •
  <a href="#pool-mode">Pool Mode</a> •
  <a href="docs/">Docs</a>
</p>

<!-- TODO: Replace with actual demo GIF — record with: vhs demo.tape -->
<!-- <p align="center"><img src="assets/demo.gif" width="640"/></p> -->

---

## Quick start (30 seconds)

```bash
git clone https://github.com/DohaerisAI/forgevm && cd forgevm
./scripts/setup.sh
./forgevm serve
# ForgeVM listening on http://localhost:7423
```

```python
pip install forgevm
```

```python
from forgevm import Client

client = Client("http://localhost:7423")
sandbox = client.spawn(image="python:3.12")

result = sandbox.exec('print("hello from my own computer")')
print(result.stdout)  # hello from my own computer

sandbox.destroy()  # gone. forever.
```

7 lines. Your AI agent now has a real, isolated machine it can use and throw away.

---

## Why ForgeVM?

You're building an AI agent. It generates code. That code needs to run somewhere safe.

**The problem:**

- **Docker** shares the host kernel. One container escape and your machine is owned. Multiple [runc CVEs in 2024-2025](https://github.com/opencontainers/runc/security/advisories) proved this isn't theoretical.
- **Cloud sandboxes** (E2B, Modal) send your code and data to someone else's servers. Adds latency, costs money, and you lose control of your data.
- **Daytona** is self-hostable but needs [12 services](https://www.daytona.io/docs/en/oss-deployment) (PostgreSQL, Redis, MinIO, Dex, registry...) just to get started.
- **Zeroboot** is blazing fast (~0.8ms) but strips everything — no networking, no filesystem, no multi-vCPU, serial-only I/O. Built for "run a function, get a result."

**ForgeVM is one binary.** Self-hosted. Boots a sandbox in ~28ms. Your data never leaves your machine. And you choose the isolation level — Docker containers for dev, gVisor for cloud VMs, Firecracker microVMs for maximum hardware-level security.

| | ForgeVM | E2B | Zeroboot | Daytona | Modal | Raw Docker |
|---|:---:|:---:|:---:|:---:|:---:|:---:|
| Self-hosted | ✅ | ❌ Cloud only | ✅ | ✅ (12 services) | ❌ Cloud only | ✅ |
| Isolation | KVM + gVisor + Docker | Container | KVM only | Container | Container | Shared kernel |
| Cold boot | **~28ms** (snapshot) | ~500ms | **~0.8ms** (CoW fork) | Seconds | Seconds | ~200ms |
| Networking | ✅ | ✅ | ❌ Serial only | ✅ | ✅ | ✅ |
| Filesystem / disk I/O | ✅ | ✅ | ❌ Memory only | ✅ | ✅ | ✅ |
| Multi-vCPU | ✅ | ✅ | ❌ Single vCPU | ✅ | ✅ | ✅ |
| Multiple providers | ✅ KVM/Docker/gVisor | ❌ | ❌ KVM only | ❌ | ❌ | N/A |
| Runs without KVM | ✅ Docker provider | N/A | ❌ | ✅ | N/A | ✅ |
| Multi-user pool mode | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| File API (read/write/glob) | ✅ 16 methods | ✅ | ❌ | ❌ | ❌ | ❌ |
| Python + TS SDKs | ✅ | ✅ | ✅ | ❌ | ✅ | ❌ |
| Your data stays local | ✅ | ❌ | ✅ | ✅ | ❌ | ✅ |
| License | MIT | Partial | Apache 2.0 | Apache 2.0 | Proprietary | N/A |

> **On speed:** Zeroboot's 0.8ms is real — they bypass Firecracker's VMM entirely and `mmap(MAP_PRIVATE)` the snapshot memory as copy-on-write. But there's no disk, no network, and I/O is serial UART only. ForgeVM's 28ms gives you a full sandbox with networking, file system, virtio, and multi-vCPU. Different tools for different jobs.

### The math

E2B charges per second. Default sandbox = 2 vCPU + 512 MiB RAM:

```
2 vCPU:    $0.000028/s
512 MiB:   $0.0000045/GiB/s × 0.5 GiB = $0.00000225/s
─────────────────────────────────────────
Total:     $0.00003025/s = $0.109/hour per sandbox
```

| Concurrent sandboxes | E2B / month | ForgeVM pool mode |
|---|---|---|
| 10 | $261 compute + $150 plan = **$411** | **$0** |
| 50 | $1,307 compute + $150 plan = **$1,457** | **$0** |
| 100 | $2,614 compute + $150 plan = **$2,764** | **$0** |

_Assumes 8h/day active. ForgeVM pool mode: 5 users per VM, your own infra._

---

## Pick your isolation level

ForgeVM has a **provider interface**. One config change swaps the entire backend. Your application code doesn't change.

```yaml
# forgevm.yaml — change one line
providers:
  default: "docker"  # or "firecracker" or "mock"
  docker:
    runtime: "runc"  # or "runsc" (gVisor) or "kata-runtime"
```

| Provider | What it does | KVM? | Boot | Use when |
|---|---|:---:|---|---|
| **Firecracker** | Real microVM. Own kernel, rootfs, network. ~28ms via snapshot restore. | Yes | ~28ms | Production. Maximum isolation. |
| **Docker** (runc) | OCI container with seccomp, cap_drop ALL, read-only rootfs, no network. | No | ~200ms | Dev, CI/CD, Mac, Windows. |
| **Docker** (gVisor) | Same as above, but syscalls hit a user-space kernel instead of host. | No | ~400ms | Cloud VMs. Stronger than containers. |
| **Docker** (Kata) | Lightweight VM per container. Hardware isolation without Firecracker setup. | Yes | ~1s | Kubernetes (AKS/GKE). |
| **Mock** | Temp directories on host. Zero overhead. | No | Instant | Testing, development. |

Every provider implements the same 16-method interface. SDKs, API, CLI, pool mode — all work identically regardless of backend.

---

## Pool mode — the feature nobody else has

Traditional sandbox tools: 1 user = 1 VM. 100 users = 100 VMs = massive bill.

ForgeVM pool mode: **1 VM serves N users.** Each gets an isolated `/workspace/{id}/`. Path traversal blocked. Optional per-user UID + PID namespace hardening.

```yaml
pool:
  enabled: true
  max_vms: 20
  max_users_per_vm: 5
  image: "python:3.12-slim"
  memory_mb: 2048
```

**100 users → 20 VMs instead of 100. 60% less infrastructure. Same isolation guarantees.**

Pool mode works with every provider — Docker containers, Firecracker microVMs, gVisor, Kata. The orchestrator handles user-to-VM assignment, workspace scoping, and cleanup automatically.

---

## SDKs

<table>
<tr>
<td width="50%"><b>Python</b></td>
<td width="50%"><b>TypeScript</b></td>
</tr>
<tr>
<td>

```python
from forgevm import Client

client = Client("http://localhost:7423")

# Context manager — auto-destroys on exit
with client.spawn(image="python:3.12") as sb:
    sb.exec("pip install pandas")
    sb.write_file("/app/analyze.py", code)
    result = sb.exec("python3 /app/analyze.py")
    print(result.stdout)

# Async support
from forgevm import AsyncClient
async with AsyncClient(url) as client:
    sb = await client.spawn()
    result = await sb.exec("whoami")
```

</td>
<td>

```typescript
import { Client } from "forgevm";

const client = new Client("http://localhost:7423");
const sb = await client.spawn({ image: "node:20" });

// Files + exec
await sb.writeFile("/app/index.js", code);
const result = await sb.exec("node /app/index.js");
console.log(result.stdout);

// Stream output in real-time
for await (const chunk of sb.execStream("npm test")) {
  process.stdout.write(chunk.data);
}

await sb.destroy();
```

</td>
</tr>
</table>

```bash
pip install forgevm    # Python
npm install forgevm    # TypeScript
```

---

## Security defaults

Every sandbox ships locked down. You opt *in* to less restriction, not out.

| Layer | Default | What it does |
|---|---|---|
| Capabilities | `cap_drop: ALL` | Can't mount, ptrace, load modules, change networking |
| Syscalls | Seccomp default profile | Blocks ~44 dangerous syscalls |
| Filesystem | Read-only rootfs | Only `/tmp` and `/workspace` writable (tmpfs, size-capped) |
| Network | `mode: none` | Zero outbound access. Can't phone home or exfiltrate. |
| Processes | PID limit: 256 | Fork bombs die immediately |
| User | Non-root (uid 1000) | No root inside the sandbox |
| Lifetime | TTL auto-expiry | Forgotten sandboxes clean themselves up |

With the Firecracker provider, you also get: dedicated kernel per sandbox, vsock-only communication (no TCP between host and guest), and ephemeral rootfs destroyed on teardown.

---

## Architecture

![ForgeVM Architecture](assets/forgevm_architecture_diagram.svg)

**Key flow:** SDK → REST API → Orchestrator (lifecycle, TTL, pool, templates) → Provider → Sandbox

**Snapshot trick:** First Firecracker spawn cold-boots (~1s) and snapshots the VM state. Every spawn after that restores from snapshot in **~28ms** — faster than most HTTP requests.

---

## Use with Kotlin / Android

```kotlin
import ai.dohaeris.forgevm.Client
import kotlinx.coroutines.runBlocking

fun main() = runBlocking {
    val client = Client("http://localhost:7423")

    val sandbox = client.spawn(image = "alpine:latest")
    val result = sandbox.exec("echo hello world")
    println(result.stdout)

    sandbox.writeFile("/app/main.sh", "echo from ForgeVM\n")
    sandbox.exec("sh /app/main.sh")

    sandbox.extendTtl("30m")
    sandbox.destroy()
}
```

The Kotlin SDK lives in [`sdk/kotlin`](./sdk/kotlin) and is intended for JVM and Android apps.

---

## CLI

```bash
forgevm serve                                  # start server
forgevm spawn --image python:3.12 --ttl 1h     # spawn
forgevm exec sb-a1b2c3d4 -- python3 app.py     # run code
forgevm list                                    # list active sandboxes
forgevm kill sb-a1b2c3d4                        # destroy
forgevm build-image python:3.12                 # pre-build rootfs
forgevm tui                                     # interactive dashboard
```

---

## REST API

Base URL: `http://localhost:7423/api/v1`

| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/sandboxes` | Spawn a sandbox |
| `GET` | `/sandboxes` | List all sandboxes |
| `GET` | `/sandboxes/:id` | Get sandbox details |
| `DELETE` | `/sandboxes/:id` | Destroy sandbox |
| `POST` | `/sandboxes/:id/exec` | Execute a command |
| `GET` | `/sandboxes/:id/exec/ws` | Execute via WebSocket |
| `POST` | `/sandboxes/:id/files` | Write a file |
| `GET` | `/sandboxes/:id/files?path=` | Read a file |
| `GET` | `/sandboxes/:id/files/list` | List files |
| `POST` | `/sandboxes/:id/extend` | Extend TTL |
| `GET` | `/sandboxes/:id/logs` | Console logs |
| `GET` | `/health` | Health check |
| `GET` | `/events` | SSE event stream |
| `POST` | `/templates` | Create template |
| `POST` | `/templates/:name/spawn` | Spawn from template |

Full API docs: [docs/api.md](docs/api.md)

---

## Configuration

```yaml
# forgevm.yaml (optional — sane defaults without it)
server:
  host: "0.0.0.0"
  port: 7423

providers:
  default: "docker"
  docker:
    enabled: true
    runtime: "runc"
    network_mode: "none"
    read_only_rootfs: true
  firecracker:
    enabled: true
    kernel_path: "/var/lib/forgevm/vmlinux.bin"

defaults:
  ttl: "30m"
  image: "alpine:latest"
  memory_mb: 512

auth:
  enabled: false
  api_key: "your-secret-key"
```

Config priority: `./forgevm.yaml` > `~/.forgevm/config.yaml` > env vars (`FORGEVM_SERVER_PORT=8080`)

---

## Web Dashboard

Built-in React dashboard for sandbox management, live terminal, file browser, and log viewer.

```bash
make web          # build frontend
./forgevm serve   # open http://localhost:7423
```

---

## Install options

**One-command setup (recommended):**
```bash
git clone https://github.com/DohaerisAI/forgevm && cd forgevm
./scripts/setup.sh    # checks Go, Docker, KVM, downloads Firecracker + kernel, builds everything
./forgevm serve
```

**Build from source:**
```bash
make build-all
sudo mkdir -p /var/lib/forgevm && sudo chown $(whoami) /var/lib/forgevm
./scripts/setup-kernel.sh
```

**Docker:**
```bash
docker build -t forgevm .
docker run -p 7423:7423 forgevm
```

**Binary download** (when releases are available):
```bash
curl -fsSL https://github.com/DohaerisAI/forgevm/releases/latest/download/forgevm-linux-amd64 -o forgevm
chmod +x forgevm && sudo mv forgevm /usr/local/bin/
```

---

## Roadmap

- [x] Firecracker provider (KVM microVMs, ~28ms snapshot restore)
- [x] Docker provider (OCI containers, seccomp, no KVM needed)
- [x] gVisor support (user-space kernel via runsc runtime)
- [x] Pool mode (N users per VM, workspace isolation)
- [x] Python SDK + TypeScript SDK
- [x] Web dashboard + TUI
- [x] Template system + warm pools
- [ ] Kata Containers provider (K8s-native)
- [ ] Persistent volumes across sandboxes
- [ ] MCP server mode
- [ ] GPU passthrough

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). PRs welcome — especially for new providers, SDK improvements, and documentation.

## License

[MIT](LICENSE) — use it however you want.

---

<p align="center">
  <b>Built by <a href="https://github.com/DohaerisAI">DohaerisAI</a></b><br>
  If ForgeVM helps you, drop a ⭐ — it helps others find it.
</p>
