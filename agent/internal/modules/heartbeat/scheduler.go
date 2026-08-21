package heartbeat

import (
	"context"
	"sync"
	"time"
)

// BeatFunc is invoked for each scheduled heartbeat. It must respect ctx.
type BeatFunc func(ctx context.Context)

// Scheduler runs BeatFunc on a fixed interval without overlapping invocations.
type Scheduler struct {
	interval time.Duration
	beat     BeatFunc

	mu       sync.Mutex
	running  bool
	cancel   context.CancelFunc
	done     chan struct{}
	inflight sync.Mutex // held for the duration of a single beat
}

// NewScheduler builds a ticker-based scheduler. interval must be > 0.
func NewScheduler(interval time.Duration, beat BeatFunc) *Scheduler {
	return &Scheduler{
		interval: interval,
		beat:     beat,
	}
}

// Start launches the ticker loop in a background goroutine.
// Calling Start twice is a no-op while already running.
func (s *Scheduler) Start(parent context.Context) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	s.done = make(chan struct{})
	s.running = true
	s.mu.Unlock()

	go s.loop(ctx)
}

// Stop cancels the loop and waits for it to exit. Safe to call multiple times.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	cancel := s.cancel
	done := s.done
	s.mu.Unlock()

	cancel()
	<-done

	s.mu.Lock()
	s.running = false
	s.cancel = nil
	s.done = nil
	s.mu.Unlock()
}

func (s *Scheduler) loop(ctx context.Context) {
	defer close(s.done)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	// Immediate first beat so online status is not delayed by a full interval.
	s.runBeat(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runBeat(ctx)
		}
	}
}

func (s *Scheduler) runBeat(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	// Skip if a previous beat is still in flight (no overlapping requests).
	if !s.inflight.TryLock() {
		return
	}
	defer s.inflight.Unlock()

	if ctx.Err() != nil {
		return
	}
	if s.beat != nil {
		s.beat(ctx)
	}
}
