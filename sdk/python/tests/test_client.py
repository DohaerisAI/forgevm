"""Tests for ForgeVM Python SDK against a live server."""

import os
import pytest

from forgevm import Client, ExecResult, SandboxNotFound


SERVER_URL = os.environ.get("FORGEVM_URL", "http://localhost:7423")


@pytest.fixture
def client():
    with Client(base_url=SERVER_URL) as c:
        yield c


@pytest.fixture
def sandbox(client):
    sb = client.spawn(image="alpine:latest")
    yield sb
    try:
        sb.destroy()
    except Exception:
        pass


class TestHealth:
    def test_health(self, client):
        result = client.health()
        assert result["status"] == "ok"


class TestSandboxLifecycle:
    def test_spawn_and_destroy(self, client):
        sb = client.spawn(image="alpine:latest")
        assert sb.id.startswith("sb-")
        assert sb.state == "running"
        sb.destroy()

    def test_list(self, client, sandbox):
        sandboxes = client.list()
        assert len(sandboxes) >= 1
        ids = [s.id for s in sandboxes]
        assert sandbox.id in ids

    def test_get(self, client, sandbox):
        got = client.get(sandbox.id)
        assert got.id == sandbox.id

    def test_get_not_found(self, client):
        with pytest.raises(SandboxNotFound):
            client.get("sb-nonexistent")


class TestExec:
    def test_exec_echo(self, sandbox):
        result = sandbox.exec("echo hello")
        assert result.exit_code == 0
        assert "hello" in result.stdout

    def test_exec_exit_code(self, sandbox):
        result = sandbox.exec("exit 42")
        assert result.exit_code == 42

    def test_exec_stderr(self, sandbox):
        result = sandbox.exec("echo oops >&2")
        assert "oops" in result.stderr


class TestFiles:
    def test_write_and_read(self, sandbox):
        sandbox.write_file("/workspace/hello.txt", "hello world")
        content = sandbox.read_file("/workspace/hello.txt")
        assert content == "hello world"

    def test_list_files(self, sandbox):
        sandbox.write_file("/workspace/a.txt", "aaa")
        sandbox.write_file("/workspace/b.txt", "bbb")
        files = sandbox.list_files("/workspace")
        names = [f["path"] for f in files]
        assert any("a.txt" in n for n in names)
        assert any("b.txt" in n for n in names)


class TestContextManager:
    def test_sandbox_context(self, client):
        with client.spawn(image="alpine:latest") as sb:
            result = sb.exec("echo inside context")
            assert result.exit_code == 0
        # After exit, sandbox should be destroyed
        with pytest.raises(SandboxNotFound):
            client.get(sb.id)
