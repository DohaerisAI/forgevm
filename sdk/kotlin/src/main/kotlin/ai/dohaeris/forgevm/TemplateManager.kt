package ai.dohaeris.forgevm

import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import kotlinx.serialization.json.putJsonObject

class TemplateManager internal constructor(
    private val api: ApiClient,
) : TemplateOperations {
    override suspend fun list(): List<Template> =
        api.get("/api/v1/templates")

    override suspend fun get(name: String): Template =
        api.get("/api/v1/templates/${name.urlEncode()}")

    override suspend fun save(config: TemplateConfig): Template {
        val body = buildJsonObject {
            put("name", config.name)
            put("image", config.image)
            config.memoryMb?.let { put("memory_mb", it) }
            config.vcpus?.let { put("vcpus", it) }
            config.ttl?.let { put("ttl", it) }
            config.provider?.let { put("provider", it) }
            config.metadata?.let { metadata ->
                putJsonObject("metadata") {
                    for ((key, value) in metadata) {
                        put(key, value)
                    }
                }
            }
        }
        return api.post("/api/v1/templates", api.toJson(body))
    }

    override suspend fun delete(name: String) {
        api.deleteUnit("/api/v1/templates/${name.urlEncode()}")
    }

    override suspend fun spawn(
        name: String,
        overrides: TemplateSpawnOverrides,
    ): Sandbox {
        val body = buildJsonObject {
            overrides.provider?.let { put("provider", it) }
            overrides.ttl?.let { put("ttl", it) }
        }
        val info: SandboxInfo = api.post(
            "/api/v1/templates/${name.urlEncode()}/spawn",
            api.toJson(body),
        )
        return Sandbox(api, info)
    }
}
