package heartbeat

import "time"

// State is the heartbeat module's own state, independent of Module Status.
type State string

const (
	StateIdle    State = "idle"
	StateRunning State = "running"
	StateSkipped State = "skipped"
	StateFailed  State = "failed"
)

// Snapshot is a diagnostic view of the latest heartbeat attempt.
type Snapshot struct {
	State          State
	LastAttemptAt  time.Time
	LastSuccessAt  time.Time
	HTTPStatusCode int
	ServerVersion  string
	ServerTime     time.Time
	RequestID      string
	ErrorCode      string
	LastError      string
	BeatsSent      uint64
}
