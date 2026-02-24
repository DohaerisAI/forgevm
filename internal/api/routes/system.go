package routes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/DohaerisAI/forgevm/internal/httputil"
	"github.com/DohaerisAI/forgevm/internal/orchestrator"
	"github.com/DohaerisAI/forgevm/internal/providers"
)

type SystemRoutes struct {
	registry  *providers.Registry
	manager   *orchestrator.Manager
	events    *orchestrator.EventBus
	startTime time.Time
	version   string
}

func NewSystemRoutes(registry *providers.Registry, manager *orchestrator.Manager, events *orchestrator.EventBus, version string) *SystemRoutes {
	return &SystemRoutes{
		registry:  registry,
		manager:   manager,
		events:    events,
		startTime: time.Now(),
		version:   version,
	}
}

func (s *SystemRoutes) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/health", s.Health)
	r.Get("/metrics", s.Metrics)
	r.Get("/events", s.Events)
	return r
}

// Health returns the health status of the server.
//
//	@Summary		Health check
//	@Description	Return the health status, version, and uptime
//	@Tags			system
//	@Produce		json
//	@Success		200	{object}	HealthResponse
//	@Security		ApiKeyAuth
//	@Router			/health [get]
func (s *SystemRoutes) Health(w http.ResponseWriter, r *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "ok",
		"version": s.version,
		"uptime":  time.Since(s.startTime).String(),
	})
}

// Metrics returns runtime metrics.
//
//	@Summary		Get metrics
//	@Description	Return runtime metrics including sandbox count, goroutines, and memory usage
//	@Tags			system
//	@Produce		json
//	@Success		200	{object}	MetricsResponse
//	@Security		ApiKeyAuth
//	@Router			/metrics [get]
func (s *SystemRoutes) Metrics(w http.ResponseWriter, r *http.Request) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	sandboxes, _ := s.manager.List(r.Context())
	active := 0
	for _, sb := range sandboxes {
		if sb.State == orchestrator.StateRunning {
			active++
		}
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"goroutines":       runtime.NumGoroutine(),
		"memory_alloc":     mem.Alloc,
		"active_sandboxes": active,
		"total_sandboxes":  len(sandboxes),
	})
}

// Events serves Server-Sent Events for real-time updates.
//
//	@Summary		Subscribe to events
//	@Description	Open an SSE stream for real-time sandbox and system events
//	@Tags			system
//	@Produce		text/event-stream
//	@Success		200	{object}	orchestrator.Event
//	@Security		ApiKeyAuth
//	@Router			/events [get]
func (s *SystemRoutes) Events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	subID := uuid.New().String()
	ch := s.events.Subscribe(subID)
	defer s.events.Unsubscribe(subID)

	for {
		select {
		case <-r.Context().Done():
			return
		case evt, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(evt)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}
