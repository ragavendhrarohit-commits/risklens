package activity

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	txerrors "risklens/agent/internal/transport/errors"
	"risklens/agent/internal/transport/response"
)

func newLifecycleModule(t *testing.T, dir string, collector *stubCollector, ft *fakeTransport) *Module {
	t.Helper()
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
	mod.WithClock(func() time.Time {
		return time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC)
	})
	return mod
}

func waitUploads(t *testing.T, ft *fakeTransport, min int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ft.callCount() >= min {
			return
		}
		time.Sleep(15 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d uploads, have %d paths=%v", min, ft.callCount(), ft.recordedPaths())
}

func TestLifecycleLogin(t *testing.T) {
	dir := t.TempDir()
	ft := &fakeTransport{resp: &response.Response{StatusCode: response.StatusOK, Body: successBody()}}
	logon := time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC)
	collector := &stubCollector{obs: Observation{Session: &LiveSession{
		WTSSessionID: 1, Username: "rohit", AccountSID: "S-1-5-21-1", LogonAt: logon,
	}}}
	mod := newLifecycleModule(t, dir, collector, ft)
	if err := mod.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer mod.Stop(context.Background())
	waitUploads(t, ft, 1)

	uploads := ft.sessionUploads()
	if len(uploads) == 0 || len(uploads[0].Sessions) != 1 {
		t.Fatalf("login uploads = %+v", uploads)
	}
	item := uploads[0].Sessions[0]
	if item.Status != StatusActive || item.LogoffAt != nil || item.LogonAt.IsZero() {
		t.Fatalf("login session = %+v", item)
	}
	if !mod.state.hasSession() {
		t.Fatal("expected persisted open session")
	}
}

func TestLifecycleLogout(t *testing.T) {
	dir := t.TempDir()
	ft := &fakeTransport{resp: &response.Response{StatusCode: response.StatusOK, Body: successBody()}}
	logon := time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC)
	collector := &stubCollector{obs: Observation{Session: &LiveSession{
		WTSSessionID: 1, Username: "rohit", AccountSID: "S-1-5-21-1", LogonAt: logon,
	}}}
	mod := newLifecycleModule(t, dir, collector, ft)
	if err := mod.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer mod.Stop(context.Background())
	waitUploads(t, ft, 1)

	mod.HandleSessionChange(WTSSessionLogoff, 1)
	waitUploads(t, ft, 2)

	var closed *SessionItem
	for _, upload := range ft.sessionUploads() {
		for i := range upload.Sessions {
			if upload.Sessions[i].Status == StatusClosed {
				closed = &upload.Sessions[i]
			}
		}
	}
	if closed == nil || closed.LogoffAt == nil {
		t.Fatalf("expected closed session with logout time, uploads=%+v", ft.sessionUploads())
	}
	if closed.LogoffAt.Before(closed.LogonAt) {
		t.Fatalf("logout before login: %+v", closed)
	}
	if mod.state.hasSession() {
		t.Fatalf("expected empty state after logout, got %+v", mod.state)
	}
	if mod.lastEventType() != EventLogoff {
		t.Fatalf("last event = %s", mod.lastEventType())
	}
}

func TestLifecycleDuplicateLogin(t *testing.T) {
	dir := t.TempDir()
	ft := &fakeTransport{resp: &response.Response{StatusCode: response.StatusOK, Body: successBody()}}
	logon := time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC)
	live := &LiveSession{WTSSessionID: 4, Username: "rohit", AccountSID: "S-1-5-21-1", LogonAt: logon}
	collector := &stubCollector{obs: Observation{Session: live}}
	mod := newLifecycleModule(t, dir, collector, ft)
	if err := mod.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer mod.Stop(context.Background())
	waitUploads(t, ft, 1)
	firstID := mod.state.SessionID

	mod.HandleSessionChange(WTSSessionLogon, 4)
	if mod.state.SessionID != firstID {
		t.Fatalf("duplicate login minted a new session: %s vs %s", firstID, mod.state.SessionID)
	}
	ids := map[string]struct{}{}
	for _, upload := range ft.sessionUploads() {
		for _, item := range upload.Sessions {
			ids[item.SessionID] = struct{}{}
		}
	}
	if len(ids) != 1 {
		t.Fatalf("expected one session id, got %v", ids)
	}
}

func TestLifecycleDuplicateLogout(t *testing.T) {
	dir := t.TempDir()
	ft := &fakeTransport{resp: &response.Response{StatusCode: response.StatusOK, Body: successBody()}}
	logon := time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC)
	collector := &stubCollector{obs: Observation{Session: &LiveSession{
		WTSSessionID: 1, Username: "rohit", AccountSID: "S-1-5-21-1", LogonAt: logon,
	}}}
	mod := newLifecycleModule(t, dir, collector, ft)
	if err := mod.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer mod.Stop(context.Background())
	waitUploads(t, ft, 1)

	mod.HandleSessionChange(WTSSessionLogoff, 1)
	waitUploads(t, ft, 2)
	afterFirst := ft.callCount()
	mod.HandleSessionChange(WTSSessionLogoff, 1)
	time.Sleep(50 * time.Millisecond)
	if ft.callCount() != afterFirst {
		t.Fatalf("duplicate logout uploaded again, calls %d -> %d", afterFirst, ft.callCount())
	}
	closed := 0
	for _, upload := range ft.sessionUploads() {
		for _, item := range upload.Sessions {
			if item.Status == StatusClosed {
				closed++
			}
		}
	}
	if closed != 1 {
		t.Fatalf("closed uploads = %d", closed)
	}
}

func TestLifecycleLockDoesNotCloseSession(t *testing.T) {
	dir := t.TempDir()
	ft := &fakeTransport{resp: &response.Response{StatusCode: response.StatusOK, Body: successBody()}}
	logon := time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC)
	collector := &stubCollector{obs: Observation{Session: &LiveSession{
		WTSSessionID: 1, Username: "rohit", AccountSID: "S-1-5-21-1", LogonAt: logon,
	}}}
	mod := newLifecycleModule(t, dir, collector, ft)
	if err := mod.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer mod.Stop(context.Background())
	waitUploads(t, ft, 1)
	sid := mod.state.SessionID

	mod.HandleSessionChange(WTSSessionLock, 1)
	if !mod.state.hasSession() || mod.state.SessionID != sid {
		t.Fatalf("lock closed or replaced session: %+v", mod.state)
	}
	if mod.lastEventType() != EventLock {
		t.Fatalf("last event = %s", mod.lastEventType())
	}
	for _, upload := range ft.sessionUploads() {
		for _, item := range upload.Sessions {
			if item.Status == StatusClosed {
				t.Fatal("lock must not close the session")
			}
		}
	}
}

func TestLifecycleUnlockKeepsSession(t *testing.T) {
	dir := t.TempDir()
	ft := &fakeTransport{resp: &response.Response{StatusCode: response.StatusOK, Body: successBody()}}
	logon := time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC)
	collector := &stubCollector{obs: Observation{Session: &LiveSession{
		WTSSessionID: 1, Username: "rohit", AccountSID: "S-1-5-21-1", LogonAt: logon,
	}}}
	mod := newLifecycleModule(t, dir, collector, ft)
	if err := mod.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer mod.Stop(context.Background())
	waitUploads(t, ft, 1)
	sid := mod.state.SessionID

	mod.HandleSessionChange(WTSSessionLock, 1)
	mod.HandleSessionChange(WTSSessionUnlock, 1)
	if mod.state.SessionID != sid {
		t.Fatalf("unlock changed session id %s -> %s", sid, mod.state.SessionID)
	}
	if mod.lastEventType() != EventUnlock {
		t.Fatalf("last event = %s", mod.lastEventType())
	}
}

func TestLifecycleSleepWakeDoesNotClose(t *testing.T) {
	dir := t.TempDir()
	ft := &fakeTransport{resp: &response.Response{StatusCode: response.StatusOK, Body: successBody()}}
	logon := time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC)
	collector := &stubCollector{obs: Observation{Session: &LiveSession{
		WTSSessionID: 1, Username: "rohit", AccountSID: "S-1-5-21-1", LogonAt: logon,
	}}}
	mod := newLifecycleModule(t, dir, collector, ft)
	if err := mod.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer mod.Stop(context.Background())
	waitUploads(t, ft, 1)
	sid := mod.state.SessionID

	mod.HandleSessionChange(WTSSessionLock, 1)
	mod.HandleSessionChange(WTSSessionUnlock, 1)
	if !mod.state.hasSession() || mod.state.SessionID != sid {
		t.Fatal("sleep/wake must keep the same active session")
	}
	for _, upload := range ft.sessionUploads() {
		for _, item := range upload.Sessions {
			if item.Status == StatusClosed {
				t.Fatal("sleep/wake must not upload a closed session")
			}
		}
	}
}

func TestLifecycleHibernateDoesNotClose(t *testing.T) {
	TestLifecycleSleepWakeDoesNotClose(t)
}

func TestLifecycleEndSessionFlushes(t *testing.T) {
	dir := t.TempDir()
	ft := &fakeTransport{resp: &response.Response{StatusCode: response.StatusOK, Body: successBody()}}
	logon := time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC)
	collector := &stubCollector{obs: Observation{Session: &LiveSession{
		WTSSessionID: 1, Username: "rohit", AccountSID: "S-1-5-21-1", LogonAt: logon,
	}}}
	mod := newLifecycleModule(t, dir, collector, ft)
	if err := mod.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer mod.Stop(context.Background())
	waitUploads(t, ft, 1)

	if err := mod.CloseSession(context.Background(), "end_session"); err != nil {
		t.Fatal(err)
	}
	if mod.state.hasSession() {
		t.Fatal("WM_ENDSESSION / Fast Startup must close the session")
	}
}

func TestLifecycleSessionTerminateCloses(t *testing.T) {
	dir := t.TempDir()
	ft := &fakeTransport{resp: &response.Response{StatusCode: response.StatusOK, Body: successBody()}}
	logon := time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC)
	collector := &stubCollector{obs: Observation{Session: &LiveSession{
		WTSSessionID: 1, Username: "rohit", AccountSID: "S-1-5-21-1", LogonAt: logon,
	}}}
	mod := newLifecycleModule(t, dir, collector, ft)
	if err := mod.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer mod.Stop(context.Background())
	waitUploads(t, ft, 1)

	mod.HandleSessionChange(WTSSessionTerminate, 1)
	waitUploads(t, ft, 2)
	if mod.state.hasSession() {
		t.Fatal("WTS session terminate must close the session")
	}
}

func TestLifecycleConsecutiveLoginLogout(t *testing.T) {
	dir := t.TempDir()
	ft := &fakeTransport{resp: &response.Response{StatusCode: response.StatusOK, Body: successBody()}}
	firstLogon := time.Date(2026, 8, 16, 8, 14, 25, 0, time.UTC)
	secondLogon := time.Date(2026, 8, 16, 12, 18, 27, 0, time.UTC)
	collector := &stubCollector{obs: Observation{Session: &LiveSession{
		WTSSessionID: 1, Username: "ROHIT", AccountSID: "S-1-5-21-1", LogonAt: firstLogon,
	}}}
	mod := newLifecycleModule(t, dir, collector, ft)
	mod.WithClock(func() time.Time { return time.Date(2026, 8, 16, 11, 40, 0, 0, time.UTC) })
	if err := mod.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer mod.Stop(context.Background())
	waitUploads(t, ft, 1)

	mod.HandleSessionChange(WTSSessionLogoff, 1)
	waitUploads(t, ft, 2)
	firstID := ""
	for _, upload := range ft.sessionUploads() {
		for _, item := range upload.Sessions {
			if item.Status == StatusClosed {
				firstID = item.SessionID
				if item.LogoffAt == nil || !item.LogoffAt.Equal(time.Date(2026, 8, 16, 11, 40, 0, 0, time.UTC)) {
					t.Fatalf("session 1 logout = %+v", item)
				}
			}
		}
	}
	if firstID == "" {
		t.Fatal("session 1 was not closed")
	}

	collector.obs = Observation{Session: &LiveSession{
		WTSSessionID: 2, Username: "ROHIT", AccountSID: "S-1-5-21-1", LogonAt: secondLogon,
	}}
	mod.closing.Store(false)
	mod.WithClock(func() time.Time { return time.Date(2026, 8, 16, 12, 30, 8, 0, time.UTC) })
	mod.HandleSessionChange(WTSSessionLogon, 2)
	waitUploads(t, ft, 3)
	if !mod.state.hasSession() || mod.state.SessionID == firstID {
		t.Fatalf("session 2 not opened: %+v", mod.state)
	}
	mod.OnWindowsShutdown(context.Background())
	if mod.state.hasSession() {
		t.Fatal("session 2 shutdown left the session open")
	}
	ids := map[string]struct{}{}
	for _, upload := range ft.sessionUploads() {
		for _, item := range upload.Sessions {
			ids[item.SessionID] = struct{}{}
		}
	}
	if len(ids) != 2 {
		t.Fatalf("expected two session ids, got %v", ids)
	}
}

func TestLifecycleRDPConnect(t *testing.T) {
	dir := t.TempDir()
	ft := &fakeTransport{resp: &response.Response{StatusCode: response.StatusOK, Body: successBody()}}
	logon := time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC)
	collector := &stubCollector{obs: Observation{Session: &LiveSession{
		WTSSessionID: 8, Username: "rohit", AccountSID: "S-1-5-21-1", LogonAt: logon,
	}}}
	mod := newLifecycleModule(t, dir, collector, ft)
	if err := mod.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer mod.Stop(context.Background())
	waitUploads(t, ft, 1)
	sid := mod.state.SessionID

	mod.HandleSessionChange(WTSRemoteConnect, 8)
	if mod.state.SessionID != sid {
		t.Fatalf("rdp connect duplicated session %s -> %s", sid, mod.state.SessionID)
	}
	if mod.lastEventType() != EventRemoteConnect {
		t.Fatalf("last event = %s", mod.lastEventType())
	}
}

func TestLifecycleRDPDisconnectDoesNotClose(t *testing.T) {
	dir := t.TempDir()
	ft := &fakeTransport{resp: &response.Response{StatusCode: response.StatusOK, Body: successBody()}}
	logon := time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC)
	collector := &stubCollector{obs: Observation{Session: &LiveSession{
		WTSSessionID: 8, Username: "rohit", AccountSID: "S-1-5-21-1", LogonAt: logon,
	}}}
	mod := newLifecycleModule(t, dir, collector, ft)
	if err := mod.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer mod.Stop(context.Background())
	waitUploads(t, ft, 1)
	sid := mod.state.SessionID

	mod.HandleSessionChange(WTSRemoteDisconnect, 8)
	if !mod.state.hasSession() || mod.state.SessionID != sid {
		t.Fatalf("rdp disconnect closed session: %+v", mod.state)
	}
	if mod.lastEventType() != EventRemoteDisconnect {
		t.Fatalf("last event = %s", mod.lastEventType())
	}
}

func TestLifecycleShutdownFlush(t *testing.T) {
	dir := t.TempDir()
	ft := &fakeTransport{resp: &response.Response{StatusCode: response.StatusOK, Body: successBody()}}
	logon := time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC)
	collector := &stubCollector{obs: Observation{Session: &LiveSession{
		WTSSessionID: 1, Username: "rohit", AccountSID: "S-1-5-21-1", LogonAt: logon,
	}}}
	mod := newLifecycleModule(t, dir, collector, ft)
	if err := mod.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer mod.Stop(context.Background())
	waitUploads(t, ft, 1)

	mod.OnWindowsShutdown(context.Background())
	waitUploads(t, ft, 2)
	found := false
	for _, upload := range ft.sessionUploads() {
		for _, item := range upload.Sessions {
			if item.Status == StatusClosed && item.LogoffAt != nil {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("shutdown did not upload closed session: %+v", ft.sessionUploads())
	}
	if mod.lastEventType() != EventShutdown {
		t.Fatalf("last event = %s", mod.lastEventType())
	}
}

func TestLifecycleRestartRecovery(t *testing.T) {
	dir := t.TempDir()
	logon := time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC)
	prev := persistedState{
		SessionID:    "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee",
		WTSSessionID: 1,
		Username:     "rohit",
		AccountSID:   "S-1-5-21-1",
		LogonAt:      logon,
	}
	data, err := json.Marshal(prev)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, stateFileName), data, 0o644); err != nil {
		t.Fatal(err)
	}

	ft := &fakeTransport{resp: &response.Response{StatusCode: response.StatusOK, Body: successBody()}}
	mod := newLifecycleModule(t, dir, &stubCollector{}, ft)
	if err := mod.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer mod.Stop(context.Background())
	waitUploads(t, ft, 1)

	uploads := ft.sessionUploads()
	if len(uploads) == 0 || len(uploads[0].Sessions) != 1 {
		t.Fatalf("recovery uploads = %+v", uploads)
	}
	item := uploads[0].Sessions[0]
	if item.SessionID != prev.SessionID || item.Status != StatusClosed || item.LogoffAt == nil {
		t.Fatalf("recovered session = %+v", item)
	}
	if item.LogonAt.UTC() != logon {
		t.Fatalf("login time lost: %v", item.LogonAt)
	}
}

func TestLifecycleOfflineRetry(t *testing.T) {
	dir := t.TempDir()
	ft := &fakeTransport{
		resp: &response.Response{StatusCode: response.StatusOK, Body: successBody()},
		err:  txerrors.New(txerrors.KindTimeout, "send", context.DeadlineExceeded),
	}
	logon := time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC)
	collector := &stubCollector{obs: Observation{Session: &LiveSession{
		WTSSessionID: 1, Username: "rohit", AccountSID: "S-1-5-21-1", LogonAt: logon,
	}}}
	mod := newLifecycleModule(t, dir, collector, ft)
	if err := mod.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer mod.Stop(context.Background())

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && mod.QueueLen() == 0 {
		time.Sleep(15 * time.Millisecond)
	}
	if mod.QueueLen() == 0 {
		t.Fatal("expected failed login to be queued")
	}
	if _, err := os.Stat(filepath.Join(dir, queueFileName)); err != nil {
		t.Fatalf("queue file: %v", err)
	}

	ft.setErr(nil)
	mod.HandleSessionChange(WTSSessionLogon, 1)
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && mod.QueueLen() > 0 {
		time.Sleep(15 * time.Millisecond)
	}
	if mod.QueueLen() != 0 {
		t.Fatalf("queue not drained after backend recovered, len=%d", mod.QueueLen())
	}
	if len(ft.sessionUploads()) == 0 {
		t.Fatal("expected retry to upload the queued session")
	}
}

func TestLifecycleQueuedCloseReplayedAfterRestart(t *testing.T) {
	dir := t.TempDir()
	logon := time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC)
	logoff := logon.Add(8 * time.Hour)
	queued := uploadQueue{Items: []queuedPayload{{
		Kind: "sessions",
		Sessions: &SessionUpload{
			AgentID:     sampleIdentity().AgentID,
			InstallID:   sampleIdentity().InstallID,
			CollectedAt: logoff,
			Sessions: []SessionItem{{
				SessionID:  "bbbbbbbb-cccc-4ddd-8eee-ffffffffffff",
				Username:   "rohit",
				AccountSID: "S-1-5-21-1",
				LogonAt:    logon,
				LogoffAt:   &logoff,
				Status:     StatusClosed,
			}},
		},
		EnqueuedAt: logoff,
	}}}
	if err := saveQueue(filepath.Join(dir, queueFileName), queued); err != nil {
		t.Fatal(err)
	}

	ft := &fakeTransport{resp: &response.Response{StatusCode: response.StatusOK, Body: successBody()}}
	mod := newLifecycleModule(t, dir, &stubCollector{}, ft)
	if err := mod.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer mod.Stop(context.Background())
	waitUploads(t, ft, 1)

	uploads := ft.sessionUploads()
	if len(uploads) == 0 || len(uploads[0].Sessions) != 1 {
		t.Fatalf("replay uploads = %+v", uploads)
	}
	item := uploads[0].Sessions[0]
	if item.Status != StatusClosed || item.LogoffAt == nil {
		t.Fatalf("replayed session = %+v", item)
	}
	if mod.QueueLen() != 0 {
		t.Fatalf("queue not drained after restart, len=%d", mod.QueueLen())
	}
}

func TestCloseSessionUsedByLogoffAndShutdown(t *testing.T) {
	dir := t.TempDir()
	ft := &fakeTransport{resp: &response.Response{StatusCode: response.StatusOK, Body: successBody()}}
	logon := time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC)
	collector := &stubCollector{obs: Observation{Session: &LiveSession{
		WTSSessionID: 1, Username: "rohit", AccountSID: "S-1-5-21-1", LogonAt: logon,
	}}}
	mod := newLifecycleModule(t, dir, collector, ft)
	if err := mod.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer mod.Stop(context.Background())
	waitUploads(t, ft, 1)

	if err := mod.CloseSession(context.Background(), "logoff"); err != nil {
		t.Fatal(err)
	}
	if mod.state.hasSession() {
		t.Fatal("CloseSession(logoff) left the session open")
	}

	collector.obs = Observation{Session: &LiveSession{
		WTSSessionID: 2, Username: "rohit", AccountSID: "S-1-5-21-1", LogonAt: logon.Add(time.Hour),
	}}
	mod.closing.Store(false)
	mod.HandleSessionChange(WTSSessionLogon, 2)
	waitUploads(t, ft, 2)
	mod.OnWindowsShutdown(context.Background())
	if mod.state.hasSession() {
		t.Fatal("CloseSession(shutdown) left the session open")
	}
}

func TestLifecycleStopDoesNotCloseSession(t *testing.T) {
	dir := t.TempDir()
	ft := &fakeTransport{resp: &response.Response{StatusCode: response.StatusOK, Body: successBody()}}
	logon := time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC)
	collector := &stubCollector{obs: Observation{Session: &LiveSession{
		WTSSessionID: 1, Username: "rohit", AccountSID: "S-1-5-21-1", LogonAt: logon,
	}}}
	mod := newLifecycleModule(t, dir, collector, ft)
	if err := mod.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitUploads(t, ft, 1)
	if err := mod.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, upload := range ft.sessionUploads() {
		for _, item := range upload.Sessions {
			if item.Status == StatusClosed {
				t.Fatal("agent stop must not close the Windows session")
			}
		}
	}
	if !mod.state.hasSession() {
		t.Fatal("agent stop must keep the persisted open session")
	}
}

func TestLifecycleLoginLoginShutdown(t *testing.T) {
	dir := t.TempDir()
	ft := &fakeTransport{resp: &response.Response{StatusCode: response.StatusOK, Body: successBody()}}
	firstLogon := time.Date(2026, 8, 16, 12, 18, 27, 0, time.UTC)
	secondLogon := time.Date(2026, 8, 16, 13, 14, 0, 0, time.UTC)
	shutdownAt := time.Date(2026, 8, 16, 13, 21, 0, 0, time.UTC)
	collector := &stubCollector{obs: Observation{Session: &LiveSession{
		WTSSessionID: 1, Username: "ROHIT", AccountSID: "S-1-5-21-1", LogonAt: firstLogon,
	}}}
	mod := newLifecycleModule(t, dir, collector, ft)
	mod.WithClock(func() time.Time { return firstLogon.Add(time.Minute) })
	if err := mod.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer mod.Stop(context.Background())
	waitUploads(t, ft, 1)
	firstID := mod.state.SessionID

	collector.obs = Observation{Session: &LiveSession{
		WTSSessionID: 2, Username: "ROHIT", AccountSID: "S-1-5-21-1", LogonAt: secondLogon,
	}}
	mod.WithClock(func() time.Time { return shutdownAt })
	mod.HandleSessionChange(WTSSessionLogon, 2)
	waitUploads(t, ft, 3)
	if !mod.state.hasSession() || mod.state.SessionID == firstID {
		t.Fatalf("second login did not open a new session: %+v", mod.state)
	}
	secondID := mod.state.SessionID

	mod.OnWindowsShutdown(context.Background())
	waitUploads(t, ft, 4)

	var firstClosed, secondClosed *SessionItem
	for _, upload := range ft.sessionUploads() {
		for i := range upload.Sessions {
			item := upload.Sessions[i]
			if item.SessionID == firstID && item.Status == StatusClosed {
				copy := item
				firstClosed = &copy
			}
			if item.SessionID == secondID && item.Status == StatusClosed {
				copy := item
				secondClosed = &copy
			}
		}
	}
	if firstClosed == nil || firstClosed.LogoffAt == nil {
		t.Fatal("session A was not closed on the second login")
	}
	if !firstClosed.LogoffAt.Equal(secondLogon) {
		t.Fatalf("session A logout = %v, want second login %v", firstClosed.LogoffAt, secondLogon)
	}
	if secondClosed == nil || secondClosed.LogoffAt == nil {
		t.Fatal("session B was not closed on shutdown")
	}
	if !secondClosed.LogoffAt.Equal(shutdownAt) {
		t.Fatalf("session B logout = %v, want shutdown %v", secondClosed.LogoffAt, shutdownAt)
	}
	if countClosedUploads(ft, firstID) != 1 {
		t.Fatalf("shutdown re-uploaded session A close, count=%d", countClosedUploads(ft, firstID))
	}
	assertNoDualActiveUpload(t, ft)
	assertCloseUploadedBeforeOpen(t, ft, firstID, secondID)
}

func assertNoDualActiveUpload(t *testing.T, ft *fakeTransport) {
	t.Helper()
	for _, upload := range ft.sessionUploads() {
		actives := 0
		for _, item := range upload.Sessions {
			if item.Status == StatusActive {
				actives++
			}
		}
		if actives > 1 {
			t.Fatalf("agent uploaded %d active sessions in one payload: %+v", actives, upload.Sessions)
		}
	}
}

func assertCloseUploadedBeforeOpen(t *testing.T, ft *fakeTransport, previousID, newID string) {
	t.Helper()
	closeSeen := false
	openSeen := false
	for _, upload := range ft.sessionUploads() {
		for _, item := range upload.Sessions {
			if item.SessionID == previousID && item.Status == StatusClosed {
				if openSeen {
					t.Fatal("new session was uploaded before the previous session was closed")
				}
				closeSeen = true
			}
			if item.SessionID == newID && item.Status == StatusActive {
				openSeen = true
			}
		}
	}
	if !closeSeen || !openSeen {
		t.Fatalf("expected previous close then new open, close=%v open=%v", closeSeen, openSeen)
	}
}

func TestLifecycleScenarioLoginLogoutLoginLogout(t *testing.T) {
	dir := t.TempDir()
	ft := &fakeTransport{resp: &response.Response{StatusCode: response.StatusOK, Body: successBody()}}
	firstLogon := time.Date(2026, 8, 16, 12, 18, 27, 0, time.UTC)
	logout1 := time.Date(2026, 8, 16, 13, 14, 0, 0, time.UTC)
	secondLogon := time.Date(2026, 8, 16, 15, 20, 0, 0, time.UTC)
	logout2 := time.Date(2026, 8, 16, 15, 45, 0, 0, time.UTC)
	collector := &stubCollector{obs: Observation{Session: &LiveSession{
		WTSSessionID: 1, Username: "ROHIT", AccountSID: "S-1-5-21-1", LogonAt: firstLogon,
	}}}
	mod := newLifecycleModule(t, dir, collector, ft)
	mod.WithClock(func() time.Time { return firstLogon.Add(time.Minute) })
	if err := mod.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer mod.Stop(context.Background())
	waitUploads(t, ft, 1)
	firstID := mod.state.SessionID

	mod.WithClock(func() time.Time { return logout1 })
	mod.HandleSessionChange(WTSSessionLogoff, 1)
	waitUploads(t, ft, 2)
	if mod.state.hasSession() {
		t.Fatalf("logout left session active: %+v", mod.state)
	}

	collector.obs = Observation{Session: &LiveSession{
		WTSSessionID: 1, Username: "ROHIT", AccountSID: "S-1-5-21-1",
	}}
	mod.closing.Store(false)
	mod.HandleSessionChange(WTSSessionLogon, 1)
	time.Sleep(50 * time.Millisecond)
	if mod.state.hasSession() {
		t.Fatalf("empty LogonTime after logout fabricated a session: %+v", mod.state)
	}

	collector.obs = Observation{Session: &LiveSession{
		WTSSessionID: 2, Username: "ROHIT", AccountSID: "S-1-5-21-1", LogonAt: secondLogon,
	}}
	mod.WithClock(func() time.Time { return secondLogon })
	mod.HandleSessionChange(WTSSessionLogon, 2)
	waitUploads(t, ft, 3)
	if !mod.state.hasSession() || mod.state.SessionID == firstID {
		t.Fatalf("session 2 not opened: %+v", mod.state)
	}
	if !mod.state.LogonAt.Equal(secondLogon) {
		t.Fatalf("session 2 login = %v, want %v", mod.state.LogonAt, secondLogon)
	}
	secondID := mod.state.SessionID

	mod.WithClock(func() time.Time { return logout2 })
	mod.HandleSessionChange(WTSSessionLogoff, 2)
	waitUploads(t, ft, 4)

	var firstClosed, secondClosed *SessionItem
	logons := map[string]time.Time{}
	for _, upload := range ft.sessionUploads() {
		for i := range upload.Sessions {
			item := upload.Sessions[i]
			logons[item.SessionID] = item.LogonAt
			if item.SessionID == firstID && item.Status == StatusClosed {
				copy := item
				firstClosed = &copy
			}
			if item.SessionID == secondID && item.Status == StatusClosed {
				copy := item
				secondClosed = &copy
			}
		}
	}
	if firstClosed == nil || firstClosed.LogoffAt == nil || !firstClosed.LogoffAt.Equal(logout1) {
		t.Fatalf("session 1 = %+v", firstClosed)
	}
	if !firstClosed.LogonAt.Equal(firstLogon) {
		t.Fatalf("session 1 login changed: %v", firstClosed.LogonAt)
	}
	if secondClosed == nil || secondClosed.LogoffAt == nil || !secondClosed.LogoffAt.Equal(logout2) {
		t.Fatalf("session 2 = %+v", secondClosed)
	}
	if !secondClosed.LogonAt.Equal(secondLogon) {
		t.Fatalf("session 2 login = %v", secondClosed.LogonAt)
	}
	for _, logon := range logons {
		if logon.Equal(logout1) {
			t.Fatal("fabricated a session that starts at the 23:14 logout")
		}
	}
	assertNoDualActiveUpload(t, ft)
}

func TestLifecycleScenarioPowerFailure(t *testing.T) {
	dir := t.TempDir()
	ft := &fakeTransport{resp: &response.Response{StatusCode: response.StatusOK, Body: successBody()}}
	firstLogon := time.Date(2026, 8, 16, 12, 18, 27, 0, time.UTC)
	secondLogon := time.Date(2026, 8, 16, 15, 20, 0, 0, time.UTC)
	collector := &stubCollector{obs: Observation{Session: &LiveSession{
		WTSSessionID: 1, Username: "ROHIT", AccountSID: "S-1-5-21-1", LogonAt: firstLogon,
	}}}
	mod := newLifecycleModule(t, dir, collector, ft)
	mod.WithClock(func() time.Time { return firstLogon.Add(time.Minute) })
	if err := mod.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer mod.Stop(context.Background())
	waitUploads(t, ft, 1)
	firstID := mod.state.SessionID

	collector.obs = Observation{Session: &LiveSession{
		WTSSessionID: 2, Username: "ROHIT", AccountSID: "S-1-5-21-1", LogonAt: secondLogon,
	}}
	mod.WithClock(func() time.Time { return secondLogon })
	mod.HandleSessionChange(WTSSessionLogon, 2)
	waitUploads(t, ft, 3)
	if !mod.state.hasSession() || mod.state.SessionID == firstID {
		t.Fatalf("session 2 not opened: %+v", mod.state)
	}
	if !mod.state.LogonAt.Equal(secondLogon) {
		t.Fatalf("session 2 login = %v", mod.state.LogonAt)
	}

	var firstClosed *SessionItem
	for _, upload := range ft.sessionUploads() {
		for i := range upload.Sessions {
			item := upload.Sessions[i]
			if item.SessionID == firstID && item.Status == StatusClosed {
				copy := item
				firstClosed = &copy
			}
		}
	}
	if firstClosed == nil || firstClosed.LogoffAt == nil || !firstClosed.LogoffAt.Equal(secondLogon) {
		t.Fatalf("missed-logout fallback = %+v", firstClosed)
	}
	assertNoDualActiveUpload(t, ft)
	assertCloseUploadedBeforeOpen(t, ft, firstID, mod.state.SessionID)
}

func TestLifecycleScenarioShutdown(t *testing.T) {
	dir := t.TempDir()
	ft := &fakeTransport{resp: &response.Response{StatusCode: response.StatusOK, Body: successBody()}}
	logon := time.Date(2026, 8, 16, 12, 18, 27, 0, time.UTC)
	shutdownAt := time.Date(2026, 8, 16, 13, 21, 0, 0, time.UTC)
	collector := &stubCollector{obs: Observation{Session: &LiveSession{
		WTSSessionID: 1, Username: "ROHIT", AccountSID: "S-1-5-21-1", LogonAt: logon,
	}}}
	mod := newLifecycleModule(t, dir, collector, ft)
	mod.WithClock(func() time.Time { return logon.Add(time.Minute) })
	if err := mod.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer mod.Stop(context.Background())
	waitUploads(t, ft, 1)

	mod.WithClock(func() time.Time { return shutdownAt })
	mod.OnWindowsShutdown(context.Background())
	waitUploads(t, ft, 2)
	found := false
	for _, upload := range ft.sessionUploads() {
		for _, item := range upload.Sessions {
			if item.Status == StatusClosed && item.LogoffAt != nil && item.LogoffAt.Equal(shutdownAt) && item.LogonAt.Equal(logon) {
				found = true
			}
			if item.Status == StatusActive && item.LogonAt.Equal(shutdownAt) {
				t.Fatal("shutdown fabricated a login at shutdown time")
			}
		}
	}
	if !found {
		t.Fatalf("shutdown did not close the 22:18 session: %+v", ft.sessionUploads())
	}
	if mod.state.hasSession() {
		t.Fatal("shutdown left the session active")
	}
}

func TestLifecycleQueuedRealLogoutReplacesNothingLocally(t *testing.T) {
	dir := t.TempDir()
	logon := time.Date(2026, 8, 16, 12, 18, 27, 0, time.UTC)
	realLogout := time.Date(2026, 8, 16, 13, 14, 0, 0, time.UTC)
	queued := uploadQueue{Items: []queuedPayload{{
		Kind: "sessions",
		Sessions: &SessionUpload{
			AgentID:     sampleIdentity().AgentID,
			InstallID:   sampleIdentity().InstallID,
			CollectedAt: realLogout,
			Sessions: []SessionItem{{
				SessionID:  "cccccccc-dddd-4eee-8fff-000000000000",
				Username:   "ROHIT",
				AccountSID: "S-1-5-21-1",
				LogonAt:    logon,
				LogoffAt:   &realLogout,
				Status:     StatusClosed,
			}},
		},
		EnqueuedAt: realLogout,
	}}}
	if err := saveQueue(filepath.Join(dir, queueFileName), queued); err != nil {
		t.Fatal(err)
	}
	ft := &fakeTransport{resp: &response.Response{StatusCode: response.StatusOK, Body: successBody()}}
	mod := newLifecycleModule(t, dir, &stubCollector{}, ft)
	if err := mod.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer mod.Stop(context.Background())
	waitUploads(t, ft, 1)
	item := ft.sessionUploads()[0].Sessions[0]
	if !item.LogonAt.Equal(logon) || item.LogoffAt == nil || !item.LogoffAt.Equal(realLogout) {
		t.Fatalf("queued real logout was rewritten: %+v", item)
	}
}

func TestLifecycleShutdownThenNewLoginDoesNotReclosePrevious(t *testing.T) {
	dir := t.TempDir()
	ft := &fakeTransport{resp: &response.Response{StatusCode: response.StatusOK, Body: successBody()}}
	loginA := time.Date(2026, 8, 16, 12, 22, 56, 0, time.UTC)
	shutdownAt := time.Date(2026, 8, 16, 12, 25, 25, 0, time.UTC)
	loginB := time.Date(2026, 8, 16, 12, 34, 19, 0, time.UTC)
	collector := &stubCollector{obs: Observation{Session: &LiveSession{
		WTSSessionID: 1, Username: "ROHIT", AccountSID: "S-1-5-21-1", LogonAt: loginA,
	}}}
	mod := newLifecycleModule(t, dir, collector, ft)
	mod.WithClock(func() time.Time { return loginA.Add(time.Minute) })
	if err := mod.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer mod.Stop(context.Background())
	waitUploads(t, ft, 1)
	firstID := mod.state.SessionID

	mod.WithClock(func() time.Time { return shutdownAt })
	mod.OnWindowsShutdown(context.Background())
	waitUploads(t, ft, 2)
	if mod.state.hasSession() {
		t.Fatal("shutdown left the session active")
	}

	collector.obs = Observation{Session: &LiveSession{
		WTSSessionID: 2, Username: "ROHIT", AccountSID: "S-1-5-21-1", LogonAt: loginB,
	}}
	mod.closing.Store(false)
	mod.WithClock(func() time.Time { return loginB })
	beforeLogin := len(ft.sessionUploads())
	mod.HandleSessionChange(WTSSessionLogon, 2)
	waitUploads(t, ft, 3)
	if !mod.state.hasSession() || mod.state.SessionID == firstID {
		t.Fatalf("new login did not create a new session: %+v", mod.state)
	}
	if !mod.state.LogonAt.Equal(loginB) {
		t.Fatalf("session B login = %v, want %v", mod.state.LogonAt, loginB)
	}

	for _, upload := range ft.sessionUploads()[beforeLogin:] {
		for _, item := range upload.Sessions {
			if item.SessionID == firstID {
				t.Fatalf("session A was uploaded again after shutdown: %+v", item)
			}
			if item.LogonAt.Equal(shutdownAt) {
				t.Fatal("fabricated a login at shutdown time")
			}
			if item.Status == StatusClosed && item.LogoffAt != nil && item.LogoffAt.After(shutdownAt) && item.LogonAt.Equal(loginA) {
				t.Fatalf("session A logout was rewritten: %+v", item)
			}
		}
	}
	assertNoDualActiveUpload(t, ft)
}

func assertSessionForgotten(t *testing.T, dir string, mod *Module, sessionID string) {
	t.Helper()
	if mod.state.SessionID == sessionID {
		t.Fatalf("in-memory activity state still contains %s: %+v", sessionID, mod.state)
	}
	disk, err := loadState(filepath.Join(dir, stateFileName))
	if err != nil {
		t.Fatal(err)
	}
	if disk.SessionID == sessionID {
		t.Fatalf("activity.json still contains %s: %+v", sessionID, disk)
	}
	if mod.queue.containsSessionID(sessionID) {
		t.Fatalf("in-memory upload queue still contains %s", sessionID)
	}
	q := loadQueue(filepath.Join(dir, queueFileName))
	if q.containsSessionID(sessionID) {
		t.Fatalf("session_upload_queue.json still contains %s: %+v", sessionID, q.Items)
	}
}

func countClosedUploads(ft *fakeTransport, sessionID string) int {
	n := 0
	for _, upload := range ft.sessionUploads() {
		for _, item := range upload.Sessions {
			if item.SessionID == sessionID && item.Status == StatusClosed {
				n++
			}
		}
	}
	return n
}

func TestSuccessfulShutdownRemovesSessionFromStateAndQueues(t *testing.T) {
	dir := t.TempDir()
	ft := &fakeTransport{resp: &response.Response{StatusCode: response.StatusOK, Body: successBody()}}
	loginA := time.Date(2026, 8, 16, 12, 22, 56, 0, time.UTC)
	shutdownAt := time.Date(2026, 8, 16, 12, 25, 25, 0, time.UTC)
	collector := &stubCollector{obs: Observation{Session: &LiveSession{
		WTSSessionID: 1, Username: "ROHIT", AccountSID: "S-1-5-21-1", LogonAt: loginA,
	}}}
	mod := newLifecycleModule(t, dir, collector, ft)
	mod.WithClock(func() time.Time { return loginA.Add(time.Minute) })
	if err := mod.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer mod.Stop(context.Background())
	waitUploads(t, ft, 1)
	firstID := mod.state.SessionID

	mod.WithClock(func() time.Time { return shutdownAt })
	mod.OnWindowsShutdown(context.Background())
	waitUploads(t, ft, 2)

	assertSessionForgotten(t, dir, mod, firstID)
	if !mod.state.alreadyUploadedClosed(firstID) {
		t.Fatalf("successful close was not recorded as uploaded: %+v", mod.state.UploadedClosedIDs)
	}
	if countClosedUploads(ft, firstID) != 1 {
		t.Fatalf("closed uploads of A = %d", countClosedUploads(ft, firstID))
	}

	after := ft.callCount()
	mod.cycleNow()
	mod.HandleSessionChange(WTSSessionLogon, 1)
	time.Sleep(50 * time.Millisecond)
	if ft.callCount() != after {
		t.Fatalf("heartbeat/login re-uploaded closed session, calls %d -> %d uploads=%+v", after, ft.callCount(), ft.sessionUploads())
	}
	assertSessionForgotten(t, dir, mod, firstID)
}

func TestRestartAfterSuccessfulCloseDoesNotReplay(t *testing.T) {
	dir := t.TempDir()
	ft := &fakeTransport{resp: &response.Response{StatusCode: response.StatusOK, Body: successBody()}}
	loginA := time.Date(2026, 8, 16, 12, 22, 56, 0, time.UTC)
	shutdownAt := time.Date(2026, 8, 16, 12, 25, 25, 0, time.UTC)
	collector := &stubCollector{obs: Observation{Session: &LiveSession{
		WTSSessionID: 1, Username: "ROHIT", AccountSID: "S-1-5-21-1", LogonAt: loginA,
	}}}
	mod := newLifecycleModule(t, dir, collector, ft)
	mod.WithClock(func() time.Time { return loginA.Add(time.Minute) })
	if err := mod.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitUploads(t, ft, 1)
	firstID := mod.state.SessionID
	mod.WithClock(func() time.Time { return shutdownAt })
	mod.OnWindowsShutdown(context.Background())
	waitUploads(t, ft, 2)
	if err := mod.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}

	ft2 := &fakeTransport{resp: &response.Response{StatusCode: response.StatusOK, Body: successBody()}}
	mod2 := newLifecycleModule(t, dir, &stubCollector{}, ft2)
	if err := mod2.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer mod2.Stop(context.Background())
	time.Sleep(80 * time.Millisecond)
	if len(ft2.sessionUploads()) != 0 {
		t.Fatalf("restart replayed successfully uploaded session: %+v", ft2.sessionUploads())
	}
	assertSessionForgotten(t, dir, mod2, firstID)
}

func TestLeftoverOpenMatchingLastClosedDoesNotReupload(t *testing.T) {
	dir := t.TempDir()
	logon := time.Date(2026, 8, 16, 12, 22, 56, 0, time.UTC)
	sessionID := "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	leftover := persistedState{
		SessionID:           sessionID,
		WTSSessionID:        1,
		Username:            "ROHIT",
		AccountSID:          "S-1-5-21-1",
		LogonAt:             logon,
		LastClosedSessionID: sessionID,
		LastClosedLogonAt:   logon,
		UploadedClosedIDs:   []string{sessionID},
	}
	if err := saveState(filepath.Join(dir, stateFileName), leftover); err != nil {
		t.Fatal(err)
	}
	queued := uploadQueue{Items: []queuedPayload{{
		Kind: "sessions",
		Sessions: &SessionUpload{
			AgentID:   sampleIdentity().AgentID,
			InstallID: sampleIdentity().InstallID,
			Sessions: []SessionItem{{
				SessionID: sessionID,
				LogonAt:   logon,
				Status:    StatusClosed,
			}},
		},
	}}}
	if err := saveQueue(filepath.Join(dir, queueFileName), queued); err != nil {
		t.Fatal(err)
	}

	ft := &fakeTransport{resp: &response.Response{StatusCode: response.StatusOK, Body: successBody()}}
	mod := newLifecycleModule(t, dir, &stubCollector{}, ft)
	if err := mod.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer mod.Stop(context.Background())
	time.Sleep(80 * time.Millisecond)
	if len(ft.sessionUploads()) != 0 {
		t.Fatalf("retry/restart replayed an already-uploaded close: %+v", ft.sessionUploads())
	}
	assertSessionForgotten(t, dir, mod, sessionID)
}

func TestFailedShutdownCloseReplaysFromQueue(t *testing.T) {
	dir := t.TempDir()
	ft := &fakeTransport{
		resp: &response.Response{StatusCode: response.StatusOK, Body: successBody()},
	}
	loginA := time.Date(2026, 8, 16, 12, 22, 56, 0, time.UTC)
	shutdownAt := time.Date(2026, 8, 16, 12, 25, 25, 0, time.UTC)
	collector := &stubCollector{obs: Observation{Session: &LiveSession{
		WTSSessionID: 1, Username: "ROHIT", AccountSID: "S-1-5-21-1", LogonAt: loginA,
	}}}
	mod := newLifecycleModule(t, dir, collector, ft)
	mod.WithClock(func() time.Time { return loginA.Add(time.Minute) })
	if err := mod.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitUploads(t, ft, 1)
	firstID := mod.state.SessionID

	ft.setErr(txerrors.New(txerrors.KindTimeout, "send", context.DeadlineExceeded))
	mod.WithClock(func() time.Time { return shutdownAt })
	mod.OnWindowsShutdown(context.Background())
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && !mod.queue.containsSessionID(firstID) {
		time.Sleep(15 * time.Millisecond)
	}
	if !mod.queue.containsSessionID(firstID) {
		t.Fatal("failed close must remain in the retry queue")
	}
	if mod.state.hasSession() {
		t.Fatalf("failed close left the session active: %+v", mod.state)
	}
	if mod.state.alreadyUploadedClosed(firstID) {
		t.Fatal("failed close must not be marked uploaded")
	}
	if err := mod.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}

	ft2 := &fakeTransport{resp: &response.Response{StatusCode: response.StatusOK, Body: successBody()}}
	mod2 := newLifecycleModule(t, dir, &stubCollector{}, ft2)
	if err := mod2.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer mod2.Stop(context.Background())
	waitUploads(t, ft2, 1)
	if countClosedUploads(ft2, firstID) != 1 {
		t.Fatalf("restart did not replay failed close: %+v", ft2.sessionUploads())
	}
	item := ft2.sessionUploads()[0].Sessions[0]
	if item.LogoffAt == nil || !item.LogoffAt.Equal(shutdownAt) {
		t.Fatalf("replayed logout time = %+v, want %v", item, shutdownAt)
	}
	assertSessionForgotten(t, dir, mod2, firstID)
	if !mod2.state.alreadyUploadedClosed(firstID) {
		t.Fatal("replayed close was not marked uploaded")
	}
}
