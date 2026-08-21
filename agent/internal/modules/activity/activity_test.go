package activity

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
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
	obs   Observation
	err   error
	calls int32
}

func (s *stubCollector) Collect(ctx context.Context) (Observation, error) {
	atomic.AddInt32(&s.calls, 1)
	return s.obs, s.err
}

type fakeTransport struct {
	resp    *response.Response
	err     error
	mu      sync.Mutex
	calls   int
	paths   []string
	bodies  []any
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
	f.paths = append(f.paths, req.Path)
	f.bodies = append(f.bodies, req.Body)
	f.lastReq = req
	err := f.err
	f.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return f.resp, nil
}

func (f *fakeTransport) setErr(err error) {
	f.mu.Lock()
	f.err = err
	f.mu.Unlock()
}

func (f *fakeTransport) sessionUploads() []SessionUpload {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []SessionUpload
	for _, body := range f.bodies {
		if upload, ok := body.(SessionUpload); ok {
			out = append(out, upload)
		}
	}
	return out
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

func (f *fakeTransport) recordedPaths() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.paths))
	copy(out, f.paths)
	return out
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
		"stored": 1,
		"inventory_status": "stored",
		"server_version": "0.1.0",
		"server_time": "2026-08-15T06:00:00Z"
	}`)
}

func TestModuleImplementsInterface(t *testing.T) {
	var _ interfaces.Module = (*Module)(nil)
}

func TestStepOpensAndClosesSession(t *testing.T) {
	agentID := sampleIdentity().AgentID
	logon := time.Date(2026, 8, 15, 4, 0, 0, 0, time.UTC)
	now := logon.Add(2 * time.Minute)
	live := &LiveSession{WTSSessionID: 1, Username: "rohit", AccountSID: "S-1-5-21-1", LogonAt: logon}

	opened := step(persistedState{}, Observation{Session: live}, agentID, now)
	if !opened.Changed || len(opened.Sessions) != 1 {
		t.Fatalf("open = %+v", opened)
	}
	if opened.Sessions[0].Status != StatusActive || opened.Sessions[0].LogoffAt != nil {
		t.Fatalf("open session = %+v", opened.Sessions[0])
	}
	if opened.Next.SessionID == "" {
		t.Fatal("expected session id")
	}

	closed := step(opened.Next, Observation{}, agentID, now.Add(time.Minute))
	if !closed.Changed || len(closed.Sessions) != 1 {
		t.Fatalf("close = %+v", closed)
	}
	if closed.Sessions[0].Status != StatusClosed || closed.Sessions[0].LogoffAt == nil {
		t.Fatalf("close session = %+v", closed.Sessions[0])
	}
	if closed.Next.hasSession() {
		t.Fatalf("expected empty next state, got %+v", closed.Next)
	}
}

func TestStepSwitchesApplicationOnSameSession(t *testing.T) {
	agentID := sampleIdentity().AgentID
	logon := time.Date(2026, 8, 15, 4, 0, 0, 0, time.UTC)
	now := logon.Add(time.Minute)
	live := &LiveSession{WTSSessionID: 2, Username: "rohit", AccountSID: "S-1-5-21-1", LogonAt: logon}
	cursor := &LiveApp{ApplicationName: "Cursor", ExecutableName: "Cursor.exe", WindowTitle: "main.go"}

	first := step(persistedState{}, Observation{Session: live, App: cursor}, agentID, now)
	if len(first.Apps) != 1 || first.Apps[0].StoppedAt != nil {
		t.Fatalf("first app = %+v", first.Apps)
	}

	chrome := &LiveApp{ApplicationName: "Chrome", ExecutableName: "chrome.exe", WindowTitle: "Inbox"}
	switched := step(first.Next, Observation{Session: live, App: chrome}, agentID, now.Add(30*time.Second))
	if !switched.Changed || len(switched.Apps) != 2 {
		t.Fatalf("switch = %+v", switched)
	}
	if switched.Apps[0].StoppedAt == nil || switched.Apps[0].ExecutableName != "Cursor.exe" {
		t.Fatalf("expected Cursor stopped, got %+v", switched.Apps[0])
	}
	if switched.Apps[1].StoppedAt != nil || switched.Apps[1].ExecutableName != "chrome.exe" {
		t.Fatalf("expected Chrome started, got %+v", switched.Apps[1])
	}
	if switched.Apps[0].SessionID != switched.Apps[1].SessionID {
		t.Fatal("app switch must stay on the same session")
	}
}

func TestStepSameSessionDoesNotMintNewID(t *testing.T) {
	agentID := sampleIdentity().AgentID
	logon := time.Date(2026, 8, 15, 4, 0, 0, 0, time.UTC)
	live := &LiveSession{WTSSessionID: 3, Username: "rohit", LogonAt: logon}
	first := step(persistedState{}, Observation{Session: live}, agentID, logon.Add(time.Second))
	second := step(first.Next, Observation{Session: live}, agentID, logon.Add(20*time.Second))
	if first.Next.SessionID != second.Next.SessionID {
		t.Fatalf("session id changed: %s vs %s", first.Next.SessionID, second.Next.SessionID)
	}
	if second.Changed {
		t.Fatal("continuing the same session should not be a structural change")
	}
}

func TestStepNewLogonClosesPreviousSession(t *testing.T) {
	agentID := sampleIdentity().AgentID
	firstLogon := time.Date(2026, 8, 16, 8, 14, 25, 0, time.UTC)
	secondLogon := time.Date(2026, 8, 16, 12, 18, 27, 0, time.UTC)
	first := step(
		persistedState{},
		Observation{Session: &LiveSession{WTSSessionID: 1, Username: "ROHIT", LogonAt: firstLogon}},
		agentID,
		firstLogon.Add(time.Minute),
	)
	switched := step(
		first.Next,
		Observation{Session: &LiveSession{WTSSessionID: 2, Username: "ROHIT", LogonAt: secondLogon}},
		agentID,
		secondLogon,
	)
	if len(switched.Sessions) != 2 {
		t.Fatalf("expected close+open, got %+v", switched.Sessions)
	}
	if switched.Sessions[0].Status != StatusClosed || switched.Sessions[0].LogoffAt == nil {
		t.Fatalf("previous session not closed: %+v", switched.Sessions[0])
	}
	if switched.Sessions[1].Status != StatusActive || switched.Sessions[1].LogoffAt != nil {
		t.Fatalf("new session not active: %+v", switched.Sessions[1])
	}
	if switched.Sessions[0].SessionID == switched.Sessions[1].SessionID {
		t.Fatal("consecutive logins must not share a session id")
	}
}

func TestStepNewLogonClosesPreviousAtNewLogonTime(t *testing.T) {
	agentID := sampleIdentity().AgentID
	firstLogon := time.Date(2026, 8, 16, 12, 18, 27, 0, time.UTC)
	secondLogon := time.Date(2026, 8, 16, 13, 14, 0, 0, time.UTC)
	shutdown := time.Date(2026, 8, 16, 13, 21, 0, 0, time.UTC)
	first := step(
		persistedState{},
		Observation{Session: &LiveSession{WTSSessionID: 1, Username: "ROHIT", LogonAt: firstLogon}},
		agentID,
		firstLogon.Add(time.Minute),
	)
	switched := step(
		first.Next,
		Observation{Session: &LiveSession{WTSSessionID: 2, Username: "ROHIT", LogonAt: secondLogon}},
		agentID,
		shutdown,
	)
	if switched.Sessions[0].Status != StatusClosed || switched.Sessions[0].LogoffAt == nil {
		t.Fatalf("previous session not closed: %+v", switched.Sessions[0])
	}
	if !switched.Sessions[0].LogoffAt.Equal(secondLogon) {
		t.Fatalf("previous logout = %v, want new login %v", switched.Sessions[0].LogoffAt, secondLogon)
	}
}

func TestCollectAndUploadPostsSessionsThenApplications(t *testing.T) {
	dir := t.TempDir()
	ft := &fakeTransport{resp: &response.Response{StatusCode: response.StatusOK, Body: successBody()}}
	logon := time.Date(2026, 8, 15, 4, 0, 0, 0, time.UTC)
	collector := &stubCollector{
		obs: Observation{
			Session: &LiveSession{WTSSessionID: 1, Username: "rohit", AccountSID: "S-1-5-21-1", LogonAt: logon},
			App:     &LiveApp{ApplicationName: "Cursor", ExecutableName: "Cursor.exe", WindowTitle: "plan.md"},
		},
	}
	mod, err := NewWithDeps(
		Config{Enabled: true, Interval: time.Hour, Timeout: time.Second, StateDir: dir},
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
		if ft.callCount() >= 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	paths := ft.recordedPaths()
	if len(paths) < 2 {
		t.Fatalf("expected dual upload, paths=%v", paths)
	}
	if paths[0] != SessionsPath || paths[1] != ApplicationsPath {
		t.Fatalf("upload order = %v", paths)
	}
	if _, err := os.Stat(filepath.Join(dir, stateFileName)); err != nil {
		t.Fatalf("state file: %v", err)
	}
	snap := mod.ActivityStatus()
	if snap.SessionsFound != 1 || snap.ActivitiesFound != 1 {
		t.Fatalf("snapshot = %+v", snap)
	}
}

func TestFlushCloseUploadsLogout(t *testing.T) {
	dir := t.TempDir()
	ft := &fakeTransport{resp: &response.Response{StatusCode: response.StatusOK, Body: successBody()}}
	logon := time.Date(2026, 8, 15, 4, 0, 0, 0, time.UTC)
	collector := &stubCollector{
		obs: Observation{
			Session: &LiveSession{WTSSessionID: 1, Username: "rohit", AccountSID: "S-1-5-21-1", LogonAt: logon},
		},
	}
	mod, err := NewWithDeps(
		Config{Enabled: true, Interval: time.Hour, Timeout: time.Second, StateDir: dir},
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
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ft.callCount() >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := mod.FlushClose(context.Background()); err != nil {
		t.Fatal(err)
	}
	_ = mod.Stop(context.Background())

	var closed SessionUpload
	ft.mu.Lock()
	for i := len(ft.paths) - 1; i >= 0; i-- {
		if ft.paths[i] == SessionsPath {
			if body, ok := ft.lastReq.Body.(SessionUpload); ok {
				closed = body
			}
			break
		}
	}
	ft.mu.Unlock()
	if len(closed.Sessions) != 1 {
		// lastReq may have been overwritten by an app upload; inspect recorded paths
		if ft.callCount() < 2 {
			t.Fatalf("expected close upload, calls=%d paths=%v", ft.callCount(), ft.recordedPaths())
		}
	}
	snap := mod.ActivityStatus()
	if snap.SessionsFound < 1 {
		t.Fatalf("expected closed session upload, snapshot=%+v paths=%v", snap, ft.recordedPaths())
	}
}

func TestDisabledSkipsNetwork(t *testing.T) {
	ft := &fakeTransport{resp: &response.Response{StatusCode: response.StatusOK, Body: successBody()}}
	mod, err := NewWithDeps(
		Config{Enabled: false, Interval: 50 * time.Millisecond, StateDir: t.TempDir()},
		stubIdentity{id: sampleIdentity(), ok: true},
		&stubCollector{obs: Observation{Session: &LiveSession{WTSSessionID: 1, Username: "rohit", LogonAt: time.Now()}}},
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

func TestStepDoesNotFabricateLogonFromNow(t *testing.T) {
	agentID := sampleIdentity().AgentID
	now := time.Date(2026, 8, 16, 13, 14, 0, 0, time.UTC)
	live := &LiveSession{WTSSessionID: 1, Username: "ROHIT", AccountSID: "S-1-5-21-1"}
	opened := step(persistedState{}, Observation{Session: live}, agentID, now)
	if opened.Changed || len(opened.Sessions) != 0 || opened.Next.hasSession() {
		t.Fatalf("zero LogonTime must not mint a session at now, got %+v", opened)
	}
}

func TestStepLogoutThenLaterLoginDoesNotFabricateLogin(t *testing.T) {
	agentID := sampleIdentity().AgentID
	firstLogon := time.Date(2026, 8, 16, 12, 18, 27, 0, time.UTC)
	logoutAt := time.Date(2026, 8, 16, 13, 14, 0, 0, time.UTC)
	secondLogon := time.Date(2026, 8, 16, 15, 20, 0, 0, time.UTC)
	logout2 := time.Date(2026, 8, 16, 15, 45, 0, 0, time.UTC)

	opened := step(
		persistedState{},
		Observation{Session: &LiveSession{WTSSessionID: 1, Username: "ROHIT", LogonAt: firstLogon}},
		agentID,
		firstLogon.Add(time.Minute),
	)
	closed := step(opened.Next, Observation{}, agentID, logoutAt)
	if len(closed.Sessions) != 1 || closed.Sessions[0].Status != StatusClosed || closed.Next.hasSession() {
		t.Fatalf("logout = %+v", closed)
	}
	if !closed.Sessions[0].LogoffAt.Equal(logoutAt) || !closed.Sessions[0].LogonAt.Equal(firstLogon) {
		t.Fatalf("session 1 times = %+v", closed.Sessions[0])
	}

	stale := step(closed.Next, Observation{Session: &LiveSession{
		WTSSessionID: 1, Username: "ROHIT",
	}}, agentID, logoutAt)
	if stale.Changed || len(stale.Sessions) != 0 || stale.Next.hasSession() {
		t.Fatalf("post-logout collect with empty LogonTime fabricated a session: %+v", stale)
	}

	lingering := step(closed.Next, Observation{Session: &LiveSession{
		WTSSessionID: 1, Username: "ROHIT", LogonAt: firstLogon,
	}}, agentID, logoutAt)
	if lingering.Changed || lingering.Next.hasSession() {
		t.Fatalf("closed session was reopened: %+v", lingering)
	}

	second := step(closed.Next, Observation{Session: &LiveSession{
		WTSSessionID: 2, Username: "ROHIT", LogonAt: secondLogon,
	}}, agentID, secondLogon)
	if len(second.Sessions) != 1 || second.Sessions[0].Status != StatusActive {
		t.Fatalf("expected only the new open, got %+v", second.Sessions)
	}
	if !second.Sessions[0].LogonAt.Equal(secondLogon) {
		t.Fatalf("session 2 login = %v, want real login %v", second.Sessions[0].LogonAt, secondLogon)
	}
	if second.Sessions[0].LogonAt.Equal(logoutAt) {
		t.Fatal("fabricated a login at the previous logout time")
	}

	secondClosed := step(second.Next, Observation{}, agentID, logout2)
	if len(secondClosed.Sessions) != 1 || !secondClosed.Sessions[0].LogoffAt.Equal(logout2) {
		t.Fatalf("session 2 logout = %+v", secondClosed.Sessions)
	}
	if !secondClosed.Sessions[0].LogonAt.Equal(secondLogon) {
		t.Fatalf("session 2 login changed to %v", secondClosed.Sessions[0].LogonAt)
	}
}

func TestStepPowerFailureClosesPreviousAtNextLogin(t *testing.T) {
	agentID := sampleIdentity().AgentID
	firstLogon := time.Date(2026, 8, 16, 12, 18, 27, 0, time.UTC)
	secondLogon := time.Date(2026, 8, 16, 15, 20, 0, 0, time.UTC)
	first := step(
		persistedState{},
		Observation{Session: &LiveSession{WTSSessionID: 1, Username: "ROHIT", LogonAt: firstLogon}},
		agentID,
		firstLogon.Add(time.Minute),
	)
	switched := step(
		first.Next,
		Observation{Session: &LiveSession{WTSSessionID: 2, Username: "ROHIT", LogonAt: secondLogon}},
		agentID,
		secondLogon,
	)
	if len(switched.Sessions) != 2 {
		t.Fatalf("expected close+open, got %+v", switched.Sessions)
	}
	if !switched.Sessions[0].LogonAt.Equal(firstLogon) || !switched.Sessions[0].LogoffAt.Equal(secondLogon) {
		t.Fatalf("missed-logout fallback = %+v", switched.Sessions[0])
	}
	if !switched.Sessions[1].LogonAt.Equal(secondLogon) || switched.Sessions[1].Status != StatusActive {
		t.Fatalf("new session = %+v", switched.Sessions[1])
	}
}

func TestSanitizeStateStripsLocallyClosedOpenSession(t *testing.T) {
	logon := time.Date(2026, 8, 16, 12, 22, 56, 0, time.UTC)
	state := persistedState{
		SessionID:           "session-a",
		WTSSessionID:        1,
		Username:            "ROHIT",
		LogonAt:             logon,
		LastClosedSessionID: "session-a",
		LastClosedLogonAt:   logon,
		UploadedClosedIDs:   []string{"session-a"},
	}
	cleaned := sanitizeState(state)
	if cleaned.hasSession() {
		t.Fatalf("activity.json must not keep a closed session: %+v", cleaned)
	}
	if cleaned.LastClosedSessionID != "session-a" {
		t.Fatalf("last-closed was lost: %+v", cleaned)
	}
	if !cleaned.alreadyUploadedClosed("session-a") {
		t.Fatalf("uploaded-closed set lost: %+v", cleaned)
	}
}

func TestFilterUnsentSessionsDropsUploadedAndActiveLocallyClosed(t *testing.T) {
	logoff := time.Date(2026, 8, 16, 12, 25, 25, 0, time.UTC)
	state := persistedState{
		LastClosedSessionID: "session-a",
		UploadedClosedIDs:   []string{"session-b"},
	}
	items := []SessionItem{
		{SessionID: "session-b", Status: StatusClosed, LogoffAt: &logoff},
		{SessionID: "session-a", Status: StatusActive},
		{SessionID: "session-a", Status: StatusClosed, LogoffAt: &logoff},
		{SessionID: "session-c", Status: StatusActive},
	}
	out := filterUnsentSessions(state, items)
	if len(out) != 2 {
		t.Fatalf("filtered = %+v", out)
	}
	if out[0].SessionID != "session-a" || out[0].Status != StatusClosed {
		t.Fatalf("unsent close of A must be kept until ack: %+v", out[0])
	}
	if out[1].SessionID != "session-c" || out[1].Status != StatusActive {
		t.Fatalf("unrelated open session dropped: %+v", out[1])
	}
}

func TestStepDoesNotRecloseLocallyClosedSession(t *testing.T) {
	agentID := sampleIdentity().AgentID
	logon := time.Date(2026, 8, 16, 12, 22, 56, 0, time.UTC)
	now := time.Date(2026, 8, 16, 12, 35, 34, 0, time.UTC)
	sid := sessionID(agentID, 1, logon)
	closed := rememberClosed(persistedState{
		SessionID:    sid,
		WTSSessionID: 1,
		Username:     "ROHIT",
		AccountSID:   "S-1-5-21-1",
		LogonAt:      logon,
	}, sid, logon)
	closed.UploadedClosedIDs = []string{sid}

	empty := step(closed, Observation{}, agentID, now)
	if len(empty.Sessions) != 0 {
		t.Fatalf("closed session was serialized again: %+v", empty.Sessions)
	}

	leftover := closed
	leftover.SessionID = sid
	leftover.WTSSessionID = 1
	leftover.Username = "ROHIT"
	leftover.AccountSID = "S-1-5-21-1"
	leftover.LogonAt = logon
	stripped := step(leftover, Observation{}, agentID, now)
	if len(stripped.Sessions) != 0 {
		t.Fatalf("leftover OPEN matching last-closed was re-closed at %v: %+v", now, stripped.Sessions)
	}
	if stripped.Next.hasSession() {
		t.Fatalf("leftover OPEN was not stripped: %+v", stripped.Next)
	}

	live := &LiveSession{WTSSessionID: 1, Username: "ROHIT", AccountSID: "S-1-5-21-1", LogonAt: logon}
	same := step(closed, Observation{Session: live}, agentID, now)
	if len(same.Sessions) != 0 || same.Next.hasSession() {
		t.Fatalf("heartbeat/login reopened closed session A: %+v", same)
	}

	nextLogon := time.Date(2026, 8, 16, 12, 34, 19, 0, time.UTC)
	opened := step(leftover, Observation{Session: &LiveSession{
		WTSSessionID: 2, Username: "ROHIT", AccountSID: "S-1-5-21-1", LogonAt: nextLogon,
	}}, agentID, nextLogon)
	if len(opened.Sessions) != 2 {
		t.Fatalf("new login must close previous then open, got %+v", opened.Sessions)
	}
	if opened.Sessions[0].SessionID != sid || opened.Sessions[0].Status != StatusClosed {
		t.Fatalf("previous session was not closed first: %+v", opened.Sessions[0])
	}
	if opened.Sessions[0].LogoffAt == nil || !opened.Sessions[0].LogoffAt.Equal(nextLogon) {
		t.Fatalf("previous logout = %+v, want new login %v", opened.Sessions[0], nextLogon)
	}
	if opened.Sessions[1].Status != StatusActive || opened.Sessions[1].SessionID == sid {
		t.Fatalf("new session = %+v", opened.Sessions[1])
	}
}
