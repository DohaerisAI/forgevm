package ai.dohaeris.forgevm

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.flow.flowOn
import kotlinx.serialization.json.add
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import kotlinx.serialization.json.putJsonArray
import kotlinx.serialization.json.putJsonObject
import okhttp3.Request

class Sandbox internal constructor(
    private val api: ApiClient,
    private var infoSnapshot: SandboxInfo,
) : SandboxSession {
    override val id: String
        get() = infoSnapshot.id

    override val state: String
        get() = infoSnapshot.state

    override val provider: String
        get() = infoSnapshot.provider

    override val image: String
        get() = infoSnapshot.image

    override val memoryMb: Int
        get() = infoSnapshot.memoryMb

    override val vcpus: Int
        get() = infoSnapshot.vcpus

    override val info: SandboxInfo
        get() = infoSnapshot.copy()

    override suspend fun exec(command: String, options: ExecOptions): ExecResult {
        val body = buildJsonObject {
            put("command", command)
            options.args?.let { args ->
                putJsonArray("args") {
                    args.forEach { add(it) }
                }
            }
            options.env?.let { env ->
                putJsonObject("env") {
                    for ((key, value) in env) {
                        put(key, value)
                    }
                }
            }
            options.workdir?.let { put("workdir", it) }
            options.timeout?.let { put("timeout", it) }
        }
        return api.post("/api/v1/sandboxes/${id.urlEncode()}/exec", api.toJson(body), id)
    }

    override fun execStream(
        command: String,
        options: ExecOptions,
    ): Flow<StreamChunk> = flow<StreamChunk> {
        val body = buildJsonObject {
            put("command", command)
            put("stream", true)
            options.args?.let { args ->
                putJsonArray("args") {
                    args.forEach { add(it) }
                }
            }
            options.env?.let { env ->
                putJsonObject("env") {
                    for ((key, value) in env) {
                        put(key, value)
                    }
                }
            }
            options.workdir?.let { put("workdir", it) }
        }

        val request: Request = api.newRequest("/api/v1/sandboxes/${id.urlEncode()}/exec")
            .post(api.jsonBody(api.toJson(body)))
            .build()

        api.execute(request).use { response ->
            api.ensureSuccess(response, id)
            val source = response.body?.source() ?: return@use
            while (!source.exhausted()) {
                val line = source.readUtf8Line() ?: continue
                if (line.isBlank()) {
                    continue
                }
                emit(api.decodeString<StreamChunk>(line))
            }
        }
    }.flowOn(Dispatchers.IO)

    override suspend fun writeFile(path: String, content: String, mode: String?) {
        val body = buildJsonObject {
            put("path", path)
            put("content", content)
            mode?.let { put("mode", it) }
        }
        api.postUnit("/api/v1/sandboxes/${id.urlEncode()}/files", api.toJson(body), id)
    }

    override suspend fun readFile(path: String): String {
        val request = api.newRequest(
            "/api/v1/sandboxes/${id.urlEncode()}/files?path=${path.urlEncode()}",
        ).get().build()
        return api.execute(request).use { response ->
            api.ensureSuccess(response, id)
            response.body?.string().orEmpty()
        }
    }

    override suspend fun listFiles(path: String): List<FileInfo> =
        api.get("/api/v1/sandboxes/${id.urlEncode()}/files/list?path=${path.urlEncode()}", id)

    override suspend fun extendTtl(ttl: String) {
        val body = buildJsonObject {
            put("ttl", ttl)
        }
        infoSnapshot = api.post("/api/v1/sandboxes/${id.urlEncode()}/extend", api.toJson(body), id)
    }

    override suspend fun destroy() {
        api.deleteUnit("/api/v1/sandboxes/${id.urlEncode()}", id)
        infoSnapshot = infoSnapshot.copy(state = "destroyed")
    }

    override suspend fun refresh(): Sandbox {
        infoSnapshot = api.get("/api/v1/sandboxes/${id.urlEncode()}", id)
        return this
    }

    override fun toString(): String = "Sandbox(id=\"$id\", state=\"$state\")"
}
