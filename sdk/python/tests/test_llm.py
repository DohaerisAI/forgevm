"""
LLM + ForgeVM compute integration test.

End-to-end agentic loop:
  1. Spawn a forgevm sandbox
  2. Call an LLM via Requesty with a compute tool
  3. LLM writes code, calls the compute tool
  4. We execute the code inside the forgevm microVM
  5. Return output to LLM
  6. LLM produces a final answer

Supports any model on Requesty:
  - anthropic/* models → Anthropic SDK (messages API)
  - everything else   → OpenAI SDK (chat completions)

Usage:
  source /home/adwitiya24/corporate/ACG_YODA/co_env/bin/activate
  python tests/test_llm.py
  LLM_MODEL=moonshot/kimi-k2.5 python tests/test_llm.py
  LLM_MODEL=openai/gpt-4o-mini python tests/test_llm.py
"""

import json
import os
import sys
import time
import asyncio
from pathlib import Path

# Load .env from ACG_YODA
env_path = Path("/home/adwitiya24/corporate/acg_yoda/backend/.env")
if env_path.exists():
    for line in env_path.read_text().splitlines():
        line = line.strip()
        if line and not line.startswith("#") and "=" in line:
            key, _, value = line.partition("=")
            os.environ.setdefault(key.strip(), value.strip())

from forgevm import Client

# ── Config ────────────────────────────────────────────────────────────────

REQUESTY_API_KEY = os.environ.get("REQUESTY_API_KEY")
MODEL = os.environ.get("LLM_MODEL", "anthropic/claude-haiku-4-5")
FORGEVM_URL = os.environ.get("FORGEVM_URL", "http://localhost:7423")
SANDBOX_IMAGE = os.environ.get("SANDBOX_IMAGE", "ghcr.io/darkxace01/ai-filegen-sandbox:py3.12-v1")

IS_ANTHROPIC = MODEL.startswith("anthropic/")
MAX_ROUNDS = 10

SYSTEM_PROMPT = (
    "You are a helpful assistant with access to a compute sandbox. "
    "When you need to calculate something, ALWAYS use the compute tool — "
    "do not compute mentally. The sandbox has Python 3.12 and sh. "
    "Prefer python3 for calculations. You can also use sh, bc, or awk. "
    "After getting the result from the tool, state the answer clearly."
)

TOOL_DESC = (
    "Execute code inside a secure Linux sandbox and return stdout/stderr. "
    "Use this for any calculations, data processing, or code execution. "
    "The sandbox has Python 3.12, sh, awk, bc, sed, grep, and Python packages: "
    "reportlab, python-docx, openpyxl, matplotlib, pillow."
)

CODE_DESC = "Shell command to execute (e.g. python3 -c '...' or python3 script.py or sh commands)."

# ── LLM client setup ─────────────────────────────────────────────────────

if IS_ANTHROPIC:
    from anthropic import AsyncAnthropic

    llm_client = AsyncAnthropic(
        api_key=REQUESTY_API_KEY,
        base_url="https://router.requesty.ai",
    )

    TOOLS = [{
        "name": "compute",
        "description": TOOL_DESC,
        "input_schema": {
            "type": "object",
            "properties": {
                "code": {"type": "string", "description": CODE_DESC},
            },
            "required": ["code"],
        },
    }]
else:
    from openai import AsyncOpenAI

    llm_client = AsyncOpenAI(
        api_key=REQUESTY_API_KEY,
        base_url="https://router.requesty.ai/v1",
    )

    TOOLS = [{
        "type": "function",
        "function": {
            "name": "compute",
            "description": TOOL_DESC,
            "parameters": {
                "type": "object",
                "properties": {
                    "code": {"type": "string", "description": CODE_DESC},
                },
                "required": ["code"],
            },
        },
    }]


# ── Helpers ───────────────────────────────────────────────────────────────

def header(name: str):
    print(f"\n{'=' * 70}")
    print(f"  TEST: {name}")
    print(f"{'=' * 70}")


def result(passed: bool, details: str = ""):
    tag = "\033[92mPASS\033[0m" if passed else "\033[91mFAIL\033[0m"
    print(f"\n  [{tag}] {details}")
    return passed


# ── Anthropic agent loop ─────────────────────────────────────────────────

async def run_anthropic_loop(sandbox, user_message):
    messages = [{"role": "user", "content": user_message}]
    tool_call_count = 0
    total_tokens = 0

    for round_num in range(1, MAX_ROUNDS + 1):
        response = await llm_client.messages.create(
            model=MODEL,
            max_tokens=4096,
            system=SYSTEM_PROMPT,
            tools=TOOLS,
            messages=messages,
        )

        total_tokens += response.usage.input_tokens + response.usage.output_tokens
        blocks = [b.type for b in response.content]
        print(f"  Round {round_num}: stop={response.stop_reason}, blocks={blocks}, tokens={total_tokens}")

        if response.stop_reason == "end_turn":
            text = "".join(b.text for b in response.content if b.type == "text")
            return text, tool_call_count, total_tokens

        if response.stop_reason == "tool_use":
            messages.append({"role": "assistant", "content": response.content})
            tool_results = []

            for b in response.content:
                if b.type == "tool_use" and b.name == "compute":
                    tool_call_count += 1
                    code = b.input.get("code", "")
                    print(f"\n  [tool call #{tool_call_count}] code:")
                    print(f"    {code[:200]}{'...' if len(code) > 200 else ''}")

                    exec_result = sandbox.exec(code)
                    output_parts = []
                    if exec_result.stdout:
                        output_parts.append(exec_result.stdout)
                    if exec_result.stderr:
                        output_parts.append(f"[stderr] {exec_result.stderr}")
                    output_parts.append(f"[exit_code={exec_result.exit_code}]")
                    tool_output = "\n".join(output_parts)

                    print(f"  [tool call #{tool_call_count}] output:")
                    print(f"    {tool_output[:200]}{'...' if len(tool_output) > 200 else ''}")

                    tool_results.append({
                        "type": "tool_result",
                        "tool_use_id": b.id,
                        "content": tool_output,
                    })

            messages.append({"role": "user", "content": tool_results})

    return "", tool_call_count, total_tokens


# ── OpenAI agent loop ────────────────────────────────────────────────────

async def run_openai_loop(sandbox, user_message):
    messages = [
        {"role": "system", "content": SYSTEM_PROMPT},
        {"role": "user", "content": user_message},
    ]
    tool_call_count = 0
    total_tokens = 0

    for round_num in range(1, MAX_ROUNDS + 1):
        response = await llm_client.chat.completions.create(
            model=MODEL,
            max_tokens=4096,
            tools=TOOLS,
            messages=messages,
        )

        usage = response.usage
        if usage:
            total_tokens += (usage.prompt_tokens or 0) + (usage.completion_tokens or 0)

        choice = response.choices[0]
        msg = choice.message
        print(f"  Round {round_num}: finish={choice.finish_reason}, tool_calls={len(msg.tool_calls or [])}, tokens={total_tokens}")

        # Append assistant message
        messages.append(msg)

        if choice.finish_reason == "stop" or not msg.tool_calls:
            return msg.content or "", tool_call_count, total_tokens

        for tc in msg.tool_calls:
            if tc.function.name != "compute":
                messages.append({
                    "role": "tool",
                    "tool_call_id": tc.id,
                    "content": f"Error: unknown tool {tc.function.name!r}",
                })
                continue

            tool_call_count += 1
            args = json.loads(tc.function.arguments)
            code = args.get("code", "")

            print(f"\n  [tool call #{tool_call_count}] code:")
            print(f"    {code[:200]}{'...' if len(code) > 200 else ''}")

            exec_result = sandbox.exec(code)
            output_parts = []
            if exec_result.stdout:
                output_parts.append(exec_result.stdout)
            if exec_result.stderr:
                output_parts.append(f"[stderr] {exec_result.stderr}")
            output_parts.append(f"[exit_code={exec_result.exit_code}]")
            tool_output = "\n".join(output_parts)

            print(f"  [tool call #{tool_call_count}] output:")
            print(f"    {tool_output[:200]}{'...' if len(tool_output) > 200 else ''}")

            messages.append({
                "role": "tool",
                "tool_call_id": tc.id,
                "content": tool_output,
            })

    return "", tool_call_count, total_tokens


# ── Dispatch ──────────────────────────────────────────────────────────────

async def run_agent_loop(sandbox, user_message):
    if IS_ANTHROPIC:
        return await run_anthropic_loop(sandbox, user_message)
    else:
        return await run_openai_loop(sandbox, user_message)


# ── Tests ─────────────────────────────────────────────────────────────────

async def test_sum_of_primes(sandbox):
    header("Sum of Primes < 50")
    start = time.time()

    try:
        answer, tool_calls, tokens = await run_agent_loop(
            sandbox,
            "What is the sum of all prime numbers less than 50? Use the compute tool.",
        )
        elapsed = time.time() - start

        print(f"\n  Answer: {answer[:300]}")
        print(f"  Tool calls: {tool_calls} | Tokens: {tokens} | Time: {elapsed:.1f}s")

        has_tool = tool_calls >= 1
        has_answer = "328" in answer
        return result(has_tool and has_answer, f"tool_calls={tool_calls}, has 328: {has_answer}")

    except Exception as e:
        print(f"  Error: {e}")
        return result(False, str(e)[:150])


async def test_fibonacci(sandbox):
    header("First 10 Fibonacci Numbers")
    start = time.time()

    try:
        answer, tool_calls, tokens = await run_agent_loop(
            sandbox,
            "Use the compute tool to print the first 10 Fibonacci numbers. Tell me what they are.",
        )
        elapsed = time.time() - start

        print(f"\n  Answer: {answer[:300]}")
        print(f"  Tool calls: {tool_calls} | Tokens: {tokens} | Time: {elapsed:.1f}s")

        has_tool = tool_calls >= 1
        has_answer = "34" in answer
        return result(has_tool and has_answer, f"tool_calls={tool_calls}, has 34: {has_answer}")

    except Exception as e:
        print(f"  Error: {e}")
        return result(False, str(e)[:150])


async def test_file_write_and_read(sandbox):
    header("File Write + Read (sandbox persistence)")
    start = time.time()

    try:
        answer, tool_calls, tokens = await run_agent_loop(
            sandbox,
            "Using the compute tool: "
            "First, create a file /workspace/nums.txt containing the numbers 10 20 30 40 50 (one per line). "
            "Then in a SECOND compute call, read the file and compute the average. "
            "Tell me the result.",
        )
        elapsed = time.time() - start

        print(f"\n  Answer: {answer[:300]}")
        print(f"  Tool calls: {tool_calls} | Tokens: {tokens} | Time: {elapsed:.1f}s")

        has_multi = tool_calls >= 2
        has_answer = "30" in answer
        return result(has_multi and has_answer, f"tool_calls={tool_calls} (>=2), has 30: {has_answer}")

    except Exception as e:
        print(f"  Error: {e}")
        return result(False, str(e)[:150])


async def test_python_factorial(sandbox):
    header("Python3 Factorial (20!)")
    start = time.time()

    try:
        answer, tool_calls, tokens = await run_agent_loop(
            sandbox,
            "Use the compute tool with python3 to calculate 20 factorial (20!). "
            "Tell me the exact number.",
        )
        elapsed = time.time() - start

        clean = answer.replace(",", "")
        print(f"\n  Answer: {answer[:300]}")
        print(f"  Tool calls: {tool_calls} | Tokens: {tokens} | Time: {elapsed:.1f}s")

        has_tool = tool_calls >= 1
        has_answer = "2432902008176640000" in clean
        return result(has_tool and has_answer, f"tool_calls={tool_calls}, has 20!: {has_answer}")

    except Exception as e:
        print(f"  Error: {e}")
        return result(False, str(e)[:150])


# ── Main ──────────────────────────────────────────────────────────────────

async def main():
    if not REQUESTY_API_KEY:
        print("FAIL: REQUESTY_API_KEY not set")
        sys.exit(1)

    sdk = "Anthropic" if IS_ANTHROPIC else "OpenAI"
    print(f"Model: {MODEL} ({sdk} SDK)")
    print(f"Sandbox: {SANDBOX_IMAGE}")
    print(f"ForgeVM: {FORGEVM_URL}")

    with Client(base_url=FORGEVM_URL, timeout=120.0) as forgevm:
        with forgevm.spawn(image=SANDBOX_IMAGE, ttl="5m") as sandbox:
            print(f"Sandbox {sandbox.id} ready.")

            tests = [
                ("sum_of_primes", test_sum_of_primes),
                ("fibonacci", test_fibonacci),
                ("file_write_and_read", test_file_write_and_read),
                ("python_factorial", test_python_factorial),
            ]

            results = {}
            for name, test_fn in tests:
                try:
                    results[name] = await test_fn(sandbox)
                except Exception as e:
                    print(f"\n  EXCEPTION: {e}")
                    import traceback
                    traceback.print_exc()
                    results[name] = False

        print(f"\nSandbox destroyed.")

    # Summary
    print(f"\n{'=' * 70}")
    print("  SUMMARY")
    print(f"{'=' * 70}")
    total = len(results)
    passed = sum(1 for v in results.values() if v)
    for name, ok in results.items():
        tag = "\033[92mPASS\033[0m" if ok else "\033[91mFAIL\033[0m"
        print(f"  [{tag}] {name}")
    print(f"\n  {passed}/{total} passed")

    sys.exit(0 if passed == total else 1)


if __name__ == "__main__":
    asyncio.run(main())
