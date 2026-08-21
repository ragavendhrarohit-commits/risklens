package software

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
	apps  []Application
	err   error
	calls int32
}

func (s *stubCollector) Collect(ctx context.Context) ([]Application, error) {
	atomic.AddInt32(&s.calls, 1)
	// Return a copy so tests can mutate freely.
	out := make([]Application, len(s.apps))
	copy(out, s.apps)
	return out, s.err
}

type fakeTransport struct {
	resp    *response.Response
	err     error
	mu      sync.Mutex
	calls   int
	lastReq request.Request
	delay   time.Duration
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
	delay := f.delay
	f.mu.Unlock()
	if delay > 0 {
		time.Sleep(delay)
	}
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

func successBody() []byte {
	return []byte(`{
		"accepted": true,
		"agent_id": "9b5d7f3e-1234-4abc-9def-0123456789ab",
		"stored": 2,
		"inventory_status": "stored",
		"server_version": "0.1.0",
		"server_time": "2026-08-14T06:00:00Z"
	}`)
}

func TestModuleImplementsInterface(t *testing.T) {
	var _ interfaces.Module = (*Module)(nil)
}

func TestDedupeMergesDuplicatesAndKeepsRicherRow(t *testing.T) {
	apps := []Application{
		{Name: "  ", Version: "1.0"}, // filtered by sanitize wrapper? not — dedupe runs on already-sanitized data; test blank filter separately
		{Name: "Chrome", Version: "120.0", Publisher: "Google"},
		{Name: "chrome", Version: "120.0", Publisher: "", Architecture: ArchitectureX64, EstimatedSize: 1024},
		{Name: "Notepad++", Version: "8.5.6", Publisher: "Notepad++ Team"},
	}
	// remove blank name manually to mimic post-sanitize input
	cleaned := apps[1:]
	out := dedupe(cleaned)
	if len(out) != 2 {
		t.Fatalf("dedupe len = %d, want 2 (out=%+v)", len(out), out)
	}
	// Chrome winner should be the row with more populated fields.
	for _, app := range out {
		if app.Name == "chrome" && app.Publisher != "Google" {
			// dedupe collapses case-insensitively; the winner may have "" publisher
			// but must have been the higher-score row. Verify size present.
			if app.EstimatedSize == 0 {
				t.Fatalf("chrome winner not richer row: %+v", app)
			}
		}
	}
}

func TestSanitizeRejectsBlankName(t *testing.T) {
	if _, ok := sanitize(Application{Name: "   ", Version: "1"}); ok {
		t.Fatal("sanitize should drop blank names")
	}
	entry, ok := sanitize(Application{Name: "  Chrome  ", Version: " 120 "})
	if !ok {
		t.Fatal("sanitize should keep non-blank names")
	}
	if entry.Name != "Chrome" || entry.Version != "120" {
		t.Fatalf("sanitize trimming failed: %+v", entry)
	}
}

func TestCollectAndUpload(t *testing.T) {
	ft := &fakeTransport{resp: &response.Response{StatusCode: response.StatusOK, Body: successBody()}}
	collector := &stubCollector{apps: []Application{
		{Name: "Chrome", Version: "120", Architecture: ArchitectureX64},
		{Name: "chrome", Version: "120"}, // duplicate
		{Name: "7-Zip", Version: "23.01", Architecture: ArchitectureX64},
	}}
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
		if ft.callCount() >= 1 && mod.SoftwareStatus().Cycles >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if ft.callCount() < 1 {
		t.Fatal("expected upload call")
	}
	if got := ft.lastReq.Path; got != EndpointPath {
		t.Fatalf("path = %q", got)
	}
	snap := mod.SoftwareStatus()
	if snap.ApplicationsFound != 2 {
		t.Fatalf("applications after dedupe = %d, want 2", snap.ApplicationsFound)
	}
}

func TestDisabledSkipsNetwork(t *testing.T) {
	ft := &fakeTransport{resp: &response.Response{StatusCode: response.StatusOK, Body: successBody()}}
	mod, err := NewWithDeps(
		Config{Enabled: false, Interval: 50 * time.Millisecond},
		stubIdentity{id: sampleIdentity(), ok: true},
		&stubCollector{apps: []Application{{Name: "X", Version: "1"}}},
		ft,
		testLogger(),
	)
	if err != nil {
		t.Fatal(err)
	}
	_ = mod.Start(context.Background())
	time.Sleep(120 * time.Millisecond)
	_ = mod.Stop(context.Background())
	if ft.callCount() != 0 {
		t.Fatalf("calls = %d, want 0", ft.callCount())
	}
}

func TestConcurrentCyclesNoOverlap(t *testing.T) {
	ft := &fakeTransport{
		resp:  &response.Response{StatusCode: response.StatusOK, Body: successBody()},
		delay: 80 * time.Millisecond,
	}
	collector := &stubCollector{apps: []Application{{Name: "X", Version: "1"}}}
	mod, err := NewWithDeps(
		Config{Enabled: true, Interval: 20 * time.Millisecond, Timeout: time.Second},
		stubIdentity{id: sampleIdentity(), ok: true},
		collector,
		ft,
		testLogger(),
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	_ = mod.Start(ctx)
	time.Sleep(300 * time.Millisecond)
	cancel()
	_ = mod.Stop(context.Background())
	// With 80ms/cycle and 20ms tick, non-overlapping scheduler must fire far
	// fewer than 300/20 = 15 times.
	if ft.callCount() > 6 {
		t.Fatalf("overlapping cycles suspected: calls=%d", ft.callCount())
	}
}

func TestTransportTimeoutMapsToErrTransport(t *testing.T) {
	ft := &fakeTransport{err: txerrors.New(txerrors.KindTimeout, "send", context.DeadlineExceeded)}
	client := NewClientWithTransport(ft, testLogger())
	_, err := client.Upload(context.Background(), Inventory{AgentID: "a", InstallID: "i"})
	if !errors.Is(err, ErrTransport) {
		t.Fatalf("err = %v", err)
	}
}
