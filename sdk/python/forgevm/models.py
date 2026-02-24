"""ForgeVM SDK data models."""

from __future__ import annotations

from dataclasses import dataclass, field


@dataclass
class ExecResult:
    """Result of executing a command in a sandbox."""

    exit_code: int
    stdout: str
    stderr: str
    duration: str = ""


@dataclass
class SandboxInfo:
    """Information about a sandbox."""

    id: str
    state: str
    provider: str
    image: str
    memory_mb: int = 512
    vcpus: int = 1
    created_at: str = ""
    expires_at: str = ""
    metadata: dict[str, str] = field(default_factory=dict)


@dataclass
class Template:
    """Sandbox template configuration."""

    name: str
    image: str
    memory_mb: int = 512
    vcpus: int = 1
    ttl: str = "30m"
    provider: str = ""
    metadata: dict[str, str] = field(default_factory=dict)
