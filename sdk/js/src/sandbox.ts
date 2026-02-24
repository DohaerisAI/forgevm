/**
 * Sandbox class -- represents a single running sandbox instance.
 *
 * A {@link Sandbox} is never constructed directly.  Instead, obtain one via
 * {@link Client.spawn}, {@link Client.get}, or
 * {@link TemplateManager.spawn}.
 *
 * @module sandbox
 */

import { handleResponse } from "./errors.js";
import { parseNDJSON } from "./streaming.js";
import type {
  ExecOptions,
  ExecResult,
  FileInfo,
  SandboxInfo,
  SandboxState,
  StreamChunk,
} from "./types.js";

/**
 * A ForgeVM sandbox instance.
 *
 * Provides methods to execute commands, manage files, and control the
 * lifecycle of a sandbox running on the ForgeVM server.
 *
 * @example
 * ```ts
 * const sandbox = await client.spawn({ image: "alpine:latest" });
 *
 * const result = await sandbox.exec("echo hello");
 * console.log(result.stdout); // "hello\n"
 *
 * await sandbox.writeFile("/tmp/greeting.txt", "Hello, world!");
 * const content = await sandbox.readFile("/tmp/greeting.txt");
 *
 * await sandbox.destroy();
 * ```
 */
export class Sandbox {
  /** Base URL of the ForgeVM API (no trailing slash). */
  private readonly _baseUrl: string;
  /** Default headers sent with every request. */
  private readonly _headers: Record<string, string>;
  /** Request timeout in milliseconds. */
  private readonly _timeout: number;
  /** Cached sandbox info from the server. */
  private _info: SandboxInfo;

  /**
   * @internal -- Use {@link Client.spawn} or {@link Client.get} instead.
   */
  constructor(
    baseUrl: string,
    headers: Record<string, string>,
    timeout: number,
    info: SandboxInfo,
  ) {
    this._baseUrl = baseUrl;
    this._headers = headers;
    this._timeout = timeout;
    this._info = info;
  }

  // -----------------------------------------------------------------------
  // Read-only properties
  // -----------------------------------------------------------------------

  /** Unique sandbox identifier (e.g. `"sb-a1b2c3d4"`). */
  get id(): string {
    return this._info.id;
  }

  /** Current state of the sandbox. */
  get state(): SandboxState {
    return this._info.state;
  }

  /** Container / VM image this sandbox was created from. */
  get image(): string {
    return this._info.image;
  }

  /** Provider running this sandbox. */
  get provider(): string {
    return this._info.provider;
  }

  /** Memory allocated in megabytes. */
  get memoryMb(): number {
    return this._info.memory_mb;
  }

  /** Number of virtual CPUs. */
  get vcpus(): number {
    return this._info.vcpus;
  }

  /** Full sandbox info snapshot. */
  get info(): SandboxInfo {
    return { ...this._info };
  }

  // -----------------------------------------------------------------------
  // Command execution
  // -----------------------------------------------------------------------

  /**
   * Execute a command inside the sandbox and wait for the result.
   *
   * @param command - The command string to execute (interpreted by the
   *   sandbox's shell).
   * @param opts - Optional execution parameters.
   * @returns The execution result including exit code, stdout, stderr, and
   *   duration.
   *
   * @throws {@link SandboxNotFoundError} if the sandbox no longer exists.
   * @throws {@link ProviderError} on server-side failures.
   *
   * @example
   * ```ts
   * const result = await sandbox.exec("ls -la /tmp");
   * if (result.exit_code !== 0) {
   *   console.error(result.stderr);
   * }
   * ```
   */
  async exec(command: string, opts?: ExecOptions): Promise<ExecResult> {
    const body: Record<string, unknown> = { command };
    if (opts?.args) body["args"] = opts.args;
    if (opts?.env) body["env"] = opts.env;
    if (opts?.workdir) body["workdir"] = opts.workdir;
    if (opts?.timeout) body["timeout"] = opts.timeout;

    const response = await this._fetch(
      `/api/v1/sandboxes/${this.id}/exec`,
      {
        method: "POST",
        body: JSON.stringify(body),
      },
    );

    await handleResponse(response, this.id);
    return (await response.json()) as ExecResult;
  }

  /**
   * Execute a command and stream output chunks as they arrive.
   *
   * Returns an {@link AsyncGenerator} that yields {@link StreamChunk}
   * objects.  Each chunk contains a `stream` field (`"stdout"` or
   * `"stderr"`) and a `data` field with the text payload.
   *
   * @param command - The command string to execute.
   * @param opts - Optional execution parameters (same as {@link exec}
   *   except `timeout` is not supported for streaming).
   *
   * @yields {StreamChunk} Parsed output chunks in real time.
   *
   * @example
   * ```ts
   * for await (const chunk of sandbox.execStream("ping -c 5 localhost")) {
   *   process.stdout.write(chunk.data);
   * }
   * ```
   */
  async *execStream(
    command: string,
    opts?: Omit<ExecOptions, "timeout">,
  ): AsyncGenerator<StreamChunk, void, undefined> {
    const body: Record<string, unknown> = { command, stream: true };
    if (opts?.args) body["args"] = opts.args;
    if (opts?.env) body["env"] = opts.env;
    if (opts?.workdir) body["workdir"] = opts.workdir;

    const response = await this._fetch(
      `/api/v1/sandboxes/${this.id}/exec`,
      {
        method: "POST",
        body: JSON.stringify(body),
      },
    );

    await handleResponse(response, this.id);

    if (!response.body) {
      return;
    }

    yield* parseNDJSON(response.body);
  }

  // -----------------------------------------------------------------------
  // File operations
  // -----------------------------------------------------------------------

  /**
   * Write a file inside the sandbox.
   *
   * @param path - Absolute path inside the sandbox (e.g. `"/tmp/hello.txt"`).
   * @param content - UTF-8 string content to write.
   * @param mode - Optional Unix file mode string (e.g. `"0644"`).
   *
   * @example
   * ```ts
   * await sandbox.writeFile("/app/config.json", JSON.stringify(config));
   * ```
   */
  async writeFile(
    path: string,
    content: string,
    mode?: string,
  ): Promise<void> {
    const body: Record<string, unknown> = { path, content };
    if (mode) body["mode"] = mode;

    const response = await this._fetch(
      `/api/v1/sandboxes/${this.id}/files`,
      {
        method: "POST",
        body: JSON.stringify(body),
      },
    );

    await handleResponse(response, this.id);
  }

  /**
   * Read a file from the sandbox.
   *
   * @param path - Absolute path inside the sandbox.
   * @returns The file content as a UTF-8 string.
   *
   * @example
   * ```ts
   * const content = await sandbox.readFile("/etc/hostname");
   * ```
   */
  async readFile(path: string): Promise<string> {
    const params = new URLSearchParams({ path });
    const response = await this._fetch(
      `/api/v1/sandboxes/${this.id}/files?${params.toString()}`,
      { method: "GET" },
    );

    await handleResponse(response, this.id);
    return response.text();
  }

  /**
   * List files in a directory inside the sandbox.
   *
   * @param path - Directory path to list (default: `"/"`).
   * @returns An array of {@link FileInfo} objects.
   *
   * @example
   * ```ts
   * const files = await sandbox.listFiles("/tmp");
   * for (const f of files) {
   *   console.log(`${f.name}  ${f.size} bytes`);
   * }
   * ```
   */
  async listFiles(path: string = "/"): Promise<FileInfo[]> {
    const params = new URLSearchParams({ path });
    const response = await this._fetch(
      `/api/v1/sandboxes/${this.id}/files/list?${params.toString()}`,
      { method: "GET" },
    );

    await handleResponse(response, this.id);
    return (await response.json()) as FileInfo[];
  }

  // -----------------------------------------------------------------------
  // TTL management
  // -----------------------------------------------------------------------

  /**
   * Extend this sandbox's TTL by the given duration.
   *
   * @param ttl - A Go-style duration string (e.g. `"30m"`, `"1h"`).
   *   The duration is added to the current expiry time, not to `now`.
   *
   * @example
   * ```ts
   * await sandbox.extendTtl("30m"); // extends by 30 minutes
   * ```
   */
  async extendTtl(ttl: string = "30m"): Promise<void> {
    const response = await this._fetch(
      `/api/v1/sandboxes/${this.id}/extend`,
      {
        method: "POST",
        body: JSON.stringify({ ttl }),
      },
    );

    await handleResponse(response, this.id);
    this._info = (await response.json()) as SandboxInfo;
  }

  // -----------------------------------------------------------------------
  // Lifecycle
  // -----------------------------------------------------------------------

  /**
   * Destroy this sandbox, releasing all resources.
   *
   * After calling `destroy()` no further operations should be performed on
   * this {@link Sandbox} instance.
   *
   * @throws {@link SandboxNotFoundError} if the sandbox was already
   *   destroyed.
   */
  async destroy(): Promise<void> {
    const response = await this._fetch(
      `/api/v1/sandboxes/${this.id}`,
      { method: "DELETE" },
    );

    await handleResponse(response, this.id);
    this._info = { ...this._info, state: "destroyed" };
  }

  /**
   * Refresh the cached sandbox info from the server.
   *
   * Call this to get the latest state if you suspect the sandbox may have
   * changed (e.g. expired via TTL).
   *
   * @returns This {@link Sandbox} instance (for chaining).
   */
  async refresh(): Promise<Sandbox> {
    const response = await this._fetch(
      `/api/v1/sandboxes/${this.id}`,
      { method: "GET" },
    );

    await handleResponse(response, this.id);
    this._info = (await response.json()) as SandboxInfo;
    return this;
  }

  // -----------------------------------------------------------------------
  // Internal helpers
  // -----------------------------------------------------------------------

  /**
   * Issue an HTTP request against the ForgeVM API.
   *
   * Handles base URL construction, default headers, and timeout via
   * `AbortSignal.timeout`.
   */
  private async _fetch(
    path: string,
    init: RequestInit,
  ): Promise<Response> {
    const url = `${this._baseUrl}${path}`;
    const headers: Record<string, string> = {
      ...this._headers,
    };

    // Only set Content-Type for requests that have a body.
    if (init.body) {
      headers["Content-Type"] = "application/json";
    }

    return fetch(url, {
      ...init,
      headers: { ...headers, ...(init.headers as Record<string, string>) },
      signal: AbortSignal.timeout(this._timeout),
    });
  }

  // -----------------------------------------------------------------------
  // Debug helpers
  // -----------------------------------------------------------------------

  /** Returns a human-readable string representation. */
  toString(): string {
    return `Sandbox(id="${this.id}", state="${this.state}")`;
  }

  /** Customise `JSON.stringify` output. */
  toJSON(): SandboxInfo {
    return this.info;
  }
}
