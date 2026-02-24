package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mainadwitiya/forgevm/internal/store"
)

// Template represents a reusable sandbox configuration.
type Template struct {
	Name         string            `json:"name" yaml:"name"`
	Version      int               `json:"version" yaml:"version"`
	Image        string            `json:"image" yaml:"image"`
	Description  string            `json:"description,omitempty" yaml:"description"`
	Setup        []string          `json:"setup,omitempty" yaml:"setup"`
	AllowedHosts []string          `json:"allowed_hosts,omitempty" yaml:"allowed_hosts"`
	MemoryMB     int               `json:"memory_mb" yaml:"memory_mb"`
	CPUCores     int               `json:"cpu_cores" yaml:"cpu_cores"`
	TTLSeconds   int               `json:"ttl_seconds" yaml:"ttl_seconds"`
	Env          map[string]string `json:"env,omitempty" yaml:"env"`
	Secrets      []SecretConfig    `json:"secrets,omitempty" yaml:"secrets"`
	PoolSize     int               `json:"pool_size" yaml:"pool_size"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

// SecretConfig defines a secret to inject into a sandbox.
type SecretConfig struct {
	Name     string `json:"name" yaml:"name"`
	InjectAt string `json:"inject_at" yaml:"inject_at"`
}

// TemplateRegistry manages sandbox templates via the store.
type TemplateRegistry struct {
	store store.Store
}

func NewTemplateRegistry(st store.Store) *TemplateRegistry {
	return &TemplateRegistry{store: st}
}

func (r *TemplateRegistry) Create(ctx context.Context, t *Template) error {
	if t.Name == "" {
		return fmt.Errorf("template name is required")
	}
	if t.Image == "" {
		return fmt.Errorf("template image is required")
	}
	if t.Version == 0 {
		t.Version = 1
	}
	if t.MemoryMB == 0 {
		t.MemoryMB = 512
	}
	if t.CPUCores == 0 {
		t.CPUCores = 1
	}
	if t.TTLSeconds == 0 {
		t.TTLSeconds = 300
	}

	now := time.Now()
	t.CreatedAt = now
	t.UpdatedAt = now

	return r.store.CreateTemplate(ctx, templateToRecord(t))
}

func (r *TemplateRegistry) Get(ctx context.Context, name string) (*Template, error) {
	rec, err := r.store.GetTemplate(ctx, name)
	if err != nil {
		return nil, err
	}
	return recordToTemplate(rec), nil
}

func (r *TemplateRegistry) List(ctx context.Context) ([]*Template, error) {
	records, err := r.store.ListTemplates(ctx)
	if err != nil {
		return nil, err
	}
	templates := make([]*Template, len(records))
	for i, rec := range records {
		templates[i] = recordToTemplate(rec)
	}
	return templates, nil
}

func (r *TemplateRegistry) Update(ctx context.Context, t *Template) error {
	if t.Name == "" {
		return fmt.Errorf("template name is required")
	}
	t.UpdatedAt = time.Now()
	return r.store.UpdateTemplate(ctx, templateToRecord(t))
}

func (r *TemplateRegistry) Delete(ctx context.Context, name string) error {
	return r.store.DeleteTemplate(ctx, name)
}

// ToSpawnRequest converts a template into a SpawnRequest.
func (r *TemplateRegistry) ToSpawnRequest(t *Template) SpawnRequest {
	ttl := fmt.Sprintf("%ds", t.TTLSeconds)
	return SpawnRequest{
		Image:    t.Image,
		Template: t.Name,
		MemoryMB: t.MemoryMB,
		VCPUs:    t.CPUCores,
		TTL:      ttl,
		Metadata: map[string]string{"template": t.Name},
	}
}

func templateToRecord(t *Template) *store.TemplateRecord {
	setup, _ := json.Marshal(t.Setup)
	hosts, _ := json.Marshal(t.AllowedHosts)
	env, _ := json.Marshal(t.Env)
	secrets, _ := json.Marshal(t.Secrets)
	return &store.TemplateRecord{
		Name:         t.Name,
		Version:      t.Version,
		Image:        t.Image,
		Description:  t.Description,
		Setup:        string(setup),
		AllowedHosts: string(hosts),
		MemoryMB:     t.MemoryMB,
		CPUCores:     t.CPUCores,
		TTLSeconds:   t.TTLSeconds,
		Env:          string(env),
		Secrets:      string(secrets),
		PoolSize:     t.PoolSize,
		CreatedAt:    t.CreatedAt,
		UpdatedAt:    t.UpdatedAt,
	}
}

func recordToTemplate(r *store.TemplateRecord) *Template {
	var setup []string
	var hosts []string
	var env map[string]string
	var secrets []SecretConfig
	json.Unmarshal([]byte(r.Setup), &setup)
	json.Unmarshal([]byte(r.AllowedHosts), &hosts)
	json.Unmarshal([]byte(r.Env), &env)
	json.Unmarshal([]byte(r.Secrets), &secrets)
	return &Template{
		Name:         r.Name,
		Version:      r.Version,
		Image:        r.Image,
		Description:  r.Description,
		Setup:        setup,
		AllowedHosts: hosts,
		MemoryMB:     r.MemoryMB,
		CPUCores:     r.CPUCores,
		TTLSeconds:   r.TTLSeconds,
		Env:          env,
		Secrets:      secrets,
		PoolSize:     r.PoolSize,
		CreatedAt:    r.CreatedAt,
		UpdatedAt:    r.UpdatedAt,
	}
}
