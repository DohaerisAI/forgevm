package environments

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DohaerisAI/forgevm/internal/store"
	"github.com/rs/zerolog"
)

func testStore(t *testing.T) *store.SQLiteStore {
	t.Helper()
	dir := t.TempDir()
	s, err := store.NewSQLiteStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestManagerBuildLocalReady(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	spec := &store.EnvironmentSpecRecord{
		ID:             "spec-1",
		OwnerID:        "user-1",
		Name:           "py-tools",
		BaseImage:      "python:3.12-slim",
		PythonPackages: `["pandas"]`,
		AptPackages:    `[]`,
		PythonVersion:  "3.12",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := st.CreateEnvironmentSpec(ctx, spec); err != nil {
		t.Fatalf("create spec: %v", err)
	}

	build := &store.EnvironmentBuildRecord{
		ID:          "build-1",
		SpecID:      spec.ID,
		Status:      BuildStatusQueued,
		CurrentStep: "queued",
		LogBlob:     "build queued",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := st.CreateEnvironmentBuild(ctx, build); err != nil {
		t.Fatalf("create build: %v", err)
	}
	if err := st.SaveEnvironmentArtifact(ctx, &store.EnvironmentArtifactRecord{
		BuildID:  build.ID,
		Target:   "local",
		ImageRef: "local/forgevm-env:test",
		Status:   "pending",
	}); err != nil {
		t.Fatalf("save artifact: %v", err)
	}

	r := &fakeRunner{
		inspectID: "sha256:local123",
	}
	m := NewManagerWithRunner(st, zerolog.Nop(), r)
	m.Start(1)
	defer m.Stop()

	if err := m.Enqueue(build.ID); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	waitForBuildStatus(t, st, build.ID, BuildStatusReady)
}

func TestManagerBuildPartialFailure(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	spec := &store.EnvironmentSpecRecord{
		ID:             "spec-2",
		OwnerID:        "user-1",
		Name:           "py-tools-2",
		BaseImage:      "python:3.12-slim",
		PythonPackages: `[]`,
		AptPackages:    `[]`,
		PythonVersion:  "3.12",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := st.CreateEnvironmentSpec(ctx, spec); err != nil {
		t.Fatalf("create spec: %v", err)
	}

	build := &store.EnvironmentBuildRecord{
		ID:          "build-2",
		SpecID:      spec.ID,
		Status:      BuildStatusQueued,
		CurrentStep: "queued",
		LogBlob:     "build queued",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := st.CreateEnvironmentBuild(ctx, build); err != nil {
		t.Fatalf("create build: %v", err)
	}
	if err := st.SaveEnvironmentArtifact(ctx, &store.EnvironmentArtifactRecord{
		BuildID:  build.ID,
		Target:   "ghcr",
		ImageRef: "ghcr.io/user/env:test",
		Status:   "pending",
	}); err != nil {
		t.Fatalf("save artifact: %v", err)
	}

	r := &fakeRunner{
		inspectID: "sha256:local123",
	}
	m := NewManagerWithRunner(st, zerolog.Nop(), r)
	m.Start(1)
	defer m.Stop()

	if err := m.Enqueue(build.ID); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	waitForBuildStatus(t, st, build.ID, BuildStatusFailed)
}

func TestManagerBuildGHCRReady(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	spec := &store.EnvironmentSpecRecord{
		ID:             "spec-3",
		OwnerID:        "user-3",
		Name:           "py-tools-3",
		BaseImage:      "python:3.12-slim",
		PythonPackages: `["requests"]`,
		AptPackages:    `[]`,
		PythonVersion:  "3.12",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := st.CreateEnvironmentSpec(ctx, spec); err != nil {
		t.Fatalf("create spec: %v", err)
	}

	build := &store.EnvironmentBuildRecord{
		ID:          "build-3",
		SpecID:      spec.ID,
		Status:      BuildStatusQueued,
		CurrentStep: "queued",
		LogBlob:     "build queued",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := st.CreateEnvironmentBuild(ctx, build); err != nil {
		t.Fatalf("create build: %v", err)
	}
	if err := st.SaveEnvironmentArtifact(ctx, &store.EnvironmentArtifactRecord{
		BuildID:  build.ID,
		Target:   "ghcr",
		ImageRef: "ghcr.io/user-3/forgevm-env:test",
		Status:   "pending",
	}); err != nil {
		t.Fatalf("save artifact: %v", err)
	}
	if err := st.SaveRegistryConnection(ctx, &store.RegistryConnectionRecord{
		ID:        "reg-1",
		OwnerID:   "user-3",
		Provider:  "ghcr",
		Username:  "octocat",
		SecretRef: "token123",
		IsDefault: true,
	}); err != nil {
		t.Fatalf("save registry: %v", err)
	}

	r := &fakeRunner{
		inspectID: "sha256:img123",
	}
	m := NewManagerWithRunner(st, zerolog.Nop(), r)
	m.Start(1)
	defer m.Stop()

	if err := m.Enqueue(build.ID); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	waitForBuildStatus(t, st, build.ID, BuildStatusReady)
}

func waitForBuildStatus(t *testing.T, st *store.SQLiteStore, buildID, want string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		build, err := st.GetEnvironmentBuild(context.Background(), buildID)
		if err == nil && build.Status == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	build, _ := st.GetEnvironmentBuild(context.Background(), buildID)
	if build == nil {
		t.Fatalf("build %s not found while waiting for status %s", buildID, want)
	}
	t.Fatalf("timeout waiting for build %s status %s; got %s (step=%s, err=%s)", buildID, want, build.Status, build.CurrentStep, build.Error)
}

type fakeRunner struct {
	mu        sync.Mutex
	inspectID string
}

func (f *fakeRunner) Run(ctx context.Context, stdin string, name string, args ...string) (string, error) {
	_ = ctx
	f.mu.Lock()
	defer f.mu.Unlock()

	if name != "docker" {
		return "", nil
	}
	if len(args) >= 2 && args[0] == "image" && args[1] == "inspect" {
		return f.inspectID + "\n", nil
	}
	if len(args) > 0 && args[0] == "login" {
		if stdin == "" {
			return "", fmt.Errorf("missing stdin token")
		}
		return "Login Succeeded\n", nil
	}
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "push") || strings.Contains(joined, "tag") || strings.Contains(joined, "build") {
		return "ok\n", nil
	}
	return "ok\n", nil
}
