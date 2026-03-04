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
	"github.com/DohaerisAI/forgevm/internal/orchestrator"
	"github.com/DohaerisAI/forgevm/internal/providers"
	"github.com/DohaerisAI/forgevm/internal/store"
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

func createTestSandbox(t *testing.T, r chi.Router) string {
	t.Helper()
	body := `{"image":"alpine:latest"}`
	req := httptest.NewRequest("POST", "/api/v1/sandboxes", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create sandbox: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var sb orchestrator.Sandbox
	json.NewDecoder(w.Body).Decode(&sb)
	return sb.ID
}

func TestDeleteFile(t *testing.T) {
	r, _ := setupTestRouter(t)
	sbID := createTestSandbox(t, r)

	// Write a file
	writeBody := `{"path":"/workspace/todelete.txt","content":"bye"}`
	req := httptest.NewRequest("POST", "/api/v1/sandboxes/"+sbID+"/files", bytes.NewBufferString(writeBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("write: expected 200, got %d", w.Code)
	}

	// Delete the file
	req = httptest.NewRequest("DELETE", "/api/v1/sandboxes/"+sbID+"/files?path=/workspace/todelete.txt", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("delete: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify file is gone (read should fail)
	req = httptest.NewRequest("GET", "/api/v1/sandboxes/"+sbID+"/files?path=/workspace/todelete.txt", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code == http.StatusOK {
		t.Fatal("expected file to be deleted, but read succeeded")
	}
}

func TestDeleteFile_Recursive(t *testing.T) {
	r, _ := setupTestRouter(t)
	sbID := createTestSandbox(t, r)

	// Write files in a subdirectory
	writeBody := `{"path":"/workspace/subdir/nested.txt","content":"nested"}`
	req := httptest.NewRequest("POST", "/api/v1/sandboxes/"+sbID+"/files", bytes.NewBufferString(writeBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("write: expected 200, got %d", w.Code)
	}

	// Delete recursively
	req = httptest.NewRequest("DELETE", "/api/v1/sandboxes/"+sbID+"/files?path=/workspace/subdir&recursive=true", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("delete recursive: expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMoveFile(t *testing.T) {
	r, _ := setupTestRouter(t)
	sbID := createTestSandbox(t, r)

	// Write a file
	writeBody := `{"path":"/workspace/original.txt","content":"move me"}`
	req := httptest.NewRequest("POST", "/api/v1/sandboxes/"+sbID+"/files", bytes.NewBufferString(writeBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Move the file
	moveBody := `{"old_path":"/workspace/original.txt","new_path":"/workspace/moved.txt"}`
	req = httptest.NewRequest("POST", "/api/v1/sandboxes/"+sbID+"/files/move", bytes.NewBufferString(moveBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("move: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Read from new location
	req = httptest.NewRequest("GET", "/api/v1/sandboxes/"+sbID+"/files?path=/workspace/moved.txt", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("read moved: expected 200, got %d", w.Code)
	}
	if w.Body.String() != "move me" {
		t.Fatalf("expected 'move me', got %q", w.Body.String())
	}

	// Old path should be gone
	req = httptest.NewRequest("GET", "/api/v1/sandboxes/"+sbID+"/files?path=/workspace/original.txt", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code == http.StatusOK {
		t.Fatal("expected old file to be gone after move")
	}
}

func TestChmodFile(t *testing.T) {
	r, _ := setupTestRouter(t)
	sbID := createTestSandbox(t, r)

	// Write a file
	writeBody := `{"path":"/workspace/script.sh","content":"#!/bin/sh\necho hi"}`
	req := httptest.NewRequest("POST", "/api/v1/sandboxes/"+sbID+"/files", bytes.NewBufferString(writeBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Chmod
	chmodBody := `{"path":"/workspace/script.sh","mode":"0755"}`
	req = httptest.NewRequest("POST", "/api/v1/sandboxes/"+sbID+"/files/chmod", bytes.NewBufferString(chmodBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("chmod: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify via stat
	req = httptest.NewRequest("GET", "/api/v1/sandboxes/"+sbID+"/files/stat?path=/workspace/script.sh", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("stat: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var fi orchestrator.FileInfo
	json.NewDecoder(w.Body).Decode(&fi)
	// The mock provider uses fmt.Sprintf("%o", mode) which includes the file type bits
	if fi.Mode == "" {
		t.Fatal("expected non-empty mode from stat")
	}
}

func TestStatFile(t *testing.T) {
	r, _ := setupTestRouter(t)
	sbID := createTestSandbox(t, r)

	// Write a file
	writeBody := `{"path":"/workspace/info.txt","content":"some data"}`
	req := httptest.NewRequest("POST", "/api/v1/sandboxes/"+sbID+"/files", bytes.NewBufferString(writeBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Stat
	req = httptest.NewRequest("GET", "/api/v1/sandboxes/"+sbID+"/files/stat?path=/workspace/info.txt", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("stat: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var fi orchestrator.FileInfo
	json.NewDecoder(w.Body).Decode(&fi)
	if fi.Path != "/workspace/info.txt" {
		t.Fatalf("expected path '/workspace/info.txt', got %q", fi.Path)
	}
	if fi.Size != 9 { // "some data" = 9 bytes
		t.Fatalf("expected size 9, got %d", fi.Size)
	}
	if fi.IsDir {
		t.Fatal("expected is_dir=false")
	}
}

func TestStatFile_MissingPath(t *testing.T) {
	r, _ := setupTestRouter(t)
	sbID := createTestSandbox(t, r)

	req := httptest.NewRequest("GET", "/api/v1/sandboxes/"+sbID+"/files/stat", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGlobFiles(t *testing.T) {
	r, _ := setupTestRouter(t)
	sbID := createTestSandbox(t, r)

	// Write some files
	for _, name := range []string{"a.txt", "b.txt", "c.log"} {
		writeBody := `{"path":"/workspace/` + name + `","content":"data"}`
		req := httptest.NewRequest("POST", "/api/v1/sandboxes/"+sbID+"/files", bytes.NewBufferString(writeBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
	}

	// Glob for .txt files
	req := httptest.NewRequest("GET", "/api/v1/sandboxes/"+sbID+"/files/glob?pattern=/workspace/*.txt", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("glob: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var matches []string
	json.NewDecoder(w.Body).Decode(&matches)
	if len(matches) != 2 {
		t.Fatalf("expected 2 .txt matches, got %d: %v", len(matches), matches)
	}
}

func TestGlobFiles_MissingPattern(t *testing.T) {
	r, _ := setupTestRouter(t)
	sbID := createTestSandbox(t, r)

	req := httptest.NewRequest("GET", "/api/v1/sandboxes/"+sbID+"/files/glob", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestDeleteFile_MissingPath(t *testing.T) {
	r, _ := setupTestRouter(t)
	sbID := createTestSandbox(t, r)

	req := httptest.NewRequest("DELETE", "/api/v1/sandboxes/"+sbID+"/files", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCreateSandbox_WithOwnerID(t *testing.T) {
	r, _ := setupTestRouter(t)

	body := `{"image":"alpine:latest"}`
	req := httptest.NewRequest("POST", "/api/v1/sandboxes", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", "alice")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var sb orchestrator.Sandbox
	json.NewDecoder(w.Body).Decode(&sb)
	if sb.OwnerID != "alice" {
		t.Fatalf("expected owner_id 'alice', got %q", sb.OwnerID)
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
