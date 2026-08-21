// Package errors defines the error vocabulary shared by every transport
// implementation. Callers branch on Kind through the exported sentinels
// using errors.Is, so they never depend on net/http or on a concrete
// transport package.
package errors

import (
	stderrors "errors"
	"fmt"
)

// Kind classifies a transport failure. It is deliberately coarse: modules
// decide policy (log, mark failed, ignore) from the kind alone.
type Kind string

const (
	// KindTimeout means the request exceeded the transport timeout or the
	// caller's context deadline.
	KindTimeout Kind = "timeout"

	// KindCanceled means the caller's context was cancelled, typically
	// because the agent is shutting down.
	KindCanceled Kind = "canceled"

	// KindConnection means the request never produced a response: DNS
	// failure, connection refused, TLS handshake failure, reset.
	KindConnection Kind = "connection"

	// KindInvalidRequest means the request was rejected before any I/O.
	KindInvalidRequest Kind = "invalid_request"

	// KindEncode means the request body could not be marshalled.
	KindEncode Kind = "encode"

	// KindDecode means the response body was not valid JSON for the target.
	KindDecode Kind = "decode"

	// KindInvalidResponse means a response arrived but is unusable: empty
	// when a body was required, or larger than the configured limit.
	KindInvalidResponse Kind = "invalid_response"

	// KindUnexpectedStatus means a well-formed response carried a status
	// the caller did not accept.
	KindUnexpectedStatus Kind = "unexpected_status"
)

// Sentinels for errors.Is. They carry no context of their own; the concrete
// *Error reports itself as matching the sentinel for its Kind.
var (
	ErrTimeout          = stderrors.New("transport: timeout")
	ErrCanceled         = stderrors.New("transport: canceled")
	ErrConnection       = stderrors.New("transport: connection failure")
	ErrInvalidRequest   = stderrors.New("transport: invalid request")
	ErrEncode           = stderrors.New("transport: encode failure")
	ErrDecode           = stderrors.New("transport: decode failure")
	ErrInvalidResponse  = stderrors.New("transport: invalid response")
	ErrUnexpectedStatus = stderrors.New("transport: unexpected status")
)

var sentinels = map[Kind]error{
	KindTimeout:          ErrTimeout,
	KindCanceled:         ErrCanceled,
	KindConnection:       ErrConnection,
	KindInvalidRequest:   ErrInvalidRequest,
	KindEncode:           ErrEncode,
	KindDecode:           ErrDecode,
	KindInvalidResponse:  ErrInvalidResponse,
	KindUnexpectedStatus: ErrUnexpectedStatus,
}

// Error is the concrete transport error. Op names the failing stage
// ("send", "decode response", ...) so logs stay actionable, and StatusCode
// is populated when the failure relates to a received response.
type Error struct {
	Kind       Kind
	Op         string
	StatusCode int
	Err        error
}

// New builds an Error with no associated HTTP status.
func New(kind Kind, op string, err error) *Error {
	return &Error{Kind: kind, Op: op, Err: err}
}

// WithStatus builds an Error tied to a received response status.
func WithStatus(kind Kind, op string, statusCode int, err error) *Error {
	return &Error{Kind: kind, Op: op, StatusCode: statusCode, Err: err}
}

func (e *Error) Error() string {
	base := string(e.Kind)
	if e.Op != "" {
		base = e.Op + ": " + base
	}
	if e.StatusCode != 0 {
		base = fmt.Sprintf("%s (HTTP %d)", base, e.StatusCode)
	}
	if e.Err != nil {
		return base + ": " + e.Err.Error()
	}
	return base
}

// Unwrap exposes the underlying cause so callers can still reach
// context.DeadlineExceeded, *net.OpError, *json.SyntaxError, and friends.
func (e *Error) Unwrap() error { return e.Err }

// Is reports the error as matching the sentinel for its Kind, which is what
// makes errors.Is(err, ErrTimeout) work without exporting the struct.
func (e *Error) Is(target error) bool {
	sentinel, ok := sentinels[e.Kind]
	return ok && sentinel == target
}

// KindOf returns the Kind of err, or "" when err is not a transport error.
func KindOf(err error) Kind {
	var transportErr *Error
	if stderrors.As(err, &transportErr) {
		return transportErr.Kind
	}
	return ""
}

// StatusOf returns the HTTP status associated with err, or 0.
func StatusOf(err error) int {
	var transportErr *Error
	if stderrors.As(err, &transportErr) {
		return transportErr.StatusCode
	}
	return 0
}
