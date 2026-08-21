// Package middleware provides composable wrappers around a transport's send
// path: header injection, correlation ids, and logging.
//
// Retry and offline queueing are deliberately absent. They are scheduled for
// a later sprint and must not be smuggled in here, because a retry that the
// caller cannot see would silently change the timing contract every module
// depends on.
package middleware

import (
	"context"
	"log/slog"
	"time"

	"risklens/agent/internal/transport/request"
	"risklens/agent/internal/transport/response"
)

// RoundTripper executes a request. The concrete transport supplies the
// innermost implementation; middleware wraps it.
type RoundTripper func(ctx context.Context, req request.Request) (*response.Response, error)

// Middleware decorates a RoundTripper.
type Middleware func(next RoundTripper) RoundTripper

// Chain composes middleware into one. The first argument is outermost, so
// Chain(a, b)(base) executes a, then b, then base.
func Chain(middlewares ...Middleware) Middleware {
	return func(next RoundTripper) RoundTripper {
		for i := len(middlewares) - 1; i >= 0; i-- {
			if middlewares[i] != nil {
				next = middlewares[i](next)
			}
		}
		return next
	}
}

// Headers adds static headers to every request. Existing per-request headers
// win, so a caller can always override a default.
func Headers(headers map[string]string) Middleware {
	return func(next RoundTripper) RoundTripper {
		if len(headers) == 0 {
			return next
		}
		return func(ctx context.Context, req request.Request) (*response.Response, error) {
			for key, value := range headers {
				if _, exists := req.Headers[key]; !exists {
					req = req.WithHeader(key, value)
				}
			}
			return next(ctx, req)
		}
	}
}

// RequestID stamps a correlation header on each request so agent logs can be
// joined against backend logs. generate must be non-nil and safe for
// concurrent use.
func RequestID(header string, generate func() string) Middleware {
	return func(next RoundTripper) RoundTripper {
		if header == "" || generate == nil {
			return next
		}
		return func(ctx context.Context, req request.Request) (*response.Response, error) {
			if _, exists := req.Headers[header]; !exists {
				if id := generate(); id != "" {
					req = req.WithHeader(header, id)
				}
			}
			return next(ctx, req)
		}
	}
}

// Logging emits one debug line before the call and one after, including
// status and duration. It never logs request or response bodies, which may
// carry machine identifiers.
func Logging(logger *slog.Logger) Middleware {
	return func(next RoundTripper) RoundTripper {
		if logger == nil {
			return next
		}
		return func(ctx context.Context, req request.Request) (*response.Response, error) {
			started := time.Now()
			logger.Debug("transport request",
				"method", req.Method.String(),
				"path", req.Path,
			)

			resp, err := next(ctx, req)
			elapsed := time.Since(started)

			if err != nil {
				logger.Debug("transport failure",
					"method", req.Method.String(),
					"path", req.Path,
					"duration_ms", elapsed.Milliseconds(),
					"error", err,
				)
				return resp, err
			}

			status := 0
			if resp != nil {
				status = resp.StatusCode
			}
			logger.Debug("transport response",
				"method", req.Method.String(),
				"path", req.Path,
				"status", status,
				"duration_ms", elapsed.Milliseconds(),
			)
			return resp, nil
		}
	}
}
