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
	"time"

	"github.com/rs/zerolog"
)

// PRootProviderConfig holds configuration for the PRoot provider.
type PRootProviderConfig struct {
	RootfsPath     string
	PRootBinary    string
	WorkspaceBase  string
	DefaultTimeout time.Duration
	MaxSandboxes   int
	MaxMemoryMB    int
	MaxDiskMB      int
	Languages      []string
}

type prootSandbox struct {
	id        string
	workspace string
	state     string
	opts      SpawnOptions
	createdAt time.Time
}

// PRootProvider implements Provider using PRoot for userspace Linux isolation.
// No root, no kernel modules, no Docker daemon required.
// Primary target: Android, also works on Raspberry Pi, Chromebooks, restricted VPS.
type PRootProvider struct {
	mu        sync.RWMutex
	sandboxes map[string]*prootSandbox
	config    PRootProviderConfig
	logger    zerolog.Logger
}

// NewPRootProvider creates a new PRoot provider.
func NewPRootProvider(cfg PRootProviderConfig, logger zerolog.Logger) *PRootProvider {
	if cfg.PRootBinary == "" {
		cfg.PRootBinary = "proot"
	}
	if cfg.DefaultTimeout == 0 {
		cfg.DefaultTimeout = 60 * time.Second
	}
	if cfg.MaxSandboxes == 0 {
		cfg.MaxSandboxes = 10
	}
	return &PRootProvider{
		sandboxes: make(map[string]*prootSandbox),
		config:    cfg,
		logger:    logger.With().Str("provider", "proot").Logger(),
	}
}

func (p *PRootProvider) Name() string { return "proot" }

// ProviderConfig returns the provider configuration for API exposure.
func (p *PRootProvider) ProviderConfig() PRootProviderConfig {
	return p.config
}

func (p *PRootProvider) Healthy(ctx context.Context) bool {
	// Check proot binary is available
	binary := p.config.PRootBinary
	if !filepath.IsAbs(binary) {
		if _, err := exec.LookPath(binary); err != nil {
			p.logger.Debug().Str("binary", binary).Msg("proot binary not found in PATH")
			return false
		}
	} else {
		if _, err := os.Stat(binary); err != nil {
			p.logger.Debug().Str("binary", binary).Msg("proot binary not found")
			return false
		}
	}

	// Check rootfs directory exists
	if p.config.RootfsPath != "" {
		if info, err := os.Stat(p.config.RootfsPath); err != nil || !info.IsDir() {
			p.logger.Debug().Str("rootfs", p.config.RootfsPath).Msg("rootfs directory not found")
			return false
		}
	}

	return true
}

func (p *PRootProvider) Spawn(ctx context.Context, opts SpawnOptions) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Enforce sandbox limit
	activeCount := 0
	for _, sb := range p.sandboxes {
		if sb.state != "destroyed" {
			activeCount++
		}
	}
	if activeCount >= p.config.MaxSandboxes {
		return "", fmt.Errorf("max sandboxes reached (%d)", p.config.MaxSandboxes)
	}

	id := generatePRootSandboxID()

	// Create workspace directory
	workspace := filepath.Join(p.config.WorkspaceBase, id)
	if err := os.MkdirAll(filepath.Join(workspace, "workspace"), 0755); err != nil {
		return "", fmt.Errorf("creating workspace dir: %w", err)
	}

	sb := &prootSandbox{
		id:        id,
		workspace: workspace,
		state:     "running",
		opts:      opts,
		createdAt: time.Now(),
	}
	p.sandboxes[id] = sb

	p.logger.Info().Str("sandbox", id).Str("workspace", workspace).Msg("sandbox spawned")
	return id, nil
}

func (p *PRootProvider) getSandbox(id string) (*prootSandbox, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	sb, ok := p.sandboxes[id]
	if !ok {
		return nil, fmt.Errorf("sandbox %q not found", id)
	}
	if sb.state == "destroyed" {
		return nil, fmt.Errorf("sandbox %q is destroyed", id)
	}
	return sb, nil
}

// safePath validates and resolves a path within the sandbox workspace,
// preventing path traversal attacks.
func (p *PRootProvider) safePath(sb *prootSandbox, path string) (string, error) {
	cleaned := filepath.Clean(path)
	full := filepath.Join(sb.workspace, cleaned)
	// Ensure resolved path is within workspace
	abs, err := filepath.Abs(full)
	if err != nil {
		return "", fmt.Errorf("resolving path: %w", err)
	}
	wsAbs, err := filepath.Abs(sb.workspace)
	if err != nil {
		return "", fmt.Errorf("resolving workspace: %w", err)
	}
	if !strings.HasPrefix(abs, wsAbs+string(filepath.Separator)) && abs != wsAbs {
		return "", fmt.Errorf("path %q escapes sandbox workspace", path)
	}
	return full, nil
}

// BuildCommand constructs the proot exec.Cmd for a sandbox and exec options.
// Exported for testing.
func (p *PRootProvider) BuildCommand(ctx context.Context, sb *prootSandbox, opts ExecOptions) *exec.Cmd {
	args := []string{
		"-0",
		"-r", p.config.RootfsPath,
		"-b", filepath.Join(sb.workspace, "workspace") + ":/workspace",
		"-b", "/dev/null:/dev/null",
		"-b", "/dev/urandom:/dev/urandom",
		"-w", "/workspace",
	}

	// Set working directory inside proot
	if opts.WorkDir != "" {
		args[len(args)-1] = opts.WorkDir
	}

	// Build the command: sh -c <command>
	args = append(args, "/bin/sh", "-c", opts.Command)

	cmd := exec.CommandContext(ctx, p.config.PRootBinary, args...)

	// Environment variables
	env := []string{
		"HOME=/root",
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"LANG=C.UTF-8",
		"TERM=xterm-256color",
	}
	for k, v := range opts.Env {
		env = append(env, k+"="+v)
	}
	cmd.Env = env

	return cmd
}

func (p *PRootProvider) Exec(ctx context.Context, sandboxID string, opts ExecOptions) (*ExecResult, error) {
	sb, err := p.getSandbox(sandboxID)
	if err != nil {
		return nil, err
	}

	timeout := p.config.DefaultTimeout
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := p.BuildCommand(execCtx, sb, opts)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("proot exec: %w", err)
		}
	}

	return &ExecResult{
		ExitCode: exitCode,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}, nil
}

func (p *PRootProvider) ExecStream(ctx context.Context, sandboxID string, opts ExecOptions) (<-chan StreamChunk, error) {
	sb, err := p.getSandbox(sandboxID)
	if err != nil {
		return nil, err
	}

	timeout := p.config.DefaultTimeout
	execCtx, cancel := context.WithTimeout(ctx, timeout)

	cmd := p.BuildCommand(execCtx, sb, opts)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	ch := make(chan StreamChunk, 64)

	if err := cmd.Start(); err != nil {
		cancel()
		close(ch)
		return nil, fmt.Errorf("start: %w", err)
	}

	go func() {
		defer close(ch)
		defer cancel()
		var wg sync.WaitGroup
		wg.Add(2)

		readStream := func(name string, r io.Reader) {
			defer wg.Done()
			buf := make([]byte, 4096)
			for {
				n, readErr := r.Read(buf)
				if n > 0 {
					select {
					case ch <- StreamChunk{Stream: name, Data: string(buf[:n])}:
					case <-execCtx.Done():
						return
					}
				}
				if readErr != nil {
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

func (p *PRootProvider) WriteFile(ctx context.Context, sandboxID string, path string, content io.Reader, mode string) error {
	sb, err := p.getSandbox(sandboxID)
	if err != nil {
		return err
	}

	fullPath, err := p.safePath(sb, path)
	if err != nil {
		return err
	}

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

func (p *PRootProvider) ReadFile(ctx context.Context, sandboxID string, path string) (io.ReadCloser, error) {
	sb, err := p.getSandbox(sandboxID)
	if err != nil {
		return nil, err
	}

	fullPath, err := p.safePath(sb, path)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(fullPath)
	if err != nil {
		return nil, fmt.Errorf("opening file: %w", err)
	}
	return f, nil
}

func (p *PRootProvider) ListFiles(ctx context.Context, sandboxID string, path string) ([]FileInfo, error) {
	sb, err := p.getSandbox(sandboxID)
	if err != nil {
		return nil, err
	}

	fullPath, err := p.safePath(sb, path)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return nil, fmt.Errorf("reading dir: %w", err)
	}

	files := make([]FileInfo, 0, len(entries))
	for _, e := range entries {
		info, infoErr := e.Info()
		if infoErr != nil {
			continue
		}
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

func (p *PRootProvider) DeleteFile(ctx context.Context, sandboxID string, path string, recursive bool) error {
	sb, err := p.getSandbox(sandboxID)
	if err != nil {
		return err
	}

	fullPath, err := p.safePath(sb, path)
	if err != nil {
		return err
	}

	if recursive {
		return os.RemoveAll(fullPath)
	}
	return os.Remove(fullPath)
}

func (p *PRootProvider) MoveFile(ctx context.Context, sandboxID string, oldPath, newPath string) error {
	sb, err := p.getSandbox(sandboxID)
	if err != nil {
		return err
	}

	fullOld, err := p.safePath(sb, oldPath)
	if err != nil {
		return err
	}
	fullNew, err := p.safePath(sb, newPath)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(fullNew), 0755); err != nil {
		return fmt.Errorf("creating dirs: %w", err)
	}

	return os.Rename(fullOld, fullNew)
}

func (p *PRootProvider) ChmodFile(ctx context.Context, sandboxID string, path string, mode string) error {
	sb, err := p.getSandbox(sandboxID)
	if err != nil {
		return err
	}

	fullPath, err := p.safePath(sb, path)
	if err != nil {
		return err
	}

	parsed, err := strconv.ParseUint(mode, 8, 32)
	if err != nil {
		return fmt.Errorf("parse mode: %w", err)
	}

	return os.Chmod(fullPath, os.FileMode(parsed))
}

func (p *PRootProvider) StatFile(ctx context.Context, sandboxID string, path string) (*FileInfo, error) {
	sb, err := p.getSandbox(sandboxID)
	if err != nil {
		return nil, err
	}

	fullPath, err := p.safePath(sb, path)
	if err != nil {
		return nil, err
	}

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

func (p *PRootProvider) GlobFiles(ctx context.Context, sandboxID string, pattern string) ([]string, error) {
	sb, err := p.getSandbox(sandboxID)
	if err != nil {
		return nil, err
	}

	// Validate pattern doesn't escape workspace
	cleanPattern := filepath.Clean(pattern)
	fullPattern := filepath.Join(sb.workspace, cleanPattern)

	matches, err := filepath.Glob(fullPattern)
	if err != nil {
		return nil, fmt.Errorf("glob: %w", err)
	}

	// Return workspace-relative paths
	result := make([]string, 0, len(matches))
	wsAbs, _ := filepath.Abs(sb.workspace)
	for _, match := range matches {
		abs, _ := filepath.Abs(match)
		if !strings.HasPrefix(abs, wsAbs) {
			continue // skip paths that escaped workspace
		}
		rel, _ := filepath.Rel(sb.workspace, match)
		result = append(result, "/"+rel)
	}

	return result, nil
}

func (p *PRootProvider) Status(ctx context.Context, sandboxID string) (*SandboxStatus, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	sb, ok := p.sandboxes[sandboxID]
	if !ok {
		return nil, fmt.Errorf("sandbox %q not found", sandboxID)
	}
	return &SandboxStatus{
		ID:    sb.id,
		State: sb.state,
	}, nil
}

func (p *PRootProvider) Destroy(ctx context.Context, sandboxID string) error {
	p.mu.Lock()
	sb, ok := p.sandboxes[sandboxID]
	if !ok {
		p.mu.Unlock()
		return fmt.Errorf("sandbox %q not found", sandboxID)
	}
	sb.state = "destroyed"
	delete(p.sandboxes, sandboxID)
	p.mu.Unlock()

	// Clean up workspace directory
	if err := os.RemoveAll(sb.workspace); err != nil {
		p.logger.Warn().Err(err).Str("sandbox", sandboxID).Msg("failed to remove workspace")
	}

	p.logger.Info().Str("sandbox", sandboxID).Msg("sandbox destroyed")
	return nil
}

func (p *PRootProvider) ConsoleLog(ctx context.Context, sandboxID string, lines int) ([]string, error) {
	sb, err := p.getSandbox(sandboxID)
	if err != nil {
		return nil, err
	}

	result := []string{
		"[INFO] proot sandbox " + sb.id + " started",
		"[INFO] rootfs: " + p.config.RootfsPath,
		"[INFO] workspace initialized at /workspace",
		"[INFO] sandbox ready",
	}
	if lines > 0 && lines < len(result) {
		result = result[len(result)-lines:]
	}
	return result, nil
}

func generatePRootSandboxID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return fmt.Sprintf("sb-%08x", b)
}
