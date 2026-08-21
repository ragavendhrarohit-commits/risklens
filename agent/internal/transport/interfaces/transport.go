// Package interfaces declares the transport contract. Modules depend on this
// package and on request/response/errors only. They never import a concrete
// transport implementation, and never import net/http.
package interfaces

import (
	"context"

	"risklens/agent/internal/transport/request"
	"risklens/agent/internal/transport/response"
)

// Transport is the single abstraction through which agent modules talk to
// the RiskLens backend.
//
// Implementations own the underlying client, its timeout, connection reuse,
// default headers, and body encoding. A module supplies intent (verb, path,
// payload) and interprets the result.
type Transport interface {
	// BuildRequest assembles a request for this transport, applying any
	// transport-level defaults. It performs no I/O and fails only on
	// programmer error such as an unsupported verb or an empty path.
	BuildRequest(method request.Method, path string, body any) (request.Request, error)

	// Send executes req and returns the response once its body has been
	// fully read and bounded.
	//
	// A non-2xx status is NOT an error: Send returns the response so the
	// caller can apply its own semantics (registration treats 409 as
	// success, for example). Send errors only when no usable response was
	// produced: timeout, cancellation, connection failure, encoding
	// failure, or an unreadable body. Every error satisfies errors.Is
	// against a sentinel in transport/errors.
	Send(ctx context.Context, req request.Request) (*response.Response, error)

	// DecodeResponse unmarshals a successful response body into target.
	// It returns ErrUnexpectedStatus for non-2xx, ErrInvalidResponse for a
	// missing body, and ErrDecode for malformed JSON. Callers that need to
	// read a body regardless of status use response.DecodeJSON instead.
	DecodeResponse(resp *response.Response, target any) error
}
