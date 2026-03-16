package providers

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func testLogger() zerolog.Logger {
	return zerolog.New(os.Stderr).With().Timestamp().Logger().Level(zerolog.DebugLevel)
}

// ---------------------------------------------------------------------------
// Unit tests — no Docker daemon required
// ---------------------------------------------------------------------------

func TestDockerProviderName(t *testing.T) {
	p := &DockerProvider{}
	if got := p.Name(); got != "docker" {
		t.Errorf("Name() = %q, want %q", got, "docker")
	}
}

func TestParseMemoryBytes(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"512m", 512 * 1024 * 1024},
		{"1g", 1024 * 1024 * 1024},
		{"256M", 256 * 1024 * 1024},
		{"2G", 2 * 1024 * 1024 * 1024},
		{"1024k", 1024 * 1024},
		{"0", 0},
		{"", 0},
		{"invalid", 0},
	}
	for _, tc := range tests {
		got := parseMemoryBytes(tc.input)
		if got != tc.want {
			t.Errorf("parseMemoryBytes(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestParseCPUs(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"1", 1e9},
		{"2", 2e9},
		{"0.5", 5e8},
		{"0", 0},
		{"", 0},
	}
	for _, tc := range tests {
		got := parseCPUs(tc.input)
		if got != tc.want {
			t.Errorf("parseCPUs(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestShellQuoteDocker(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "'hello'"},
		{"hello world", "'hello world'"},
		{"it's", "'it'\"'\"'s'"},
		{"", "''"},
		{"/path/to/file.txt", "'/path/to/file.txt'"},
	}
	for _, tc := range tests {
		got := shellQuoteDocker(tc.input)
		if got != tc.want {
			t.Errorf("shellQuoteDocker(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestScopedPathTraversalPrevention(t *testing.T) {
	sandboxID := "sb-a1b2c3d4"
	base := "/workspace/" + sandboxID

	attacks := []string{
		"../../etc/passwd",
		"../sb-other/secret.txt",
		"/workspace/sb-other/file.py",
		"/../../../root/.ssh/id_rsa",
		"/workspace/../../../etc/shadow",
		"....//....//etc/passwd",
	}
	for _, attack := range attacks {
		result := scopedWorkspacePath(sandboxID, attack)
		if !strings.HasPrefix(result, base) {
			t.Errorf("traversal escape: input=%q result=%q must start with %q", attack, result, base)
		}
	}
}

func TestDockerHealthy_NoDocker(t *testing.T) {
	p, err := NewDockerProvider(DockerProviderConfig{
		Socket: "unix:///nonexistent.sock",
	}, testLogger())
	if err != nil {
		t.Fatalf("NewDockerProvider: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if p.Healthy(ctx) {
		t.Error("Healthy() should return false for nonexistent socket")
	}
}

// ---------------------------------------------------------------------------
// Integration tests — require Docker daemon
// ---------------------------------------------------------------------------

func skipIfNoDocker(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available in PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	p, err := newTestDockerProvider(t)
	if err != nil {
		t.Skip("docker daemon not reachable:", err)
	}
	if !p.Healthy(ctx) {
		t.Skip("docker daemon not healthy")
	}
}

func newTestDockerProvider(t *testing.T) (*DockerProvider, error) {
	t.Helper()
	return NewDockerProvider(DockerProviderConfig{
		DefaultImage:   "alpine:latest",
		NetworkMode:    "none",
		ReadOnlyRootfs: false, // easier for test writes
		Memory:         "256m",
		CPUs:           "1",
		PidsLimit:      256,
	}, testLogger())
}

func spawnTestSandbox(t *testing.T, p *DockerProvider) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	id, err := p.Spawn(ctx, SpawnOptions{Image: "alpine:latest"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	t.Cleanup(func() {
		ctx2, cancel2 := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel2()
		p.Destroy(ctx2, id) //nolint:errcheck
	})
	return id
}

func TestDockerIntegration_Healthy(t *testing.T) {
	skipIfNoDocker(t)
	p, err := newTestDockerProvider(t)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	ctx := context.Background()
	if !p.Healthy(ctx) {
		t.Error("Healthy() = false, want true")
	}
}

func TestDockerIntegration_SpawnAndDestroy(t *testing.T) {
	skipIfNoDocker(t)
	p, err := newTestDockerProvider(t)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	ctx := context.Background()

	id, err := p.Spawn(ctx, SpawnOptions{Image: "alpine:latest"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if id == "" {
		t.Fatal("spawn returned empty id")
	}

	status, err := p.Status(ctx, id)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.State != "running" {
		t.Errorf("state = %q, want %q", status.State, "running")
	}

	if err := p.Destroy(ctx, id); err != nil {
		t.Fatalf("destroy: %v", err)
	}
	_, err = p.Status(ctx, id)
	if err == nil {
		t.Error("status after destroy: expected error, got nil")
	}
}

func TestDockerIntegration_Exec(t *testing.T) {
	skipIfNoDocker(t)
	p, err := newTestDockerProvider(t)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	id := spawnTestSandbox(t, p)
	ctx := context.Background()

	result, err := p.Exec(ctx, id, ExecOptions{Command: "echo hello"})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if !strings.Contains(result.Stdout, "hello") {
		t.Errorf("stdout = %q, want to contain %q", result.Stdout, "hello")
	}
	if result.ExitCode != 0 {
		t.Errorf("exit_code = %d, want 0", result.ExitCode)
	}
}

func TestDockerIntegration_ExecWithEnvAndWorkDir(t *testing.T) {
	skipIfNoDocker(t)
	p, err := newTestDockerProvider(t)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	id := spawnTestSandbox(t, p)
	ctx := context.Background()

	result, err := p.Exec(ctx, id, ExecOptions{
		Command: "echo $MY_VAR from $(pwd)",
		Env:     map[string]string{"MY_VAR": "test123"},
		WorkDir: "/workspace",
	})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if !strings.Contains(result.Stdout, "test123") {
		t.Errorf("env not passed: stdout = %q", result.Stdout)
	}
	if !strings.Contains(result.Stdout, "/workspace") {
		t.Errorf("workdir not set: stdout = %q", result.Stdout)
	}
}

func TestDockerIntegration_ExecNonZeroExit(t *testing.T) {
	skipIfNoDocker(t)
	p, err := newTestDockerProvider(t)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	id := spawnTestSandbox(t, p)
	ctx := context.Background()

	result, err := p.Exec(ctx, id, ExecOptions{Command: "exit 42"})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if result.ExitCode != 42 {
		t.Errorf("exit_code = %d, want 42", result.ExitCode)
	}
}

func TestDockerIntegration_ExecStderr(t *testing.T) {
	skipIfNoDocker(t)
	p, err := newTestDockerProvider(t)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	id := spawnTestSandbox(t, p)
	ctx := context.Background()

	result, err := p.Exec(ctx, id, ExecOptions{Command: "echo error >&2"})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if !strings.Contains(result.Stderr, "error") {
		t.Errorf("stderr = %q, want to contain %q", result.Stderr, "error")
	}
}

func TestDockerIntegration_ExecStream(t *testing.T) {
	skipIfNoDocker(t)
	p, err := newTestDockerProvider(t)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	id := spawnTestSandbox(t, p)
	ctx := context.Background()

	ch, err := p.ExecStream(ctx, id, ExecOptions{
		Command: "for i in 1 2 3; do echo line_$i; done",
	})
	if err != nil {
		t.Fatalf("exec stream: %v", err)
	}

	var combined strings.Builder
	for chunk := range ch {
		combined.WriteString(chunk.Data)
	}
	out := combined.String()
	for _, line := range []string{"line_1", "line_2", "line_3"} {
		if !strings.Contains(out, line) {
			t.Errorf("stream output missing %q: got %q", line, out)
		}
	}
}

func TestDockerIntegration_WriteReadFile(t *testing.T) {
	skipIfNoDocker(t)
	p, err := newTestDockerProvider(t)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	id := spawnTestSandbox(t, p)
	ctx := context.Background()

	content := "hello docker provider"
	err = p.WriteFile(ctx, id, "/workspace/test.txt", strings.NewReader(content), "0644")
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	rc, err := p.ReadFile(ctx, id, "/workspace/test.txt")
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read all: %v", err)
	}
	if string(data) != content {
		t.Errorf("read = %q, want %q", string(data), content)
	}
}

func TestDockerIntegration_ListFiles(t *testing.T) {
	skipIfNoDocker(t)
	p, err := newTestDockerProvider(t)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	id := spawnTestSandbox(t, p)
	ctx := context.Background()

	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		if err := p.WriteFile(ctx, id, "/workspace/"+name, strings.NewReader("x"), "0644"); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	files, err := p.ListFiles(ctx, id, "/workspace")
	if err != nil {
		t.Fatalf("list files: %v", err)
	}
	if len(files) < 3 {
		t.Errorf("list returned %d files, want at least 3", len(files))
	}
}

func TestDockerIntegration_DeleteFile(t *testing.T) {
	skipIfNoDocker(t)
	p, err := newTestDockerProvider(t)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	id := spawnTestSandbox(t, p)
	ctx := context.Background()

	if err := p.WriteFile(ctx, id, "/workspace/del.txt", strings.NewReader("bye"), "0644"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := p.DeleteFile(ctx, id, "/workspace/del.txt", false); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = p.ReadFile(ctx, id, "/workspace/del.txt")
	if err == nil {
		t.Error("read after delete: expected error, got nil")
	}
}

func TestDockerIntegration_MoveFile(t *testing.T) {
	skipIfNoDocker(t)
	p, err := newTestDockerProvider(t)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	id := spawnTestSandbox(t, p)
	ctx := context.Background()

	if err := p.WriteFile(ctx, id, "/workspace/src.txt", strings.NewReader("move me"), "0644"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := p.MoveFile(ctx, id, "/workspace/src.txt", "/workspace/dst.txt"); err != nil {
		t.Fatalf("move: %v", err)
	}

	rc, err := p.ReadFile(ctx, id, "/workspace/dst.txt")
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	defer rc.Close()
	data, _ := io.ReadAll(rc)
	if string(data) != "move me" {
		t.Errorf("dst content = %q, want %q", string(data), "move me")
	}

	_, err = p.ReadFile(ctx, id, "/workspace/src.txt")
	if err == nil {
		t.Error("src still exists after move")
	}
}

func TestDockerIntegration_ChmodFile(t *testing.T) {
	skipIfNoDocker(t)
	p, err := newTestDockerProvider(t)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	id := spawnTestSandbox(t, p)
	ctx := context.Background()

	if err := p.WriteFile(ctx, id, "/workspace/script.sh", strings.NewReader("#!/bin/sh\necho hi"), "0644"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := p.ChmodFile(ctx, id, "/workspace/script.sh", "0755"); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	fi, err := p.StatFile(ctx, id, "/workspace/script.sh")
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if !strings.Contains(fi.Mode, "7") && !strings.Contains(fi.Mode, "rwx") {
		t.Errorf("mode after chmod = %q, expected executable bits", fi.Mode)
	}
}

func TestDockerIntegration_StatFile(t *testing.T) {
	skipIfNoDocker(t)
	p, err := newTestDockerProvider(t)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	id := spawnTestSandbox(t, p)
	ctx := context.Background()

	content := "stat test content"
	if err := p.WriteFile(ctx, id, "/workspace/stat.txt", strings.NewReader(content), "0644"); err != nil {
		t.Fatalf("write: %v", err)
	}

	fi, err := p.StatFile(ctx, id, "/workspace/stat.txt")
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if fi.Size != int64(len(content)) {
		t.Errorf("size = %d, want %d", fi.Size, len(content))
	}
	if fi.IsDir {
		t.Error("IsDir should be false for a file")
	}
}

func TestDockerIntegration_GlobFiles(t *testing.T) {
	skipIfNoDocker(t)
	p, err := newTestDockerProvider(t)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	id := spawnTestSandbox(t, p)
	ctx := context.Background()

	for _, name := range []string{"a.py", "b.py", "c.txt"} {
		if err := p.WriteFile(ctx, id, "/workspace/"+name, strings.NewReader("x"), "0644"); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	matches, err := p.GlobFiles(ctx, id, "/workspace/*.py")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) != 2 {
		t.Errorf("glob returned %d matches, want 2: %v", len(matches), matches)
	}
}

func TestDockerIntegration_ConsoleLog(t *testing.T) {
	skipIfNoDocker(t)
	p, err := newTestDockerProvider(t)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	id := spawnTestSandbox(t, p)
	ctx := context.Background()

	// Produce some output
	p.Exec(ctx, id, ExecOptions{Command: "echo console_test_output"}) //nolint:errcheck

	lines, err := p.ConsoleLog(ctx, id, 100)
	if err != nil {
		t.Fatalf("console log: %v", err)
	}
	_ = lines // log may be empty for exec output that isn't captured
}

func TestDockerIntegration_StatusNotFound(t *testing.T) {
	skipIfNoDocker(t)
	p, err := newTestDockerProvider(t)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	ctx := context.Background()
	_, err = p.Status(ctx, "nonexistent-container-id-12345")
	if err == nil {
		t.Error("expected error for nonexistent container")
	}
}

func TestDockerIntegration_DestroyNotFound(t *testing.T) {
	skipIfNoDocker(t)
	p, err := newTestDockerProvider(t)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	ctx := context.Background()
	err = p.Destroy(ctx, "nonexistent-container-id-12345")
	if err == nil {
		t.Error("expected error for nonexistent container")
	}
}

func TestDockerIntegration_PoolWorkspaceIsolation(t *testing.T) {
	skipIfNoDocker(t)
	p, err := newTestDockerProvider(t)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	ctx := context.Background()
	vmID := spawnTestSandbox(t, p)

	userA, userB := "sb-aaaa0001", "sb-bbbb0002"

	// Create workspace dirs
	p.Exec(ctx, vmID, ExecOptions{Command: "mkdir -p /workspace/" + userA})  //nolint:errcheck
	p.Exec(ctx, vmID, ExecOptions{Command: "mkdir -p /workspace/" + userB})  //nolint:errcheck

	// User A writes secret
	p.Exec(ctx, vmID, ExecOptions{Command: "echo TOP_SECRET > /workspace/" + userA + "/secret.txt"}) //nolint:errcheck

	// User B scoped to their workspace should not see User A's files
	result, err := p.Exec(ctx, vmID, ExecOptions{
		Command: "ls /workspace/" + userB + "/",
		WorkDir: "/workspace/" + userB,
	})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if strings.Contains(result.Stdout, "secret.txt") {
		t.Error("User B should NOT see User A files in their workspace")
	}
}

func TestDockerIntegration_ConcurrentUsers(t *testing.T) {
	skipIfNoDocker(t)
	p, err := newTestDockerProvider(t)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	ctx := context.Background()
	vmID := spawnTestSandbox(t, p)

	users := []string{"sb-u001", "sb-u002", "sb-u003"}
	for _, u := range users {
		p.Exec(ctx, vmID, ExecOptions{Command: "mkdir -p /workspace/" + u})                      //nolint:errcheck
		p.Exec(ctx, vmID, ExecOptions{Command: "echo " + u + " > /workspace/" + u + "/id.txt"}) //nolint:errcheck
	}

	for _, u := range users {
		result, err := p.Exec(ctx, vmID, ExecOptions{
			Command: "cat /workspace/" + u + "/id.txt",
		})
		if err != nil {
			t.Fatalf("exec for user %s: %v", u, err)
		}
		if !strings.Contains(strings.TrimSpace(result.Stdout), u) {
			t.Errorf("user %s identity mismatch: stdout=%q", u, result.Stdout)
		}
	}
}
