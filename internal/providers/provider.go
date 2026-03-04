package providers

import (
	"context"
	"io"
	"time"
)

type SpawnOptions struct {
	Image      string
	MemoryMB   int
	VCPUs      int
	DiskSizeMB int
	Metadata   map[string]string
}

type ExecOptions struct {
	Command string
	Args    []string
	Env     map[string]string
	WorkDir string
}

type ExecResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

type StreamChunk struct {
	Stream string `json:"stream"` // "stdout" or "stderr"
	Data   string `json:"data"`
}

type FileInfo struct {
	Path    string
	Size    int64
	Mode    string
	IsDir   bool
	ModTime string
}

type SandboxStatus struct {
	ID    string
	State string
}

// Provider defines the interface for sandbox execution backends.
type Provider interface {
	// Name returns the unique provider identifier.
	Name() string

	// Spawn creates a new sandbox and returns its ID.
	Spawn(ctx context.Context, opts SpawnOptions) (string, error)

	// Exec runs a command in the sandbox and returns the result.
	Exec(ctx context.Context, sandboxID string, opts ExecOptions) (*ExecResult, error)

	// ExecStream runs a command and streams output chunks.
	ExecStream(ctx context.Context, sandboxID string, opts ExecOptions) (<-chan StreamChunk, error)

	// WriteFile writes content to a file in the sandbox.
	WriteFile(ctx context.Context, sandboxID string, path string, content io.Reader, mode string) error

	// ReadFile reads a file from the sandbox.
	ReadFile(ctx context.Context, sandboxID string, path string) (io.ReadCloser, error)

	// ListFiles lists files at the given path in the sandbox.
	ListFiles(ctx context.Context, sandboxID string, path string) ([]FileInfo, error)

	// DeleteFile deletes a file or directory in the sandbox.
	DeleteFile(ctx context.Context, sandboxID string, path string, recursive bool) error

	// MoveFile moves/renames a file in the sandbox.
	MoveFile(ctx context.Context, sandboxID string, oldPath, newPath string) error

	// ChmodFile changes file permissions in the sandbox.
	ChmodFile(ctx context.Context, sandboxID string, path string, mode string) error

	// StatFile returns info about a single file in the sandbox.
	StatFile(ctx context.Context, sandboxID string, path string) (*FileInfo, error)

	// GlobFiles returns paths matching a glob pattern in the sandbox.
	GlobFiles(ctx context.Context, sandboxID string, pattern string) ([]string, error)

	// Status returns the current status of a sandbox.
	Status(ctx context.Context, sandboxID string) (*SandboxStatus, error)

	// Destroy tears down a sandbox.
	Destroy(ctx context.Context, sandboxID string) error

	// ConsoleLog returns the last n lines of the sandbox's console output.
	ConsoleLog(ctx context.Context, sandboxID string, lines int) ([]string, error)

	// Healthy returns true if the provider is operational.
	Healthy(ctx context.Context) bool
}

// SnapshotSummary describes a pre-built VM snapshot available for fast restore.
type SnapshotSummary struct {
	Image     string    `json:"image"`
	Provider  string    `json:"provider"`
	CreatedAt time.Time `json:"created_at"`
}

// SnapshotLister is an optional interface that providers can implement
// to expose their cached snapshots.
type SnapshotLister interface {
	ListSnapshots() []SnapshotSummary
}
