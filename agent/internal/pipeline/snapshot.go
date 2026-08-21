// Package pipeline is generic collection orchestration.
//
// It knows Collector, Snapshot, Fingerprint, and Uploader only.
// It MUST NOT import device, users, software, or any concrete payload type.
// It MUST NOT hash payloads; fingerprint.Of owns hashing.
package pipeline

import (
	"time"

	"risklens/agent/internal/fingerprint"
)

const (
	// SchemaVersionV1 is the first Snapshot envelope version.
	SchemaVersionV1 = 1
)

// Fingerprint is the opaque comparable identity of snapshot content.
// Values are produced by fingerprint.Of, not by collectors or Pipeline.
type Fingerprint = fingerprint.Fingerprint

// Snapshot is one immutable sample of one collector kind.
// It is not module diagnostic state (device.Snapshot / deviceupload.Snapshot).
type Snapshot struct {
	Kind          string      `json:"kind"`
	SchemaVersion int         `json:"schema_version"`
	CollectedAt   time.Time   `json:"collected_at"`
	Fingerprint   Fingerprint `json:"fingerprint"`
	Payload       any         `json:"payload,omitempty"`
	Partial       bool        `json:"partial"`
	CollectError  string      `json:"collect_error,omitempty"`
}

// Uploadable reports whether the Snapshot has a payload the Pipeline may send.
// Fingerprint is computed by fingerprint.Of after Collect, not required here.
// CollectedAt is not required.
func (s Snapshot) Uploadable() bool {
	return s.Payload != nil
}
