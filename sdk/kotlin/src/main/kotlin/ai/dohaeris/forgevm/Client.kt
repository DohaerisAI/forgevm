package ai.dohaeris.forgevm

import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import kotlinx.serialization.json.putJsonObject
import okhttp3.OkHttpClient

class Client @JvmOverloads constructor(
    baseUrl: String = DEFAULT_BASE_URL,
    apiKey: String? = null,
    timeoutMillis: Long = DEFAULT_TIMEOUT_MS,
    okHttpClient: OkHttpClient? = null,
) : ForgevmClient {
    private val api = ApiClient(baseUrl, apiKey, timeoutMillis, okHttpClient)

    override val templates: TemplateManager = TemplateManager(api)

    override suspend fun spawn(
        image: String,
        provider: String?,
        memoryMb: Int?,
        vcpus: Int?,
        ttl: String?,
        metadata: Map<String, String>?,
    ): SandboxSession {
        val body = buildJsonObject {
            put("image", image)
            provider?.let { put("provider", it) }
            memoryMb?.let { put("memory_mb", it) }
            vcpus?.let { put("vcpus", it) }
            ttl?.let { put("ttl", it) }
            metadata?.let { metadataMap ->
                putJsonObject("metadata") {
                    for ((key, value) in metadataMap) {
                        put(key, value)
                    }
                }
            }
        }
        val info: SandboxInfo = api.post("/api/v1/sandboxes", api.toJson(body))
        return Sandbox(api, info)
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

    override suspend fun get(sandboxId: String): SandboxSession {
        val info: SandboxInfo = api.get("/api/v1/sandboxes/${sandboxId.urlEncode()}", sandboxId)
        return Sandbox(api, info)
    }

    override suspend fun list(): List<SandboxInfo> =
        api.get("/api/v1/sandboxes")

    override suspend fun prune(): Int {
        val result: PruneResponse = api.delete("/api/v1/sandboxes")
        return result.pruned
    }

    override suspend fun health(): HealthInfo =
        api.get("/api/v1/health")

    override suspend fun providers(): List<ProviderInfo> =
        api.get("/api/v1/providers")

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

    companion object {
        const val DEFAULT_BASE_URL: String = "http://localhost:7423"
        const val DEFAULT_TIMEOUT_MS: Long = 30_000

        @JvmStatic
        @JvmOverloads
        fun fromHost(
            host: String = "localhost",
            port: Int = 7423,
            apiKey: String? = null,
            timeoutMillis: Long = DEFAULT_TIMEOUT_MS,
            okHttpClient: OkHttpClient? = null,
        ): Client = Client(
            baseUrl = "http://$host:$port",
            apiKey = apiKey,
            timeoutMillis = timeoutMillis,
            okHttpClient = okHttpClient,
        )

        @JvmStatic
        fun local(config: LocalClientConfig): LocalClient = LocalClient(config)
    }
}

@kotlinx.serialization.Serializable
private data class PruneResponse(val pruned: Int)
