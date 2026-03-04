package store

var migrations = []struct {
	version int
	sql     string
}{
	{
		version: 1,
		sql: `
CREATE TABLE IF NOT EXISTS sandboxes (
    id         TEXT PRIMARY KEY,
    state      TEXT NOT NULL DEFAULT 'creating',
    provider   TEXT NOT NULL,
    image      TEXT NOT NULL DEFAULT '',
    memory_mb  INTEGER NOT NULL DEFAULT 512,
    vcpus      INTEGER NOT NULL DEFAULT 1,
    metadata   TEXT NOT NULL DEFAULT '{}',
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    expires_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS exec_logs (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    sandbox_id TEXT NOT NULL REFERENCES sandboxes(id),
    command    TEXT NOT NULL,
    exit_code  INTEGER NOT NULL DEFAULT 0,
    stdout     TEXT NOT NULL DEFAULT '',
    stderr     TEXT NOT NULL DEFAULT '',
    duration   TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_exec_logs_sandbox ON exec_logs(sandbox_id);

CREATE TABLE IF NOT EXISTS provider_configs (
    name       TEXT PRIMARY KEY,
    config     TEXT NOT NULL DEFAULT '{}',
    enabled    BOOLEAN NOT NULL DEFAULT 0,
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS schema_migrations (
    version    INTEGER PRIMARY KEY,
    applied_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
`,
	},
	{
		version: 2,
		sql: `
CREATE TABLE IF NOT EXISTS templates (
    name         TEXT PRIMARY KEY,
    version      INTEGER NOT NULL DEFAULT 1,
    image        TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    setup        TEXT NOT NULL DEFAULT '[]',
    allowed_hosts TEXT NOT NULL DEFAULT '[]',
    memory_mb    INTEGER NOT NULL DEFAULT 512,
    cpu_cores    INTEGER NOT NULL DEFAULT 1,
    ttl_seconds  INTEGER NOT NULL DEFAULT 300,
    env          TEXT NOT NULL DEFAULT '{}',
    secrets      TEXT NOT NULL DEFAULT '[]',
    pool_size    INTEGER NOT NULL DEFAULT 0,
    created_at   DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at   DATETIME NOT NULL DEFAULT (datetime('now'))
);

ALTER TABLE sandboxes ADD COLUMN template TEXT NOT NULL DEFAULT '';
`,
	},
	{
		version: 3,
		sql: `
CREATE TABLE IF NOT EXISTS environment_specs (
    id               TEXT PRIMARY KEY,
    owner_id         TEXT NOT NULL,
    name             TEXT NOT NULL,
    base_image       TEXT NOT NULL,
    python_packages  TEXT NOT NULL DEFAULT '[]',
    apt_packages     TEXT NOT NULL DEFAULT '[]',
    python_version   TEXT NOT NULL DEFAULT '',
    created_at       DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at       DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(owner_id, name)
);

CREATE INDEX IF NOT EXISTS idx_environment_specs_owner ON environment_specs(owner_id);

CREATE TABLE IF NOT EXISTS environment_builds (
    id               TEXT PRIMARY KEY,
    spec_id          TEXT NOT NULL REFERENCES environment_specs(id),
    status           TEXT NOT NULL DEFAULT 'queued',
    current_step     TEXT NOT NULL DEFAULT '',
    log_blob         TEXT NOT NULL DEFAULT '',
    image_size_bytes INTEGER NOT NULL DEFAULT 0,
    digest_local     TEXT NOT NULL DEFAULT '',
    error            TEXT NOT NULL DEFAULT '',
    created_at       DATETIME NOT NULL DEFAULT (datetime('now')),
    finished_at      DATETIME,
    updated_at       DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_environment_builds_spec ON environment_builds(spec_id);
CREATE INDEX IF NOT EXISTS idx_environment_builds_status ON environment_builds(status);

CREATE TABLE IF NOT EXISTS environment_artifacts (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    build_id         TEXT NOT NULL REFERENCES environment_builds(id),
    target           TEXT NOT NULL,
    image_ref        TEXT NOT NULL,
    digest           TEXT NOT NULL DEFAULT '',
    status           TEXT NOT NULL DEFAULT 'pending',
    error            TEXT NOT NULL DEFAULT '',
    created_at       DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at       DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(build_id, target)
);

CREATE INDEX IF NOT EXISTS idx_environment_artifacts_build ON environment_artifacts(build_id);
CREATE INDEX IF NOT EXISTS idx_environment_artifacts_target ON environment_artifacts(target);

CREATE TABLE IF NOT EXISTS registry_connections (
    id               TEXT PRIMARY KEY,
    owner_id         TEXT NOT NULL,
    provider         TEXT NOT NULL,
    username         TEXT NOT NULL,
    secret_ref       TEXT NOT NULL,
    is_default       BOOLEAN NOT NULL DEFAULT 0,
    created_at       DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at       DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(owner_id, provider, username)
);

CREATE INDEX IF NOT EXISTS idx_registry_connections_owner ON registry_connections(owner_id);
CREATE INDEX IF NOT EXISTS idx_registry_connections_provider ON registry_connections(provider);
`,
	},
	{
		version: 4,
		sql: `
ALTER TABLE sandboxes ADD COLUMN owner_id TEXT NOT NULL DEFAULT '';
ALTER TABLE sandboxes ADD COLUMN vm_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_sandboxes_owner ON sandboxes(owner_id);
CREATE INDEX IF NOT EXISTS idx_sandboxes_vm ON sandboxes(vm_id);
`,
	},
}
