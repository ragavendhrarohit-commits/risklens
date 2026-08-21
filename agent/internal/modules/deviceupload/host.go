package deviceupload

import (
	"context"
	"errors"
	"log/slog"

	"risklens/agent/internal/modules/device"
	"risklens/agent/internal/pipeline"
)

// HealthObserver receives collect and upload outcomes from the Device pipeline.
// Pipeline itself is not modified; the host wraps adapters.
type HealthObserver interface {
	OnCollect()
	OnUpload(err error)
}

// PipelineHost assembles a Device Pipeline from the existing adapters.
// Start runs the Pipeline scheduler. RunOnce remains available for tests.
type PipelineHost struct {
	pipe      *pipeline.Pipeline
	collector pipeline.Collector
	uploader  pipeline.Uploader
	config    pipeline.Config
}

// NewPipelineHost wires collector adapter, uploader adapter, and pipeline.New.
// The returned Pipeline is not started. observer may be nil.
func NewPipelineHost(
	cfg pipeline.Config,
	refresher device.InventoryRefresher,
	identity IdentityProvider,
	client *Client,
	logger *slog.Logger,
	observer HealthObserver,
) (*PipelineHost, error) {
	if refresher == nil {
		return nil, errors.New("device refresher is required")
	}
	if identity == nil {
		return nil, errors.New("identity provider is required")
	}
	if client == nil {
		return nil, errors.New("client is required")
	}
	if logger == nil {
		logger = slog.Default()
	}

	collector, err := device.NewPipelineCollector(refresher)
	if err != nil {
		return nil, err
	}
	uploader, err := NewUploader(identity, client)
	if err != nil {
		return nil, err
	}
	pipe, err := pipeline.New(cfg, observeCollector(collector, observer), observeUploader(uploader, observer), logger)
	if err != nil {
		return nil, err
	}

	return &PipelineHost{
		pipe:      pipe,
		collector: collector,
		uploader:  uploader,
		config:    cfg,
	}, nil
}

// Pipeline is the constructed instance.
func (h *PipelineHost) Pipeline() *pipeline.Pipeline {
	if h == nil {
		return nil
	}
	return h.pipe
}

// Collector is the injected pipeline.Collector (Device adapter).
func (h *PipelineHost) Collector() pipeline.Collector {
	if h == nil {
		return nil
	}
	return h.collector
}

// Uploader is the injected pipeline.Uploader (Device Upload adapter).
func (h *PipelineHost) Uploader() pipeline.Uploader {
	if h == nil {
		return nil
	}
	return h.uploader
}

// Config is the pipeline.Config passed at construction.
func (h *PipelineHost) Config() pipeline.Config {
	if h == nil {
		return pipeline.Config{}
	}
	return h.config
}

// Start launches the Pipeline scheduler. The first tick runs immediately.
func (h *PipelineHost) Start(ctx context.Context) {
	if h == nil || h.pipe == nil {
		return
	}
	h.pipe.Start(ctx)
}

// Stop ends the Pipeline scheduler and waits for the loop to exit.
func (h *PipelineHost) Stop() {
	if h == nil || h.pipe == nil {
		return
	}
	h.pipe.Stop()
}

// RunOnce executes one pipeline cycle on the calling goroutine.
// It MUST NOT start the scheduler.
func (h *PipelineHost) RunOnce(ctx context.Context) {
	if h == nil || h.pipe == nil {
		return
	}
	h.pipe.RunOnce(ctx)
}

type observedCollector struct {
	inner    pipeline.Collector
	observer HealthObserver
}

func observeCollector(inner pipeline.Collector, observer HealthObserver) pipeline.Collector {
	if observer == nil {
		return inner
	}
	return &observedCollector{inner: inner, observer: observer}
}

func (c *observedCollector) Kind() string { return c.inner.Kind() }

func (c *observedCollector) Collect(ctx context.Context) (pipeline.Snapshot, error) {
	snap, err := c.inner.Collect(ctx)
	c.observer.OnCollect()
	return snap, err
}

type observedUploader struct {
	inner    pipeline.Uploader
	observer HealthObserver
}

func observeUploader(inner pipeline.Uploader, observer HealthObserver) pipeline.Uploader {
	if observer == nil {
		return inner
	}
	return &observedUploader{inner: inner, observer: observer}
}

func (u *observedUploader) Upload(ctx context.Context, snap pipeline.Snapshot) error {
	err := u.inner.Upload(ctx, snap)
	u.observer.OnUpload(err)
	return err
}
