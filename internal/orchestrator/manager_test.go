package orchestrator

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/DohaerisAI/forgevm/internal/providers"
	"github.com/DohaerisAI/forgevm/internal/store"
	"github.com/rs/zerolog"
)

func setupManager(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	st, err := store.NewSQLiteStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	reg := providers.NewRegistry()
	mock := providers.NewMockProvider()
	reg.Register(mock)
	reg.SetDefault("mock")

	events := NewEventBus()
	logger := zerolog.Nop()

	m := NewManager(reg, st, events, logger, ManagerConfig{
		DefaultTTL:    5 * time.Minute,
		DefaultImage:  "alpine:latest",
		DefaultMemory: 512,
		DefaultVCPUs:  1,
	})
	m.Start()
	t.Cleanup(func() { m.Stop() })
	return m
}

func TestManager_SpawnAndGet(t *testing.T) {
	m := setupManager(t)
	ctx := context.Background()

	sb, err := m.Spawn(ctx, SpawnRequest{Image: "alpine:latest"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if sb.State != StateRunning {
		t.Fatalf("expected running, got %s", sb.State)
	}

	got, err := m.Get(ctx, sb.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != sb.ID {
		t.Fatalf("ID mismatch")
	}
}

func TestManager_List(t *testing.T) {
	m := setupManager(t)
	ctx := context.Background()

	m.Spawn(ctx, SpawnRequest{Image: "alpine:latest"})
	m.Spawn(ctx, SpawnRequest{Image: "ubuntu:latest"})

	list, err := m.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2, got %d", len(list))
	}
}

func TestManager_Exec(t *testing.T) {
	m := setupManager(t)
	ctx := context.Background()

	sb, _ := m.Spawn(ctx, SpawnRequest{Image: "alpine:latest"})

	result, err := m.Exec(ctx, sb.ID, ExecRequest{Command: "echo hello from manager"})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d", result.ExitCode)
	}
	if result.Stdout == "" {
		t.Fatal("expected stdout")
	}
}

func TestManager_WriteAndReadFile(t *testing.T) {
	m := setupManager(t)
	ctx := context.Background()

	sb, _ := m.Spawn(ctx, SpawnRequest{Image: "alpine:latest"})

	err := m.WriteFile(ctx, sb.ID, FileWriteRequest{
		Path:    "/workspace/greeting.txt",
		Content: "hello forgevm",
	})
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	data, err := m.ReadFile(ctx, sb.ID, "/workspace/greeting.txt")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "hello forgevm" {
		t.Fatalf("expected 'hello forgevm', got %q", string(data))
	}
}

func TestManager_Destroy(t *testing.T) {
	m := setupManager(t)
	ctx := context.Background()

	sb, _ := m.Spawn(ctx, SpawnRequest{Image: "alpine:latest"})

	err := m.Destroy(ctx, sb.ID)
	if err != nil {
		t.Fatalf("destroy: %v", err)
	}

	// Exec should fail on destroyed sandbox
	_, err = m.Exec(ctx, sb.ID, ExecRequest{Command: "echo"})
	if err == nil {
		t.Fatal("expected error after destroy")
	}
}

func TestManager_TTLExpiry(t *testing.T) {
	m := setupManager(t)
	ctx := context.Background()

	// Spawn with very short TTL
	sb, err := m.Spawn(ctx, SpawnRequest{Image: "alpine:latest", TTL: "1ms"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	// Wait for it to expire
	time.Sleep(10 * time.Millisecond)

	// Prune should clean it up
	count, err := m.Prune(ctx)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 pruned, got %d", count)
	}

	// Verify it's gone from listing
	list, _ := m.List(ctx)
	for _, s := range list {
		if s.ID == sb.ID {
			t.Fatal("expected sandbox to be pruned from list")
		}
	}
}

func TestManager_ExtendTTL(t *testing.T) {
	m := setupManager(t)
	ctx := context.Background()

	sb, err := m.Spawn(ctx, SpawnRequest{Image: "alpine:latest", TTL: "5m"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	originalExpiry := sb.ExpiresAt

	// Extend by 30 minutes
	updated, err := m.ExtendTTL(ctx, sb.ID, 30*time.Minute)
	if err != nil {
		t.Fatalf("extend: %v", err)
	}

	expectedExpiry := originalExpiry.Add(30 * time.Minute)
	if !updated.ExpiresAt.Equal(expectedExpiry) {
		t.Fatalf("expected expires_at %v, got %v", expectedExpiry, updated.ExpiresAt)
	}

	// Verify via Get
	got, _ := m.Get(ctx, sb.ID)
	if !got.ExpiresAt.Equal(expectedExpiry) {
		t.Fatalf("Get: expected expires_at %v, got %v", expectedExpiry, got.ExpiresAt)
	}
}

func TestManager_ExtendTTL_NotFound(t *testing.T) {
	m := setupManager(t)
	_, err := m.ExtendTTL(context.Background(), "sb-nope", 30*time.Minute)
	if err == nil {
		t.Fatal("expected error for nonexistent sandbox")
	}
}

func TestManager_ExtendTTL_Destroyed(t *testing.T) {
	m := setupManager(t)
	ctx := context.Background()

	sb, _ := m.Spawn(ctx, SpawnRequest{Image: "alpine:latest"})
	m.Destroy(ctx, sb.ID)

	_, err := m.ExtendTTL(ctx, sb.ID, 30*time.Minute)
	if err == nil {
		t.Fatal("expected error extending destroyed sandbox")
	}
}

func TestManager_StateTransitions(t *testing.T) {
	m := setupManager(t)
	ctx := context.Background()

	// Creating → Running
	sb, err := m.Spawn(ctx, SpawnRequest{Image: "alpine:latest"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if sb.State != StateRunning {
		t.Fatalf("expected running after spawn, got %s", sb.State)
	}

	// Running → Destroyed
	m.Destroy(ctx, sb.ID)
	rec, err := m.store.GetSandbox(ctx, sb.ID)
	if err != nil {
		t.Fatalf("get from store: %v", err)
	}
	if rec.State != string(StateDestroyed) {
		t.Fatalf("expected destroyed in store, got %s", rec.State)
	}
}
