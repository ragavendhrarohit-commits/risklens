package schedule

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewRejectsInvalidConfig(t *testing.T) {
	fn := func(context.Context) {}
	cases := []struct {
		name string
		cfg  Config
		fn   Func
		want error
	}{
		{name: "zero interval", cfg: Config{Interval: 0}, fn: fn, want: ErrInvalidInterval},
		{name: "negative interval", cfg: Config{Interval: -time.Second}, fn: fn, want: ErrInvalidInterval},
		{name: "negative jitter", cfg: Config{Interval: time.Second, Jitter: -1}, fn: fn, want: ErrInvalidJitter},
		{name: "nil func", cfg: Config{Interval: time.Second}, fn: nil, want: ErrNilFunc},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := New(tc.cfg, tc.fn)
			if got != nil {
				t.Fatal("expected nil scheduler")
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestExecutesImmediately(t *testing.T) {
	ran := make(chan struct{}, 1)
	sched, err := New(Config{Interval: time.Hour}, func(context.Context) {
		select {
		case ran <- struct{}{}:
		default:
		}
	})
	if err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	sched.Start(context.Background())
	defer sched.Stop()

	select {
	case <-ran:
		if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
			t.Fatalf("first run waited %s; want immediate", elapsed)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("first run did not execute immediately")
	}
}

func TestRepeatsOnInterval(t *testing.T) {
	var calls atomic.Int32
	sched, err := New(Config{Interval: 40 * time.Millisecond}, func(context.Context) {
		calls.Add(1)
	})
	if err != nil {
		t.Fatal(err)
	}

	sched.Start(context.Background())
	time.Sleep(130 * time.Millisecond)
	sched.Stop()

	got := calls.Load()
	// Immediate + ~3 waits of 40ms in 130ms → typically 4. Allow 3–5 for scheduling slack.
	if got < 3 || got > 5 {
		t.Fatalf("calls = %d, want 3–5 in 130ms at 40ms interval", got)
	}
}

func TestAppliesJitter(t *testing.T) {
	var times []time.Time
	var mu sync.Mutex

	sched, err := New(Config{Interval: 60 * time.Millisecond, Jitter: 40 * time.Millisecond}, func(context.Context) {
		mu.Lock()
		times = append(times, time.Now())
		mu.Unlock()
	})
	if err != nil {
		t.Fatal(err)
	}
	// Force every wait to interval + jitter so the gap is distinguishable from Interval.
	sched.offset = func(max time.Duration) time.Duration { return max }

	sched.Start(context.Background())
	deadline := time.After(350 * time.Millisecond)
	for {
		mu.Lock()
		n := len(times)
		mu.Unlock()
		if n >= 3 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("only %d runs before deadline", n)
		case <-time.After(10 * time.Millisecond):
		}
	}
	sched.Stop()

	mu.Lock()
	defer mu.Unlock()
	if len(times) < 3 {
		t.Fatalf("runs = %d", len(times))
	}
	gap := times[1].Sub(times[0])
	// Expected wait is 60ms + 40ms = 100ms. Immediate first run has no jitter.
	if gap < 85*time.Millisecond || gap > 150*time.Millisecond {
		t.Fatalf("gap after first run = %s, want ~100ms (interval+jitter), not 60ms", gap)
	}
}

func TestZeroJitterUsesExactInterval(t *testing.T) {
	var times []time.Time
	var mu sync.Mutex
	sched, err := New(Config{Interval: 50 * time.Millisecond, Jitter: 0}, func(context.Context) {
		mu.Lock()
		times = append(times, time.Now())
		mu.Unlock()
	})
	if err != nil {
		t.Fatal(err)
	}

	sched.Start(context.Background())
	time.Sleep(130 * time.Millisecond)
	sched.Stop()

	mu.Lock()
	defer mu.Unlock()
	if len(times) < 2 {
		t.Fatalf("runs = %d", len(times))
	}
	gap := times[1].Sub(times[0])
	if gap < 35*time.Millisecond || gap > 80*time.Millisecond {
		t.Fatalf("gap = %s, want ~50ms with zero jitter", gap)
	}
}

func TestNeverOverlapsExecutions(t *testing.T) {
	var concurrent atomic.Int32
	var maxConcurrent atomic.Int32
	var calls atomic.Int32

	sched, err := New(Config{Interval: 20 * time.Millisecond}, func(context.Context) {
		calls.Add(1)
		cur := concurrent.Add(1)
		for {
			old := maxConcurrent.Load()
			if cur <= old || maxConcurrent.CompareAndSwap(old, cur) {
				break
			}
		}
		time.Sleep(70 * time.Millisecond)
		concurrent.Add(-1)
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	sched.Start(ctx)
	time.Sleep(180 * time.Millisecond)
	cancel()
	sched.Stop()

	if maxConcurrent.Load() > 1 {
		t.Fatalf("overlapping executions detected: max=%d", maxConcurrent.Load())
	}
	if calls.Load() < 1 {
		t.Fatal("expected at least one execution")
	}
}

func TestGracefulStop(t *testing.T) {
	started := make(chan struct{}, 1)
	sched, err := New(Config{Interval: time.Hour}, func(ctx context.Context) {
		select {
		case started <- struct{}{}:
		default:
		}
		<-ctx.Done()
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	sched.Start(ctx)
	<-started
	cancel()

	done := make(chan struct{})
	go func() {
		sched.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("scheduler stop hung")
	}
}

func TestContextCancellationStopsLoop(t *testing.T) {
	var calls atomic.Int32
	sched, err := New(Config{Interval: 25 * time.Millisecond}, func(context.Context) {
		calls.Add(1)
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	sched.Start(ctx)
	time.Sleep(40 * time.Millisecond)
	cancel()
	time.Sleep(80 * time.Millisecond)
	afterCancel := calls.Load()
	time.Sleep(80 * time.Millisecond)
	if calls.Load() != afterCancel {
		t.Fatalf("loop kept running after parent cancel: before=%d after=%d", afterCancel, calls.Load())
	}
	sched.Stop()
}

func TestInFlightSeesCancellation(t *testing.T) {
	started := make(chan struct{})
	sawCancel := make(chan struct{})
	sched, err := New(Config{Interval: time.Hour}, func(ctx context.Context) {
		close(started)
		<-ctx.Done()
		close(sawCancel)
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	sched.Start(ctx)
	<-started
	cancel()

	select {
	case <-sawCancel:
	case <-time.After(2 * time.Second):
		t.Fatal("in-flight func did not observe ctx cancellation")
	}
	sched.Stop()
}

func TestStopIsIdempotent(t *testing.T) {
	sched, err := New(Config{Interval: time.Hour}, func(context.Context) {})
	if err != nil {
		t.Fatal(err)
	}
	sched.Start(context.Background())
	sched.Stop()
	sched.Stop()
}

func TestStartWhileRunningDoesNotDoubleLoop(t *testing.T) {
	var calls atomic.Int32
	sched, err := New(Config{Interval: 40 * time.Millisecond}, func(context.Context) {
		calls.Add(1)
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	sched.Start(ctx)
	sched.Start(ctx)
	time.Sleep(90 * time.Millisecond)
	sched.Stop()

	got := calls.Load()
	// One loop: immediate + ~2 intervals. Two loops would be roughly double.
	if got > 5 {
		t.Fatalf("calls = %d; second Start likely started another loop", got)
	}
	if got < 1 {
		t.Fatal("expected at least the immediate run")
	}
}

func TestNextWaitClampsNegativeJitterOffset(t *testing.T) {
	sched, err := New(Config{Interval: 10 * time.Millisecond, Jitter: 50 * time.Millisecond}, func(context.Context) {})
	if err != nil {
		t.Fatal(err)
	}
	sched.offset = func(time.Duration) time.Duration { return -50 * time.Millisecond }
	if wait := sched.nextWait(); wait != minWait {
		t.Fatalf("wait = %s, want %s", wait, minWait)
	}
}
