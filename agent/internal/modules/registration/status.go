package registration

import "time"

// State is the registration state machine. It is orthogonal to the
// modules/interfaces.Status lifecycle: a module can be Running while its
// registration State is Failed (registration attempt failed but the module
// itself is up and reporting).
//
//	NotRegistered -> Registering -> Registered
//	                            \-> Failed
type State string

const (
	StateNotRegistered State = "not_registered"
	StateRegistering   State = "registering"
	StateRegistered    State = "registered"
	StateFailed        State = "failed"
)

// Snapshot is an immutable view of the module's registration state, safe to
// hand to other components. Zero-value fields mean "not yet available".
type Snapshot struct {
	State          State
	LastAttemptAt  time.Time
	HTTPStatusCode int
	ServerVersion  string
	ServerTime     time.Time
	RequestID      string
	LastError      string
	ErrorCode      string
}
