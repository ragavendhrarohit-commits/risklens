package users

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	idmodel "risklens/agent/internal/identity/model"
	"risklens/agent/internal/modules/interfaces"
	txerrors "risklens/agent/internal/transport/errors"
	txinterfaces "risklens/agent/internal/transport/interfaces"
	"risklens/agent/internal/transport/request"
	"risklens/agent/internal/transport/response"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type stubIdentity struct {
	id idmodel.Identity
	ok bool
}

func (s stubIdentity) Current() (idmodel.Identity, bool) { return s.id, s.ok }

type stubCollector struct {
	accounts []Account
	err      error
	calls    int32
}

func (s *stubCollector) Collect(ctx context.Context) ([]Account, error) {
	atomic.AddInt32(&s.calls, 1)
	return s.accounts, s.err
}

type fakeTransport struct {
	resp    *response.Response
	err     error
	mu      sync.Mutex
	calls   int
	lastReq request.Request
}

func (f *fakeTransport) BuildRequest(method request.Method, path string, body any) (request.Request, error) {
	req := request.Request{Method: method, Path: path, Body: body}
	if err := req.Validate(); err != nil {
		return request.Request{}, txerrors.New(txerrors.KindInvalidRequest, "build", err)
	}
	return req, nil
}

func (f *fakeTransport) Send(ctx context.Context, req request.Request) (*response.Response, error) {
	f.mu.Lock()
	f.calls++
	f.lastReq = req
	f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
}

func (f *fakeTransport) DecodeResponse(resp *response.Response, target any) error {
	if resp == nil {
		return txerrors.New(txerrors.KindInvalidResponse, "decode", errors.New("nil"))
	}
	if !resp.IsSuccess() {
		return txerrors.WithStatus(txerrors.KindUnexpectedStatus, "decode", resp.StatusCode, nil)
	}
	if target == nil {
		return nil
	}
	return json.Unmarshal(resp.Body, target)
}

func (f *fakeTransport) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

var _ txinterfaces.Transport = (*fakeTransport)(nil)

func sampleIdentity() idmodel.Identity {
	return idmodel.Identity{
		AgentID:   "9b5d7f3e-1234-4abc-9def-0123456789ab",
		InstallID: "install-1",
	}
}

func sampleAccounts() []Account {
	return []Account{
		{
			Username:         "alice",
			SID:              "S-1-5-21-1-2-3-1001",
			DisplayName:      "Alice",
			AccountType:      AccountTypeLocal,
			Enabled:          true,
			Builtin:          false,
			PasswordRequired: true,
			Source:           SourceLocal,
		},
	}
}

func successBody() []byte {
	return []byte(`{
		"accepted": true,
		"agent_id": "9b5d7f3e-1234-4abc-9def-0123456789ab",
		"stored": 1,
		"inventory_status": "stored",
		"server_version": "0.1.0",
		"server_time": "2026-08-14T06:00:00Z"
	}`)
}

func TestModuleImplementsInterface(t *testing.T) {
	var _ interfaces.Module = (*Module)(nil)
}

func TestCollectAndUpload(t *testing.T) {
	ft := &fakeTransport{resp: &response.Response{StatusCode: response.StatusOK, Body: successBody()}}
	collector := &stubCollector{accounts: sampleAccounts()}
	mod, err := NewWithDeps(
		Config{Enabled: true, Interval: time.Hour, Timeout: time.Second},
		stubIdentity{id: sampleIdentity(), ok: true},
		collector,
		ft,
		testLogger(),
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := mod.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer mod.Stop(context.Background())

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ft.callCount() >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if ft.callCount() < 1 {
		t.Fatal("expected upload")
	}
	if atomic.LoadInt32(&collector.calls) < 1 {
		t.Fatal("expected collect")
	}
	if got := ft.lastReq.Path; got != EndpointPath {
		t.Fatalf("path = %q", got)
	}
	snap := mod.UsersStatus()
	if snap.AccountsFound != 1 || snap.Cycles < 1 {
		t.Fatalf("snapshot = %+v", snap)
	}
}

func TestConcurrentUploadNoOverlap(t *testing.T) {
	var concurrent int32
	var maxConcurrent int32
	ft := &fakeTransport{resp: &response.Response{StatusCode: response.StatusOK, Body: successBody()}}
	slow := &stubCollector{
		accounts: sampleAccounts(),
	}
	// Wrap transport to sleep while holding "upload"
	orig := ft
	blocking := &blockingTransport{inner: orig, delay: 80 * time.Millisecond, concurrent: &concurrent, max: &maxConcurrent}

	mod, err := NewWithDeps(
		Config{Enabled: true, Interval: 30 * time.Millisecond, Timeout: time.Second},
		stubIdentity{id: sampleIdentity(), ok: true},
		slow,
		blocking,
		testLogger(),
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := mod.Start(ctx); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	cancel()
	_ = mod.Stop(context.Background())
	if atomic.LoadInt32(&maxConcurrent) > 1 {
		t.Fatalf("overlapping cycles: max=%d", maxConcurrent)
	}
}

type blockingTransport struct {
	inner      *fakeTransport
	delay      time.Duration
	concurrent *int32
	max        *int32
}

func (b *blockingTransport) BuildRequest(method request.Method, path string, body any) (request.Request, error) {
	return b.inner.BuildRequest(method, path, body)
}

func (b *blockingTransport) Send(ctx context.Context, req request.Request) (*response.Response, error) {
	cur := atomic.AddInt32(b.concurrent, 1)
	for {
		old := atomic.LoadInt32(b.max)
		if cur <= old || atomic.CompareAndSwapInt32(b.max, old, cur) {
			break
		}
	}
	time.Sleep(b.delay)
	atomic.AddInt32(b.concurrent, -1)
	return b.inner.Send(ctx, req)
}

func (b *blockingTransport) DecodeResponse(resp *response.Response, target any) error {
	return b.inner.DecodeResponse(resp, target)
}

func TestDisabledSkipsNetwork(t *testing.T) {
	ft := &fakeTransport{resp: &response.Response{StatusCode: response.StatusOK, Body: successBody()}}
	mod, err := NewWithDeps(
		Config{Enabled: false, Interval: 50 * time.Millisecond},
		stubIdentity{id: sampleIdentity(), ok: true},
		&stubCollector{accounts: sampleAccounts()},
		ft,
		testLogger(),
	)
	if err != nil {
		t.Fatal(err)
	}
	_ = mod.Start(context.Background())
	time.Sleep(100 * time.Millisecond)
	_ = mod.Stop(context.Background())
	if ft.callCount() != 0 {
		t.Fatalf("calls=%d", ft.callCount())
	}
}
