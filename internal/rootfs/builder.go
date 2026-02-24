// Package rootfs builds ext4 rootfs images from Docker/OCI images
// with the forgevm-agent baked in.
package rootfs

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Builder creates rootfs images from Docker images.
type Builder struct {
	CacheDir  string // Directory to cache built rootfs images.
	AgentPath string // Path to the compiled forgevm-agent binary.
}

// GetOrBuild returns the path to a rootfs image for the given Docker image,
// building it if not cached. Requires: docker, mkfs.ext4, mount (needs sudo).
func (b *Builder) GetOrBuild(ctx context.Context, image string, diskSizeMB int) (string, error) {
	if diskSizeMB <= 0 {
		diskSizeMB = 1024
	}

	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(image)))[:16]
	cachePath := filepath.Join(b.CacheDir, hash+".ext4")

	// Return cached if exists.
	if _, err := os.Stat(cachePath); err == nil {
		return cachePath, nil
	}

	if err := os.MkdirAll(b.CacheDir, 0755); err != nil {
		return "", fmt.Errorf("create cache dir: %w", err)
	}

	// Verify agent binary exists.
	if _, err := os.Stat(b.AgentPath); err != nil {
		return "", fmt.Errorf("agent binary not found at %s: %w", b.AgentPath, err)
	}

	// Create temp working directory.
	tmpDir, err := os.MkdirTemp("", "forgevm-rootfs-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	tarPath := filepath.Join(tmpDir, "rootfs.tar")
	imgPath := filepath.Join(tmpDir, "rootfs.ext4")
	mntPath := filepath.Join(tmpDir, "mnt")

	// Export Docker image to tar.
	containerID, err := runCmd(ctx, "docker", "create", image)
	if err != nil {
		return "", fmt.Errorf("docker create: %w", err)
	}
	containerID = strings.TrimSpace(containerID)
	defer runCmd(ctx, "docker", "rm", containerID)

	if _, err := runCmd(ctx, "docker", "export", "-o", tarPath, containerID); err != nil {
		return "", fmt.Errorf("docker export: %w", err)
	}

	// Create blank ext4 image.
	if _, err := runCmd(ctx, "dd", "if=/dev/zero", "of="+imgPath,
		"bs=1M", fmt.Sprintf("count=%d", diskSizeMB)); err != nil {
		return "", fmt.Errorf("dd: %w", err)
	}
	if _, err := runCmd(ctx, "mkfs.ext4", "-F", imgPath); err != nil {
		return "", fmt.Errorf("mkfs.ext4: %w", err)
	}

	// Mount and populate.
	if err := os.MkdirAll(mntPath, 0755); err != nil {
		return "", fmt.Errorf("create mount dir: %w", err)
	}
	if _, err := runCmd(ctx, "sudo", "mount", "-o", "loop", imgPath, mntPath); err != nil {
		return "", fmt.Errorf("mount: %w", err)
	}
	defer runCmd(ctx, "sudo", "umount", mntPath)

	// Extract rootfs.
	if _, err := runCmd(ctx, "sudo", "tar", "xf", tarPath, "-C", mntPath); err != nil {
		return "", fmt.Errorf("extract tar: %w", err)
	}

	// Copy agent binary.
	agentDst := filepath.Join(mntPath, "usr", "local", "bin", "forgevm-agent")
	if _, err := runCmd(ctx, "sudo", "mkdir", "-p", filepath.Dir(agentDst)); err != nil {
		return "", fmt.Errorf("mkdir agent dir: %w", err)
	}
	if _, err := runCmd(ctx, "sudo", "cp", b.AgentPath, agentDst); err != nil {
		return "", fmt.Errorf("copy agent: %w", err)
	}
	if _, err := runCmd(ctx, "sudo", "chmod", "755", agentDst); err != nil {
		return "", fmt.Errorf("chmod agent: %w", err)
	}

	// Write init script that starts the agent.
	initScript := `#!/bin/sh
mount -t proc proc /proc
mount -t sysfs sysfs /sys
mount -t devtmpfs devtmpfs /dev
exec /usr/local/bin/forgevm-agent
`
	initPath := filepath.Join(tmpDir, "init")
	if err := os.WriteFile(initPath, []byte(initScript), 0755); err != nil {
		return "", fmt.Errorf("write init script: %w", err)
	}
	sbinInit := filepath.Join(mntPath, "sbin", "init")
	if _, err := runCmd(ctx, "sudo", "mkdir", "-p", filepath.Join(mntPath, "sbin")); err != nil {
		return "", fmt.Errorf("mkdir sbin: %w", err)
	}
	if _, err := runCmd(ctx, "sudo", "cp", initPath, sbinInit); err != nil {
		return "", fmt.Errorf("copy init: %w", err)
	}
	if _, err := runCmd(ctx, "sudo", "chmod", "755", sbinInit); err != nil {
		return "", fmt.Errorf("chmod init: %w", err)
	}

	// Unmount before moving to cache.
	if _, err := runCmd(ctx, "sudo", "umount", mntPath); err != nil {
		return "", fmt.Errorf("unmount: %w", err)
	}

	// Move to cache.
	if err := os.Rename(imgPath, cachePath); err != nil {
		// rename may fail across filesystems, fall back to copy.
		if _, err := runCmd(ctx, "cp", imgPath, cachePath); err != nil {
			return "", fmt.Errorf("move to cache: %w", err)
		}
	}

	return cachePath, nil
}

func runCmd(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s %v: %w: %s", name, args, err, string(out))
	}
	return string(out), nil
}
