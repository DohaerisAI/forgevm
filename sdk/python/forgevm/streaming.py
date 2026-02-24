"""NDJSON streaming support for exec output."""

from __future__ import annotations

import json
from dataclasses import dataclass
from typing import Iterator

import httpx


@dataclass
class StreamChunk:
    """A chunk of streaming exec output."""

    stream: str  # "stdout" or "stderr"
    data: str


def iter_ndjson(response: httpx.Response) -> Iterator[StreamChunk]:
    """Iterate over NDJSON lines from a streaming response."""
    for line in response.iter_lines():
        line = line.strip()
        if not line:
            continue
        try:
            obj = json.loads(line)
            yield StreamChunk(
                stream=obj.get("stream", "stdout"),
                data=obj.get("data", ""),
            )
        except json.JSONDecodeError:
            continue
