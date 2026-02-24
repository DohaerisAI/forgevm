package orchestrator

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// PoolManager maintains pre-warmed sandbox pools per template.
type PoolManager struct {
	mu       sync.RWMutex
	pools    map[string]*pool // templateName -> pool
	manager  *Manager
	registry *TemplateRegistry
	logger   zerolog.Logger

	ctx    context.Context
	cancel context.CancelFunc
}

type pool struct {
	templateName string
	targetSize   int
	sandboxes    []string // sandbox IDs ready to be handed off
}

func NewPoolManager(mgr *Manager, registry *TemplateRegistry, logger zerolog.Logger) *PoolManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &PoolManager{
		pools:    make(map[string]*pool),
		manager:  mgr,
		registry: registry,
		logger:   logger.With().Str("component", "pool").Logger(),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start begins the background refill goroutine.
func (pm *PoolManager) Start() {
	go pm.refillLoop()
}

// Stop halts the pool manager.
func (pm *PoolManager) Stop() {
	pm.cancel()
}

// Configure sets the pool size for a template.
func (pm *PoolManager) Configure(templateName string, size int) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if p, ok := pm.pools[templateName]; ok {
		p.targetSize = size
	} else {
		pm.pools[templateName] = &pool{
			templateName: templateName,
			targetSize:   size,
			sandboxes:    make([]string, 0),
		}
	}
}

// Acquire takes a pre-warmed sandbox from the pool. Returns empty string if none available.
func (pm *PoolManager) Acquire(templateName string) string {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	p, ok := pm.pools[templateName]
	if !ok || len(p.sandboxes) == 0 {
		return ""
	}
	id := p.sandboxes[0]
	p.sandboxes = p.sandboxes[1:]
	pm.logger.Info().Str("template", templateName).Str("sandbox", id).Int("remaining", len(p.sandboxes)).Msg("sandbox acquired from pool")
	return id
}

// Status returns pool stats for all templates.
func (pm *PoolManager) Status() map[string]PoolStatus {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	status := make(map[string]PoolStatus)
	for name, p := range pm.pools {
		status[name] = PoolStatus{
			TemplateName: name,
			TargetSize:   p.targetSize,
			CurrentSize:  len(p.sandboxes),
		}
	}
	return status
}

// PoolStatus represents the state of a template's pool.
type PoolStatus struct {
	TemplateName string `json:"template_name"`
	TargetSize   int    `json:"target_size"`
	CurrentSize  int    `json:"current_size"`
}

func (pm *PoolManager) refillLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-pm.ctx.Done():
			return
		case <-ticker.C:
			pm.refill()
		}
	}
}

func (pm *PoolManager) refill() {
	pm.mu.RLock()
	var toRefill []struct {
		name string
		need int
	}
	for name, p := range pm.pools {
		if p.targetSize > 0 && len(p.sandboxes) < p.targetSize {
			toRefill = append(toRefill, struct {
				name string
				need int
			}{name, p.targetSize - len(p.sandboxes)})
		}
	}
	pm.mu.RUnlock()

	for _, item := range toRefill {
		for range item.need {
			tmpl, err := pm.registry.Get(pm.ctx, item.name)
			if err != nil {
				pm.logger.Error().Err(err).Str("template", item.name).Msg("refill: failed to get template")
				break
			}
			req := pm.registry.ToSpawnRequest(tmpl)
			sb, err := pm.manager.Spawn(pm.ctx, req)
			if err != nil {
				pm.logger.Error().Err(err).Str("template", item.name).Msg("refill: failed to spawn sandbox")
				break
			}
			pm.mu.Lock()
			if p, ok := pm.pools[item.name]; ok {
				p.sandboxes = append(p.sandboxes, sb.ID)
			}
			pm.mu.Unlock()
			pm.logger.Info().Str("template", item.name).Str("sandbox", sb.ID).Msg("pool refilled")
		}
	}
}
