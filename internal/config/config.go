package config

import (
	"fmt"
	"os"
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
	E2B         E2BConfig         `mapstructure:"e2b"`
	Custom      CustomConfig      `mapstructure:"custom"`
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

	v.SetDefault("defaults.ttl", "30m")
	v.SetDefault("defaults.image", "alpine:latest")
	v.SetDefault("defaults.memory_mb", 512)
	v.SetDefault("defaults.vcpus", 1)
	v.SetDefault("defaults.disk_size_mb", 1024)
	v.SetDefault("defaults.pool_size", 0)
	v.SetDefault("defaults.pool_template", "")

	v.SetDefault("auth.enabled", false)
	v.SetDefault("auth.api_key", "")

	v.SetDefault("database.path", "forgevm.db")

	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")
}

func Load() (*Config, error) {
	v := viper.New()
	setDefaults(v)

	v.SetConfigType("yaml")

	v.SetEnvPrefix("HATCHIT")
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
