"""ForgeVM Client — main entry point for the SDK."""

from __future__ import annotations

import httpx

from forgevm.exceptions import ConnectionError, handle_response
from forgevm.models import SandboxInfo
from forgevm.sandbox import Sandbox


class Client:
    """Client for the ForgeVM API server.

    Usage:
        with Client("http://localhost:7423") as client:
            sandbox = client.spawn(image="alpine:latest")
            result = sandbox.exec("echo hello")
            print(result.stdout)
            sandbox.destroy()
    """

    def __init__(
        self,
        base_url: str = "http://localhost:7423",
        api_key: str | None = None,
        user_id: str | None = None,
        timeout: float = 30.0,
    ):
        headers = {}
        if api_key:
            headers["X-API-Key"] = api_key
        if user_id:
            headers["X-User-ID"] = user_id

        self._http = httpx.Client(
            base_url=base_url,
            headers=headers,
            timeout=timeout,
        )

    def spawn(
        self,
        image: str = "alpine:latest",
        provider: str | None = None,
        memory_mb: int | None = None,
        vcpus: int | None = None,
        ttl: str | None = None,
        metadata: dict[str, str] | None = None,
    ) -> Sandbox:
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
        if metadata:
            body["metadata"] = metadata

        try:
            resp = self._http.post("/api/v1/sandboxes", json=body)
        except httpx.ConnectError as e:
            raise ConnectionError(f"Cannot connect to ForgeVM server: {e}")

        handle_response(resp)
        data = resp.json()
        return Sandbox(self._http, data["id"], info=data)

    def get(self, sandbox_id: str) -> Sandbox:
        """Get an existing sandbox by ID."""
        resp = self._http.get(f"/api/v1/sandboxes/{sandbox_id}")
        handle_response(resp)
        data = resp.json()
        return Sandbox(self._http, data["id"], info=data)

    def list(self) -> list[SandboxInfo]:
        """List all active sandboxes."""
        resp = self._http.get("/api/v1/sandboxes")
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

    def prune(self) -> int:
        """Prune expired sandboxes. Returns count pruned."""
        resp = self._http.delete("/api/v1/sandboxes")
        handle_response(resp)
        return resp.json().get("pruned", 0)

    def pool_status(self) -> dict:
        """Get VM pool status."""
        resp = self._http.get("/api/v1/pool/status")
        handle_response(resp)
        return resp.json()

    def health(self) -> dict:
        """Check server health."""
        resp = self._http.get("/api/v1/health")
        handle_response(resp)
        return resp.json()

    def close(self) -> None:
        """Close the HTTP client."""
        self._http.close()

    def __enter__(self):
        return self

    def __exit__(self, *args):
        self.close()
