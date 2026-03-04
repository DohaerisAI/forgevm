"""ForgeVM AsyncSandbox — async variant of Sandbox."""

from __future__ import annotations

from typing import AsyncIterator

import httpx

from forgevm.exceptions import handle_response
from forgevm.models import ExecResult
from forgevm.streaming import StreamChunk


class AsyncSandbox:
    """An async ForgeVM sandbox instance. Supports async context manager for auto-cleanup."""

    def __init__(self, http: httpx.AsyncClient, sandbox_id: str, info: dict | None = None):
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

    async def exec(
        self,
        command: str,
        args: list[str] | None = None,
        env: dict[str, str] | None = None,
        workdir: str | None = None,
    ) -> ExecResult:
        """Execute a command in the sandbox."""
        body: dict = {"command": command}
        if args:
            body["args"] = args
        if env:
            body["env"] = env
        if workdir:
            body["workdir"] = workdir

        resp = await self._http.post(f"/api/v1/sandboxes/{self.id}/exec", json=body)
        handle_response(resp)
        data = resp.json()
        return ExecResult(
            exit_code=data["exit_code"],
            stdout=data["stdout"],
            stderr=data["stderr"],
            duration=data.get("duration", ""),
        )

    async def exec_stream(
        self,
        command: str,
        args: list[str] | None = None,
        env: dict[str, str] | None = None,
        workdir: str | None = None,
    ) -> AsyncIterator[StreamChunk]:
        """Execute a command and stream output chunks asynchronously."""
        import json as jsonlib

        body: dict = {"command": command, "stream": True}
        if args:
            body["args"] = args
        if env:
            body["env"] = env
        if workdir:
            body["workdir"] = workdir

        async with self._http.stream("POST", f"/api/v1/sandboxes/{self.id}/exec", json=body) as resp:
            handle_response(resp)
            async for line in resp.aiter_lines():
                line = line.strip()
                if not line:
                    continue
                data = jsonlib.loads(line)
                yield StreamChunk(
                    stream=data.get("stream", "stdout"),
                    data=data.get("data", ""),
                )

    async def write_file(self, path: str, content: str, mode: str | None = None) -> None:
        """Write a file to the sandbox."""
        body: dict = {"path": path, "content": content}
        if mode:
            body["mode"] = mode
        resp = await self._http.post(f"/api/v1/sandboxes/{self.id}/files", json=body)
        handle_response(resp)

    async def read_file(self, path: str) -> str:
        """Read a file from the sandbox."""
        resp = await self._http.get(f"/api/v1/sandboxes/{self.id}/files", params={"path": path})
        handle_response(resp)
        return resp.text

    async def list_files(self, path: str = "/") -> list[dict]:
        """List files in a sandbox directory."""
        resp = await self._http.get(f"/api/v1/sandboxes/{self.id}/files/list", params={"path": path})
        handle_response(resp)
        return resp.json()

    async def delete_file(self, path: str, recursive: bool = False) -> None:
        """Delete a file or directory from the sandbox."""
        params = {"path": path}
        if recursive:
            params["recursive"] = "true"
        resp = await self._http.delete(f"/api/v1/sandboxes/{self.id}/files", params=params)
        handle_response(resp)

    async def move_file(self, old_path: str, new_path: str) -> None:
        """Move/rename a file in the sandbox."""
        resp = await self._http.post(
            f"/api/v1/sandboxes/{self.id}/files/move",
            json={"old_path": old_path, "new_path": new_path},
        )
        handle_response(resp)

    async def chmod_file(self, path: str, mode: str) -> None:
        """Change file permissions in the sandbox."""
        resp = await self._http.post(
            f"/api/v1/sandboxes/{self.id}/files/chmod",
            json={"path": path, "mode": mode},
        )
        handle_response(resp)

    async def stat_file(self, path: str) -> dict:
        """Get file info for a single file in the sandbox."""
        resp = await self._http.get(
            f"/api/v1/sandboxes/{self.id}/files/stat", params={"path": path}
        )
        handle_response(resp)
        return resp.json()

    async def glob_files(self, pattern: str) -> list[str]:
        """Return paths matching a glob pattern in the sandbox."""
        resp = await self._http.get(
            f"/api/v1/sandboxes/{self.id}/files/glob", params={"pattern": pattern}
        )
        handle_response(resp)
        return resp.json()

    async def destroy(self) -> None:
        """Destroy this sandbox."""
        resp = await self._http.delete(f"/api/v1/sandboxes/{self.id}")
        handle_response(resp)

    async def refresh(self) -> None:
        """Refresh sandbox info from the server."""
        resp = await self._http.get(f"/api/v1/sandboxes/{self.id}")
        handle_response(resp)
        self._info = resp.json()

    async def __aenter__(self):
        return self

    async def __aexit__(self, *args):
        try:
            await self.destroy()
        except Exception:
            pass

    def __repr__(self) -> str:
        return f"AsyncSandbox(id={self.id!r}, state={self.state!r})"
