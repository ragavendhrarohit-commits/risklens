package service

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"risklens/agent/internal/app"
)

func testApp(t *testing.T) Application {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	application, err := app.New(2*time.Second, logger)
	if err != nil {
		t.Fatal(err)
	}
	return application
}

func waitState(t *testing.T, mu *sync.Mutex, states *[]State, want State) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		for _, state := range *states {
			if state == want {
				mu.Unlock()
				return
			}
		}
		mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for state %d", want)
}

func runLoop(t *testing.T, application Application, timeout time.Duration, requests <-chan Request, report func(State, time.Duration)) <-chan uint32 {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	done := make(chan uint32, 1)
	go func() {
		done <- executeLoop(application, logger, timeout, requests, report)
	}()
	return done
}

func TestLoopStop(t *testing.T) {
	requests := make(chan Request, 4)
	var mu sync.Mutex
	var states []State
	report := func(state State, _ time.Duration) {
		mu.Lock()
		states = append(states, state)
		mu.Unlock()
	}

	application := testApp(t)
	done := runLoop(t, application, 2*time.Second, requests, report)

	waitState(t, &mu, &states, StateRunning)
	requests <- Request{Cmd: CmdStop}

	select {
	case errno := <-done:
		if errno != 0 {
			t.Fatalf("errno = %d", errno)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("loop did not stop")
	}

	waitState(t, &mu, &states, StateStopPending)
}

func TestLoopInterrogateDoesNotStop(t *testing.T) {
	requests := make(chan Request, 4)
	var mu sync.Mutex
	var states []State
	report := func(state State, _ time.Duration) {
		mu.Lock()
		states = append(states, state)
		mu.Unlock()
	}

	application := testApp(t)
	done := runLoop(t, application, 2*time.Second, requests, report)

	waitState(t, &mu, &states, StateRunning)
	requests <- Request{Cmd: CmdInterrogate, Current: StateRunning}

	select {
	case errno := <-done:
		t.Fatalf("interrogate stopped the loop, errno=%d", errno)
	case <-time.After(150 * time.Millisecond):
	}

	requests <- Request{Cmd: CmdShutdown}
	select {
	case errno := <-done:
		if errno != 0 {
			t.Fatalf("errno = %d", errno)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("loop did not stop after shutdown")
	}
}

func TestLoopReportsStartPendingThenRunning(t *testing.T) {
	requests := make(chan Request)
	var mu sync.Mutex
	var states []State
	report := func(state State, _ time.Duration) {
		mu.Lock()
		states = append(states, state)
		mu.Unlock()
	}

	application := testApp(t)
	done := runLoop(t, application, time.Second, requests, report)
	waitState(t, &mu, &states, StateRunning)
	requests <- Request{Cmd: CmdStop}
	<-done

	mu.Lock()
	defer mu.Unlock()
	if len(states) < 2 || states[0] != StateStartPending || states[1] != StateRunning {
		t.Fatalf("states = %v", states)
	}
}

func TestLoopNilApplication(t *testing.T) {
	errno := executeLoop(nil, slog.New(slog.NewTextHandler(io.Discard, nil)), time.Second, nil, nil)
	if errno != 40 {
		t.Fatalf("errno = %d", errno)
	}
}

type hangApp struct{}

func (hangApp) Run(ctx context.Context) error {
	time.Sleep(2 * time.Second)
	return nil
}

func TestLoopShutdownTimeout(t *testing.T) {
	requests := make(chan Request, 1)
	done := runLoop(t, hangApp{}, 20*time.Millisecond, requests, nil)
	requests <- Request{Cmd: CmdStop}

	select {
	case errno := <-done:
		if errno != 50 {
			t.Fatalf("errno = %d, want 50", errno)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout path did not return")
	}
}

type panicApp struct{}

func (panicApp) Run(ctx context.Context) error {
	panic("boom")
}

func TestLoopRecoversPanic(t *testing.T) {
	done := runLoop(t, panicApp{}, time.Second, make(chan Request), nil)
	select {
	case errno := <-done:
		if errno != 40 {
			t.Fatalf("errno = %d, want 40", errno)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("panic was not recovered")
	}
}
