/**
 * Error classes for the ForgeVM SDK.
 *
 * All SDK errors extend {@link ForgevmError} so callers can catch the base
 * class for generic handling or catch specific subclasses for fine-grained
 * control.
 *
 * @module errors
 */

import type { ApiErrorBody } from "./types.js";

// ---------------------------------------------------------------------------
// Base error
// ---------------------------------------------------------------------------

/**
 * Base error class for all ForgeVM SDK errors.
 *
 * @example
 * ```ts
 * try {
 *   await client.spawn();
 * } catch (err) {
 *   if (err instanceof ForgevmError) {
 *     console.error(`ForgeVM error [${err.code}]: ${err.message}`);
 *   }
 * }
 * ```
 */
export class ForgevmError extends Error {
  /** Machine-readable error code (e.g. `"NOT_FOUND"`, `"INTERNAL"`). */
  public readonly code: string;

  /** HTTP status code from the server response, if available. */
  public readonly statusCode: number | undefined;

  constructor(message: string, code?: string, statusCode?: number) {
    super(message);
    this.name = "ForgevmError";
    this.code = code ?? "";
    this.statusCode = statusCode;

    // Maintain proper prototype chain for instanceof checks.
    Object.setPrototypeOf(this, new.target.prototype);
  }
}

// ---------------------------------------------------------------------------
// Specific error types
// ---------------------------------------------------------------------------

/**
 * Raised when a sandbox cannot be found (HTTP 404).
 *
 * @example
 * ```ts
 * try {
 *   await client.get("sb-nonexistent");
 * } catch (err) {
 *   if (err instanceof SandboxNotFoundError) {
 *     console.log(`Sandbox ${err.sandboxId} does not exist`);
 *   }
 * }
 * ```
 */
export class SandboxNotFoundError extends ForgevmError {
  /** The sandbox ID that was not found. */
  public readonly sandboxId: string;

  constructor(sandboxId: string, message?: string) {
    super(
      message ?? `Sandbox '${sandboxId}' not found`,
      "NOT_FOUND",
      404,
    );
    this.name = "SandboxNotFoundError";
    this.sandboxId = sandboxId;
    Object.setPrototypeOf(this, new.target.prototype);
  }
}

/**
 * Raised when a provider operation fails (HTTP 5xx from the API).
 */
export class ProviderError extends ForgevmError {
  constructor(message: string, code?: string, statusCode?: number) {
    super(message, code ?? "PROVIDER_ERROR", statusCode);
    this.name = "ProviderError";
    Object.setPrototypeOf(this, new.target.prototype);
  }
}

/**
 * Raised when the SDK cannot connect to the ForgeVM server at all
 * (network error, DNS failure, connection refused, etc.).
 */
export class ConnectionError extends ForgevmError {
  constructor(message: string) {
    super(message, "CONNECTION_ERROR");
    this.name = "ConnectionError";
    Object.setPrototypeOf(this, new.target.prototype);
  }
}

// ---------------------------------------------------------------------------
// Response handler
// ---------------------------------------------------------------------------

/**
 * Inspect an HTTP {@link Response} and throw a typed error if the request
 * was not successful.
 *
 * The function attempts to parse the response body as JSON to extract the
 * server's `code` and `message` fields.  If parsing fails it falls back to
 * the raw response text.
 *
 * @param response - The {@link Response} object from `fetch`.
 * @param sandboxId - Optional sandbox ID for better 404 error messages.
 *
 * @throws {@link SandboxNotFoundError} on 404
 * @throws {@link ForgevmError} with code `"UNAUTHORIZED"` on 401
 * @throws {@link ProviderError} on 5xx
 * @throws {@link ForgevmError} on any other non-success status
 */
export async function handleResponse(
  response: Response,
  sandboxId?: string,
): Promise<void> {
  if (response.ok) {
    return;
  }

  let code = "";
  let message = "";

  try {
    const body = (await response.json()) as Partial<ApiErrorBody>;
    code = body.code ?? "";
    message = body.message ?? response.statusText;
  } catch {
    // Response body is not JSON -- use status text as the message.
    message = (await response.text().catch(() => "")) || response.statusText;
  }

  if (response.status === 404) {
    throw new SandboxNotFoundError(sandboxId ?? message, message);
  }

  if (response.status === 401) {
    throw new ForgevmError(message, "UNAUTHORIZED", 401);
  }

  if (response.status >= 500) {
    throw new ProviderError(message, code, response.status);
  }

  throw new ForgevmError(message, code, response.status);
}
