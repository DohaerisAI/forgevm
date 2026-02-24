/**
 * NDJSON (newline-delimited JSON) streaming utilities.
 *
 * Used internally by {@link Sandbox.execStream} to parse chunked exec output
 * from the ForgeVM API.
 *
 * @module streaming
 */

import type { StreamChunk } from "./types.js";

/**
 * Parse an NDJSON response body into an async iterable of {@link StreamChunk}
 * objects.
 *
 * Each line of the response is expected to be a valid JSON object with
 * `stream` (`"stdout"` or `"stderr"`) and `data` fields.  Blank lines and
 * lines that fail JSON parsing are silently skipped to match the resilient
 * behaviour of the Python SDK.
 *
 * @param body - A {@link ReadableStream} of `Uint8Array` chunks (typically
 *   `response.body` from a `fetch` call).
 *
 * @yields {StreamChunk} Parsed output chunks in the order they arrive.
 *
 * @example
 * ```ts
 * const response = await fetch(url);
 * for await (const chunk of parseNDJSON(response.body!)) {
 *   process.stdout.write(chunk.data);
 * }
 * ```
 */
export async function* parseNDJSON(
  body: ReadableStream<Uint8Array>,
): AsyncGenerator<StreamChunk, void, undefined> {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  try {
    for (;;) {
      const { done, value } = await reader.read();

      if (value !== undefined) {
        buffer += decoder.decode(value, { stream: true });
      }

      // Process all complete lines currently in the buffer.
      let newlineIndex: number;
      while ((newlineIndex = buffer.indexOf("\n")) !== -1) {
        const line = buffer.slice(0, newlineIndex).trim();
        buffer = buffer.slice(newlineIndex + 1);

        if (line.length === 0) {
          continue;
        }

        try {
          const obj = JSON.parse(line) as Record<string, unknown>;
          yield {
            stream: (obj["stream"] as StreamChunk["stream"]) ?? "stdout",
            data: (obj["data"] as string) ?? "",
          };
        } catch {
          // Malformed JSON line -- skip silently.
        }
      }

      if (done) {
        break;
      }
    }

    // Process any remaining data after the stream ends (no trailing newline).
    const remaining = buffer.trim();
    if (remaining.length > 0) {
      try {
        const obj = JSON.parse(remaining) as Record<string, unknown>;
        yield {
          stream: (obj["stream"] as StreamChunk["stream"]) ?? "stdout",
          data: (obj["data"] as string) ?? "",
        };
      } catch {
        // Ignore trailing non-JSON data.
      }
    }
  } finally {
    reader.releaseLock();
  }
}
