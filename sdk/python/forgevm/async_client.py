"""ForgeVM Async Client — async variant using httpx.AsyncClient."""

from __future__ import annotations

import httpx

from forgevm.async_sandbox import AsyncSandbox
from forgevm.exceptions import ConnectionError, handle_response
from forgevm.models import SandboxInfo


class AsyncClient:
    """Async client for the ForgeVM API server.

    Usage:
        async with AsyncClient("http://localhost:7423") as client:
            sandbox = await client.spawn(image="alpine:latest")
            result = await sandbox.exec("echo hello")
            print(result.stdout)
            await sandbox.destroy()
    """

    def __init__(
        self,
        base_url: str = "http://localhost:7423",
        api_key: str | None = None,
        timeout: float = 30.0,
    ):
        headers = {}
        if api_key:
            headers["X-API-Key"] = api_key

        self._http = httpx.AsyncClient(
            base_url=base_url,
            headers=headers,
            timeout=timeout,
        )

    async def spawn(
        self,
        image: str = "alpine:latest",
        provider: str | None = None,
        memory_mb: int | None = None,
        vcpus: int | None = None,
        ttl: str | None = None,
        template: str | None = None,
        metadata: dict[str, str] | None = None,
    ) -> AsyncSandbox:
        """Spawn a new sandbox."""
        body: dict = {"image": image}
        if provider:
            body["provider"] = provider
        if memory_mb:
            body["memory_mb"] = memory_mb
        if vcpus:
            body["vcpus"] = vcpus
        if ttl:
            body["ttl"] = ttl
        if template:
            body["template"] = template
        if metadata:
            body["metadata"] = metadata

        try:
            resp = await self._http.post("/api/v1/sandboxes", json=body)
        except httpx.ConnectError as e:
            raise ConnectionError(f"Cannot connect to ForgeVM server: {e}")

        handle_response(resp)
        data = resp.json()
        return AsyncSandbox(self._http, data["id"], info=data)

    async def spawn_template(self, template_name: str) -> AsyncSandbox:
        """Spawn a sandbox from a saved template."""
        try:
            resp = await self._http.post(f"/api/v1/templates/{template_name}/spawn")
        except httpx.ConnectError as e:
            raise ConnectionError(f"Cannot connect to ForgeVM server: {e}")

        handle_response(resp)
        data = resp.json()
        return AsyncSandbox(self._http, data["id"], info=data)

    async def get(self, sandbox_id: str) -> AsyncSandbox:
        """Get an existing sandbox by ID."""
        resp = await self._http.get(f"/api/v1/sandboxes/{sandbox_id}")
        handle_response(resp)
        data = resp.json()
        return AsyncSandbox(self._http, data["id"], info=data)

    async def list(self) -> list[SandboxInfo]:
        """List all active sandboxes."""
        resp = await self._http.get("/api/v1/sandboxes")
        handle_response(resp)
        return [
            SandboxInfo(
                id=s["id"],
                state=s["state"],
                provider=s["provider"],
                image=s["image"],
                memory_mb=s.get("memory_mb", 512),
                vcpus=s.get("vcpus", 1),
                created_at=s.get("created_at", ""),
                expires_at=s.get("expires_at", ""),
                metadata=s.get("metadata", {}),
            )
            for s in resp.json()
        ]

    async def prune(self) -> int:
        """Prune expired sandboxes. Returns count pruned."""
        resp = await self._http.delete("/api/v1/sandboxes")
        handle_response(resp)
        return resp.json().get("pruned", 0)

    async def health(self) -> dict:
        """Check server health."""
        resp = await self._http.get("/api/v1/health")
        handle_response(resp)
        return resp.json()

    async def close(self) -> None:
        """Close the HTTP client."""
        await self._http.aclose()

    async def __aenter__(self):
        return self

    async def __aexit__(self, *args):
        await self.close()
