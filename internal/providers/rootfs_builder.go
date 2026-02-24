package providers

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// ImageCache manages cached rootfs images built from Docker images.
// Each unique Docker image is built once and stored as an ext4 file.
// The build process uses Docker for all privileged operations (mount, ext4 populate)
// so no root/sudo access is needed on the host — only Docker.
type ImageCache struct {
	mu        sync.Mutex
	cacheDir  string
	agentPath string
	diskMB    int
	logger    zerolog.Logger

	// building tracks in-progress builds so concurrent Spawns for the same
	// image wait for a single build rather than starting duplicates.
	building map[string]*buildResult
}

type buildResult struct {
	done chan struct{}
	path string
	err  error
}

// NewImageCache creates a new image cache.
func NewImageCache(cacheDir, agentPath string, diskMB int, logger zerolog.Logger) *ImageCache {
	if diskMB <= 0 {
		diskMB = 1024
	}
	return &ImageCache{
		cacheDir:  cacheDir,
		agentPath: agentPath,
		diskMB:    diskMB,
		logger:    logger.With().Str("component", "image-cache").Logger(),
		building:  make(map[string]*buildResult),
	}
}

// Get returns the path to a cached rootfs for the given Docker image.
// If the rootfs doesn't exist yet, it builds it from the Docker image.
// Returns ("", nil) if the image name is empty.
func (ic *ImageCache) Get(image string) (string, error) {
	if image == "" {
		return "", nil
	}

	cachedPath := ic.pathFor(image)

	// Fast path: already cached.
	if _, err := os.Stat(cachedPath); err == nil {
		ic.logger.Debug().Str("image", image).Str("path", cachedPath).Msg("rootfs cache hit")
		return cachedPath, nil
	}

	// Slow path: need to build.
	ic.mu.Lock()
	if br, ok := ic.building[image]; ok {
		ic.mu.Unlock()
		ic.logger.Info().Str("image", image).Msg("waiting for in-progress build")
		<-br.done
		return br.path, br.err
	}

	if _, err := os.Stat(cachedPath); err == nil {
		ic.mu.Unlock()
		return cachedPath, nil
	}

	br := &buildResult{done: make(chan struct{})}
	ic.building[image] = br
	ic.mu.Unlock()

	ic.logger.Info().Str("image", image).Msg("rootfs not cached, starting build")
	start := time.Now()

	br.path, br.err = ic.build(image, cachedPath)
	close(br.done)

	ic.mu.Lock()
	delete(ic.building, image)
	ic.mu.Unlock()

	if br.err != nil {
		ic.logger.Error().Err(br.err).Str("image", image).Dur("elapsed", time.Since(start)).Msg("rootfs build failed")
	} else {
		ic.logger.Info().Str("image", image).Str("path", br.path).Dur("elapsed", time.Since(start)).Msg("rootfs build complete")
	}

	return br.path, br.err
}

// pathFor returns the cache path for a given Docker image name.
func (ic *ImageCache) pathFor(image string) string {
	h := sha256.Sum256([]byte(image))
	safe := sanitizeImageName(image)
	name := fmt.Sprintf("%s-%x.ext4", safe, h[:4])
	return filepath.Join(ic.cacheDir, name)
}

// build creates a rootfs ext4 image from a Docker image.
func (ic *ImageCache) build(image, outputPath string) (string, error) {
	if err := os.MkdirAll(ic.cacheDir, 0755); err != nil {
		return "", fmt.Errorf("create cache dir: %w", err)
	}

	agentAbs, err := filepath.Abs(ic.agentPath)
	if err != nil {
		return "", fmt.Errorf("resolve agent path: %w", err)
	}
	if _, err := os.Stat(agentAbs); err != nil {
		return "", fmt.Errorf("agent binary not found at %s: %w", agentAbs, err)
	}

	if _, err := exec.LookPath("docker"); err != nil {
		return "", fmt.Errorf("docker not found in PATH (required for building images): %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "forgevm-rootfs-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	tarPath := filepath.Join(tmpDir, "rootfs.tar")

	// Step 1: Pull image (skip if already available locally)
	if _, err := runCmdCombined("docker", "image", "inspect", image); err != nil {
		ic.logger.Info().Str("image", image).Msg("pulling docker image")
		if out, err := runCmdCombined("docker", "pull", image); err != nil {
			return "", fmt.Errorf("docker pull %s: %w\n%s", image, err, out)
		}
	} else {
		ic.logger.Info().Str("image", image).Msg("image found locally, skipping pull")
	}

	// Step 2: Export filesystem
	ic.logger.Info().Str("image", image).Msg("exporting docker filesystem")
	containerID, err := runCmdOutput("docker", "create", image)
	if err != nil {
		return "", fmt.Errorf("docker create %s: %w", image, err)
	}
	containerID = strings.TrimSpace(containerID)
	defer runCmdCombined("docker", "rm", containerID)

	if out, err := runCmdCombined("docker", "export", containerID, "-o", tarPath); err != nil {
		return "", fmt.Errorf("docker export: %w\n%s", err, out)
	}

	// Step 3: Build ext4 in privileged container
	ic.logger.Info().Str("image", image).Int("disk_mb", ic.diskMB).Msg("building ext4 rootfs in container")

	buildScript := fmt.Sprintf(`set -e
apk add --no-cache e2fsprogs e2fsprogs-extra >/dev/null 2>&1
dd if=/dev/zero of=/work/rootfs.ext4 bs=1M count=%d status=none
mkfs.ext4 -F /work/rootfs.ext4 >/dev/null 2>&1
mkdir -p /mnt/rootfs
mount -o loop /work/rootfs.ext4 /mnt/rootfs
tar xf /work/rootfs.tar -C /mnt/rootfs 2>/dev/null || true
mkdir -p /mnt/rootfs/usr/local/bin
cp /work/agent /mnt/rootfs/usr/local/bin/forgevm-agent
chmod 755 /mnt/rootfs/usr/local/bin/forgevm-agent
mkdir -p /mnt/rootfs/sbin
rm -f /mnt/rootfs/sbin/init
cat > /mnt/rootfs/sbin/init <<'INITEOF'
#!/bin/sh
mount -t proc proc /proc
mount -t sysfs sysfs /sys
mount -t devtmpfs devtmpfs /dev
exec /usr/local/bin/forgevm-agent
INITEOF
chmod 755 /mnt/rootfs/sbin/init
umount /mnt/rootfs
e2fsck -fy /work/rootfs.ext4 >/dev/null 2>&1 || true
resize2fs -M /work/rootfs.ext4 >/dev/null 2>&1
echo DONE`, ic.diskMB)

	if out, err := runCmdCombined("docker", "run", "--privileged", "--rm",
		"-v", tmpDir+":/work",
		"-v", agentAbs+":/work/agent:ro",
		"alpine:latest", "sh", "-c", buildScript,
	); err != nil {
		return "", fmt.Errorf("build rootfs in container: %w\n%s", err, out)
	}

	// Step 4: Move to cache
	ext4Path := filepath.Join(tmpDir, "rootfs.ext4")
	if _, err := os.Stat(ext4Path); err != nil {
		return "", fmt.Errorf("rootfs.ext4 not created: %w", err)
	}

	if err := os.Rename(ext4Path, outputPath); err != nil {
		if err := copyFile(ext4Path, outputPath); err != nil {
			return "", fmt.Errorf("move to cache: %w", err)
		}
	}

	return outputPath, nil
}

// sanitizeImageName converts a Docker image name to a filesystem-safe string.
func sanitizeImageName(image string) string {
	r := strings.NewReplacer(
		"/", "_",
		":", "-",
		"@", "_at_",
	)
	s := r.Replace(image)
	if len(s) > 60 {
		s = s[:60]
	}
	return s
}

// runCmdCombined runs a command and returns combined stdout+stderr.
func runCmdCombined(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

// runCmdOutput runs a command and returns its stdout.
func runCmdOutput(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, stderr.String())
	}
	return string(out), nil
}
