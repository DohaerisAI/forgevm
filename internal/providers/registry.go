package providers

import (
	"fmt"
	"sync"
)

// Registry manages available providers.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
	dflt      string
}

func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]Provider),
	}
}

// Register adds a provider to the registry.
func (r *Registry) Register(p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[p.Name()] = p
}

// SetDefault sets the default provider name.
func (r *Registry) SetDefault(name string) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if _, ok := r.providers[name]; !ok {
		return fmt.Errorf("provider %q not registered", name)
	}
	r.dflt = name
	return nil
}

// Get returns a provider by name, or the default if name is empty.
func (r *Registry) Get(name string) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if name == "" {
		name = r.dflt
	}
	if name == "" {
		return nil, fmt.Errorf("no provider specified and no default set")
	}

	p, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("provider %q not found", name)
	}
	return p, nil
}

// List returns all registered provider names.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}

// Default returns the default provider name.
func (r *Registry) Default() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.dflt
}
