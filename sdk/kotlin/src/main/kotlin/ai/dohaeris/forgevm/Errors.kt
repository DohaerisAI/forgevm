package ai.dohaeris.forgevm

open class ForgevmException(
    override val message: String,
    val code: String = "",
    val statusCode: Int? = null,
) : RuntimeException(message)

class SandboxNotFoundException(
    val sandboxId: String,
    message: String = "Sandbox '$sandboxId' not found",
) : ForgevmException(message, code = "NOT_FOUND", statusCode = 404)

class ConnectionException(message: String) :
    ForgevmException(message, code = "CONNECTION_ERROR")

class ProviderException(
    message: String,
    code: String = "PROVIDER_ERROR",
    statusCode: Int? = null,
) : ForgevmException(message, code, statusCode)
