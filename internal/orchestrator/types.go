package orchestrator

import "time"

type SandboxState string

const (
	StateCreating  SandboxState = "creating"
	StateRunning   SandboxState = "running"
	StateIdle      SandboxState = "idle"
	StateDestroyed SandboxState = "destroyed"
	StateError     SandboxState = "error"
)

type Sandbox struct {
	ID        string       `json:"id"`
	State     SandboxState `json:"state"`
	Provider  string       `json:"provider"`
	Image     string       `json:"image"`
	MemoryMB  int          `json:"memory_mb"`
	VCPUs     int          `json:"vcpus"`
	OwnerID   string       `json:"owner_id,omitempty"`
	VMID      string       `json:"vm_id,omitempty"`
	CreatedAt time.Time    `json:"created_at"`
	ExpiresAt time.Time    `json:"expires_at"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type SpawnRequest struct {
	Image    string            `json:"image"`
	Provider string            `json:"provider,omitempty"`
	MemoryMB int               `json:"memory_mb,omitempty"`
	VCPUs    int               `json:"vcpus,omitempty"`
	TTL      string            `json:"ttl,omitempty"`
	Template string            `json:"template,omitempty"`
	OwnerID  string            `json:"owner_id,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type ExecRequest struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	WorkDir string            `json:"workdir,omitempty"`
	Timeout string            `json:"timeout,omitempty"`
	Stream  bool              `json:"stream,omitempty"`
}

type ExecResult struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Duration string `json:"duration"`
}

type FileWriteRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Mode    string `json:"mode,omitempty"`
}

type FileInfo struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	Mode    string `json:"mode"`
	IsDir   bool   `json:"is_dir"`
	ModTime string `json:"mod_time"`
}

type FileDeleteRequest struct {
	Path      string `json:"path"`
	Recursive bool   `json:"recursive"`
}

type FileMoveRequest struct {
	OldPath string `json:"old_path"`
	NewPath string `json:"new_path"`
}

type FileChmodRequest struct {
	Path string `json:"path"`
	Mode string `json:"mode"`
}

type SandboxInfo struct {
	ID         string            `json:"id"`
	State      SandboxState      `json:"state"`
	Provider   string            `json:"provider"`
	Image      string            `json:"image"`
	MemoryMB   int               `json:"memory_mb"`
	VCPUs      int               `json:"vcpus"`
	CreatedAt  string            `json:"created_at"`
	ExpiresAt  string            `json:"expires_at"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	ExecCount  int               `json:"exec_count"`
	FileCount  int               `json:"file_count"`
}
