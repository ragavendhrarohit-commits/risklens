package device

import (
	"time"

	"risklens/agent/internal/modules/interfaces"
)

// State is the device inventory state machine. It is orthogonal to the
// modules/interfaces.Status lifecycle: a module can be Running while its
// inventory State is Failed (collection failed but the module itself is up).
//
//	NotCollected -> Collecting -> Collected
//	                           \-> Failed
//	                           \-> Partial (some fields missing, still usable)
type State string

const (
	StateNotCollected State = "not_collected"
	StateCollecting   State = "collecting"
	StateCollected    State = "collected"
	StatePartial      State = "partial"
	StateFailed       State = "failed"
)

// Snapshot is an immutable view of the module's inventory state, safe to
// hand to other components. Zero-value fields mean "not yet available".
type Snapshot struct {
	State       State
	CollectedAt time.Time
	LastError   string
	Info        Info
}

// ComparablePayload is the fingerprint input. Device owns which fields are facts.
func (s Snapshot) ComparablePayload() Info {
	return s.Info.Comparable()
}

// SampleTime is the collect timestamp for the pipeline envelope.
func (s Snapshot) SampleTime() time.Time {
	if !s.Info.CollectedAt.IsZero() {
		return s.Info.CollectedAt
	}
	return s.CollectedAt
}

// CollectionHealth maps inventory state to collector health.
// HTTP/upload outcomes must not use this mapping.
func (s Snapshot) CollectionHealth() interfaces.Health {
	switch s.State {
	case StateCollected:
		return interfaces.HealthHealthy
	case StatePartial:
		return interfaces.HealthDegraded
	case StateFailed:
		return interfaces.HealthUnhealthy
	default:
		return interfaces.HealthUnknown
	}
}
