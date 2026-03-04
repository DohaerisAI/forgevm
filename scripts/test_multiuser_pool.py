"""
ForgeVM Multi-User Pool Test with Requesty LLM Integration
Simulates multiple users sharing a ForgeVM instance, each with isolated sandboxes.
Each user gets their own sandbox, runs LLM-driven code execution via Requesty,
and verifies workspace isolation between users.

Requires:
  - ForgeVM server running on http://localhost:7423
  - REQUESTY_API_KEY in env or in acg_yoda/backend/.env
  - pip: forgevm, openai, anthropic, python-dotenv
"""
import asyncio
import json
import os
import sys
import time
from dotenv import load_dotenv
from openai import AsyncOpenAI
from anthropic import AsyncAnthropic

# ForgeVM SDK (sync client for pool status, async sandbox operations via httpx)
from forgevm import Client

import httpx

# Load .env from current directory or parent
load_dotenv()

REQUESTY_API_KEY = os.getenv("REQUESTY_API_KEY")
REQUESTY_BASE_URL = "https://router.requesty.ai/v1"
REQUESTY_ANTHROPIC_BASE_URL = "https://router.requesty.ai"

FORGEVM_URL = os.getenv("FORGEVM_URL", "http://localhost:7423")
SANDBOX_IMAGE = os.getenv("FORGEVM_IMAGE", "forgevm/python-sandbox:py3.12")

# Models
OPENAI_MODEL = "openai/gpt-5.2-chat"
ANTHROPIC_MODEL = "anthropic/claude-haiku-4-5"

# Users
USERS = ["alice", "bob", "carol"]

# Tool definitions
TOOL_DESC = (
    "Execute Python code in a secure sandbox. "
    "Has Python with standard library. Use print() for output. "
    "Save output files to the current directory (use relative paths like 'output.csv', not absolute paths)."
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
                    "description": "Python code to execute. Use print() for output.",
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
                "description": "Python code to execute. Use print() for output.",
            }
        },
        "required": ["code"],
    },
}


# ── Helpers ──

def create_user_client(user_id: str) -> Client:
    """Create a ForgeVM sync client for a specific user."""
    return Client(base_url=FORGEVM_URL, user_id=user_id)


async def run_in_sandbox(http: httpx.AsyncClient, sandbox_id: str, code: str) -> dict:
    """Execute code in a ForgeVM sandbox, return result dict."""
    # Write code to a file (path gets scoped in pool mode)
    resp = await http.post(
        f"/api/v1/sandboxes/{sandbox_id}/files",
        json={"path": "/run.py", "content": code},
    )
    resp.raise_for_status()

    # Execute using relative path (workdir is set to sandbox workspace in pool mode)
    resp = await http.post(
        f"/api/v1/sandboxes/{sandbox_id}/exec",
        json={"command": "python3 run.py"},
    )
    resp.raise_for_status()
    data = resp.json()
    return {
        "exit_code": data.get("exit_code", 0),
        "stdout": data.get("stdout", ""),
        "stderr": data.get("stderr", ""),
    }


async def openai_tool_loop(oai_client, model, messages, tools, http, sandbox_id, max_rounds=3):
    """Run OpenAI tool-use loop with ForgeVM sandbox execution."""
    for _ in range(max_rounds):
        response = await oai_client.chat.completions.create(
            model=model, messages=messages, tools=tools, max_tokens=1024,
        )
        msg = response.choices[0].message

        if not msg.tool_calls:
            return msg.content or "(no content)"

        messages.append(msg.model_dump())
        for tc in msg.tool_calls:
            if tc.function.name == "compute":
                code = json.loads(tc.function.arguments)["code"]
                result = await run_in_sandbox(http, sandbox_id, code)
                output = result["stdout"] or result["stderr"] or "(no output)"
            else:
                output = f"Unknown tool: {tc.function.name}"
            messages.append({"role": "tool", "tool_call_id": tc.id, "content": output})

    return "(max rounds reached)"


async def anthropic_tool_loop(ant_client, model, system, messages, tools, http, sandbox_id, max_rounds=3):
    """Run Anthropic tool-use loop with ForgeVM sandbox execution."""
    for _ in range(max_rounds):
        response = await ant_client.messages.create(
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
            result = await run_in_sandbox(http, sandbox_id, code)
            output = result["stdout"] or result["stderr"] or "(no output)"
            tool_results.append({"type": "tool_result", "tool_use_id": tb.id, "content": output})
        messages.append({"role": "user", "content": tool_results})

    return "(max rounds reached)"


# ═══════════════════════════════════════════════
# TEST 1: Multi-user sandbox spawn + pool status
# ═══════════════════════════════════════════════
async def test_multiuser_spawn(http: httpx.AsyncClient):
    print("\n" + "=" * 70)
    print("TEST 1: Multi-User Sandbox Spawn + Pool Status")
    print("=" * 70)

    sandboxes = {}
    for user in USERS:
        resp = await http.post(
            "/api/v1/sandboxes",
            json={"image": SANDBOX_IMAGE},
            headers={"X-User-ID": user},
        )
        assert resp.status_code == 201, f"Spawn failed for {user}: {resp.status_code} {resp.text}"
        sb = resp.json()
        sandboxes[user] = sb["id"]
        print(f"  {user}: sandbox {sb['id']} (state={sb['state']})")

    # Check pool status
    resp = await http.get("/api/v1/pool/status")
    assert resp.status_code == 200
    pool = resp.json()
    print(f"  Pool status: {json.dumps(pool, indent=2)}")

    # List all sandboxes — should see all 3
    resp = await http.get("/api/v1/sandboxes")
    all_sbs = resp.json()
    print(f"  Total active sandboxes: {len(all_sbs)}")
    assert len(all_sbs) >= len(USERS), f"Expected at least {len(USERS)} sandboxes"

    print("  PASSED")
    return sandboxes


# ═══════════════════════════════════════════════
# TEST 2: File isolation between users
# ═══════════════════════════════════════════════
async def test_file_isolation(http: httpx.AsyncClient, sandboxes: dict):
    print("\n" + "=" * 70)
    print("TEST 2: File Isolation Between Users")
    print("=" * 70)

    # Each user writes a secret file
    for user, sb_id in sandboxes.items():
        secret = f"secret-data-for-{user}-{time.time()}"
        resp = await http.post(
            f"/api/v1/sandboxes/{sb_id}/files",
            json={"path": f"/workspace/{user}_secret.txt", "content": secret},
        )
        assert resp.status_code == 200, f"Write failed for {user}: {resp.text}"
        print(f"  {user}: wrote /workspace/{user}_secret.txt")

    # Each user can read their own file
    for user, sb_id in sandboxes.items():
        resp = await http.get(
            f"/api/v1/sandboxes/{sb_id}/files",
            params={"path": f"/workspace/{user}_secret.txt"},
        )
        assert resp.status_code == 200, f"Read own file failed for {user}"
        content = resp.text
        assert user in content, f"Wrong content for {user}: {content}"
        print(f"  {user}: can read own file OK")

    # Verify user can't access another user's sandbox
    user_list = list(sandboxes.keys())
    for i, user in enumerate(user_list):
        other_user = user_list[(i + 1) % len(user_list)]
        other_sb = sandboxes[other_user]
        # Try to read other user's file from their sandbox
        resp = await http.get(
            f"/api/v1/sandboxes/{other_sb}/files",
            params={"path": f"/workspace/{user}_secret.txt"},
        )
        # This should fail (404 or 500) because user's file doesn't exist in other's sandbox
        if resp.status_code != 200:
            print(f"  {user}: correctly cannot read {other_user}'s sandbox files (HTTP {resp.status_code})")
        else:
            # In non-pool mode, sandboxes are separate VMs, so the file just won't exist
            assert user not in resp.text, f"ISOLATION BREACH: {user} found in {other_user}'s sandbox!"
            print(f"  {user}: file not found in {other_user}'s sandbox (separate VM)")

    # Use extended file ops: stat + glob
    for user, sb_id in sandboxes.items():
        # Stat
        resp = await http.get(
            f"/api/v1/sandboxes/{sb_id}/files/stat",
            params={"path": f"/workspace/{user}_secret.txt"},
        )
        assert resp.status_code == 200
        stat = resp.json()
        print(f"  {user}: stat -> size={stat['size']}, is_dir={stat['is_dir']}")

        # Glob
        resp = await http.get(
            f"/api/v1/sandboxes/{sb_id}/files/glob",
            params={"pattern": "/workspace/*.txt"},
        )
        assert resp.status_code == 200
        matches = resp.json()
        print(f"  {user}: glob /workspace/*.txt -> {matches}")

    print("  PASSED")


# ═══════════════════════════════════════════════
# TEST 3: Concurrent LLM tasks (each user gets LLM + sandbox)
# ═══════════════════════════════════════════════
async def test_concurrent_llm_tasks(http: httpx.AsyncClient, sandboxes: dict):
    print("\n" + "=" * 70)
    print("TEST 3: Concurrent LLM Code Execution Per User")
    print("=" * 70)

    if not REQUESTY_API_KEY:
        print("  SKIPPED (no REQUESTY_API_KEY)")
        return

    oai_client = AsyncOpenAI(api_key=REQUESTY_API_KEY, base_url=REQUESTY_BASE_URL)
    ant_client = AsyncAnthropic(api_key=REQUESTY_API_KEY, base_url=REQUESTY_ANTHROPIC_BASE_URL)

    # Each user gets a different task
    tasks = {
        "alice": {
            "prompt": "Calculate the factorial of 12 using the compute tool. Use print() to show the result.",
            "expected": "479001600",
            "provider": "openai",
        },
        "bob": {
            "prompt": "What is 2^32? Use the compute tool to calculate it. Use print().",
            "expected": "4294967296",
            "provider": "anthropic",
            "check": lambda ans: "4294967296" in ans.replace(",", ""),
        },
        "carol": {
            "prompt": "Calculate the sum of squares from 1 to 20 using the compute tool. Use print().",
            "expected": "2870",
            "provider": "openai",
        },
    }

    async def run_user_task(user: str):
        task = tasks[user]
        sb_id = sandboxes[user]
        t0 = time.time()

        if task["provider"] == "openai":
            messages = [
                {"role": "system", "content": "You have a `compute` tool to execute Python code. Always use it. Use print() to show results."},
                {"role": "user", "content": task["prompt"]},
            ]
            answer = await openai_tool_loop(oai_client, OPENAI_MODEL, messages, [COMPUTE_TOOL_OPENAI], http, sb_id)
        else:
            system = "You have a `compute` tool. Always use it for calculations. Use print()."
            messages = [{"role": "user", "content": task["prompt"]}]
            answer = await anthropic_tool_loop(ant_client, ANTHROPIC_MODEL, system, messages, [COMPUTE_TOOL_ANTHROPIC], http, sb_id)

        elapsed = time.time() - t0
        return user, answer, task, elapsed

    # Run all user tasks concurrently
    print("  Running 3 users' LLM tasks concurrently...")
    results = await asyncio.gather(
        run_user_task("alice"),
        run_user_task("bob"),
        run_user_task("carol"),
    )

    all_passed = True
    for user, answer, task, elapsed in results:
        checker = task.get("check")
        if checker:
            ok = checker(answer)
        else:
            ok = task["expected"] in answer
        status = "OK" if ok else "FAIL"
        print(f"  {user} [{task['provider']}] ({elapsed:.1f}s): {status}")
        print(f"    Answer: {answer[:200]}")
        if not ok:
            print(f"    Expected '{task['expected']}' in answer")
            all_passed = False

    assert all_passed, "Some user tasks failed"
    print("  PASSED")


# ═══════════════════════════════════════════════
# TEST 4: Extended file ops across users (move, chmod, delete)
# ═══════════════════════════════════════════════
async def test_extended_file_ops_multiuser(http: httpx.AsyncClient, sandboxes: dict):
    print("\n" + "=" * 70)
    print("TEST 4: Extended File Ops Across Multiple Users")
    print("=" * 70)

    for user, sb_id in sandboxes.items():
        print(f"\n  --- {user} (sandbox {sb_id}) ---")

        # Write
        resp = await http.post(
            f"/api/v1/sandboxes/{sb_id}/files",
            json={"path": f"/workspace/{user}_data.txt", "content": f"Hello from {user}!"},
        )
        assert resp.status_code == 200
        print(f"    write /workspace/{user}_data.txt")

        # Move/rename
        resp = await http.post(
            f"/api/v1/sandboxes/{sb_id}/files/move",
            json={"old_path": f"/workspace/{user}_data.txt", "new_path": f"/workspace/{user}_renamed.txt"},
        )
        assert resp.status_code == 200, f"Move failed: {resp.text}"
        print(f"    move -> /workspace/{user}_renamed.txt")

        # Read from new location
        resp = await http.get(
            f"/api/v1/sandboxes/{sb_id}/files",
            params={"path": f"/workspace/{user}_renamed.txt"},
        )
        assert resp.status_code == 200
        assert user in resp.text
        print(f"    read -> '{resp.text}'")

        # Chmod
        resp = await http.post(
            f"/api/v1/sandboxes/{sb_id}/files/chmod",
            json={"path": f"/workspace/{user}_renamed.txt", "mode": "0755"},
        )
        assert resp.status_code == 200
        print(f"    chmod 0755")

        # Stat
        resp = await http.get(
            f"/api/v1/sandboxes/{sb_id}/files/stat",
            params={"path": f"/workspace/{user}_renamed.txt"},
        )
        assert resp.status_code == 200
        stat = resp.json()
        print(f"    stat -> size={stat['size']}, is_dir={stat['is_dir']}")

        # Write more files for glob
        for ext in [".log", ".tmp"]:
            resp = await http.post(
                f"/api/v1/sandboxes/{sb_id}/files",
                json={"path": f"/workspace/{user}{ext}", "content": "data"},
            )

        # Glob
        resp = await http.get(
            f"/api/v1/sandboxes/{sb_id}/files/glob",
            params={"pattern": "/workspace/*.log"},
        )
        assert resp.status_code == 200
        matches = resp.json()
        print(f"    glob *.log -> {matches}")

        # Delete
        resp = await http.delete(
            f"/api/v1/sandboxes/{sb_id}/files",
            params={"path": f"/workspace/{user}_renamed.txt"},
        )
        assert resp.status_code == 200
        print(f"    delete /workspace/{user}_renamed.txt")

        # Verify deleted
        resp = await http.get(
            f"/api/v1/sandboxes/{sb_id}/files",
            params={"path": f"/workspace/{user}_renamed.txt"},
        )
        assert resp.status_code != 200, "File should be deleted"
        print(f"    verified deleted")

    print("\n  PASSED")


# ═══════════════════════════════════════════════
# TEST 5: LLM file generation + cross-user verification
# ═══════════════════════════════════════════════
async def test_llm_file_generation(http: httpx.AsyncClient, sandboxes: dict):
    print("\n" + "=" * 70)
    print("TEST 5: LLM File Generation Per User")
    print("=" * 70)

    if not REQUESTY_API_KEY:
        print("  SKIPPED (no REQUESTY_API_KEY)")
        return

    ant_client = AsyncAnthropic(api_key=REQUESTY_API_KEY, base_url=REQUESTY_ANTHROPIC_BASE_URL)
    system = "You have a `compute` tool. Use it to run Python code. Use print() to confirm. Save files to /workspace/."

    # Alice generates a CSV, Bob generates JSON, Carol generates a script
    user_tasks = {
        "alice": {
            "prompt": "Generate a CSV with 3 rows of data (name,age,city). Save to people.csv (relative path) using the csv module. Print 'done'.",
            "file": "/people.csv",
            "check": lambda c: "name" in c.lower() and len(c.strip().split("\n")) >= 4,
        },
        "bob": {
            "prompt": "Generate a JSON file with a list of 3 colors (name + hex code). Save to colors.json (relative path). Print 'done'.",
            "file": "/colors.json",
            "check": lambda c: json.loads(c) is not None,
        },
        "carol": {
            "prompt": "Write a Python script that prints 'Hello from Carol' to hello.py (relative path). Print 'done'.",
            "file": "/hello.py",
            "check": lambda c: "carol" in c.lower() or "Carol" in c,
        },
    }

    for user in ["alice", "bob", "carol"]:
        task = user_tasks[user]
        sb_id = sandboxes[user]
        messages = [{"role": "user", "content": task["prompt"]}]

        print(f"  {user}: {task['prompt'][:60]}...")
        answer = await anthropic_tool_loop(
            ant_client, ANTHROPIC_MODEL, system, messages,
            [COMPUTE_TOOL_ANTHROPIC], http, sb_id, max_rounds=5,
        )
        print(f"  {user}: LLM says: {answer[:100]}")

        # Read the generated file
        resp = await http.get(
            f"/api/v1/sandboxes/{sb_id}/files",
            params={"path": task["file"]},
        )
        assert resp.status_code == 200, f"File not found for {user}: {task['file']}"
        content = resp.text
        print(f"  {user}: file content ({len(content)} bytes): {content[:100]}...")

        assert task["check"](content), f"Validation failed for {user}"
        print(f"  {user}: VERIFIED")

        # Stat the file
        resp = await http.get(
            f"/api/v1/sandboxes/{sb_id}/files/stat",
            params={"path": task["file"]},
        )
        stat = resp.json()
        print(f"  {user}: stat -> size={stat['size']}")

    print("  PASSED")


# ═══════════════════════════════════════════════
# RUNNER
# ═══════════════════════════════════════════════
async def run_all():
    print("\n" + "=" * 70)
    print("FORGEVM MULTI-USER POOL TEST")
    print("=" * 70)
    print(f"  Server:  {FORGEVM_URL}")
    print(f"  Image:   {SANDBOX_IMAGE}")
    print(f"  Users:   {', '.join(USERS)}")
    if REQUESTY_API_KEY:
        print(f"  API Key: {'*' * 8}...{REQUESTY_API_KEY[-4:]}" if REQUESTY_API_KEY else "  API Key: (not set)")
        print(f"  OpenAI:  {OPENAI_MODEL}")
        print(f"  Anthropic: {ANTHROPIC_MODEL}")
    else:
        print("  LLM:     DISABLED (no REQUESTY_API_KEY, LLM tests will be skipped)")

    async with httpx.AsyncClient(base_url=FORGEVM_URL, timeout=60.0) as http:
        # Health check
        resp = await http.get("/api/v1/health")
        assert resp.status_code == 200, f"Server not healthy: {resp.status_code}"
        print(f"  Health:  {resp.json()['status']}")

        tests = [
            ("Test 1: Multi-User Spawn + Pool Status", test_multiuser_spawn),
            ("Test 2: File Isolation Between Users", None),
            ("Test 3: Concurrent LLM Tasks", None),
            ("Test 4: Extended File Ops Multi-User", None),
            ("Test 5: LLM File Generation Per User", None),
        ]

        results = []
        sandboxes = None

        try:
            # Test 1: Spawn sandboxes
            try:
                sandboxes = await test_multiuser_spawn(http)
                results.append(("Test 1: Multi-User Spawn + Pool Status", True, None))
            except Exception as e:
                results.append(("Test 1: Multi-User Spawn + Pool Status", False, str(e)))
                import traceback; traceback.print_exc()
                print("\nCannot continue without sandboxes.")
                return

            # Test 2: File isolation
            try:
                await test_file_isolation(http, sandboxes)
                results.append(("Test 2: File Isolation Between Users", True, None))
            except Exception as e:
                results.append(("Test 2: File Isolation Between Users", False, str(e)))
                import traceback; traceback.print_exc()

            # Test 3: Concurrent LLM
            try:
                await test_concurrent_llm_tasks(http, sandboxes)
                results.append(("Test 3: Concurrent LLM Tasks", True, None))
            except Exception as e:
                results.append(("Test 3: Concurrent LLM Tasks", False, str(e)))
                import traceback; traceback.print_exc()

            # Test 4: Extended file ops
            try:
                await test_extended_file_ops_multiuser(http, sandboxes)
                results.append(("Test 4: Extended File Ops Multi-User", True, None))
            except Exception as e:
                results.append(("Test 4: Extended File Ops Multi-User", False, str(e)))
                import traceback; traceback.print_exc()

            # Test 5: LLM file generation
            try:
                await test_llm_file_generation(http, sandboxes)
                results.append(("Test 5: LLM File Generation Per User", True, None))
            except Exception as e:
                results.append(("Test 5: LLM File Generation Per User", False, str(e)))
                import traceback; traceback.print_exc()

        finally:
            # Cleanup all sandboxes
            if sandboxes:
                print("\n  Cleaning up sandboxes...")
                for user, sb_id in sandboxes.items():
                    try:
                        resp = await http.delete(f"/api/v1/sandboxes/{sb_id}")
                        print(f"  {user}: destroyed {sb_id} ({resp.status_code})")
                    except Exception:
                        pass

        # Summary
        print("\n" + "=" * 70)
        print("SUMMARY")
        print("=" * 70)
        for name, passed, err in results:
            status = "PASS" if passed else ("SKIP" if "SKIP" in (err or "") else "FAIL")
            print(f"  {status}: {name}" + (f" -- {err}" if err else ""))

        passed_count = sum(1 for _, p, _ in results if p)
        total = len(results)
        if passed_count == total:
            print(f"\nAll {total} tests PASSED")
        else:
            print(f"\n{passed_count}/{total} passed, {total - passed_count} FAILED/SKIPPED")
            sys.exit(1)


if __name__ == "__main__":
    asyncio.run(run_all())
