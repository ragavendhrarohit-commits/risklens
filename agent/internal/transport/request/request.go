// Package request describes an outbound call in protocol-neutral terms.
// It intentionally contains no net/http types so that modules building
// requests never acquire a dependency on the HTTP stack.
package request

import (
	"errors"
	"sort"
	"strings"
)

// Method is the verb of a request.
type Method string

const (
	MethodGET    Method = "GET"
	MethodPOST   Method = "POST"
	MethodPUT    Method = "PUT"
	MethodDELETE Method = "DELETE"
)

// Valid reports whether m is one of the verbs this transport supports.
func (m Method) Valid() bool {
	switch m {
	case MethodGET, MethodPOST, MethodPUT, MethodDELETE:
		return true
	default:
		return false
	}
}

func (m Method) String() string { return string(m) }

// Request is an immutable-by-convention description of one call. Path is
// relative to the transport's base URL. Body, when non-nil, is JSON encoded
// by the transport; the caller passes a plain Go value.
//
// The With* helpers return a copy, so a Request can be safely used as a
// template for several calls.
type Request struct {
	Method  Method
	Path    string
	Query   map[string][]string
	Headers map[string]string
	Body    any
}

// New builds a request with no body.
func New(method Method, path string) Request {
	return Request{Method: method, Path: path}
}

// GET builds a bodyless GET request.
func GET(path string) Request { return New(MethodGET, path) }

// POST builds a POST request whose body is JSON encoded by the transport.
func POST(path string, body any) Request {
	return Request{Method: MethodPOST, Path: path, Body: body}
}

// PUT builds a PUT request whose body is JSON encoded by the transport.
func PUT(path string, body any) Request {
	return Request{Method: MethodPUT, Path: path, Body: body}
}

// DELETE builds a bodyless DELETE request.
func DELETE(path string) Request { return New(MethodDELETE, path) }

// WithHeader returns a copy carrying an additional header.
func (r Request) WithHeader(key, value string) Request {
	out := r.clone()
	if out.Headers == nil {
		out.Headers = make(map[string]string, 1)
	}
	out.Headers[key] = value
	return out
}

// WithHeaders returns a copy carrying all of the supplied headers.
func (r Request) WithHeaders(headers map[string]string) Request {
	if len(headers) == 0 {
		return r
	}
	out := r.clone()
	if out.Headers == nil {
		out.Headers = make(map[string]string, len(headers))
	}
	for key, value := range headers {
		out.Headers[key] = value
	}
	return out
}

// WithQuery returns a copy with an additional query parameter appended.
func (r Request) WithQuery(key, value string) Request {
	out := r.clone()
	if out.Query == nil {
		out.Query = make(map[string][]string, 1)
	}
	out.Query[key] = append(out.Query[key], value)
	return out
}

// WithBody returns a copy carrying the supplied body.
func (r Request) WithBody(body any) Request {
	out := r.clone()
	out.Body = body
	return out
}

// Validate checks the request can be executed. Transports call this before
// touching the network so misuse fails fast and identically everywhere.
func (r Request) Validate() error {
	if !r.Method.Valid() {
		return errors.New("unsupported method " + string(r.Method))
	}
	if strings.TrimSpace(r.Path) == "" {
		return errors.New("path is required")
	}
	if r.Body != nil && (r.Method == MethodGET || r.Method == MethodDELETE) {
		return errors.New(string(r.Method) + " requests must not carry a body")
	}
	return nil
}

// EncodedQuery renders Query in a stable order so logs and tests are
// deterministic. It returns "" when there are no parameters.
func (r Request) EncodedQuery() string {
	if len(r.Query) == 0 {
		return ""
	}
	keys := make([]string, 0, len(r.Query))
	for key := range r.Query {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var builder strings.Builder
	for _, key := range keys {
		for _, value := range r.Query[key] {
			if builder.Len() > 0 {
				builder.WriteByte('&')
			}
			builder.WriteString(escape(key))
			builder.WriteByte('=')
			builder.WriteString(escape(value))
		}
	}
	return builder.String()
}

func (r Request) clone() Request {
	out := r
	if r.Headers != nil {
		out.Headers = make(map[string]string, len(r.Headers))
		for key, value := range r.Headers {
			out.Headers[key] = value
		}
	}
	if r.Query != nil {
		out.Query = make(map[string][]string, len(r.Query))
		for key, values := range r.Query {
			out.Query[key] = append([]string(nil), values...)
		}
	}
	return out
}

const upperhex = "0123456789ABCDEF"

// escape percent-encodes a query component. Implemented here rather than via
// net/url so this package stays free of net/* imports.
func escape(s string) string {
	var builder strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9',
			c == '-', c == '_', c == '.', c == '~':
			builder.WriteByte(c)
		case c == ' ':
			builder.WriteByte('+')
		default:
			builder.WriteByte('%')
			builder.WriteByte(upperhex[c>>4])
			builder.WriteByte(upperhex[c&0x0f])
		}
	}
	return builder.String()
}
