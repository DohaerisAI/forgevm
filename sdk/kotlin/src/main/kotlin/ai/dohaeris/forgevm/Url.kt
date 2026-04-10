package ai.dohaeris.forgevm

import java.net.URLEncoder
import java.nio.charset.StandardCharsets

internal fun String.urlEncode(): String =
    URLEncoder.encode(this, StandardCharsets.UTF_8.toString())
