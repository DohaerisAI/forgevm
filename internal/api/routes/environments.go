package routes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DohaerisAI/forgevm/internal/httputil"
	"github.com/DohaerisAI/forgevm/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const (
	envBuildQueued   = "queued"
	envBuildBuilding = "building"
	envBuildReady    = "ready"
	envBuildFailed   = "failed"
	envBuildCanceled = "canceled"
)

var (
	validTargets = map[string]struct{}{
		"local":     {},
		"ghcr":      {},
		"dockerhub": {},
	}
	tagSanitizer = regexp.MustCompile(`[^a-z0-9._-]+`)
)

type EnvironmentRoutes struct {
	store store.Store
	build BuildStarter
}

type BuildStarter interface {
	Enqueue(buildID string) error
}

func NewEnvironmentRoutes(st store.Store, build BuildStarter) *EnvironmentRoutes {
	return &EnvironmentRoutes{store: st, build: build}
}

func (e *EnvironmentRoutes) Routes() chi.Router {
	r := chi.NewRouter()

	r.Route("/specs", func(r chi.Router) {
		r.Post("/", e.CreateSpec)
		r.Get("/", e.ListSpecs)
		r.Get("/{specID}", e.GetSpec)
		r.Get("/{specID}/suggestions", e.Suggestions)
	})

	r.Route("/builds", func(r chi.Router) {
		r.Post("/", e.StartBuild)
		r.Get("/", e.ListBuilds)
		r.Get("/{buildID}", e.GetBuild)
		r.Post("/{buildID}/cancel", e.CancelBuild)
		r.Get("/{buildID}/spawn-config", e.SpawnConfig)
	})

	r.Route("/registry-connections", func(r chi.Router) {
		r.Post("/", e.SaveRegistryConnection)
		r.Get("/", e.ListRegistryConnections)
		r.Delete("/{connectionID}", e.DeleteRegistryConnection)
	})

	return r
}

type createSpecRequest struct {
	OwnerID        string   `json:"owner_id"`
	Name           string   `json:"name"`
	BaseImage      string   `json:"base_image"`
	PythonPackages []string `json:"python_packages"`
	AptPackages    []string `json:"apt_packages,omitempty"`
	PythonVersion  string   `json:"python_version,omitempty"`
}

type environmentSpecResponse struct {
	ID             string    `json:"id"`
	OwnerID        string    `json:"owner_id"`
	Name           string    `json:"name"`
	BaseImage      string    `json:"base_image"`
	PythonPackages []string  `json:"python_packages"`
	AptPackages    []string  `json:"apt_packages"`
	PythonVersion  string    `json:"python_version,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type startBuildRequest struct {
	SpecID     string   `json:"spec_id"`
	Targets    []string `json:"targets"`
	Visibility string   `json:"visibility,omitempty"`
}

type saveRegistryConnectionRequest struct {
	ID        string `json:"id,omitempty"`
	OwnerID   string `json:"owner_id"`
	Provider  string `json:"provider"`
	Username  string `json:"username"`
	SecretRef string `json:"secret_ref"`
	IsDefault bool   `json:"is_default"`
}

type registryConnectionResponse struct {
	ID        string    `json:"id"`
	OwnerID   string    `json:"owner_id"`
	Provider  string    `json:"provider"`
	Username  string    `json:"username"`
	IsDefault bool      `json:"is_default"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type environmentArtifactResponse struct {
	Target    string    `json:"target"`
	ImageRef  string    `json:"image_ref"`
	Digest    string    `json:"digest,omitempty"`
	Status    string    `json:"status"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type environmentBuildResponse struct {
	ID             string                        `json:"id"`
	SpecID         string                        `json:"spec_id"`
	Status         string                        `json:"status"`
	CurrentStep    string                        `json:"current_step"`
	Log            string                        `json:"log"`
	ImageSizeBytes int64                         `json:"image_size_bytes"`
	DigestLocal    string                        `json:"digest_local,omitempty"`
	Error          string                        `json:"error,omitempty"`
	CreatedAt      time.Time                     `json:"created_at"`
	FinishedAt     *time.Time                    `json:"finished_at,omitempty"`
	UpdatedAt      time.Time                     `json:"updated_at"`
	Artifacts      []environmentArtifactResponse `json:"artifacts"`
}

type environmentBuildListItem struct {
	Build environmentBuildResponse `json:"build"`
	Spec  environmentSpecResponse  `json:"spec"`
}

func (e *EnvironmentRoutes) CreateSpec(w http.ResponseWriter, r *http.Request) {
	var req createSpecRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "invalid request body")
		return
	}

	req.OwnerID = strings.TrimSpace(req.OwnerID)
	req.Name = strings.TrimSpace(req.Name)
	req.BaseImage = strings.TrimSpace(req.BaseImage)
	req.PythonVersion = strings.TrimSpace(req.PythonVersion)
	if req.OwnerID == "" || req.Name == "" || req.BaseImage == "" {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "owner_id, name, and base_image are required")
		return
	}

	now := time.Now().UTC()
	pkgsJSON, _ := json.Marshal(cleanStringList(req.PythonPackages))
	aptJSON, _ := json.Marshal(cleanStringList(req.AptPackages))

	rec := &store.EnvironmentSpecRecord{
		ID:             uuid.NewString(),
		OwnerID:        req.OwnerID,
		Name:           req.Name,
		BaseImage:      req.BaseImage,
		PythonPackages: string(pkgsJSON),
		AptPackages:    string(aptJSON),
		PythonVersion:  req.PythonVersion,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := e.store.CreateEnvironmentSpec(r.Context(), rec); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			httputil.WriteError(w, http.StatusConflict, httputil.CodeConflict, "spec name already exists for this owner")
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, toSpecResponse(rec))
}

func (e *EnvironmentRoutes) ListSpecs(w http.ResponseWriter, r *http.Request) {
	ownerID := strings.TrimSpace(r.URL.Query().Get("owner_id"))
	if ownerID == "" {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "owner_id query parameter required")
		return
	}

	recs, err := e.store.ListEnvironmentSpecs(r.Context(), ownerID)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}

	out := make([]environmentSpecResponse, len(recs))
	for i, rec := range recs {
		out[i] = toSpecResponse(rec)
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

func (e *EnvironmentRoutes) GetSpec(w http.ResponseWriter, r *http.Request) {
	specID := chi.URLParam(r, "specID")
	rec, err := e.store.GetEnvironmentSpec(r.Context(), specID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			httputil.WriteError(w, http.StatusNotFound, httputil.CodeNotFound, err.Error())
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, toSpecResponse(rec))
}

func (e *EnvironmentRoutes) Suggestions(w http.ResponseWriter, r *http.Request) {
	specID := chi.URLParam(r, "specID")
	rec, err := e.store.GetEnvironmentSpec(r.Context(), specID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			httputil.WriteError(w, http.StatusNotFound, httputil.CodeNotFound, err.Error())
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}

	suggestions := []string{"pytest", "requests", "python-dotenv"}
	base := strings.ToLower(rec.BaseImage)
	if strings.Contains(base, "python") {
		suggestions = append(suggestions, "pandas", "numpy")
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"spec_id":     specID,
		"suggestions": suggestions,
	})
}

func (e *EnvironmentRoutes) StartBuild(w http.ResponseWriter, r *http.Request) {
	var req startBuildRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "invalid request body")
		return
	}

	req.SpecID = strings.TrimSpace(req.SpecID)
	if req.SpecID == "" {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "spec_id is required")
		return
	}

	spec, err := e.store.GetEnvironmentSpec(r.Context(), req.SpecID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			httputil.WriteError(w, http.StatusNotFound, httputil.CodeNotFound, err.Error())
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}

	targets, err := normalizeTargets(req.Targets)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, err.Error())
		return
	}

	now := time.Now().UTC()
	build := &store.EnvironmentBuildRecord{
		ID:          uuid.NewString(),
		SpecID:      req.SpecID,
		Status:      envBuildQueued,
		CurrentStep: "queued",
		LogBlob:     "build queued",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := e.store.CreateEnvironmentBuild(r.Context(), build); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}

	for _, target := range targets {
		artifact := &store.EnvironmentArtifactRecord{
			BuildID:   build.ID,
			Target:    target,
			ImageRef:  plannedImageRef(spec, build.ID, target),
			Status:    "pending",
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := e.store.SaveEnvironmentArtifact(r.Context(), artifact); err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
			return
		}
	}

	resp, err := e.getBuildResponse(r, build.ID)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}

	if e.build != nil {
		if err := e.build.Enqueue(build.ID); err != nil {
			resp.Error = "build queued but worker enqueue failed: " + err.Error()
		}
	}
	httputil.WriteJSON(w, http.StatusCreated, resp)
}

func (e *EnvironmentRoutes) GetBuild(w http.ResponseWriter, r *http.Request) {
	buildID := chi.URLParam(r, "buildID")
	resp, err := e.getBuildResponse(r, buildID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			httputil.WriteError(w, http.StatusNotFound, httputil.CodeNotFound, err.Error())
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (e *EnvironmentRoutes) ListBuilds(w http.ResponseWriter, r *http.Request) {
	ownerID := strings.TrimSpace(r.URL.Query().Get("owner_id"))
	if ownerID == "" {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "owner_id query parameter required")
		return
	}

	limit := 30
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 || n > 200 {
			httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "limit must be between 1 and 200")
			return
		}
		limit = n
	}

	specs, err := e.store.ListEnvironmentSpecs(r.Context(), ownerID)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}

	var items []environmentBuildListItem
	for _, spec := range specs {
		builds, err := e.store.ListEnvironmentBuilds(r.Context(), spec.ID)
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
			return
		}
		for _, build := range builds {
			resp, err := e.getBuildResponse(r, build.ID)
			if err != nil {
				httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
				return
			}
			items = append(items, environmentBuildListItem{
				Build: *resp,
				Spec:  toSpecResponse(spec),
			})
		}
	}

	sort.SliceStable(items, func(i, j int) bool {
		return items[i].Build.CreatedAt.After(items[j].Build.CreatedAt)
	})
	if len(items) > limit {
		items = items[:limit]
	}

	httputil.WriteJSON(w, http.StatusOK, items)
}

func (e *EnvironmentRoutes) CancelBuild(w http.ResponseWriter, r *http.Request) {
	buildID := chi.URLParam(r, "buildID")
	build, err := e.store.GetEnvironmentBuild(r.Context(), buildID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			httputil.WriteError(w, http.StatusNotFound, httputil.CodeNotFound, err.Error())
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}

	switch build.Status {
	case envBuildReady, envBuildFailed, envBuildCanceled:
		httputil.WriteError(w, http.StatusConflict, httputil.CodeConflict, "build already finalized")
		return
	}

	now := time.Now().UTC()
	build.Status = envBuildCanceled
	build.CurrentStep = "canceled"
	build.Error = "build canceled by user"
	build.FinishedAt = &now
	build.UpdatedAt = now

	if err := e.store.UpdateEnvironmentBuild(r.Context(), build); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}

	resp, err := e.getBuildResponse(r, build.ID)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (e *EnvironmentRoutes) SpawnConfig(w http.ResponseWriter, r *http.Request) {
	buildID := chi.URLParam(r, "buildID")
	build, err := e.store.GetEnvironmentBuild(r.Context(), buildID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			httputil.WriteError(w, http.StatusNotFound, httputil.CodeNotFound, err.Error())
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}

	artifacts, err := e.store.ListEnvironmentArtifacts(r.Context(), buildID)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}
	if len(artifacts) == 0 {
		httputil.WriteError(w, http.StatusNotFound, httputil.CodeNotFound, "no artifacts found for build")
		return
	}

	sort.SliceStable(artifacts, func(i, j int) bool {
		return targetOrder(artifacts[i].Target) < targetOrder(artifacts[j].Target)
	})
	selected := artifacts[0]
	for _, a := range artifacts {
		if a.Status == "ready" {
			selected = a
			break
		}
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"build_id":     buildID,
		"build_status": build.Status,
		"ready":        build.Status == envBuildReady || selected.Status == "ready",
		"provider":     "firecracker",
		"image":        selected.ImageRef,
		"target":       selected.Target,
		"digest":       selected.Digest,
		"note":         "use exact image value in spawn request; do not rewrite image ref",
	})
}

func (e *EnvironmentRoutes) SaveRegistryConnection(w http.ResponseWriter, r *http.Request) {
	var req saveRegistryConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "invalid request body")
		return
	}
	req.OwnerID = strings.TrimSpace(req.OwnerID)
	req.Provider = strings.ToLower(strings.TrimSpace(req.Provider))
	req.Username = strings.TrimSpace(req.Username)
	req.SecretRef = strings.TrimSpace(req.SecretRef)
	if req.OwnerID == "" || req.Provider == "" || req.Username == "" || req.SecretRef == "" {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "owner_id, provider, username, and secret_ref are required")
		return
	}
	if req.Provider != "ghcr" && req.Provider != "dockerhub" {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "provider must be one of: ghcr, dockerhub")
		return
	}
	id := req.ID
	if strings.TrimSpace(id) == "" {
		id = uuid.NewString()
	}
	now := time.Now().UTC()
	rec := &store.RegistryConnectionRecord{
		ID:        id,
		OwnerID:   req.OwnerID,
		Provider:  req.Provider,
		Username:  req.Username,
		SecretRef: req.SecretRef,
		IsDefault: req.IsDefault,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := e.store.SaveRegistryConnection(r.Context(), rec); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			httputil.WriteError(w, http.StatusConflict, httputil.CodeConflict, "registry connection already exists")
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, toRegistryConnectionResponse(rec))
}

func (e *EnvironmentRoutes) ListRegistryConnections(w http.ResponseWriter, r *http.Request) {
	ownerID := strings.TrimSpace(r.URL.Query().Get("owner_id"))
	if ownerID == "" {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "owner_id query parameter required")
		return
	}
	recs, err := e.store.ListRegistryConnections(r.Context(), ownerID)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}
	out := make([]registryConnectionResponse, len(recs))
	for i, rec := range recs {
		out[i] = toRegistryConnectionResponse(rec)
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

func (e *EnvironmentRoutes) DeleteRegistryConnection(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "connectionID")
	if err := e.store.DeleteRegistryConnection(r.Context(), id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			httputil.WriteError(w, http.StatusNotFound, httputil.CodeNotFound, err.Error())
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (e *EnvironmentRoutes) getBuildResponse(r *http.Request, buildID string) (*environmentBuildResponse, error) {
	build, err := e.store.GetEnvironmentBuild(r.Context(), buildID)
	if err != nil {
		return nil, err
	}
	artifacts, err := e.store.ListEnvironmentArtifacts(r.Context(), buildID)
	if err != nil {
		return nil, err
	}

	out := make([]environmentArtifactResponse, len(artifacts))
	for i, a := range artifacts {
		out[i] = environmentArtifactResponse{
			Target:    a.Target,
			ImageRef:  a.ImageRef,
			Digest:    a.Digest,
			Status:    a.Status,
			Error:     a.Error,
			CreatedAt: a.CreatedAt,
			UpdatedAt: a.UpdatedAt,
		}
	}

	return &environmentBuildResponse{
		ID:             build.ID,
		SpecID:         build.SpecID,
		Status:         build.Status,
		CurrentStep:    build.CurrentStep,
		Log:            build.LogBlob,
		ImageSizeBytes: build.ImageSizeBytes,
		DigestLocal:    build.DigestLocal,
		Error:          build.Error,
		CreatedAt:      build.CreatedAt,
		FinishedAt:     build.FinishedAt,
		UpdatedAt:      build.UpdatedAt,
		Artifacts:      out,
	}, nil
}

func toSpecResponse(rec *store.EnvironmentSpecRecord) environmentSpecResponse {
	return environmentSpecResponse{
		ID:             rec.ID,
		OwnerID:        rec.OwnerID,
		Name:           rec.Name,
		BaseImage:      rec.BaseImage,
		PythonPackages: decodeStringSlice(rec.PythonPackages),
		AptPackages:    decodeStringSlice(rec.AptPackages),
		PythonVersion:  rec.PythonVersion,
		CreatedAt:      rec.CreatedAt,
		UpdatedAt:      rec.UpdatedAt,
	}
}

func toRegistryConnectionResponse(rec *store.RegistryConnectionRecord) registryConnectionResponse {
	return registryConnectionResponse{
		ID:        rec.ID,
		OwnerID:   rec.OwnerID,
		Provider:  rec.Provider,
		Username:  rec.Username,
		IsDefault: rec.IsDefault,
		CreatedAt: rec.CreatedAt,
		UpdatedAt: rec.UpdatedAt,
	}
}

func decodeStringSlice(raw string) []string {
	var out []string
	_ = json.Unmarshal([]byte(raw), &out)
	if out == nil {
		return []string{}
	}
	return out
}

func cleanStringList(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	clean := make([]string, 0, len(items))
	for _, item := range items {
		v := strings.TrimSpace(item)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		clean = append(clean, v)
	}
	return clean
}

func normalizeTargets(input []string) ([]string, error) {
	if len(input) == 0 {
		return []string{"local"}, nil
	}
	seen := make(map[string]struct{}, len(input))
	var targets []string
	for _, t := range input {
		t = strings.ToLower(strings.TrimSpace(t))
		if _, ok := validTargets[t]; !ok {
			return nil, fmt.Errorf("invalid target %q (allowed: local, ghcr, dockerhub)", t)
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		targets = append(targets, t)
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("at least one target is required")
	}
	return targets, nil
}

func plannedImageRef(spec *store.EnvironmentSpecRecord, buildID, target string) string {
	baseTag := buildTag(spec.Name, buildID)
	switch target {
	case "ghcr":
		return "ghcr.io/registry-user/forgevm-env:" + baseTag
	case "dockerhub":
		return "docker.io/registry-user/forgevm-env:" + baseTag
	default:
		return "local/forgevm-env:" + baseTag
	}
}

func buildTag(name, buildID string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	n = strings.ReplaceAll(n, " ", "-")
	n = tagSanitizer.ReplaceAllString(n, "-")
	n = strings.Trim(n, "-")
	if n == "" {
		n = "env"
	}
	if len(n) > 24 {
		n = n[:24]
	}
	suffix := buildID
	if len(suffix) > 8 {
		suffix = suffix[:8]
	}
	return fmt.Sprintf("%s-%s", n, suffix)
}

func targetOrder(target string) int {
	switch target {
	case "local":
		return 0
	case "ghcr":
		return 1
	case "dockerhub":
		return 2
	default:
		return 99
	}
}
