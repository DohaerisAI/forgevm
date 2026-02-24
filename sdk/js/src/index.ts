/**
 * ForgeVM TypeScript SDK -- client for the ForgeVM sandbox orchestrator.
 *
 * @example
 * ```ts
 * import { Client } from "forgevm";
 *
 * const client = new Client();
 * const sandbox = await client.spawn({ image: "alpine:latest" });
 *
 * const result = await sandbox.exec("echo hello");
 * console.log(result.stdout);
 *
 * await sandbox.destroy();
 * ```
 *
 * @packageDocumentation
 */

// -- Core classes -----------------------------------------------------------
export { Client } from "./client.js";
export { Sandbox } from "./sandbox.js";
export { TemplateManager } from "./templates.js";

// -- Error classes ----------------------------------------------------------
export {
  ForgevmError,
  SandboxNotFoundError,
  ProviderError,
  ConnectionError,
  handleResponse,
} from "./errors.js";

// -- Streaming utilities ----------------------------------------------------
export { parseNDJSON } from "./streaming.js";

// -- Type definitions -------------------------------------------------------
export type {
  SandboxConfig,
  SpawnOptions,
  ExecOptions,
  ExecResult,
  SandboxInfo,
  SandboxState,
  Template,
  TemplateConfig,
  TemplateSpawnOverrides,
  ProviderInfo,
  HealthInfo,
  StreamChunk,
  FileInfo,
  ForgevmClientOptions,
  ApiErrorBody,
} from "./types.js";
