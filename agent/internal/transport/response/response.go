// Package response describes an inbound reply in protocol-neutral terms.
// Like package request it avoids net/http so consumers of the transport
// interface never import the HTTP stack.
package response

import (
	"encoding/json"
	"errors"
	"strings"
)

var errNilResponse = errors.New("response is nil")

// Status codes the agent actually reasons about. Declared here so modules
// can switch on them without importing net/http.
const (
	StatusOK                  = 200
	StatusCreated             = 201
	StatusAccepted            = 202
	StatusNoContent           = 204
	StatusBadRequest          = 400
	StatusUnauthorized        = 401
	StatusForbidden           = 403
	StatusNotFound            = 404
	StatusConflict            = 409
	StatusUnprocessableEntity = 422
	StatusTooManyRequests     = 429
	StatusInternalServerError = 500
	StatusBadGateway          = 502
	StatusServiceUnavailable  = 503
)

// Header holds response headers. Lookups are case-insensitive, matching HTTP
// semantics, without depending on net/textproto canonicalization.
type Header map[string][]string

// Get returns the first value for key, or "".
func (h Header) Get(key string) string {
	if len(h) == 0 {
		return ""
	}
	if values, ok := h[key]; ok && len(values) > 0 {
		return values[0]
	}
	for name, values := range h {
		if strings.EqualFold(name, key) && len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

// Values returns every value for key, or nil.
func (h Header) Values(key string) []string {
	if len(h) == 0 {
		return nil
	}
	if values, ok := h[key]; ok {
		return values
	}
	for name, values := range h {
		if strings.EqualFold(name, key) {
			return values
		}
	}
	return nil
}

// Response is a fully-read reply. Body is already drained and bounded by the
// transport, so consumers never handle an io.ReadCloser or leak connections.
type Response struct {
	StatusCode int
	Headers    Header
	Body       []byte
}

// IsSuccess reports a 2xx status.
func (r *Response) IsSuccess() bool {
	return r != nil && r.StatusCode >= 200 && r.StatusCode < 300
}

// IsClientError reports a 4xx status.
func (r *Response) IsClientError() bool {
	return r != nil && r.StatusCode >= 400 && r.StatusCode < 500
}

// IsServerError reports a 5xx status.
func (r *Response) IsServerError() bool {
	return r != nil && r.StatusCode >= 500
}

// ContentType returns the Content-Type header without parameters.
func (r *Response) ContentType() string {
	if r == nil {
		return ""
	}
	value := r.Headers.Get("Content-Type")
	if index := strings.IndexByte(value, ';'); index >= 0 {
		value = value[:index]
	}
	return strings.TrimSpace(value)
}

// DecodeJSON unmarshals the body into target regardless of status code.
// Use it to read error envelopes; use Transport.DecodeResponse when a
// non-2xx status should itself be treated as an error.
func (r *Response) DecodeJSON(target any) error {
	if r == nil {
		return errNilResponse
	}
	return json.Unmarshal(r.Body, target)
}
