package ai.dohaeris.forgevm

import java.io.File
import java.util.concurrent.TimeUnit

internal fun parseDurationMillis(value: String): Long {
    val regex = Regex("(\\d+)(ms|s|m|h)")
    val matches = regex.findAll(value.trim()).toList()
    if (matches.isEmpty()) {
        throw ForgevmException("Unsupported duration: $value", "INVALID_DURATION", 400)
    }
    val consumed = matches.joinToString(separator = "") { it.value }
    if (consumed != value.trim()) {
        throw ForgevmException("Unsupported duration: $value", "INVALID_DURATION", 400)
    }
    var total = 0L
    for (match in matches) {
        val amount = match.groupValues[1].toLong()
        total += when (match.groupValues[2]) {
            "ms" -> amount
            "s" -> TimeUnit.SECONDS.toMillis(amount)
            "m" -> TimeUnit.MINUTES.toMillis(amount)
            "h" -> TimeUnit.HOURS.toMillis(amount)
            else -> error("unreachable")
        }
    }
    return total
}

internal fun formatDurationNanos(nanos: Long): String {
    val millis = TimeUnit.NANOSECONDS.toMillis(nanos)
    return "${millis}ms"
}

internal fun shellEscape(value: String): String =
    "'" + value.replace("'", "'\"'\"'") + "'"

internal fun applyMode(file: File, mode: String) {
    if (mode.length != 4) {
        return
    }
    val owner = mode[1].digitToIntOrNull() ?: return
    file.setReadable(owner and 4 != 0, true)
    file.setWritable(owner and 2 != 0, true)
    file.setExecutable(owner and 1 != 0, true)
}

internal fun fileMode(file: File): String {
    val owner =
        (if (file.canRead()) 4 else 0) +
            (if (file.canWrite()) 2 else 0) +
            (if (file.canExecute()) 1 else 0)
    return "0${owner}${owner}${owner}"
}
