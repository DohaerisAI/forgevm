package ai.dohaeris.forgevm

import java.io.File
import java.time.Instant
import java.util.UUID
import java.util.concurrent.ConcurrentHashMap
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.channels.awaitClose
import kotlinx.coroutines.channels.trySendBlocking
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.channelFlow
import kotlinx.coroutines.withContext

data class LocalClientConfig(
    val rootDir: File,
    val shellExecutable: String = "/bin/sh",
    val shellArgument: String = "-c",
    val version: String = "local",
)

class LocalClient(
    internal val config: LocalClientConfig,
) : ForgevmClient {
    private val sandboxes = ConcurrentHashMap<String, LocalSandboxState>()
    private val templateStore = ConcurrentHashMap<String, Template>()

    override val templates: TemplateOperations = LocalTemplateOperations()

    init {
        config.rootDir.mkdirs()
    }

    override suspend fun spawn(
        image: String,
        provider: String?,
        memoryMb: Int?,
        vcpus: Int?,
        ttl: String?,
        metadata: Map<String, String>?,
    ): SandboxSession = withContext(Dispatchers.IO) {
        val id = "local-${UUID.randomUUID().toString().replace("-", "").take(12)}"
        val sandboxDir = File(config.rootDir, id)
        sandboxDir.mkdirs()

        val expiresAt = ttl?.let { Instant.now().plusMillis(parseDurationMillis(it)) }
        val info = SandboxInfo(
            id = id,
            state = "running",
            provider = provider ?: "local",
            image = image,
            memoryMb = memoryMb ?: 512,
            vcpus = vcpus ?: 1,
            createdAt = Instant.now().toString(),
            expiresAt = expiresAt?.toString().orEmpty(),
            metadata = metadata ?: emptyMap(),
        )
        val state = LocalSandboxState(
            info = info,
            rootDir = sandboxDir,
            expiresAt = expiresAt,
        )
        sandboxes[id] = state
        LocalSandbox(this@LocalClient, state)
    }

    override suspend fun spawn(options: SpawnOptions): SandboxSession =
        spawn(
            image = options.image,
            provider = options.provider,
            memoryMb = options.memoryMb,
            vcpus = options.vcpus,
            ttl = options.ttl,
            metadata = options.metadata,
        )

    override suspend fun get(sandboxId: String): SandboxSession =
        LocalSandbox(this, requireSandbox(sandboxId))

    override suspend fun list(): List<SandboxInfo> {
        pruneExpired()
        return sandboxes.values.map { it.info }
    }

    override suspend fun prune(): Int {
        return pruneExpired()
    }

    override suspend fun health(): HealthInfo =
        HealthInfo(status = "ok", version = config.version, uptime = "local")

    override suspend fun providers(): List<ProviderInfo> =
        listOf(ProviderInfo(name = "local", healthy = true, default = true))

    override suspend fun <T> withSandbox(
        options: SpawnOptions,
        block: suspend (SandboxSession) -> T,
    ): T {
        val sandbox = spawn(options)
        try {
            return block(sandbox)
        } finally {
            runCatching { sandbox.destroy() }
        }
    }

    internal fun requireSandbox(id: String): LocalSandboxState {
        pruneExpired()
        return sandboxes[id] ?: throw SandboxNotFoundException(id)
    }

    internal fun updateSandbox(state: LocalSandboxState) {
        sandboxes[state.info.id] = state
    }

    internal fun destroySandbox(id: String) {
        val removed = sandboxes.remove(id) ?: throw SandboxNotFoundException(id)
        removed.rootDir.deleteRecursively()
    }

    private fun pruneExpired(): Int {
        val now = Instant.now()
        val expired = sandboxes.values.filter { it.expiresAt?.isBefore(now) == true || it.expiresAt == now }
        expired.forEach {
            sandboxes.remove(it.info.id)
            it.rootDir.deleteRecursively()
        }
        return expired.size
    }

    private inner class LocalTemplateOperations : TemplateOperations {
        override suspend fun list(): List<Template> =
            templateStore.values.sortedBy { it.name }

        override suspend fun get(name: String): Template =
            templateStore[name] ?: throw ForgevmException("Template '$name' not found", "NOT_FOUND", 404)

        override suspend fun save(config: TemplateConfig): Template {
            val template = Template(
                name = config.name,
                image = config.image,
                memoryMb = config.memoryMb ?: 512,
                vcpus = config.vcpus ?: 1,
                ttl = config.ttl ?: "30m",
                provider = config.provider ?: "local",
                metadata = config.metadata ?: emptyMap(),
            )
            templateStore[template.name] = template
            return template
        }

        override suspend fun delete(name: String) {
            if (templateStore.remove(name) == null) {
                throw ForgevmException("Template '$name' not found", "NOT_FOUND", 404)
            }
        }

        override suspend fun spawn(
            name: String,
            overrides: TemplateSpawnOverrides,
        ): SandboxSession {
            val template = get(name)
            return this@LocalClient.spawn(
                image = template.image,
                provider = overrides.provider ?: template.provider,
                memoryMb = template.memoryMb,
                vcpus = template.vcpus,
                ttl = overrides.ttl ?: template.ttl,
                metadata = template.metadata,
            )
        }
    }
}

internal data class LocalSandboxState(
    val info: SandboxInfo,
    val rootDir: File,
    val expiresAt: Instant?,
)

class LocalSandbox internal constructor(
    private val client: LocalClient,
    private var snapshot: LocalSandboxState,
) : SandboxSession {
    override val id: String
        get() = snapshot.info.id

    override val state: String
        get() = snapshot.info.state

    override val provider: String
        get() = snapshot.info.provider

    override val image: String
        get() = snapshot.info.image

    override val memoryMb: Int
        get() = snapshot.info.memoryMb

    override val vcpus: Int
        get() = snapshot.info.vcpus

    override val info: SandboxInfo
        get() = snapshot.info.copy()

    override suspend fun exec(command: String, options: ExecOptions): ExecResult =
        withContext(Dispatchers.IO) {
            val startedAt = System.nanoTime()
            val process = buildProcess(command, options).start()
            val stdout = StringBuffer()
            val stderr = StringBuffer()
            val stdoutThread = Thread {
                process.inputStream.bufferedReader().useLines { lines ->
                    for (line in lines) {
                        stdout.append(line).append('\n')
                    }
                }
            }
            val stderrThread = Thread {
                process.errorStream.bufferedReader().useLines { lines ->
                    for (line in lines) {
                        stderr.append(line).append('\n')
                    }
                }
            }
            stdoutThread.start()
            stderrThread.start()
            val timeoutMillis = options.timeout?.let(::parseDurationMillis)
            val finished = if (timeoutMillis != null) {
                process.waitFor(timeoutMillis, java.util.concurrent.TimeUnit.MILLISECONDS)
            } else {
                process.waitFor()
                true
            }
            if (!finished) {
                process.destroyForcibly()
                process.waitFor()
                stdoutThread.join()
                stderrThread.join()
                return@withContext ExecResult(
                    exitCode = -1,
                    stdout = stdout.toString(),
                    stderr = stderr.toString() + "process timed out after ${options.timeout}",
                    duration = formatDurationNanos(System.nanoTime() - startedAt),
                )
            }
            stdoutThread.join()
            stderrThread.join()
            ExecResult(
                exitCode = process.exitValue(),
                stdout = stdout.toString(),
                stderr = stderr.toString(),
                duration = formatDurationNanos(System.nanoTime() - startedAt),
            )
        }

    override fun execStream(command: String, options: ExecOptions): Flow<StreamChunk> = channelFlow {
        val process = buildProcess(command, options).start()
        val stdoutThread = Thread {
            process.inputStream.bufferedReader().useLines { lines ->
                for (line in lines) {
                    trySendBlocking(StreamChunk(stream = "stdout", data = "$line\n"))
                }
            }
        }
        val stderrThread = Thread {
            process.errorStream.bufferedReader().useLines { lines ->
                for (line in lines) {
                    trySendBlocking(StreamChunk(stream = "stderr", data = "$line\n"))
                }
            }
        }

        stdoutThread.start()
        stderrThread.start()
        val timeoutMillis = options.timeout?.let(::parseDurationMillis)
        if (timeoutMillis != null) {
            val finished = process.waitFor(timeoutMillis, java.util.concurrent.TimeUnit.MILLISECONDS)
            if (!finished) {
                process.destroyForcibly()
            }
        } else {
            process.waitFor()
        }
        stdoutThread.join()
        stderrThread.join()
        close()
        awaitClose { process.destroy() }
    }

    override suspend fun writeFile(path: String, content: String, mode: String?) = withContext(Dispatchers.IO) {
        val file = resolveSandboxPath(path)
        file.parentFile?.mkdirs()
        file.writeText(content)
        if (mode != null) {
            applyMode(file, mode)
        }
    }

    override suspend fun readFile(path: String): String = withContext(Dispatchers.IO) {
        resolveSandboxPath(path).readText()
    }

    override suspend fun listFiles(path: String): List<FileInfo> = withContext(Dispatchers.IO) {
        val dir = resolveSandboxPath(path)
        val files = dir.listFiles()?.sortedBy { it.name }.orEmpty()
        files.map { file ->
            FileInfo(
                name = file.name,
                path = toSandboxPath(file),
                size = file.length(),
                isDir = file.isDirectory,
                modTime = Instant.ofEpochMilli(file.lastModified()).toString(),
                mode = fileMode(file),
            )
        }
    }

    override suspend fun extendTtl(ttl: String) {
        val expiresAt = Instant.now().plusMillis(parseDurationMillis(ttl))
        snapshot = snapshot.copy(
            info = snapshot.info.copy(expiresAt = expiresAt.toString()),
            expiresAt = expiresAt,
        )
        client.updateSandbox(snapshot)
    }

    override suspend fun destroy() {
        client.destroySandbox(id)
        snapshot = snapshot.copy(info = snapshot.info.copy(state = "destroyed"))
    }

    override suspend fun refresh(): SandboxSession {
        snapshot = client.requireSandbox(id)
        return this
    }

    private fun buildProcess(command: String, options: ExecOptions): ProcessBuilder {
        refreshIfNeeded()
        val pb = ProcessBuilder(configuredCommand(command, options.args.orEmpty()))
        pb.directory(resolveWorkdir(options.workdir))
        val env = pb.environment()
        env.putAll(options.env.orEmpty())
        return pb
    }

    private fun configuredCommand(command: String, args: List<String>): List<String> {
        val full = if (args.isEmpty()) {
            command
        } else {
            buildString {
                append(command)
                for (arg in args) {
                    append(' ')
                    append(shellEscape(arg))
                }
            }
        }
        return listOf(clientConfig().shellExecutable, clientConfig().shellArgument, full)
    }

    private fun resolveWorkdir(workdir: String?): File =
        if (workdir.isNullOrBlank()) snapshot.rootDir else resolveSandboxPath(workdir)

    private fun resolveSandboxPath(path: String): File {
        refreshIfNeeded()
        val relative = path.removePrefix("/")
        val candidate = File(snapshot.rootDir, relative).canonicalFile
        val root = snapshot.rootDir.canonicalFile
        if (candidate.path != root.path && !candidate.path.startsWith(root.path + File.separator)) {
            throw ForgevmException("Path escapes sandbox root: $path", "INVALID_PATH", 400)
        }
        return candidate
    }

    private fun toSandboxPath(file: File): String {
        val root = snapshot.rootDir.canonicalFile.toPath()
        val current = file.canonicalFile.toPath()
        val relative = root.relativize(current).toString().replace(File.separatorChar, '/')
        return if (relative.isBlank()) "/" else "/$relative"
    }

    private fun refreshIfNeeded() {
        snapshot = client.requireSandbox(id)
    }

    private fun clientConfig(): LocalClientConfig = client.config
}
