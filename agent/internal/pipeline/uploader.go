package pipeline

import "context"

// Uploader sends one Snapshot through Transport.
// Implementations MUST NOT schedule, collect, fingerprint, or retry.
type Uploader interface {
	// Upload maps Snapshot.Payload to an HTTP DTO and sends it.
	// Success is a nil error. Any error is failure for the Pipeline.
	Upload(ctx context.Context, snap Snapshot) error
}
