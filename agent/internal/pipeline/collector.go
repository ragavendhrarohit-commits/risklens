package pipeline

import "context"

// Collector gathers one kind of local fact and returns a Snapshot.
// Implementations MUST NOT upload, schedule, or call the backend.
type Collector interface {
	// Kind is a stable unique id (device, users, software, processes, …).
	Kind() string
	// Collect produces one Snapshot (payload + envelope). It MUST NOT hash.
	// fingerprint.Of owns fingerprint generation.
	// A partial Snapshot plus error is preferred over an empty failure.
	Collect(ctx context.Context) (Snapshot, error)
}
