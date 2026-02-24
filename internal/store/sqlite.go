package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(path string) (*SQLiteStore, error) {
	dsn := path + "?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	db.SetMaxOpenConns(1)

	s := &SQLiteStore{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	return s, nil
}

func (s *SQLiteStore) migrate() error {
	// Ensure schema_migrations table exists
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		applied_at DATETIME NOT NULL DEFAULT (datetime('now'))
	)`)
	if err != nil {
		return err
	}

	for _, m := range migrations {
		var count int
		err := s.db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = ?", m.version).Scan(&count)
		if err != nil {
			return err
		}
		if count > 0 {
			continue
		}

		tx, err := s.db.Begin()
		if err != nil {
			return err
		}

		if _, err := tx.Exec(m.sql); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration v%d: %w", m.version, err)
		}
		if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", m.version); err != nil {
			tx.Rollback()
			return err
		}

		if err := tx.Commit(); err != nil {
			return err
		}
	}

	return nil
}

// --- Sandbox CRUD ---

func (s *SQLiteStore) CreateSandbox(ctx context.Context, sb *SandboxRecord) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sandboxes (id, state, provider, image, memory_mb, vcpus, metadata, created_at, expires_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sb.ID, sb.State, sb.Provider, sb.Image, sb.MemoryMB, sb.VCPUs, sb.Metadata,
		sb.CreatedAt.UTC(), sb.ExpiresAt.UTC(), sb.UpdatedAt.UTC(),
	)
	return err
}

func (s *SQLiteStore) GetSandbox(ctx context.Context, id string) (*SandboxRecord, error) {
	sb := &SandboxRecord{}
	err := s.db.QueryRowContext(ctx, `
		SELECT id, state, provider, image, memory_mb, vcpus, metadata, created_at, expires_at, updated_at
		FROM sandboxes WHERE id = ?`, id,
	).Scan(&sb.ID, &sb.State, &sb.Provider, &sb.Image, &sb.MemoryMB, &sb.VCPUs,
		&sb.Metadata, &sb.CreatedAt, &sb.ExpiresAt, &sb.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("sandbox %q not found", id)
	}
	return sb, err
}

func (s *SQLiteStore) ListSandboxes(ctx context.Context) ([]*SandboxRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, state, provider, image, memory_mb, vcpus, metadata, created_at, expires_at, updated_at
		FROM sandboxes WHERE state != 'destroyed' ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sandboxes []*SandboxRecord
	for rows.Next() {
		sb := &SandboxRecord{}
		if err := rows.Scan(&sb.ID, &sb.State, &sb.Provider, &sb.Image, &sb.MemoryMB, &sb.VCPUs,
			&sb.Metadata, &sb.CreatedAt, &sb.ExpiresAt, &sb.UpdatedAt); err != nil {
			return nil, err
		}
		sandboxes = append(sandboxes, sb)
	}
	return sandboxes, rows.Err()
}

func (s *SQLiteStore) UpdateSandboxState(ctx context.Context, id string, state string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE sandboxes SET state = ?, updated_at = ? WHERE id = ?`,
		state, time.Now().UTC(), id,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("sandbox %q not found", id)
	}
	return nil
}

func (s *SQLiteStore) UpdateSandboxExpiresAt(ctx context.Context, id string, expiresAt time.Time) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE sandboxes SET expires_at = ?, updated_at = ? WHERE id = ? AND state != 'destroyed'`,
		expiresAt.UTC(), time.Now().UTC(), id,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("sandbox %q not found or already destroyed", id)
	}
	return nil
}

func (s *SQLiteStore) DeleteSandbox(ctx context.Context, id string) error {
	return s.UpdateSandboxState(ctx, id, "destroyed")
}

func (s *SQLiteStore) ListExpiredSandboxes(ctx context.Context, before time.Time) ([]*SandboxRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, state, provider, image, memory_mb, vcpus, metadata, created_at, expires_at, updated_at
		FROM sandboxes WHERE state NOT IN ('destroyed') AND expires_at < ?`,
		before.UTC(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sandboxes []*SandboxRecord
	for rows.Next() {
		sb := &SandboxRecord{}
		if err := rows.Scan(&sb.ID, &sb.State, &sb.Provider, &sb.Image, &sb.MemoryMB, &sb.VCPUs,
			&sb.Metadata, &sb.CreatedAt, &sb.ExpiresAt, &sb.UpdatedAt); err != nil {
			return nil, err
		}
		sandboxes = append(sandboxes, sb)
	}
	return sandboxes, rows.Err()
}

// --- Exec Logs ---

func (s *SQLiteStore) CreateExecLog(ctx context.Context, log *ExecLogRecord) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO exec_logs (sandbox_id, command, exit_code, stdout, stderr, duration, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		log.SandboxID, log.Command, log.ExitCode, log.Stdout, log.Stderr, log.Duration, log.CreatedAt.UTC(),
	)
	return err
}

func (s *SQLiteStore) ListExecLogs(ctx context.Context, sandboxID string) ([]*ExecLogRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, sandbox_id, command, exit_code, stdout, stderr, duration, created_at
		FROM exec_logs WHERE sandbox_id = ? ORDER BY created_at DESC`, sandboxID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []*ExecLogRecord
	for rows.Next() {
		l := &ExecLogRecord{}
		if err := rows.Scan(&l.ID, &l.SandboxID, &l.Command, &l.ExitCode, &l.Stdout, &l.Stderr,
			&l.Duration, &l.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

// --- Provider Configs ---

func (s *SQLiteStore) GetProviderConfig(ctx context.Context, name string) (*ProviderConfigRecord, error) {
	cfg := &ProviderConfigRecord{}
	err := s.db.QueryRowContext(ctx, `
		SELECT name, config, enabled, updated_at FROM provider_configs WHERE name = ?`, name,
	).Scan(&cfg.Name, &cfg.Config, &cfg.Enabled, &cfg.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("provider config %q not found", name)
	}
	return cfg, err
}

func (s *SQLiteStore) SaveProviderConfig(ctx context.Context, cfg *ProviderConfigRecord) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO provider_configs (name, config, enabled, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET config = excluded.config, enabled = excluded.enabled, updated_at = excluded.updated_at`,
		cfg.Name, cfg.Config, cfg.Enabled, time.Now().UTC(),
	)
	return err
}

func (s *SQLiteStore) ListProviderConfigs(ctx context.Context) ([]*ProviderConfigRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT name, config, enabled, updated_at FROM provider_configs`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var configs []*ProviderConfigRecord
	for rows.Next() {
		cfg := &ProviderConfigRecord{}
		if err := rows.Scan(&cfg.Name, &cfg.Config, &cfg.Enabled, &cfg.UpdatedAt); err != nil {
			return nil, err
		}
		configs = append(configs, cfg)
	}
	return configs, rows.Err()
}

// --- Templates ---

func (s *SQLiteStore) CreateTemplate(ctx context.Context, t *TemplateRecord) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO templates (name, version, image, description, setup, allowed_hosts, memory_mb, cpu_cores, ttl_seconds, env, secrets, pool_size, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.Name, t.Version, t.Image, t.Description, t.Setup, t.AllowedHosts,
		t.MemoryMB, t.CPUCores, t.TTLSeconds, t.Env, t.Secrets, t.PoolSize,
		t.CreatedAt.UTC(), t.UpdatedAt.UTC(),
	)
	return err
}

func (s *SQLiteStore) GetTemplate(ctx context.Context, name string) (*TemplateRecord, error) {
	t := &TemplateRecord{}
	err := s.db.QueryRowContext(ctx, `
		SELECT name, version, image, description, setup, allowed_hosts, memory_mb, cpu_cores, ttl_seconds, env, secrets, pool_size, created_at, updated_at
		FROM templates WHERE name = ?`, name,
	).Scan(&t.Name, &t.Version, &t.Image, &t.Description, &t.Setup, &t.AllowedHosts,
		&t.MemoryMB, &t.CPUCores, &t.TTLSeconds, &t.Env, &t.Secrets, &t.PoolSize,
		&t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("template %q not found", name)
	}
	return t, err
}

func (s *SQLiteStore) ListTemplates(ctx context.Context) ([]*TemplateRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT name, version, image, description, setup, allowed_hosts, memory_mb, cpu_cores, ttl_seconds, env, secrets, pool_size, created_at, updated_at
		FROM templates ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var templates []*TemplateRecord
	for rows.Next() {
		t := &TemplateRecord{}
		if err := rows.Scan(&t.Name, &t.Version, &t.Image, &t.Description, &t.Setup, &t.AllowedHosts,
			&t.MemoryMB, &t.CPUCores, &t.TTLSeconds, &t.Env, &t.Secrets, &t.PoolSize,
			&t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		templates = append(templates, t)
	}
	return templates, rows.Err()
}

func (s *SQLiteStore) UpdateTemplate(ctx context.Context, t *TemplateRecord) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE templates SET version = ?, image = ?, description = ?, setup = ?, allowed_hosts = ?,
		memory_mb = ?, cpu_cores = ?, ttl_seconds = ?, env = ?, secrets = ?, pool_size = ?, updated_at = ?
		WHERE name = ?`,
		t.Version, t.Image, t.Description, t.Setup, t.AllowedHosts,
		t.MemoryMB, t.CPUCores, t.TTLSeconds, t.Env, t.Secrets, t.PoolSize,
		time.Now().UTC(), t.Name,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("template %q not found", t.Name)
	}
	return nil
}

func (s *SQLiteStore) DeleteTemplate(ctx context.Context, name string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM templates WHERE name = ?`, name)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("template %q not found", name)
	}
	return nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
