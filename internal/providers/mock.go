package providers

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

type mockSandbox struct {
	id    string
	root  string
	state string
	opts  SpawnOptions
}

// MockProvider implements Provider using temp directories and os/exec.
type MockProvider struct {
	mu        sync.RWMutex
	sandboxes map[string]*mockSandbox
}

func NewMockProvider() *MockProvider {
	return &MockProvider{
		sandboxes: make(map[string]*mockSandbox),
	}
}

func (m *MockProvider) Name() string { return "mock" }

func generateSandboxID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return fmt.Sprintf("sb-%08x", b)
}

func (m *MockProvider) Spawn(ctx context.Context, opts SpawnOptions) (string, error) {
	id := generateSandboxID()
	root, err := os.MkdirTemp("", "forgevm-"+id+"-")
	if err != nil {
		return "", fmt.Errorf("creating sandbox dir: %w", err)
	}

	// Create default workspace directory
	if err := os.MkdirAll(filepath.Join(root, "workspace"), 0755); err != nil {
		os.RemoveAll(root)
		return "", fmt.Errorf("creating workspace dir: %w", err)
	}

	sb := &mockSandbox{
		id:    id,
		root:  root,
		state: "running",
		opts:  opts,
	}

	m.mu.Lock()
	m.sandboxes[id] = sb
	m.mu.Unlock()

	return id, nil
}

func (m *MockProvider) getSandbox(id string) (*mockSandbox, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sb, ok := m.sandboxes[id]
	if !ok {
		return nil, fmt.Errorf("sandbox %q not found", id)
	}
	if sb.state == "destroyed" {
		return nil, fmt.Errorf("sandbox %q is destroyed", id)
	}
	return sb, nil
}

func (m *MockProvider) Exec(ctx context.Context, sandboxID string, opts ExecOptions) (*ExecResult, error) {
	sb, err := m.getSandbox(sandboxID)
	if err != nil {
		return nil, err
	}

	// Use sandbox root as working dir and set up env so /workspace resolves
	args := append([]string{"-c", opts.Command}, opts.Args...)
	cmd := exec.CommandContext(ctx, "sh", args...)
	cmd.Dir = filepath.Join(sb.root, "workspace")
	if opts.WorkDir != "" {
		cmd.Dir = filepath.Join(sb.root, opts.WorkDir)
	}

	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "HOME="+sb.root)
	cmd.Env = append(cmd.Env, "WORKSPACE="+filepath.Join(sb.root, "workspace"))
	cmd.Env = append(cmd.Env, "SANDBOX_ROOT="+sb.root)
	cmd.Env = append(cmd.Env, "PATH="+os.Getenv("PATH"))
	for k, v := range opts.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("exec: %w", err)
		}
	}

	return &ExecResult{
		ExitCode: exitCode,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}, nil
}

func (m *MockProvider) ExecStream(ctx context.Context, sandboxID string, opts ExecOptions) (<-chan StreamChunk, error) {
	sb, err := m.getSandbox(sandboxID)
	if err != nil {
		return nil, err
	}

	args := append([]string{"-c", opts.Command}, opts.Args...)
	cmd := exec.CommandContext(ctx, "sh", args...)
	cmd.Dir = filepath.Join(sb.root, "workspace")
	if opts.WorkDir != "" {
		cmd.Dir = filepath.Join(sb.root, opts.WorkDir)
	}

	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "HOME="+sb.root)
	cmd.Env = append(cmd.Env, "WORKSPACE="+filepath.Join(sb.root, "workspace"))
	cmd.Env = append(cmd.Env, "SANDBOX_ROOT="+sb.root)
	cmd.Env = append(cmd.Env, "PATH="+os.Getenv("PATH"))
	for k, v := range opts.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	ch := make(chan StreamChunk, 64)

	if err := cmd.Start(); err != nil {
		close(ch)
		return nil, fmt.Errorf("start: %w", err)
	}

	go func() {
		defer close(ch)
		var wg sync.WaitGroup
		wg.Add(2)

		readStream := func(name string, r io.Reader) {
			defer wg.Done()
			buf := make([]byte, 4096)
			for {
				n, err := r.Read(buf)
				if n > 0 {
					ch <- StreamChunk{Stream: name, Data: string(buf[:n])}
				}
				if err != nil {
					return
				}
			}
		}

		go readStream("stdout", stdoutPipe)
		go readStream("stderr", stderrPipe)

		wg.Wait()
		cmd.Wait()
	}()

	return ch, nil
}

func (m *MockProvider) WriteFile(ctx context.Context, sandboxID string, path string, content io.Reader, mode string) error {
	sb, err := m.getSandbox(sandboxID)
	if err != nil {
		return err
	}

	fullPath := filepath.Join(sb.root, filepath.Clean(path))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return fmt.Errorf("creating dirs: %w", err)
	}

	data, err := io.ReadAll(content)
	if err != nil {
		return fmt.Errorf("reading content: %w", err)
	}

	perm := os.FileMode(0644)
	if mode == "0755" || mode == "executable" {
		perm = 0755
	}

	return os.WriteFile(fullPath, data, perm)
}

func (m *MockProvider) ReadFile(ctx context.Context, sandboxID string, path string) (io.ReadCloser, error) {
	sb, err := m.getSandbox(sandboxID)
	if err != nil {
		return nil, err
	}

	fullPath := filepath.Join(sb.root, filepath.Clean(path))
	f, err := os.Open(fullPath)
	if err != nil {
		return nil, fmt.Errorf("opening file: %w", err)
	}
	return f, nil
}

func (m *MockProvider) ListFiles(ctx context.Context, sandboxID string, path string) ([]FileInfo, error) {
	sb, err := m.getSandbox(sandboxID)
	if err != nil {
		return nil, err
	}

	dir := filepath.Join(sb.root, filepath.Clean(path))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading dir: %w", err)
	}

	files := make([]FileInfo, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		// Return paths relative to sandbox root
		relPath := strings.TrimPrefix(filepath.Join(path, e.Name()), "/")
		files = append(files, FileInfo{
			Path:    relPath,
			Size:    info.Size(),
			Mode:    info.Mode().String(),
			IsDir:   e.IsDir(),
			ModTime: info.ModTime().UTC().Format("2006-01-02T15:04:05Z"),
		})
	}
	return files, nil
}

func (m *MockProvider) DeleteFile(ctx context.Context, sandboxID string, path string, recursive bool) error {
	sb, err := m.getSandbox(sandboxID)
	if err != nil {
		return err
	}

	fullPath := filepath.Join(sb.root, filepath.Clean(path))
	if recursive {
		return os.RemoveAll(fullPath)
	}
	return os.Remove(fullPath)
}

func (m *MockProvider) MoveFile(ctx context.Context, sandboxID string, oldPath, newPath string) error {
	sb, err := m.getSandbox(sandboxID)
	if err != nil {
		return err
	}

	fullOld := filepath.Join(sb.root, filepath.Clean(oldPath))
	fullNew := filepath.Join(sb.root, filepath.Clean(newPath))

	if err := os.MkdirAll(filepath.Dir(fullNew), 0755); err != nil {
		return fmt.Errorf("creating dirs: %w", err)
	}

	return os.Rename(fullOld, fullNew)
}

func (m *MockProvider) ChmodFile(ctx context.Context, sandboxID string, path string, mode string) error {
	sb, err := m.getSandbox(sandboxID)
	if err != nil {
		return err
	}

	fullPath := filepath.Join(sb.root, filepath.Clean(path))

	parsed, err := strconv.ParseUint(mode, 8, 32)
	if err != nil {
		return fmt.Errorf("parse mode: %w", err)
	}

	return os.Chmod(fullPath, os.FileMode(parsed))
}

func (m *MockProvider) StatFile(ctx context.Context, sandboxID string, path string) (*FileInfo, error) {
	sb, err := m.getSandbox(sandboxID)
	if err != nil {
		return nil, err
	}

	fullPath := filepath.Join(sb.root, filepath.Clean(path))
	info, err := os.Stat(fullPath)
	if err != nil {
		return nil, fmt.Errorf("stat: %w", err)
	}

	return &FileInfo{
		Path:    path,
		Size:    info.Size(),
		Mode:    fmt.Sprintf("%o", info.Mode()),
		IsDir:   info.IsDir(),
		ModTime: info.ModTime().UTC().Format("2006-01-02T15:04:05Z"),
	}, nil
}

func (m *MockProvider) GlobFiles(ctx context.Context, sandboxID string, pattern string) ([]string, error) {
	sb, err := m.getSandbox(sandboxID)
	if err != nil {
		return nil, err
	}

	fullPattern := filepath.Join(sb.root, filepath.Clean(pattern))
	matches, err := filepath.Glob(fullPattern)
	if err != nil {
		return nil, fmt.Errorf("glob: %w", err)
	}

	// Strip sandbox root prefix to return sandbox-relative paths
	result := make([]string, len(matches))
	for i, match := range matches {
		rel, _ := filepath.Rel(sb.root, match)
		result[i] = "/" + rel
	}

	return result, nil
}

func (m *MockProvider) Status(ctx context.Context, sandboxID string) (*SandboxStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sb, ok := m.sandboxes[sandboxID]
	if !ok {
		return nil, fmt.Errorf("sandbox %q not found", sandboxID)
	}
	return &SandboxStatus{
		ID:    sb.id,
		State: sb.state,
	}, nil
}

func (m *MockProvider) Destroy(ctx context.Context, sandboxID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	sb, ok := m.sandboxes[sandboxID]
	if !ok {
		return fmt.Errorf("sandbox %q not found", sandboxID)
	}
	sb.state = "destroyed"
	return os.RemoveAll(sb.root)
}

func (m *MockProvider) ConsoleLog(ctx context.Context, sandboxID string, lines int) ([]string, error) {
	_, err := m.getSandbox(sandboxID)
	if err != nil {
		return nil, err
	}
	// Mock provider returns synthetic log lines
	result := []string{
		"[INFO] mock sandbox " + sandboxID + " started",
		"[INFO] workspace initialized at /workspace",
		"[INFO] sandbox ready",
	}
	if lines > 0 && lines < len(result) {
		result = result[len(result)-lines:]
	}
	return result, nil
}

func (m *MockProvider) Healthy(ctx context.Context) bool {
	return true
}
