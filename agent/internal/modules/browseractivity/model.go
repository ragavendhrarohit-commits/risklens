package browseractivity

import (
	"crypto/sha1"
	"fmt"
	"strings"
	"time"
)

const (
	NativeHostName = "com.risklens.browser"
	MessageActive  = "active_tab"
)

// ExtensionMessage is the Native Messaging contract. The extension never
// talks to the backend. Extra fields are ignored and never persisted.
type ExtensionMessage struct {
	Type       string    `json:"type"`
	Browser    string    `json:"browser"`
	Executable string    `json:"executable,omitempty"`
	URL        string    `json:"url"`
	Title      string    `json:"title"`
	Profile    string    `json:"profile"`
	Incognito  bool      `json:"incognito"`
	WindowID   int64     `json:"window_id"`
	TabID      int64     `json:"tab_id"`
	At         time.Time `json:"at"`
}

// Sample is a sanitized active-tab observation ready for gating and storage.
type Sample struct {
	Browser    string    `json:"browser"`
	Executable string    `json:"executable"`
	Domain     string    `json:"domain"`
	URL        string    `json:"url"`
	Title      string    `json:"title"`
	Profile    string    `json:"profile"`
	WindowID   int64     `json:"window_id"`
	TabID      int64     `json:"tab_id"`
	At         time.Time `json:"at"`
}

// Foreground is the Application Activity clock: current session and exe.
type Foreground struct {
	SessionID      string
	ExecutableName string
}

// BrowserItem is one label segment in POST /api/v1/browser.
type BrowserItem struct {
	ID              string     `json:"id"`
	SessionID       string     `json:"session_id"`
	ExecutableName  string     `json:"executable_name"`
	Domain          string     `json:"domain"`
	URL             string     `json:"url"`
	Title           string     `json:"title"`
	Profile         string     `json:"profile"`
	WindowID        int64      `json:"window_id"`
	TabID           int64      `json:"tab_id"`
	StartedAt       time.Time  `json:"started_at"`
	StoppedAt       *time.Time `json:"stopped_at"`
	DurationSeconds int        `json:"duration_seconds"`
}

// BrowserUpload is POST /api/v1/browser.
type BrowserUpload struct {
	AgentID     string        `json:"agent_id"`
	InstallID   string        `json:"install_id"`
	CollectedAt time.Time     `json:"collected_at"`
	Activities  []BrowserItem `json:"activities"`
}

type persistedState struct {
	ID             string    `json:"id"`
	SessionID      string    `json:"session_id"`
	ExecutableName string    `json:"executable_name"`
	Domain         string    `json:"domain"`
	URL            string    `json:"url"`
	Title          string    `json:"title"`
	Profile        string    `json:"profile"`
	WindowID       int64     `json:"window_id"`
	TabID          int64     `json:"tab_id"`
	StartedAt      time.Time `json:"started_at"`
}

func (s persistedState) open() bool {
	return strings.TrimSpace(s.ID) != ""
}

func sameLabel(prev persistedState, sample Sample, sessionID string) bool {
	return prev.open() &&
		prev.SessionID == sessionID &&
		NormalizeExecutable(prev.ExecutableName) == NormalizeExecutable(sample.Executable) &&
		prev.Domain == sample.Domain &&
		prev.WindowID == sample.WindowID &&
		prev.TabID == sample.TabID
}

func durationSeconds(start, end time.Time) int {
	if start.IsZero() || end.Before(start) {
		return 0
	}
	n := int(end.Sub(start).Seconds())
	if n < 0 {
		return 0
	}
	return n
}

func browserActivityID(sessionID, executable, domain string, windowID, tabID int64, startedAt time.Time) string {
	started := startedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
	canonical := sessionID + "\x00" +
		NormalizeExecutable(executable) + "\x00" +
		strings.ToLower(strings.TrimSpace(domain)) + "\x00" +
		fmt.Sprintf("%d\x00%d\x00%s", windowID, tabID, started)
	sum := sha1.Sum([]byte(canonical))
	return fmt.Sprintf("%x", sum)
}
