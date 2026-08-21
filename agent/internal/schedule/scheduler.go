// Package schedule is the shared ticker for every recurring agent job.
//
// It has no knowledge of collectors, snapshots, HTTP, or inventory types.
// Future pipeline hosts (device, users, software, processes, activity)
// and other loops such as heartbeat should use this package instead of
// copying a ticker.
package schedule

import (
	"context"
	"errors"
	"math/rand/v2"
	"sync"
	"time"
)

const minWait = time.Millisecond

var (
	// ErrInvalidInterval is returned when Config.Interval is not > 0.
	ErrInvalidInterval = errors.New("schedule: interval must be greater than 0")
	// ErrInvalidJitter is returned when Config.Jitter is negative.
	ErrInvalidJitter = errors.New("schedule: jitter must be greater than or equal to 0")
	// ErrNilFunc is returned when New is called without a Func.
	ErrNilFunc = errors.New("schedule: func is required")
)

// Func is invoked on each tick. It must honour ctx and return promptly
// after cancellation so Stop can complete.
type Func func(ctx context.Context)

// Config controls how often Func runs.
type Config struct {
	// Interval is the base wait between ticks after the immediate first run.
	// Must be > 0.
	Interval time.Duration
	// Jitter is the maximum absolute offset applied to waits after the first
	// run. Each wait is Interval + offset with offset in [-Jitter, +Jitter].
	// The result is clamped to at least 1ms. Zero disables jitter.
	// The immediate first execution never waits and never applies jitter.
	Jitter time.Duration
}

// jitterOffsetFunc returns a duration in [-max, +max]. Tests replace it.
type jitterOffsetFunc func(max time.Duration) time.Duration

// Scheduler runs Func on a fixed interval without overlapping invocations.
type Scheduler struct {
	interval time.Duration
	jitter   time.Duration
	fn       Func
	offset   jitterOffsetFunc

	mu       sync.Mutex
	running  bool
	cancel   context.CancelFunc
	done     chan struct{}
	inflight sync.Mutex
}

// New builds a scheduler. It does not start the loop.
func New(cfg Config, fn Func) (*Scheduler, error) {
	if cfg.Interval <= 0 {
		return nil, ErrInvalidInterval
	}
	if cfg.Jitter < 0 {
		return nil, ErrInvalidJitter
	}
	if fn == nil {
		return nil, ErrNilFunc
	}
	return &Scheduler{
		interval: cfg.Interval,
		jitter:   cfg.Jitter,
		fn:       fn,
		offset:   randomJitterOffset,
	}, nil
}

// Start launches the loop in a background goroutine.
// The first Func call runs immediately.
// Calling Start twice is a no-op while already running.
func (s *Scheduler) Start(parent context.Context) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	s.done = make(chan struct{})
	s.running = true
	s.mu.Unlock()

	go s.loop(ctx)
}

// Stop cancels the loop and waits for it to exit. Safe to call multiple times.
// An in-flight Func is not abandoned; it must return when ctx is cancelled.
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

	s.run(ctx)

	for {
		if ctx.Err() != nil {
			return
		}
		timer := time.NewTimer(s.nextWait())
		select {
		case <-ctx.Done():
			stopTimer(timer)
			return
		case <-timer.C:
			s.run(ctx)
		}
	}
}

func (s *Scheduler) run(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	if !s.inflight.TryLock() {
		return
	}
	defer s.inflight.Unlock()

	if ctx.Err() != nil {
		return
	}
	s.fn(ctx)
}

func (s *Scheduler) nextWait() time.Duration {
	wait := s.interval
	if s.jitter > 0 && s.offset != nil {
		wait += s.offset(s.jitter)
	}
	if wait < minWait {
		return minWait
	}
	return wait
}

func randomJitterOffset(max time.Duration) time.Duration {
	if max <= 0 {
		return 0
	}
	span := int64(max)*2 + 1
	return time.Duration(rand.Int64N(span)) - max
}

func stopTimer(timer *time.Timer) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}
