package manager

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"risklens/agent/internal/configuration/model"
	"risklens/agent/internal/configuration/storage"
	"risklens/agent/internal/configuration/validation"
)

func writeLegacy(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "agent.yaml")
	logFile := filepath.ToSlash(filepath.Join(dir, "agent.log"))
	content := []byte("agent:\n  log_level: info\n  log_file: '" + logFile + "'\n  shutdown_timeout: 15s\n" +
		"service:\n  name: RiskLensAgent\n  display_name: RiskLens Endpoint Agent\n")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func newMgr(t *testing.T, path string) *Manager {
	t.Helper()
	store, err := storage.NewYAMLStore(path)
	if err != nil {
		t.Fatal(err)
	}
	mgr, err := NewWithInterval(store, slog.New(slog.NewTextHandler(io.Discard, nil)), 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	return mgr
}

func TestLoadAndCurrent(t *testing.T) {
	path := writeLegacy(t, t.TempDir())
	mgr := newMgr(t, path)
	cfg, err := mgr.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.General.ServiceName != "RiskLensAgent" {
		t.Fatalf("service = %q", cfg.General.ServiceName)
	}
	current, ok := mgr.Current()
	if !ok || current.Logging.Level != "info" {
		t.Fatalf("current = %+v ok=%v", current, ok)
	}
}

func TestReload(t *testing.T) {
	dir := t.TempDir()
	path := writeLegacy(t, dir)
	mgr := newMgr(t, path)
	if _, err := mgr.Load(); err != nil {
		t.Fatal(err)
	}

	logFile := filepath.ToSlash(filepath.Join(dir, "agent.log"))
	updated := []byte("agent:\n  log_level: debug\n  log_file: '" + logFile + "'\n  shutdown_timeout: 15s\n" +
		"service:\n  name: RiskLensAgent\n  display_name: RiskLens Endpoint Agent\n")
	if err := os.WriteFile(path, updated, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := mgr.Reload()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Logging.Level != "debug" {
		t.Fatalf("level = %q", cfg.Logging.Level)
	}
}

func TestInvalidReloadKeepsPrevious(t *testing.T) {
	dir := t.TempDir()
	path := writeLegacy(t, dir)
	mgr := newMgr(t, path)
	if _, err := mgr.Load(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("agent: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := mgr.Reload()
	if err == nil {
		t.Fatal("expected reload error")
	}
	current, ok := mgr.Current()
	if !ok || current.Logging.Level != "info" {
		t.Fatalf("previous config lost: %+v", current)
	}
}

func TestValidateInvalid(t *testing.T) {
	mgr := newMgr(t, writeLegacy(t, t.TempDir()))
	cfg := model.Defaults()
	cfg.Logging.Level = "nope"
	if err := mgr.Validate(&cfg); !errors.Is(err, validation.ErrInvalid) {
		t.Fatalf("got %v", err)
	}
}

func TestWatcherNotifications(t *testing.T) {
	dir := t.TempDir()
	path := writeLegacy(t, dir)
	mgr := newMgr(t, path)
	if _, err := mgr.Load(); err != nil {
		t.Fatal(err)
	}

	notified := make(chan model.Config, 1)
	unsub := mgr.Subscribe(func(cfg model.Config) {
		select {
		case notified <- cfg:
		default:
		}
	})
	defer unsub()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := mgr.Watch(ctx); err != nil {
		t.Fatal(err)
	}

	logFile := filepath.ToSlash(filepath.Join(dir, "agent.log"))
	updated := []byte("agent:\n  log_level: warn\n  log_file: '" + logFile + "'\n  shutdown_timeout: 15s\n" +
		"service:\n  name: RiskLensAgent\n  display_name: RiskLens Endpoint Agent\n")
	time.Sleep(40 * time.Millisecond)
	if err := os.WriteFile(path, updated, 0o600); err != nil {
		t.Fatal(err)
	}

	select {
	case cfg := <-notified:
		if cfg.Logging.Level != "warn" {
			t.Fatalf("notified level = %q", cfg.Logging.Level)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("did not receive watcher notification")
	}
}

func TestConcurrentCurrentReads(t *testing.T) {
	mgr := newMgr(t, writeLegacy(t, t.TempDir()))
	cfg, err := mgr.Load()
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, ok := mgr.Current()
			if !ok || got.General.ServiceName != cfg.General.ServiceName {
				errCh <- errors.New("current read failed")
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}
}

func TestNewRequiresStore(t *testing.T) {
	if _, err := New(nil, slog.New(slog.NewTextHandler(io.Discard, nil))); err == nil {
		t.Fatal("expected error")
	}
}

func TestEnvOverride(t *testing.T) {
	dir := t.TempDir()
	path := writeLegacy(t, dir)
	t.Setenv(model.EnvLogLevel, "error")
	mgr := newMgr(t, path)
	cfg, err := mgr.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Logging.Level != "error" {
		t.Fatalf("level = %q", cfg.Logging.Level)
	}
}

func TestEnvDeviceUploadIntervalOverride(t *testing.T) {
	dir := t.TempDir()
	path := writeLegacy(t, dir)
	t.Setenv(model.EnvDeviceUploadInterval, "10m")
	mgr := newMgr(t, path)
	cfg, err := mgr.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DeviceUpload.Interval != 10*time.Minute {
		t.Fatalf("interval = %s", cfg.DeviceUpload.Interval)
	}
}
