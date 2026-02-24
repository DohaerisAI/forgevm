/**
 * Basic ForgeVM TypeScript SDK usage.
 *
 * Demonstrates:
 *   - Connecting to a ForgeVM server
 *   - Spawning a sandbox
 *   - Executing a command
 *   - Writing and reading a file
 *   - Using withSandbox for automatic cleanup
 *
 * Prerequisites:
 *   npm install forgevm    (or link the local SDK)
 *   # ForgeVM server running at localhost:7423
 *
 * Run with:
 *   npx tsx examples/js/basic.ts
 */

import { Client } from "forgevm";

async function main(): Promise<void> {
  // Connect to a local ForgeVM server on the default port.
  // Pass apiKey: "..." if authentication is enabled.
  const client = new Client({ host: "localhost", port: 7423 });

  // Check server health.
  const health = await client.health();
  console.log(`Server status: ${health.status}  version: ${health.version}`);

  // List available providers.
  const providers = await client.providers();
  for (const p of providers) {
    console.log(`  provider: ${p.name}  healthy=${p.healthy}  default=${p.default}`);
  }

  // --- Manual lifecycle --------------------------------------------------

  console.log("\n=== Manual lifecycle ===");
  const sandbox = await client.spawn({
    image: "alpine:latest",
    ttl: "10m",
  });
  console.log(`Spawned: ${sandbox.id}  state=${sandbox.state}`);

  // Execute a command.
  const result = await sandbox.exec("echo 'Hello from ForgeVM!'");
  console.log(`Exit code: ${result.exit_code}`);
  console.log(`Stdout: ${result.stdout.trim()}`);

  // Write a file.
  await sandbox.writeFile("/tmp/greeting.txt", "Hello, world!\n");
  console.log("Wrote /tmp/greeting.txt");

  // Read it back.
  const content = await sandbox.readFile("/tmp/greeting.txt");
  console.log(`Read back: ${JSON.stringify(content.trim())}`);

  // List files.
  const files = await sandbox.listFiles("/tmp");
  console.log(`Files in /tmp: ${files.map((f) => f.name).join(", ")}`);

  // Destroy.
  await sandbox.destroy();
  console.log(`Sandbox ${sandbox.id} destroyed.\n`);

  // --- withSandbox (automatic cleanup) -----------------------------------

  console.log("=== withSandbox ===");
  const output = await client.withSandbox(
    { image: "alpine:latest", ttl: "5m" },
    async (sb) => {
      await sb.writeFile("/app/main.sh", '#!/bin/sh\necho "Computed: $((6 * 7))"');
      const res = await sb.exec("sh /app/main.sh");
      return res.stdout.trim();
    },
  );
  console.log(`Result: ${output}`);
  console.log("Sandbox automatically destroyed.");
}

main().catch((err) => {
  console.error("Fatal:", err);
  process.exit(1);
});
