package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/DohaerisAI/forgevm/internal/config"
	"github.com/rs/zerolog"
)

// ErrVMPoolFull is returned when the VM pool is at capacity and overflow is "reject".
var ErrVMPoolFull = errors.New("vm pool at capacity")

// VMSlot tracks a single VM in the pool.
type VMSlot struct {
	VMID      string
	Provider  string
	Image     string
	UserCount int
	MaxUsers  int
	CreatedAt time.Time
}

// VMPoolStatus is returned by the pool status endpoint.
type VMPoolStatus struct {
	Enabled       bool `json:"enabled"`
	VMs           int  `json:"vms"`
	MaxVMs        int  `json:"max_vms"`
	TotalUsers    int  `json:"total_users"`
	MaxUsersPerVM int  `json:"max_users_per_vm"`
}

// VMPoolManager handles multi-user VM pool scheduling.
type VMPoolManager struct {
	mu     sync.RWMutex
	vms    map[string]*VMSlot // vmID → slot
	config config.PoolConfig
	spawn  func(ctx context.Context, req SpawnRequest) (*Sandbox, error)
	logger zerolog.Logger
}

// NewVMPoolManager creates a VM pool manager. The spawnFn is called to create new VMs.
func NewVMPoolManager(cfg config.PoolConfig, spawnFn func(ctx context.Context, req SpawnRequest) (*Sandbox, error), logger zerolog.Logger) *VMPoolManager {
	return &VMPoolManager{
		vms:    make(map[string]*VMSlot),
		config: cfg,
		spawn:  spawnFn,
		logger: logger.With().Str("component", "vm_pool").Logger(),
	}
}

// Acquire finds or creates a VM with capacity and returns the vmID.
func (p *VMPoolManager) Acquire(ctx context.Context, sandboxID string) (string, error) {
	p.mu.Lock()

	// Find an existing VM with capacity.
	for _, slot := range p.vms {
		if slot.UserCount < slot.MaxUsers {
			slot.UserCount++
			vmID := slot.VMID
			p.mu.Unlock()
			p.logger.Info().Str("vm_id", vmID).Str("sandbox_id", sandboxID).Int("users", slot.UserCount).Msg("assigned sandbox to existing VM")
			return vmID, nil
		}
	}

	// No capacity — can we create a new VM?
	if len(p.vms) >= p.config.MaxVMs {
		p.mu.Unlock()
		return "", ErrVMPoolFull
	}

	p.mu.Unlock()

	// Spawn a new pool VM (outside lock to avoid deadlock).
	sb, err := p.spawn(ctx, SpawnRequest{
		Image:    p.config.Image,
		MemoryMB: p.config.MemoryMB,
		VCPUs:    p.config.VCPUs,
	})
	if err != nil {
		return "", fmt.Errorf("spawn pool VM: %w", err)
	}

	slot := &VMSlot{
		VMID:      sb.ID,
		Provider:  sb.Provider,
		Image:     p.config.Image,
		UserCount: 1,
		MaxUsers:  p.config.MaxUsersPerVM,
		CreatedAt: time.Now(),
	}

	p.mu.Lock()
	p.vms[sb.ID] = slot
	p.mu.Unlock()

	p.logger.Info().Str("vm_id", sb.ID).Str("sandbox_id", sandboxID).Msg("created new pool VM")
	return sb.ID, nil
}

// Release decrements the user count for a VM. Returns true if the VM is now empty and should be destroyed.
func (p *VMPoolManager) Release(vmID, sandboxID string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	slot, ok := p.vms[vmID]
	if !ok {
		return false
	}

	slot.UserCount--
	p.logger.Info().Str("vm_id", vmID).Str("sandbox_id", sandboxID).Int("remaining", slot.UserCount).Msg("released sandbox from pool VM")

	if slot.UserCount <= 0 {
		delete(p.vms, vmID)
		return true // caller should destroy the VM
	}
	return false
}

// Status returns the current pool status.
func (p *VMPoolManager) Status() VMPoolStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()

	totalUsers := 0
	for _, slot := range p.vms {
		totalUsers += slot.UserCount
	}

	return VMPoolStatus{
		Enabled:       true,
		VMs:           len(p.vms),
		MaxVMs:        p.config.MaxVMs,
		TotalUsers:    totalUsers,
		MaxUsersPerVM: p.config.MaxUsersPerVM,
	}
}
