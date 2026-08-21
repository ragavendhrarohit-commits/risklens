//go:build windows

package service

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/windows/svc"
)

type recordingSink struct {
	mu        sync.Mutex
	events    []uint32
	shutdowns int
}

func (r *recordingSink) OnWindowsSessionChange(eventType uint32, sessionID uint32) {
	r.mu.Lock()
	r.events = append(r.events, eventType)
	r.mu.Unlock()
}

func (r *recordingSink) OnWindowsShutdown(ctx context.Context) {
	r.mu.Lock()
	r.shutdowns++
	r.mu.Unlock()
}

func TestHandlerStop(t *testing.T) {
	h := &Handler{
		App:     testApp(t),
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		Timeout: 2 * time.Second,
	}

	requests := make(chan svc.ChangeRequest)
	changes := make(chan svc.Status, 16)
	done := make(chan struct{})
	var ssec bool
	var errno uint32
	go func() {
		ssec, errno = h.Execute(nil, requests, changes)
		close(done)
	}()

	if !waitSCMState(t, changes, svc.Running) {
		t.Fatal("service did not enter Running")
	}

	requests <- svc.ChangeRequest{
		Cmd:           svc.Interrogate,
		CurrentStatus: svc.Status{State: svc.Running},
	}

	select {
	case <-done:
		t.Fatal("interrogate stopped the handler")
	case <-time.After(100 * time.Millisecond):
	}

	requests <- svc.ChangeRequest{Cmd: svc.Stop}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("handler did not stop")
	}
	if errno != 0 {
		t.Fatalf("errno = %d ssec=%v", errno, ssec)
	}
	if h.ExitCode() != 0 {
		t.Fatalf("ExitCode = %d", h.ExitCode())
	}
}

func TestHandlerSessionChangeDoesNotStop(t *testing.T) {
	sink := &recordingSink{}
	h := &Handler{
		App:           testApp(t),
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		Timeout:       2 * time.Second,
		SessionChange: sink,
	}

	requests := make(chan svc.ChangeRequest)
	changes := make(chan svc.Status, 16)
	done := make(chan struct{})
	go func() {
		_, _ = h.Execute(nil, requests, changes)
		close(done)
	}()

	if !waitSCMState(t, changes, svc.Running) {
		t.Fatal("service did not enter Running")
	}

	requests <- svc.ChangeRequest{Cmd: svc.SessionChange, EventType: 0x5}

	select {
	case <-done:
		t.Fatal("session change stopped the handler")
	case <-time.After(150 * time.Millisecond):
	}

	deadline := time.Now().Add(time.Second)
	var events []uint32
	for time.Now().Before(deadline) {
		sink.mu.Lock()
		events = append([]uint32(nil), sink.events...)
		sink.mu.Unlock()
		if len(events) == 1 && events[0] == 0x5 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(events) != 1 || events[0] != 0x5 {
		t.Fatalf("events = %v", events)
	}

	requests <- svc.ChangeRequest{Cmd: svc.Stop}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("handler did not stop")
	}
}

func TestHandlerPreShutdownFlushesSessions(t *testing.T) {
	sink := &recordingSink{}
	h := &Handler{
		App:           testApp(t),
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		Timeout:       2 * time.Second,
		SessionChange: sink,
	}

	requests := make(chan svc.ChangeRequest)
	changes := make(chan svc.Status, 16)
	done := make(chan struct{})
	go func() {
		_, _ = h.Execute(nil, requests, changes)
		close(done)
	}()

	if !waitSCMState(t, changes, svc.Running) {
		t.Fatal("service did not enter Running")
	}

	requests <- svc.ChangeRequest{Cmd: svc.PreShutdown}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("handler did not stop after preshutdown")
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if sink.shutdowns != 1 {
		t.Fatalf("shutdowns = %d", sink.shutdowns)
	}
}

func TestHandlerStopDoesNotFlushSessions(t *testing.T) {
	sink := &recordingSink{}
	h := &Handler{
		App:           testApp(t),
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		Timeout:       2 * time.Second,
		SessionChange: sink,
	}

	requests := make(chan svc.ChangeRequest)
	changes := make(chan svc.Status, 16)
	done := make(chan struct{})
	go func() {
		_, _ = h.Execute(nil, requests, changes)
		close(done)
	}()

	if !waitSCMState(t, changes, svc.Running) {
		t.Fatal("service did not enter Running")
	}

	requests <- svc.ChangeRequest{Cmd: svc.Stop}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("handler did not stop")
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if sink.shutdowns != 0 {
		t.Fatalf("service stop must not flush sessions, shutdowns=%d", sink.shutdowns)
	}
}

func TestHandlerShutdownFlushesSessions(t *testing.T) {
	sink := &recordingSink{}
	h := &Handler{
		App:           testApp(t),
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		Timeout:       2 * time.Second,
		SessionChange: sink,
	}

	requests := make(chan svc.ChangeRequest)
	changes := make(chan svc.Status, 16)
	done := make(chan struct{})
	go func() {
		_, _ = h.Execute(nil, requests, changes)
		close(done)
	}()

	if !waitSCMState(t, changes, svc.Running) {
		t.Fatal("service did not enter Running")
	}

	requests <- svc.ChangeRequest{Cmd: svc.Shutdown}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("handler did not stop after shutdown")
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if sink.shutdowns != 1 {
		t.Fatalf("shutdowns = %d", sink.shutdowns)
	}
}

func waitSCMState(t *testing.T, changes <-chan svc.Status, want svc.State) bool {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case status := <-changes:
			if status.State == want {
				return true
			}
		case <-deadline:
			return false
		}
	}
}
