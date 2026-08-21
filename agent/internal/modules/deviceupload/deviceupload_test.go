package deviceupload

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	idmodel "risklens/agent/internal/identity/model"
	"risklens/agent/internal/modules/device"
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

type stubInventory struct {
	snap device.Snapshot
}

func (s stubInventory) Inventory() device.Snapshot { return s.snap }
func (s stubInventory) Info() device.Info          { return s.snap.Info }
func (s stubInventory) CollectionHealth() interfaces.Health {
	return s.snap.CollectionHealth()
}
func (s stubInventory) Refresh(ctx context.Context) device.Snapshot {
	return s.snap
}

type fakeTransport struct {
	resp     *response.Response
	resps    []*response.Response
	err      error
	errs     []error
	mu       sync.Mutex
	calls    int
	lastReq  request.Request
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
	i := f.calls
	f.calls++
	f.lastReq = req
	err := f.err
	if i < len(f.errs) {
		err = f.errs[i]
	}
	resp := f.resp
	if i < len(f.resps) {
		resp = f.resps[i]
	} else if len(f.resps) > 0 {
		resp = f.resps[len(f.resps)-1]
	}
	f.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return resp, nil
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

func (f *fakeTransport) lastRequest() request.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastReq
}

func waitPosts(t *testing.T, transport *fakeTransport, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if transport.callCount() >= n {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("posts = %d, want >= %d", transport.callCount(), n)
}

func waitUploadHealth(t *testing.T, m *Module, want interfaces.Health) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if m.UploadHealth() == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("upload health = %q, want %q (overall=%q collector=%q)", m.UploadHealth(), want, m.Health(), m.CollectionHealth())
}

func testUploadCfg() Config {
	return Config{
		Enabled:    true,
		BackendURL: "http://backend",
		Timeout:    time.Second,
		Interval:   25 * time.Millisecond,
	}
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

func sampleInfo() device.Info {
	return device.Info{
		MachineName:     "DESKTOP-01",
		Hostname:        "DESKTOP-01",
		OperatingSystem: "Windows 11 Pro",
		OSVersion:       "10.0 (25H2)",
		WindowsBuild:    "26200",
		Architecture:    "x86_64",
		CPUModel:        "Intel Core Ultra 9",
		CPUCores:        24,
		MemoryBytes:     32 * 1024 * 1024 * 1024,
		DiskSizeBytes:   1024 * 1024 * 1024 * 1024,
		DiskFreeBytes:   512 * 1024 * 1024 * 1024,
		Manufacturer:    "ExampleCorp",
		Model:           "ExampleBook",
		SerialNumber:    "",
		BootTime:        time.Date(2026, 8, 11, 6, 0, 0, 0, time.UTC),
		CurrentUser:     `EXAMPLE\alice`,
		AgentVersion:    "0.1.0",
		CollectedAt:     time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC),
	}
}

func successBody(created bool) string {
	b, _ := json.Marshal(SuccessResponse{
		Accepted: true, AgentID: sampleIdentity().AgentID, Created: created,
		InventoryStatus: "stored", ServerVersion: "0.1.0",
		ServerTime: time.Date(2026, 8, 14, 13, 0, 0, 0, time.UTC),
	})
	return string(b)
}

func respond(status int, body string) *fakeTransport {
	return &fakeTransport{resp: &response.Response{StatusCode: status, Body: []byte(body)}}
}

func TestRequestFromInventoryMapsFields(t *testing.T) {
	req := RequestFromInventory("a", "i", sampleInfo())
	if req.AgentID != "a" || req.InstallID != "i" {
		t.Fatalf("ids = %s %s", req.AgentID, req.InstallID)
	}
	if req.MachineName != "DESKTOP-01" || req.CPUCores != 24 {
		t.Fatalf("mapped poorly: %+v", req)
	}
	if req.DiskTotalBytes != sampleInfo().DiskSizeBytes {
		t.Fatal("disk total not mapped")
	}
	if req.BootTime == nil || req.BootTime.IsZero() {
		t.Fatal("boot time missing")
	}
}

func TestUploadSuccess201(t *testing.T) {
	transport := respond(response.StatusCreated, successBody(true))
	client := NewClientWithTransport(transport, testLogger())
	result, err := client.Upload(context.Background(), RequestFromInventory("a", "i", sampleInfo()))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Created || result.HTTPStatusCode != 201 {
		t.Fatalf("result = %+v", result)
	}
	if transport.lastReq.Path != EndpointPath || transport.lastReq.Method != request.MethodPOST {
		t.Fatalf("request = %+v", transport.lastReq)
	}
}

func TestUploadSuccess200Update(t *testing.T) {
	transport := respond(response.StatusOK, successBody(false))
	client := NewClientWithTransport(transport, testLogger())
	result, err := client.Upload(context.Background(), RequestFromInventory("a", "i", sampleInfo()))
	if err != nil {
		t.Fatal(err)
	}
	if result.Created {
		t.Fatal("expected created=false")
	}
}

func TestUploadUnknownAgent(t *testing.T) {
	transport := respond(response.StatusNotFound, `{"error":{"code":"unknown_agent","message":"nope"}}`)
	client := NewClientWithTransport(transport, testLogger())
	_, err := client.Upload(context.Background(), RequestFromInventory("a", "i", sampleInfo()))
	if !errors.Is(err, ErrUnknownAgent) {
		t.Fatalf("got %v", err)
	}
}

func TestUploadValidation(t *testing.T) {
	transport := respond(response.StatusUnprocessableEntity, `{"error":{"code":"validation_error"}}`)
	client := NewClientWithTransport(transport, testLogger())
	if _, err := client.Upload(context.Background(), RequestFromInventory("a", "i", sampleInfo())); !errors.Is(err, ErrValidation) {
		t.Fatalf("got %v", err)
	}
}

func TestModuleStartSuccessfulUpload(t *testing.T) {
	transport := respond(response.StatusCreated, successBody(true))
	m, err := NewWithTransport(
		testUploadCfg(),
		stubIdentity{id: sampleIdentity(), ok: true},
		stubInventory{snap: device.Snapshot{State: device.StateCollected, Info: sampleInfo()}},
		transport,
		testLogger(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Stop(context.Background()) }()

	waitPosts(t, transport, 1)
	if m.Status() != interfaces.StatusRunning {
		t.Fatalf("status = %q", m.Status())
	}
	body, ok := transport.lastRequest().Body.(Request)
	if !ok || body.AgentID != sampleIdentity().AgentID {
		t.Fatalf("body = %#v", transport.lastRequest().Body)
	}
	if transport.lastRequest().Path != EndpointPath || transport.lastRequest().Method != request.MethodPOST {
		t.Fatalf("request = %+v", transport.lastRequest())
	}
	wantJSON, err := json.Marshal(RequestFromInventory(sampleIdentity().AgentID, sampleIdentity().InstallID, sampleInfo()))
	if err != nil {
		t.Fatal(err)
	}
	gotJSON, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	if string(wantJSON) != string(gotJSON) {
		t.Fatalf("JSON mismatch\nwant %s\ngot  %s", wantJSON, gotJSON)
	}
	if _, ok := m.host.Collector().(*device.PipelineCollector); !ok {
		t.Fatalf("collector = %T", m.host.Collector())
	}
	if _, ok := m.host.Uploader().(*Uploader); !ok {
		t.Fatalf("uploader = %T", m.host.Uploader())
	}
}

func TestModuleSkipsWhenInventoryMissing(t *testing.T) {
	transport := respond(response.StatusCreated, successBody(true))
	m, err := NewWithTransport(
		testUploadCfg(),
		stubIdentity{id: sampleIdentity(), ok: true},
		stubInventory{snap: device.Snapshot{State: device.StateFailed}},
		transport,
		testLogger(),
	)
	if err != nil {
		t.Fatal(err)
	}
	_ = m.Start(context.Background())
	defer func() { _ = m.Stop(context.Background()) }()
	time.Sleep(40 * time.Millisecond)
	if m.Status() != interfaces.StatusRunning {
		t.Fatalf("status = %q", m.Status())
	}
	if transport.callCount() != 0 {
		t.Fatal("must not upload without inventory")
	}
}

func TestModuleDisabled(t *testing.T) {
	transport := respond(response.StatusCreated, successBody(true))
	m, err := NewWithTransport(
		Config{Enabled: false},
		stubIdentity{id: sampleIdentity(), ok: true},
		stubInventory{snap: device.Snapshot{State: device.StateCollected, Info: sampleInfo()}},
		transport,
		testLogger(),
	)
	if err != nil {
		t.Fatal(err)
	}
	_ = m.Start(context.Background())
	if m.UploadStatus().State != StateSkipped {
		t.Fatalf("state = %q", m.UploadStatus().State)
	}
	if transport.callCount() != 0 {
		t.Fatal("disabled must not call transport")
	}
}

func TestModuleUnknownAgentRetriesAndStaysRunning(t *testing.T) {
	transport := &fakeTransport{
		resps: []*response.Response{
			{StatusCode: response.StatusNotFound, Body: []byte(`{"error":{"code":"unknown_agent"}}`)},
			{StatusCode: response.StatusCreated, Body: []byte(successBody(true))},
		},
	}
	m, err := NewWithTransport(
		testUploadCfg(),
		stubIdentity{id: sampleIdentity(), ok: true},
		stubInventory{snap: device.Snapshot{State: device.StateCollected, Info: sampleInfo()}},
		transport,
		testLogger(),
	)
	if err != nil {
		t.Fatal(err)
	}
	_ = m.Start(context.Background())
	defer func() { _ = m.Stop(context.Background()) }()
	waitPosts(t, transport, 2)
	if m.Status() != interfaces.StatusRunning {
		t.Fatalf("status = %q", m.Status())
	}
}

func TestModuleDoesNotRecollect(t *testing.T) {
	info := sampleInfo()
	info.MachineName = "FROM-DEVICE-MODULE-ONLY"
	transport := respond(response.StatusCreated, successBody(true))
	m, err := NewWithTransport(
		testUploadCfg(),
		stubIdentity{id: sampleIdentity(), ok: true},
		stubInventory{snap: device.Snapshot{State: device.StateCollected, Info: info}},
		transport,
		testLogger(),
	)
	if err != nil {
		t.Fatal(err)
	}
	_ = m.Start(context.Background())
	defer func() { _ = m.Stop(context.Background()) }()
	waitPosts(t, transport, 1)
	body := transport.lastRequest().Body.(Request)
	if body.MachineName != "FROM-DEVICE-MODULE-ONLY" {
		t.Fatalf("machine_name = %q", body.MachineName)
	}
}

func TestModuleStartImmediateFirstUpload(t *testing.T) {
	probe := &countingProbe{info: sampleInfo()}
	dev, err := device.NewWithCollector(device.Config{Enabled: true, SkipStartCollect: true}, probe, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	transport := respond(response.StatusCreated, successBody(true))
	m, err := NewWithTransport(
		Config{Enabled: true, BackendURL: "http://backend", Timeout: time.Second, Interval: time.Hour},
		stubIdentity{id: sampleIdentity(), ok: true},
		dev,
		transport,
		testLogger(),
	)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Stop(context.Background()) }()
	waitPosts(t, transport, 1)
	if time.Since(start) > 2*time.Second {
		t.Fatal("first upload waited for the interval")
	}
	if probe.calls() != 1 {
		t.Fatalf("collects = %d, want 1", probe.calls())
	}
	if _, ok := m.host.Collector().(*device.PipelineCollector); !ok {
		t.Fatalf("collector = %T", m.host.Collector())
	}
	if _, ok := m.host.Uploader().(*Uploader); !ok {
		t.Fatalf("uploader = %T", m.host.Uploader())
	}
}

func TestModuleStartNoDuplicateUploadWhenUnchanged(t *testing.T) {
	info := sampleInfo()
	info.CollectedAt = time.Time{}
	probe := &countingProbe{info: info}
	dev, err := device.NewWithCollector(device.Config{Enabled: true, SkipStartCollect: true}, probe, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	transport := respond(response.StatusCreated, successBody(true))
	m, err := NewWithTransport(
		testUploadCfg(),
		stubIdentity{id: sampleIdentity(), ok: true},
		dev,
		transport,
		testLogger(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Stop(context.Background()) }()
	waitPosts(t, transport, 1)
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if probe.calls() >= 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if probe.calls() < 2 {
		t.Fatalf("collects = %d, scheduler must tick again", probe.calls())
	}
	if transport.callCount() != 1 {
		t.Fatalf("posts = %d, unchanged inventory must not re-upload", transport.callCount())
	}
}

func TestModuleStartUploadsWhenInventoryChanges(t *testing.T) {
	probe := &countingProbe{info: sampleInfo()}
	dev, err := device.NewWithCollector(device.Config{Enabled: true, SkipStartCollect: true}, probe, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	transport := respond(response.StatusCreated, successBody(true))
	m, err := NewWithTransport(
		testUploadCfg(),
		stubIdentity{id: sampleIdentity(), ok: true},
		dev,
		transport,
		testLogger(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Stop(context.Background()) }()
	waitPosts(t, transport, 1)

	changed := sampleInfo()
	changed.MachineName = "CHANGED-HOST"
	changed.Hostname = "CHANGED-HOST"
	probe.setInfo(changed)
	waitPosts(t, transport, 2)
	body := transport.lastRequest().Body.(Request)
	if body.MachineName != "CHANGED-HOST" {
		t.Fatalf("machine_name = %q", body.MachineName)
	}
}

func TestModuleStartRetriesUploadAfterFailure(t *testing.T) {
	probe := &countingProbe{info: sampleInfo()}
	dev, err := device.NewWithCollector(device.Config{Enabled: true, SkipStartCollect: true}, probe, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	transport := &fakeTransport{
		resps: []*response.Response{
			{StatusCode: response.StatusInternalServerError, Body: []byte(`{"error":{"code":"server_error"}}`)},
			{StatusCode: response.StatusCreated, Body: []byte(successBody(true))},
		},
	}
	m, err := NewWithTransport(
		testUploadCfg(),
		stubIdentity{id: sampleIdentity(), ok: true},
		dev,
		transport,
		testLogger(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Stop(context.Background()) }()
	waitPosts(t, transport, 2)
	waitUploadHealth(t, m, interfaces.HealthHealthy)
	if m.Status() != interfaces.StatusRunning {
		t.Fatalf("status = %q", m.Status())
	}
	if m.host.Pipeline().Pending() {
		t.Fatal("pending should clear after successful retry")
	}
	if dev.CollectionHealth() != interfaces.HealthHealthy {
		t.Fatalf("collector health = %q", dev.CollectionHealth())
	}
	if m.UploadHealth() != interfaces.HealthHealthy {
		t.Fatalf("upload health = %q", m.UploadHealth())
	}
	if m.Health() != interfaces.HealthHealthy {
		t.Fatalf("overall = %q", m.Health())
	}
}

func TestModuleStopStopsScheduler(t *testing.T) {
	probe := &countingProbe{info: sampleInfo()}
	dev, err := device.NewWithCollector(device.Config{Enabled: true, SkipStartCollect: true}, probe, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	transport := respond(response.StatusCreated, successBody(true))
	m, err := NewWithTransport(
		testUploadCfg(),
		stubIdentity{id: sampleIdentity(), ok: true},
		dev,
		transport,
		testLogger(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitPosts(t, transport, 1)
	if err := m.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	collects := probe.calls()
	posts := transport.callCount()
	time.Sleep(80 * time.Millisecond)
	if probe.calls() != collects {
		t.Fatalf("collects after stop = %d, was %d", probe.calls(), collects)
	}
	if transport.callCount() != posts {
		t.Fatalf("posts after stop = %d, was %d", transport.callCount(), posts)
	}
}

func TestModuleContextCancelStopsScheduler(t *testing.T) {
	probe := &countingProbe{info: sampleInfo()}
	dev, err := device.NewWithCollector(device.Config{Enabled: true, SkipStartCollect: true}, probe, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	transport := respond(response.StatusCreated, successBody(true))
	m, err := NewWithTransport(
		testUploadCfg(),
		stubIdentity{id: sampleIdentity(), ok: true},
		dev,
		transport,
		testLogger(),
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	waitPosts(t, transport, 1)
	cancel()
	deadline := time.Now().Add(500 * time.Millisecond)
	var collects int
	for time.Now().Before(deadline) {
		collects = probe.calls()
		time.Sleep(30 * time.Millisecond)
		if probe.calls() == collects {
			break
		}
	}
	time.Sleep(80 * time.Millisecond)
	if probe.calls() != collects {
		t.Fatalf("collects after cancel = %d, was %d", probe.calls(), collects)
	}
	_ = m.Stop(context.Background())
}

func TestModuleNoOverlappingExecutions(t *testing.T) {
	probe := &countingProbe{info: sampleInfo(), hold: 80 * time.Millisecond}
	dev, err := device.NewWithCollector(device.Config{Enabled: true, SkipStartCollect: true}, probe, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	transport := respond(response.StatusCreated, successBody(true))
	m, err := NewWithTransport(
		testUploadCfg(),
		stubIdentity{id: sampleIdentity(), ok: true},
		dev,
		transport,
		testLogger(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(120 * time.Millisecond)
	_ = m.Stop(context.Background())
	if probe.max() > 1 {
		t.Fatalf("overlapping collects: max inflight = %d", probe.max())
	}
}

func TestModuleUploadFailureKeepsCollectorHealthy(t *testing.T) {
	probe := &countingProbe{info: sampleInfo()}
	dev, err := device.NewWithCollector(device.Config{Enabled: true, SkipStartCollect: true}, probe, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	transport := respond(response.StatusInternalServerError, `{"error":{"code":"server_error"}}`)
	m, err := NewWithTransport(
		testUploadCfg(),
		stubIdentity{id: sampleIdentity(), ok: true},
		dev,
		transport,
		testLogger(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Stop(context.Background()) }()
	waitPosts(t, transport, 1)
	waitUploadHealth(t, m, interfaces.HealthDegraded)

	if dev.CollectionHealth() != interfaces.HealthHealthy {
		t.Fatalf("collector = %q, HTTP fail must not fail collection", dev.CollectionHealth())
	}
	if dev.Health() != interfaces.HealthHealthy {
		t.Fatalf("device module health = %q", dev.Health())
	}
	if m.UploadHealth() != interfaces.HealthDegraded {
		t.Fatalf("upload = %q, want warning/degraded", m.UploadHealth())
	}
	if m.Health() != interfaces.HealthDegraded {
		t.Fatalf("overall device = %q, want warning/degraded", m.Health())
	}
	if m.Status() != interfaces.StatusRunning {
		t.Fatalf("status = %q, must not MarkFailed on HTTP error", m.Status())
	}
}
