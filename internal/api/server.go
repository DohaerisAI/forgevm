package api

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/DohaerisAI/forgevm/internal/api/middleware"
	"github.com/DohaerisAI/forgevm/internal/api/routes"
	"github.com/DohaerisAI/forgevm/internal/orchestrator"
	"github.com/DohaerisAI/forgevm/internal/providers"
	"github.com/rs/zerolog"
	httpSwagger "github.com/swaggo/http-swagger/v2"

	_ "github.com/DohaerisAI/forgevm/docs"
)

type ServerConfig struct {
	Addr    string
	APIKey  string
	Version string
}

type Server struct {
	httpServer *http.Server
	logger     zerolog.Logger
}

//	@title			ForgeVM API
//	@version		1.0
//	@description	ForgeVM microVM sandbox orchestrator API

//	@host		localhost:7423
//	@BasePath	/api/v1

//	@securityDefinitions.apikey	ApiKeyAuth
//	@in							header
//	@name						X-API-Key

func NewServer(cfg ServerConfig, registry *providers.Registry, manager *orchestrator.Manager, events *orchestrator.EventBus, templates *orchestrator.TemplateRegistry, pool *orchestrator.PoolManager, logger zerolog.Logger) *Server {
	r := chi.NewRouter()

	// Global middleware (applies to all routes including swagger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.RealIP)
	r.Use(middleware.RequestID)
	r.Use(middleware.Logging(logger))

	// Swagger UI — public, no auth required
	r.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	))

	// API routes — with auth and CORS
	r.Group(func(r chi.Router) {
		if cfg.APIKey != "" {
			r.Use(middleware.Auth(cfg.APIKey))
		}

		// CORS
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Access-Control-Allow-Origin", "*")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key, X-Request-ID")
				if r.Method == "OPTIONS" {
					w.WriteHeader(http.StatusOK)
					return
				}
				next.ServeHTTP(w, r)
			})
		})

		// Routes
		sandboxRoutes := routes.NewSandboxRoutes(manager)
		providerRoutes := routes.NewProviderRoutes(registry, manager)
		templateRoutes := routes.NewTemplateRoutes(templates, manager)
		snapshotRoutes := routes.NewSnapshotRoutes(registry)
		systemRoutes := routes.NewSystemRoutes(registry, manager, events, cfg.Version)

		r.Route("/api/v1", func(r chi.Router) {
			r.Mount("/sandboxes", sandboxRoutes.Routes())
			r.Mount("/providers", providerRoutes.Routes())
			r.Mount("/templates", templateRoutes.Routes())
			r.Mount("/snapshots", snapshotRoutes.Routes())
			r.Mount("/", systemRoutes.Routes())
		})
	})

	return &Server{
		httpServer: &http.Server{
			Addr:         cfg.Addr,
			Handler:      r,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 120 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
		logger: logger,
	}
}

func (s *Server) Start() error {
	s.logger.Info().Str("addr", s.httpServer.Addr).Msg("starting HTTP server")
	return s.httpServer.ListenAndServe()
}

func (s *Server) Handler() http.Handler {
	return s.httpServer.Handler
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info().Msg("shutting down HTTP server")
	return s.httpServer.Shutdown(ctx)
}
