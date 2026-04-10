package config

import "testing"

func TestEnvironment_String(t *testing.T) {
	tests := []struct {
		env  Environment
		want string
	}{
		{EnvUnknown, "unknown"},
		{EnvAndroid, "android"},
		{EnvDocker, "docker"},
		{EnvFirecracker, "firecracker"},
		{EnvPRoot, "proot"},
	}
	for _, tt := range tests {
		if got := tt.env.String(); got != tt.want {
			t.Errorf("Environment(%d).String() = %q, want %q", tt.env, got, tt.want)
		}
	}
}

func TestDefaultProviderForEnv(t *testing.T) {
	tests := []struct {
		env  Environment
		want string
	}{
		{EnvAndroid, "proot"},
		{EnvFirecracker, "firecracker"},
		{EnvDocker, "docker"},
		{EnvPRoot, "proot"},
		{EnvUnknown, "mock"},
	}
	for _, tt := range tests {
		if got := DefaultProviderForEnv(tt.env); got != tt.want {
			t.Errorf("DefaultProviderForEnv(%s) = %q, want %q", tt.env, got, tt.want)
		}
	}
}

func TestDetectEnvironment_DoesNotPanic(t *testing.T) {
	// Just verify it runs without panicking on the current system
	env := DetectEnvironment()
	_ = env.String() // ensure String() works on the detected value
}
