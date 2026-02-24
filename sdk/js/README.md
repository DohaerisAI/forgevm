# ForgeVM TypeScript SDK

TypeScript/JavaScript client for [ForgeVM](https://github.com/mainadwitiya/forgevm) -- self-hosted compute sandboxes for LLMs.

## Install

```bash
npm install forgevm
```

## Quick Start

```typescript
import { Client } from "forgevm";

const client = new Client("http://localhost:7423");

// Spawn a sandbox
const sandbox = await client.spawn({ image: "alpine:latest" });

// Execute commands
const result = await sandbox.exec("echo hello");
console.log(result.stdout); // "hello\n"

// File operations
await sandbox.writeFile("/tmp/hello.txt", "Hello, world!");
const content = await sandbox.readFile("/tmp/hello.txt");

// Extend TTL
await sandbox.extendTtl("30m");

// Streaming output
for await (const chunk of sandbox.execStream("ping -c 3 localhost")) {
  process.stdout.write(chunk.data);
}

await sandbox.destroy();
```

## License

MIT
