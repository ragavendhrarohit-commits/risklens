package registration

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	txerrors "risklens/agent/internal/transport/errors"
	txinterfaces "risklens/agent/internal/transport/interfaces"
	"risklens/agent/internal/transport/request"
	"risklens/agent/internal/transport/response"
)

// fakeTransport is a full stand-in for the HTTP transport. Its presence is
// the point of Sprint 6: the registration package can be exercised end to
// end without a socket, and without importing net/http anywhere.
type fakeTransport struct {
	resp *response.Response
	err  error

	mu       sync.Mutex
	calls    int
	lastReq  request.Request
	lastCtx  context.Context
	buildErr error
}

func (f *fakeTransport) BuildRequest(method request.Method, path string, body any) (request.Request, error) {
	if f.buildErr != nil {
		return request.Request{}, f.buildErr
	}
	req := request.Request{Method: method, Path: path, Body: body}
	if err := req.Validate(); err != nil {
		return request.Request{}, txerrors.New(txerrors.KindInvalidRequest, "build request", err)
	}
	return req, nil
}

func (f *fakeTransport) Send(ctx context.Context, req request.Request) (*response.Response, error) {
	f.mu.Lock()
	f.calls++
	f.lastReq = req
	f.lastCtx = ctx
	f.mu.Unlock()

	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
}

func (f *fakeTransport) DecodeResponse(resp *response.Response, target any) error {
	if resp == nil {
		return txerrors.New(txerrors.KindInvalidResponse, "decode response", errors.New("no response"))
	}
	if !resp.IsSuccess() {
		return txerrors.WithStatus(txerrors.KindUnexpectedStatus, "decode response", resp.StatusCode, nil)
	}
	if target == nil {
		return nil
	}
	if len(resp.Body) == 0 {
		return txerrors.WithStatus(txerrors.KindInvalidResponse, "decode response", resp.StatusCode,
			errors.New("empty body"))
	}
	if err := json.Unmarshal(resp.Body, target); err != nil {
		return txerrors.WithStatus(txerrors.KindDecode, "decode response", resp.StatusCode, err)
	}
	return nil
}

func (f *fakeTransport) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeTransport) request() request.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastReq
}

// respondWith builds a transport that returns the given status and raw body.
func respondWith(status int, body string) *fakeTransport {
	return &fakeTransport{
		resp: &response.Response{
			StatusCode: status,
			Headers:    response.Header{"Content-Type": {"application/json"}},
			Body:       []byte(body),
		},
	}
}

// respondJSON builds a transport that returns the JSON encoding of value.
func respondJSON(t *testing.T, status int, value any) *fakeTransport {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return respondWith(status, string(encoded))
}

// failWith builds a transport whose Send always fails.
func failWith(err error) *fakeTransport {
	return &fakeTransport{err: err}
}

var _ txinterfaces.Transport = (*fakeTransport)(nil)
