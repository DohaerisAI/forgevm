package providers

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
)

func TestMockProvider_Name(t *testing.T) {
	p := NewMockProvider()
	if p.Name() != "mock" {
		t.Fatalf("expected name 'mock', got %q", p.Name())
	}
}

func TestMockProvider_Healthy(t *testing.T) {
	p := NewMockProvider()
	if !p.Healthy(context.Background()) {
		t.Fatal("expected healthy")
	}
}

func TestMockProvider_Spawn(t *testing.T) {
	p := NewMockProvider()
	ctx := context.Background()

	id, err := p.Spawn(ctx, SpawnOptions{Image: "alpine:latest"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if !strings.HasPrefix(id, "sb-") {
		t.Fatalf("expected id prefix 'sb-', got %q", id)
	}

	status, err := p.Status(ctx, id)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.State != "running" {
		t.Fatalf("expected state 'running', got %q", status.State)
	}

	// Cleanup
	p.Destroy(ctx, id)
}

func TestMockProvider_Exec(t *testing.T) {
	p := NewMockProvider()
	ctx := context.Background()

	id, err := p.Spawn(ctx, SpawnOptions{Image: "alpine:latest"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer p.Destroy(ctx, id)

	result, err := p.Exec(ctx, id, ExecOptions{Command: "echo hello world"})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d", result.ExitCode)
	}
	if strings.TrimSpace(result.Stdout) != "hello world" {
		t.Fatalf("expected stdout 'hello world', got %q", result.Stdout)
	}
}

func TestMockProvider_ExecNonZero(t *testing.T) {
	p := NewMockProvider()
	ctx := context.Background()

	id, err := p.Spawn(ctx, SpawnOptions{})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer p.Destroy(ctx, id)

	result, err := p.Exec(ctx, id, ExecOptions{Command: "exit 42"})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if result.ExitCode != 42 {
		t.Fatalf("expected exit 42, got %d", result.ExitCode)
	}
}

func TestMockProvider_WriteReadFile(t *testing.T) {
	p := NewMockProvider()
	ctx := context.Background()

	id, err := p.Spawn(ctx, SpawnOptions{})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer p.Destroy(ctx, id)

	content := "hello from forgevm"
	err = p.WriteFile(ctx, id, "/workspace/test.txt", bytes.NewReader([]byte(content)), "0644")
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
		t.Fatalf("reading: %v", err)
	}
	if string(data) != content {
		t.Fatalf("expected %q, got %q", content, string(data))
	}
}

func TestMockProvider_ListFiles(t *testing.T) {
	p := NewMockProvider()
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
		t.Fatalf("list files: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
}

func TestMockProvider_Destroy(t *testing.T) {
	p := NewMockProvider()
	ctx := context.Background()

	id, err := p.Spawn(ctx, SpawnOptions{})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	err = p.Destroy(ctx, id)
	if err != nil {
		t.Fatalf("destroy: %v", err)
	}

	status, err := p.Status(ctx, id)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.State != "destroyed" {
		t.Fatalf("expected state 'destroyed', got %q", status.State)
	}

	// Exec on destroyed sandbox should fail
	_, err = p.Exec(ctx, id, ExecOptions{Command: "echo"})
	if err == nil {
		t.Fatal("expected error on destroyed sandbox")
	}
}

func TestMockProvider_ExecStream(t *testing.T) {
	p := NewMockProvider()
	ctx := context.Background()

	id, err := p.Spawn(ctx, SpawnOptions{})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer p.Destroy(ctx, id)

	ch, err := p.ExecStream(ctx, id, ExecOptions{Command: "echo streaming"})
	if err != nil {
		t.Fatalf("exec stream: %v", err)
	}

	var output strings.Builder
	for chunk := range ch {
		output.WriteString(chunk.Data)
	}
	if !strings.Contains(output.String(), "streaming") {
		t.Fatalf("expected 'streaming' in output, got %q", output.String())
	}
}
