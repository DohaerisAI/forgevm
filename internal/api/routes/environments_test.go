package routes

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DohaerisAI/forgevm/internal/store"
	"github.com/go-chi/chi/v5"
)

func setupEnvTestRouter(t *testing.T) chi.Router {
	t.Helper()
	dir := t.TempDir()
	st, err := store.NewSQLiteStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	r := chi.NewRouter()
	r.Mount("/api/v1/environments", NewEnvironmentRoutes(st, nil).Routes())
	return r
}

func TestEnvironmentFlow_CreateBuildSpawnConfig(t *testing.T) {
	r := setupEnvTestRouter(t)

	specReq := map[string]any{
		"owner_id":        "user-1",
		"name":            "data-tools",
		"base_image":      "python:3.12-slim",
		"python_packages": []string{"pandas", "numpy"},
		"apt_packages":    []string{"curl"},
	}
	body, _ := json.Marshal(specReq)
	req := httptest.NewRequest("POST", "/api/v1/environments/specs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create spec: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var spec struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(w.Body).Decode(&spec); err != nil {
		t.Fatalf("decode spec: %v", err)
	}
	if spec.ID == "" {
		t.Fatal("spec id is empty")
	}

	buildReq := map[string]any{
		"spec_id": spec.ID,
		"targets": []string{"local", "ghcr"},
	}
	body, _ = json.Marshal(buildReq)
	req = httptest.NewRequest("POST", "/api/v1/environments/builds", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("start build: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var build struct {
		ID        string `json:"id"`
		Status    string `json:"status"`
		Artifacts []any  `json:"artifacts"`
	}
	if err := json.NewDecoder(w.Body).Decode(&build); err != nil {
		t.Fatalf("decode build: %v", err)
	}
	if build.ID == "" {
		t.Fatal("build id is empty")
	}
	if build.Status != "queued" {
		t.Fatalf("expected queued status, got %s", build.Status)
	}
	if len(build.Artifacts) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(build.Artifacts))
	}

	req = httptest.NewRequest("GET", "/api/v1/environments/builds/"+build.ID+"/spawn-config", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("spawn config: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var spawnCfg struct {
		Image    string `json:"image"`
		Provider string `json:"provider"`
	}
	if err := json.NewDecoder(w.Body).Decode(&spawnCfg); err != nil {
		t.Fatalf("decode spawn config: %v", err)
	}
	if spawnCfg.Image == "" {
		t.Fatal("spawn config image is empty")
	}
	if spawnCfg.Provider != "firecracker" {
		t.Fatalf("expected provider firecracker, got %s", spawnCfg.Provider)
	}
}

func TestEnvironmentBuildCancel(t *testing.T) {
	r := setupEnvTestRouter(t)

	specReq := map[string]any{
		"owner_id":   "user-2",
		"name":       "quick-env",
		"base_image": "python:3.12-slim",
	}
	body, _ := json.Marshal(specReq)
	req := httptest.NewRequest("POST", "/api/v1/environments/specs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create spec: expected 201, got %d", w.Code)
	}
	var spec struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(w.Body).Decode(&spec)

	buildReq := map[string]any{"spec_id": spec.ID}
	body, _ = json.Marshal(buildReq)
	req = httptest.NewRequest("POST", "/api/v1/environments/builds", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("start build: expected 201, got %d", w.Code)
	}
	var build struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(w.Body).Decode(&build)

	req = httptest.NewRequest("POST", "/api/v1/environments/builds/"+build.ID+"/cancel", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("cancel build: expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRegistryConnectionCRUD(t *testing.T) {
	r := setupEnvTestRouter(t)

	createReq := map[string]any{
		"owner_id":   "user-4",
		"provider":   "ghcr",
		"username":   "octocat",
		"secret_ref": "token123",
		"is_default": true,
	}
	body, _ := json.Marshal(createReq)
	req := httptest.NewRequest("POST", "/api/v1/environments/registry-connections", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create registry connection: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var conn struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(w.Body).Decode(&conn)
	if conn.ID == "" {
		t.Fatal("connection id empty")
	}

	req = httptest.NewRequest("GET", "/api/v1/environments/registry-connections?owner_id=user-4", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list registry connections: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest("DELETE", "/api/v1/environments/registry-connections/"+conn.ID, nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("delete registry connection: expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListBuildsByOwner(t *testing.T) {
	r := setupEnvTestRouter(t)

	specReq := map[string]any{
		"owner_id":        "user-list",
		"name":            "data-tools",
		"base_image":      "python:3.12-slim",
		"python_packages": []string{"pandas", "numpy"},
	}
	body, _ := json.Marshal(specReq)
	req := httptest.NewRequest("POST", "/api/v1/environments/specs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create spec: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var spec struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(w.Body).Decode(&spec)

	buildReq := map[string]any{"spec_id": spec.ID}
	body, _ = json.Marshal(buildReq)
	req = httptest.NewRequest("POST", "/api/v1/environments/builds", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("start build: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest("GET", "/api/v1/environments/builds?owner_id=user-list", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list builds: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var out []struct {
		Build struct {
			ID string `json:"id"`
		} `json:"build"`
		Spec struct {
			Name           string   `json:"name"`
			PythonPackages []string `json:"python_packages"`
		} `json:"spec"`
	}
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatalf("decode list builds response: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 build item, got %d", len(out))
	}
	if out[0].Spec.Name != "data-tools" {
		t.Fatalf("unexpected spec name: %s", out[0].Spec.Name)
	}
	if len(out[0].Spec.PythonPackages) != 2 {
		t.Fatalf("expected 2 python packages, got %d", len(out[0].Spec.PythonPackages))
	}
}

func TestBuildPlannedImageRefDoesNotUseOwnerIDNamespace(t *testing.T) {
	r := setupEnvTestRouter(t)

	specReq := map[string]any{
		"owner_id":   "Jewel",
		"name":       "testing-ghcr",
		"base_image": "python:3.12-slim",
	}
	body, _ := json.Marshal(specReq)
	req := httptest.NewRequest("POST", "/api/v1/environments/specs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create spec: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var spec struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(w.Body).Decode(&spec)

	buildReq := map[string]any{
		"spec_id": spec.ID,
		"targets": []string{"ghcr"},
	}
	body, _ = json.Marshal(buildReq)
	req = httptest.NewRequest("POST", "/api/v1/environments/builds", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("start build: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var build struct {
		Artifacts []struct {
			Target   string `json:"target"`
			ImageRef string `json:"image_ref"`
		} `json:"artifacts"`
	}
	if err := json.NewDecoder(w.Body).Decode(&build); err != nil {
		t.Fatalf("decode build response: %v", err)
	}
	if len(build.Artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(build.Artifacts))
	}
	if build.Artifacts[0].Target != "ghcr" {
		t.Fatalf("expected ghcr target, got %s", build.Artifacts[0].Target)
	}
	if !strings.Contains(build.Artifacts[0].ImageRef, "ghcr.io/registry-user/") {
		t.Fatalf("expected registry placeholder namespace in image_ref, got %s", build.Artifacts[0].ImageRef)
	}
}
