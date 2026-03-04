package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testStore(t *testing.T) *SQLiteStore {
	t.Helper()
	dir := t.TempDir()
	s, err := NewSQLiteStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestMigrations(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// First open creates tables
	s1, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	s1.Close()

	// Second open is idempotent
	s2, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	s2.Close()

	// Verify file was created
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatal("database file not created")
	}
}

func TestSandboxCRUD(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	sb := &SandboxRecord{
		ID:        "sb-00000001",
		State:     "running",
		Provider:  "mock",
		Image:     "alpine:latest",
		MemoryMB:  512,
		VCPUs:     1,
		Metadata:  `{"env":"test"}`,
		CreatedAt: now,
		ExpiresAt: now.Add(30 * time.Minute),
		UpdatedAt: now,
	}

	// Create
	if err := s.CreateSandbox(ctx, sb); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Get
	got, err := s.GetSandbox(ctx, "sb-00000001")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.State != "running" {
		t.Fatalf("expected running, got %s", got.State)
	}
	if got.Image != "alpine:latest" {
		t.Fatalf("expected alpine:latest, got %s", got.Image)
	}

	// List
	list, err := s.ListSandboxes(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1, got %d", len(list))
	}

	// Update state
	if err := s.UpdateSandboxState(ctx, "sb-00000001", "idle"); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ = s.GetSandbox(ctx, "sb-00000001")
	if got.State != "idle" {
		t.Fatalf("expected idle, got %s", got.State)
	}

	// Delete (sets state to destroyed)
	if err := s.DeleteSandbox(ctx, "sb-00000001"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	list, _ = s.ListSandboxes(ctx)
	if len(list) != 0 {
		t.Fatalf("expected 0 after delete, got %d", len(list))
	}
}

func TestExecLogs(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// Need a sandbox first
	sb := &SandboxRecord{
		ID: "sb-00000002", State: "running", Provider: "mock",
		CreatedAt: now, ExpiresAt: now.Add(time.Hour), UpdatedAt: now,
	}
	s.CreateSandbox(ctx, sb)

	log := &ExecLogRecord{
		SandboxID: "sb-00000002",
		Command:   "echo hello",
		ExitCode:  0,
		Stdout:    "hello\n",
		Stderr:    "",
		Duration:  "5ms",
		CreatedAt: now,
	}

	if err := s.CreateExecLog(ctx, log); err != nil {
		t.Fatalf("create exec log: %v", err)
	}

	logs, err := s.ListExecLogs(ctx, "sb-00000002")
	if err != nil {
		t.Fatalf("list exec logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}
	if logs[0].Command != "echo hello" {
		t.Fatalf("expected 'echo hello', got %q", logs[0].Command)
	}
}

func TestProviderConfigs(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	cfg := &ProviderConfigRecord{
		Name:    "mock",
		Config:  `{"enabled": true}`,
		Enabled: true,
	}

	if err := s.SaveProviderConfig(ctx, cfg); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := s.GetProviderConfig(ctx, "mock")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !got.Enabled {
		t.Fatal("expected enabled")
	}

	// Upsert
	cfg.Config = `{"enabled": true, "fast": true}`
	if err := s.SaveProviderConfig(ctx, cfg); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	list, err := s.ListProviderConfigs(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1, got %d", len(list))
	}
}

func TestExpiredSandboxes(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// One expired, one not
	s.CreateSandbox(ctx, &SandboxRecord{
		ID: "sb-expired", State: "running", Provider: "mock",
		CreatedAt: now.Add(-time.Hour), ExpiresAt: now.Add(-time.Minute), UpdatedAt: now,
	})
	s.CreateSandbox(ctx, &SandboxRecord{
		ID: "sb-active", State: "running", Provider: "mock",
		CreatedAt: now, ExpiresAt: now.Add(time.Hour), UpdatedAt: now,
	})

	expired, err := s.ListExpiredSandboxes(ctx, now)
	if err != nil {
		t.Fatalf("list expired: %v", err)
	}
	if len(expired) != 1 {
		t.Fatalf("expected 1 expired, got %d", len(expired))
	}
	if expired[0].ID != "sb-expired" {
		t.Fatalf("expected sb-expired, got %s", expired[0].ID)
	}
}

func TestUpdateSandboxExpiresAt(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	sb := &SandboxRecord{
		ID: "sb-extend01", State: "running", Provider: "mock",
		CreatedAt: now, ExpiresAt: now.Add(30 * time.Minute), UpdatedAt: now,
	}
	s.CreateSandbox(ctx, sb)

	// Extend by 30 minutes
	newExpiry := sb.ExpiresAt.Add(30 * time.Minute)
	if err := s.UpdateSandboxExpiresAt(ctx, "sb-extend01", newExpiry); err != nil {
		t.Fatalf("update expires_at: %v", err)
	}

	got, _ := s.GetSandbox(ctx, "sb-extend01")
	// Compare truncated to seconds since SQLite doesn't store sub-second
	if got.ExpiresAt.Truncate(time.Second) != newExpiry.Truncate(time.Second) {
		t.Fatalf("expected expires_at %v, got %v", newExpiry, got.ExpiresAt)
	}
}

func TestUpdateSandboxExpiresAt_Destroyed(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	sb := &SandboxRecord{
		ID: "sb-extend02", State: "destroyed", Provider: "mock",
		CreatedAt: now, ExpiresAt: now.Add(30 * time.Minute), UpdatedAt: now,
	}
	s.CreateSandbox(ctx, sb)

	err := s.UpdateSandboxExpiresAt(ctx, "sb-extend02", now.Add(time.Hour))
	if err == nil {
		t.Fatal("expected error extending destroyed sandbox")
	}
}

func TestUpdateSandboxExpiresAt_NotFound(t *testing.T) {
	s := testStore(t)
	err := s.UpdateSandboxExpiresAt(context.Background(), "sb-nope", time.Now().Add(time.Hour))
	if err == nil {
		t.Fatal("expected error for nonexistent sandbox")
	}
}

func TestGetSandboxNotFound(t *testing.T) {
	s := testStore(t)
	_, err := s.GetSandbox(context.Background(), "sb-nope")
	if err == nil {
		t.Fatal("expected error for nonexistent sandbox")
	}
}

func TestEnvironmentSpecCRUD(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	spec := &EnvironmentSpecRecord{
		ID:             "envspec-1",
		OwnerID:        "user-1",
		Name:           "py-ds",
		BaseImage:      "python:3.12-slim",
		PythonPackages: `["pandas","numpy"]`,
		AptPackages:    `["curl"]`,
		PythonVersion:  "3.12",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := s.CreateEnvironmentSpec(ctx, spec); err != nil {
		t.Fatalf("create spec: %v", err)
	}

	got, err := s.GetEnvironmentSpec(ctx, spec.ID)
	if err != nil {
		t.Fatalf("get spec: %v", err)
	}
	if got.BaseImage != "python:3.12-slim" {
		t.Fatalf("base image mismatch: %s", got.BaseImage)
	}

	spec.Name = "py-ds-v2"
	spec.PythonPackages = `["pandas","numpy","matplotlib"]`
	if err := s.UpdateEnvironmentSpec(ctx, spec); err != nil {
		t.Fatalf("update spec: %v", err)
	}

	list, err := s.ListEnvironmentSpecs(ctx, "user-1")
	if err != nil {
		t.Fatalf("list specs: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(list))
	}

	if err := s.DeleteEnvironmentSpec(ctx, spec.ID); err != nil {
		t.Fatalf("delete spec: %v", err)
	}
	if _, err := s.GetEnvironmentSpec(ctx, spec.ID); err == nil {
		t.Fatal("expected get to fail after delete")
	}
}

func TestEnvironmentBuildArtifactAndRegistryCRUD(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	spec := &EnvironmentSpecRecord{
		ID:             "envspec-2",
		OwnerID:        "user-2",
		Name:           "py-tools",
		BaseImage:      "python:3.12-slim",
		PythonPackages: `[]`,
		AptPackages:    `[]`,
		PythonVersion:  "3.12",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.CreateEnvironmentSpec(ctx, spec); err != nil {
		t.Fatalf("create spec: %v", err)
	}

	build := &EnvironmentBuildRecord{
		ID:             "envbuild-1",
		SpecID:         spec.ID,
		Status:         "queued",
		CurrentStep:    "validate_spec",
		LogBlob:        "",
		ImageSizeBytes: 0,
		DigestLocal:    "",
		Error:          "",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.CreateEnvironmentBuild(ctx, build); err != nil {
		t.Fatalf("create build: %v", err)
	}

	build.Status = "ready"
	build.CurrentStep = "finalize"
	finished := time.Now().UTC()
	build.FinishedAt = &finished
	build.DigestLocal = "sha256:abc123"
	if err := s.UpdateEnvironmentBuild(ctx, build); err != nil {
		t.Fatalf("update build: %v", err)
	}

	gb, err := s.GetEnvironmentBuild(ctx, build.ID)
	if err != nil {
		t.Fatalf("get build: %v", err)
	}
	if gb.Status != "ready" {
		t.Fatalf("expected ready build status, got %s", gb.Status)
	}

	artifact := &EnvironmentArtifactRecord{
		BuildID:  build.ID,
		Target:   "local",
		ImageRef: "local/env:1",
		Digest:   "sha256:def456",
		Status:   "ready",
		Error:    "",
	}
	if err := s.SaveEnvironmentArtifact(ctx, artifact); err != nil {
		t.Fatalf("save artifact: %v", err)
	}

	artifacts, err := s.ListEnvironmentArtifacts(ctx, build.ID)
	if err != nil {
		t.Fatalf("list artifacts: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}

	conn := &RegistryConnectionRecord{
		ID:        "reg-1",
		OwnerID:   "user-2",
		Provider:  "ghcr",
		Username:  "octocat",
		SecretRef: "secret://ghcr/user-2/default",
		IsDefault: true,
	}
	if err := s.SaveRegistryConnection(ctx, conn); err != nil {
		t.Fatalf("save registry connection: %v", err)
	}

	conns, err := s.ListRegistryConnections(ctx, "user-2")
	if err != nil {
		t.Fatalf("list registry connections: %v", err)
	}
	if len(conns) != 1 {
		t.Fatalf("expected 1 registry connection, got %d", len(conns))
	}

	if err := s.DeleteRegistryConnection(ctx, conn.ID); err != nil {
		t.Fatalf("delete registry connection: %v", err)
	}
}
