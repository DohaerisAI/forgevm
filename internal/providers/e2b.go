package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// E2BProvider wraps the E2B REST API (no official Go SDK).
type E2BProvider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

type E2BConfig struct {
	APIKey  string `yaml:"api_key" mapstructure:"api_key"`
	BaseURL string `yaml:"base_url" mapstructure:"base_url"`
}

func NewE2BProvider(cfg E2BConfig) *E2BProvider {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.e2b.dev"
	}
	return &E2BProvider{
		apiKey:  cfg.APIKey,
		baseURL: baseURL,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

func (p *E2BProvider) Name() string { return "e2b" }

func (p *E2BProvider) doRequest(ctx context.Context, method, path string, body interface{}) ([]byte, int, error) {
	var bodyReader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, bodyReader)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return data, resp.StatusCode, nil
}

func (p *E2BProvider) Spawn(ctx context.Context, opts SpawnOptions) (string, error) {
	body := map[string]interface{}{
		"template": opts.Image,
	}
	if opts.Metadata != nil {
		body["metadata"] = opts.Metadata
	}

	data, code, err := p.doRequest(ctx, "POST", "/sandboxes", body)
	if err != nil {
		return "", fmt.Errorf("e2b spawn: %w", err)
	}
	if code >= 400 {
		return "", fmt.Errorf("e2b spawn failed (HTTP %d): %s", code, string(data))
	}

	var result struct {
		SandboxID string `json:"sandboxID"`
	}
	json.Unmarshal(data, &result)
	if result.SandboxID == "" {
		return "", fmt.Errorf("e2b: no sandbox ID in response")
	}
	return result.SandboxID, nil
}

func (p *E2BProvider) Exec(ctx context.Context, sandboxID string, opts ExecOptions) (*ExecResult, error) {
	body := map[string]interface{}{
		"cmd": opts.Command,
	}
	if opts.WorkDir != "" {
		body["cwd"] = opts.WorkDir
	}
	if opts.Env != nil {
		body["envVars"] = opts.Env
	}

	data, code, err := p.doRequest(ctx, "POST", fmt.Sprintf("/sandboxes/%s/commands", sandboxID), body)
	if err != nil {
		return nil, fmt.Errorf("e2b exec: %w", err)
	}
	if code >= 400 {
		return nil, fmt.Errorf("e2b exec failed (HTTP %d): %s", code, string(data))
	}

	var result struct {
		Stdout   string `json:"stdout"`
		Stderr   string `json:"stderr"`
		ExitCode int    `json:"exitCode"`
	}
	json.Unmarshal(data, &result)
	return &ExecResult{
		ExitCode: result.ExitCode,
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
	}, nil
}

func (p *E2BProvider) ExecStream(ctx context.Context, sandboxID string, opts ExecOptions) (<-chan StreamChunk, error) {
	// E2B doesn't natively support streaming via REST, so we fall back to buffered exec
	result, err := p.Exec(ctx, sandboxID, opts)
	if err != nil {
		return nil, err
	}
	ch := make(chan StreamChunk, 2)
	go func() {
		defer close(ch)
		if result.Stdout != "" {
			ch <- StreamChunk{Stream: "stdout", Data: result.Stdout}
		}
		if result.Stderr != "" {
			ch <- StreamChunk{Stream: "stderr", Data: result.Stderr}
		}
	}()
	return ch, nil
}

func (p *E2BProvider) WriteFile(ctx context.Context, sandboxID string, path string, content io.Reader, mode string) error {
	data, err := io.ReadAll(content)
	if err != nil {
		return err
	}
	body := map[string]string{
		"path":    path,
		"content": string(data),
	}
	_, code, err := p.doRequest(ctx, "POST", fmt.Sprintf("/sandboxes/%s/files", sandboxID), body)
	if err != nil {
		return fmt.Errorf("e2b write: %w", err)
	}
	if code >= 400 {
		return fmt.Errorf("e2b write failed (HTTP %d)", code)
	}
	return nil
}

func (p *E2BProvider) ReadFile(ctx context.Context, sandboxID string, path string) (io.ReadCloser, error) {
	data, code, err := p.doRequest(ctx, "GET", fmt.Sprintf("/sandboxes/%s/files?path=%s", sandboxID, path), nil)
	if err != nil {
		return nil, fmt.Errorf("e2b read: %w", err)
	}
	if code >= 400 {
		return nil, fmt.Errorf("e2b read failed (HTTP %d)", code)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (p *E2BProvider) ListFiles(ctx context.Context, sandboxID string, path string) ([]FileInfo, error) {
	data, code, err := p.doRequest(ctx, "GET", fmt.Sprintf("/sandboxes/%s/files?path=%s&list=true", sandboxID, path), nil)
	if err != nil {
		return nil, fmt.Errorf("e2b list: %w", err)
	}
	if code >= 400 {
		return nil, fmt.Errorf("e2b list failed (HTTP %d)", code)
	}

	var files []struct {
		Name  string `json:"name"`
		Size  int64  `json:"size"`
		IsDir bool   `json:"isDir"`
	}
	json.Unmarshal(data, &files)

	result := make([]FileInfo, len(files))
	for i, f := range files {
		result[i] = FileInfo{Path: f.Name, Size: f.Size, IsDir: f.IsDir}
	}
	return result, nil
}

func (p *E2BProvider) DeleteFile(ctx context.Context, sandboxID string, path string, recursive bool) error {
	return fmt.Errorf("e2b: delete_file not supported")
}

func (p *E2BProvider) MoveFile(ctx context.Context, sandboxID string, oldPath, newPath string) error {
	return fmt.Errorf("e2b: move_file not supported")
}

func (p *E2BProvider) ChmodFile(ctx context.Context, sandboxID string, path string, mode string) error {
	return fmt.Errorf("e2b: chmod_file not supported")
}

func (p *E2BProvider) StatFile(ctx context.Context, sandboxID string, path string) (*FileInfo, error) {
	return nil, fmt.Errorf("e2b: stat_file not supported")
}

func (p *E2BProvider) GlobFiles(ctx context.Context, sandboxID string, pattern string) ([]string, error) {
	return nil, fmt.Errorf("e2b: glob_files not supported")
}

func (p *E2BProvider) Status(ctx context.Context, sandboxID string) (*SandboxStatus, error) {
	data, code, err := p.doRequest(ctx, "GET", fmt.Sprintf("/sandboxes/%s", sandboxID), nil)
	if err != nil {
		return nil, fmt.Errorf("e2b status: %w", err)
	}
	if code == 404 {
		return &SandboxStatus{ID: sandboxID, State: "destroyed"}, nil
	}
	if code >= 400 {
		return nil, fmt.Errorf("e2b status failed (HTTP %d): %s", code, string(data))
	}
	return &SandboxStatus{ID: sandboxID, State: "running"}, nil
}

func (p *E2BProvider) Destroy(ctx context.Context, sandboxID string) error {
	_, code, err := p.doRequest(ctx, "DELETE", fmt.Sprintf("/sandboxes/%s", sandboxID), nil)
	if err != nil {
		return fmt.Errorf("e2b destroy: %w", err)
	}
	if code >= 400 && code != 404 {
		return fmt.Errorf("e2b destroy failed (HTTP %d)", code)
	}
	return nil
}

func (p *E2BProvider) ConsoleLog(_ context.Context, _ string, _ int) ([]string, error) {
	return []string{"[INFO] console logs not available for E2B provider"}, nil
}

func (p *E2BProvider) Healthy(ctx context.Context) bool {
	_, code, err := p.doRequest(ctx, "GET", "/health", nil)
	return err == nil && code < 400
}
