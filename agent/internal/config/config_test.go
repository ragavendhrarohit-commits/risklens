package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testdata(name string) string {
	return filepath.Join("..", "..", "testdata", "config", name)
}

func TestLoadExampleConfig(t *testing.T) {
	path := filepath.Join("..", "..", "configs", "agent.example.yaml")
	result, err := Load(path, Overrides{})
	if err != nil {
		t.Fatalf("example config must be valid: %v", err)
	}
	if result.Config.Service.Name != "RiskLensAgent" {
		t.Fatalf("service name = %q", result.Config.Service.Name)
	}
	if result.Config.Agent.ShutdownTimeout != 15*time.Second {
		t.Fatalf("shutdown timeout = %s", result.Config.Agent.ShutdownTimeout)
	}
	if result.Config.Path != filepath.Clean(path) {
		t.Fatalf("path = %q", result.Config.Path)
	}
}

func TestLoadValid(t *testing.T) {
	result, err := Load(testdata("valid.yaml"), Overrides{})
	if err != nil {
		t.Fatalf("Load valid.yaml: %v", err)
	}
	if result.Config.Agent.LogLevel != "info" {
		t.Fatalf("log_level = %q", result.Config.Agent.LogLevel)
	}
	if result.Config.Service.Name != "RiskLensAgent" {
		t.Fatalf("service name = %q", result.Config.Service.Name)
	}
}

func TestLoadMissing(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "missing.yaml"), Overrides{})
	if !errors.Is(err, ErrMissing) {
		t.Fatalf("expected ErrMissing, got %v", err)
	}
}

func TestLoadMalformed(t *testing.T) {
	_, err := Load(testdata("malformed.yaml"), Overrides{})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ErrInvalid, got %v", err)
	}
}

func TestLoadInvalidLevel(t *testing.T) {
	_, err := Load(testdata("invalid_level.yaml"), Overrides{})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ErrInvalid, got %v", err)
	}
}

func TestLoadInvalidTimeout(t *testing.T) {
	_, err := Load(testdata("invalid_timeout.yaml"), Overrides{})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ErrInvalid, got %v", err)
	}
}

func TestLoadEmptyServiceName(t *testing.T) {
	_, err := Load(testdata("empty_service.yaml"), Overrides{})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ErrInvalid, got %v", err)
	}
}

func TestUnknownKeysWarn(t *testing.T) {
	result, err := Load(testdata("unknown_keys.yaml"), Overrides{})
	if err != nil {
		t.Fatalf("unknown keys must not fail load: %v", err)
	}
	if len(result.Warnings) < 2 {
		t.Fatalf("expected unknown key warnings, got %v", result.Warnings)
	}
	if result.Warnings[0] > result.Warnings[1] {
		t.Fatalf("warnings are not sorted: %v", result.Warnings)
	}
}

func TestEnvOverrides(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "override.log")
	t.Setenv(EnvLogLevel, "debug")
	t.Setenv(EnvLogFile, logFile)
	t.Setenv(EnvShutdownTimeout, "20s")

	result, err := Load(testdata("valid.yaml"), Overrides{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if result.Config.Agent.LogLevel != "debug" {
		t.Fatalf("log_level = %q", result.Config.Agent.LogLevel)
	}
	if result.Config.Agent.LogFile != filepath.Clean(logFile) {
		t.Fatalf("log_file = %q", result.Config.Agent.LogFile)
	}
	if result.Config.Agent.ShutdownTimeout != 20*time.Second {
		t.Fatalf("shutdown timeout = %s", result.Config.Agent.ShutdownTimeout)
	}
}

func TestFlagOverridesBeatEnv(t *testing.T) {
	t.Setenv(EnvLogLevel, "debug")
	result, err := Load(testdata("valid.yaml"), Overrides{LogLevel: "warn"})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if result.Config.Agent.LogLevel != "warn" {
		t.Fatalf("log_level = %q", result.Config.Agent.LogLevel)
	}
}

func TestValidateTimeoutBounds(t *testing.T) {
	cfg := Defaults()
	cfg.Agent.LogFile = filepath.Join(t.TempDir(), "agent.log")
	cfg.Agent.ShutdownTimeoutRaw = "0s"
	if err := Validate(&cfg); err == nil {
		t.Fatal("expected error for 0s timeout")
	}
	cfg.Agent.ShutdownTimeoutRaw = "3m"
	if err := Validate(&cfg); err == nil {
		t.Fatal("expected error for 3m timeout")
	}
}

func TestDefaultPath(t *testing.T) {
	t.Setenv("PROGRAMDATA", `D:\ProgramData`)
	got := DefaultPath()
	want := filepath.Join(`D:\ProgramData`, "RiskLens", "agent.yaml")
	if got != want {
		t.Fatalf("DefaultPath = %q, want %q", got, want)
	}
}

func TestLoadUsesEnvConfigPath(t *testing.T) {
	t.Setenv(EnvConfig, testdata("valid.yaml"))
	result, err := Load("", Overrides{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if result.Config.Service.Name != "RiskLensAgent" {
		t.Fatalf("service name = %q", result.Config.Service.Name)
	}
}

func TestInvalidServiceName(t *testing.T) {
	cfg := Defaults()
	cfg.Agent.LogFile = filepath.Join(t.TempDir(), "agent.log")
	cfg.Service.Name = `RiskLens\Agent`
	if err := Validate(&cfg); err == nil {
		t.Fatal("expected error for service name with backslash")
	}
}

func TestOversizedConfigRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.yaml")
	data := make([]byte, maxConfigBytes+1)
	for i := range data {
		data[i] = 'a'
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path, Overrides{})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ErrInvalid, got %v", err)
	}
}

func TestDirectoryConfigRejected(t *testing.T) {
	_, err := Load(t.TempDir(), Overrides{})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ErrInvalid, got %v", err)
	}
}

func TestRelativeLogFileRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	content := []byte("agent:\n  log_level: info\n  log_file: relative.log\n  shutdown_timeout: 15s\nservice:\n  name: RiskLensAgent\n")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path, Overrides{})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ErrInvalid, got %v", err)
	}
}
