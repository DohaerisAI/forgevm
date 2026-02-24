#!/usr/bin/env python3
"""Ollama + ForgeVM integration example.

Demonstrates an agentic loop where:
  1. A local Ollama LLM generates Python code to solve a task.
  2. The code is written into a ForgeVM sandbox.
  3. The sandbox executes the code in isolation.
  4. The output is fed back to the LLM for evaluation.

Prerequisites:
  pip install forgevm httpx
  # Ollama running locally (ollama serve)
  # A model pulled: ollama pull llama3
  # ForgeVM server running at localhost:7423
"""

from __future__ import annotations

import json
import sys

import httpx

from forgevm import Client

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------
OLLAMA_URL = "http://localhost:11434"
OLLAMA_MODEL = "llama3"

FORGEVM_URL = "http://localhost:7423"

SYSTEM_PROMPT = """\
You are a helpful coding assistant. When the user asks you to solve a task,
respond ONLY with a Python script that prints the answer to stdout.
Do not include any explanation outside of code comments.
Wrap the code in ```python ... ``` markers.
"""

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def ask_ollama(prompt: str, history: list[dict]) -> str:
    """Send a chat message to Ollama and return the assistant reply."""
    messages = [{"role": "system", "content": SYSTEM_PROMPT}]
    messages.extend(history)
    messages.append({"role": "user", "content": prompt})

    resp = httpx.post(
        f"{OLLAMA_URL}/api/chat",
        json={"model": OLLAMA_MODEL, "messages": messages, "stream": False},
        timeout=120.0,
    )
    resp.raise_for_status()
    reply = resp.json()["message"]["content"]
    history.append({"role": "user", "content": prompt})
    history.append({"role": "assistant", "content": reply})
    return reply


def extract_code(reply: str) -> str | None:
    """Extract the first ```python ... ``` block from the LLM reply."""
    marker = "```python"
    start = reply.find(marker)
    if start == -1:
        return None
    start += len(marker)
    end = reply.find("```", start)
    if end == -1:
        return None
    return reply[start:end].strip()


# ---------------------------------------------------------------------------
# Main agent loop
# ---------------------------------------------------------------------------


def main() -> None:
    task = (
        "Write a Python script that computes the first 20 Fibonacci numbers "
        "and prints them as a comma-separated list."
    )
    if len(sys.argv) > 1:
        task = " ".join(sys.argv[1:])

    print(f"Task: {task}\n")

    history: list[dict] = []

    with Client(FORGEVM_URL) as client:
        with client.spawn(image="alpine:latest", ttl="10m") as sandbox:
            print(f"Sandbox {sandbox.id} ready.\n")

            for attempt in range(1, 4):
                print(f"--- Attempt {attempt} ---")

                # 1. Ask the LLM to generate code.
                if attempt == 1:
                    prompt = task
                else:
                    prompt = (
                        f"The previous code had this output:\n"
                        f"stdout: {stdout!r}\n"
                        f"stderr: {stderr!r}\n"
                        f"exit code: {exit_code}\n"
                        f"Please fix the script."
                    )

                print("Asking Ollama...")
                reply = ask_ollama(prompt, history)

                code = extract_code(reply)
                if code is None:
                    print("LLM did not return a code block. Raw reply:")
                    print(reply)
                    continue

                print(f"Generated code:\n{code}\n")

                # 2. Write the code into the sandbox.
                sandbox.write_file("/tmp/solution.py", code)

                # 3. Execute it.
                result = sandbox.exec("python3 /tmp/solution.py")
                stdout = result.stdout
                stderr = result.stderr
                exit_code = result.exit_code

                print(f"Exit code: {exit_code}")
                if stdout:
                    print(f"Stdout:\n{stdout}")
                if stderr:
                    print(f"Stderr:\n{stderr}")

                # 4. Success?
                if exit_code == 0 and stdout.strip():
                    print("\nTask completed successfully.")
                    break
            else:
                print("\nFailed to produce a working solution after 3 attempts.")

        print(f"Sandbox destroyed.")


if __name__ == "__main__":
    main()
