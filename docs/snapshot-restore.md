# Firecracker Snapshot/Restore: 1,165ms to 28ms

## The Numbers

| Metric | Before | After | Improvement |
|--------|--------|-------|-------------|
| Single spawn | 1,165ms | 28ms | **41x faster** |
| 3 concurrent spawns | 3,495ms sequential | 43ms wall clock | **81x faster** |

For context, here's how this compares to production sandbox providers:

| Provider | Cold Start |
|----------|-----------|
| AWS Lambda | ~200-500ms |
| E2B | ~300-600ms |
| Fly Machines | ~300ms |
| Modal | ~100-200ms |
| **ForgeVM** | **28ms** |

## Where the Time Was Going

```
BEFORE (cold boot = 1,165ms)
==============================
resolve image         0.1ms   ░
copy rootfs            10ms   █
start firecracker       6ms   █
configure VM (4 API)    1ms   ░
InstanceStart           8ms   █
wait for agent      1,140ms   ████████████████████████████████████████████████  ← 97.8%
```

Nearly all of it was the Linux kernel booting (4.14), mounting the rootfs, and the guest agent starting up. The Firecracker process itself is instant. The host-side setup is instant. The bottleneck is always inside the VM.

## How Snapshot/Restore Eliminates It

```
AFTER (snapshot restore = 28ms)
================================
copy rootfs             7ms   ████████
start firecracker       5ms   ██████
snapshot load           4ms   █████
agent reconnect        12ms   ███████████████
                              ─────────────────
                              28ms total
```

The kernel never boots. The agent never starts. The VM resumes from a paused state where everything is already initialized.

## How It Works

### The Golden Image Pattern

**Once per image** (background, ~1.5s):
1. Boot a temporary VM from the rootfs
2. Wait for the guest agent to become ready
3. Pause the VM (`PATCH /vm {"state": "Paused"}`)
4. Snapshot: `PUT /snapshot/create` produces `vmstate.bin` (CPU/device state, 16KB) + `memory.bin` (full RAM, 512MB)
5. Kill the temporary VM, keep the snapshot files

**Every spawn after that** (28ms):
1. Sparse-copy the snapshot's clean rootfs to a new sandbox directory
2. Start a fresh Firecracker process (no VM config needed)
3. `PUT /snapshot/load` with `resume_vm: true` -- VM resumes instantly
4. Connect to the already-running guest agent over vsock

### The Relative Path Trick

Firecracker bakes drive and vsock paths into the snapshot state. You **cannot** reconfigure them before or after loading a snapshot. This seems like a dealbreaker for running multiple VMs from one snapshot.

The solution: use **relative paths** during snapshot creation.

```go
// During snapshot creation:
api.put("/drives/rootfs", {"path_on_host": "rootfs.ext4"})   // relative
api.put("/vsock", {"uds_path": "v.sock"})                     // relative
```

Each Firecracker process runs with `cmd.Dir` set to its sandbox directory. When a snapshot is loaded, Firecracker resolves `rootfs.ext4` and `v.sock` against its working directory. Each restored VM gets its own rootfs and vsock automatically -- no path conflicts, no reconfiguration needed.

### Snapshot Storage

```
/var/lib/forgevm/snapshots/{sha256-of-rootfs-path}/
  vmstate.bin    16 KB    CPU registers, device state, interrupt controllers
  memory.bin    512 MB    Full guest RAM (could use diff snapshots later)
  rootfs.ext4    64 MB    Clean baseline for sparse-copying
```

### Concurrency

Each restore is fully independent:
- Own Firecracker process
- Own sandbox directory with own rootfs copy
- Own vsock UDS (relative path resolves to own dir)
- Own CID from atomic counter

Three concurrent spawns measured at 28ms, 29ms, 33ms. The snapshot files are read-only after creation -- no locking needed.

## What We Learned the Hard Way

1. **`/vm` is PATCH, not PUT.** Firecracker returns 400 with a cryptic "Invalid request method" error if you use PUT to pause/resume.

2. **No drive/vsock config with snapshots.** Firecracker rejects snapshot load if you configured any "boot-specific resources" beforehand. The error message ("Loading a microVM snapshot not allowed after configuring boot-specific resources") doesn't tell you that you also can't configure them *after* load.

3. **Relative paths are the key.** Every production Firecracker deployment using snapshots needs this trick. The snapshot state stores the exact paths used during creation. Relative paths + per-sandbox working directories = unlimited concurrent restores from one snapshot.

## Files Changed

| File | What |
|------|------|
| `internal/providers/firecracker.go` | `snapshotInfo`, `createBaseSnapshot()`, `restoreFromSnapshot()`, `getSnapshot()`, modified `Spawn()` |
| `internal/providers/firecracker_api.go` | Added `patch()` method for PATCH requests |

Zero changes to the provider interface, API routes, guest agent, or any other file. Snapshot/restore is a fully transparent optimization inside the Firecracker provider.
