package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/DohaerisAI/forgevm/internal/providers"
	"github.com/DohaerisAI/forgevm/internal/store"
	"github.com/rs/zerolog"
)

type Manager struct {
	registry *providers.Registry
	store    store.Store
	events   *EventBus
	logger   zerolog.Logger

	mu        sync.RWMutex
	sandboxes map[string]*Sandbox

	defaultTTL    time.Duration
	defaultImage  string
	defaultMemory int
	defaultVCPUs  int

	ctx    context.Context
	cancel context.CancelFunc
}

type ManagerConfig struct {
	DefaultTTL    time.Duration
	DefaultImage  string
	DefaultMemory int
	DefaultVCPUs  int
}

func NewManager(registry *providers.Registry, st store.Store, events *EventBus, logger zerolog.Logger, cfg ManagerConfig) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		registry:      registry,
		store:         st,
		events:        events,
		logger:        logger.With().Str("component", "manager").Logger(),
		sandboxes:     make(map[string]*Sandbox),
		defaultTTL:    cfg.DefaultTTL,
		defaultImage:  cfg.DefaultImage,
		defaultMemory: cfg.DefaultMemory,
		defaultVCPUs:  cfg.DefaultVCPUs,
		ctx:           ctx,
		cancel:        cancel,
	}
	if m.defaultTTL == 0 {
		m.defaultTTL = 30 * time.Minute
	}
	if m.defaultImage == "" {
		m.defaultImage = "alpine:latest"
	}
	if m.defaultMemory == 0 {
		m.defaultMemory = 512
	}
	if m.defaultVCPUs == 0 {
		m.defaultVCPUs = 1
	}
	return m
}

// Start launches the TTL reaper goroutine.
func (m *Manager) Start() {
	go m.reaper()
}

// Stop cancels the reaper and cleans up.
func (m *Manager) Stop() {
	m.cancel()
}

func (m *Manager) reaper() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.pruneExpired()
		}
	}
}

func (m *Manager) pruneExpired() {
	expired, err := m.store.ListExpiredSandboxes(m.ctx, time.Now())
	if err != nil {
		m.logger.Error().Err(err).Msg("listing expired sandboxes")
		return
	}
	for _, sb := range expired {
		m.logger.Info().Str("sandbox", sb.ID).Msg("reaping expired sandbox")
		if err := m.Destroy(m.ctx, sb.ID); err != nil {
			m.logger.Error().Err(err).Str("sandbox", sb.ID).Msg("destroying expired sandbox")
		}
	}
}

func (m *Manager) Spawn(ctx context.Context, req SpawnRequest) (*Sandbox, error) {
	providerName := req.Provider
	prov, err := m.registry.Get(providerName)
	if err != nil {
		return nil, fmt.Errorf("getting provider: %w", err)
	}

	image := req.Image
	if image == "" {
		image = m.defaultImage
	}
	memMB := req.MemoryMB
	if memMB == 0 {
		memMB = m.defaultMemory
	}
	vcpus := req.VCPUs
	if vcpus == 0 {
		vcpus = m.defaultVCPUs
	}
	ttl := m.defaultTTL
	if req.TTL != "" {
		parsed, err := time.ParseDuration(req.TTL)
		if err != nil {
			return nil, fmt.Errorf("parsing TTL: %w", err)
		}
		ttl = parsed
	}

	now := time.Now()
	sb := &Sandbox{
		State:     StateCreating,
		Provider:  prov.Name(),
		Image:     image,
		MemoryMB:  memMB,
		VCPUs:     vcpus,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
		Metadata:  req.Metadata,
	}

	// Spawn via provider
	id, err := prov.Spawn(ctx, providers.SpawnOptions{
		Image:    image,
		MemoryMB: memMB,
		VCPUs:    vcpus,
		Metadata: req.Metadata,
	})
	if err != nil {
		return nil, fmt.Errorf("spawning sandbox: %w", err)
	}
	sb.ID = id
	sb.State = StateRunning

	// Persist
	metaJSON, _ := json.Marshal(sb.Metadata)
	if err := m.store.CreateSandbox(ctx, &store.SandboxRecord{
		ID:        sb.ID,
		State:     string(sb.State),
		Provider:  sb.Provider,
		Image:     sb.Image,
		MemoryMB:  sb.MemoryMB,
		VCPUs:     sb.VCPUs,
		Metadata:  string(metaJSON),
		CreatedAt: sb.CreatedAt,
		ExpiresAt: sb.ExpiresAt,
		UpdatedAt: now,
	}); err != nil {
		// Best effort: destroy the sandbox if DB write fails
		prov.Destroy(ctx, id)
		return nil, fmt.Errorf("persisting sandbox: %w", err)
	}

	m.mu.Lock()
	m.sandboxes[id] = sb
	m.mu.Unlock()

	m.events.Publish(Event{
		Type:      EventSandboxCreated,
		SandboxID: id,
	})
	m.events.Publish(Event{
		Type:      EventSandboxRunning,
		SandboxID: id,
	})

	m.logger.Info().Str("sandbox", id).Str("provider", prov.Name()).Str("image", image).Msg("sandbox spawned")
	return sb, nil
}

func (m *Manager) Exec(ctx context.Context, sandboxID string, req ExecRequest) (*ExecResult, error) {
	sb, prov, err := m.getSandboxAndProvider(sandboxID)
	if err != nil {
		return nil, err
	}
	_ = sb

	m.events.Publish(Event{
		Type:      EventExecStarted,
		SandboxID: sandboxID,
	})

	start := time.Now()
	result, err := prov.Exec(ctx, sandboxID, providers.ExecOptions{
		Command: req.Command,
		Args:    req.Args,
		Env:     req.Env,
		WorkDir: req.WorkDir,
	})
	if err != nil {
		return nil, fmt.Errorf("exec: %w", err)
	}
	duration := time.Since(start)

	execResult := &ExecResult{
		ExitCode: result.ExitCode,
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
		Duration: duration.String(),
	}

	// Log to store
	m.store.CreateExecLog(ctx, &store.ExecLogRecord{
		SandboxID: sandboxID,
		Command:   req.Command,
		ExitCode:  result.ExitCode,
		Stdout:    result.Stdout,
		Stderr:    result.Stderr,
		Duration:  duration.String(),
		CreatedAt: time.Now(),
	})

	m.events.Publish(Event{
		Type:      EventExecCompleted,
		SandboxID: sandboxID,
	})

	return execResult, nil
}

func (m *Manager) ExecStream(ctx context.Context, sandboxID string, req ExecRequest) (<-chan providers.StreamChunk, error) {
	_, prov, err := m.getSandboxAndProvider(sandboxID)
	if err != nil {
		return nil, err
	}

	return prov.ExecStream(ctx, sandboxID, providers.ExecOptions{
		Command: req.Command,
		Args:    req.Args,
		Env:     req.Env,
		WorkDir: req.WorkDir,
	})
}

func (m *Manager) WriteFile(ctx context.Context, sandboxID string, req FileWriteRequest) error {
	_, prov, err := m.getSandboxAndProvider(sandboxID)
	if err != nil {
		return err
	}

	mode := req.Mode
	if mode == "" {
		mode = "0644"
	}

	if err := prov.WriteFile(ctx, sandboxID, req.Path, strings.NewReader(req.Content), mode); err != nil {
		return fmt.Errorf("writing file: %w", err)
	}

	m.events.Publish(Event{
		Type:      EventFileWritten,
		SandboxID: sandboxID,
	})
	return nil
}

func (m *Manager) ReadFile(ctx context.Context, sandboxID string, path string) ([]byte, error) {
	_, prov, err := m.getSandboxAndProvider(sandboxID)
	if err != nil {
		return nil, err
	}

	rc, err := prov.ReadFile(ctx, sandboxID, path)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}
	defer rc.Close()

	buf, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("reading file content: %w", err)
	}

	m.events.Publish(Event{
		Type:      EventFileRead,
		SandboxID: sandboxID,
	})
	return buf, nil
}

func (m *Manager) ListFiles(ctx context.Context, sandboxID string, path string) ([]FileInfo, error) {
	_, prov, err := m.getSandboxAndProvider(sandboxID)
	if err != nil {
		return nil, err
	}

	pFiles, err := prov.ListFiles(ctx, sandboxID, path)
	if err != nil {
		return nil, err
	}

	files := make([]FileInfo, len(pFiles))
	for i, f := range pFiles {
		files[i] = FileInfo{
			Path:    f.Path,
			Size:    f.Size,
			Mode:    f.Mode,
			IsDir:   f.IsDir,
			ModTime: f.ModTime,
		}
	}
	return files, nil
}

func (m *Manager) ConsoleLog(ctx context.Context, sandboxID string, lines int) ([]string, error) {
	_, prov, err := m.getSandboxAndProvider(sandboxID)
	if err != nil {
		return nil, err
	}
	return prov.ConsoleLog(ctx, sandboxID, lines)
}

func (m *Manager) Get(ctx context.Context, id string) (*Sandbox, error) {
	m.mu.RLock()
	sb, ok := m.sandboxes[id]
	m.mu.RUnlock()

	if ok {
		return sb, nil
	}

	// Fall back to store
	rec, err := m.store.GetSandbox(ctx, id)
	if err != nil {
		return nil, err
	}
	sb = recordToSandbox(rec)
	if sb.State == StateDestroyed {
		return nil, fmt.Errorf("sandbox %q not found", id)
	}
	return sb, nil
}

func (m *Manager) CountByProvider(_ context.Context, provider string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	count := 0
	for _, sb := range m.sandboxes {
		if sb.Provider == provider {
			count++
		}
	}
	return count
}

func (m *Manager) List(ctx context.Context) ([]*Sandbox, error) {
	records, err := m.store.ListSandboxes(ctx)
	if err != nil {
		return nil, err
	}
	sandboxes := make([]*Sandbox, len(records))
	for i, r := range records {
		sandboxes[i] = recordToSandbox(r)
	}
	return sandboxes, nil
}

func (m *Manager) Destroy(ctx context.Context, id string) error {
	prov, err := m.getProvider(id)
	if err != nil {
		// If we can't find the provider, just update the store
		m.store.DeleteSandbox(ctx, id)
		m.mu.Lock()
		delete(m.sandboxes, id)
		m.mu.Unlock()
		return nil
	}

	if err := prov.Destroy(ctx, id); err != nil {
		// Debug-level: this is expected when VMs were killed externally (e.g. process restart).
		m.logger.Debug().Err(err).Str("sandbox", id).Msg("provider destroy failed (VM may already be gone)")
	}

	m.store.UpdateSandboxState(ctx, id, string(StateDestroyed))
	m.mu.Lock()
	if sb, ok := m.sandboxes[id]; ok {
		sb.State = StateDestroyed
	}
	delete(m.sandboxes, id)
	m.mu.Unlock()

	m.events.Publish(Event{
		Type:      EventSandboxDestroyed,
		SandboxID: id,
	})

	m.logger.Info().Str("sandbox", id).Msg("sandbox destroyed")
	return nil
}

func (m *Manager) ExtendTTL(ctx context.Context, id string, extra time.Duration) (*Sandbox, error) {
	sb, err := m.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	newExpiresAt := sb.ExpiresAt.Add(extra)
	if err := m.store.UpdateSandboxExpiresAt(ctx, id, newExpiresAt); err != nil {
		return nil, fmt.Errorf("extending TTL: %w", err)
	}

	sb.ExpiresAt = newExpiresAt

	m.mu.Lock()
	if cached, ok := m.sandboxes[id]; ok {
		cached.ExpiresAt = newExpiresAt
	}
	m.mu.Unlock()

	m.logger.Info().Str("sandbox", id).Time("new_expires_at", newExpiresAt).Msg("TTL extended")
	return sb, nil
}

func (m *Manager) Prune(ctx context.Context) (int, error) {
	sandboxes, err := m.store.ListSandboxes(ctx)
	if err != nil {
		return 0, err
	}

	count := 0
	now := time.Now()
	for _, sb := range sandboxes {
		if sb.ExpiresAt.Before(now) {
			if err := m.Destroy(ctx, sb.ID); err != nil {
				m.logger.Error().Err(err).Str("sandbox", sb.ID).Msg("prune destroy failed")
				continue
			}
			count++
		}
	}
	return count, nil
}

func (m *Manager) getSandboxAndProvider(id string) (*Sandbox, providers.Provider, error) {
	m.mu.RLock()
	sb, ok := m.sandboxes[id]
	m.mu.RUnlock()

	if !ok {
		rec, err := m.store.GetSandbox(context.Background(), id)
		if err != nil {
			return nil, nil, fmt.Errorf("sandbox %q not found", id)
		}
		sb = recordToSandbox(rec)
	}

	if sb.State == StateDestroyed {
		return nil, nil, fmt.Errorf("sandbox %q is destroyed", id)
	}

	prov, err := m.registry.Get(sb.Provider)
	if err != nil {
		return nil, nil, fmt.Errorf("provider %q: %w", sb.Provider, err)
	}

	return sb, prov, nil
}

func (m *Manager) getProvider(id string) (providers.Provider, error) {
	m.mu.RLock()
	sb, ok := m.sandboxes[id]
	m.mu.RUnlock()

	if !ok {
		rec, err := m.store.GetSandbox(context.Background(), id)
		if err != nil {
			return nil, fmt.Errorf("sandbox %q not found", id)
		}
		sb = recordToSandbox(rec)
	}

	return m.registry.Get(sb.Provider)
}

func recordToSandbox(r *store.SandboxRecord) *Sandbox {
	var metadata map[string]string
	json.Unmarshal([]byte(r.Metadata), &metadata)
	return &Sandbox{
		ID:        r.ID,
		State:     SandboxState(r.State),
		Provider:  r.Provider,
		Image:     r.Image,
		MemoryMB:  r.MemoryMB,
		VCPUs:     r.VCPUs,
		CreatedAt: r.CreatedAt,
		ExpiresAt: r.ExpiresAt,
		Metadata:  metadata,
	}
}

