package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DohaerisAI/forgevm/internal/api"
	"github.com/DohaerisAI/forgevm/internal/orchestrator"
	"github.com/DohaerisAI/forgevm/internal/providers"
	"github.com/DohaerisAI/forgevm/internal/store"
	"github.com/rs/zerolog"
)

func setupTestServer(t *testing.T) *httptest.Server {
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

	events := orchestrator.NewEventBus()
	logger := zerolog.Nop()

	mgr := orchestrator.NewManager(reg, st, events, logger, orchestrator.ManagerConfig{
		DefaultTTL:    5 * time.Minute,
		DefaultImage:  "alpine:latest",
		DefaultMemory: 512,
		DefaultVCPUs:  1,
	})
	mgr.Start()
	t.Cleanup(func() { mgr.Stop() })

	templates := orchestrator.NewTemplateRegistry(st)
	pool := orchestrator.NewPoolManager(mgr, templates, logger)

	srv := api.NewServer(api.ServerConfig{
		Addr:    ":0",
		Version: "test",
	}, reg, mgr, events, templates, pool, st, nil, logger)

	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func doJSON(t *testing.T, ts *httptest.Server, method, path string, body interface{}) *http.Response {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, ts.URL+path, bodyReader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	return resp
}

func decodeJSON(t *testing.T, resp *http.Response, v interface{}) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode: %v", err)
	}
}

func TestFullAPIFlow(t *testing.T) {
	ts := setupTestServer(t)

	// 1. Health check
	resp := doJSON(t, ts, "GET", "/api/v1/health", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("health: expected 200, got %d", resp.StatusCode)
	}
	var health map[string]interface{}
	decodeJSON(t, resp, &health)
	if health["status"] != "ok" {
		t.Fatalf("health status: %v", health["status"])
	}

	// 2. List providers
	resp = doJSON(t, ts, "GET", "/api/v1/providers", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("providers: expected 200, got %d", resp.StatusCode)
	}

	// 3. Spawn sandbox
	resp = doJSON(t, ts, "POST", "/api/v1/sandboxes", map[string]interface{}{
		"image": "alpine:latest",
	})
	if resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("spawn: expected 201, got %d: %s", resp.StatusCode, string(body))
	}
	var sandbox orchestrator.Sandbox
	decodeJSON(t, resp, &sandbox)
	if sandbox.ID == "" {
		t.Fatal("sandbox ID is empty")
	}
	if sandbox.State != orchestrator.StateRunning {
		t.Fatalf("expected running, got %s", sandbox.State)
	}
	sandboxID := sandbox.ID

	// 4. List sandboxes
	resp = doJSON(t, ts, "GET", "/api/v1/sandboxes", nil)
	var sandboxes []orchestrator.Sandbox
	decodeJSON(t, resp, &sandboxes)
	if len(sandboxes) != 1 {
		t.Fatalf("expected 1 sandbox, got %d", len(sandboxes))
	}

	// 5. Execute command
	resp = doJSON(t, ts, "POST", "/api/v1/sandboxes/"+sandboxID+"/exec", map[string]string{
		"command": "echo hello world",
	})
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("exec: expected 200, got %d: %s", resp.StatusCode, string(body))
	}
	var execResult orchestrator.ExecResult
	decodeJSON(t, resp, &execResult)
	if execResult.ExitCode != 0 {
		t.Fatalf("exec exit code: %d", execResult.ExitCode)
	}
	if !strings.Contains(execResult.Stdout, "hello world") {
		t.Fatalf("expected 'hello world' in stdout, got %q", execResult.Stdout)
	}

	// 6. Write file
	resp = doJSON(t, ts, "POST", "/api/v1/sandboxes/"+sandboxID+"/files", map[string]string{
		"path":    "/workspace/test.txt",
		"content": "file content here",
	})
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("write file: expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	// 7. Read file
	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/sandboxes/"+sandboxID+"/files?path=/workspace/test.txt", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("read file request: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("read file: expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "file content here" {
		t.Fatalf("expected 'file content here', got %q", string(body))
	}

	// 8. Destroy sandbox
	resp = doJSON(t, ts, "DELETE", "/api/v1/sandboxes/"+sandboxID, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("destroy: expected 200, got %d", resp.StatusCode)
	}

	// 9. Verify destroyed
	resp = doJSON(t, ts, "GET", "/api/v1/sandboxes", nil)
	var afterDestroy []orchestrator.Sandbox
	decodeJSON(t, resp, &afterDestroy)
	if len(afterDestroy) != 0 {
		t.Fatalf("expected 0 sandboxes after destroy, got %d", len(afterDestroy))
	}
}

func TestExecNonZeroExit(t *testing.T) {
	ts := setupTestServer(t)

	resp := doJSON(t, ts, "POST", "/api/v1/sandboxes", map[string]string{"image": "alpine"})
	var sb orchestrator.Sandbox
	decodeJSON(t, resp, &sb)

	resp = doJSON(t, ts, "POST", "/api/v1/sandboxes/"+sb.ID+"/exec", map[string]string{
		"command": "exit 1",
	})
	var result orchestrator.ExecResult
	decodeJSON(t, resp, &result)
	if result.ExitCode != 1 {
		t.Fatalf("expected exit 1, got %d", result.ExitCode)
	}
}

func TestSandboxNotFound(t *testing.T) {
	ts := setupTestServer(t)

	resp := doJSON(t, ts, "GET", "/api/v1/sandboxes/sb-doesnotexist", nil)
	if resp.StatusCode != 404 {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}
