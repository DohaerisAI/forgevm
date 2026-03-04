package environments

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/DohaerisAI/forgevm/internal/store"
	"github.com/rs/zerolog"
)

const (
	BuildStatusQueued   = "queued"
	BuildStatusBuilding = "building"
	BuildStatusReady    = "ready"
	BuildStatusFailed   = "failed"
	BuildStatusCanceled = "canceled"
)

type Manager struct {
	store store.Store
	log   zerolog.Logger
	run   CommandRunner

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	jobs chan string
}

type CommandRunner interface {
	Run(ctx context.Context, stdin string, name string, args ...string) (string, error)
}

type OSRunner struct{}

func (OSRunner) Run(ctx context.Context, stdin string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

var safePkgPattern = regexp.MustCompile(`^[a-zA-Z0-9._+\-<>=!~\[\],:@/]+$`)

func NewManager(st store.Store, logger zerolog.Logger) *Manager {
	return NewManagerWithRunner(st, logger, OSRunner{})
}

func NewManagerWithRunner(st store.Store, logger zerolog.Logger, runner CommandRunner) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		store:  st,
		log:    logger.With().Str("component", "env-build-manager").Logger(),
		run:    runner,
		ctx:    ctx,
		cancel: cancel,
		jobs:   make(chan string, 128),
	}
}

func (m *Manager) Start(workers int) {
	if workers <= 0 {
		workers = 1
	}
	for i := 0; i < workers; i++ {
		m.wg.Add(1)
		go m.worker(i + 1)
	}
}

func (m *Manager) Stop() {
	m.cancel()
	m.wg.Wait()
}

func (m *Manager) Enqueue(buildID string) error {
	select {
	case <-m.ctx.Done():
		return fmt.Errorf("environment build manager is stopped")
	case m.jobs <- buildID:
		return nil
	default:
		return fmt.Errorf("environment build queue is full")
	}
}

func (m *Manager) worker(workerID int) {
	defer m.wg.Done()
	logger := m.log.With().Int("worker", workerID).Logger()
	for {
		select {
		case <-m.ctx.Done():
			return
		case buildID := <-m.jobs:
			if err := m.processBuild(buildID); err != nil {
				logger.Error().Err(err).Str("build_id", buildID).Msg("process build failed")
			}
		}
	}
}

func (m *Manager) processBuild(buildID string) error {
	ctx := context.Background()

	build, err := m.store.GetEnvironmentBuild(ctx, buildID)
	if err != nil {
		return err
	}
	if build.Status == BuildStatusCanceled || build.Status == BuildStatusReady || build.Status == BuildStatusFailed {
		return nil
	}

	if err := m.updateBuild(ctx, build, BuildStatusBuilding, "validate_spec", "build started", ""); err != nil {
		return err
	}

	spec, err := m.store.GetEnvironmentSpec(ctx, build.SpecID)
	if err != nil {
		return m.failBuild(ctx, build, "validate_spec", fmt.Sprintf("load spec: %v", err))
	}
	if strings.TrimSpace(spec.BaseImage) == "" {
		return m.failBuild(ctx, build, "validate_spec", "base_image is empty")
	}
	pythonPkgs := decodePkgList(spec.PythonPackages)
	aptPkgs := decodePkgList(spec.AptPackages)
	if err := validatePackages(pythonPkgs); err != nil {
		return m.failBuild(ctx, build, "validate_spec", "invalid python package list: "+err.Error())
	}
	if err := validatePackages(aptPkgs); err != nil {
		return m.failBuild(ctx, build, "validate_spec", "invalid apt package list: "+err.Error())
	}
	if err := m.updateBuild(ctx, build, BuildStatusBuilding, "validate_spec", fmt.Sprintf("validated packages: pip=%d apt=%d", len(pythonPkgs), len(aptPkgs)), ""); err != nil {
		return err
	}

	if err := m.updateBuild(ctx, build, BuildStatusBuilding, "generate_dockerfile", "dockerfile generated", ""); err != nil {
		return err
	}

	artifacts, err := m.store.ListEnvironmentArtifacts(ctx, buildID)
	if err != nil {
		return m.failBuild(ctx, build, "tag_and_push_targets", fmt.Sprintf("list artifacts: %v", err))
	}
	if len(artifacts) == 0 {
		return m.failBuild(ctx, build, "tag_and_push_targets", "no artifacts requested")
	}
	localRef := localBuildRef(buildID)
	for _, a := range artifacts {
		if a.Target == "local" {
			localRef = a.ImageRef
			break
		}
	}

	if err := m.updateBuild(ctx, build, BuildStatusBuilding, "docker_build", "docker build started", ""); err != nil {
		return err
	}
	if err := m.buildLocalImage(ctx, spec.BaseImage, aptPkgs, pythonPkgs, localRef); err != nil {
		return m.failBuild(ctx, build, "docker_build", err.Error())
	}
	if err := m.updateBuild(ctx, build, BuildStatusBuilding, "docker_build", "docker build completed", ""); err != nil {
		return err
	}
	if id, err := m.inspectImageID(ctx, localRef); err == nil {
		build.DigestLocal = id
	}

	allReady := true

	for _, a := range artifacts {
		switch a.Target {
		case "local":
			a.Status = "ready"
			a.Error = ""
			id, err := m.inspectImageID(ctx, localRef)
			if err != nil {
				allReady = false
				a.Status = "failed"
				a.Error = "inspect local image: " + err.Error()
			} else {
				a.Digest = id
				build.DigestLocal = id
			}
		case "ghcr", "dockerhub":
			finalRef, err := m.publishToTarget(ctx, spec.OwnerID, a.Target, localRef, a.ImageRef)
			if finalRef != "" {
				a.ImageRef = finalRef
			}
			if err != nil {
				allReady = false
				a.Status = "failed"
				a.Error = err.Error()
			} else {
				a.Status = "ready"
				a.Error = ""
				if id, err := m.inspectImageID(ctx, a.ImageRef); err == nil {
					a.Digest = id
				}
			}
		default:
			allReady = false
			a.Status = "failed"
			a.Error = "unsupported target"
		}
		if err := m.store.SaveEnvironmentArtifact(ctx, a); err != nil {
			return m.failBuild(ctx, build, "tag_and_push_targets", fmt.Sprintf("save artifact %s: %v", a.Target, err))
		}
	}

	build.ImageSizeBytes = 0

	now := time.Now().UTC()
	build.FinishedAt = &now
	if allReady {
		return m.updateBuild(ctx, build, BuildStatusReady, "finalize", "build completed", "")
	}
	return m.updateBuild(ctx, build, BuildStatusFailed, "finalize", "build completed with target failures", "one or more publish targets failed")
}

func (m *Manager) failBuild(ctx context.Context, build *store.EnvironmentBuildRecord, step, errMsg string) error {
	now := time.Now().UTC()
	build.FinishedAt = &now
	return m.updateBuild(ctx, build, BuildStatusFailed, step, errMsg, errMsg)
}

func (m *Manager) updateBuild(ctx context.Context, build *store.EnvironmentBuildRecord, status, step, logLine, errLine string) error {
	build.Status = status
	build.CurrentStep = step
	if logLine != "" {
		if build.LogBlob == "" {
			build.LogBlob = logLine
		} else {
			build.LogBlob = build.LogBlob + "\n" + logLine
		}
	}
	if errLine != "" {
		build.Error = errLine
	}
	build.UpdatedAt = time.Now().UTC()
	return m.store.UpdateEnvironmentBuild(ctx, build)
}

func decodePkgList(raw string) []string {
	var out []string
	_ = json.Unmarshal([]byte(raw), &out)
	if out == nil {
		return []string{}
	}
	return out
}

func validatePackages(pkgs []string) error {
	for _, p := range pkgs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !safePkgPattern.MatchString(p) {
			return fmt.Errorf("package %q has unsupported characters", p)
		}
	}
	return nil
}

func localBuildRef(buildID string) string {
	id := buildID
	if len(id) > 8 {
		id = id[:8]
	}
	return "local/forgevm-env:" + id
}

func (m *Manager) buildLocalImage(ctx context.Context, baseImage string, aptPkgs, pyPkgs []string, tag string) error {
	tmpDir, err := os.MkdirTemp("", "forgevm-env-build-*")
	if err != nil {
		return fmt.Errorf("create build temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	df := generateDockerfile(baseImage, aptPkgs, pyPkgs)
	if err := os.WriteFile(filepath.Join(tmpDir, "Dockerfile"), []byte(df), 0644); err != nil {
		return fmt.Errorf("write Dockerfile: %w", err)
	}
	if _, err := m.run.Run(ctx, "", "docker", "build", "-t", tag, tmpDir); err != nil {
		return fmt.Errorf("docker build failed: %w", err)
	}
	return nil
}

func (m *Manager) publishToTarget(ctx context.Context, ownerID, target, sourceTag, requestedRef string) (string, error) {
	conn, err := m.resolveRegistryConnection(ctx, ownerID, target)
	if err != nil {
		return "", err
	}
	registryHost := registryHostForTarget(target)
	if registryHost == "" {
		return "", fmt.Errorf("unsupported publish target %q", target)
	}
	targetRef := buildRegistryImageRef(target, conn.Username, requestedRef)
	if targetRef == "" {
		return "", fmt.Errorf("could not compute target image ref for %q", target)
	}
	loginArgs := []string{"login", "-u", conn.Username, "--password-stdin"}
	if target == "ghcr" {
		loginArgs = append(loginArgs, registryHost)
	}
	if _, err := m.run.Run(ctx, conn.SecretRef, "docker", loginArgs...); err != nil {
		return targetRef, fmt.Errorf("docker login %s failed: %w", target, err)
	}
	if _, err := m.run.Run(ctx, "", "docker", "tag", sourceTag, targetRef); err != nil {
		return targetRef, fmt.Errorf("docker tag failed: %w", err)
	}
	if _, err := m.run.Run(ctx, "", "docker", "push", targetRef); err != nil {
		return targetRef, fmt.Errorf("docker push failed: %w", err)
	}
	return targetRef, nil
}

func (m *Manager) resolveRegistryConnection(ctx context.Context, ownerID, target string) (*store.RegistryConnectionRecord, error) {
	conns, err := m.store.ListRegistryConnections(ctx, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list registry connections: %w", err)
	}
	var fallback *store.RegistryConnectionRecord
	for _, c := range conns {
		if c.Provider != target {
			continue
		}
		if c.IsDefault {
			return c, nil
		}
		if fallback == nil {
			fallback = c
		}
	}
	if fallback != nil {
		return fallback, nil
	}
	return nil, fmt.Errorf("no registry connection configured for owner=%q provider=%q", ownerID, target)
}

func registryHostForTarget(target string) string {
	switch target {
	case "ghcr":
		return "ghcr.io"
	case "dockerhub":
		return "docker.io"
	default:
		return ""
	}
}

func buildRegistryImageRef(target, username, requestedRef string) string {
	host := registryHostForTarget(target)
	if host == "" {
		return ""
	}
	ns := strings.ToLower(strings.TrimSpace(username))
	if ns == "" {
		return ""
	}
	tag := extractTag(requestedRef)
	return fmt.Sprintf("%s/%s/forgevm-env:%s", host, ns, tag)
}

func extractTag(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "latest"
	}
	lastSlash := strings.LastIndex(ref, "/")
	lastColon := strings.LastIndex(ref, ":")
	if lastColon > lastSlash && lastColon+1 < len(ref) {
		return strings.TrimSpace(ref[lastColon+1:])
	}
	return "latest"
}

func (m *Manager) inspectImageID(ctx context.Context, ref string) (string, error) {
	out, err := m.run.Run(ctx, "", "docker", "image", "inspect", "--format", "{{.Id}}", ref)
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(out)
	if id == "" {
		return "", fmt.Errorf("empty image id for %s", ref)
	}
	return id, nil
}

func generateDockerfile(baseImage string, aptPkgs, pyPkgs []string) string {
	var b strings.Builder
	b.WriteString("FROM " + baseImage + "\n")
	if len(aptPkgs) > 0 {
		b.WriteString("RUN apt-get update && apt-get install -y --no-install-recommends " + strings.Join(aptPkgs, " ") + " && rm -rf /var/lib/apt/lists/*\n")
	}
	if len(pyPkgs) > 0 {
		b.WriteString("RUN pip install --no-cache-dir " + strings.Join(pyPkgs, " ") + "\n")
	}
	b.WriteString("RUN mkdir -p /workspace /output /data\n")
	b.WriteString("WORKDIR /workspace\n")
	return b.String()
}
