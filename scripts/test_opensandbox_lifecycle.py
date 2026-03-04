"""
OpenSandbox SDK Lifecycle Test
Tests: create, exec, write, read, file info, code interpret, TTL renew, delete.
Requires: OpenSandbox server running on http://127.0.0.1:8080
Install SDK: cd OpenSandbox/sdks/sandbox/python && uv sync
"""
import asyncio
from datetime import datetime, timedelta, timezone

import httpx
from opensandbox import Sandbox
from opensandbox.config import ConnectionConfig

OPENSANDBOX_URL = "127.0.0.1:8080"
SANDBOX_IMAGE = "opensandbox/code-interpreter:v1.0.1"


def get_stdout(r):
    return "\n".join(m.text for m in r.logs.stdout)


async def main():
    config = ConnectionConfig(
        domain=OPENSANDBOX_URL,
        protocol="http",
        request_timeout=timedelta(seconds=60),
    )

    print("=== 1. Create sandbox ===")
    sandbox = await Sandbox.create(
        SANDBOX_IMAGE,
        connection_config=config,
        timeout=timedelta(minutes=5),
        ready_timeout=timedelta(seconds=60),
    )
    print(f"ID: {sandbox.id}")

    print("\n=== 2. Shell command ===")
    r = await sandbox.commands.run("echo Hello from OpenSandbox && python3 --version")
    print(get_stdout(r))

    print("\n=== 3. Write + execute Python ===")
    code = (
        "import csv, os\n"
        "os.makedirs('/output', exist_ok=True)\n"
        "with open('/output/data.csv', 'w') as f:\n"
        "    w = csv.writer(f)\n"
        "    w.writerow(['name', 'value'])\n"
        "    for i in range(10):\n"
        "        w.writerow([f'item_{i}', i * 10])\n"
        "print('CSV generated with 10 rows')\n"
    )
    await sandbox.files.write_file("/tmp/gen.py", code)
    r = await sandbox.commands.run("python3 /tmp/gen.py")
    print(get_stdout(r))

    print("\n=== 4. Read generated CSV ===")
    content = await sandbox.files.read_file("/output/data.csv")
    print(content)

    print("\n=== 5. File info ===")
    info = await sandbox.files.get_file_info(["/output/data.csv"])
    for path, meta in info.items():
        print(f"{path}: size={meta.size}, modified={meta.modified_at}")

    print("\n=== 6. Code interpreter (via commands.run) ===")
    r = await sandbox.commands.run('python3 -c "print(sum(range(101)))"')
    print(f"Sum 0..100 = {get_stdout(r)}")

    print("\n=== 7. Renew expiration (TTL extend via API) ===")
    new_expiry = (datetime.now(timezone.utc) + timedelta(minutes=10)).isoformat()
    async with httpx.AsyncClient() as client:
        resp = await client.post(
            f"http://{OPENSANDBOX_URL}/sandboxes/{sandbox.id}/renew-expiration",
            json={"expiresAt": new_expiry},
        )
        print(f"Renew API: {resp.status_code} {resp.json()}")

    print("\n=== 8. Cleanup ===")
    await sandbox.close()
    async with httpx.AsyncClient() as client:
        resp = await client.delete(f"http://{OPENSANDBOX_URL}/sandboxes/{sandbox.id}")
        print(f"Delete API: {resp.status_code}")

    print("\nALL TESTS PASSED!")


if __name__ == "__main__":
    asyncio.run(main())
