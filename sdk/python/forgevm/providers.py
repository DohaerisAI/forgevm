"""Provider config helpers."""

from __future__ import annotations

import httpx

from forgevm.exceptions import handle_response


class ProviderManager:
    """Manage provider configurations."""

    def __init__(self, http: httpx.Client):
        self._http = http

    def list(self) -> list[dict]:
        """List all providers."""
        resp = self._http.get("/api/v1/providers")
        handle_response(resp)
        return resp.json()

    def test(self) -> dict[str, bool]:
        """Test all provider health."""
        resp = self._http.post("/api/v1/providers/test")
        handle_response(resp)
        return resp.json()
