package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// CustomProvider forwards all Provider interface calls to a user-specified
// HTTP endpoint.  This allows users to bring their own sandbox backend.
//
// The remote endpoint is expected to expose the following paths:
//
//	POST   /spawn           — create a new sandbox
//	POST   /exec            — execute a command
//	POST   /files           — write a file
//	GET    /files            — read a file  (query: sandbox_id, path)
//	GET    /files/list       — list files   (query: sandbox_id, path)
//	GET    /status/:id       — sandbox status
//	DELETE /sandboxes/:id    — destroy a sandbox
//	GET    /health           — health check
type CustomProvider struct {
	name    string
	baseURL string
	apiKey  string
	client  *http.Client
}

// CustomProviderConfig holds the configuration for a custom HTTP provider.
type CustomProviderConfig struct {
	ProviderName string        `yaml:"name"     mapstructure:"name"`
	BaseURL      string        `yaml:"base_url" mapstructure:"base_url"`
	APIKey       string        `yaml:"api_key"  mapstructure:"api_key"`
	Timeout      time.Duration `yaml:"timeout"  mapstructure:"timeout"` // 0 means 60s default
}

// NewCustomProvider creates a new CustomProvider that forwards calls to the
// given HTTP endpoint.
func NewCustomProvider(cfg CustomProviderConfig) *CustomProvider {
	name := cfg.ProviderName
	if name == "" {
		name = "custom"
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	return &CustomProvider{
		name:    name,
		baseURL: cfg.BaseURL,
		apiKey:  cfg.APIKey,
		client:  &http.Client{Timeout: timeout},
	}
}

func (p *CustomProvider) Name() string { return p.name }

// doRequest is a shared helper that builds, signs, and executes an HTTP
// request against the remote custom endpoint.
func (p *CustomProvider) doRequest(ctx context.Context, method, path string, body interface{}) ([]byte, int, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshaling request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, bodyReader)
	if err != nil {
		return nil, 0, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("X-API-Key", p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return data, resp.StatusCode, nil
}

// doStreamRequest is like doRequest but returns the raw response for streaming
// consumption. The caller is responsible for closing the response body.
func (p *CustomProvider) doStreamRequest(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshaling request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/x-ndjson")
	if p.apiKey != "" {
		req.Header.Set("X-API-Key", p.apiKey)
	}

	return p.client.Do(req)
}

// Spawn creates a new sandbox on the remote endpoint.
// POST /spawn  { image, memory_mb, vcpus, disk_size_mb, metadata }
// Expects response: { "id": "..." }
func (p *CustomProvider) Spawn(ctx context.Context, opts SpawnOptions) (string, error) {
	body := map[string]interface{}{
		"image": opts.Image,
	}
	if opts.MemoryMB > 0 {
		body["memory_mb"] = opts.MemoryMB
	}
	if opts.VCPUs > 0 {
		body["vcpus"] = opts.VCPUs
	}
	if opts.DiskSizeMB > 0 {
		body["disk_size_mb"] = opts.DiskSizeMB
	}
	if opts.Metadata != nil {
		body["metadata"] = opts.Metadata
	}

	data, code, err := p.doRequest(ctx, "POST", "/spawn", body)
	if err != nil {
		return "", fmt.Errorf("custom spawn: %w", err)
	}
	if code >= 400 {
		return "", fmt.Errorf("custom spawn failed (HTTP %d): %s", code, string(data))
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", fmt.Errorf("custom spawn: invalid response: %w", err)
	}
	if result.ID == "" {
		return "", fmt.Errorf("custom spawn: no sandbox ID in response")
	}
	return result.ID, nil
}

// Exec runs a command in the sandbox.
// POST /exec  { sandbox_id, command, args, env, workdir }
// Expects response: { "exit_code": 0, "stdout": "...", "stderr": "..." }
func (p *CustomProvider) Exec(ctx context.Context, sandboxID string, opts ExecOptions) (*ExecResult, error) {
	body := map[string]interface{}{
		"sandbox_id": sandboxID,
		"command":    opts.Command,
	}
	if len(opts.Args) > 0 {
		body["args"] = opts.Args
	}
	if opts.Env != nil {
		body["env"] = opts.Env
	}
	if opts.WorkDir != "" {
		body["workdir"] = opts.WorkDir
	}

	data, code, err := p.doRequest(ctx, "POST", "/exec", body)
	if err != nil {
		return nil, fmt.Errorf("custom exec: %w", err)
	}
	if code >= 400 {
		return nil, fmt.Errorf("custom exec failed (HTTP %d): %s", code, string(data))
	}

	var result struct {
		ExitCode int    `json:"exit_code"`
		Stdout   string `json:"stdout"`
		Stderr   string `json:"stderr"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("custom exec: invalid response: %w", err)
	}
	return &ExecResult{
		ExitCode: result.ExitCode,
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
	}, nil
}

// ExecStream runs a command and streams NDJSON output chunks.
// POST /exec  { sandbox_id, command, args, env, workdir, stream: true }
// Expects NDJSON response: { "stream": "stdout"|"stderr", "data": "..." }
func (p *CustomProvider) ExecStream(ctx context.Context, sandboxID string, opts ExecOptions) (<-chan StreamChunk, error) {
	body := map[string]interface{}{
		"sandbox_id": sandboxID,
		"command":    opts.Command,
		"stream":     true,
	}
	if len(opts.Args) > 0 {
		body["args"] = opts.Args
	}
	if opts.Env != nil {
		body["env"] = opts.Env
	}
	if opts.WorkDir != "" {
		body["workdir"] = opts.WorkDir
	}

	resp, err := p.doStreamRequest(ctx, "POST", "/exec", body)
	if err != nil {
		return nil, fmt.Errorf("custom exec stream: %w", err)
	}
	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("custom exec stream failed (HTTP %d): %s", resp.StatusCode, string(data))
	}

	ch := make(chan StreamChunk, 64)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}
			var chunk StreamChunk
			if err := json.Unmarshal(line, &chunk); err == nil {
				ch <- chunk
			}
		}
	}()

	return ch, nil
}

// WriteFile writes content to a file in the sandbox.
// POST /files  { sandbox_id, path, content, mode }
func (p *CustomProvider) WriteFile(ctx context.Context, sandboxID string, path string, content io.Reader, mode string) error {
	data, err := io.ReadAll(content)
	if err != nil {
		return fmt.Errorf("reading content: %w", err)
	}

	body := map[string]string{
		"sandbox_id": sandboxID,
		"path":       path,
		"content":    string(data),
	}
	if mode != "" {
		body["mode"] = mode
	}

	respData, code, err := p.doRequest(ctx, "POST", "/files", body)
	if err != nil {
		return fmt.Errorf("custom write: %w", err)
	}
	if code >= 400 {
		return fmt.Errorf("custom write failed (HTTP %d): %s", code, string(respData))
	}
	return nil
}

// ReadFile reads a file from the sandbox.
// GET /files?sandbox_id=...&path=...
func (p *CustomProvider) ReadFile(ctx context.Context, sandboxID string, path string) (io.ReadCloser, error) {
	params := url.Values{}
	params.Set("sandbox_id", sandboxID)
	params.Set("path", path)

	data, code, err := p.doRequest(ctx, "GET", "/files?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("custom read: %w", err)
	}
	if code >= 400 {
		return nil, fmt.Errorf("custom read failed (HTTP %d): %s", code, string(data))
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

// ListFiles lists files at the given path in the sandbox.
// GET /files/list?sandbox_id=...&path=...
func (p *CustomProvider) ListFiles(ctx context.Context, sandboxID string, path string) ([]FileInfo, error) {
	params := url.Values{}
	params.Set("sandbox_id", sandboxID)
	params.Set("path", path)

	data, code, err := p.doRequest(ctx, "GET", "/files/list?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("custom list: %w", err)
	}
	if code >= 400 {
		return nil, fmt.Errorf("custom list failed (HTTP %d): %s", code, string(data))
	}

	var files []FileInfo
	if err := json.Unmarshal(data, &files); err != nil {
		return nil, fmt.Errorf("custom list: invalid response: %w", err)
	}
	return files, nil
}

// Status returns the current status of a sandbox.
// GET /status/:id
func (p *CustomProvider) Status(ctx context.Context, sandboxID string) (*SandboxStatus, error) {
	data, code, err := p.doRequest(ctx, "GET", "/status/"+url.PathEscape(sandboxID), nil)
	if err != nil {
		return nil, fmt.Errorf("custom status: %w", err)
	}
	if code == 404 {
		return &SandboxStatus{ID: sandboxID, State: "destroyed"}, nil
	}
	if code >= 400 {
		return nil, fmt.Errorf("custom status failed (HTTP %d): %s", code, string(data))
	}

	var result struct {
		ID    string `json:"id"`
		State string `json:"state"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("custom status: invalid response: %w", err)
	}

	id := result.ID
	if id == "" {
		id = sandboxID
	}
	state := result.State
	if state == "" {
		state = "running"
	}

	return &SandboxStatus{ID: id, State: state}, nil
}

// Destroy tears down a sandbox.
// DELETE /sandboxes/:id
func (p *CustomProvider) Destroy(ctx context.Context, sandboxID string) error {
	data, code, err := p.doRequest(ctx, "DELETE", "/sandboxes/"+url.PathEscape(sandboxID), nil)
	if err != nil {
		return fmt.Errorf("custom destroy: %w", err)
	}
	if code >= 400 && code != 404 {
		return fmt.Errorf("custom destroy failed (HTTP %d): %s", code, string(data))
	}
	return nil
}

func (p *CustomProvider) ConsoleLog(_ context.Context, _ string, _ int) ([]string, error) {
	return []string{"[INFO] console logs not available for custom provider"}, nil
}

// Healthy returns true if the remote provider endpoint is reachable and
// reports itself as healthy.
// GET /health
func (p *CustomProvider) Healthy(ctx context.Context) bool {
	_, code, err := p.doRequest(ctx, "GET", "/health", nil)
	return err == nil && code < 400
}
