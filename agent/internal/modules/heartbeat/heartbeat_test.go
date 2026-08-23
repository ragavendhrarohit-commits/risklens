package heartbeat

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
	"risklens/agent/internal/modules/health"
	"risklens/agent/internal/modules/interfaces"
	"risklens/agent/internal/modules/network"
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

type fakeTransport struct {
	resp     *response.Response
	err      error
	mu       sync.Mutex
	calls    int
	lastReq  request.Request
	delay    time.Duration
	buildErr error
}

func (f *fakeTransport) BuildRequest(method request.Method, path string, body any) (request.Request, error) {
	if f.buildErr != nil {
		return request.Request{}, f.buildErr
	}
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
		select {
		case <-ctx.Done():
			return nil, txerrors.New(txerrors.KindTimeout, "send", ctx.Err())
		case <-time.After(delay):
		}
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

func (f *fakeTransport) lastPath() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastReq.Path
}

var _ txinterfaces.Transport = (*fakeTransport)(nil)

func sampleIdentity() idmodel.Identity {
	return idmodel.Identity{
		AgentID:      "9b5d7f3e-1234-4abc-9def-0123456789ab",
		InstallID:    "install-1",
		AgentVersion: "0.1.0",
		MachineName:  "TEST-HOST",
	}
}

func successBody() []byte {
	return []byte(`{
		"accepted": true,
		"agent_id": "9b5d7f3e-1234-4abc-9def-0123456789ab",
		"heartbeat_status": "ok",
		"server_version": "0.1.0",
		"server_time": "2026-08-14T04:00:00Z",
		"received_at": "2026-08-14T04:00:00.100Z"
	}`)
}

func TestModuleImplementsInterface(t *testing.T) {
	var _ interfaces.Module = (*Module)(nil)
}

func TestHeartbeatSuccess(t *testing.T) {
	ft := &fakeTransport{resp: &response.Response{StatusCode: response.StatusOK, Body: successBody()}}
	mod, err := NewWithTransport(
		Config{Enabled: true, Interval: time.Hour, Timeout: time.Second},
		stubIdentity{id: sampleIdentity(), ok: true},
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
		if ft.callCount() >= 1 && mod.HeartbeatStatus().BeatsSent >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if ft.callCount() < 1 {
		t.Fatal("expected heartbeat transport call")
	}
	if got := ft.lastPath(); got != "/api/v1/agents/9b5d7f3e-1234-4abc-9def-0123456789ab/heartbeat" {
		t.Fatalf("path = %q", got)
	}
	snap := mod.HeartbeatStatus()
	if snap.State != StateRunning || snap.BeatsSent < 1 {
		t.Fatalf("snapshot = %+v", snap)
	}
}

func TestHeartbeatDisabledSkipsNetwork(t *testing.T) {
	ft := &fakeTransport{resp: &response.Response{StatusCode: response.StatusOK, Body: successBody()}}
	mod, err := NewWithTransport(
		Config{Enabled: false, Interval: 50 * time.Millisecond},
		stubIdentity{id: sampleIdentity(), ok: true},
		ft,
		testLogger(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := mod.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(120 * time.Millisecond)
	_ = mod.Stop(context.Background())
	if ft.callCount() != 0 {
		t.Fatalf("calls = %d", ft.callCount())
	}
	if mod.HeartbeatStatus().State != StateSkipped {
		t.Fatalf("state = %s", mod.HeartbeatStatus().State)
	}
}

func TestTransportTimeout(t *testing.T) {
	ft := &fakeTransport{
		err: txerrors.New(txerrors.KindTimeout, "send", context.DeadlineExceeded),
	}
	client := NewClientWithTransport(ft, testLogger())
	_, err := client.Send(context.Background(), RequestFromIdentity(sampleIdentity(), time.Now().UTC(), network.EndpointNetwork{}, health.Snapshot{}))
	if !errors.Is(err, ErrTransport) {
		t.Fatalf("err = %v", err)
	}
}

func TestSchedulerNoOverlap(t *testing.T) {
	var concurrent int32
	var maxConcurrent int32
	var calls int32

	sched := NewScheduler(30*time.Millisecond, func(ctx context.Context) {
		atomic.AddInt32(&calls, 1)
		cur := atomic.AddInt32(&concurrent, 1)
		for {
			old := atomic.LoadInt32(&maxConcurrent)
			if cur <= old || atomic.CompareAndSwapInt32(&maxConcurrent, old, cur) {
				break
			}
		}
		time.Sleep(80 * time.Millisecond)
		atomic.AddInt32(&concurrent, -1)
	})

	ctx, cancel := context.WithCancel(context.Background())
	sched.Start(ctx)
	time.Sleep(200 * time.Millisecond)
	cancel()
	sched.Stop()

	if atomic.LoadInt32(&maxConcurrent) > 1 {
		t.Fatalf("overlapping beats detected: max=%d", maxConcurrent)
	}
	if atomic.LoadInt32(&calls) < 1 {
		t.Fatal("expected at least one beat")
	}
}

func TestSchedulerGracefulStop(t *testing.T) {
	started := make(chan struct{}, 1)
	sched := NewScheduler(time.Hour, func(ctx context.Context) {
		select {
		case started <- struct{}{}:
		default:
		}
		<-ctx.Done()
	})
	ctx, cancel := context.WithCancel(context.Background())
	sched.Start(ctx)
	<-started
	cancel()
	done := make(chan struct{})
	go func() {
		sched.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("scheduler stop hung")
	}
}

func TestRequestFromIdentityDoesNotInventIDs(t *testing.T) {
	id := sampleIdentity()
	req := RequestFromIdentity(id, time.Date(2026, 8, 14, 4, 0, 0, 0, time.UTC), network.EndpointNetwork{}, health.Snapshot{})
	if req.AgentID != id.AgentID || req.InstallID != id.InstallID {
		t.Fatalf("ids mutated: %+v", req)
	}
	if req.Timestamp.Location() != time.UTC {
		t.Fatalf("timestamp not UTC: %v", req.Timestamp.Location())
	}
}

func TestHeartbeatRequestCannotCarrySessions(t *testing.T) {
	req := RequestFromIdentity(sampleIdentity(), time.Date(2026, 8, 16, 12, 35, 34, 0, time.UTC), network.EndpointNetwork{}, health.Snapshot{})
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"sessions", "session_id", "logout_time", "logoff_at"} {
		if _, ok := payload[key]; ok {
			t.Fatalf("heartbeat payload included %s: %s", key, data)
		}
	}
	if len(payload) != 3 {
		t.Fatalf("heartbeat payload = %s", data)
	}
}

func TestHeartbeatRequestIncludesHealthWhenPresent(t *testing.T) {
	snap := health.Snapshot{
		CPUPercent:    12.5,
		CPUModel:      "Test CPU",
		LogicalCPUs:   8,
		PhysicalCores: 4,
		Memory: health.MemoryStats{
			Total: 1024, Used: 512, Free: 512, Percent: 50,
		},
		Disks: []health.DiskStats{{
			Name: "C:", Total: 2048, Used: 1024, Free: 1024, Percent: 50,
		}},
	}
	req := RequestFromIdentity(sampleIdentity(), time.Date(2026, 8, 16, 12, 35, 34, 0, time.UTC), network.EndpointNetwork{}, snap)
	if req.Health == nil {
		t.Fatal("expected health snapshot")
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	healthObj, ok := payload["health"].(map[string]any)
	if !ok {
		t.Fatalf("missing health object: %s", data)
	}
	if healthObj["cpu_percent"] != 12.5 {
		t.Fatalf("cpu_percent = %v", healthObj["cpu_percent"])
	}
}
