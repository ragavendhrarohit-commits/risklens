package browseractivity

import "time"

type State string

const (
	StateIdle       State = "idle"
	StateCollecting State = "collecting"
	StateUploading  State = "uploading"
	StateReady      State = "ready"
	StateFailed     State = "failed"
	StateSkipped    State = "skipped"
)

type Snapshot struct {
	State          State
	LastAttemptAt  time.Time
	LastSuccessAt  time.Time
	SamplesFound   int
	Stored         int
	HTTPStatusCode int
	LastError      string
	Cycles         uint64
}
