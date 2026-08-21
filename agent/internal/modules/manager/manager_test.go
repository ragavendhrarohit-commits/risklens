package manager

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"risklens/agent/internal/modules/base"
	"risklens/agent/internal/modules/interfaces"
	"risklens/agent/internal/modules/registry"
)

type orderBook struct {
	mu    sync.Mutex
	start []string
	stop  []string
}

func (o *orderBook) recordStart(name string) {
	o.mu.Lock()
	o.start = append(o.start, name)
	o.mu.Unlock()
}

func (o *orderBook) recordStop(name string) {
	o.mu.Lock()
	o.stop = append(o.stop, name)
	o.mu.Unlock()
}

type stub struct {
	*base.Module
	book      *orderBook
	startErr  error
	stopErr   error
	block     <-chan struct{}
	startedCh chan struct{}
}

func (s *stub) Start(ctx context.Context) error {
	s.MarkStarting()
	if s.block != nil {
		select {
		case <-ctx.Done():
			s.MarkFailed()
			return ctx.Err()
		case <-s.block:
		}
	}
	if err := ctx.Err(); err != nil {
		s.MarkFailed()
		return err
	}
	if s.startErr != nil {
		s.MarkFailed()
		return s.startErr
	}
	if s.book != nil {
		s.book.recordStart(s.Name())
	}
	if s.startedCh != nil {
		close(s.startedCh)
	}
	s.MarkRunning()
	return nil
}

func (s *stub) Stop(ctx context.Context) error {
	s.MarkStopping()
	if s.stopErr != nil {
		s.MarkFailed()
		return s.stopErr
	}
	if s.book != nil {
		s.book.recordStop(s.Name())
	}
	s.MarkStopped()
	return nil
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newManager(t *testing.T, modules ...interfaces.Module) *Manager {
	t.Helper()
	reg := registry.New()
	for _, module := range modules {
		if err := reg.Register(module); err != nil {
			t.Fatal(err)
		}
	}
	return New(reg, testLogger())
}

func TestStartStopOrder(t *testing.T) {
	book := &orderBook{}
	alpha := &stub{Module: base.New("alpha", "1.0.0"), book: book}
	beta := &stub{Module: base.New("beta", "1.0.0"), book: book}
	gamma := &stub{Module: base.New("gamma", "1.0.0"), book: book}
	mgr := newManager(t, alpha, beta, gamma)

	if err := mgr.StartAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := mgr.StopAll(context.Background()); err != nil {
		t.Fatal(err)
	}

	book.mu.Lock()
	defer book.mu.Unlock()
	if got := join(book.start); got != "alpha,beta,gamma" {
		t.Fatalf("start order = %s", got)
	}
	if got := join(book.stop); got != "gamma,beta,alpha" {
		t.Fatalf("stop order = %s", got)
	}
}

func TestStartFailureRollsBack(t *testing.T) {
	book := &orderBook{}
	alpha := &stub{Module: base.New("alpha", "1.0.0"), book: book}
	beta := &stub{Module: base.New("beta", "1.0.0"), book: book, startErr: errors.New("boom")}
	gamma := &stub{Module: base.New("gamma", "1.0.0"), book: book}
	mgr := newManager(t, alpha, beta, gamma)

	err := mgr.StartAll(context.Background())
	if err == nil {
		t.Fatal("expected start failure")
	}
	if !errors.Is(err, beta.startErr) {
		t.Fatalf("error = %v", err)
	}

	book.mu.Lock()
	defer book.mu.Unlock()
	if got := join(book.start); got != "alpha" {
		t.Fatalf("started = %s", got)
	}
	if got := join(book.stop); got != "alpha" {
		t.Fatalf("rolled back = %s", got)
	}
	if gamma.Status() != interfaces.StatusRegistered {
		t.Fatalf("gamma should not start, status=%s", gamma.Status())
	}
	if alpha.Status() != interfaces.StatusStopped {
		t.Fatalf("alpha status = %s", alpha.Status())
	}
	if beta.Status() != interfaces.StatusFailed {
		t.Fatalf("beta status = %s", beta.Status())
	}
}

func TestStopFailureContinues(t *testing.T) {
	book := &orderBook{}
	alpha := &stub{Module: base.New("alpha", "1.0.0"), book: book}
	beta := &stub{Module: base.New("beta", "1.0.0"), book: book, stopErr: errors.New("stop failed")}
	mgr := newManager(t, alpha, beta)

	if err := mgr.StartAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	err := mgr.StopAll(context.Background())
	if err == nil {
		t.Fatal("expected stop failure")
	}
	book.mu.Lock()
	defer book.mu.Unlock()
	if got := join(book.stop); got != "alpha" {
		t.Fatalf("stop continued after beta failure, got %s", got)
	}
}

func TestCancelledContextBeforeStart(t *testing.T) {
	book := &orderBook{}
	alpha := &stub{Module: base.New("alpha", "1.0.0"), book: book}
	mgr := newManager(t, alpha)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := mgr.StartAll(ctx); err == nil {
		t.Fatal("expected cancelled context error")
	}
	book.mu.Lock()
	defer book.mu.Unlock()
	if len(book.start) != 0 {
		t.Fatalf("started = %v", book.start)
	}
}

func TestCancelledContextDuringStart(t *testing.T) {
	book := &orderBook{}
	block := make(chan struct{})
	alpha := &stub{Module: base.New("alpha", "1.0.0"), book: book, block: block}
	beta := &stub{Module: base.New("beta", "1.0.0"), book: book}
	mgr := newManager(t, alpha, beta)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- mgr.StartAll(ctx)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("StartAll did not return after cancel")
	}

	book.mu.Lock()
	defer book.mu.Unlock()
	if len(book.start) != 0 {
		t.Fatalf("started = %v", book.start)
	}
	if beta.Status() != interfaces.StatusRegistered {
		t.Fatalf("beta status = %s", beta.Status())
	}
}

func TestNilContextRejected(t *testing.T) {
	mgr := newManager(t)
	if err := mgr.StartAll(nil); !errors.Is(err, ErrNilContext) {
		t.Fatalf("StartAll: %v", err)
	}
	if err := mgr.StopAll(nil); !errors.Is(err, ErrNilContext) {
		t.Fatalf("StopAll: %v", err)
	}
}

func TestAlreadyStarted(t *testing.T) {
	alpha := &stub{Module: base.New("alpha", "1.0.0"), book: &orderBook{}}
	mgr := newManager(t, alpha)
	if err := mgr.StartAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := mgr.StartAll(context.Background()); !errors.Is(err, ErrAlreadyStarted) {
		t.Fatalf("expected ErrAlreadyStarted, got %v", err)
	}
	if err := mgr.StopAll(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestStatusReport(t *testing.T) {
	alpha := &stub{Module: base.New("alpha", "1.2.3"), book: &orderBook{}}
	mgr := newManager(t, alpha)
	if err := mgr.StartAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	snaps := mgr.Status()
	if len(snaps) != 1 {
		t.Fatalf("len = %d", len(snaps))
	}
	if snaps[0].Name != "alpha" || snaps[0].Version != "1.2.3" {
		t.Fatalf("snapshot = %+v", snaps[0])
	}
	if snaps[0].Status != interfaces.StatusRunning {
		t.Fatalf("status = %s", snaps[0].Status)
	}
	if snaps[0].Health != interfaces.HealthHealthy {
		t.Fatalf("health = %s", snaps[0].Health)
	}
	if err := mgr.StopAll(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestEmptyRegistry(t *testing.T) {
	mgr := newManager(t)
	if err := mgr.StartAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := mgr.StopAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(mgr.Status()) != 0 {
		t.Fatal("expected no snapshots")
	}
}

func join(names []string) string {
	out := ""
	for i, name := range names {
		if i > 0 {
			out += ","
		}
		out += name
	}
	return out
}
