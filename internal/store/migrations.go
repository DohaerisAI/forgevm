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
}
