package device

import (
	"context"
	"errors"

	"risklens/agent/internal/pipeline"
)

const pipelineKind = "device"

// InventoryRefresher is the Device module surface the pipeline adapter needs.
// *Module implements it via Refresh.
type InventoryRefresher interface {
	Refresh(ctx context.Context) Snapshot
}

// PipelineCollector adapts Device Refresh to pipeline.Collector.
// It translates Device snapshot → pipeline.Snapshot only. Device owns
// comparable vs transport payload construction.
type PipelineCollector struct {
	source InventoryRefresher
}

// NewPipelineCollector wraps a Device refresher.
func NewPipelineCollector(source InventoryRefresher) (*PipelineCollector, error) {
	if source == nil {
		return nil, errors.New("device refresher is required")
	}
	return &PipelineCollector{source: source}, nil
}

// Kind is the stable pipeline collector id.
func (c *PipelineCollector) Kind() string {
	return pipelineKind
}

// Collect calls Refresh and maps the Device snapshot onto a pipeline.Snapshot.
// Fingerprint is left empty; fingerprint.Of owns hashing.
func (c *PipelineCollector) Collect(ctx context.Context) (pipeline.Snapshot, error) {
	dev := c.source.Refresh(ctx)
	out := pipeline.Snapshot{
		Kind:          pipelineKind,
		SchemaVersion: pipeline.SchemaVersionV1,
		CollectedAt:   dev.SampleTime(),
		Payload:       dev.ComparablePayload(),
		Partial:       dev.State == StatePartial,
		CollectError:  dev.LastError,
	}
	if dev.LastError != "" {
		return out, errors.New(dev.LastError)
	}
	return out, nil
}

var _ pipeline.Collector = (*PipelineCollector)(nil)
var _ InventoryRefresher = (*Module)(nil)
