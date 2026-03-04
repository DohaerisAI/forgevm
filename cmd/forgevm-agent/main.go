// forgevm-agent runs inside a Firecracker VM and serves exec/file requests
// over vsock. Build with: GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/forgevm-agent ./cmd/forgevm-agent
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
	"unsafe"

	"github.com/DohaerisAI/forgevm/internal/agentproto"
	"github.com/mdlayher/vsock"
)

const vsockPort = 1024

func main() {
	// If running as PID 1 inside a VM, set up basic mounts.
	if os.Getpid() == 1 {
		setupInit()
	}

	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Printf("forgevm-agent starting on vsock port %d", vsockPort)

	l, err := vsock.Listen(vsockPort, nil)
	if err != nil {
		log.Fatalf("vsock listen: %v", err)
	}
	defer l.Close()

	log.Println("listening for connections")
	for {
		conn, err := l.Accept()
		if err != nil {
			log.Printf("accept error: %v", err)
			continue
		}
		go handleConn(conn)
	}
}

func setupInit() {
	// Set a sane PATH — minimal init environments have none.
	os.Setenv("PATH", "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin")

	mounts := []struct {
		source, target, fstype string
	}{
		{"proc", "/proc", "proc"},
		{"sysfs", "/sys", "sysfs"},
		{"devtmpfs", "/dev", "devtmpfs"},
	}
	for _, m := range mounts {
		os.MkdirAll(m.target, 0755)
		if err := syscall.Mount(m.source, m.target, m.fstype, 0, ""); err != nil {
			// Not fatal — may already be mounted.
			log.Printf("mount %s: %v", m.target, err)
		}
	}

	// Seed entropy — Firecracker VMs start with zero entropy which causes
	// getrandom() to block forever (breaks numpy, pandas, etc.).
	seedEntropy()
}

// seedEntropy writes random bytes to /dev/urandom and credits them to the
// kernel entropy pool via RNDADDTOENTCNT ioctl. Without this, getrandom()
// blocks indefinitely in Firecracker VMs that have no hardware RNG.
func seedEntropy() {
	const RNDADDTOENTCNT = 0x40045201

	f, err := os.OpenFile("/dev/urandom", os.O_WRONLY, 0)
	if err != nil {
		log.Printf("seed entropy: open /dev/urandom: %v", err)
		return
	}
	defer f.Close()

	// Generate seed from time + PID (can't use crypto/rand — it also needs entropy).
	seed := make([]byte, 512)
	t := time.Now().UnixNano() ^ int64(os.Getpid())
	for i := range seed {
		t = t*6364136223846793005 + 1442695040888963407
		seed[i] = byte(t >> 33)
	}

	if _, err := f.Write(seed); err != nil {
		log.Printf("seed entropy: write: %v", err)
		return
	}

	// Credit the entropy pool so getrandom() unblocks.
	bits := int32(len(seed) * 8)
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), RNDADDTOENTCNT, uintptr(unsafe.Pointer(&bits))); errno != 0 {
		log.Printf("seed entropy: ioctl RNDADDTOENTCNT: %v", errno)
		return
	}

	log.Printf("seeded entropy pool with %d bytes (%d bits)", len(seed), bits)
}

func handleConn(conn net.Conn) {
	defer conn.Close()
	log.Printf("new connection from %s", conn.RemoteAddr())

	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)

	for {
		req, err := agentproto.ReadRequest(r)
		if err != nil {
			if err != io.EOF {
				log.Printf("read request: %v", err)
			}
			return
		}

		switch req.Method {
		case agentproto.MethodPing:
			handlePing(w, req)
		case agentproto.MethodExec:
			handleExec(w, req)
		case agentproto.MethodExecStream:
			handleExecStream(w, req)
		case agentproto.MethodWriteFile:
			handleWriteFile(w, req)
		case agentproto.MethodReadFile:
			handleReadFile(w, req)
		case agentproto.MethodListFiles:
			handleListFiles(w, req)
		case agentproto.MethodDeleteFile:
			handleDeleteFile(w, req)
		case agentproto.MethodMoveFile:
			handleMoveFile(w, req)
		case agentproto.MethodChmodFile:
			handleChmodFile(w, req)
		case agentproto.MethodStatFile:
			handleStatFile(w, req)
		case agentproto.MethodGlobFiles:
			handleGlobFiles(w, req)
		case agentproto.MethodShutdown:
			handleShutdown(w, req)
		default:
			sendError(w, req.ID, fmt.Sprintf("unknown method: %s", req.Method))
		}
		w.Flush()
	}
}

func handlePing(w io.Writer, req *agentproto.Request) {
	result, _ := agentproto.MarshalResult(&agentproto.PingResult{Pong: true})
	agentproto.SendResponse(w, &agentproto.Response{
		ID:     req.ID,
		Result: result,
	})
}

func handleExec(w io.Writer, req *agentproto.Request) {
	var params agentproto.ExecParams
	if err := agentproto.UnmarshalParams(req.Params, &params); err != nil {
		sendError(w, req.ID, fmt.Sprintf("bad params: %v", err))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", params.Command)
	if params.WorkDir != "" {
		cmd.Dir = params.WorkDir
	}
	for k, v := range params.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	if len(cmd.Env) > 0 {
		cmd.Env = append(os.Environ(), cmd.Env...)
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			sendError(w, req.ID, fmt.Sprintf("exec: %v", err))
			return
		}
	}

	result, _ := agentproto.MarshalResult(&agentproto.ExecResult{
		ExitCode: exitCode,
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
	})
	agentproto.SendResponse(w, &agentproto.Response{
		ID:     req.ID,
		Result: result,
	})
}

func handleExecStream(w io.Writer, req *agentproto.Request) {
	var params agentproto.ExecParams
	if err := agentproto.UnmarshalParams(req.Params, &params); err != nil {
		sendError(w, req.ID, fmt.Sprintf("bad params: %v", err))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", params.Command)
	if params.WorkDir != "" {
		cmd.Dir = params.WorkDir
	}
	for k, v := range params.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	if len(cmd.Env) > 0 {
		cmd.Env = append(os.Environ(), cmd.Env...)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		agentproto.SendStreamResponse(w, &agentproto.StreamResponse{
			ID: req.ID, Error: fmt.Sprintf("stdout pipe: %v", err), Done: true,
		})
		return
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		agentproto.SendStreamResponse(w, &agentproto.StreamResponse{
			ID: req.ID, Error: fmt.Sprintf("stderr pipe: %v", err), Done: true,
		})
		return
	}

	if err := cmd.Start(); err != nil {
		agentproto.SendStreamResponse(w, &agentproto.StreamResponse{
			ID: req.ID, Error: fmt.Sprintf("start: %v", err), Done: true,
		})
		return
	}

	// Stream stdout and stderr concurrently.
	done := make(chan struct{}, 2)
	streamPipe := func(pipe io.Reader, stream string) {
		defer func() { done <- struct{}{} }()
		buf := make([]byte, 4096)
		for {
			n, err := pipe.Read(buf)
			if n > 0 {
				agentproto.SendStreamResponse(w, &agentproto.StreamResponse{
					ID: req.ID, Stream: stream, Data: string(buf[:n]),
				})
			}
			if err != nil {
				return
			}
		}
	}
	go streamPipe(stdoutPipe, "stdout")
	go streamPipe(stderrPipe, "stderr")
	<-done
	<-done

	exitCode := 0
	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}

	agentproto.SendStreamResponse(w, &agentproto.StreamResponse{
		ID: req.ID, Done: true, ExitCode: exitCode,
	})
}

func handleWriteFile(w io.Writer, req *agentproto.Request) {
	var params agentproto.WriteFileParams
	if err := agentproto.UnmarshalParams(req.Params, &params); err != nil {
		sendError(w, req.ID, fmt.Sprintf("bad params: %v", err))
		return
	}

	if err := os.MkdirAll(filepath.Dir(params.Path), 0755); err != nil {
		sendError(w, req.ID, fmt.Sprintf("mkdir: %v", err))
		return
	}

	mode := os.FileMode(0644)
	if params.Mode != "" {
		if m, err := strconv.ParseUint(params.Mode, 8, 32); err == nil {
			mode = os.FileMode(m)
		}
	}

	if err := os.WriteFile(params.Path, params.Content, mode); err != nil {
		sendError(w, req.ID, fmt.Sprintf("write: %v", err))
		return
	}

	result, _ := agentproto.MarshalResult(map[string]bool{"ok": true})
	agentproto.SendResponse(w, &agentproto.Response{ID: req.ID, Result: result})
}

func handleReadFile(w io.Writer, req *agentproto.Request) {
	var params agentproto.ReadFileParams
	if err := agentproto.UnmarshalParams(req.Params, &params); err != nil {
		sendError(w, req.ID, fmt.Sprintf("bad params: %v", err))
		return
	}

	data, err := os.ReadFile(params.Path)
	if err != nil {
		sendError(w, req.ID, fmt.Sprintf("read: %v", err))
		return
	}

	result, _ := agentproto.MarshalResult(&agentproto.ReadFileResult{Content: data})
	agentproto.SendResponse(w, &agentproto.Response{ID: req.ID, Result: result})
}

func handleListFiles(w io.Writer, req *agentproto.Request) {
	var params agentproto.ListFilesParams
	if err := agentproto.UnmarshalParams(req.Params, &params); err != nil {
		sendError(w, req.ID, fmt.Sprintf("bad params: %v", err))
		return
	}

	entries, err := os.ReadDir(params.Path)
	if err != nil {
		sendError(w, req.ID, fmt.Sprintf("readdir: %v", err))
		return
	}

	files := make([]agentproto.FileInfoResult, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, agentproto.FileInfoResult{
			Path:    filepath.Join(params.Path, e.Name()),
			Size:    info.Size(),
			Mode:    fmt.Sprintf("%o", info.Mode()),
			IsDir:   e.IsDir(),
			ModTime: info.ModTime().UTC().Format(time.RFC3339),
		})
	}

	result, _ := agentproto.MarshalResult(&agentproto.ListFilesResult{Files: files})
	agentproto.SendResponse(w, &agentproto.Response{ID: req.ID, Result: result})
}

func handleDeleteFile(w io.Writer, req *agentproto.Request) {
	var params agentproto.DeleteFileParams
	if err := agentproto.UnmarshalParams(req.Params, &params); err != nil {
		sendError(w, req.ID, fmt.Sprintf("bad params: %v", err))
		return
	}

	var err error
	if params.Recursive {
		err = os.RemoveAll(params.Path)
	} else {
		err = os.Remove(params.Path)
	}
	if err != nil {
		sendError(w, req.ID, fmt.Sprintf("delete: %v", err))
		return
	}

	result, _ := agentproto.MarshalResult(map[string]bool{"ok": true})
	agentproto.SendResponse(w, &agentproto.Response{ID: req.ID, Result: result})
}

func handleMoveFile(w io.Writer, req *agentproto.Request) {
	var params agentproto.MoveFileParams
	if err := agentproto.UnmarshalParams(req.Params, &params); err != nil {
		sendError(w, req.ID, fmt.Sprintf("bad params: %v", err))
		return
	}

	if err := os.MkdirAll(filepath.Dir(params.NewPath), 0755); err != nil {
		sendError(w, req.ID, fmt.Sprintf("mkdir: %v", err))
		return
	}

	if err := os.Rename(params.OldPath, params.NewPath); err != nil {
		sendError(w, req.ID, fmt.Sprintf("rename: %v", err))
		return
	}

	result, _ := agentproto.MarshalResult(map[string]bool{"ok": true})
	agentproto.SendResponse(w, &agentproto.Response{ID: req.ID, Result: result})
}

func handleChmodFile(w io.Writer, req *agentproto.Request) {
	var params agentproto.ChmodFileParams
	if err := agentproto.UnmarshalParams(req.Params, &params); err != nil {
		sendError(w, req.ID, fmt.Sprintf("bad params: %v", err))
		return
	}

	mode, err := strconv.ParseUint(params.Mode, 8, 32)
	if err != nil {
		sendError(w, req.ID, fmt.Sprintf("parse mode: %v", err))
		return
	}

	if err := os.Chmod(params.Path, os.FileMode(mode)); err != nil {
		sendError(w, req.ID, fmt.Sprintf("chmod: %v", err))
		return
	}

	result, _ := agentproto.MarshalResult(map[string]bool{"ok": true})
	agentproto.SendResponse(w, &agentproto.Response{ID: req.ID, Result: result})
}

func handleStatFile(w io.Writer, req *agentproto.Request) {
	var params agentproto.StatFileParams
	if err := agentproto.UnmarshalParams(req.Params, &params); err != nil {
		sendError(w, req.ID, fmt.Sprintf("bad params: %v", err))
		return
	}

	info, err := os.Stat(params.Path)
	if err != nil {
		sendError(w, req.ID, fmt.Sprintf("stat: %v", err))
		return
	}

	result, _ := agentproto.MarshalResult(&agentproto.FileInfoResult{
		Path:    params.Path,
		Size:    info.Size(),
		Mode:    fmt.Sprintf("%o", info.Mode()),
		IsDir:   info.IsDir(),
		ModTime: info.ModTime().UTC().Format(time.RFC3339),
	})
	agentproto.SendResponse(w, &agentproto.Response{ID: req.ID, Result: result})
}

func handleGlobFiles(w io.Writer, req *agentproto.Request) {
	var params agentproto.GlobFilesParams
	if err := agentproto.UnmarshalParams(req.Params, &params); err != nil {
		sendError(w, req.ID, fmt.Sprintf("bad params: %v", err))
		return
	}

	matches, err := filepath.Glob(params.Pattern)
	if err != nil {
		sendError(w, req.ID, fmt.Sprintf("glob: %v", err))
		return
	}
	if matches == nil {
		matches = []string{}
	}

	result, _ := agentproto.MarshalResult(&agentproto.GlobFilesResult{Matches: matches})
	agentproto.SendResponse(w, &agentproto.Response{ID: req.ID, Result: result})
}

func handleShutdown(w io.Writer, req *agentproto.Request) {
	result, _ := agentproto.MarshalResult(map[string]bool{"ok": true})
	agentproto.SendResponse(w, &agentproto.Response{ID: req.ID, Result: result})

	// Flush the writer if it supports flushing.
	if f, ok := w.(interface{ Flush() error }); ok {
		f.Flush()
	}

	log.Println("shutdown requested, powering off")
	syscall.Sync()
	syscall.Reboot(syscall.LINUX_REBOOT_CMD_POWER_OFF)
}

func sendError(w io.Writer, id string, msg string) {
	agentproto.SendResponse(w, &agentproto.Response{ID: id, Error: msg})
}

// Ensure json is used (suppress unused import if linter complains).
var _ = json.Marshal
