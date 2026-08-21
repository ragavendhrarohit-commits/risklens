package pipeline

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"risklens/agent/internal/fingerprint"
	"risklens/agent/internal/schedule"
)

var (
	ErrNilCollector = errors.New("pipeline: collector is required")
	ErrNilUploader  = errors.New("pipeline: uploader is required")
)

// Config is per-kind scheduling for one Pipeline instance.
type Config struct {
	Interval time.Duration
	Jitter   time.Duration
	// Timeout, if > 0, is a deadline applied to each collect+upload tick.
	Timeout time.Duration
}

// Pipeline orchestrates one collector kind: collect, compare, decide, upload,
// remember. It MUST NOT import concrete payload types.
type Pipeline struct {
	collector Collector
	uploader  Uploader
	logger    *slog.Logger
	timeout   time.Duration

	scheduler *schedule.Scheduler

	mu          sync.Mutex
	running     bool
	health      Health
	lastSuccess Fingerprint
	pending     bool
}

// New builds a Pipeline. It does not start the scheduler.
func New(cfg Config, collector Collector, uploader Uploader, logger *slog.Logger) (*Pipeline, error) {
	if collector == nil {
		return nil, ErrNilCollector
	}
	if uploader == nil {
		return nil, ErrNilUploader
	}
	if logger == nil {
		logger = slog.Default()
	}
	p := &Pipeline{
		collector: collector,
		uploader:  uploader,
		logger:    logger.With("kind", collector.Kind()),
		timeout:   cfg.Timeout,
		health:    HealthDisabled,
	}
	sched, err := schedule.New(schedule.Config{Interval: cfg.Interval, Jitter: cfg.Jitter}, p.RunOnce)
	if err != nil {
		return nil, err
	}
	p.scheduler = sched
	return p, nil
}

// Kind returns the collector kind string.
func (p *Pipeline) Kind() string {
	return p.collector.Kind()
}

// Health is this instance's collector health. Independent of other kinds.
func (p *Pipeline) Health() Health {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.health
}

// LastSuccess is the fingerprint of the last successful upload, or empty.
func (p *Pipeline) LastSuccess() Fingerprint {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastSuccess
}

// Pending reports whether the next tick must retry upload.
func (p *Pipeline) Pending() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.pending
}

// Start launches this kind's scheduler. Other kinds are unaffected.
// Each scheduled tick invokes RunOnce. Start does not contain collect,
// compare, upload, or remember logic.
func (p *Pipeline) Start(ctx context.Context) {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return
	}
	p.running = true
	if p.health == HealthDisabled {
		p.health = HealthWarning
	}
	p.mu.Unlock()

	p.logger.Info("Collector Started")
	p.scheduler.Start(ctx)
}

// Stop ends this kind's scheduler. Other kinds are unaffected.
func (p *Pipeline) Stop() {
	p.scheduler.Stop()
	p.mu.Lock()
	p.running = false
	p.health = HealthDisabled
	p.mu.Unlock()
}

// RunOnce executes one pipeline cycle on the calling goroutine:
// collect, compare, upload if required, remember, return.
// It does not start the scheduler, spawn a goroutine, or wait for Interval.
func (p *Pipeline) RunOnce(ctx context.Context) {
	defer func() {
		if rec := recover(); rec != nil {
			p.logger.Error("Collector Recovered", "panic", fmt.Sprint(rec))
			p.setHealth(HealthError)
		}
	}()

	if p.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.timeout)
		defer cancel()
	}

	snap, collectErr := p.collector.Collect(ctx)
	fp := fingerprint.Of(snap.Kind, snap.SchemaVersion, snap.Partial, snap.Payload)
	if ctx.Err() != nil && !snap.Uploadable() {
		return
	}

	if !snap.Uploadable() || fp.Empty() {
		p.logger.Error("Collection Failed", "error", errorString(collectErr))
		p.setHealth(HealthError)
		p.logger.Info("Collector Finished")
		return
	}

	if collectErr != nil || snap.Partial {
		p.logger.Warn("partial collect", "partial", snap.Partial, "error", errorString(collectErr))
	}

	if p.shouldSkip(fp) {
		if snap.Partial {
			p.setHealth(HealthWarning)
		} else {
			p.setHealth(HealthHealthy)
		}
		p.logger.Info("Collector Finished")
		return
	}

	p.logger.Info("Upload Started")
	err := p.uploader.Upload(ctx, snap)
	if err != nil {
		p.logger.Error("Upload Failed", "error", err)
		p.markPending()
		p.logger.Info("Retry Scheduled")
		p.logger.Info("Collector Finished")
		return
	}

	p.remember(fp, snap.Partial)
	p.logger.Info("Collector Finished")
}

func (p *Pipeline) shouldSkip(fp Fingerprint) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return !p.lastSuccess.Empty() && !p.pending && p.lastSuccess.Equal(fp)
}

func (p *Pipeline) markPending() {
	p.mu.Lock()
	p.pending = true
	p.health = HealthWarning
	p.mu.Unlock()
}

func (p *Pipeline) remember(fp Fingerprint, partial bool) {
	p.mu.Lock()
	p.lastSuccess = fp
	p.pending = false
	if partial {
		p.health = HealthWarning
	} else {
		p.health = HealthHealthy
	}
	p.mu.Unlock()
}

func (p *Pipeline) setHealth(h Health) {
	p.mu.Lock()
	p.health = h
	p.mu.Unlock()
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
