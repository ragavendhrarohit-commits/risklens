package app

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	cfgmodel "risklens/agent/internal/configuration/model"
	"risklens/agent/internal/identity/model"
	"risklens/agent/internal/modules/base"
	"risklens/agent/internal/modules/interfaces"
)

type stubIdentity struct {
	id model.Identity
}

type stubConfiguration struct {
	cfg cfgmodel.Config
}

func (s stubConfiguration) Current() (cfgmodel.Config, bool) {
	if s.cfg.General.ServiceName == "" {
		return cfgmodel.Config{}, false
	}
	return s.cfg, true
}

func (s stubIdentity) Current() (model.Identity, bool) {
	if s.id.AgentID == "" {
		return model.Identity{}, false
	}
	return s.id, true
}

type syncBuffer struct {
	mu  sync.Mutex
	buf []byte
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buf = append(s.buf, p...)
	return len(p), nil
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return string(s.buf)
}

func testLogger(buf *syncBuffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

func TestNewNilLogger(t *testing.T) {
	if _, err := New(time.Second, nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestNewDefaultTimeout(t *testing.T) {
	var buf syncBuffer
	application, err := New(0, testLogger(&buf))
	if err != nil {
		t.Fatal(err)
	}
	if application.shutdownTimeout != 15*time.Second {
		t.Fatalf("timeout = %s", application.shutdownTimeout)
	}
}

func TestRunStartsAndStops(t *testing.T) {
	var buf syncBuffer
	application, err := New(2*time.Second, testLogger(&buf))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- application.Run(ctx)
	}()

	waitForLog(t, &buf, "Service started")
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after cancel")
	}

	out := buf.String()
	for _, msg := range []string{"Service started", "Service stopping", "Service stopped"} {
		if !strings.Contains(out, msg) {
			t.Fatalf("missing %q in logs:\n%s", msg, out)
		}
	}
}

func TestRunLogsAgentID(t *testing.T) {
	var buf syncBuffer
	application, err := New(2*time.Second, testLogger(&buf))
	if err != nil {
		t.Fatal(err)
	}
	application.UseIdentity(stubIdentity{id: model.Identity{
		AgentID:   "550e8400-e29b-41d4-a716-446655440000",
		InstallID: "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
	}})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- application.Run(ctx)
	}()
	waitForLog(t, &buf, "550e8400-e29b-41d4-a716-446655440000")
	cancel()
	<-done
}

func TestUseConfiguration(t *testing.T) {
	var buf syncBuffer
	application, err := New(time.Second, testLogger(&buf))
	if err != nil {
		t.Fatal(err)
	}
	application.UseConfiguration(stubConfiguration{cfg: cfgmodel.Config{
		General: cfgmodel.General{ServiceName: "RiskLensAgent"},
	}})
	got, ok := application.configuration.Current()
	if !ok || got.General.ServiceName != "RiskLensAgent" {
		t.Fatalf("configuration not attached: %+v ok=%v", got, ok)
	}
}

func TestRunNilContext(t *testing.T) {
	var buf syncBuffer
	application, err := New(time.Second, testLogger(&buf))
	if err != nil {
		t.Fatal(err)
	}
	if err := application.Run(nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestShutdownCancelledContext(t *testing.T) {
	var buf syncBuffer
	application, err := New(2*time.Second, testLogger(&buf))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = application.Shutdown(ctx)
	if !errors.Is(err, ErrShutdownTimeout) {
		t.Fatalf("expected ErrShutdownTimeout, got %v", err)
	}
	if !strings.Contains(buf.String(), "Service stopped") {
		t.Fatalf("expected Service stopped after timeout path:\n%s", buf.String())
	}
}

type stubModule struct {
	*base.Module
}

func (s *stubModule) Start(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		s.MarkFailed()
		return err
	}
	s.MarkRunning()
	return nil
}

func (s *stubModule) Stop(ctx context.Context) error {
	s.MarkStopped()
	return nil
}

func TestRunStartsAndStopsModules(t *testing.T) {
	var buf syncBuffer
	mod := &stubModule{Module: base.New("alpha", "1.0.0")}
	application, err := New(2*time.Second, testLogger(&buf), mod)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- application.Run(ctx)
	}()

	waitForLog(t, &buf, "Service started")
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && mod.Status() != interfaces.StatusRunning {
		time.Sleep(10 * time.Millisecond)
	}
	if mod.Status() != interfaces.StatusRunning {
		t.Fatalf("module status = %s", mod.Status())
	}
	snaps := application.ModuleStatus()
	if len(snaps) != 1 || snaps[0].Name != "alpha" {
		t.Fatalf("status = %+v", snaps)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return")
	}
	if mod.Status() != interfaces.StatusStopped {
		t.Fatalf("module status after stop = %s", mod.Status())
	}
}

func TestNewRejectsDuplicateModules(t *testing.T) {
	var buf syncBuffer
	_, err := New(time.Second, testLogger(&buf),
		&stubModule{Module: base.New("alpha", "1.0.0")},
		&stubModule{Module: base.New("alpha", "2.0.0")},
	)
	if err == nil {
		t.Fatal("expected duplicate error")
	}
}

func waitForLog(t *testing.T, buf *syncBuffer, msg string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(buf.String(), msg) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %q in:\n%s", msg, buf.String())
}
