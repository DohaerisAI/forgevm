"""
OpenSandbox + Requesty LLM Integration Tests
Full loop: LLM generates code via tool_use -> execute in OpenSandbox -> return result.
Tests the same pattern YODA uses with the compute tool.

Requires:
  - OpenSandbox server running on http://127.0.0.1:8080
  - REQUESTY_API_KEY in env or in acg_yoda/backend/.env
  - pip: opensandbox, openai, anthropic, python-dotenv, httpx
"""
import asyncio
import json
import os
import sys
from datetime import timedelta
from dotenv import load_dotenv
from openai import AsyncOpenAI
from anthropic import AsyncAnthropic
from opensandbox import Sandbox
from opensandbox.config import ConnectionConfig

# Load .env from current directory or parent
load_dotenv()

REQUESTY_API_KEY = os.getenv("REQUESTY_API_KEY")
REQUESTY_BASE_URL = "https://router.requesty.ai/v1"
REQUESTY_ANTHROPIC_BASE_URL = "https://router.requesty.ai"

OPENSANDBOX_URL = "127.0.0.1:8080"
SANDBOX_IMAGE = "opensandbox/code-interpreter:v1.0.1"

OPENSANDBOX_CONFIG = ConnectionConfig(
    domain=OPENSANDBOX_URL,
    protocol="http",
    request_timeout=timedelta(seconds=60),
)

# Models
OPENAI_MODEL = "openai/gpt-5.2-chat"
ANTHROPIC_MODEL = "anthropic/claude-haiku-4-5"

# ── Tool definitions ──
# NOTE: code-interpreter image has stdlib only (no pandas/numpy).
TOOL_DESC = (
    "Execute Python code in a secure sandbox. "
    "Has Python 3.12 with standard library (csv, json, math, os, etc). "
    "No pandas/numpy — use stdlib csv module. "
    "Save output files to /output/."
)

COMPUTE_TOOL_OPENAI = {
    "type": "function",
    "function": {
        "name": "compute",
        "description": TOOL_DESC,
        "parameters": {
            "type": "object",
            "properties": {
                "code": {
                    "type": "string",
                    "description": "Python code to execute as a script via python3. Use print() for output.",
                }
            },
            "required": ["code"],
        },
    },
}

COMPUTE_TOOL_ANTHROPIC = {
    "name": "compute",
    "description": TOOL_DESC,
    "input_schema": {
        "type": "object",
        "properties": {
            "code": {
                "type": "string",
                "description": "Python code to execute as a script via python3. Use print() for output.",
            }
        },
        "required": ["code"],
    },
}


def get_stdout(r):
    return "\n".join(m.text for m in r.logs.stdout)


def get_stderr(r):
    return "\n".join(m.text for m in r.logs.stderr)


async def run_in_sandbox(sandbox: Sandbox, code: str) -> dict:
    """Execute code in OpenSandbox, return result dict matching YODA's format."""
    await sandbox.files.write_file("/tmp/run.py", code)
    r = await sandbox.commands.run("python3 /tmp/run.py")

    stdout = get_stdout(r)
    stderr = get_stderr(r)
    exit_code = r.exit_code if hasattr(r, "exit_code") else 0

    return {"exit_code": exit_code, "stdout": stdout, "stderr": stderr}


async def openai_tool_loop(client, model, messages, tools, sandbox, max_rounds=3):
    """Run OpenAI tool-use loop: LLM -> tool call -> execute -> feed back -> repeat."""
    for _ in range(max_rounds):
        response = await client.chat.completions.create(
            model=model, messages=messages, tools=tools, max_tokens=1024,
        )
        msg = response.choices[0].message

        if not msg.tool_calls:
            return msg.content or "(no content)"

        messages.append(msg.model_dump())
        for tc in msg.tool_calls:
            if tc.function.name == "compute":
                code = json.loads(tc.function.arguments)["code"]
                result = await run_in_sandbox(sandbox, code)
                output = result["stdout"] or result["stderr"] or "(no output)"
            else:
                output = f"Unknown tool: {tc.function.name}"
            messages.append({"role": "tool", "tool_call_id": tc.id, "content": output})

    return "(max rounds reached)"


async def anthropic_tool_loop(client, model, system, messages, tools, sandbox, max_rounds=3):
    """Run Anthropic tool-use loop: LLM -> tool call -> execute -> feed back -> repeat."""
    for _ in range(max_rounds):
        response = await client.messages.create(
            model=model, max_tokens=1024, system=system, messages=messages, tools=tools,
        )

        text_parts = []
        tool_blocks = []
        for block in response.content:
            if hasattr(block, "text"):
                text_parts.append(block.text)
            if block.type == "tool_use" and block.name == "compute":
                tool_blocks.append(block)

        if not tool_blocks:
            return "".join(text_parts) or "(no content)"

        messages.append({"role": "assistant", "content": response.content})
        tool_results = []
        for tb in tool_blocks:
            code = tb.input["code"]
            result = await run_in_sandbox(sandbox, code)
            output = result["stdout"] or result["stderr"] or "(no output)"
            tool_results.append({"type": "tool_result", "tool_use_id": tb.id, "content": output})
        messages.append({"role": "user", "content": tool_results})

    return "(max rounds reached)"


# ═══════════════════════════════════════════════
# TEST 1: OpenAI + tool_use -> OpenSandbox
# ═══════════════════════════════════════════════
async def test_openai_tool_use(sandbox: Sandbox):
    print("\n" + "=" * 70)
    print(f"TEST 1: OpenAI {OPENAI_MODEL} + tool_use -> OpenSandbox")
    print("=" * 70)

    client = AsyncOpenAI(api_key=REQUESTY_API_KEY, base_url=REQUESTY_BASE_URL)
    messages = [
        {"role": "system", "content": "You have a `compute` tool to execute Python code. Always use it for any calculation. Use print() to show results."},
        {"role": "user", "content": "What is the sum of all prime numbers below 100? Use the compute tool."},
    ]
    print("  Prompt: What is the sum of all prime numbers below 100?")

    final = await openai_tool_loop(client, OPENAI_MODEL, messages, [COMPUTE_TOOL_OPENAI], sandbox)
    print(f"  LLM final answer: {final[:300]}")

    assert "1060" in final, f"Expected 1060 in answer, got: {final}"
    print("  PASSED")


# ═══════════════════════════════════════════════
# TEST 2: Anthropic + tool_use -> OpenSandbox
# ═══════════════════════════════════════════════
async def test_anthropic_tool_use(sandbox: Sandbox):
    print("\n" + "=" * 70)
    print(f"TEST 2: Anthropic {ANTHROPIC_MODEL} + tool_use -> OpenSandbox")
    print("=" * 70)

    client = AsyncAnthropic(api_key=REQUESTY_API_KEY, base_url=REQUESTY_ANTHROPIC_BASE_URL)
    system = "You have a `compute` tool. Always use it for calculations. Use print() to show results."
    messages = [
        {"role": "user", "content": "Use the compute tool to calculate: what's the 20th Fibonacci number?"}
    ]
    print("  Prompt: What's the 20th Fibonacci number?")

    final = await anthropic_tool_loop(client, ANTHROPIC_MODEL, system, messages, [COMPUTE_TOOL_ANTHROPIC], sandbox)
    print(f"  LLM final answer: {final[:300]}")

    assert "6765" in final, f"Expected 6765 in answer, got: {final}"
    print("  PASSED")


# ═══════════════════════════════════════════════
# TEST 3: File generation (CSV) via Anthropic -> sandbox
# ═══════════════════════════════════════════════
async def test_file_generation(sandbox: Sandbox):
    print("\n" + "=" * 70)
    print("TEST 3: File Generation (Anthropic -> compute -> /output/)")
    print("=" * 70)

    client = AsyncAnthropic(api_key=REQUESTY_API_KEY, base_url=REQUESTY_ANTHROPIC_BASE_URL)
    await sandbox.commands.run("rm -f /output/employees.csv")

    system = "You have a `compute` tool. Use it to run Python code. Use the csv module (stdlib). Save output files to /output/. Use print() to confirm."
    messages = [
        {"role": "user", "content": "Generate a CSV file with 5 rows of sample employee data (name, department, salary). Save it to /output/employees.csv. Use the csv module, not pandas."},
    ]
    print("  Prompt: Generate employee CSV to /output/employees.csv")

    final = await anthropic_tool_loop(client, ANTHROPIC_MODEL, system, messages, [COMPUTE_TOOL_ANTHROPIC], sandbox)
    print(f"  LLM response: {final[:200]}")

    content = await sandbox.files.read_file("/output/employees.csv")
    print(f"  Generated CSV:\n    {content.strip().replace(chr(10), chr(10) + '    ')}")

    assert "name" in content.lower() or "Name" in content, "CSV should have name column"
    lines = [l for l in content.strip().split("\n") if l.strip()]
    assert len(lines) >= 6, f"Expected header + 5 rows, got {len(lines)} lines"
    print(f"  File verified: {len(lines) - 1} data rows")
    print("  PASSED")


# ═══════════════════════════════════════════════
# TEST 4: Data upload + analysis (YODA sandbox data routing)
# ═══════════════════════════════════════════════
async def test_data_upload_and_analysis(sandbox: Sandbox):
    print("\n" + "=" * 70)
    print(f"TEST 4: Data Upload + Analysis ({OPENAI_MODEL})")
    print("=" * 70)

    csv_data = (
        "product,region,sales,units\n"
        "Widget A,North,15000,150\n"
        "Widget A,South,12000,120\n"
        "Widget B,North,8000,80\n"
        "Widget B,South,22000,220\n"
        "Widget C,North,18000,180\n"
        "Widget C,South,9000,90\n"
    )
    await sandbox.commands.run("mkdir -p /data")
    await sandbox.files.write_file("/data/sales.csv", csv_data)
    print("  Uploaded sales.csv to /data/sales.csv")

    client = AsyncOpenAI(api_key=REQUESTY_API_KEY, base_url=REQUESTY_BASE_URL)
    messages = [
        {"role": "system", "content": "You have a `compute` tool for running Python. Data files are in /data/. Use the csv module (stdlib, no pandas). Use print() to show results."},
        {"role": "user", "content": (
            "I uploaded sales.csv to /data/sales.csv.\n"
            "Columns: product, region, sales, units\n"
            "6 rows of sales data.\n\n"
            "Which product has the highest total sales across all regions? Use the compute tool with the csv module."
        )},
    ]
    print("  Prompt: Which product has highest total sales?")

    final = await openai_tool_loop(client, OPENAI_MODEL, messages, [COMPUTE_TOOL_OPENAI], sandbox)
    print(f"  LLM answer: {final[:400]}")

    assert "Widget B" in final or "widget b" in final.lower() or "30000" in final or "30,000" in final, \
        f"Expected Widget B (30000 total), got: {final}"
    print("  PASSED")


# ═══════════════════════════════════════════════
# TEST 5: Multi-round agentic (Anthropic + file gen + verify)
# ═══════════════════════════════════════════════
async def test_multi_round(sandbox: Sandbox):
    print("\n" + "=" * 70)
    print("TEST 5: Multi-round Agentic (Anthropic -> compute -> file)")
    print("=" * 70)

    await sandbox.commands.run("rm -f /output/fibonacci.json")

    client = AsyncAnthropic(api_key=REQUESTY_API_KEY, base_url=REQUESTY_ANTHROPIC_BASE_URL)
    system = "You have a `compute` tool. Use it for ALL calculations. Save files to /output/. Use print() to show results."
    messages = [
        {"role": "user", "content": (
            "Calculate the first 15 Fibonacci numbers and save them as a JSON list to /output/fibonacci.json. "
            "Also print them. Use the compute tool."
        )},
    ]
    print("  Prompt: Calculate Fibonacci + save as JSON")

    final = await anthropic_tool_loop(client, ANTHROPIC_MODEL, system, messages, [COMPUTE_TOOL_ANTHROPIC], sandbox)
    print(f"  LLM response: {final[:200]}")

    content = await sandbox.files.read_file("/output/fibonacci.json")
    fib_data = json.loads(content)
    print(f"  Generated JSON: {fib_data}")

    assert isinstance(fib_data, list), f"Expected list, got {type(fib_data)}"
    assert len(fib_data) >= 15, f"Expected 15+ items, got {len(fib_data)}"
    assert 610 in fib_data or 377 in fib_data, f"Expected 610 or 377 in Fibonacci, got: {fib_data}"
    print("  PASSED")


# ═══════════════════════════════════════════════
# RUNNER
# ═══════════════════════════════════════════════
async def run_all():
    print("\n" + "=" * 70)
    print("OPENSANDBOX + REQUESTY LLM INTEGRATION TESTS")
    print("=" * 70)

    if not REQUESTY_API_KEY:
        print("ERROR: REQUESTY_API_KEY not found. Set it in env or in acg_yoda/backend/.env")
        sys.exit(1)
    print(f"  API Key: {'*' * 8}...{REQUESTY_API_KEY[-4:]}" if REQUESTY_API_KEY else "  API Key: (not set)")
    print(f"  OpenAI model:    {OPENAI_MODEL}")
    print(f"  Anthropic model: {ANTHROPIC_MODEL}")

    print("\n  Creating sandbox...")
    sandbox = await Sandbox.create(
        SANDBOX_IMAGE,
        connection_config=OPENSANDBOX_CONFIG,
        timeout=timedelta(minutes=10),
        ready_timeout=timedelta(seconds=60),
    )
    print(f"  Sandbox ID: {sandbox.id}")
    await sandbox.commands.run("mkdir -p /output /data")

    tests = [
        (f"Test 1: OpenAI {OPENAI_MODEL} tool_use", test_openai_tool_use),
        (f"Test 2: Anthropic {ANTHROPIC_MODEL} tool_use", test_anthropic_tool_use),
        ("Test 3: File generation (Anthropic)", test_file_generation),
        (f"Test 4: Data upload + analysis ({OPENAI_MODEL})", test_data_upload_and_analysis),
        ("Test 5: Multi-round agentic (Anthropic)", test_multi_round),
    ]

    results = []
    for name, fn in tests:
        try:
            await fn(sandbox)
            results.append((name, True, None))
        except Exception as e:
            results.append((name, False, str(e)))
            import traceback
            traceback.print_exc()

    # Cleanup
    print("\n  Cleaning up sandbox...")
    await sandbox.close()
    import httpx
    async with httpx.AsyncClient() as hc:
        await hc.delete(f"http://{OPENSANDBOX_URL}/sandboxes/{sandbox.id}")

    # Summary
    print("\n" + "=" * 70)
    print("SUMMARY")
    print("=" * 70)
    for name, passed, err in results:
        status = "PASS" if passed else "FAIL"
        print(f"  {status}: {name}" + (f" — {err}" if err else ""))

    passed_count = sum(1 for _, p, _ in results if p)
    total = len(results)
    if passed_count == total:
        print(f"\nAll {total} tests PASSED")
    else:
        print(f"\n{passed_count}/{total} passed, {total - passed_count} FAILED")
        sys.exit(1)


if __name__ == "__main__":
    asyncio.run(run_all())
