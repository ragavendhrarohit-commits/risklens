package storage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadLegacyExample(t *testing.T) {
	path, err := filepath.Abs(filepath.Join("..", "..", "..", "configs", "agent.example.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewYAMLStore(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.General.ServiceName != "RiskLensAgent" {
		t.Fatalf("service = %q", cfg.General.ServiceName)
	}
	if cfg.Logging.Level != "info" {
		t.Fatalf("level = %q", cfg.Logging.Level)
	}
	if cfg.General.ShutdownTimeoutRaw != "15s" {
		t.Fatalf("timeout = %q", cfg.General.ShutdownTimeoutRaw)
	}
	if cfg.DeviceUpload.IntervalRaw != "5m" {
		t.Fatalf("device_upload.interval = %q", cfg.DeviceUpload.IntervalRaw)
	}
}

func TestLoadNewSections(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	logFile := filepath.ToSlash(filepath.Join(dir, "agent.log"))
	stateDir := filepath.ToSlash(dir)
	identity := filepath.ToSlash(filepath.Join(dir, "identity.json"))
	content := []byte("schema_version: 1\n" +
		"general:\n  service_name: RiskLensAgent\n  display_name: RiskLens Endpoint Agent\n  shutdown_timeout: 20s\n" +
		"logging:\n  level: debug\n  file: '" + logFile + "'\n" +
		"modules:\n  enabled: [eventlog]\n" +
		"performance:\n  max_concurrency: 2\n" +
		"storage:\n  state_dir: '" + stateDir + "'\n  identity_file: '" + identity + "'\n")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewYAMLStore(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Logging.Level != "debug" || cfg.General.ShutdownTimeoutRaw != "20s" {
		t.Fatalf("cfg = %+v", cfg)
	}
	if len(cfg.Modules.Enabled) != 1 || cfg.Modules.Enabled[0] != "eventlog" {
		t.Fatalf("modules = %v", cfg.Modules.Enabled)
	}
}

func TestLoadMissing(t *testing.T) {
	store, err := NewYAMLStore(filepath.Join(t.TempDir(), "missing.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Load()
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestLoadCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.yaml")
	if err := os.WriteFile(path, []byte("agent: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewYAMLStore(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Load()
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("got %v", err)
	}
}

func TestSaveRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.yaml")
	store, err := NewYAMLStore(path)
	if err != nil {
		t.Fatal(err)
	}
	src, err := NewYAMLStore(mustAbs(t, filepath.Join("..", "..", "..", "configs", "agent.example.yaml")))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := src.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.General.ServiceName != cfg.General.ServiceName || got.Logging.Level != cfg.Logging.Level {
		t.Fatalf("round trip mismatch: %+v", got)
	}
}

func TestRelativePathRejected(t *testing.T) {
	if _, err := NewYAMLStore("agent.yaml"); err == nil {
		t.Fatal("expected error")
	}
}

func mustAbs(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}
