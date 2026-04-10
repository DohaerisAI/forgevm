package ai.dohaeris.forgevm

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

@Serializable
data class SandboxInfo(
    val id: String,
    val state: String,
    val provider: String,
    val image: String,
    @SerialName("memory_mb") val memoryMb: Int = 512,
    val vcpus: Int = 1,
    @SerialName("created_at") val createdAt: String = "",
    @SerialName("expires_at") val expiresAt: String = "",
    val metadata: Map<String, String> = emptyMap(),
)

@Serializable
data class SpawnOptions(
    val image: String = "alpine:latest",
    val provider: String? = null,
    @SerialName("memory_mb") val memoryMb: Int? = null,
    val vcpus: Int? = null,
    val ttl: String? = null,
    val metadata: Map<String, String>? = null,
)

@Serializable
data class ExecOptions(
    val args: List<String>? = null,
    val env: Map<String, String>? = null,
    val workdir: String? = null,
    val timeout: String? = null,
)

@Serializable
data class ExecResult(
    @SerialName("exit_code") val exitCode: Int,
    val stdout: String,
    val stderr: String,
    val duration: String = "",
)

@Serializable
data class StreamChunk(
    val stream: String,
    val data: String,
)

@Serializable
data class FileInfo(
    val name: String,
    val path: String,
    val size: Long,
    @SerialName("is_dir") val isDir: Boolean,
    @SerialName("mod_time") val modTime: String,
    val mode: String,
)

@Serializable
data class Template(
    val name: String,
    val image: String,
    @SerialName("memory_mb") val memoryMb: Int = 512,
    val vcpus: Int = 1,
    val ttl: String = "30m",
    val provider: String = "",
    val metadata: Map<String, String> = emptyMap(),
)

@Serializable
data class TemplateConfig(
    val name: String,
    val image: String,
    @SerialName("memory_mb") val memoryMb: Int? = null,
    val vcpus: Int? = null,
    val ttl: String? = null,
    val provider: String? = null,
    val metadata: Map<String, String>? = null,
)

@Serializable
data class TemplateSpawnOverrides(
    val provider: String? = null,
    val ttl: String? = null,
)

@Serializable
data class ProviderInfo(
    val name: String,
    val healthy: Boolean,
    val default: Boolean,
)

@Serializable
data class HealthInfo(
    val status: String,
    val version: String,
    val uptime: String,
)

@Serializable
internal data class ApiErrorBody(
    val code: String? = null,
    val message: String? = null,
)
