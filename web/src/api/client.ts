// ============================================================================
// ForgeVM API Client — typed fetch wrapper for all REST endpoints
// ============================================================================

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface Sandbox {
  id: string;
  image: string;
  provider: string;
  status: string;
  ip: string;
  created_at: string;
  expires_at: string;
  ttl: string;
  metadata?: Record<string, string>;
}

export interface CreateSandboxRequest {
  image: string;
  ttl?: string;
  provider?: string;
}

export interface ExecRequest {
  command: string;
}

export interface ExecResult {
  exit_code: number;
  stdout: string;
  stderr: string;
  duration: string;
}

export interface WriteFileRequest {
  path: string;
  content: string;
}

export interface FileEntry {
  path: string;
  size: number;
  mode: string;
  is_dir: boolean;
}

export interface Template {
  name: string;
  image: string;
  provider?: string;
  ttl?: string;
  description?: string;
  init_commands?: string[];
  files?: Record<string, string>;
  env?: Record<string, string>;
  created_at?: string;
  updated_at?: string;
}

export interface CreateTemplateRequest {
  name: string;
  image: string;
  provider?: string;
  ttl?: string;
  description?: string;
  init_commands?: string[];
  files?: Record<string, string>;
  env?: Record<string, string>;
}

export interface Provider {
  name: string;
  is_default: boolean;
  healthy: boolean;
}

export interface HealthResponse {
  status: string;
  version: string;
  uptime: string;
}

export interface MetricsResponse {
  goroutines: number;
  memory_alloc: number;
  active_sandboxes: number;
  total_sandboxes: number;
}

export interface SSEEvent {
  type: string;
  data: string;
  id?: string;
}

// ---------------------------------------------------------------------------
// API Error
// ---------------------------------------------------------------------------

export class ApiError extends Error {
  constructor(
    public status: number,
    public statusText: string,
    public body: string,
  ) {
    super(`API Error ${status}: ${statusText}`);
    this.name = 'ApiError';
  }
}

// ---------------------------------------------------------------------------
// Base fetch helper
// ---------------------------------------------------------------------------

const BASE = '/api/v1';

async function request<T>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const url = `${BASE}${path}`;
  const headers: Record<string, string> = {
    ...(options.headers as Record<string, string>),
  };

  if (options.body && typeof options.body === 'string') {
    headers['Content-Type'] = 'application/json';
  }

  const res = await fetch(url, {
    ...options,
    headers,
  });

  if (!res.ok) {
    const body = await res.text();
    throw new ApiError(res.status, res.statusText, body);
  }

  const contentType = res.headers.get('content-type') || '';
  if (contentType.includes('application/json')) {
    return res.json() as Promise<T>;
  }

  return res.text() as unknown as T;
}

// ---------------------------------------------------------------------------
// Sandboxes
// ---------------------------------------------------------------------------

export async function createSandbox(
  req: CreateSandboxRequest,
): Promise<Sandbox> {
  return request<Sandbox>('/sandboxes', {
    method: 'POST',
    body: JSON.stringify(req),
  });
}

export async function listSandboxes(): Promise<Sandbox[]> {
  const result = await request<Sandbox[] | null>('/sandboxes');
  return result ?? [];
}

export async function getSandbox(id: string): Promise<Sandbox> {
  return request<Sandbox>(`/sandboxes/${encodeURIComponent(id)}`);
}

export async function destroySandbox(id: string): Promise<void> {
  await request<void>(`/sandboxes/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  });
}

export async function extendSandboxTTL(
  id: string,
  ttl: string,
): Promise<Sandbox> {
  return request<Sandbox>(
    `/sandboxes/${encodeURIComponent(id)}/extend`,
    {
      method: 'POST',
      body: JSON.stringify({ ttl }),
    },
  );
}

export async function pruneExpired(): Promise<void> {
  await request<void>('/sandboxes', { method: 'DELETE' });
}

// ---------------------------------------------------------------------------
// Exec
// ---------------------------------------------------------------------------

export async function execCommand(
  sandboxId: string,
  command: string,
): Promise<ExecResult> {
  return request<ExecResult>(
    `/sandboxes/${encodeURIComponent(sandboxId)}/exec`,
    {
      method: 'POST',
      body: JSON.stringify({ command }),
    },
  );
}

export interface StreamChunk {
  stream: string;
  data: string;
}

export async function execStreamNDJSON(
  sandboxId: string,
  command: string,
  onChunk: (chunk: StreamChunk) => void,
  signal?: AbortSignal,
): Promise<void> {
  const url = `${BASE}/sandboxes/${encodeURIComponent(sandboxId)}/exec`;
  const res = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ command, stream: true }),
    signal,
  });

  if (!res.ok) {
    const body = await res.text();
    throw new ApiError(res.status, res.statusText, body);
  }

  const reader = res.body?.getReader();
  if (!reader) return;

  const decoder = new TextDecoder();
  let buffer = '';

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;

    buffer += decoder.decode(value, { stream: true });
    const lines = buffer.split('\n');
    buffer = lines.pop() ?? '';

    for (const line of lines) {
      const trimmed = line.trim();
      if (!trimmed) continue;
      try {
        onChunk(JSON.parse(trimmed) as StreamChunk);
      } catch {
        // skip non-JSON lines
      }
    }
  }

  // Process remaining buffer
  if (buffer.trim()) {
    try {
      onChunk(JSON.parse(buffer.trim()) as StreamChunk);
    } catch {
      // skip
    }
  }
}

// ---------------------------------------------------------------------------
// Console Logs
// ---------------------------------------------------------------------------

export async function getSandboxLogs(
  sandboxId: string,
  lines = 100,
): Promise<string[]> {
  const result = await request<string[] | null>(
    `/sandboxes/${encodeURIComponent(sandboxId)}/logs?lines=${lines}`,
  );
  return result ?? [];
}

export type LogLevel = 'INFO' | 'WARN' | 'ERROR' | 'DEBUG';

export function parseLogLevel(line: string): LogLevel {
  const upper = line.toUpperCase();
  if (upper.includes('[ERROR]') || upper.includes('ERROR:') || upper.includes(' ERR ')) return 'ERROR';
  if (upper.includes('[WARN]') || upper.includes('WARNING:') || upper.includes(' WRN ')) return 'WARN';
  if (upper.includes('[DEBUG]') || upper.includes('DEBUG:') || upper.includes(' DBG ')) return 'DEBUG';
  return 'INFO';
}

// ---------------------------------------------------------------------------
// Files
// ---------------------------------------------------------------------------

export async function writeFile(
  sandboxId: string,
  path: string,
  content: string,
): Promise<void> {
  await request<void>(
    `/sandboxes/${encodeURIComponent(sandboxId)}/files`,
    {
      method: 'POST',
      body: JSON.stringify({ path, content }),
    },
  );
}

export async function readFile(
  sandboxId: string,
  path: string,
): Promise<string> {
  return request<string>(
    `/sandboxes/${encodeURIComponent(sandboxId)}/files?path=${encodeURIComponent(path)}`,
  );
}

export async function listFiles(
  sandboxId: string,
  path: string,
): Promise<FileEntry[]> {
  const result = await request<FileEntry[] | null>(
    `/sandboxes/${encodeURIComponent(sandboxId)}/files/list?path=${encodeURIComponent(path)}`,
  );
  return result ?? [];
}

// ---------------------------------------------------------------------------
// Templates
// ---------------------------------------------------------------------------

export async function createTemplate(
  req: CreateTemplateRequest,
): Promise<Template> {
  return request<Template>('/templates', {
    method: 'POST',
    body: JSON.stringify(req),
  });
}

export async function listTemplates(): Promise<Template[]> {
  const result = await request<Template[] | null>('/templates');
  return result ?? [];
}

export async function getTemplate(name: string): Promise<Template> {
  return request<Template>(`/templates/${encodeURIComponent(name)}`);
}

export async function updateTemplate(
  name: string,
  req: CreateTemplateRequest,
): Promise<Template> {
  return request<Template>(`/templates/${encodeURIComponent(name)}`, {
    method: 'PUT',
    body: JSON.stringify(req),
  });
}

export async function deleteTemplate(name: string): Promise<void> {
  await request<void>(`/templates/${encodeURIComponent(name)}`, {
    method: 'DELETE',
  });
}

export async function spawnFromTemplate(
  name: string,
): Promise<Sandbox> {
  return request<Sandbox>(
    `/templates/${encodeURIComponent(name)}/spawn`,
    { method: 'POST' },
  );
}

// ---------------------------------------------------------------------------
// Providers
// ---------------------------------------------------------------------------

export async function listProviders(): Promise<Provider[]> {
  const result = await request<Provider[] | null>('/providers');
  return result ?? [];
}

export interface ProviderDetail {
  name: string;
  healthy: boolean;
  default: boolean;
  sandbox_count: number;
  config: Record<string, string>;
}

export async function getProviderDetail(name: string): Promise<ProviderDetail> {
  return request<ProviderDetail>(`/providers/${encodeURIComponent(name)}`);
}

// ---------------------------------------------------------------------------
// Snapshots
// ---------------------------------------------------------------------------

export interface SnapshotSummary {
  image: string;
  provider: string;
  created_at: string;
}

export async function listSnapshots(): Promise<SnapshotSummary[]> {
  const result = await request<SnapshotSummary[] | null>('/snapshots');
  return result ?? [];
}

// ---------------------------------------------------------------------------
// Health & Metrics
// ---------------------------------------------------------------------------

export async function getHealth(): Promise<HealthResponse> {
  return request<HealthResponse>('/health');
}

export async function getMetrics(): Promise<MetricsResponse> {
  return request<MetricsResponse>('/metrics');
}

// ---------------------------------------------------------------------------
// SSE Event Stream
// ---------------------------------------------------------------------------

export function subscribeEvents(
  onEvent: (event: SSEEvent) => void,
  onError?: (error: Event) => void,
): () => void {
  const source = new EventSource(`${BASE}/events`);

  source.onmessage = (e: MessageEvent) => {
    onEvent({
      type: e.type,
      data: e.data,
      id: e.lastEventId || undefined,
    });
  };

  // Listen for typed events
  const eventTypes = [
    'sandbox.created',
    'sandbox.destroyed',
    'sandbox.expired',
    'sandbox.exec',
    'template.created',
    'template.deleted',
  ];

  for (const eventType of eventTypes) {
    source.addEventListener(eventType, ((e: MessageEvent) => {
      onEvent({
        type: eventType,
        data: e.data,
        id: e.lastEventId || undefined,
      });
    }) as EventListener);
  }

  source.onerror = (e: Event) => {
    if (onError) {
      onError(e);
    }
  };

  return () => {
    source.close();
  };
}
