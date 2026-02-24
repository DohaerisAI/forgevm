/**
 * Template management for the ForgeVM SDK.
 *
 * Templates are pre-configured sandbox blueprints stored on the server.
 * They allow you to spawn sandboxes quickly without repeating configuration.
 *
 * @module templates
 */

import { handleResponse } from "./errors.js";
import { Sandbox } from "./sandbox.js";
import type {
  SandboxInfo,
  Template,
  TemplateConfig,
  TemplateSpawnOverrides,
} from "./types.js";

/**
 * Manages sandbox templates on the ForgeVM server.
 *
 * Access this via {@link Client.templates} rather than constructing
 * directly.
 *
 * @example
 * ```ts
 * // Save a template
 * await client.templates.save({
 *   name: "python-dev",
 *   image: "python:3.12-slim",
 *   memory_mb: 1024,
 *   vcpus: 2,
 * });
 *
 * // List all templates
 * const templates = await client.templates.list();
 *
 * // Spawn from a template
 * const sandbox = await client.templates.spawn("python-dev");
 * ```
 */
export class TemplateManager {
  /** Base URL of the ForgeVM API (no trailing slash). */
  private readonly _baseUrl: string;
  /** Default headers sent with every request. */
  private readonly _headers: Record<string, string>;
  /** Request timeout in milliseconds. */
  private readonly _timeout: number;

  /**
   * @internal -- Constructed by {@link Client}, not intended for direct use.
   */
  constructor(
    baseUrl: string,
    headers: Record<string, string>,
    timeout: number,
  ) {
    this._baseUrl = baseUrl;
    this._headers = headers;
    this._timeout = timeout;
  }

  /**
   * List all templates stored on the server.
   *
   * @returns An array of {@link Template} objects.
   */
  async list(): Promise<Template[]> {
    const response = await this._fetch("/api/v1/templates", {
      method: "GET",
    });

    await handleResponse(response);
    return (await response.json()) as Template[];
  }

  /**
   * Retrieve a single template by name.
   *
   * @param name - The template name.
   * @returns The {@link Template} object.
   *
   * @throws {@link ForgevmError} with code `"NOT_FOUND"` if the template
   *   does not exist.
   */
  async get(name: string): Promise<Template> {
    const response = await this._fetch(`/api/v1/templates/${encodeURIComponent(name)}`, {
      method: "GET",
    });

    await handleResponse(response);
    return (await response.json()) as Template;
  }

  /**
   * Create or update a template on the server.
   *
   * If a template with the given name already exists, the server will
   * reject the request with a 409 Conflict.  Use separate update logic or
   * delete-then-save if you need to overwrite.
   *
   * @param config - Template configuration.  `name` and `image` are
   *   required.
   * @returns The template as stored by the server (with defaults filled in).
   */
  async save(config: TemplateConfig): Promise<Template> {
    const body: Record<string, unknown> = {
      name: config.name,
      image: config.image,
    };
    if (config.memory_mb !== undefined) body["memory_mb"] = config.memory_mb;
    if (config.vcpus !== undefined) body["vcpus"] = config.vcpus;
    if (config.ttl !== undefined) body["ttl"] = config.ttl;
    if (config.provider !== undefined) body["provider"] = config.provider;
    if (config.metadata !== undefined) body["metadata"] = config.metadata;

    const response = await this._fetch("/api/v1/templates", {
      method: "POST",
      body: JSON.stringify(body),
    });

    await handleResponse(response);
    return (await response.json()) as Template;
  }

  /**
   * Delete a template from the server.
   *
   * @param name - The template name to delete.
   *
   * @throws {@link ForgevmError} with code `"NOT_FOUND"` if the template
   *   does not exist.
   */
  async delete(name: string): Promise<void> {
    const response = await this._fetch(
      `/api/v1/templates/${encodeURIComponent(name)}`,
      { method: "DELETE" },
    );

    await handleResponse(response);
  }

  /**
   * Spawn a new sandbox from a named template.
   *
   * The sandbox inherits all configuration from the template.  You can
   * optionally override `provider` and `ttl` at spawn time.
   *
   * @param name - The template name to spawn from.
   * @param overrides - Optional overrides for provider and/or TTL.
   * @returns A {@link Sandbox} instance ready for use.
   *
   * @example
   * ```ts
   * const sandbox = await client.templates.spawn("python-dev", {
   *   ttl: "1h",
   * });
   * ```
   */
  async spawn(
    name: string,
    overrides?: TemplateSpawnOverrides,
  ): Promise<Sandbox> {
    const body: Record<string, unknown> = {};
    if (overrides?.provider) body["provider"] = overrides.provider;
    if (overrides?.ttl) body["ttl"] = overrides.ttl;

    const response = await this._fetch(
      `/api/v1/templates/${encodeURIComponent(name)}/spawn`,
      {
        method: "POST",
        body: JSON.stringify(body),
      },
    );

    await handleResponse(response);
    const data = (await response.json()) as SandboxInfo;

    return new Sandbox(this._baseUrl, this._headers, this._timeout, data);
  }

  // -----------------------------------------------------------------------
  // Internal helpers
  // -----------------------------------------------------------------------

  /**
   * Issue an HTTP request against the ForgeVM API.
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

    return fetch(url, {
      ...init,
      headers: { ...headers, ...(init.headers as Record<string, string>) },
      signal: AbortSignal.timeout(this._timeout),
    });
  }
}
