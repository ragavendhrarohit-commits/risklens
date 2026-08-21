package deviceupload

import "time"

// State is the device-upload state machine.
//
//	NotUploaded -> Uploading -> Uploaded
//	                         \-> Failed
//	                         \-> Skipped (disabled / inventory unavailable)
type State string

const (
	StateNotUploaded State = "not_uploaded"
	StateUploading   State = "uploading"
	StateUploaded    State = "uploaded"
	StateSkipped     State = "skipped"
	StateFailed      State = "failed"
)

// Snapshot is an immutable view of the upload outcome.
type Snapshot struct {
	State          State
	LastAttemptAt  time.Time
	HTTPStatusCode int
	ServerVersion  string
	ServerTime     time.Time
	RequestID      string
	Created        bool
	LastError      string
	ErrorCode      string
}
