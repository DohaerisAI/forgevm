package ai.dohaeris.forgevm

import kotlinx.coroutines.flow.Flow

interface SandboxSession {
    val id: String
    val state: String
    val provider: String
    val image: String
    val memoryMb: Int
    val vcpus: Int
    val info: SandboxInfo

    suspend fun exec(command: String, options: ExecOptions = ExecOptions()): ExecResult
    fun execStream(command: String, options: ExecOptions = ExecOptions()): Flow<StreamChunk>
    suspend fun writeFile(path: String, content: String, mode: String? = null)
    suspend fun readFile(path: String): String
    suspend fun listFiles(path: String = "/"): List<FileInfo>
    suspend fun extendTtl(ttl: String = "30m")
    suspend fun destroy()
    suspend fun refresh(): SandboxSession
}

interface TemplateOperations {
    suspend fun list(): List<Template>
    suspend fun get(name: String): Template
    suspend fun save(config: TemplateConfig): Template
    suspend fun delete(name: String)
    suspend fun spawn(
        name: String,
        overrides: TemplateSpawnOverrides = TemplateSpawnOverrides(),
    ): SandboxSession
}

interface ForgevmClient {
    val templates: TemplateOperations

    suspend fun spawn(
        image: String = "alpine:latest",
        provider: String? = null,
        memoryMb: Int? = null,
        vcpus: Int? = null,
        ttl: String? = null,
        metadata: Map<String, String>? = null,
    ): SandboxSession

    suspend fun spawn(options: SpawnOptions): SandboxSession
    suspend fun get(sandboxId: String): SandboxSession
    suspend fun list(): List<SandboxInfo>
    suspend fun prune(): Int
    suspend fun health(): HealthInfo
    suspend fun providers(): List<ProviderInfo>

    suspend fun <T> withSandbox(
        options: SpawnOptions = SpawnOptions(),
        block: suspend (SandboxSession) -> T,
    ): T
}
