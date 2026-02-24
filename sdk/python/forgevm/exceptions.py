"""ForgeVM SDK exceptions."""

from __future__ import annotations

import httpx


class ForgevmError(Exception):
    """Base exception for ForgeVM SDK."""

    def __init__(self, message: str, code: str | None = None):
        super().__init__(message)
        self.code = code


class SandboxNotFound(ForgevmError):
    """Raised when a sandbox is not found."""

    def __init__(self, sandbox_id: str):
        super().__init__(f"Sandbox {sandbox_id!r} not found", code="NOT_FOUND")
        self.sandbox_id = sandbox_id


class ProviderError(ForgevmError):
    """Raised when a provider operation fails."""

    pass


class ConnectionError(ForgevmError):
    """Raised when the connection to the ForgeVM server fails."""

    pass


def handle_response(response: httpx.Response) -> None:
    """Check response status and raise appropriate exceptions."""
    if response.is_success:
        return

    try:
        data = response.json()
        code = data.get("code", "")
        message = data.get("message", response.text)
    except Exception:
        code = ""
        message = response.text

    if response.status_code == 404:
        raise SandboxNotFound(message)
    if response.status_code == 401:
        raise ForgevmError(message, code="UNAUTHORIZED")
    if response.status_code >= 500:
        raise ProviderError(message, code=code)

    raise ForgevmError(message, code=code)
