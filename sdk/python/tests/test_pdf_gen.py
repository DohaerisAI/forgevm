"""
LLM + ForgeVM PDF generation test.

LLM writes reportlab code -> runs in forgevm sandbox -> PDF extracted to local disk.

Supports any model on Requesty:
  - anthropic/* models → Anthropic SDK (messages API)
  - everything else   → OpenAI SDK (chat completions)

Output dir: /home/adwitiya24/sandbox_files/ (override with LLM_OUTPUT_DIR)

Usage:
  source /home/adwitiya24/corporate/ACG_YODA/co_env/bin/activate
  python tests/test_pdf_gen.py
  LLM_MODEL=moonshot/kimi-k2.5 python tests/test_pdf_gen.py
"""

import base64
import json
import os
import re
import sys
import time
import asyncio
from pathlib import Path
from datetime import datetime

# Load .env if dotenv is available
try:
    from dotenv import load_dotenv
    load_dotenv()
except ImportError:
    pass

from forgevm import Client

# ── Config ────────────────────────────────────────────────────────────────

REQUESTY_API_KEY = os.environ.get("REQUESTY_API_KEY")
MODEL = os.environ.get("LLM_MODEL", "anthropic/claude-haiku-4-5")
FORGEVM_URL = os.environ.get("FORGEVM_URL", "http://localhost:7423")
SANDBOX_IMAGE = os.environ.get("SANDBOX_IMAGE", "ghcr.io/darkxace01/ai-filegen-sandbox:py3.12-v1")
OUTPUT_DIR = Path(os.environ.get("LLM_OUTPUT_DIR", "./sandbox_files"))
BINARY_EXTENSIONS = {".pdf", ".docx", ".xlsx", ".png", ".jpg", ".jpeg", ".gif", ".zip"}

IS_ANTHROPIC = MODEL.startswith("anthropic/")
MAX_ROUNDS = 10

SYSTEM_PROMPT = (
    "You are a helpful assistant with access to a compute sandbox. "
    "The sandbox has Python 3.12 with reportlab, python-docx, openpyxl, matplotlib, pillow. "
    "When asked to generate files, use the compute tool to run python3 scripts. "
    "Save generated files to /output/ directory. Print the filename when done."
)

TOOL_DESC = (
    "Execute code inside a secure Linux sandbox and return stdout/stderr. "
    "The sandbox has Python 3.12, sh, and Python packages: "
    "reportlab, python-docx, openpyxl, matplotlib, pillow. "
    "Save generated files to /output/."
)

CODE_DESC = "Shell command to execute (e.g. python3 -c '...' or python3 script.py)."

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


def extract_file(sandbox, remote_path):
    """Pull a file out of the sandbox to local disk."""
    OUTPUT_DIR.mkdir(parents=True, exist_ok=True)
    fname = Path(remote_path).name
    ext = Path(fname).suffix.lower()

    if ext in BINARY_EXTENSIONS:
        r = sandbox.exec(f"base64 {remote_path}")
        file_bytes = base64.b64decode(r.stdout.strip())
    else:
        r = sandbox.exec(f"cat {remote_path}")
        file_bytes = r.stdout.encode()

    ts = datetime.now().strftime("%Y%m%d_%H%M%S")
    safe_name = re.sub(r"[^\w\-.]", "_", fname)
    local_path = OUTPUT_DIR / f"{ts}_{safe_name}"
    local_path.write_bytes(file_bytes)
    return local_path


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
                    print(f"    {code[:300]}{'...' if len(code) > 300 else ''}")

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
            print(f"    {code[:300]}{'...' if len(code) > 300 else ''}")

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

async def test_simple_pdf(sandbox):
    header("Simple PDF (reportlab canvas)")
    start = time.time()

    try:
        answer, tool_calls, tokens = await run_agent_loop(
            sandbox,
            "Do these two steps using the compute tool:\n"
            "Step 1: Run a python3 script that uses reportlab.lib.pagesizes and "
            "reportlab.pdfgen.canvas to create a one-page PDF at /output/hello.pdf "
            "with the text 'Hello from ForgeVM!'. Print 'created' when done.\n"
            "Step 2: Run: python3 -c \"import os; print(os.path.getsize('/output/hello.pdf'))\"\n"
            "Tell me the file size in bytes.",
        )
        elapsed = time.time() - start

        print(f"\n  LLM: {answer[:400]}")
        print(f"  Tool calls: {tool_calls} | Tokens: {tokens} | Time: {elapsed:.1f}s")

        has_response = bool(answer) and any(c.isdigit() for c in answer)

        local_path = extract_file(sandbox, "/output/hello.pdf")
        size = local_path.stat().st_size
        is_pdf = local_path.read_bytes()[:5] == b"%PDF-"

        print(f"  Extracted: {local_path} ({size:,} bytes)")
        print(f"  Valid PDF: {is_pdf}")

        passed = tool_calls >= 1 and has_response and size > 0 and is_pdf
        return result(passed, f"tool_calls={tool_calls}, size={size}, valid_pdf={is_pdf}")

    except Exception as e:
        print(f"  Error: {e}")
        import traceback
        traceback.print_exc()
        return result(False, str(e)[:150])


async def test_styled_pdf(sandbox):
    header("Styled PDF (platypus + table)")
    start = time.time()

    try:
        answer, tool_calls, tokens = await run_agent_loop(
            sandbox,
            "Using the compute tool, write a python3 script that uses reportlab to create "
            "a professional PDF at /output/report.pdf with:\n"
            "- A title 'ForgeVM Status Report'\n"
            "- A paragraph of text describing a sandbox orchestration platform\n"
            "- A small table with 3 rows: (Feature, Status) -> "
            "(Spawn, Complete), (Exec, Complete), (Files, Complete)\n"
            "Save to /output/report.pdf and print the file size when done.\n"
            "Use reportlab.platypus (SimpleDocTemplate, Paragraph, Table, Spacer) "
            "and reportlab.lib.styles.",
        )
        elapsed = time.time() - start

        print(f"\n  LLM: {answer[:400]}")
        print(f"  Tool calls: {tool_calls} | Tokens: {tokens} | Time: {elapsed:.1f}s")

        has_response = bool(answer) and ("pdf" in answer.lower() or "report" in answer.lower())

        local_path = extract_file(sandbox, "/output/report.pdf")
        size = local_path.stat().st_size
        is_pdf = local_path.read_bytes()[:5] == b"%PDF-"

        print(f"  Extracted: {local_path} ({size:,} bytes)")
        print(f"  Valid PDF: {is_pdf}")

        passed = tool_calls >= 1 and has_response and size > 500 and is_pdf
        return result(passed, f"tool_calls={tool_calls}, size={size}, valid_pdf={is_pdf}, responded={has_response}")

    except Exception as e:
        print(f"  Error: {e}")
        import traceback
        traceback.print_exc()
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
    print(f"Output: {OUTPUT_DIR}")

    with Client(base_url=FORGEVM_URL, timeout=120.0) as forgevm:
        with forgevm.spawn(image=SANDBOX_IMAGE, ttl="5m") as sandbox:
            print(f"Sandbox {sandbox.id} ready.")
            sandbox.exec("mkdir -p /output")

            tests = [
                ("simple_pdf", test_simple_pdf),
                ("styled_pdf", test_styled_pdf),
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
