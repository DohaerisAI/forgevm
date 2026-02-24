package routes

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/mainadwitiya/forgevm/internal/orchestrator"
	"github.com/mainadwitiya/forgevm/internal/providers"
	"github.com/mainadwitiya/forgevm/internal/store"
	"github.com/rs/zerolog"
)

func setupTestRouter(t *testing.T) (chi.Router, *orchestrator.Manager) {
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

	routes := NewSandboxRoutes(mgr)
	r := chi.NewRouter()
	r.Mount("/api/v1/sandboxes", routes.Routes())
	return r, mgr
}

func TestCreateSandbox(t *testing.T) {
	r, _ := setupTestRouter(t)

	body := `{"image":"alpine:latest"}`
	req := httptest.NewRequest("POST", "/api/v1/sandboxes", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var sb orchestrator.Sandbox
	json.NewDecoder(w.Body).Decode(&sb)
	if sb.ID == "" {
		t.Fatal("expected sandbox ID")
	}
	if sb.State != orchestrator.StateRunning {
		t.Fatalf("expected running, got %s", sb.State)
	}
}

func TestListSandboxes(t *testing.T) {
	r, _ := setupTestRouter(t)

	// Create one first
	body := `{"image":"alpine:latest"}`
	req := httptest.NewRequest("POST", "/api/v1/sandboxes", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// List
	req = httptest.NewRequest("GET", "/api/v1/sandboxes", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var sandboxes []orchestrator.Sandbox
	json.NewDecoder(w.Body).Decode(&sandboxes)
	if len(sandboxes) != 1 {
		t.Fatalf("expected 1 sandbox, got %d", len(sandboxes))
	}
}

func TestExecInSandbox(t *testing.T) {
	r, _ := setupTestRouter(t)

	// Create sandbox
	body := `{"image":"alpine:latest"}`
	req := httptest.NewRequest("POST", "/api/v1/sandboxes", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var sb orchestrator.Sandbox
	json.NewDecoder(w.Body).Decode(&sb)

	// Exec
	execBody := `{"command":"echo hello"}`
	req = httptest.NewRequest("POST", "/api/v1/sandboxes/"+sb.ID+"/exec", bytes.NewBufferString(execBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result orchestrator.ExecResult
	json.NewDecoder(w.Body).Decode(&result)
	if result.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d", result.ExitCode)
	}
}

func TestDestroyAndGet404(t *testing.T) {
	r, _ := setupTestRouter(t)

	// Create
	body := `{"image":"alpine:latest"}`
	req := httptest.NewRequest("POST", "/api/v1/sandboxes", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var sb orchestrator.Sandbox
	json.NewDecoder(w.Body).Decode(&sb)

	// Destroy
	req = httptest.NewRequest("DELETE", "/api/v1/sandboxes/"+sb.ID, nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Get nonexistent
	req = httptest.NewRequest("GET", "/api/v1/sandboxes/sb-nonexistent", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestExtendSandbox(t *testing.T) {
	r, _ := setupTestRouter(t)

	// Create
	body := `{"image":"alpine:latest","ttl":"5m"}`
	req := httptest.NewRequest("POST", "/api/v1/sandboxes", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var sb orchestrator.Sandbox
	json.NewDecoder(w.Body).Decode(&sb)
	originalExpiry := sb.ExpiresAt

	// Extend
	extendBody := `{"ttl":"30m"}`
	req = httptest.NewRequest("POST", "/api/v1/sandboxes/"+sb.ID+"/extend", bytes.NewBufferString(extendBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var updated orchestrator.Sandbox
	json.NewDecoder(w.Body).Decode(&updated)
	if !updated.ExpiresAt.After(originalExpiry) {
		t.Fatalf("expected extended expiry after %v, got %v", originalExpiry, updated.ExpiresAt)
	}
}

func TestExtendSandbox_InvalidTTL(t *testing.T) {
	r, _ := setupTestRouter(t)

	// Create
	body := `{"image":"alpine:latest"}`
	req := httptest.NewRequest("POST", "/api/v1/sandboxes", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var sb orchestrator.Sandbox
	json.NewDecoder(w.Body).Decode(&sb)

	// Empty TTL
	req = httptest.NewRequest("POST", "/api/v1/sandboxes/"+sb.ID+"/extend", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty ttl, got %d", w.Code)
	}

	// Negative TTL
	req = httptest.NewRequest("POST", "/api/v1/sandboxes/"+sb.ID+"/extend", bytes.NewBufferString(`{"ttl":"-5m"}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for negative ttl, got %d", w.Code)
	}

	// Bogus TTL
	req = httptest.NewRequest("POST", "/api/v1/sandboxes/"+sb.ID+"/extend", bytes.NewBufferString(`{"ttl":"not-a-duration"}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bogus ttl, got %d", w.Code)
	}
}

func TestExtendSandbox_NotFound(t *testing.T) {
	r, _ := setupTestRouter(t)

	req := httptest.NewRequest("POST", "/api/v1/sandboxes/sb-nope/extend", bytes.NewBufferString(`{"ttl":"30m"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestWriteAndReadFile(t *testing.T) {
	r, _ := setupTestRouter(t)

	// Create
	body := `{"image":"alpine:latest"}`
	req := httptest.NewRequest("POST", "/api/v1/sandboxes", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var sb orchestrator.Sandbox
	json.NewDecoder(w.Body).Decode(&sb)

	// Write file
	writeBody := `{"path":"/workspace/test.txt","content":"hello file"}`
	req = httptest.NewRequest("POST", "/api/v1/sandboxes/"+sb.ID+"/files", bytes.NewBufferString(writeBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("write expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Read file
	req = httptest.NewRequest("GET", "/api/v1/sandboxes/"+sb.ID+"/files?path=/workspace/test.txt", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("read expected 200, got %d", w.Code)
	}
	if w.Body.String() != "hello file" {
		t.Fatalf("expected 'hello file', got %q", w.Body.String())
	}
}
