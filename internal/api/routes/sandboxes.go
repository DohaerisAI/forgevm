package routes

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/mainadwitiya/forgevm/internal/httputil"
	"github.com/mainadwitiya/forgevm/internal/orchestrator"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

type SandboxRoutes struct {
	manager *orchestrator.Manager
}

func NewSandboxRoutes(manager *orchestrator.Manager) *SandboxRoutes {
	return &SandboxRoutes{manager: manager}
}

func (s *SandboxRoutes) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", s.Create)
	r.Get("/", s.List)
	r.Delete("/", s.Prune)
	r.Route("/{sandboxID}", func(r chi.Router) {
		r.Get("/", s.Get)
		r.Delete("/", s.Destroy)
		r.Post("/extend", s.Extend)
		r.Post("/exec", s.Exec)
		r.Get("/exec/ws", s.ExecWebSocket)
		r.Post("/files", s.WriteFile)
		r.Get("/files", s.ReadFile)
		r.Get("/files/list", s.ListFiles)
		r.Get("/logs", s.ConsoleLog)
	})
	return r
}

// Create creates a new sandbox.
//
//	@Summary		Create a sandbox
//	@Description	Spawn a new sandbox with the given configuration
//	@Tags			sandboxes
//	@Accept			json
//	@Produce		json
//	@Param			request	body		orchestrator.SpawnRequest	true	"Spawn request"
//	@Success		201		{object}	orchestrator.Sandbox
//	@Failure		400		{object}	httputil.APIError
//	@Failure		500		{object}	httputil.APIError
//	@Security		ApiKeyAuth
//	@Router			/sandboxes [post]
func (s *SandboxRoutes) Create(w http.ResponseWriter, r *http.Request) {
	var req orchestrator.SpawnRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "invalid request body")
		return
	}

	sb, err := s.manager.Spawn(r.Context(), req)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, sb)
}

// List lists all active sandboxes.
//
//	@Summary		List sandboxes
//	@Description	Return all active sandboxes
//	@Tags			sandboxes
//	@Produce		json
//	@Success		200	{array}		orchestrator.Sandbox
//	@Failure		500	{object}	httputil.APIError
//	@Security		ApiKeyAuth
//	@Router			/sandboxes [get]
func (s *SandboxRoutes) List(w http.ResponseWriter, r *http.Request) {
	sandboxes, err := s.manager.List(r.Context())
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}
	if sandboxes == nil {
		sandboxes = []*orchestrator.Sandbox{}
	}
	httputil.WriteJSON(w, http.StatusOK, sandboxes)
}

// Get returns a single sandbox by ID.
//
//	@Summary		Get a sandbox
//	@Description	Return a sandbox by its ID
//	@Tags			sandboxes
//	@Produce		json
//	@Param			sandboxID	path		string	true	"Sandbox ID"
//	@Success		200			{object}	orchestrator.Sandbox
//	@Failure		404			{object}	httputil.APIError
//	@Failure		500			{object}	httputil.APIError
//	@Security		ApiKeyAuth
//	@Router			/sandboxes/{sandboxID} [get]
func (s *SandboxRoutes) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "sandboxID")
	sb, err := s.manager.Get(r.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			httputil.WriteError(w, http.StatusNotFound, httputil.CodeNotFound, err.Error())
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, sb)
}

// Destroy destroys a sandbox by ID.
//
//	@Summary		Destroy a sandbox
//	@Description	Destroy a sandbox and release its resources
//	@Tags			sandboxes
//	@Produce		json
//	@Param			sandboxID	path		string	true	"Sandbox ID"
//	@Success		200			{object}	StatusResponse
//	@Failure		404			{object}	httputil.APIError
//	@Failure		500			{object}	httputil.APIError
//	@Security		ApiKeyAuth
//	@Router			/sandboxes/{sandboxID} [delete]
func (s *SandboxRoutes) Destroy(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "sandboxID")
	if err := s.manager.Destroy(r.Context(), id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			httputil.WriteError(w, http.StatusNotFound, httputil.CodeNotFound, err.Error())
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "destroyed"})
}

// Extend extends a sandbox's TTL.
//
//	@Summary		Extend sandbox TTL
//	@Description	Add additional time to a sandbox's expiration
//	@Tags			sandboxes
//	@Accept			json
//	@Produce		json
//	@Param			sandboxID	path		string	true	"Sandbox ID"
//	@Param			request		body		object{ttl=string}	true	"TTL extension"
//	@Success		200			{object}	orchestrator.Sandbox
//	@Failure		400			{object}	httputil.APIError
//	@Failure		404			{object}	httputil.APIError
//	@Failure		500			{object}	httputil.APIError
//	@Security		ApiKeyAuth
//	@Router			/sandboxes/{sandboxID}/extend [post]
func (s *SandboxRoutes) Extend(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "sandboxID")
	var req struct {
		TTL string `json:"ttl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "invalid request body")
		return
	}

	if req.TTL == "" {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "ttl is required")
		return
	}

	duration, err := time.ParseDuration(req.TTL)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "invalid ttl format: "+err.Error())
		return
	}

	if duration <= 0 {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "ttl must be positive")
		return
	}

	sb, err := s.manager.ExtendTTL(r.Context(), id, duration)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			httputil.WriteError(w, http.StatusNotFound, httputil.CodeNotFound, err.Error())
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusOK, sb)
}

// Exec executes a command in a sandbox.
//
//	@Summary		Execute a command
//	@Description	Run a command inside a sandbox. Set stream=true for streaming output.
//	@Tags			sandboxes
//	@Accept			json
//	@Produce		json
//	@Param			sandboxID	path		string					true	"Sandbox ID"
//	@Param			request		body		orchestrator.ExecRequest	true	"Exec request"
//	@Success		200			{object}	orchestrator.ExecResult
//	@Failure		404			{object}	httputil.APIError
//	@Failure		500			{object}	httputil.APIError
//	@Security		ApiKeyAuth
//	@Router			/sandboxes/{sandboxID}/exec [post]
func (s *SandboxRoutes) Exec(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "sandboxID")
	var req orchestrator.ExecRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "invalid request body")
		return
	}

	if req.Stream {
		s.execStream(w, r, id, req)
		return
	}

	result, err := s.manager.Exec(r.Context(), id, req)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "destroyed") {
			httputil.WriteError(w, http.StatusNotFound, httputil.CodeNotFound, err.Error())
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, result)
}

func (s *SandboxRoutes) execStream(w http.ResponseWriter, r *http.Request, id string, req orchestrator.ExecRequest) {
	ch, err := s.manager.ExecStream(r.Context(), id, req)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)
	enc := json.NewEncoder(w)

	for chunk := range ch {
		enc.Encode(chunk)
		if flusher != nil {
			flusher.Flush()
		}
	}
}

// WriteFile writes a file into a sandbox.
//
//	@Summary		Write a file
//	@Description	Write content to a file inside a sandbox
//	@Tags			sandboxes
//	@Accept			json
//	@Produce		json
//	@Param			sandboxID	path		string						true	"Sandbox ID"
//	@Param			request		body		orchestrator.FileWriteRequest	true	"File write request"
//	@Success		200			{object}	StatusResponse
//	@Failure		400			{object}	httputil.APIError
//	@Failure		404			{object}	httputil.APIError
//	@Failure		500			{object}	httputil.APIError
//	@Security		ApiKeyAuth
//	@Router			/sandboxes/{sandboxID}/files [post]
func (s *SandboxRoutes) WriteFile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "sandboxID")
	var req orchestrator.FileWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "invalid request body")
		return
	}

	if err := s.manager.WriteFile(r.Context(), id, req); err != nil {
		if strings.Contains(err.Error(), "not found") {
			httputil.WriteError(w, http.StatusNotFound, httputil.CodeNotFound, err.Error())
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "written"})
}

// ReadFile reads a file from a sandbox.
//
//	@Summary		Read a file
//	@Description	Read file content from a sandbox
//	@Tags			sandboxes
//	@Produce		octet-stream
//	@Param			sandboxID	path		string	true	"Sandbox ID"
//	@Param			path		query		string	true	"File path inside the sandbox"
//	@Success		200			{file}		binary
//	@Failure		400			{object}	httputil.APIError
//	@Failure		404			{object}	httputil.APIError
//	@Failure		500			{object}	httputil.APIError
//	@Security		ApiKeyAuth
//	@Router			/sandboxes/{sandboxID}/files [get]
func (s *SandboxRoutes) ReadFile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "sandboxID")
	path := r.URL.Query().Get("path")
	if path == "" {
		httputil.WriteError(w, http.StatusBadRequest, httputil.CodeBadRequest, "path query parameter required")
		return
	}

	data, err := s.manager.ReadFile(r.Context(), id, path)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			httputil.WriteError(w, http.StatusNotFound, httputil.CodeNotFound, err.Error())
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// ListFiles lists files in a sandbox directory.
//
//	@Summary		List files
//	@Description	List files in a directory inside a sandbox
//	@Tags			sandboxes
//	@Produce		json
//	@Param			sandboxID	path		string	true	"Sandbox ID"
//	@Param			path		query		string	false	"Directory path (default: /)"
//	@Success		200			{array}		orchestrator.FileInfo
//	@Failure		404			{object}	httputil.APIError
//	@Failure		500			{object}	httputil.APIError
//	@Security		ApiKeyAuth
//	@Router			/sandboxes/{sandboxID}/files/list [get]
func (s *SandboxRoutes) ListFiles(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "sandboxID")
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "/"
	}

	files, err := s.manager.ListFiles(r.Context(), id, path)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			httputil.WriteError(w, http.StatusNotFound, httputil.CodeNotFound, err.Error())
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, files)
}

// Prune destroys all expired sandboxes.
//
//	@Summary		Prune sandboxes
//	@Description	Destroy all expired sandboxes and return the count
//	@Tags			sandboxes
//	@Produce		json
//	@Success		200	{object}	PruneResponse
//	@Failure		500	{object}	httputil.APIError
//	@Security		ApiKeyAuth
//	@Router			/sandboxes [delete]
func (s *SandboxRoutes) Prune(w http.ResponseWriter, r *http.Request) {
	count, err := s.manager.Prune(r.Context())
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]int{"pruned": count})
}

// ConsoleLog retrieves console log output from a sandbox.
//
//	@Summary		Get console logs
//	@Description	Retrieve console log lines from a sandbox
//	@Tags			sandboxes
//	@Produce		json
//	@Param			sandboxID	path		string	true	"Sandbox ID"
//	@Param			lines		query		int		false	"Number of lines to retrieve (default: 100)"
//	@Success		200			{array}		string
//	@Failure		404			{object}	httputil.APIError
//	@Failure		500			{object}	httputil.APIError
//	@Security		ApiKeyAuth
//	@Router			/sandboxes/{sandboxID}/logs [get]
func (s *SandboxRoutes) ConsoleLog(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "sandboxID")
	lines := 100
	if q := r.URL.Query().Get("lines"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 {
			lines = n
		}
	}

	log, err := s.manager.ConsoleLog(r.Context(), id, lines)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			httputil.WriteError(w, http.StatusNotFound, httputil.CodeNotFound, err.Error())
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, log)
}

// ExecWebSocket executes a command over WebSocket.
//
//	@Summary		Execute via WebSocket
//	@Description	Open a WebSocket connection to execute a command with streaming output
//	@Tags			sandboxes
//	@Param			sandboxID	path	string	true	"Sandbox ID"
//	@Success		101			"WebSocket upgrade"
//	@Failure		400			"Bad request"
//	@Security		ApiKeyAuth
//	@Router			/sandboxes/{sandboxID}/exec/ws [get]
func (s *SandboxRoutes) ExecWebSocket(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "sandboxID")

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	ctx := r.Context()

	// Read command from client
	var req orchestrator.ExecRequest
	if err := wsjson.Read(ctx, conn, &req); err != nil {
		conn.Close(websocket.StatusInvalidFramePayloadData, "invalid request")
		return
	}

	ch, err := s.manager.ExecStream(ctx, id, req)
	if err != nil {
		wsjson.Write(ctx, conn, map[string]string{"error": err.Error()})
		conn.Close(websocket.StatusInternalError, "exec failed")
		return
	}

	for chunk := range ch {
		if err := wsjson.Write(ctx, conn, chunk); err != nil {
			return
		}
	}

	wsjson.Write(ctx, conn, map[string]string{"type": "done"})
}
