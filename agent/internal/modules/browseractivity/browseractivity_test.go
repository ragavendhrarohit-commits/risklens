package browseractivity

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	idmodel "risklens/agent/internal/identity/model"
	"risklens/agent/internal/modules/activity"
	"risklens/agent/internal/modules/interfaces"
	txerrors "risklens/agent/internal/transport/errors"
	"risklens/agent/internal/transport/request"
	"risklens/agent/internal/transport/response"
)

func TestModuleImplementsInterface(t *testing.T) {
	var _ interfaces.Module = (*Module)(nil)
}

func TestApplicationsPathUnchanged(t *testing.T) {
	if activity.ApplicationsPath != "/api/v1/applications" {
		t.Fatalf("application activity path changed: %s", activity.ApplicationsPath)
	}
	if BrowserPath != "/api/v1/browser" {
		t.Fatalf("browser path = %s", BrowserPath)
	}
	if BrowserPath == activity.ApplicationsPath {
		t.Fatal("browser upload must not reuse the applications path")
	}
}

func TestStripURLDropsSecretsAndInternalPages(t *testing.T) {
	host, stripped, err := StripURL("https://user:pass@github.com/org/repo?token=secret#frag")
	if err != nil {
		t.Fatal(err)
	}
	if host != "github.com" {
		t.Fatalf("host = %s", host)
	}
	if stripped != "https://github.com" {
		t.Fatalf("stripped = %s", stripped)
	}
	if strings.Contains(stripped, "token") || strings.Contains(stripped, "user") {
		t.Fatalf("secret leaked: %s", stripped)
	}

	if _, _, err := StripURL("chrome://settings"); !errors.Is(err, errForbiddenURL) {
		t.Fatalf("chrome scheme: %v", err)
	}
	if _, _, err := StripURL("file:///c:/secrets.txt"); !errors.Is(err, errForbiddenURL) {
		t.Fatalf("file scheme: %v", err)
	}
}

func TestSanitizeDropsIncognitoAndUnsupported(t *testing.T) {
	_, dropped, err := Sanitize(ExtensionMessage{
		Type: MessageActive, Browser: "chrome", URL: "https://github.com", Incognito: true,
	})
	if !dropped || !errors.Is(err, errIncognito) {
		t.Fatalf("incognito: dropped=%v err=%v", dropped, err)
	}

	sample, dropped, err := Sanitize(ExtensionMessage{
		Type: MessageActive, Browser: "chrome", URL: "https://www.GitHub.com/org?x=1",
		Title: "GitHub", WindowID: 1, TabID: 9,
		At: time.Date(2026, 8, 21, 1, 0, 0, 0, time.UTC),
	})
	if err != nil || dropped {
		t.Fatalf("sanitize: dropped=%v err=%v", dropped, err)
	}
	if sample.Executable != ExeChrome || sample.Domain != "github.com" || sample.URL != "https://github.com" {
		t.Fatalf("sample = %+v", sample)
	}
}

func TestSupportedBrowsers(t *testing.T) {
	cases := []struct {
		browser, exe string
	}{
		{"chrome", ExeChrome},
		{"edge", ExeEdge},
		{"firefox", ExeFirefox},
	}
	for _, tc := range cases {
		sample, dropped, err := Sanitize(ExtensionMessage{
			Type: MessageActive, Browser: tc.browser, URL: "https://github.com",
			WindowID: 1, TabID: 1, At: time.Now().UTC(),
		})
		if err != nil || dropped {
			t.Fatalf("%s: %v dropped=%v", tc.browser, err, dropped)
		}
		if sample.Executable != tc.exe {
			t.Fatalf("%s exe = %s want %s", tc.browser, sample.Executable, tc.exe)
		}
		if !IsBrowserExecutable(tc.exe) {
			t.Fatalf("%s should be supported", tc.exe)
		}
	}
	if IsBrowserExecutable("excel.exe") {
		t.Fatal("excel is not a browser")
	}
}

func TestForegroundGateDropsBackgroundBrowser(t *testing.T) {
	fg := Foreground{SessionID: "sess-1", ExecutableName: "EXCEL.EXE"}
	sample := Sample{Executable: ExeChrome, Domain: "github.com"}
	if AllowSample(fg, sample) {
		t.Fatal("must drop chrome sample while excel is focused")
	}
	fg.ExecutableName = ExeChrome
	if !AllowSample(fg, sample) {
		t.Fatal("chrome+chrome should pass")
	}
	if AllowSample(Foreground{SessionID: "sess-1", ExecutableName: ExeEdge}, sample) {
		t.Fatal("edge focused must drop chrome sample")
	}
}

func TestActiveTabCaptured(t *testing.T) {
	now := time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC)
	fg := Foreground{SessionID: "sess-1", ExecutableName: ExeChrome}
	sample := Sample{
		Executable: ExeChrome, Domain: "github.com", URL: "https://github.com",
		Title: "GitHub", WindowID: 2, TabID: 11, At: now,
	}
	result := step(persistedState{}, fg, &sample, now)
	if !result.Changed || len(result.Items) != 1 {
		t.Fatalf("result = %+v", result)
	}
	if result.Items[0].Domain != "github.com" || result.Items[0].TabID != 11 {
		t.Fatalf("item = %+v", result.Items[0])
	}
	if result.Items[0].StoppedAt != nil {
		t.Fatal("open row must not have stopped_at")
	}
}

func TestTabSwitchDetected(t *testing.T) {
	now := time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC)
	fg := Foreground{SessionID: "sess-1", ExecutableName: ExeChrome}
	first := Sample{Executable: ExeChrome, Domain: "github.com", URL: "https://github.com", WindowID: 1, TabID: 1, At: now}
	open := step(persistedState{}, fg, &first, now)
	later := now.Add(5 * time.Minute)
	second := Sample{Executable: ExeChrome, Domain: "office.com", URL: "https://office.com", WindowID: 1, TabID: 2, At: later}
	switched := step(open.Next, fg, &second, later)
	if len(switched.Items) != 2 {
		t.Fatalf("want close+open, got %d items: %+v", len(switched.Items), switched.Items)
	}
	if switched.Items[0].StoppedAt == nil || switched.Items[0].TabID != 1 {
		t.Fatalf("closed tab = %+v", switched.Items[0])
	}
	if switched.Items[1].TabID != 2 || switched.Items[1].Domain != "office.com" {
		t.Fatalf("opened tab = %+v", switched.Items[1])
	}
}

func TestBrowserSwitchDetected(t *testing.T) {
	now := time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC)
	chromeFG := Foreground{SessionID: "sess-1", ExecutableName: ExeChrome}
	first := Sample{Executable: ExeChrome, Domain: "github.com", URL: "https://github.com", WindowID: 1, TabID: 1, At: now}
	open := step(persistedState{}, chromeFG, &first, now)
	later := now.Add(10 * time.Minute)
	edgeFG := Foreground{SessionID: "sess-1", ExecutableName: ExeEdge}
	edge := Sample{Executable: ExeEdge, Domain: "youtube.com", URL: "https://youtube.com", WindowID: 9, TabID: 3, At: later}
	switched := step(open.Next, edgeFG, &edge, later)
	if len(switched.Items) != 2 {
		t.Fatalf("want close chrome + open edge, got %+v", switched.Items)
	}
	if NormalizeExecutable(switched.Items[0].ExecutableName) != ExeChrome {
		t.Fatalf("closed = %+v", switched.Items[0])
	}
	if NormalizeExecutable(switched.Items[1].ExecutableName) != ExeEdge {
		t.Fatalf("opened = %+v", switched.Items[1])
	}
}

func TestNativeMessagingRoundTrip(t *testing.T) {
	msg, _ := json.Marshal(ExtensionMessage{
		Type: MessageActive, Browser: "firefox", URL: "https://github.com/x?t=1",
		Title: "GitHub", WindowID: 4, TabID: 8, At: time.Now().UTC(),
	})
	var buf bytes.Buffer
	if err := WriteNativeMessage(&buf, msg); err != nil {
		t.Fatal(err)
	}
	raw, err := ReadNativeMessage(&buf)
	if err != nil {
		t.Fatal(err)
	}
	sample, dropped, err := ParseAndSanitize(raw)
	if err != nil || dropped {
		t.Fatalf("dropped=%v err=%v", dropped, err)
	}
	if sample.Executable != ExeFirefox || sample.Domain != "github.com" {
		t.Fatalf("sample = %+v", sample)
	}

	dir := t.TempDir()
	path := inboxPath(dir)
	if err := AppendInbox(path, sample); err != nil {
		t.Fatal(err)
	}
	got, err := ReadInbox(path)
	if err != nil || len(got) != 1 {
		t.Fatalf("inbox = %v err=%v", got, err)
	}
}

func TestExtensionPermissions(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	root := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	for _, rel := range []string{
		filepath.Join("browser-extension", "chromium", "manifest.json"),
		filepath.Join("browser-extension", "firefox", "manifest.json"),
	} {
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		if !strings.Contains(text, `"tabs"`) || !strings.Contains(text, `"nativeMessaging"`) {
			t.Fatalf("%s missing required permissions", rel)
		}
		for _, banned := range []string{`"cookies"`, `"clipboardRead"`, `"downloads"`, `"history"`, `"webRequest"`, `"<all_urls>"`} {
			if strings.Contains(text, banned) {
				t.Fatalf("%s must not request %s", rel, banned)
			}
		}
	}
	js, err := os.ReadFile(filepath.Join(root, "browser-extension", "chromium", "background.js"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(js), "active: true, lastFocusedWindow: true") {
		t.Fatal("extension must query only the active tab of the last focused window")
	}
}

func TestNativeHostManifests(t *testing.T) {
	dir := t.TempDir()
	chromePath, firefoxPath, err := writeNativeManifests(dir, `C:\RiskLens\RiskLensAgent.exe`, "abcdefghijklmnopabcdefghijklmnop", defaultFirefoxExtensionID)
	if err != nil {
		t.Fatal(err)
	}
	chrome, _ := os.ReadFile(chromePath)
	if !strings.Contains(string(chrome), "chrome-extension://abcdefghijklmnopabcdefghijklmnop/") {
		t.Fatalf("chromium manifest: %s", chrome)
	}
	if strings.Contains(string(chrome), "allowed_extensions") {
		t.Fatal("chromium manifest must not use firefox allowed_extensions")
	}
	ff, _ := os.ReadFile(firefoxPath)
	if !strings.Contains(string(ff), defaultFirefoxExtensionID) {
		t.Fatalf("firefox manifest: %s", ff)
	}
}

type stubIdentity struct {
	id idmodel.Identity
	ok bool
}

func (s stubIdentity) Current() (idmodel.Identity, bool) { return s.id, s.ok }

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
	return json.Unmarshal(resp.Body, target)
}

func TestUploaderUsesExistingTransport(t *testing.T) {
	now := time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	tx := &fakeTransport{
		resp: &response.Response{
			StatusCode: 200,
			Body: []byte(`{
				"accepted": true,
				"agent_id": "9b5d7f3e-1234-4abc-9def-0123456789ab",
				"stored": 1,
				"inventory_status": "stored",
				"server_version": "0.1.0",
				"server_time": "2026-08-21T10:00:00Z"
			}`),
		},
	}
	mod, err := NewWithDeps(Config{
		Enabled:  true,
		Interval: time.Hour,
		StateDir: dir,
	}, stubIdentity{id: idmodel.Identity{
		AgentID:   "9b5d7f3e-1234-4abc-9def-0123456789ab",
		InstallID: "install-1",
	}, ok: true}, tx, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	mod.WithClock(func() time.Time { return now })
	mod.WithForeground(func() (Foreground, bool) {
		return Foreground{SessionID: "11111111-1111-4111-8111-111111111111", ExecutableName: ExeChrome}, true
	})
	sample := Sample{
		Executable: ExeChrome, Domain: "github.com", URL: "https://github.com",
		WindowID: 1, TabID: 1, At: now,
	}
	if err := AppendInbox(inboxPath(dir), sample); err != nil {
		t.Fatal(err)
	}
	mod.cycle(context.Background())
	tx.mu.Lock()
	defer tx.mu.Unlock()
	if tx.calls != 1 {
		t.Fatalf("calls = %d", tx.calls)
	}
	if tx.lastReq.Path != BrowserPath {
		t.Fatalf("path = %s", tx.lastReq.Path)
	}
	upload, ok := tx.lastReq.Body.(BrowserUpload)
	if !ok || len(upload.Activities) == 0 {
		t.Fatalf("body = %+v", tx.lastReq.Body)
	}
	if upload.Activities[0].SessionID != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("session_id not reused: %+v", upload.Activities[0])
	}
	if upload.Activities[0].Domain != "github.com" {
		t.Fatalf("domain = %s", upload.Activities[0].Domain)
	}
}

func TestIsNativeHostInvocation(t *testing.T) {
	if !IsNativeHostInvocation([]string{"--browser-native-host"}) {
		t.Fatal("flag")
	}
	if !IsNativeHostInvocation([]string{"chrome-extension://abc/"}) {
		t.Fatal("chrome origin")
	}
	if !IsNativeHostInvocation([]string{defaultFirefoxExtensionID}) {
		t.Fatal("firefox id")
	}
	if IsNativeHostInvocation([]string{"--console"}) {
		t.Fatal("console is not native host")
	}
}
