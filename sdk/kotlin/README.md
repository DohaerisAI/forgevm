# ForgeVM Kotlin SDK

Kotlin client for [ForgeVM](https://github.com/DohaerisAI/forgevm), intended for JVM and Android apps.

This SDK uses:

- `OkHttp` for HTTP
- `kotlinx.serialization` for JSON
- Kotlin coroutines and `Flow` for async APIs and streaming output

## Status

This is a standalone Gradle module inside the repo. It is designed to be Android-friendly, but it is not an Android Gradle plugin module by itself.

## Build

```bash
cd sdk/kotlin
gradle build
```

## Usage

```kotlin
import ai.dohaeris.forgevm.Client
import ai.dohaeris.forgevm.ExecOptions
import kotlinx.coroutines.runBlocking

fun main() = runBlocking {
    val client = Client("http://localhost:7423")

    val sandbox = client.spawn(image = "alpine:latest")
    val result = sandbox.exec("echo hello from Kotlin")
    println(result.stdout)

    sandbox.writeFile("/app/hello.sh", "echo hello from file\n")
    sandbox.exec("sh /app/hello.sh")

    sandbox.extendTtl("30m")

    sandbox.execStream("ping -c 2 localhost").collect { chunk ->
        print(chunk.data)
    }

    sandbox.destroy()
}
```

## Local On-Device Usage

```kotlin
import ai.dohaeris.forgevm.Client
import ai.dohaeris.forgevm.LocalClientConfig
import kotlinx.coroutines.runBlocking
import java.io.File

fun main() = runBlocking {
    val client = Client.local(
        LocalClientConfig(
            rootDir = File("/data/user/0/your.app/files/forgevm"),
            shellExecutable = "/system/bin/sh",
        ),
    )

    val sandbox = client.spawn(image = "local-shell")
    sandbox.writeFile("/tool.sh", "echo hello from device\n", "0700")
    val result = sandbox.exec("sh", ai.dohaeris.forgevm.ExecOptions(args = listOf("/tool.sh")))
    println(result.stdout)
}
```

This local mode is the Android-facing path if your app needs ForgeVM-style tooling without a remote server. It is process/filesystem isolation, not Firecracker on Android.

## Android

Call the SDK from a background coroutine, for example from `Dispatchers.IO` or a ViewModel scope. The library performs blocking HTTP work off the main thread internally, but app code should still treat sandbox operations as network calls.
