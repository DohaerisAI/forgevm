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
	OwnerID   string
	VMID      string
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

type EnvironmentSpecRecord struct {
	ID             string
	OwnerID        string
	Name           string
	BaseImage      string
	PythonPackages string // JSON array
	AptPackages    string // JSON array
	PythonVersion  string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type EnvironmentBuildRecord struct {
	ID             string
	SpecID         string
	Status         string
	CurrentStep    string
	LogBlob        string
	ImageSizeBytes int64
	DigestLocal    string
	Error          string
	CreatedAt      time.Time
	FinishedAt     *time.Time
	UpdatedAt      time.Time
}

type EnvironmentArtifactRecord struct {
	ID        int64
	BuildID   string
	Target    string
	ImageRef  string
	Digest    string
	Status    string
	Error     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type RegistryConnectionRecord struct {
	ID        string
	OwnerID   string
	Provider  string
	Username  string
	SecretRef string
	IsDefault bool
	CreatedAt time.Time
	UpdatedAt time.Time
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
	ListSandboxesByOwner(ctx context.Context, ownerID string) ([]*SandboxRecord, error)
	CountSandboxesByVM(ctx context.Context, vmID string) (int, error)

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

	// Environment specs
	CreateEnvironmentSpec(ctx context.Context, spec *EnvironmentSpecRecord) error
	GetEnvironmentSpec(ctx context.Context, id string) (*EnvironmentSpecRecord, error)
	ListEnvironmentSpecs(ctx context.Context, ownerID string) ([]*EnvironmentSpecRecord, error)
	UpdateEnvironmentSpec(ctx context.Context, spec *EnvironmentSpecRecord) error
	DeleteEnvironmentSpec(ctx context.Context, id string) error

	// Environment builds
	CreateEnvironmentBuild(ctx context.Context, build *EnvironmentBuildRecord) error
	GetEnvironmentBuild(ctx context.Context, id string) (*EnvironmentBuildRecord, error)
	ListEnvironmentBuilds(ctx context.Context, specID string) ([]*EnvironmentBuildRecord, error)
	UpdateEnvironmentBuild(ctx context.Context, build *EnvironmentBuildRecord) error

	// Environment artifacts
	SaveEnvironmentArtifact(ctx context.Context, artifact *EnvironmentArtifactRecord) error
	ListEnvironmentArtifacts(ctx context.Context, buildID string) ([]*EnvironmentArtifactRecord, error)

	// Registry connections
	SaveRegistryConnection(ctx context.Context, conn *RegistryConnectionRecord) error
	GetRegistryConnection(ctx context.Context, id string) (*RegistryConnectionRecord, error)
	ListRegistryConnections(ctx context.Context, ownerID string) ([]*RegistryConnectionRecord, error)
	DeleteRegistryConnection(ctx context.Context, id string) error

	// Lifecycle
	Close() error
}
