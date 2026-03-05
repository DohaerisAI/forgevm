package orchestrator

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/DohaerisAI/forgevm/internal/config"
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

	vmPoolMgr  *VMPoolManager
	poolConfig config.PoolConfig

	ctx    context.Context
	cancel context.CancelFunc
}

type ManagerConfig struct {
	DefaultTTL    time.Duration
	DefaultImage  string
	DefaultMemory int
	DefaultVCPUs  int
	Pool          config.PoolConfig
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
		poolConfig:    cfg.Pool,
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

	// Pool mode: acquire a VM slot instead of spawning a new VM.
	if m.vmPoolMgr != nil {
		return m.spawnPooled(ctx, prov, req, image, memMB, vcpus, ttl, now)
	}

	sb := &Sandbox{
		State:     StateCreating,
		Provider:  prov.Name(),
		Image:     image,
		MemoryMB:  memMB,
		VCPUs:     vcpus,
		OwnerID:   req.OwnerID,
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
		OwnerID:   sb.OwnerID,
		VMID:      sb.VMID,
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

// spawnPooled creates a logical sandbox on a shared pool VM.
func (m *Manager) spawnPooled(ctx context.Context, prov providers.Provider, req SpawnRequest, image string, memMB, vcpus int, ttl time.Duration, now time.Time) (*Sandbox, error) {
	// Generate a logical sandbox ID (not tied to a VM).
	b := make([]byte, 4)
	rand.Read(b)
	sandboxID := fmt.Sprintf("sb-%08x", b)

	// Acquire a VM slot (may spawn a new VM if needed).
	vmID, err := m.vmPoolMgr.Acquire(ctx, sandboxID)
	if err != nil {
		return nil, fmt.Errorf("pool acquire: %w", err)
	}

	sb := &Sandbox{
		ID:        sandboxID,
		State:     StateRunning,
		Provider:  prov.Name(),
		Image:     image,
		MemoryMB:  memMB,
		VCPUs:     vcpus,
		OwnerID:   req.OwnerID,
		VMID:      vmID,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
		Metadata:  req.Metadata,
	}

	// Create the sandbox's isolated workspace on the VM.
	workspaceDir := "/workspace/" + sandboxID
	_, err = prov.Exec(ctx, vmID, providers.ExecOptions{
		Command: "mkdir -p " + workspaceDir,
	})
	if err != nil {
		m.vmPoolMgr.Release(vmID, sandboxID)
		return nil, fmt.Errorf("creating workspace: %w", err)
	}

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
		OwnerID:   sb.OwnerID,
		VMID:      sb.VMID,
		CreatedAt: sb.CreatedAt,
		ExpiresAt: sb.ExpiresAt,
		UpdatedAt: now,
	}); err != nil {
		m.vmPoolMgr.Release(vmID, sandboxID)
		return nil, fmt.Errorf("persisting sandbox: %w", err)
	}

	m.mu.Lock()
	m.sandboxes[sandboxID] = sb
	m.mu.Unlock()

	m.events.Publish(Event{Type: EventSandboxCreated, SandboxID: sandboxID})
	m.events.Publish(Event{Type: EventSandboxRunning, SandboxID: sandboxID})

	m.logger.Info().
		Str("sandbox", sandboxID).
		Str("vm_id", vmID).
		Str("owner", req.OwnerID).
		Msg("pooled sandbox spawned")
	return sb, nil
}

func (m *Manager) Exec(ctx context.Context, sandboxID string, req ExecRequest) (*ExecResult, error) {
	sb, prov, err := m.getSandboxAndProvider(sandboxID)
	if err != nil {
		return nil, err
	}

	m.events.Publish(Event{
		Type:      EventExecStarted,
		SandboxID: sandboxID,
	})

	// In pool mode, default workdir to the sandbox's workspace.
	workDir := req.WorkDir
	if workDir == "" && sb.VMID != "" {
		workDir = "/workspace/" + sandboxID
	}

	start := time.Now()
	result, err := prov.Exec(ctx, m.resolveVMID(sb), providers.ExecOptions{
		Command: req.Command,
		Args:    req.Args,
		Env:     req.Env,
		WorkDir: workDir,
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
	sb, prov, err := m.getSandboxAndProvider(sandboxID)
	if err != nil {
		return nil, err
	}

	workDir := req.WorkDir
	if workDir == "" && sb.VMID != "" {
		workDir = "/workspace/" + sandboxID
	}

	return prov.ExecStream(ctx, m.resolveVMID(sb), providers.ExecOptions{
		Command: req.Command,
		Args:    req.Args,
		Env:     req.Env,
		WorkDir: workDir,
	})
}

func (m *Manager) WriteFile(ctx context.Context, sandboxID string, req FileWriteRequest) error {
	sb, prov, err := m.getSandboxAndProvider(sandboxID)
	if err != nil {
		return err
	}

	mode := req.Mode
	if mode == "" {
		mode = "0644"
	}

	path := m.scopedPath(sb, req.Path)
	if err := prov.WriteFile(ctx, m.resolveVMID(sb), path, strings.NewReader(req.Content), mode); err != nil {
		return fmt.Errorf("writing file: %w", err)
	}

	m.events.Publish(Event{
		Type:      EventFileWritten,
		SandboxID: sandboxID,
	})
	return nil
}

func (m *Manager) ReadFile(ctx context.Context, sandboxID string, path string) ([]byte, error) {
	sb, prov, err := m.getSandboxAndProvider(sandboxID)
	if err != nil {
		return nil, err
	}

	rc, err := prov.ReadFile(ctx, m.resolveVMID(sb), m.scopedPath(sb, path))
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
	sb, prov, err := m.getSandboxAndProvider(sandboxID)
	if err != nil {
		return nil, err
	}

	pFiles, err := prov.ListFiles(ctx, m.resolveVMID(sb), m.scopedPath(sb, path))
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

func (m *Manager) DeleteFile(ctx context.Context, sandboxID string, req FileDeleteRequest) error {
	sb, prov, err := m.getSandboxAndProvider(sandboxID)
	if err != nil {
		return err
	}

	path := m.scopedPath(sb, req.Path)
	return prov.DeleteFile(ctx, m.resolveVMID(sb), path, req.Recursive)
}

func (m *Manager) MoveFile(ctx context.Context, sandboxID string, req FileMoveRequest) error {
	sb, prov, err := m.getSandboxAndProvider(sandboxID)
	if err != nil {
		return err
	}

	oldPath := m.scopedPath(sb, req.OldPath)
	newPath := m.scopedPath(sb, req.NewPath)
	return prov.MoveFile(ctx, m.resolveVMID(sb), oldPath, newPath)
}

func (m *Manager) ChmodFile(ctx context.Context, sandboxID string, req FileChmodRequest) error {
	sb, prov, err := m.getSandboxAndProvider(sandboxID)
	if err != nil {
		return err
	}

	path := m.scopedPath(sb, req.Path)
	return prov.ChmodFile(ctx, m.resolveVMID(sb), path, req.Mode)
}

func (m *Manager) StatFile(ctx context.Context, sandboxID string, path string) (*FileInfo, error) {
	sb, prov, err := m.getSandboxAndProvider(sandboxID)
	if err != nil {
		return nil, err
	}

	scopedPath := m.scopedPath(sb, path)
	fi, err := prov.StatFile(ctx, m.resolveVMID(sb), scopedPath)
	if err != nil {
		return nil, err
	}
	return &FileInfo{
		Path:    fi.Path,
		Size:    fi.Size,
		Mode:    fi.Mode,
		IsDir:   fi.IsDir,
		ModTime: fi.ModTime,
	}, nil
}

func (m *Manager) GlobFiles(ctx context.Context, sandboxID string, pattern string) ([]string, error) {
	sb, prov, err := m.getSandboxAndProvider(sandboxID)
	if err != nil {
		return nil, err
	}

	scopedPattern := m.scopedPath(sb, pattern)
	return prov.GlobFiles(ctx, m.resolveVMID(sb), scopedPattern)
}

// scopedPath prefixes a path with the sandbox workspace when running in pool mode.
func (m *Manager) scopedPath(sb *Sandbox, path string) string {
	if sb.VMID == "" {
		return path // dedicated VM, no scoping
	}
	base := "/workspace/" + sb.ID
	cleaned := filepath.Clean(filepath.Join(base, path))
	if !strings.HasPrefix(cleaned, base+"/") && cleaned != base {
		return base
	}
	return cleaned
}

// resolveVMID returns the VM sandbox ID for provider calls.
// In pool mode, operations go to the VM (sb.VMID); in 1:1 mode, to the sandbox itself.
func (m *Manager) resolveVMID(sb *Sandbox) string {
	if sb.VMID != "" {
		return sb.VMID
	}
	return sb.ID
}

// VMPoolStatus returns the current VM pool status. Returns nil if pool is disabled.
func (m *Manager) VMPoolStatus() *VMPoolStatus {
	if m.vmPoolMgr == nil {
		return nil
	}
	status := m.vmPoolMgr.Status()
	return &status
}

// InitVMPool initializes the VM pool manager if pool mode is enabled.
func (m *Manager) InitVMPool() {
	if !m.poolConfig.Enabled {
		return
	}
	// Use spawnDirect (bypasses pool check) to avoid infinite recursion.
	m.vmPoolMgr = NewVMPoolManager(m.poolConfig, m.spawnDirect, m.logger)
	m.logger.Info().Int("max_vms", m.poolConfig.MaxVMs).Int("max_users_per_vm", m.poolConfig.MaxUsersPerVM).Msg("VM pool mode enabled")
}

// spawnDirect spawns a VM directly via the provider, bypassing pool logic.
// Used by the pool manager to create pool VMs without recursion.
func (m *Manager) spawnDirect(ctx context.Context, req SpawnRequest) (*Sandbox, error) {
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

	now := time.Now()
	id, err := prov.Spawn(ctx, providers.SpawnOptions{
		Image:    image,
		MemoryMB: memMB,
		VCPUs:    vcpus,
	})
	if err != nil {
		return nil, fmt.Errorf("spawning VM: %w", err)
	}

	sb := &Sandbox{
		ID:        id,
		State:     StateRunning,
		Provider:  prov.Name(),
		Image:     image,
		MemoryMB:  memMB,
		VCPUs:     vcpus,
		CreatedAt: now,
		ExpiresAt: now.Add(m.defaultTTL),
	}

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
		prov.Destroy(ctx, id)
		return nil, fmt.Errorf("persisting pool VM: %w", err)
	}

	m.mu.Lock()
	m.sandboxes[id] = sb
	m.mu.Unlock()

	m.logger.Info().Str("vm_id", id).Str("image", image).Msg("pool VM spawned")
	return sb, nil
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
	// Check if this is a pooled sandbox.
	m.mu.RLock()
	sb := m.sandboxes[id]
	m.mu.RUnlock()

	if sb != nil && sb.VMID != "" && m.vmPoolMgr != nil {
		return m.destroyPooled(ctx, id, sb)
	}

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

// destroyPooled cleans up a pooled sandbox's workspace and releases the VM slot.
func (m *Manager) destroyPooled(ctx context.Context, id string, sb *Sandbox) error {
	vmID := sb.VMID

	// Clean up workspace on the VM.
	prov, err := m.registry.Get(sb.Provider)
	if err == nil {
		workspaceDir := "/workspace/" + id
		_ = prov.DeleteFile(ctx, vmID, workspaceDir, true)
	}

	// Release the VM slot; if VM is now empty, destroy it.
	shouldDestroy := m.vmPoolMgr.Release(vmID, id)
	if shouldDestroy {
		if prov != nil {
			if err := prov.Destroy(ctx, vmID); err != nil {
				m.logger.Debug().Err(err).Str("vm_id", vmID).Msg("pool VM destroy failed")
			}
		}
		// Clean up the VM's sandbox record too.
		m.store.DeleteSandbox(ctx, vmID)
		m.mu.Lock()
		delete(m.sandboxes, vmID)
		m.mu.Unlock()
		m.logger.Info().Str("vm_id", vmID).Msg("empty pool VM destroyed")
	}

	// Clean up the logical sandbox record.
	m.store.UpdateSandboxState(ctx, id, string(StateDestroyed))
	m.mu.Lock()
	if cached, ok := m.sandboxes[id]; ok {
		cached.State = StateDestroyed
	}
	delete(m.sandboxes, id)
	m.mu.Unlock()

	m.events.Publish(Event{Type: EventSandboxDestroyed, SandboxID: id})
	m.logger.Info().Str("sandbox", id).Str("vm_id", vmID).Msg("pooled sandbox destroyed")
	return nil
}

func (m *Manager) ExtendTTL(ctx context.Context, id string, extra time.Duration) (*Sandbox, error) {
	sb, err := m.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	newExpiresAt := time.Now().Add(extra)
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
		OwnerID:   r.OwnerID,
		VMID:      r.VMID,
		CreatedAt: r.CreatedAt,
		ExpiresAt: r.ExpiresAt,
		Metadata:  metadata,
	}
}

