package ai.dohaeris.forgevm

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertTrue
import kotlinx.coroutines.flow.toList
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer

class ClientTest {
    @Test
    fun `spawn sends expected request and parses sandbox response`() = runBlocking {
        MockWebServer().use { server ->
            server.enqueueJson(
                """
                {
                  "id": "sb-123",
                  "state": "running",
                  "provider": "mock",
                  "image": "alpine:latest",
                  "memory_mb": 512,
                  "vcpus": 1,
                  "created_at": "2026-04-10T00:00:00Z",
                  "expires_at": "",
                  "metadata": {"source": "android"}
                }
                """.trimIndent(),
            )

            val client = Client(server.url("/").toString(), apiKey = "secret")
            val sandbox = client.spawn(
                image = "alpine:latest",
                ttl = "30m",
                metadata = mapOf("source" to "android"),
            )

            val request = server.takeRequest()
            assertEquals("/api/v1/sandboxes", request.path)
            assertEquals("POST", request.method)
            assertEquals("secret", request.getHeader("X-API-Key"))
            assertTrue(request.body.readUtf8().contains("\"ttl\":\"30m\""))

            assertEquals("sb-123", sandbox.id)
            assertEquals("running", sandbox.state)
            assertEquals("android", sandbox.info.metadata["source"])
        }
    }

    @Test
    fun `sandbox operations cover exec files ttl and destroy`() = runBlocking {
        MockWebServer().use { server ->
            server.enqueueJson("""{"exit_code":0,"stdout":"hello\n","stderr":"","duration":"2ms"}""")
            server.enqueueJson("")
            server.enqueue(MockResponse().setResponseCode(200).setBody("file body"))
            server.enqueueJson("""[{"name":"tmp","path":"/tmp","size":0,"is_dir":true,"mod_time":"2026-04-10T00:00:00Z","mode":"0755"}]""")
            server.enqueueJson(
                """
                {
                  "id": "sb-123",
                  "state": "running",
                  "provider": "mock",
                  "image": "alpine:latest",
                  "memory_mb": 512,
                  "vcpus": 1,
                  "created_at": "2026-04-10T00:00:00Z",
                  "expires_at": "2026-04-10T00:30:00Z",
                  "metadata": {}
                }
                """.trimIndent(),
            )
            server.enqueueJson("")

            val sandbox = Sandbox(
                api = ApiClient(server.url("/").toString().removeSuffix("/"), null, 30_000, null),
                infoSnapshot = SandboxInfo(
                    id = "sb-123",
                    state = "running",
                    provider = "mock",
                    image = "alpine:latest",
                ),
            )

            val exec = sandbox.exec("echo hello")
            sandbox.writeFile("/tmp/demo.txt", "abc", "0644")
            val content = sandbox.readFile("/tmp/demo.txt")
            val files = sandbox.listFiles("/tmp")
            sandbox.extendTtl("30m")
            sandbox.destroy()

            assertEquals("hello\n", exec.stdout)
            assertEquals("file body", content)
            assertEquals("/tmp", files.single().path)
            assertEquals("destroyed", sandbox.state)

            val requests = List(6) { server.takeRequest() }
            assertEquals("/api/v1/sandboxes/sb-123/exec", requests[0].path)
            assertEquals("/api/v1/sandboxes/sb-123/files", requests[1].path)
            assertEquals("/api/v1/sandboxes/sb-123/files?path=%2Ftmp%2Fdemo.txt", requests[2].path)
            assertEquals("/api/v1/sandboxes/sb-123/files/list?path=%2Ftmp", requests[3].path)
            assertEquals("/api/v1/sandboxes/sb-123/extend", requests[4].path)
            assertEquals("/api/v1/sandboxes/sb-123", requests[5].path)
        }
    }

    @Test
    fun `execStream parses ndjson chunks`() = runBlocking {
        MockWebServer().use { server ->
            server.enqueue(
                MockResponse()
                    .setResponseCode(200)
                    .setHeader("Content-Type", "application/x-ndjson")
                    .setBody(
                        """
                        {"stream":"stdout","data":"hello "}
                        {"stream":"stderr","data":"warn"}
                        {"stream":"stdout","data":"world"}
                        """.trimIndent() + "\n",
                    ),
            )

            val sandbox = Sandbox(
                api = ApiClient(server.url("/").toString().removeSuffix("/"), null, 30_000, null),
                infoSnapshot = SandboxInfo(
                    id = "sb-stream",
                    state = "running",
                    provider = "mock",
                    image = "alpine:latest",
                ),
            )

            val chunks = sandbox.execStream("echo hi").toList()

            assertEquals(3, chunks.size)
            assertEquals("stdout", chunks[0].stream)
            assertEquals("hello ", chunks[0].data)
            assertEquals("warn", chunks[1].data)
            assertEquals("world", chunks[2].data)
        }
    }

    @Test
    fun `template manager and cleanup flow work`() = runBlocking {
        MockWebServer().use { server ->
            server.enqueueJson("""[{"name":"python-dev","image":"python:3.12-slim","memory_mb":1024,"vcpus":2,"ttl":"30m","provider":"mock","metadata":{}}]""")
            server.enqueueJson("""{"name":"python-dev","image":"python:3.12-slim","memory_mb":1024,"vcpus":2,"ttl":"30m","provider":"mock","metadata":{}}""")
            server.enqueueJson(
                """
                {
                  "id": "sb-template",
                  "state": "running",
                  "provider": "mock",
                  "image": "python:3.12-slim",
                  "memory_mb": 1024,
                  "vcpus": 2,
                  "created_at": "2026-04-10T00:00:00Z",
                  "expires_at": "",
                  "metadata": {}
                }
                """.trimIndent(),
            )
            server.enqueueJson(
                """
                {
                  "id": "sb-auto",
                  "state": "running",
                  "provider": "mock",
                  "image": "alpine:latest",
                  "memory_mb": 512,
                  "vcpus": 1,
                  "created_at": "2026-04-10T00:00:00Z",
                  "expires_at": "",
                  "metadata": {}
                }
                """.trimIndent(),
            )
            server.enqueueJson("")

            val client = Client(server.url("/").toString())
            val templates = client.templates.list()
            val saved = client.templates.save(
                TemplateConfig(
                    name = "python-dev",
                    image = "python:3.12-slim",
                    memoryMb = 1024,
                    vcpus = 2,
                ),
            )
            val spawned = client.templates.spawn("python-dev")
            val result = client.withSandbox {
                it.id
            }

            assertEquals("python-dev", templates.single().name)
            assertEquals("python-dev", saved.name)
            assertEquals("sb-template", spawned.id)
            assertEquals("sb-auto", result)

            val requests = List(5) { server.takeRequest() }
            assertEquals("/api/v1/templates", requests[0].path)
            assertEquals("/api/v1/templates", requests[1].path)
            assertEquals("/api/v1/templates/python-dev/spawn", requests[2].path)
            assertEquals("/api/v1/sandboxes", requests[3].path)
            assertEquals("/api/v1/sandboxes/sb-auto", requests[4].path)
        }
    }

    @Test
    fun `404 maps to sandbox not found exception`() = runBlocking {
        MockWebServer().use { server ->
            server.enqueueJson("""{"code":"NOT_FOUND","message":"sandbox missing"}""", 404)

            val client = Client(server.url("/").toString())

            val error = assertFailsWith<SandboxNotFoundException> {
                client.get("sb-missing")
            }

            assertEquals("sb-missing", error.sandboxId)
        }
    }

    private fun MockWebServer.enqueueJson(body: String, status: Int = 200) {
        enqueue(
            MockResponse()
                .setResponseCode(status)
                .setHeader("Content-Type", "application/json")
                .setBody(body),
        )
    }
}
