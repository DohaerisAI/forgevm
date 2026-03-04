/**
 * ForgeVM Client -- main entry point for the TypeScript SDK.
 *
 * @module client
 */

import {
  ConnectionError,
  handleResponse,
} from "./errors.js";
import { Sandbox } from "./sandbox.js";
import { TemplateManager } from "./templates.js";
import type {
  ForgevmClientOptions,
  HealthInfo,
  ProviderInfo,
  SandboxInfo,
  SpawnOptions,
  VMPoolStatus,
} from "./types.js";

/**
 * Default port for the ForgeVM API server.
 */
const DEFAULT_PORT = 7423;

/**
 * Default request timeout in milliseconds (30 seconds).
 */
const DEFAULT_TIMEOUT = 30_000;

/**
 * Client for the ForgeVM API.
 *
 * This is the primary entry point for interacting with a ForgeVM server.
 * It provides methods to spawn, list, and manage sandboxes, as well as
 * access to template and provider management.
 *
 * @example
 * ```ts
 * import { Client } from "forgevm";
 *
 * const client = new Client({ host: "localhost", port: 7423 });
 *
 * // Spawn a sandbox, run a command, and clean up
 * const result = await client.withSandbox(
 *   { image: "alpine:latest" },
 *   async (sandbox) => {
 *     const res = await sandbox.exec("echo hello");
 *     return res.stdout;
 *   },
 * );
 * console.log(result); // "hello\n"
 * ```
 *
 * @example
 * ```ts
 * // Manual lifecycle management
 * const sandbox = await client.spawn({ image: "python:3.12-slim" });
 * try {
 *   await sandbox.writeFile("/app/main.py", 'print("Hello from ForgeVM!")');
 *   const result = await sandbox.exec("python /app/main.py");
 *   console.log(result.stdout);
 * } finally {
 *   await sandbox.destroy();
 * }
 * ```
 */
export class Client {
  /** Resolved base URL (e.g. `"http://localhost:7423"`). */
  private readonly _baseUrl: string;
  /** Default headers sent with every request. */
  private readonly _headers: Record<string, string>;
  /** Request timeout in milliseconds. */
  private readonly _timeout: number;

  /**
   * Template management interface.
   *
   * Provides methods to list, create, delete, and spawn from templates.
   *
   * @example
   * ```ts
   * const templates = await client.templates.list();
   * const sandbox = await client.templates.spawn("my-template");
   * ```
   */
  public readonly templates: TemplateManager;

  /**
   * Create a new ForgeVM client.
   *
   * @param options - Connection configuration.  All fields are optional
   *   and have sensible defaults for local development.
   *
   * @example
   * ```ts
   * // Connect to local server on default port
   * const client = new Client();
   *
   * // Connect with explicit options
   * const client = new Client({
   *   host: "forgevm.internal",
   *   port: 7423,
   *   apiKey: "secret-key",
   *   timeout: 60_000,
   * });
   *
   * // Connect with a full base URL
   * const client = new Client({
   *   baseUrl: "https://forgevm.example.com",
   * });
   * ```
   */
  constructor(options?: ForgevmClientOptions) {
    const opts = options ?? {};

    if (opts.baseUrl) {
      // Strip trailing slash for consistent URL construction.
      this._baseUrl = opts.baseUrl.replace(/\/+$/, "");
    } else {
      const host = opts.host ?? "localhost";
      const port = opts.port ?? DEFAULT_PORT;
      this._baseUrl = `http://${host}:${port}`;
    }

    this._timeout = opts.timeout ?? DEFAULT_TIMEOUT;

    this._headers = {};
    if (opts.apiKey) {
      this._headers["X-API-Key"] = opts.apiKey;
    }
    if (opts.userId) {
      this._headers["X-User-ID"] = opts.userId;
    }

    this.templates = new TemplateManager(
      this._baseUrl,
      this._headers,
      this._timeout,
    );
  }

  // -----------------------------------------------------------------------
  // Sandbox management
  // -----------------------------------------------------------------------

  /**
   * Spawn a new sandbox.
   *
   * @param opts - Sandbox configuration.  All fields are optional; the
   *   server applies defaults (e.g. `image: "alpine:latest"`).
   * @returns A {@link Sandbox} instance ready for use.
   *
   * @throws {@link ConnectionError} if the server is unreachable.
   * @throws {@link ProviderError} if the provider fails to create the VM.
   *
   * @example
   * ```ts
   * const sandbox = await client.spawn({
   *   image: "ubuntu:22.04",
   *   memory_mb: 1024,
   *   vcpus: 2,
   *   ttl: "1h",
   * });
   * ```
   */
  async spawn(opts?: SpawnOptions): Promise<Sandbox> {
    const body: Record<string, unknown> = {
      image: opts?.image ?? "alpine:latest",
    };
    if (opts?.provider) body["provider"] = opts.provider;
    if (opts?.memory_mb !== undefined) body["memory_mb"] = opts.memory_mb;
    if (opts?.vcpus !== undefined) body["vcpus"] = opts.vcpus;
    if (opts?.ttl) body["ttl"] = opts.ttl;
    if (opts?.metadata) body["metadata"] = opts.metadata;

    const response = await this._fetch("/api/v1/sandboxes", {
      method: "POST",
      body: JSON.stringify(body),
    });

    await handleResponse(response);
    const data = (await response.json()) as SandboxInfo;

    return new Sandbox(this._baseUrl, this._headers, this._timeout, data);
  }

  /**
   * Retrieve an existing sandbox by its ID.
   *
   * @param sandboxId - The sandbox identifier (e.g. `"sb-a1b2c3d4"`).
   * @returns A {@link Sandbox} instance.
   *
   * @throws {@link SandboxNotFoundError} if no sandbox with the given ID
   *   exists.
   *
   * @example
   * ```ts
   * const sandbox = await client.get("sb-a1b2c3d4");
   * console.log(sandbox.state); // "running"
   * ```
   */
  async get(sandboxId: string): Promise<Sandbox> {
    const response = await this._fetch(
      `/api/v1/sandboxes/${encodeURIComponent(sandboxId)}`,
      { method: "GET" },
    );

    await handleResponse(response, sandboxId);
    const data = (await response.json()) as SandboxInfo;

    return new Sandbox(this._baseUrl, this._headers, this._timeout, data);
  }

  /**
   * List all active sandboxes on the server.
   *
   * @returns An array of {@link SandboxInfo} objects.
   *
   * @example
   * ```ts
   * const sandboxes = await client.list();
   * for (const sb of sandboxes) {
   *   console.log(`${sb.id}  ${sb.state}  ${sb.image}`);
   * }
   * ```
   */
  async list(): Promise<SandboxInfo[]> {
    const response = await this._fetch("/api/v1/sandboxes", {
      method: "GET",
    });

    await handleResponse(response);
    return (await response.json()) as SandboxInfo[];
  }

  /**
   * Prune all expired sandboxes.
   *
   * @returns The number of sandboxes that were pruned.
   *
   * @example
   * ```ts
   * const pruned = await client.prune();
   * console.log(`Pruned ${pruned} expired sandboxes`);
   * ```
   */
  async prune(): Promise<number> {
    const response = await this._fetch("/api/v1/sandboxes", {
      method: "DELETE",
    });

    await handleResponse(response);
    const data = (await response.json()) as { pruned: number };
    return data.pruned;
  }

  // -----------------------------------------------------------------------
  // System endpoints
  // -----------------------------------------------------------------------

  /**
   * Check server health.
   *
   * @returns Health information including server status, version, and uptime.
   *
   * @throws {@link ConnectionError} if the server is unreachable.
   *
   * @example
   * ```ts
   * const health = await client.health();
   * console.log(`Server ${health.version} is ${health.status}`);
   * ```
   */
  async health(): Promise<HealthInfo> {
    const response = await this._fetch("/api/v1/health", {
      method: "GET",
    });

    await handleResponse(response);
    return (await response.json()) as HealthInfo;
  }

  /**
   * List all registered providers and their health status.
   *
   * @returns An array of {@link ProviderInfo} objects.
   *
   * @example
   * ```ts
   * const providers = await client.providers();
   * for (const p of providers) {
   *   console.log(`${p.name}: healthy=${p.healthy} default=${p.default}`);
   * }
   * ```
   */
  async providers(): Promise<ProviderInfo[]> {
    const response = await this._fetch("/api/v1/providers", {
      method: "GET",
    });

    await handleResponse(response);
    return (await response.json()) as ProviderInfo[];
  }

  /**
   * Get the current VM pool status.
   *
   * @returns Pool status information, or `{ enabled: false }` if pool mode
   *   is not active.
   */
  async poolStatus(): Promise<VMPoolStatus> {
    const response = await this._fetch("/api/v1/pool/status", {
      method: "GET",
    });

    await handleResponse(response);
    return (await response.json()) as VMPoolStatus;
  }

  // -----------------------------------------------------------------------
  // Convenience patterns
  // -----------------------------------------------------------------------

  /**
   * Spawn a sandbox, execute a callback, and automatically destroy it.
   *
   * This is the recommended way to use sandboxes for short-lived tasks.
   * The sandbox is guaranteed to be destroyed even if the callback throws
   * an error (analogous to Python's `with` statement or Go's `defer`).
   *
   * @typeParam T - The return type of the callback.
   * @param opts - Sandbox spawn options.
   * @param callback - An async function that receives the {@link Sandbox}
   *   and returns a value of type `T`.
   * @returns The value returned by the callback.
   *
   * @example
   * ```ts
   * const output = await client.withSandbox(
   *   { image: "node:20-slim" },
   *   async (sb) => {
   *     await sb.writeFile("/app/index.js", 'console.log("hi")');
   *     const result = await sb.exec("node /app/index.js");
   *     return result.stdout;
   *   },
   * );
   * ```
   */
  async withSandbox<T>(
    opts: SpawnOptions,
    callback: (sandbox: Sandbox) => Promise<T>,
  ): Promise<T> {
    const sandbox = await this.spawn(opts);
    try {
      return await callback(sandbox);
    } finally {
      try {
        await sandbox.destroy();
      } catch {
        // Best-effort cleanup -- swallow errors just like Python SDK.
      }
    }
  }

  // -----------------------------------------------------------------------
  // Internal helpers
  // -----------------------------------------------------------------------

  /**
   * Issue an HTTP request against the ForgeVM API.
   *
   * Wraps `fetch` with base URL construction, default headers, timeout,
   * and connection error handling.
   */
  private async _fetch(
    path: string,
    init: RequestInit,
  ): Promise<Response> {
    const url = `${this._baseUrl}${path}`;
    const headers: Record<string, string> = {
      ...this._headers,
    };

    if (init.body) {
      headers["Content-Type"] = "application/json";
    }

    try {
      return await fetch(url, {
        ...init,
        headers: { ...headers, ...(init.headers as Record<string, string>) },
        signal: AbortSignal.timeout(this._timeout),
      });
    } catch (err: unknown) {
      // Distinguish connection errors from other failures so callers can
      // handle network issues specifically.
      if (err instanceof TypeError) {
        throw new ConnectionError(
          `Cannot connect to ForgeVM server at ${this._baseUrl}: ${err.message}`,
        );
      }
      if (err instanceof DOMException && err.name === "TimeoutError") {
        throw new ConnectionError(
          `Request to ${url} timed out after ${this._timeout}ms`,
        );
      }
      throw err;
    }
  }
}
