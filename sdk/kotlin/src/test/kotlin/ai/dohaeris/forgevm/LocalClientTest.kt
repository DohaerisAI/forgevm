package ai.dohaeris.forgevm

import java.io.File
import java.nio.file.Files
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse
import kotlin.test.assertTrue
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.toList
import kotlinx.coroutines.runBlocking

class LocalClientTest {
    @Test
    fun `local client runs commands and manages files`() = runBlocking {
        val root = tempDir("forgevm-local-test-")
        try {
            val client = LocalClient(LocalClientConfig(rootDir = root))
            val sandbox = client.spawn(image = "local-shell")

            sandbox.writeFile("/script.sh", "echo local-run\n", "0700")
            val exec = sandbox.exec("sh", ExecOptions(args = listOf("script.sh")))
            val files = sandbox.listFiles("/")
            val content = sandbox.readFile("/script.sh")

            assertEquals(0, exec.exitCode)
            assertEquals("local-run\n", exec.stdout)
            assertTrue(files.any { it.path == "/script.sh" })
            assertTrue(content.contains("local-run"))
        } finally {
            root.deleteRecursively()
        }
    }

    @Test
    fun `local execStream emits stdout and stderr`() = runBlocking {
        val root = tempDir("forgevm-local-stream-")
        try {
            val client = LocalClient(LocalClientConfig(rootDir = root))
            val sandbox = client.spawn(image = "local-shell")

            val chunks = sandbox.execStream("printf 'a\\n'; printf 'b\\n' 1>&2").toList()

            assertEquals(listOf("stderr", "stdout"), chunks.map { it.stream }.sorted())
            assertTrue(chunks.any { it.data == "a\n" })
            assertTrue(chunks.any { it.data == "b\n" })
        } finally {
            root.deleteRecursively()
        }
    }

    @Test
    fun `local templates and withSandbox work`() = runBlocking {
        val root = tempDir("forgevm-local-template-")
        try {
            val client = Client.local(LocalClientConfig(rootDir = root))
            client.templates.save(
                TemplateConfig(
                    name = "android-tool",
                    image = "local-shell",
                    ttl = "5m",
                ),
            )

            val sandbox = client.templates.spawn("android-tool")
            val result = client.withSandbox {
                it.exec("printf 'ok'").stdout
            }

            assertEquals("local-shell", sandbox.image)
            assertEquals("ok", result.trim())
        } finally {
            root.deleteRecursively()
        }
    }

    @Test
    fun `local prune removes expired sandboxes`() = runBlocking {
        val root = tempDir("forgevm-local-prune-")
        try {
            val client = LocalClient(LocalClientConfig(rootDir = root))
            val sandbox = client.spawn(ttl = "1ms")
            delay(10)

            val pruned = client.prune()

            assertEquals(1, pruned)
            assertFailsWith<SandboxNotFoundException> {
                client.get(sandbox.id)
            }
        } finally {
            root.deleteRecursively()
        }
    }

    @Test
    fun `local timeout and path escape are enforced`() = runBlocking {
        val root = tempDir("forgevm-local-guards-")
        try {
            val client = LocalClient(LocalClientConfig(rootDir = root))
            val sandbox = client.spawn(image = "local-shell")

            val timeout = sandbox.exec("sleep 1", ExecOptions(timeout = "10ms"))

            assertEquals(-1, timeout.exitCode)
            assertTrue(timeout.stderr.contains("timed out"))

            val pathError = assertFailsWith<ForgevmException> {
                sandbox.writeFile("/../../escape.txt", "nope")
            }
            assertEquals("INVALID_PATH", pathError.code)
        } finally {
            root.deleteRecursively()
        }
    }

    @Test
    fun `withSandbox destroys local sandbox directory`() = runBlocking {
        val root = tempDir("forgevm-local-cleanup-")
        try {
            val client = LocalClient(LocalClientConfig(rootDir = root))
            var sandboxDir: File? = null

            client.withSandbox {
                sandboxDir = File(root, it.id)
                assertTrue(sandboxDir!!.exists())
                it.id
            }

            assertFalse(sandboxDir!!.exists())
        } finally {
            root.deleteRecursively()
        }
    }

    private fun tempDir(prefix: String): File =
        Files.createTempDirectory(prefix).toFile()
}
