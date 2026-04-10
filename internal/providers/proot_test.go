package providers

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func testPRootLogger() zerolog.Logger {
	return zerolog.New(zerolog.NewTestWriter(nil)).Level(zerolog.Disabled)
}

func testPRootProvider(t *testing.T) *PRootProvider {
	t.Helper()
	tmpDir := t.TempDir()
	return NewPRootProvider(PRootProviderConfig{
		RootfsPath:     tmpDir, // dummy rootfs for unit tests
		PRootBinary:    "/nonexistent/proot",
		WorkspaceBase:  filepath.Join(tmpDir, "workspaces"),
		DefaultTimeout: 30 * time.Second,
		MaxSandboxes:   3,
	}, testPRootLogger())
}

// skipIfNoPRoot skips integration tests when proot is not available.
func skipIfNoPRoot(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("proot")
	if err != nil {
		t.Skip("proot binary not found, skipping integration test")
	}
	return path
}

// --- Unit Tests (no proot binary needed) ---

func TestPRootProvider_Name(t *testing.T) {
	p := testPRootProvider(t)
	if p.Name() != "proot" {
		t.Fatalf("expected name 'proot', got %q", p.Name())
	}
}

func TestPRootProvider_Healthy_NoBinary(t *testing.T) {
	p := testPRootProvider(t)
	// Binary is /nonexistent/proot, so should be unhealthy
	if p.Healthy(context.Background()) {
		t.Fatal("expected unhealthy when proot binary missing")
	}
}

func TestPRootProvider_BuildCommand(t *testing.T) {
	p := testPRootProvider(t)
	p.config.PRootBinary = "/usr/bin/proot"
	p.config.RootfsPath = "/opt/rootfs"

	sb := &prootSandbox{
		id:        "sb-test1234",
		workspace: "/tmp/test-workspace",
	}

	cmd := p.BuildCommand(context.Background(), sb, ExecOptions{
		Command: "echo hello",
		Env:     map[string]string{"FOO": "bar"},
	})

	// Verify binary
	if cmd.Path != "/usr/bin/proot" && !strings.HasSuffix(cmd.Path, "proot") {
		t.Fatalf("expected proot binary, got %q", cmd.Path)
	}

	args := strings.Join(cmd.Args, " ")

	// Verify key flags
	if !strings.Contains(args, "-0") {
		t.Fatal("expected -0 (fake root) flag")
	}
	if !strings.Contains(args, "-r /opt/rootfs") {
		t.Fatal("expected -r rootfs flag")
	}
	if !strings.Contains(args, "-b /tmp/test-workspace/workspace:/workspace") {
		t.Fatalf("expected workspace bind mount, got args: %s", args)
	}
	if !strings.Contains(args, "-w /workspace") {
		t.Fatal("expected -w /workspace flag")
	}
	if !strings.Contains(args, "echo hello") {
		t.Fatal("expected command in args")
	}

	// Verify env includes custom var
	found := false
	for _, e := range cmd.Env {
		if e == "FOO=bar" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected FOO=bar in environment")
	}
}

func TestPRootProvider_BuildCommand_WorkDir(t *testing.T) {
	p := testPRootProvider(t)
	sb := &prootSandbox{id: "sb-test", workspace: "/tmp/ws"}

	cmd := p.BuildCommand(context.Background(), sb, ExecOptions{
		Command: "ls",
		WorkDir: "/app",
	})

	args := strings.Join(cmd.Args, " ")
	if !strings.Contains(args, "-w /app") {
		t.Fatalf("expected -w /app, got args: %s", args)
	}
}

func TestPRootProvider_MaxSandboxes(t *testing.T) {
	p := testPRootProvider(t)
	ctx := context.Background()

	// Spawn up to max (3)
	ids := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		id, err := p.Spawn(ctx, SpawnOptions{})
		if err != nil {
			t.Fatalf("spawn %d: %v", i, err)
		}
		ids = append(ids, id)
	}

	// 4th should fail
	_, err := p.Spawn(ctx, SpawnOptions{})
	if err == nil {
		t.Fatal("expected error when exceeding max sandboxes")
	}
	if !strings.Contains(err.Error(), "max sandboxes") {
		t.Fatalf("unexpected error: %v", err)
	}

	// Cleanup
	for _, id := range ids {
		p.Destroy(ctx, id)
	}
}

func TestPRootProvider_PathTraversal(t *testing.T) {
	p := testPRootProvider(t)
	ctx := context.Background()

	id, err := p.Spawn(ctx, SpawnOptions{})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer p.Destroy(ctx, id)

	// Try path traversal via WriteFile
	err = p.WriteFile(ctx, id, "../../etc/passwd", bytes.NewReader([]byte("hacked")), "0644")
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
	if !strings.Contains(err.Error(), "escapes sandbox") {
		t.Fatalf("unexpected error: %v", err)
	}

	// Try path traversal via ReadFile
	_, err = p.ReadFile(ctx, id, "../../../etc/shadow")
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
}

func TestPRootProvider_DestroyedSandboxRejects(t *testing.T) {
	p := testPRootProvider(t)
	ctx := context.Background()

	id, err := p.Spawn(ctx, SpawnOptions{})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	if err := p.Destroy(ctx, id); err != nil {
		t.Fatalf("destroy: %v", err)
	}

	// All operations should fail on destroyed sandbox
	_, err = p.Exec(ctx, id, ExecOptions{Command: "echo"})
	if err == nil {
		t.Fatal("expected error on destroyed sandbox exec")
	}

	err = p.WriteFile(ctx, id, "/workspace/test.txt", bytes.NewReader([]byte("data")), "0644")
	if err == nil {
		t.Fatal("expected error on destroyed sandbox write")
	}

	_, err = p.ReadFile(ctx, id, "/workspace/test.txt")
	if err == nil {
		t.Fatal("expected error on destroyed sandbox read")
	}

	_, err = p.ListFiles(ctx, id, "/workspace")
	if err == nil {
		t.Fatal("expected error on destroyed sandbox list")
	}
}

func TestPRootProvider_SpawnDestroy(t *testing.T) {
	p := testPRootProvider(t)
	ctx := context.Background()

	id, err := p.Spawn(ctx, SpawnOptions{Image: "alpine:latest"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if !strings.HasPrefix(id, "sb-") {
		t.Fatalf("expected id prefix 'sb-', got %q", id)
	}

	// Workspace dir should exist
	sb, _ := p.getSandbox(id)
	if _, err := os.Stat(filepath.Join(sb.workspace, "workspace")); err != nil {
		t.Fatalf("workspace dir should exist: %v", err)
	}

	// Status should be running
	status, err := p.Status(ctx, id)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.State != "running" {
		t.Fatalf("expected state 'running', got %q", status.State)
	}

	// Destroy
	if err := p.Destroy(ctx, id); err != nil {
		t.Fatalf("destroy: %v", err)
	}

	// Workspace dir should be cleaned up
	if _, err := os.Stat(sb.workspace); !os.IsNotExist(err) {
		t.Fatal("workspace should be removed after destroy")
	}
}

func TestPRootProvider_WriteReadFile(t *testing.T) {
	p := testPRootProvider(t)
	ctx := context.Background()

	id, err := p.Spawn(ctx, SpawnOptions{})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer p.Destroy(ctx, id)

	content := "hello from proot sandbox"
	err = p.WriteFile(ctx, id, "/workspace/test.txt", bytes.NewReader([]byte(content)), "0644")
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	rc, err := p.ReadFile(ctx, id, "/workspace/test.txt")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read all: %v", err)
	}
	if string(data) != content {
		t.Fatalf("expected %q, got %q", content, string(data))
	}
}

func TestPRootProvider_ListFiles(t *testing.T) {
	p := testPRootProvider(t)
	ctx := context.Background()

	id, err := p.Spawn(ctx, SpawnOptions{})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer p.Destroy(ctx, id)

	_ = p.WriteFile(ctx, id, "/workspace/a.txt", bytes.NewReader([]byte("a")), "0644")
	_ = p.WriteFile(ctx, id, "/workspace/b.txt", bytes.NewReader([]byte("b")), "0644")

	files, err := p.ListFiles(ctx, id, "/workspace")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
}

func TestPRootProvider_DeleteFile(t *testing.T) {
	p := testPRootProvider(t)
	ctx := context.Background()

	id, err := p.Spawn(ctx, SpawnOptions{})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer p.Destroy(ctx, id)

	_ = p.WriteFile(ctx, id, "/workspace/delete-me.txt", bytes.NewReader([]byte("bye")), "0644")

	err = p.DeleteFile(ctx, id, "/workspace/delete-me.txt", false)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err = p.ReadFile(ctx, id, "/workspace/delete-me.txt")
	if err == nil {
		t.Fatal("expected error reading deleted file")
	}
}

func TestPRootProvider_MoveFile(t *testing.T) {
	p := testPRootProvider(t)
	ctx := context.Background()

	id, err := p.Spawn(ctx, SpawnOptions{})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer p.Destroy(ctx, id)

	_ = p.WriteFile(ctx, id, "/workspace/old.txt", bytes.NewReader([]byte("data")), "0644")

	err = p.MoveFile(ctx, id, "/workspace/old.txt", "/workspace/new.txt")
	if err != nil {
		t.Fatalf("move: %v", err)
	}

	// Old path should not exist
	_, err = p.ReadFile(ctx, id, "/workspace/old.txt")
	if err == nil {
		t.Fatal("old path should not exist after move")
	}

	// New path should exist with correct content
	rc, err := p.ReadFile(ctx, id, "/workspace/new.txt")
	if err != nil {
		t.Fatalf("read new: %v", err)
	}
	defer rc.Close()
	data, _ := io.ReadAll(rc)
	if string(data) != "data" {
		t.Fatalf("expected 'data', got %q", string(data))
	}
}

func TestPRootProvider_ChmodFile(t *testing.T) {
	p := testPRootProvider(t)
	ctx := context.Background()

	id, err := p.Spawn(ctx, SpawnOptions{})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer p.Destroy(ctx, id)

	_ = p.WriteFile(ctx, id, "/workspace/script.sh", bytes.NewReader([]byte("#!/bin/sh")), "0644")

	err = p.ChmodFile(ctx, id, "/workspace/script.sh", "0755")
	if err != nil {
		t.Fatalf("chmod: %v", err)
	}

	info, err := p.StatFile(ctx, id, "/workspace/script.sh")
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	// Mode should contain 755
	if !strings.Contains(info.Mode, "755") {
		t.Fatalf("expected mode containing 755, got %q", info.Mode)
	}
}

func TestPRootProvider_StatFile(t *testing.T) {
	p := testPRootProvider(t)
	ctx := context.Background()

	id, err := p.Spawn(ctx, SpawnOptions{})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer p.Destroy(ctx, id)

	content := "stat test content"
	_ = p.WriteFile(ctx, id, "/workspace/stat.txt", bytes.NewReader([]byte(content)), "0644")

	info, err := p.StatFile(ctx, id, "/workspace/stat.txt")
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size != int64(len(content)) {
		t.Fatalf("expected size %d, got %d", len(content), info.Size)
	}
	if info.IsDir {
		t.Fatal("expected file, got dir")
	}
	if info.Path != "/workspace/stat.txt" {
		t.Fatalf("expected path '/workspace/stat.txt', got %q", info.Path)
	}
}

func TestPRootProvider_GlobFiles(t *testing.T) {
	p := testPRootProvider(t)
	ctx := context.Background()

	id, err := p.Spawn(ctx, SpawnOptions{})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer p.Destroy(ctx, id)

	_ = p.WriteFile(ctx, id, "/workspace/foo.txt", bytes.NewReader([]byte("f")), "0644")
	_ = p.WriteFile(ctx, id, "/workspace/bar.txt", bytes.NewReader([]byte("b")), "0644")
	_ = p.WriteFile(ctx, id, "/workspace/baz.log", bytes.NewReader([]byte("l")), "0644")

	matches, err := p.GlobFiles(ctx, id, "/workspace/*.txt")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("expected 2 .txt matches, got %d: %v", len(matches), matches)
	}
}

func TestPRootProvider_ConsoleLog(t *testing.T) {
	p := testPRootProvider(t)
	ctx := context.Background()

	id, err := p.Spawn(ctx, SpawnOptions{})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer p.Destroy(ctx, id)

	logs, err := p.ConsoleLog(ctx, id, 0)
	if err != nil {
		t.Fatalf("console log: %v", err)
	}
	if len(logs) == 0 {
		t.Fatal("expected non-empty console log")
	}
	if !strings.Contains(logs[0], "proot sandbox") {
		t.Fatalf("expected proot sandbox reference, got %q", logs[0])
	}
}

func TestPRootProvider_ConsoleLog_LimitLines(t *testing.T) {
	p := testPRootProvider(t)
	ctx := context.Background()

	id, err := p.Spawn(ctx, SpawnOptions{})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer p.Destroy(ctx, id)

	logs, err := p.ConsoleLog(ctx, id, 2)
	if err != nil {
		t.Fatalf("console log: %v", err)
	}
	if len(logs) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(logs))
	}
}

// --- Integration Tests (require proot binary) ---

func TestPRootProvider_Integration_Exec(t *testing.T) {
	prootPath := skipIfNoPRoot(t)
	tmpDir := t.TempDir()

	p := NewPRootProvider(PRootProviderConfig{
		RootfsPath:     "/", // use host rootfs for integration tests
		PRootBinary:    prootPath,
		WorkspaceBase:  filepath.Join(tmpDir, "workspaces"),
		DefaultTimeout: 30 * time.Second,
		MaxSandboxes:   5,
	}, testPRootLogger())

	ctx := context.Background()

	id, err := p.Spawn(ctx, SpawnOptions{})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer p.Destroy(ctx, id)

	// Basic echo
	result, err := p.Exec(ctx, id, ExecOptions{Command: "echo hello proot"})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", result.ExitCode, result.Stderr)
	}
	if strings.TrimSpace(result.Stdout) != "hello proot" {
		t.Fatalf("expected 'hello proot', got %q", result.Stdout)
	}

	// Non-zero exit
	result, err = p.Exec(ctx, id, ExecOptions{Command: "exit 42"})
	if err != nil {
		t.Fatalf("exec non-zero: %v", err)
	}
	if result.ExitCode != 42 {
		t.Fatalf("expected exit 42, got %d", result.ExitCode)
	}

	// Environment variables
	result, err = p.Exec(ctx, id, ExecOptions{
		Command: "echo $MY_VAR",
		Env:     map[string]string{"MY_VAR": "proot_test"},
	})
	if err != nil {
		t.Fatalf("exec env: %v", err)
	}
	if strings.TrimSpace(result.Stdout) != "proot_test" {
		t.Fatalf("expected 'proot_test', got %q", result.Stdout)
	}
}

func TestPRootProvider_Integration_ExecStream(t *testing.T) {
	prootPath := skipIfNoPRoot(t)
	tmpDir := t.TempDir()

	p := NewPRootProvider(PRootProviderConfig{
		RootfsPath:     "/",
		PRootBinary:    prootPath,
		WorkspaceBase:  filepath.Join(tmpDir, "workspaces"),
		DefaultTimeout: 30 * time.Second,
		MaxSandboxes:   5,
	}, testPRootLogger())

	ctx := context.Background()

	id, err := p.Spawn(ctx, SpawnOptions{})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer p.Destroy(ctx, id)

	ch, err := p.ExecStream(ctx, id, ExecOptions{Command: "echo streaming from proot"})
	if err != nil {
		t.Fatalf("exec stream: %v", err)
	}

	var output strings.Builder
	for chunk := range ch {
		output.WriteString(chunk.Data)
	}
	if !strings.Contains(output.String(), "streaming from proot") {
		t.Fatalf("expected 'streaming from proot' in output, got %q", output.String())
	}
}
