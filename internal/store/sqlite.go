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

// --- Environment Specs ---

func (s *SQLiteStore) CreateEnvironmentSpec(ctx context.Context, spec *EnvironmentSpecRecord) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO environment_specs (id, owner_id, name, base_image, python_packages, apt_packages, python_version, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		spec.ID, spec.OwnerID, spec.Name, spec.BaseImage, spec.PythonPackages, spec.AptPackages, spec.PythonVersion,
		spec.CreatedAt.UTC(), spec.UpdatedAt.UTC(),
	)
	return err
}

func (s *SQLiteStore) GetEnvironmentSpec(ctx context.Context, id string) (*EnvironmentSpecRecord, error) {
	spec := &EnvironmentSpecRecord{}
	err := s.db.QueryRowContext(ctx, `
		SELECT id, owner_id, name, base_image, python_packages, apt_packages, python_version, created_at, updated_at
		FROM environment_specs WHERE id = ?`, id,
	).Scan(
		&spec.ID, &spec.OwnerID, &spec.Name, &spec.BaseImage, &spec.PythonPackages, &spec.AptPackages,
		&spec.PythonVersion, &spec.CreatedAt, &spec.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("environment spec %q not found", id)
	}
	return spec, err
}

func (s *SQLiteStore) ListEnvironmentSpecs(ctx context.Context, ownerID string) ([]*EnvironmentSpecRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, owner_id, name, base_image, python_packages, apt_packages, python_version, created_at, updated_at
		FROM environment_specs
		WHERE owner_id = ?
		ORDER BY created_at DESC`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var specs []*EnvironmentSpecRecord
	for rows.Next() {
		spec := &EnvironmentSpecRecord{}
		if err := rows.Scan(
			&spec.ID, &spec.OwnerID, &spec.Name, &spec.BaseImage, &spec.PythonPackages, &spec.AptPackages,
			&spec.PythonVersion, &spec.CreatedAt, &spec.UpdatedAt,
		); err != nil {
			return nil, err
		}
		specs = append(specs, spec)
	}
	return specs, rows.Err()
}

func (s *SQLiteStore) UpdateEnvironmentSpec(ctx context.Context, spec *EnvironmentSpecRecord) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE environment_specs
		SET name = ?, base_image = ?, python_packages = ?, apt_packages = ?, python_version = ?, updated_at = ?
		WHERE id = ?`,
		spec.Name, spec.BaseImage, spec.PythonPackages, spec.AptPackages, spec.PythonVersion,
		time.Now().UTC(), spec.ID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("environment spec %q not found", spec.ID)
	}
	return nil
}

func (s *SQLiteStore) DeleteEnvironmentSpec(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM environment_specs WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("environment spec %q not found", id)
	}
	return nil
}

// --- Environment Builds ---

func (s *SQLiteStore) CreateEnvironmentBuild(ctx context.Context, build *EnvironmentBuildRecord) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO environment_builds (
			id, spec_id, status, current_step, log_blob, image_size_bytes, digest_local, error, created_at, finished_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		build.ID, build.SpecID, build.Status, build.CurrentStep, build.LogBlob, build.ImageSizeBytes,
		build.DigestLocal, build.Error, build.CreatedAt.UTC(), nullableTime(build.FinishedAt), build.UpdatedAt.UTC(),
	)
	return err
}

func (s *SQLiteStore) GetEnvironmentBuild(ctx context.Context, id string) (*EnvironmentBuildRecord, error) {
	build := &EnvironmentBuildRecord{}
	var finishedAt sql.NullTime

	err := s.db.QueryRowContext(ctx, `
		SELECT id, spec_id, status, current_step, log_blob, image_size_bytes, digest_local, error, created_at, finished_at, updated_at
		FROM environment_builds WHERE id = ?`, id,
	).Scan(
		&build.ID, &build.SpecID, &build.Status, &build.CurrentStep, &build.LogBlob, &build.ImageSizeBytes,
		&build.DigestLocal, &build.Error, &build.CreatedAt, &finishedAt, &build.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("environment build %q not found", id)
	}
	if err != nil {
		return nil, err
	}
	if finishedAt.Valid {
		t := finishedAt.Time
		build.FinishedAt = &t
	}
	return build, nil
}

func (s *SQLiteStore) ListEnvironmentBuilds(ctx context.Context, specID string) ([]*EnvironmentBuildRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, spec_id, status, current_step, log_blob, image_size_bytes, digest_local, error, created_at, finished_at, updated_at
		FROM environment_builds
		WHERE spec_id = ?
		ORDER BY created_at DESC`, specID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var builds []*EnvironmentBuildRecord
	for rows.Next() {
		build := &EnvironmentBuildRecord{}
		var finishedAt sql.NullTime
		if err := rows.Scan(
			&build.ID, &build.SpecID, &build.Status, &build.CurrentStep, &build.LogBlob, &build.ImageSizeBytes,
			&build.DigestLocal, &build.Error, &build.CreatedAt, &finishedAt, &build.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if finishedAt.Valid {
			t := finishedAt.Time
			build.FinishedAt = &t
		}
		builds = append(builds, build)
	}
	return builds, rows.Err()
}

func (s *SQLiteStore) UpdateEnvironmentBuild(ctx context.Context, build *EnvironmentBuildRecord) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE environment_builds
		SET status = ?, current_step = ?, log_blob = ?, image_size_bytes = ?, digest_local = ?, error = ?, finished_at = ?, updated_at = ?
		WHERE id = ?`,
		build.Status, build.CurrentStep, build.LogBlob, build.ImageSizeBytes, build.DigestLocal, build.Error,
		nullableTime(build.FinishedAt), time.Now().UTC(), build.ID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("environment build %q not found", build.ID)
	}
	return nil
}

// --- Environment Artifacts ---

func (s *SQLiteStore) SaveEnvironmentArtifact(ctx context.Context, artifact *EnvironmentArtifactRecord) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO environment_artifacts (build_id, target, image_ref, digest, status, error, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(build_id, target) DO UPDATE SET
			image_ref = excluded.image_ref,
			digest = excluded.digest,
			status = excluded.status,
			error = excluded.error,
			updated_at = excluded.updated_at`,
		artifact.BuildID, artifact.Target, artifact.ImageRef, artifact.Digest, artifact.Status, artifact.Error,
		time.Now().UTC(), time.Now().UTC(),
	)
	return err
}

func (s *SQLiteStore) ListEnvironmentArtifacts(ctx context.Context, buildID string) ([]*EnvironmentArtifactRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, build_id, target, image_ref, digest, status, error, created_at, updated_at
		FROM environment_artifacts
		WHERE build_id = ?
		ORDER BY id ASC`, buildID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var artifacts []*EnvironmentArtifactRecord
	for rows.Next() {
		artifact := &EnvironmentArtifactRecord{}
		if err := rows.Scan(
			&artifact.ID, &artifact.BuildID, &artifact.Target, &artifact.ImageRef, &artifact.Digest,
			&artifact.Status, &artifact.Error, &artifact.CreatedAt, &artifact.UpdatedAt,
		); err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts, rows.Err()
}

// --- Registry Connections ---

func (s *SQLiteStore) SaveRegistryConnection(ctx context.Context, conn *RegistryConnectionRecord) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO registry_connections (id, owner_id, provider, username, secret_ref, is_default, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			owner_id = excluded.owner_id,
			provider = excluded.provider,
			username = excluded.username,
			secret_ref = excluded.secret_ref,
			is_default = excluded.is_default,
			updated_at = excluded.updated_at`,
		conn.ID, conn.OwnerID, conn.Provider, conn.Username, conn.SecretRef, conn.IsDefault,
		time.Now().UTC(), time.Now().UTC(),
	)
	return err
}

func (s *SQLiteStore) GetRegistryConnection(ctx context.Context, id string) (*RegistryConnectionRecord, error) {
	conn := &RegistryConnectionRecord{}
	err := s.db.QueryRowContext(ctx, `
		SELECT id, owner_id, provider, username, secret_ref, is_default, created_at, updated_at
		FROM registry_connections
		WHERE id = ?`, id,
	).Scan(&conn.ID, &conn.OwnerID, &conn.Provider, &conn.Username, &conn.SecretRef, &conn.IsDefault, &conn.CreatedAt, &conn.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("registry connection %q not found", id)
	}
	return conn, err
}

func (s *SQLiteStore) ListRegistryConnections(ctx context.Context, ownerID string) ([]*RegistryConnectionRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, owner_id, provider, username, secret_ref, is_default, created_at, updated_at
		FROM registry_connections
		WHERE owner_id = ?
		ORDER BY created_at DESC`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conns []*RegistryConnectionRecord
	for rows.Next() {
		conn := &RegistryConnectionRecord{}
		if err := rows.Scan(
			&conn.ID, &conn.OwnerID, &conn.Provider, &conn.Username, &conn.SecretRef, &conn.IsDefault, &conn.CreatedAt, &conn.UpdatedAt,
		); err != nil {
			return nil, err
		}
		conns = append(conns, conn)
	}
	return conns, rows.Err()
}

func (s *SQLiteStore) DeleteRegistryConnection(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM registry_connections WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("registry connection %q not found", id)
	}
	return nil
}

func nullableTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC()
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
