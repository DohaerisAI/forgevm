/**
 * LM Studio + ForgeVM integration example.
 *
 * Demonstrates an agentic loop where:
 *   1. A local LM Studio model generates code to solve a task.
 *   2. The code is written into a ForgeVM sandbox.
 *   3. The sandbox executes the code in isolation.
 *   4. The output is fed back to the LLM for evaluation.
 *
 * LM Studio exposes an OpenAI-compatible API on http://localhost:1234.
 *
 * Prerequisites:
 *   npm install forgevm    (or link the local SDK)
 *   # LM Studio running with a loaded model at localhost:1234
 *   # ForgeVM server running at localhost:7423
 *
 * Run with:
 *   npx tsx examples/js/lmstudio_agent.ts "compute the first 15 primes"
 */

import { Client } from "forgevm";

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

const LMSTUDIO_URL = "http://localhost:1234/v1/chat/completions";
const LMSTUDIO_MODEL = "local-model"; // LM Studio uses this as a placeholder

const FORGEVM_URL = "http://localhost:7423";

const SYSTEM_PROMPT = `\
You are a helpful coding assistant. When the user asks you to solve a task,
respond ONLY with a Python script that prints the answer to stdout.
Do not include any explanation outside of code comments.
Wrap the code in \`\`\`python ... \`\`\` markers.`;

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

interface ChatMessage {
  role: "system" | "user" | "assistant";
  content: string;
}

async function askLMStudio(
  prompt: string,
  history: ChatMessage[],
): Promise<string> {
  const messages: ChatMessage[] = [
    { role: "system", content: SYSTEM_PROMPT },
    ...history,
    { role: "user", content: prompt },
  ];

  const response = await fetch(LMSTUDIO_URL, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      model: LMSTUDIO_MODEL,
      messages,
      temperature: 0.2,
      max_tokens: 2048,
    }),
    signal: AbortSignal.timeout(120_000),
  });

  if (!response.ok) {
    throw new Error(
      `LM Studio returned HTTP ${response.status}: ${await response.text()}`,
    );
  }

  const data = (await response.json()) as {
    choices: { message: { content: string } }[];
  };
  const reply = data.choices[0]?.message?.content ?? "";

  history.push({ role: "user", content: prompt });
  history.push({ role: "assistant", content: reply });
  return reply;
}

function extractCode(reply: string): string | null {
  const marker = "```python";
  const start = reply.indexOf(marker);
  if (start === -1) return null;
  const codeStart = start + marker.length;
  const end = reply.indexOf("```", codeStart);
  if (end === -1) return null;
  return reply.slice(codeStart, end).trim();
}

// ---------------------------------------------------------------------------
// Main agent loop
// ---------------------------------------------------------------------------

async function main(): Promise<void> {
  const task =
    process.argv.slice(2).join(" ") ||
    "Write a Python script that computes the first 20 Fibonacci numbers and prints them as a comma-separated list.";

  console.log(`Task: ${task}\n`);

  const history: ChatMessage[] = [];
  const client = new Client({ baseUrl: FORGEVM_URL });

  const output = await client.withSandbox(
    { image: "alpine:latest", ttl: "10m" },
    async (sandbox) => {
      console.log(`Sandbox ${sandbox.id} ready.\n`);

      let stdout = "";
      let stderr = "";
      let exitCode = -1;

      for (let attempt = 1; attempt <= 3; attempt++) {
        console.log(`--- Attempt ${attempt} ---`);

        // 1. Ask the LLM.
        let prompt: string;
        if (attempt === 1) {
          prompt = task;
        } else {
          prompt =
            `The previous code had this output:\n` +
            `stdout: ${JSON.stringify(stdout)}\n` +
            `stderr: ${JSON.stringify(stderr)}\n` +
            `exit code: ${exitCode}\n` +
            `Please fix the script.`;
        }

        console.log("Asking LM Studio...");
        const reply = await askLMStudio(prompt, history);
        const code = extractCode(reply);

        if (!code) {
          console.log("LLM did not return a code block. Raw reply:");
          console.log(reply);
          continue;
        }

        console.log(`Generated code:\n${code}\n`);

        // 2. Write code into the sandbox.
        await sandbox.writeFile("/tmp/solution.py", code);

        // 3. Execute it.
        const result = await sandbox.exec("python3 /tmp/solution.py");
        stdout = result.stdout;
        stderr = result.stderr;
        exitCode = result.exit_code;

        console.log(`Exit code: ${exitCode}`);
        if (stdout) console.log(`Stdout:\n${stdout}`);
        if (stderr) console.log(`Stderr:\n${stderr}`);

        // 4. Success?
        if (exitCode === 0 && stdout.trim().length > 0) {
          console.log("\nTask completed successfully.");
          return stdout.trim();
        }
      }

      console.log("\nFailed to produce a working solution after 3 attempts.");
      return null;
    },
  );

  if (output) {
    console.log(`\nFinal result: ${output}`);
  }
  console.log("Sandbox destroyed.");
}

main().catch((err) => {
  console.error("Fatal:", err);
  process.exit(1);
});
