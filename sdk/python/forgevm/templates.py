"""Template management helpers."""

from __future__ import annotations

import httpx

from forgevm.exceptions import handle_response
from forgevm.models import Template


class TemplateManager:
    """Manage sandbox templates on the ForgeVM server."""

    def __init__(self, http: httpx.Client):
        self._http = http

    def list(self) -> list[Template]:
        """List all templates."""
        resp = self._http.get("/api/v1/templates")
        handle_response(resp)
        return [self._parse(t) for t in resp.json()]

    def get(self, name: str) -> Template:
        """Get a template by name."""
        resp = self._http.get(f"/api/v1/templates/{name}")
        handle_response(resp)
        return self._parse(resp.json())

    def save(self, template: Template) -> Template:
        """Create or update a template."""
        body = {
            "name": template.name,
            "image": template.image,
            "memory_mb": template.memory_mb,
            "vcpus": template.vcpus,
            "ttl": template.ttl,
            "provider": template.provider,
            "metadata": template.metadata,
        }
        resp = self._http.post("/api/v1/templates", json=body)
        handle_response(resp)
        return self._parse(resp.json())

    def delete(self, name: str) -> None:
        """Delete a template."""
        resp = self._http.delete(f"/api/v1/templates/{name}")
        handle_response(resp)

    @staticmethod
    def _parse(data: dict) -> Template:
        return Template(
            name=data["name"],
            image=data["image"],
            memory_mb=data.get("memory_mb", 512),
            vcpus=data.get("vcpus", 1),
            ttl=data.get("ttl", "30m"),
            provider=data.get("provider", ""),
            metadata=data.get("metadata", {}),
        )
