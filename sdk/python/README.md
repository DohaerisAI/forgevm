# ForgeVM Python SDK

Python client for [ForgeVM](https://github.com/DohaerisAI/forgevm) -- self-hosted compute sandboxes for LLMs.

## Install

```bash
pip install forgevm
```

## Quick Start

```python
from forgevm import Client

client = Client("http://localhost:7423")

# Spawn a sandbox
sandbox = client.spawn(image="alpine:latest")

# Execute commands
result = sandbox.exec("echo hello")
print(result.stdout)  # "hello\n"

# File operations
sandbox.write_file("/tmp/hello.txt", "Hello, world!")
content = sandbox.read_file("/tmp/hello.txt")

# Extend TTL
sandbox.extend_ttl("30m")

# Auto-cleanup with context manager
with client.spawn(image="python:3.12") as sb:
    sb.exec("python3 -c 'print(1+1)'")
# sandbox destroyed automatically

# Streaming output
for chunk in sandbox.exec_stream("ping -c 3 localhost"):
    print(chunk.data, end="")

sandbox.destroy()
```

## Async Support

```python
from forgevm import AsyncClient

async with AsyncClient("http://localhost:7423") as client:
    sandbox = await client.spawn(image="alpine:latest")
    result = await sandbox.exec("echo hello")
    await sandbox.destroy()
```

## License

MIT
