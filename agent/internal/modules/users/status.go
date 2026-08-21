package users

import "time"

// State is the users module collection/upload state.
type State string

const (
	StateIdle       State = "idle"
	StateCollecting State = "collecting"
	StateUploading  State = "uploading"
	StateReady      State = "ready"
	StatePartial    State = "partial"
	StateFailed     State = "failed"
	StateSkipped    State = "skipped"
)

// Snapshot is a diagnostic view of the latest collect/upload cycle.
type Snapshot struct {
	State          State
	LastAttemptAt  time.Time
	LastSuccessAt  time.Time
	AccountsFound  int
	HTTPStatusCode int
	ServerVersion  string
	RequestID      string
	ErrorCode      string
	LastError      string
	Cycles         uint64
}
