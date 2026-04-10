package config

import (
	"os"
	"os/exec"
)

// Environment represents the detected runtime environment.
type Environment int

const (
	EnvUnknown     Environment = iota
	EnvAndroid                         // Android device (has /system/build.prop)
	EnvDocker                          // Docker daemon available
	EnvFirecracker                     // Firecracker binary available
	EnvPRoot                           // PRoot binary available
)

// String returns the environment name.
func (e Environment) String() string {
	switch e {
	case EnvAndroid:
		return "android"
	case EnvDocker:
		return "docker"
	case EnvFirecracker:
		return "firecracker"
	case EnvPRoot:
		return "proot"
	default:
		return "unknown"
	}
}

// DetectEnvironment probes the runtime to determine the best provider.
// Detection order: Android > Firecracker > Docker > PRoot > Unknown.
func DetectEnvironment() Environment {
	// Android: presence of /system/build.prop
	if _, err := os.Stat("/system/build.prop"); err == nil {
		return EnvAndroid
	}

	// Firecracker: binary in PATH
	if _, err := exec.LookPath("firecracker"); err == nil {
		return EnvFirecracker
	}

	// Docker: binary in PATH
	if _, err := exec.LookPath("docker"); err == nil {
		return EnvDocker
	}

	// PRoot: binary in PATH
	if _, err := exec.LookPath("proot"); err == nil {
		return EnvPRoot
	}

	return EnvUnknown
}

// DefaultProviderForEnv returns the recommended default provider for an environment.
func DefaultProviderForEnv(env Environment) string {
	switch env {
	case EnvAndroid:
		return "proot"
	case EnvFirecracker:
		return "firecracker"
	case EnvDocker:
		return "docker"
	case EnvPRoot:
		return "proot"
	default:
		return "mock"
	}
}
