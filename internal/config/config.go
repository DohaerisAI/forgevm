package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Providers ProvidersConfig `mapstructure:"providers"`
	Defaults  DefaultsConfig  `mapstructure:"defaults"`
	Auth      AuthConfig      `mapstructure:"auth"`
	Database  DatabaseConfig  `mapstructure:"database"`
	Logging   LoggingConfig   `mapstructure:"logging"`
	Pool      PoolConfig      `mapstructure:"pool"`
}

type PoolConfig struct {
	Enabled       bool   `mapstructure:"enabled"`
	MaxVMs        int    `mapstructure:"max_vms"`
	MaxUsersPerVM int    `mapstructure:"max_users_per_vm"`
	Image         string `mapstructure:"image"`
	MemoryMB      int    `mapstructure:"memory_mb"`
	VCPUs         int    `mapstructure:"vcpus"`
	Overflow      string `mapstructure:"overflow"` // "reject" or "queue"
}

type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

func (s ServerConfig) Addr() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

type ProvidersConfig struct {
	Default     string            `mapstructure:"default"`
	Mock        MockConfig        `mapstructure:"mock"`
	Firecracker FirecrackerConfig `mapstructure:"firecracker"`
	Docker      DockerConfig      `mapstructure:"docker"`
	E2B         E2BConfig         `mapstructure:"e2b"`
	Custom      CustomConfig      `mapstructure:"custom"`
	PRoot       PRootConfig       `mapstructure:"proot"`
}

type PRootConfig struct {
	Enabled        bool     `mapstructure:"enabled"`
	RootfsPath     string   `mapstructure:"rootfs_path"`
	PRootBinary    string   `mapstructure:"proot_binary"`
	WorkspaceBase  string   `mapstructure:"workspace_base"`
	DefaultTimeout string   `mapstructure:"default_timeout"`
	MaxSandboxes   int      `mapstructure:"max_sandboxes"`
	MaxMemoryMB    int      `mapstructure:"max_memory_mb"`
	MaxDiskMB      int      `mapstructure:"max_disk_mb"`
	Languages      []string `mapstructure:"languages"`
}

type DockerConfig struct {
	Enabled        bool              `mapstructure:"enabled"`
	Socket         string            `mapstructure:"socket"`
	Runtime        string            `mapstructure:"runtime"`
	DefaultImage   string            `mapstructure:"default_image"`
	NetworkMode    string            `mapstructure:"network_mode"`
	SeccompProfile string            `mapstructure:"seccomp_profile"`
	ReadOnlyRootfs bool              `mapstructure:"read_only_rootfs"`
	Memory         string            `mapstructure:"memory"`
	CPUs           string            `mapstructure:"cpus"`
	PidsLimit      int64             `mapstructure:"pids_limit"`
	User           string            `mapstructure:"user"`
	DroppedCaps    []string          `mapstructure:"dropped_caps"`
	AddedCaps      []string          `mapstructure:"added_caps"`
	Tmpfs          map[string]string `mapstructure:"tmpfs"`
	PoolSecurity   PoolSecurityConfig `mapstructure:"pool_security"`
}

type PoolSecurityConfig struct {
	PerUserUID           bool `mapstructure:"per_user_uid"`
	PIDNamespace         bool `mapstructure:"pid_namespace"`
	WorkspacePermissions bool `mapstructure:"workspace_permissions"`
	HidePID              bool `mapstructure:"hidepid"`
}

type E2BConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	APIKey  string `mapstructure:"api_key"`
	BaseURL string `mapstructure:"base_url"`
}

type CustomConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Name    string `mapstructure:"name"`
	BaseURL string `mapstructure:"base_url"`
	APIKey  string `mapstructure:"api_key"`
	Timeout string `mapstructure:"timeout"`
}

type MockConfig struct {
	Enabled bool `mapstructure:"enabled"`
}

type FirecrackerConfig struct {
	Enabled         bool   `mapstructure:"enabled"`
	FirecrackerPath string `mapstructure:"firecracker_path"`
	KernelPath      string `mapstructure:"kernel_path"`
	DefaultRootfs   string `mapstructure:"default_rootfs"`
	AgentPath       string `mapstructure:"agent_path"`
	DataDir         string `mapstructure:"data_dir"`
}

type DefaultsConfig struct {
	TTL          string `mapstructure:"ttl"`
	Image        string `mapstructure:"image"`
	MemoryMB     int    `mapstructure:"memory_mb"`
	VCPUs        int    `mapstructure:"vcpus"`
	DiskSizeMB   int    `mapstructure:"disk_size_mb"`
	PoolSize     int    `mapstructure:"pool_size"`
	PoolTemplate string `mapstructure:"pool_template"`
}

type AuthConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	APIKey  string `mapstructure:"api_key"`
}

type DatabaseConfig struct {
	Path string `mapstructure:"path"`
}

type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 7423)

	v.SetDefault("providers.default", "firecracker")
	v.SetDefault("providers.mock.enabled", false)
	v.SetDefault("providers.firecracker.enabled", true)
	v.SetDefault("providers.firecracker.firecracker_path", "/usr/local/bin/firecracker")
	v.SetDefault("providers.firecracker.kernel_path", "/var/lib/forgevm/vmlinux.bin")
	v.SetDefault("providers.firecracker.default_rootfs", "")
	v.SetDefault("providers.firecracker.agent_path", "./bin/forgevm-agent")
	v.SetDefault("providers.firecracker.data_dir", "/var/lib/forgevm")
	v.SetDefault("providers.e2b.enabled", false)
	v.SetDefault("providers.e2b.api_key", "")
	v.SetDefault("providers.e2b.base_url", "https://api.e2b.dev")
	v.SetDefault("providers.custom.enabled", false)
	v.SetDefault("providers.custom.name", "custom")
	v.SetDefault("providers.custom.base_url", "")
	v.SetDefault("providers.custom.api_key", "")
	v.SetDefault("providers.custom.timeout", "60s")

	v.SetDefault("providers.docker.enabled", false)
	v.SetDefault("providers.docker.socket", "unix:///var/run/docker.sock")
	v.SetDefault("providers.docker.runtime", "runc")
	v.SetDefault("providers.docker.default_image", "alpine:latest")
	v.SetDefault("providers.docker.network_mode", "none")
	v.SetDefault("providers.docker.seccomp_profile", "default")
	v.SetDefault("providers.docker.read_only_rootfs", true)
	v.SetDefault("providers.docker.memory", "512m")
	v.SetDefault("providers.docker.cpus", "1")
	v.SetDefault("providers.docker.pids_limit", 256)
	v.SetDefault("providers.docker.user", "")
	v.SetDefault("providers.docker.dropped_caps", []string{"ALL"})
	v.SetDefault("providers.docker.added_caps", []string{})
	v.SetDefault("providers.docker.pool_security.per_user_uid", false)
	v.SetDefault("providers.docker.pool_security.pid_namespace", false)
	v.SetDefault("providers.docker.pool_security.workspace_permissions", true)
	v.SetDefault("providers.docker.pool_security.hidepid", false)

	v.SetDefault("providers.proot.enabled", false)
	v.SetDefault("providers.proot.proot_binary", "proot")
	v.SetDefault("providers.proot.rootfs_path", "/var/lib/forgevm/rootfs")
	v.SetDefault("providers.proot.workspace_base", "/var/lib/forgevm/workspaces")
	v.SetDefault("providers.proot.default_timeout", "60s")
	v.SetDefault("providers.proot.max_sandboxes", 10)
	v.SetDefault("providers.proot.max_memory_mb", 512)
	v.SetDefault("providers.proot.max_disk_mb", 1024)
	v.SetDefault("providers.proot.languages", []string{"python3", "node", "bash"})

	v.SetDefault("defaults.ttl", "30m")
	v.SetDefault("defaults.image", "alpine:latest")
	v.SetDefault("defaults.memory_mb", 1024)
	v.SetDefault("defaults.vcpus", 1)
	v.SetDefault("defaults.disk_size_mb", 1024)
	v.SetDefault("defaults.pool_size", 0)
	v.SetDefault("defaults.pool_template", "")

	v.SetDefault("auth.enabled", false)
	v.SetDefault("auth.api_key", "")

	v.SetDefault("database.path", "forgevm.db")

	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")

	v.SetDefault("pool.enabled", false)
	v.SetDefault("pool.max_vms", 10)
	v.SetDefault("pool.max_users_per_vm", 5)
	v.SetDefault("pool.image", "alpine:latest")
	v.SetDefault("pool.memory_mb", 2048)
	v.SetDefault("pool.vcpus", 2)
	v.SetDefault("pool.overflow", "reject")
}

// ResolveAgentPath resolves the agent_path to an absolute path.
// If the path is relative, it tries (in order):
//  1. Relative to the executable's directory
//  2. Relative to CWD
//  3. PATH lookup via exec.LookPath (for /usr/local/bin installs)
func (c *Config) ResolveAgentPath() string {
	p := c.Providers.Firecracker.AgentPath
	if filepath.IsAbs(p) {
		return p
	}
	// Try relative to executable first
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), p)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	// Try CWD-relative
	if abs, err := filepath.Abs(p); err == nil {
		if _, err := os.Stat(abs); err == nil {
			return abs
		}
	}
	// Fallback: look up the binary name in PATH
	base := filepath.Base(p)
	if found, err := exec.LookPath(base); err == nil {
		return found
	}
	// Last resort: return as-is
	return p
}

func Load() (*Config, error) {
	v := viper.New()
	setDefaults(v)

	v.SetConfigType("yaml")

	v.SetEnvPrefix("FORGEVM")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Try explicit config files in order of priority
	configPaths := []string{"forgevm.yaml"}
	if home, err := os.UserHomeDir(); err == nil {
		configPaths = append(configPaths, filepath.Join(home, ".forgevm", "config.yaml"))
	}

	loaded := false
	for _, p := range configPaths {
		if _, err := os.Stat(p); err == nil {
			v.SetConfigFile(p)
			if err := v.ReadInConfig(); err != nil {
				return nil, fmt.Errorf("reading config %s: %w", p, err)
			}
			loaded = true
			break
		}
	}
	_ = loaded // defaults are fine if no config file found

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	return &cfg, nil
}
