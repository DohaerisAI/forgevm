#!/usr/bin/env python3
"""Streaming exec example using the ForgeVM Python SDK.

Demonstrates:
  - Streaming command output in real time with exec_stream
  - Processing stdout and stderr chunks as they arrive
  - Using the sandbox as a context manager for automatic cleanup

Prerequisites:
  pip install forgevm
  # ForgeVM server running at localhost:7423
"""

import sys

from forgevm import Client


def main() -> None:
    with Client("http://localhost:7423") as client:
        # Use a context manager so the sandbox is automatically destroyed
        # when we leave the block, even if an exception occurs.
        with client.spawn(image="alpine:latest", ttl="5m") as sandbox:
            print(f"Sandbox {sandbox.id} ready.\n")

            # --- Example 1: Stream a simple counting loop ---------------
            print("=== Counting to 5 ===")
            for chunk in sandbox.exec_stream("for i in 1 2 3 4 5; do echo $i; sleep 0.2; done"):
                # chunk.stream is "stdout" or "stderr"
                # chunk.data contains the text payload
                sys.stdout.write(chunk.data)
            print()

            # --- Example 2: Interleaved stdout and stderr ---------------
            print("=== Interleaved stdout/stderr ===")
            script = (
                "echo 'out 1'; echo 'err 1' >&2; "
                "echo 'out 2'; echo 'err 2' >&2"
            )
            for chunk in sandbox.exec_stream(script):
                prefix = "[stdout]" if chunk.stream == "stdout" else "[stderr]"
                sys.stdout.write(f"{prefix} {chunk.data}")
            print()

            # --- Example 3: Long-running process ------------------------
            print("=== Generating data ===")
            sandbox.write_file("/tmp/gen.sh", (
                "#!/bin/sh\n"
                "for i in $(seq 1 20); do\n"
                "  echo \"line $i: $(date +%T)\"\n"
                "  sleep 0.1\n"
                "done\n"
            ), mode="0755")

            for chunk in sandbox.exec_stream("sh /tmp/gen.sh"):
                sys.stdout.write(chunk.data)
            print()

        print("Sandbox destroyed (context manager).")


if __name__ == "__main__":
    main()
