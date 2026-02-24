"""ForgeVM Sandbox — represents a single sandbox instance."""

from __future__ import annotations

from typing import Iterator

import httpx

from forgevm.exceptions import handle_response
from forgevm.models import ExecResult
from forgevm.streaming import StreamChunk, iter_ndjson


class Sandbox:
    """A ForgeVM sandbox instance. Supports context manager for auto-cleanup."""

    def __init__(self, http: httpx.Client, sandbox_id: str, info: dict | None = None):
        self._http = http
        self.id = sandbox_id
        self._info = info or {}

    @property
    def state(self) -> str:
        return self._info.get("state", "unknown")

    @property
    def provider(self) -> str:
        return self._info.get("provider", "")

    @property
    def image(self) -> str:
        return self._info.get("image", "")

    def exec(
        self,
        command: str,
        args: list[str] | None = None,
        env: dict[str, str] | None = None,
        workdir: str | None = None,
        timeout: str | None = None,
    ) -> ExecResult:
        """Execute a command in the sandbox."""
        body: dict = {"command": command}
        if args:
            body["args"] = args
        if env:
            body["env"] = env
        if workdir:
            body["workdir"] = workdir
        if timeout:
            body["timeout"] = timeout

        resp = self._http.post(f"/api/v1/sandboxes/{self.id}/exec", json=body)
        handle_response(resp)
        data = resp.json()
        return ExecResult(
            exit_code=data["exit_code"],
            stdout=data["stdout"],
            stderr=data["stderr"],
            duration=data.get("duration", ""),
        )

    def exec_stream(
        self,
        command: str,
        args: list[str] | None = None,
        env: dict[str, str] | None = None,
        workdir: str | None = None,
    ) -> Iterator[StreamChunk]:
        """Execute a command and stream output chunks."""
        body: dict = {"command": command, "stream": True}
        if args:
            body["args"] = args
        if env:
            body["env"] = env
        if workdir:
            body["workdir"] = workdir

        with self._http.stream("POST", f"/api/v1/sandboxes/{self.id}/exec", json=body) as resp:
            handle_response(resp)
            yield from iter_ndjson(resp)

    def write_file(self, path: str, content: str, mode: str | None = None) -> None:
        """Write a file to the sandbox."""
        body: dict = {"path": path, "content": content}
        if mode:
            body["mode"] = mode
        resp = self._http.post(f"/api/v1/sandboxes/{self.id}/files", json=body)
        handle_response(resp)

    def read_file(self, path: str) -> str:
        """Read a file from the sandbox."""
        resp = self._http.get(f"/api/v1/sandboxes/{self.id}/files", params={"path": path})
        handle_response(resp)
        return resp.text

    def list_files(self, path: str = "/") -> list[dict]:
        """List files in a sandbox directory."""
        resp = self._http.get(f"/api/v1/sandboxes/{self.id}/files/list", params={"path": path})
        handle_response(resp)
        return resp.json()

    def extend_ttl(self, ttl: str = "30m") -> None:
        """Extend this sandbox's TTL by the given duration."""
        resp = self._http.post(f"/api/v1/sandboxes/{self.id}/extend", json={"ttl": ttl})
        handle_response(resp)
        self.refresh()

    def destroy(self) -> None:
        """Destroy this sandbox."""
        resp = self._http.delete(f"/api/v1/sandboxes/{self.id}")
        handle_response(resp)

    def refresh(self) -> None:
        """Refresh sandbox info from the server."""
        resp = self._http.get(f"/api/v1/sandboxes/{self.id}")
        handle_response(resp)
        self._info = resp.json()

    def __enter__(self):
        return self

    def __exit__(self, *args):
        try:
            self.destroy()
        except Exception:
            pass

    def __repr__(self) -> str:
        return f"Sandbox(id={self.id!r}, state={self.state!r})"
