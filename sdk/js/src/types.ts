/**
 * TypeScript type definitions for the ForgeVM SDK.
 *
 * @module types
 */

// ---------------------------------------------------------------------------
// Sandbox types
// ---------------------------------------------------------------------------

/**
 * Possible states a sandbox can be in during its lifecycle.
 *
 * - `creating` -- the sandbox is being provisioned
 * - `running`  -- the sandbox is up and accepting commands
 * - `stopped`  -- the sandbox has been gracefully stopped
 * - `destroyed` -- the sandbox has been torn down
 * - `error`    -- the sandbox encountered a fatal error
 */
export type SandboxState =
  | "creating"
  | "running"
  | "stopped"
  | "destroyed"
  | "error";

/**
 * Configuration values for a sandbox, as returned by the API.
 */
export interface SandboxConfig {
  /** Container / VM image to use (e.g. `"alpine:latest"`). */
  image: string;
  /** Number of virtual CPUs allocated. */
  vcpus: number;
  /** Memory allocated in megabytes. */
  memory_mb: number;
  /** Provider name (e.g. `"mock"`, `"firecracker"`). */
  provider: string;
  /** Time-to-live duration string (e.g. `"30m"`, `"2h"`). */
  ttl: string;
  /** Arbitrary key-value metadata attached to the sandbox. */
  metadata: Record<string, string>;
}

/**
 * Options accepted when spawning a new sandbox via {@link Client.spawn}.
 * All fields are optional; the server applies sensible defaults.
 */
export interface SpawnOptions {
  /** Container / VM image (default: `"alpine:latest"`). */
  image?: string;
  /** Provider to use for this sandbox. */
  provider?: string;
  /** Memory in megabytes. */
  memory_mb?: number;
  /** Number of virtual CPUs. */
  vcpus?: number;
  /** Time-to-live duration string. */
  ttl?: string;
  /** Arbitrary key-value metadata. */
  metadata?: Record<string, string>;
}

/**
 * Full information about a sandbox as returned by list / get endpoints.
 */
export interface SandboxInfo {
  /** Unique sandbox identifier (e.g. `"sb-a1b2c3d4"`). */
  id: string;
  /** Current state of the sandbox. */
  state: SandboxState;
  /** Provider that is running this sandbox. */
  provider: string;
  /** Image the sandbox was created from. */
  image: string;
  /** Memory allocated in megabytes. */
  memory_mb: number;
  /** Number of virtual CPUs. */
  vcpus: number;
  /** ISO-8601 creation timestamp. */
  created_at: string;
  /** ISO-8601 expiry timestamp (empty string if no TTL). */
  expires_at: string;
  /** Arbitrary key-value metadata. */
  metadata: Record<string, string>;
}

// ---------------------------------------------------------------------------
// Execution types
// ---------------------------------------------------------------------------

/**
 * Options for executing a command inside a sandbox.
 */
export interface ExecOptions {
  /** Positional arguments appended to the command. */
  args?: string[];
  /** Environment variables injected into the command. */
  env?: Record<string, string>;
  /** Working directory inside the sandbox. */
  workdir?: string;
  /** Timeout duration string (e.g. `"10s"`, `"1m"`). */
  timeout?: string;
}

/**
 * Result of a non-streaming command execution.
 */
export interface ExecResult {
  /** Process exit code (`0` indicates success). */
  exit_code: number;
  /** Captured standard output. */
  stdout: string;
  /** Captured standard error. */
  stderr: string;
  /** Human-readable execution duration (e.g. `"123ms"`). */
  duration: string;
}

/**
 * A single chunk emitted during streaming execution.
 */
export interface StreamChunk {
  /** Which output stream this chunk belongs to. */
  stream: "stdout" | "stderr";
  /** The data payload for this chunk. */
  data: string;
}

// ---------------------------------------------------------------------------
// File types
// ---------------------------------------------------------------------------

/**
 * Information about a file or directory inside a sandbox.
 */
export interface FileInfo {
  /** File or directory name. */
  name: string;
  /** Full path inside the sandbox. */
  path: string;
  /** Size in bytes. */
  size: number;
  /** Whether this entry is a directory. */
  is_dir: boolean;
  /** Last modification time as ISO-8601 string. */
  mod_time: string;
  /** Unix file mode (e.g. `"0644"`). */
  mode: string;
}

// ---------------------------------------------------------------------------
// Template types
// ---------------------------------------------------------------------------

/**
 * A sandbox template stored on the server.
 */
export interface Template {
  /** Unique template name. */
  name: string;
  /** Container / VM image. */
  image: string;
  /** Memory in megabytes. */
  memory_mb: number;
  /** Number of virtual CPUs. */
  vcpus: number;
  /** Time-to-live duration string. */
  ttl: string;
  /** Default provider. */
  provider: string;
  /** Arbitrary metadata. */
  metadata: Record<string, string>;
}

/**
 * Options for creating or updating a template.
 * `name` and `image` are required; everything else has server-side defaults.
 */
export interface TemplateConfig {
  /** Unique template name. */
  name: string;
  /** Container / VM image. */
  image: string;
  /** Memory in megabytes (default: 512). */
  memory_mb?: number;
  /** Number of virtual CPUs (default: 1). */
  vcpus?: number;
  /** Time-to-live duration string (default: `"30m"`). */
  ttl?: string;
  /** Default provider. */
  provider?: string;
  /** Arbitrary metadata. */
  metadata?: Record<string, string>;
}

/**
 * Override options when spawning from a template.
 */
export interface TemplateSpawnOverrides {
  /** Override the template's default provider. */
  provider?: string;
  /** Override the template's default TTL. */
  ttl?: string;
}

// ---------------------------------------------------------------------------
// Provider types
// ---------------------------------------------------------------------------

/**
 * Information about a registered provider.
 */
export interface ProviderInfo {
  /** Provider name (e.g. `"mock"`, `"firecracker"`). */
  name: string;
  /** Whether the provider is currently healthy. */
  healthy: boolean;
  /** Whether this is the default provider. */
  default: boolean;
}

// ---------------------------------------------------------------------------
// System types
// ---------------------------------------------------------------------------

/**
 * Health check response from the server.
 */
export interface HealthInfo {
  /** Server health status (e.g. `"ok"`). */
  status: string;
  /** Server version string. */
  version: string;
  /** Human-readable uptime (e.g. `"2h30m15s"`). */
  uptime: string;
}

// ---------------------------------------------------------------------------
// Client configuration
// ---------------------------------------------------------------------------

/**
 * Options for constructing a {@link Client} instance.
 */
export interface ForgevmClientOptions {
  /**
   * Hostname or IP of the ForgeVM server.
   * @defaultValue `"localhost"`
   */
  host?: string;
  /**
   * Port the ForgeVM server is listening on.
   * @defaultValue `7423`
   */
  port?: number;
  /**
   * Full base URL (overrides `host` and `port` if provided).
   * Example: `"https://forgevm.example.com"`.
   */
  baseUrl?: string;
  /**
   * API key for authenticated access.  Sent as `X-API-Key` header.
   */
  apiKey?: string;
  /**
   * User ID for multi-tenant pool mode. Sent as `X-User-ID` header.
   */
  userId?: string;
  /**
   * Request timeout in milliseconds.
   * @defaultValue `30000`
   */
  timeout?: number;
}

/**
 * VM pool status information.
 */
export interface VMPoolStatus {
  enabled: boolean;
  vms: number;
  max_vms: number;
  total_users: number;
  max_users_per_vm: number;
}

// ---------------------------------------------------------------------------
// API error envelope
// ---------------------------------------------------------------------------

/**
 * Shape of an error response body returned by the ForgeVM API.
 */
export interface ApiErrorBody {
  /** Machine-readable error code (e.g. `"NOT_FOUND"`, `"INTERNAL"`). */
  code: string;
  /** Human-readable error message. */
  message: string;
}
