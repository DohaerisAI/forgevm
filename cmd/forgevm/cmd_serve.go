package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mainadwitiya/forgevm/internal/api"
	"github.com/mainadwitiya/forgevm/internal/config"
	"github.com/mainadwitiya/forgevm/internal/orchestrator"
	"github.com/mainadwitiya/forgevm/internal/providers"
	"github.com/mainadwitiya/forgevm/internal/store"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
)

func newServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start the ForgeVM API server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe()
		},
	}
}

func runServe() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Logger
	var logger zerolog.Logger
	if cfg.Logging.Format == "pretty" {
		logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout}).With().Timestamp().Logger()
	} else {
		logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
	}
	level, err := zerolog.ParseLevel(cfg.Logging.Level)
	if err == nil {
		logger = logger.Level(level)
	}

	// Store
	st, err := store.NewSQLiteStore(cfg.Database.Path)
	if err != nil {
		return err
	}
	defer st.Close()

	// Event bus
	events := orchestrator.NewEventBus()

	// Provider registry
	registry := providers.NewRegistry()
	if cfg.Providers.Firecracker.Enabled {
		fc := providers.NewFirecrackerProvider(providers.FirecrackerProviderConfig{
			FirecrackerPath: cfg.Providers.Firecracker.FirecrackerPath,
			KernelPath:      cfg.Providers.Firecracker.KernelPath,
			DefaultRootfs:   cfg.Providers.Firecracker.DefaultRootfs,
			AgentPath:       cfg.ResolveAgentPath(),
			DataDir:         cfg.Providers.Firecracker.DataDir,
		}, logger)
		registry.Register(fc)
		fc.CheckDockerAccess()
		logger.Info().Str("path", cfg.Providers.Firecracker.FirecrackerPath).Msg("firecracker provider registered")
	}
	if cfg.Providers.E2B.Enabled {
		e2b := providers.NewE2BProvider(providers.E2BConfig{
			APIKey:  cfg.Providers.E2B.APIKey,
			BaseURL: cfg.Providers.E2B.BaseURL,
		})
		registry.Register(e2b)
		logger.Info().Msg("e2b provider registered")
	}
	if cfg.Providers.Custom.Enabled && cfg.Providers.Custom.BaseURL != "" {
		timeout, _ := time.ParseDuration(cfg.Providers.Custom.Timeout)
		if timeout == 0 {
			timeout = 60 * time.Second
		}
		custom := providers.NewCustomProvider(providers.CustomProviderConfig{
			ProviderName: cfg.Providers.Custom.Name,
			BaseURL:      cfg.Providers.Custom.BaseURL,
			APIKey:       cfg.Providers.Custom.APIKey,
			Timeout:      timeout,
		})
		registry.Register(custom)
		logger.Info().Str("name", cfg.Providers.Custom.Name).Str("url", cfg.Providers.Custom.BaseURL).Msg("custom provider registered")
	}
	if err := registry.SetDefault(cfg.Providers.Default); err != nil {
		logger.Warn().Err(err).Msg("setting default provider")
	}

	// Manager
	ttl, _ := time.ParseDuration(cfg.Defaults.TTL)
	mgr := orchestrator.NewManager(registry, st, events, logger, orchestrator.ManagerConfig{
		DefaultTTL:    ttl,
		DefaultImage:  cfg.Defaults.Image,
		DefaultMemory: cfg.Defaults.MemoryMB,
		DefaultVCPUs:  cfg.Defaults.VCPUs,
	})
	mgr.Start()
	defer mgr.Stop()

	// Template registry
	templates := orchestrator.NewTemplateRegistry(st)

	// Pool manager
	pool := orchestrator.NewPoolManager(mgr, templates, logger)
	pool.Start()
	defer pool.Stop()

	// Auth warning
	if cfg.Auth.APIKey == "" {
		logger.Warn().Msg("WARNING: no API key configured — all endpoints are unauthenticated. Set auth.api_key in config or FORGEVM_AUTH_API_KEY env var")
	}

	// Server
	srv := api.NewServer(api.ServerConfig{
		Addr:    cfg.Server.Addr(),
		APIKey:  cfg.Auth.APIKey,
		Version: version,
	}, registry, mgr, events, templates, pool, logger)

	// Graceful shutdown
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-quit:
		logger.Info().Msg("shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(ctx)
	case err := <-errCh:
		return err
	}
}
