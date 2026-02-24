package store

import (
	"context"
	"time"
)

type SandboxRecord struct {
	ID        string
	State     string
	Provider  string
	Image     string
	MemoryMB  int
	VCPUs     int
	Metadata  string // JSON
	CreatedAt time.Time
	ExpiresAt time.Time
	UpdatedAt time.Time
}

type ExecLogRecord struct {
	ID        int64
	SandboxID string
	Command   string
	ExitCode  int
	Stdout    string
	Stderr    string
	Duration  string
	CreatedAt time.Time
}

type ProviderConfigRecord struct {
	Name      string
	Config    string // JSON
	Enabled   bool
	UpdatedAt time.Time
}

type TemplateRecord struct {
	Name         string
	Version      int
	Image        string
	Description  string
	Setup        string // JSON array
	AllowedHosts string // JSON array
	MemoryMB     int
	CPUCores     int
	TTLSeconds   int
	Env          string // JSON object
	Secrets      string // JSON array
	PoolSize     int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Store defines the persistence interface.
type Store interface {
	// Sandbox CRUD
	CreateSandbox(ctx context.Context, sb *SandboxRecord) error
	GetSandbox(ctx context.Context, id string) (*SandboxRecord, error)
	ListSandboxes(ctx context.Context) ([]*SandboxRecord, error)
	UpdateSandboxState(ctx context.Context, id string, state string) error
	UpdateSandboxExpiresAt(ctx context.Context, id string, expiresAt time.Time) error
	DeleteSandbox(ctx context.Context, id string) error
	ListExpiredSandboxes(ctx context.Context, before time.Time) ([]*SandboxRecord, error)

	// Exec logs
	CreateExecLog(ctx context.Context, log *ExecLogRecord) error
	ListExecLogs(ctx context.Context, sandboxID string) ([]*ExecLogRecord, error)

	// Provider configs
	GetProviderConfig(ctx context.Context, name string) (*ProviderConfigRecord, error)
	SaveProviderConfig(ctx context.Context, cfg *ProviderConfigRecord) error
	ListProviderConfigs(ctx context.Context) ([]*ProviderConfigRecord, error)

	// Templates
	CreateTemplate(ctx context.Context, t *TemplateRecord) error
	GetTemplate(ctx context.Context, name string) (*TemplateRecord, error)
	ListTemplates(ctx context.Context) ([]*TemplateRecord, error)
	UpdateTemplate(ctx context.Context, t *TemplateRecord) error
	DeleteTemplate(ctx context.Context, name string) error

	// Lifecycle
	Close() error
}
