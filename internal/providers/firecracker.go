package providers

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mainadwitiya/forgevm/internal/agentproto"
	"github.com/rs/zerolog"
)

// FirecrackerProvider manages Firecracker microVMs directly via the Firecracker REST API and a custom vsock guest agent.
type FirecrackerProvider struct {
	mu         sync.RWMutex
	vms        map[string]*vmInstance
	config     FirecrackerProviderConfig
	nextCID    atomic.Uint32
	imageCache *ImageCache
	logger     zerolog.Logger

	// snapMu guards snapshot creation to prevent concurrent builds for the same image.
	snapMu       sync.Mutex
	snapBuilding map[string]chan struct{} // rootfsSrc -> done channel
}

// snapshotInfo holds paths to a base snapshot's files.
type snapshotInfo struct {
	dir        string // snapshot directory
	vmstatePath string // CPU/device state
	memoryPath  string // full RAM snapshot
	rootfsPath  string // clean baseline rootfs
}

// snapshotMeta is persisted as meta.json in each snapshot directory.
type snapshotMeta struct {
	Image     string    `json:"image"`
	CreatedAt time.Time `json:"created_at"`
}

// FirecrackerProviderConfig holds configuration for the provider.
type FirecrackerProviderConfig struct {
	FirecrackerPath string
	KernelPath      string
	DefaultRootfs   string
	AgentPath       string
	DataDir         string
}

type vmInstance struct {
	sandboxID    string
	dir          string
	apiSockPath  string
	vsockUDSPath string
	rootfsPath   string
	cid          uint32
	process      *os.Process
	conn         net.Conn
	connMu       sync.Mutex
	consoleBuf   *RingBuffer
}

// NewFirecrackerProvider creates a new provider.
func NewFirecrackerProvider(cfg FirecrackerProviderConfig, logger zerolog.Logger) *FirecrackerProvider {
	l := logger.With().Str("provider", "firecracker").Logger()
	p := &FirecrackerProvider{
		vms:          make(map[string]*vmInstance),
		config:       cfg,
		logger:       l,
		snapBuilding: make(map[string]chan struct{}),
		imageCache: NewImageCache(
			filepath.Join(cfg.DataDir, "images"),
			cfg.AgentPath,
			0, // default disk size
			l,
		),
	}
	// CIDs start at 3 (0=hypervisor, 1=loopback, 2=host).
	p.nextCID.Store(3)
	return p
}

func (p *FirecrackerProvider) Name() string { return "firecracker" }

// snapshotDir returns the directory where snapshots for a given rootfs source are stored.
func (p *FirecrackerProvider) snapshotDir(rootfsSrc string) string {
	h := sha256.Sum256([]byte(rootfsSrc))
	return filepath.Join(p.config.DataDir, "snapshots", hex.EncodeToString(h[:8]))
}

// getSnapshot checks if a usable snapshot exists for the given rootfs source.
// Returns nil if no snapshot is available.
func (p *FirecrackerProvider) getSnapshot(rootfsSrc string) *snapshotInfo {
	dir := p.snapshotDir(rootfsSrc)
	vmstate := filepath.Join(dir, "vmstate.bin")
	memory := filepath.Join(dir, "memory.bin")
	rootfs := filepath.Join(dir, "rootfs.ext4")

	if _, err := os.Stat(vmstate); err != nil {
		return nil
	}
	if _, err := os.Stat(memory); err != nil {
		return nil
	}
	if _, err := os.Stat(rootfs); err != nil {
		return nil
	}

	return &snapshotInfo{
		dir:         dir,
		vmstatePath: vmstate,
		memoryPath:  memory,
		rootfsPath:  rootfs,
	}
}

// createBaseSnapshot boots a temporary VM from rootfsSrc, waits for the agent,
// pauses the VM, and takes a full Firecracker snapshot. The snapshot files are
// stored in snapshotDir(rootfsSrc) for later use by restoreFromSnapshot.
// The image parameter is stored in meta.json so ListSnapshots can map back to the Docker image name.
func (p *FirecrackerProvider) createBaseSnapshot(rootfsSrc, image string) error {
	snapDir := p.snapshotDir(rootfsSrc)

	// Deduplicate: if another goroutine is already building this snapshot, wait.
	p.snapMu.Lock()
	if ch, ok := p.snapBuilding[rootfsSrc]; ok {
		p.snapMu.Unlock()
		<-ch
		// Check if the other goroutine succeeded.
		if p.getSnapshot(rootfsSrc) != nil {
			return nil
		}
		return fmt.Errorf("concurrent snapshot build failed")
	}
	ch := make(chan struct{})
	p.snapBuilding[rootfsSrc] = ch
	p.snapMu.Unlock()

	defer func() {
		p.snapMu.Lock()
		delete(p.snapBuilding, rootfsSrc)
		close(ch)
		p.snapMu.Unlock()
	}()

	start := time.Now()
	p.logger.Info().Str("rootfs", rootfsSrc).Msg("snapshot: creating base snapshot")

	if err := os.MkdirAll(snapDir, 0700); err != nil {
		return fmt.Errorf("create snapshot dir: %w", err)
	}

	// Sparse-copy rootfs into snapshot dir as the baseline.
	snapRootfs := filepath.Join(snapDir, "rootfs.ext4")
	if err := copyFile(rootfsSrc, snapRootfs); err != nil {
		os.RemoveAll(snapDir)
		return fmt.Errorf("copy rootfs to snapshot dir: %w", err)
	}

	// Boot a temporary VM.
	cid := p.nextCID.Add(1) - 1
	apiSock := filepath.Join(snapDir, "api.sock")
	vsockUDS := filepath.Join(snapDir, "v.sock")

	consoleBuf := NewRingBuffer(200)
	cmd := exec.Command(p.config.FirecrackerPath, "--api-sock", apiSock)
	cmd.Dir = snapDir
	cmd.Stdout = consoleBuf
	cmd.Stderr = consoleBuf
	if err := cmd.Start(); err != nil {
		os.RemoveAll(snapDir)
		return fmt.Errorf("start firecracker for snapshot: %w", err)
	}

	// cleanup helper — kills FC process and removes snapshot dir on failure.
	cleanup := func() {
		cmd.Process.Kill()
		cmd.Process.Wait()
		os.RemoveAll(snapDir)
	}

	if err := waitForSocket(apiSock, 2*time.Second); err != nil {
		cleanup()
		return fmt.Errorf("snapshot: wait for api socket: %w", err)
	}

	ctx := context.Background()
	api := newFirecrackerAPI(apiSock)

	// Configure VM identically to a normal spawn.
	if err := api.put(ctx, "/machine-config", map[string]any{
		"vcpu_count":   1,
		"mem_size_mib": 512,
	}); err != nil {
		cleanup()
		return fmt.Errorf("snapshot: set machine config: %w", err)
	}

	if err := api.put(ctx, "/boot-source", map[string]any{
		"kernel_image_path": p.config.KernelPath,
		"boot_args":         "console=ttyS0 reboot=k panic=1 pci=off init=/usr/local/bin/forgevm-agent",
	}); err != nil {
		cleanup()
		return fmt.Errorf("snapshot: set boot source: %w", err)
	}

	// Use RELATIVE paths for rootfs and vsock so that restored VMs resolve
	// them against their own working directory (cmd.Dir).
	if err := api.put(ctx, "/drives/rootfs", map[string]any{
		"drive_id":       "rootfs",
		"path_on_host":   "rootfs.ext4",
		"is_root_device": true,
		"is_read_only":   false,
	}); err != nil {
		cleanup()
		return fmt.Errorf("snapshot: set root drive: %w", err)
	}

	if err := api.put(ctx, "/vsock", map[string]any{
		"guest_cid": cid,
		"uds_path":  "v.sock",
	}); err != nil {
		cleanup()
		return fmt.Errorf("snapshot: set vsock: %w", err)
	}

	// Boot the VM.
	if err := api.put(ctx, "/actions", map[string]any{
		"action_type": "InstanceStart",
	}); err != nil {
		cleanup()
		return fmt.Errorf("snapshot: start instance: %w", err)
	}

	// Wait for agent to be ready.
	conn, err := waitForAgent(vsockUDS, cid, 10*time.Second)
	if err != nil {
		cleanup()
		return fmt.Errorf("snapshot: wait for agent: %w", err)
	}
	conn.Close()

	// Pause the VM before snapshotting.
	if err := api.patch(ctx, "/vm", map[string]any{
		"state": "Paused",
	}); err != nil {
		cleanup()
		return fmt.Errorf("snapshot: pause VM: %w", err)
	}

	// Create the snapshot.
	if err := api.put(ctx, "/snapshot/create", map[string]any{
		"snapshot_type": "Full",
		"snapshot_path": filepath.Join(snapDir, "vmstate.bin"),
		"mem_file_path": filepath.Join(snapDir, "memory.bin"),
	}); err != nil {
		cleanup()
		return fmt.Errorf("snapshot: create snapshot: %w", err)
	}

	// Kill the temporary VM — we only keep the snapshot files.
	cmd.Process.Kill()
	cmd.Process.Wait()

	// Clean up temporary sockets, keep snapshot files.
	os.Remove(apiSock)
	os.Remove(vsockUDS)
	// Firecracker creates per-port UDS files relative to the vsock uds_path.
	os.Remove(filepath.Join(snapDir, "v.sock_1024"))

	// Write metadata so ListSnapshots can map this snapshot back to its image.
	meta := snapshotMeta{Image: image, CreatedAt: time.Now()}
	metaBytes, _ := json.Marshal(meta)
	os.WriteFile(filepath.Join(snapDir, "meta.json"), metaBytes, 0644)

	p.logger.Info().
		Dur("elapsed_ms", time.Since(start)).
		Str("dir", snapDir).
		Msg("snapshot: base snapshot created")

	return nil
}

// restoreFromSnapshot creates a new sandbox by restoring a Firecracker snapshot.
// This is the fast path (~100-200ms vs ~1.1s for cold boot).
func (p *FirecrackerProvider) restoreFromSnapshot(ctx context.Context, snap *snapshotInfo) (string, error) {
	spawnStart := time.Now()
	sandboxID := generateSandboxID()
	cid := p.nextCID.Add(1) - 1

	// Create sandbox working directory.
	dir := filepath.Join(p.config.DataDir, sandboxID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("create dir: %w", err)
	}

	// Sparse-copy the snapshot's baseline rootfs to the new sandbox.
	t0 := time.Now()
	rootfsDst := filepath.Join(dir, "rootfs.ext4")
	if err := copyFile(snap.rootfsPath, rootfsDst); err != nil {
		os.RemoveAll(dir)
		return "", fmt.Errorf("restore: copy rootfs: %w", err)
	}
	p.logger.Info().Dur("elapsed_ms", time.Since(t0)).Msg("spawn: restore copy rootfs")

	apiSockPath := filepath.Join(dir, "api.sock")
	vsockUDSPath := filepath.Join(dir, "v.sock")

	// Start a fresh Firecracker process.
	t1 := time.Now()
	consoleBuf := NewRingBuffer(1000)
	cmd := exec.Command(p.config.FirecrackerPath, "--api-sock", apiSockPath)
	cmd.Dir = dir
	cmd.Stdout = consoleBuf
	cmd.Stderr = consoleBuf
	if err := cmd.Start(); err != nil {
		os.RemoveAll(dir)
		return "", fmt.Errorf("restore: start firecracker: %w", err)
	}

	cleanup := func() {
		cmd.Process.Kill()
		cmd.Process.Wait()
		os.RemoveAll(dir)
	}

	if err := waitForSocket(apiSockPath, 2*time.Second); err != nil {
		cleanup()
		return "", fmt.Errorf("restore: wait for api socket: %w", err)
	}
	p.logger.Info().Dur("elapsed_ms", time.Since(t1)).Msg("spawn: restore start firecracker")

	api := newFirecrackerAPI(apiSockPath)

	// Load the snapshot. The snapshot was created with relative paths
	// ("rootfs.ext4", "v.sock") so Firecracker resolves them against cmd.Dir
	// which is the new sandbox directory. resume_vm: true unpauses immediately.
	t2 := time.Now()
	if err := api.put(ctx, "/snapshot/load", map[string]any{
		"snapshot_path": snap.vmstatePath,
		"mem_backend": map[string]any{
			"backend_type": "File",
			"backend_path": snap.memoryPath,
		},
		"resume_vm": true,
	}); err != nil {
		cleanup()
		return "", fmt.Errorf("restore: load snapshot: %w", err)
	}
	p.logger.Info().Dur("elapsed_ms", time.Since(t2)).Msg("spawn: snapshot-load API call")

	// Connect to the agent — it's already running inside the restored VM.
	t4 := time.Now()
	conn, err := waitForAgent(vsockUDSPath, cid, 5*time.Second)
	if err != nil {
		cleanup()
		return "", fmt.Errorf("restore: agent reconnect: %w", err)
	}
	p.logger.Info().Dur("elapsed_ms", time.Since(t4)).Msg("spawn: agent reconnect")
	p.logger.Info().Dur("total_ms", time.Since(spawnStart)).Str("sandbox", sandboxID).Msg("spawn: restore from snapshot TOTAL")

	vm := &vmInstance{
		sandboxID:    sandboxID,
		dir:          dir,
		apiSockPath:  apiSockPath,
		vsockUDSPath: vsockUDSPath,
		rootfsPath:   rootfsDst,
		cid:          cid,
		process:      cmd.Process,
		conn:         conn,
		consoleBuf:   consoleBuf,
	}

	p.mu.Lock()
	p.vms[sandboxID] = vm
	p.mu.Unlock()

	return sandboxID, nil
}

func (p *FirecrackerProvider) Spawn(ctx context.Context, opts SpawnOptions) (string, error) {
	// Determine rootfs source. If an image is specified, try to find or build
	// an image-specific rootfs. If the build fails, return an error rather than
	// silently falling back to the default rootfs (which would give the user a
	// wrong environment).
	t0 := time.Now()
	rootfsSrc := p.config.DefaultRootfs
	if opts.Image != "" {
		cached, err := p.imageCache.Get(opts.Image)
		if err != nil {
			p.logger.Error().Err(err).Str("image", opts.Image).Msg("image rootfs build failed")
			return "", fmt.Errorf("image %q: %w (pre-build with: forgevm build-image %s)", opts.Image, err, opts.Image)
		} else if cached != "" {
			rootfsSrc = cached
			p.logger.Info().Str("image", opts.Image).Str("rootfs", cached).Msg("using cached image rootfs")
		}
	}
	if rootfsSrc == "" {
		return "", fmt.Errorf("no rootfs configured (set default_rootfs in config)")
	}
	p.logger.Info().Dur("elapsed_ms", time.Since(t0)).Msg("spawn: resolve image")

	// Fast path: try snapshot restore.
	if snap := p.getSnapshot(rootfsSrc); snap != nil {
		p.logger.Info().Msg("spawn: restore from snapshot")
		id, err := p.restoreFromSnapshot(ctx, snap)
		if err != nil {
			p.logger.Warn().Err(err).Msg("spawn: snapshot restore failed, falling back to cold boot")
		} else {
			return id, nil
		}
	}

	// Cold boot path.
	spawnStart := time.Now()
	sandboxID := generateSandboxID()
	cid := p.nextCID.Add(1) - 1

	// Create working directory.
	dir := filepath.Join(p.config.DataDir, sandboxID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("create dir: %w", err)
	}

	// Copy rootfs to per-VM writable copy.
	t1 := time.Now()
	rootfsDst := filepath.Join(dir, "rootfs.ext4")
	if err := copyFile(rootfsSrc, rootfsDst); err != nil {
		os.RemoveAll(dir)
		return "", fmt.Errorf("copy rootfs: %w", err)
	}
	p.logger.Info().Dur("elapsed_ms", time.Since(t1)).Msg("spawn: copy rootfs")

	apiSockPath := filepath.Join(dir, "api.sock")
	vsockUDSPath := filepath.Join(dir, "v.sock")

	// Start firecracker process. Use Background context — the VM must outlive the spawn request.
	t2 := time.Now()
	consoleBuf := NewRingBuffer(1000)
	cmd := exec.Command(p.config.FirecrackerPath, "--api-sock", apiSockPath)
	cmd.Dir = dir
	cmd.Stdout = consoleBuf
	cmd.Stderr = consoleBuf
	if err := cmd.Start(); err != nil {
		os.RemoveAll(dir)
		return "", fmt.Errorf("start firecracker: %w", err)
	}

	// Wait for API socket to appear.
	if err := waitForSocket(apiSockPath, 2*time.Second); err != nil {
		cmd.Process.Kill()
		cmd.Process.Wait()
		os.RemoveAll(dir)
		return "", fmt.Errorf("wait for api socket: %w", err)
	}
	p.logger.Info().Dur("elapsed_ms", time.Since(t2)).Msg("spawn: start firecracker + wait for socket")

	// Configure VM via Firecracker API.
	t3 := time.Now()
	api := newFirecrackerAPI(apiSockPath)

	vcpus := opts.VCPUs
	if vcpus == 0 {
		vcpus = 1
	}
	memMB := opts.MemoryMB
	if memMB == 0 {
		memMB = 512
	}

	// Machine config.
	if err := api.put(ctx, "/machine-config", map[string]any{
		"vcpu_count":  vcpus,
		"mem_size_mib": memMB,
	}); err != nil {
		cmd.Process.Kill()
		cmd.Process.Wait()
		os.RemoveAll(dir)
		return "", fmt.Errorf("set machine config: %w", err)
	}

	// Boot source.
	if err := api.put(ctx, "/boot-source", map[string]any{
		"kernel_image_path": p.config.KernelPath,
		"boot_args":         "console=ttyS0 reboot=k panic=1 pci=off init=/usr/local/bin/forgevm-agent",
	}); err != nil {
		cmd.Process.Kill()
		cmd.Process.Wait()
		os.RemoveAll(dir)
		return "", fmt.Errorf("set boot source: %w", err)
	}

	// Root drive.
	if err := api.put(ctx, "/drives/rootfs", map[string]any{
		"drive_id":       "rootfs",
		"path_on_host":   rootfsDst,
		"is_root_device": true,
		"is_read_only":   false,
	}); err != nil {
		cmd.Process.Kill()
		cmd.Process.Wait()
		os.RemoveAll(dir)
		return "", fmt.Errorf("set root drive: %w", err)
	}

	// Vsock.
	if err := api.put(ctx, "/vsock", map[string]any{
		"guest_cid": cid,
		"uds_path":  vsockUDSPath,
	}); err != nil {
		cmd.Process.Kill()
		cmd.Process.Wait()
		os.RemoveAll(dir)
		return "", fmt.Errorf("set vsock: %w", err)
	}
	p.logger.Info().Dur("elapsed_ms", time.Since(t3)).Msg("spawn: configure VM (4 API calls)")

	// Start the VM.
	t4 := time.Now()
	if err := api.put(ctx, "/actions", map[string]any{
		"action_type": "InstanceStart",
	}); err != nil {
		cmd.Process.Kill()
		cmd.Process.Wait()
		os.RemoveAll(dir)
		return "", fmt.Errorf("start instance: %w", err)
	}
	p.logger.Info().Dur("elapsed_ms", time.Since(t4)).Msg("spawn: InstanceStart API call")

	// Wait for agent readiness over vsock UDS.
	t5 := time.Now()
	conn, err := waitForAgent(vsockUDSPath, cid, 10*time.Second)
	if err != nil {
		cmd.Process.Kill()
		cmd.Process.Wait()
		os.RemoveAll(dir)
		return "", fmt.Errorf("wait for agent: %w", err)
	}
	p.logger.Info().Dur("elapsed_ms", time.Since(t5)).Msg("spawn: wait for agent")
	p.logger.Info().Dur("total_ms", time.Since(spawnStart)).Str("sandbox", sandboxID).Msg("spawn: cold boot (no snapshot) TOTAL")

	vm := &vmInstance{
		sandboxID:    sandboxID,
		dir:          dir,
		apiSockPath:  apiSockPath,
		vsockUDSPath: vsockUDSPath,
		rootfsPath:   rootfsDst,
		cid:          cid,
		process:      cmd.Process,
		conn:         conn,
		consoleBuf:   consoleBuf,
	}

	p.mu.Lock()
	p.vms[sandboxID] = vm
	p.mu.Unlock()

	// Create a base snapshot in the background so subsequent spawns are fast.
	go func() {
		snapImage := opts.Image
		if snapImage == "" {
			snapImage = "alpine:latest"
		}
		if err := p.createBaseSnapshot(rootfsSrc, snapImage); err != nil {
			p.logger.Error().Err(err).Msg("background snapshot creation failed")
		} else {
			p.logger.Info().Msg("base snapshot created in background")
		}
	}()

	return sandboxID, nil
}

func (p *FirecrackerProvider) getVM(sandboxID string) (*vmInstance, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	vm, ok := p.vms[sandboxID]
	if !ok {
		return nil, fmt.Errorf("sandbox %q not found", sandboxID)
	}
	return vm, nil
}

func (p *FirecrackerProvider) Exec(ctx context.Context, sandboxID string, opts ExecOptions) (*ExecResult, error) {
	vm, err := p.getVM(sandboxID)
	if err != nil {
		return nil, err
	}

	params, _ := agentproto.MarshalParams(&agentproto.ExecParams{
		Command: opts.Command,
		Args:    opts.Args,
		WorkDir: opts.WorkDir,
		Env:     opts.Env,
	})

	resp, err := vm.sendRequest(&agentproto.Request{
		ID:     generateRequestID(),
		Method: agentproto.MethodExec,
		Params: params,
	})
	if err != nil {
		return nil, fmt.Errorf("agent exec: %w", err)
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("agent exec: %s", resp.Error)
	}

	var result agentproto.ExecResult
	if err := agentproto.UnmarshalResult(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("unmarshal exec result: %w", err)
	}

	return &ExecResult{
		ExitCode: result.ExitCode,
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
	}, nil
}

func (p *FirecrackerProvider) ExecStream(ctx context.Context, sandboxID string, opts ExecOptions) (<-chan StreamChunk, error) {
	vm, err := p.getVM(sandboxID)
	if err != nil {
		return nil, err
	}

	params, _ := agentproto.MarshalParams(&agentproto.ExecParams{
		Command: opts.Command,
		Args:    opts.Args,
		WorkDir: opts.WorkDir,
		Env:     opts.Env,
	})

	ch := make(chan StreamChunk, 64)

	vm.connMu.Lock()
	if err := agentproto.SendRequest(vm.conn, &agentproto.Request{
		ID:     generateRequestID(),
		Method: agentproto.MethodExecStream,
		Params: params,
	}); err != nil {
		vm.connMu.Unlock()
		return nil, fmt.Errorf("send exec_stream: %w", err)
	}

	go func() {
		defer vm.connMu.Unlock()
		defer close(ch)
		for {
			sresp, err := agentproto.ReadStreamResponse(vm.conn)
			if err != nil {
				return
			}
			if sresp.Data != "" {
				ch <- StreamChunk{Stream: sresp.Stream, Data: sresp.Data}
			}
			if sresp.Done {
				return
			}
		}
	}()

	return ch, nil
}

func (p *FirecrackerProvider) WriteFile(ctx context.Context, sandboxID string, path string, content io.Reader, mode string) error {
	vm, err := p.getVM(sandboxID)
	if err != nil {
		return err
	}

	data, err := io.ReadAll(content)
	if err != nil {
		return fmt.Errorf("read content: %w", err)
	}

	if mode == "" {
		mode = "644"
	}

	params, _ := agentproto.MarshalParams(&agentproto.WriteFileParams{
		Path:    path,
		Content: data,
		Mode:    mode,
	})

	resp, err := vm.sendRequest(&agentproto.Request{
		ID:     generateRequestID(),
		Method: agentproto.MethodWriteFile,
		Params: params,
	})
	if err != nil {
		return fmt.Errorf("agent write_file: %w", err)
	}
	if resp.Error != "" {
		return fmt.Errorf("agent write_file: %s", resp.Error)
	}
	return nil
}

func (p *FirecrackerProvider) ReadFile(ctx context.Context, sandboxID string, path string) (io.ReadCloser, error) {
	vm, err := p.getVM(sandboxID)
	if err != nil {
		return nil, err
	}

	params, _ := agentproto.MarshalParams(&agentproto.ReadFileParams{Path: path})

	resp, err := vm.sendRequest(&agentproto.Request{
		ID:     generateRequestID(),
		Method: agentproto.MethodReadFile,
		Params: params,
	})
	if err != nil {
		return nil, fmt.Errorf("agent read_file: %w", err)
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("agent read_file: %s", resp.Error)
	}

	var result agentproto.ReadFileResult
	if err := agentproto.UnmarshalResult(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("unmarshal read_file result: %w", err)
	}

	return io.NopCloser(bytes.NewReader(result.Content)), nil
}

func (p *FirecrackerProvider) ListFiles(ctx context.Context, sandboxID string, path string) ([]FileInfo, error) {
	vm, err := p.getVM(sandboxID)
	if err != nil {
		return nil, err
	}

	params, _ := agentproto.MarshalParams(&agentproto.ListFilesParams{Path: path})

	resp, err := vm.sendRequest(&agentproto.Request{
		ID:     generateRequestID(),
		Method: agentproto.MethodListFiles,
		Params: params,
	})
	if err != nil {
		return nil, fmt.Errorf("agent list_files: %w", err)
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("agent list_files: %s", resp.Error)
	}

	var result agentproto.ListFilesResult
	if err := agentproto.UnmarshalResult(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("unmarshal list_files result: %w", err)
	}

	files := make([]FileInfo, len(result.Files))
	for i, f := range result.Files {
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

func (p *FirecrackerProvider) Status(ctx context.Context, sandboxID string) (*SandboxStatus, error) {
	vm, err := p.getVM(sandboxID)
	if err != nil {
		return nil, err
	}

	state := "running"
	if err := vm.process.Signal(syscall0); err != nil {
		state = "stopped"
	}

	return &SandboxStatus{
		ID:    sandboxID,
		State: state,
	}, nil
}

func (p *FirecrackerProvider) Destroy(ctx context.Context, sandboxID string) error {
	p.mu.Lock()
	vm, ok := p.vms[sandboxID]
	if !ok {
		p.mu.Unlock()
		return fmt.Errorf("sandbox %q not found", sandboxID)
	}
	delete(p.vms, sandboxID)
	p.mu.Unlock()

	// Try graceful shutdown via agent.
	params, _ := agentproto.MarshalParams(map[string]any{})
	vm.sendRequest(&agentproto.Request{
		ID:     generateRequestID(),
		Method: agentproto.MethodShutdown,
		Params: params,
	})

	// Close vsock connection.
	if vm.conn != nil {
		vm.conn.Close()
	}

	// Kill firecracker process.
	if vm.process != nil {
		vm.process.Kill()
		vm.process.Wait()
	}

	// Clean up directory.
	os.RemoveAll(vm.dir)
	return nil
}

func (p *FirecrackerProvider) ConsoleLog(ctx context.Context, sandboxID string, lines int) ([]string, error) {
	p.mu.RLock()
	vm, ok := p.vms[sandboxID]
	p.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("sandbox %q not found", sandboxID)
	}
	return vm.consoleBuf.Lines(lines), nil
}

// ProviderConfig returns a copy of the provider's configuration.
func (p *FirecrackerProvider) ProviderConfig() FirecrackerProviderConfig {
	return p.config
}

// BuildImage pre-builds a rootfs for the given Docker image and caches it.
// Returns the path to the cached rootfs.
func (p *FirecrackerProvider) BuildImage(image string) (string, error) {
	return p.imageCache.Get(image)
}

func (p *FirecrackerProvider) Healthy(ctx context.Context) bool {
	// Check that firecracker binary exists.
	if _, err := exec.LookPath(p.config.FirecrackerPath); err != nil {
		if _, err := os.Stat(p.config.FirecrackerPath); err != nil {
			return false
		}
	}
	// Check kernel exists.
	if _, err := os.Stat(p.config.KernelPath); err != nil {
		return false
	}
	return true
}

// CheckDockerAccess logs a warning if Docker is not accessible.
// Called at startup so the user knows image builds won't work.
func (p *FirecrackerProvider) CheckDockerAccess() {
	cmd := exec.Command("docker", "info")
	if err := cmd.Run(); err != nil {
		p.logger.Warn().Msg("docker is not accessible — custom image builds will fail. Start with: sg docker -c \"./bin/forgevm serve\" or add your user to the docker group")
	} else {
		p.logger.Info().Msg("docker access confirmed — custom image builds enabled")
	}
}

// ListSnapshots returns all valid snapshots with their metadata.
func (p *FirecrackerProvider) ListSnapshots() []SnapshotSummary {
	snapRoot := filepath.Join(p.config.DataDir, "snapshots")
	entries, err := os.ReadDir(snapRoot)
	if err != nil {
		return nil
	}

	var out []SnapshotSummary
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(snapRoot, e.Name())

		// Validate all 3 required files exist.
		if _, err := os.Stat(filepath.Join(dir, "vmstate.bin")); err != nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, "memory.bin")); err != nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, "rootfs.ext4")); err != nil {
			continue
		}

		// Read metadata.
		metaPath := filepath.Join(dir, "meta.json")
		data, err := os.ReadFile(metaPath)
		if err != nil {
			continue
		}
		var meta snapshotMeta
		if err := json.Unmarshal(data, &meta); err != nil {
			continue
		}

		out = append(out, SnapshotSummary{
			Image:     meta.Image,
			Provider:  p.Name(),
			CreatedAt: meta.CreatedAt,
		})
	}
	return out
}

// sendRequest serializes access to the vsock connection and sends a request/response pair.
func (vm *vmInstance) sendRequest(req *agentproto.Request) (*agentproto.Response, error) {
	vm.connMu.Lock()
	defer vm.connMu.Unlock()

	if err := agentproto.SendRequest(vm.conn, req); err != nil {
		return nil, err
	}
	return agentproto.ReadResponse(vm.conn)
}

// --- helpers ---

// syscall0 is os.Signal(syscall.Signal(0)) for checking process liveness.
var syscall0 = os.Signal(signalZero(0))

type signalZero int

func (signalZero) Signal() {}
func (signalZero) String() string { return "signal 0" }

func generateRequestID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func waitForSocket(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		time.Sleep(5 * time.Millisecond)
	}
	return fmt.Errorf("socket %s did not appear within %s", path, timeout)
}

func waitForAgent(vsockUDSPath string, cid uint32, timeout time.Duration) (net.Conn, error) {
	// Firecracker exposes vsock via a host-side Unix socket.
	// To reach a guest vsock port, the host connects to the UDS and sends
	// "CONNECT {port}\n". Firecracker responds with "OK {id}\n" on success.
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("unix", vsockUDSPath, 1*time.Second)
		if err != nil {
			lastErr = err
			time.Sleep(10 * time.Millisecond)
			continue
		}

		// Send CONNECT to reach guest agent on port 1024.
		if _, err := fmt.Fprintf(conn, "CONNECT 1024\n"); err != nil {
			conn.Close()
			lastErr = err
			time.Sleep(10 * time.Millisecond)
			continue
		}

		// Read "OK {id}\n" response.
		buf := make([]byte, 64)
		n, err := conn.Read(buf)
		if err != nil || n == 0 {
			conn.Close()
			lastErr = fmt.Errorf("vsock CONNECT: no response")
			time.Sleep(10 * time.Millisecond)
			continue
		}
		resp := string(buf[:n])
		if len(resp) < 2 || resp[:2] != "OK" {
			conn.Close()
			lastErr = fmt.Errorf("vsock CONNECT failed: %s", resp)
			time.Sleep(10 * time.Millisecond)
			continue
		}

		// Send ping to verify agent is responsive.
		params, _ := json.Marshal(map[string]any{})
		if err := agentproto.SendRequest(conn, &agentproto.Request{
			ID:     generateRequestID(),
			Method: agentproto.MethodPing,
			Params: params,
		}); err != nil {
			conn.Close()
			lastErr = err
			time.Sleep(10 * time.Millisecond)
			continue
		}

		pingResp, err := agentproto.ReadResponse(conn)
		if err != nil {
			conn.Close()
			lastErr = err
			time.Sleep(10 * time.Millisecond)
			continue
		}

		if pingResp.Error != "" {
			conn.Close()
			lastErr = fmt.Errorf("ping error: %s", pingResp.Error)
			time.Sleep(10 * time.Millisecond)
			continue
		}

		return conn, nil
	}
	return nil, fmt.Errorf("agent not ready after %s: %w", timeout, lastErr)
}

func copyFile(src, dst string) error {
	// Use cp --sparse=always to skip zero blocks. For a 1GB rootfs with ~100MB
	// of actual data, this reduces copy time from ~900ms to ~100ms.
	cmd := exec.Command("cp", "--sparse=always", src, dst)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("cp --sparse=always: %s: %w", string(out), err)
	}
	return nil
}
