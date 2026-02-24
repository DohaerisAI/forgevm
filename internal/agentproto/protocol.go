// Package agentproto defines the wire protocol between the ForgeVM host
// and the guest agent running inside Firecracker VMs.
// Messages are framed as 4-byte big-endian length prefix + JSON payload.
package agentproto

import "encoding/json"

// Method constants.
const (
	MethodPing       = "ping"
	MethodExec       = "exec"
	MethodExecStream = "exec_stream"
	MethodWriteFile  = "write_file"
	MethodReadFile   = "read_file"
	MethodListFiles  = "list_files"
	MethodShutdown   = "shutdown"
)

// Request is the envelope sent from host to agent.
type Request struct {
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Response is the envelope sent from agent to host for non-streaming methods.
type Response struct {
	ID     string          `json:"id"`
	Error  string          `json:"error,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
}

// StreamResponse is sent from agent to host for exec_stream, one per chunk.
type StreamResponse struct {
	ID       string `json:"id"`
	Stream   string `json:"stream,omitempty"`   // "stdout" or "stderr"
	Data     string `json:"data,omitempty"`
	Error    string `json:"error,omitempty"`
	Done     bool   `json:"done,omitempty"`
	ExitCode int    `json:"exit_code,omitempty"`
}

// PingResult is returned by the ping method.
type PingResult struct {
	Pong bool `json:"pong"`
}

// ExecParams are parameters for exec and exec_stream methods.
type ExecParams struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	WorkDir string            `json:"work_dir,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// ExecResult is returned by the exec method.
type ExecResult struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

// WriteFileParams are parameters for write_file.
type WriteFileParams struct {
	Path    string `json:"path"`
	Content []byte `json:"content"`
	Mode    string `json:"mode,omitempty"`
}

// ReadFileParams are parameters for read_file.
type ReadFileParams struct {
	Path string `json:"path"`
}

// ReadFileResult is returned by read_file.
type ReadFileResult struct {
	Content []byte `json:"content"`
}

// ListFilesParams are parameters for list_files.
type ListFilesParams struct {
	Path string `json:"path"`
}

// ListFilesResult is returned by list_files.
type ListFilesResult struct {
	Files []FileInfoResult `json:"files"`
}

// FileInfoResult describes a single file entry.
type FileInfoResult struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	Mode    string `json:"mode"`
	IsDir   bool   `json:"is_dir"`
	ModTime string `json:"mod_time"`
}
