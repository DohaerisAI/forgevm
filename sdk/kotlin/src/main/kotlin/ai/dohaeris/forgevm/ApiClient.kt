package ai.dohaeris.forgevm

import java.io.IOException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.Json
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.Response
import okhttp3.ResponseBody

internal class ApiClient(
    baseUrl: String,
    apiKey: String?,
    timeoutMillis: Long,
    okHttpClient: OkHttpClient?,
) {
    @PublishedApi
    internal val json = Json {
        ignoreUnknownKeys = true
        explicitNulls = false
    }

    @PublishedApi
    internal val normalizedBaseUrl = baseUrl.trimEnd('/')

    @PublishedApi
    internal val defaultHeaders: Map<String, String> =
        buildMap {
            if (!apiKey.isNullOrBlank()) {
                put("X-API-Key", apiKey)
            }
        }

    @PublishedApi
    internal val client = okHttpClient ?: OkHttpClient.Builder()
        .callTimeout(timeoutMillis, java.util.concurrent.TimeUnit.MILLISECONDS)
        .build()

    internal suspend inline fun <reified T> get(
        path: String,
        sandboxId: String? = null,
    ): T = executeAndDecode(newRequest(path).get().build(), sandboxId)

    internal suspend inline fun <reified T> post(
        path: String,
        body: String,
        sandboxId: String? = null,
    ): T = executeAndDecode(newRequest(path).post(jsonBody(body)).build(), sandboxId)

    suspend fun postUnit(path: String, body: String, sandboxId: String? = null) {
        execute(newRequest(path).post(jsonBody(body)).build()).use { response ->
            ensureSuccess(response, sandboxId)
        }
    }

    suspend fun deleteUnit(path: String, sandboxId: String? = null) {
        execute(newRequest(path).delete().build()).use { response ->
            ensureSuccess(response, sandboxId)
        }
    }

    internal suspend inline fun <reified T> delete(
        path: String,
        sandboxId: String? = null,
    ): T = executeAndDecode(newRequest(path).delete().build(), sandboxId)

    suspend fun execute(request: Request): Response = withContext(Dispatchers.IO) {
        try {
            client.newCall(request).execute()
        } catch (e: IOException) {
            throw ConnectionException(
                "Cannot connect to ForgeVM server at $normalizedBaseUrl: ${e.message ?: "network error"}",
            )
        }
    }

    internal suspend inline fun <reified T> executeAndDecode(
        request: Request,
        sandboxId: String? = null,
    ): T {
        return execute(request).use { response ->
            ensureSuccess(response, sandboxId)
            decodeResponse(response.body, response.request.url.toString())
        }
    }

    fun newRequest(path: String): Request.Builder {
        val builder = Request.Builder().url("$normalizedBaseUrl$path")
        for ((key, value) in defaultHeaders) {
            builder.header(key, value)
        }
        return builder
    }

    fun jsonBody(body: String): RequestBody =
        body.toRequestBody("application/json; charset=utf-8".toMediaType())

    suspend fun ensureSuccess(response: Response, sandboxId: String? = null) {
        if (response.isSuccessful) {
            return
        }

        val rawBody = response.body?.string().orEmpty()
        val parsed = rawBody
            .takeIf { it.isNotBlank() }
            ?.let { runCatching { json.decodeFromString<ApiErrorBody>(it) }.getOrNull() }

        val message = parsed?.message?.ifBlank { null }
            ?: rawBody.ifBlank { response.message.ifBlank { "request failed" } }
        val code = parsed?.code.orEmpty()

        when {
            response.code == 404 -> throw SandboxNotFoundException(
                sandboxId ?: message,
                message,
            )
            response.code == 401 -> throw ForgevmException(message, "UNAUTHORIZED", 401)
            response.code >= 500 -> throw ProviderException(message, code.ifBlank { "PROVIDER_ERROR" }, response.code)
            else -> throw ForgevmException(message, code, response.code)
        }
    }

    internal inline fun <reified T> decodeResponse(body: ResponseBody?, location: String): T {
        val raw = body?.string().orEmpty()
        if (raw.isBlank()) {
            throw ForgevmException("Empty response body from $location")
        }
        return json.decodeFromString(raw)
    }

    internal inline fun <reified T> decodeString(raw: String): T =
        json.decodeFromString(raw)

    fun toJson(value: kotlinx.serialization.json.JsonElement): String = json.encodeToString(
        kotlinx.serialization.json.JsonElement.serializer(),
        value,
    )
}
