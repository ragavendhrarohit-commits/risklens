package activity

import (
	"crypto/sha1"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

const (
	StatusActive = "active"
	StatusClosed = "closed"
)

// LiveSession is the current interactive Windows session, if any.
type LiveSession struct {
	WTSSessionID uint32
	Username     string
	AccountSID   string
	LogonAt      time.Time
}

// LiveApp is the current foreground window, if any.
type LiveApp struct {
	ApplicationName string
	ExecutableName  string
	WindowTitle     string
}

// Observation is one poll of WTS + foreground state.
type Observation struct {
	Session *LiveSession
	App     *LiveApp
}

// SessionItem is one computer session in the upload payload.
type SessionItem struct {
	SessionID  string     `json:"session_id"`
	Username   string     `json:"username"`
	AccountSID string     `json:"account_sid"`
	LogonAt    time.Time  `json:"logon_at"`
	LogoffAt   *time.Time `json:"logoff_at"`
	Status     string     `json:"status"`
}

// ActivityItem is one application activity segment in the upload payload.
type ActivityItem struct {
	ID              string     `json:"id"`
	SessionID       string     `json:"session_id"`
	ApplicationName string     `json:"application_name"`
	ExecutableName  string     `json:"executable_name"`
	WindowTitle     string     `json:"window_title"`
	StartedAt       time.Time  `json:"started_at"`
	StoppedAt       *time.Time `json:"stopped_at"`
	DurationSeconds int        `json:"duration_seconds"`
}

// SessionUpload is POST /api/v1/sessions.
type SessionUpload struct {
	AgentID     string        `json:"agent_id"`
	InstallID   string        `json:"install_id"`
	CollectedAt time.Time     `json:"collected_at"`
	Sessions    []SessionItem `json:"sessions"`
}

// ApplicationUpload is POST /api/v1/applications.
type ApplicationUpload struct {
	AgentID     string         `json:"agent_id"`
	InstallID   string         `json:"install_id"`
	CollectedAt time.Time      `json:"collected_at"`
	Activities  []ActivityItem `json:"activities"`
}

// persistedState is written to %ProgramData%\RiskLens\state\activity.json.
type persistedState struct {
	SessionID       string    `json:"session_id"`
	WTSSessionID    uint32    `json:"wts_session_id"`
	Username        string    `json:"username"`
	AccountSID      string    `json:"account_sid"`
	LogonAt         time.Time `json:"logon_at"`
	AppID           string    `json:"app_id"`
	ApplicationName string    `json:"application_name"`
	ExecutableName  string    `json:"executable_name"`
	WindowTitle     string    `json:"window_title"`
	StartedAt       time.Time `json:"started_at"`
	LastClosedSessionID string    `json:"last_closed_session_id,omitempty"`
	LastClosedLogonAt   time.Time `json:"last_closed_logon_at,omitempty"`
	UploadedClosedIDs   []string  `json:"uploaded_closed_ids,omitempty"`
}

const maxUploadedClosedIDs = 32

func (s persistedState) hasSession() bool {
	return strings.TrimSpace(s.SessionID) != ""
}

func (s persistedState) hasApp() bool {
	return strings.TrimSpace(s.AppID) != ""
}

func (s persistedState) alreadyUploadedClosed(sessionID string) bool {
	id := strings.TrimSpace(sessionID)
	if id == "" {
		return false
	}
	for _, existing := range s.UploadedClosedIDs {
		if existing == id {
			return true
		}
	}
	return false
}

func (s persistedState) alreadyClosedLocally(sessionID string) bool {
	id := strings.TrimSpace(sessionID)
	if id == "" {
		return false
	}
	if id == strings.TrimSpace(s.LastClosedSessionID) {
		return true
	}
	return s.alreadyUploadedClosed(id)
}

func rememberClosed(prev persistedState, sessionID string, logonAt time.Time) persistedState {
	next := persistedState{
		LastClosedSessionID: sessionID,
		LastClosedLogonAt:   logonAt,
		UploadedClosedIDs:   append([]string(nil), prev.UploadedClosedIDs...),
	}
	if strings.TrimSpace(next.LastClosedSessionID) == "" {
		next.LastClosedSessionID = prev.LastClosedSessionID
		next.LastClosedLogonAt = prev.LastClosedLogonAt
	}
	return next
}

func withUploadedClosed(state persistedState, ids ...string) persistedState {
	next := state
	next.UploadedClosedIDs = append([]string(nil), state.UploadedClosedIDs...)
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || next.alreadyUploadedClosed(id) {
			continue
		}
		next.UploadedClosedIDs = append(next.UploadedClosedIDs, id)
	}
	if len(next.UploadedClosedIDs) > maxUploadedClosedIDs {
		next.UploadedClosedIDs = next.UploadedClosedIDs[len(next.UploadedClosedIDs)-maxUploadedClosedIDs:]
	}
	return next
}

func preserveClosedMeta(cur, next persistedState) persistedState {
	if strings.TrimSpace(next.LastClosedSessionID) == "" {
		next.LastClosedSessionID = cur.LastClosedSessionID
		next.LastClosedLogonAt = cur.LastClosedLogonAt
	}
	next = withUploadedClosed(next, cur.UploadedClosedIDs...)
	return sanitizeState(next)
}

func sanitizeState(state persistedState) persistedState {
	if state.hasSession() && state.alreadyClosedLocally(state.SessionID) {
		return rememberClosed(state, state.SessionID, state.LogonAt)
	}
	return state
}

func closedSessionIDs(items []SessionItem) []string {
	var ids []string
	for _, item := range items {
		if item.Status == StatusClosed && strings.TrimSpace(item.SessionID) != "" {
			ids = append(ids, item.SessionID)
		}
	}
	return ids
}

func filterUnsentSessions(state persistedState, items []SessionItem) []SessionItem {
	if len(items) == 0 {
		return items
	}
	out := make([]SessionItem, 0, len(items))
	for _, item := range items {
		if state.alreadyUploadedClosed(item.SessionID) {
			continue
		}
		if item.Status != StatusClosed && state.alreadyClosedLocally(item.SessionID) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func applicationName(exe, title string) string {
	base := filepath.Base(strings.TrimSpace(exe))
	name := strings.TrimSuffix(base, filepath.Ext(base))
	if name != "" {
		return name
	}
	if strings.TrimSpace(title) != "" {
		return strings.TrimSpace(title)
	}
	return "Unknown"
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

func activityID(sessionID, executable string, startedAt time.Time) string {
	started := startedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
	canonical := sessionID + "\x00" + strings.ToLower(strings.TrimSpace(executable)) + "\x00" + started
	sum := sha1.Sum([]byte(canonical))
	return fmt.Sprintf("%x", sum)
}

// risklensNamespace is a fixed UUID used as the v5 namespace.
var risklensNamespace = [16]byte{
	0x6b, 0xa7, 0xb8, 0x12, 0x9d, 0xad, 0x11, 0xd1,
	0x80, 0xb4, 0x00, 0xc0, 0x4f, 0xd4, 0x30, 0xc8,
}

func sessionID(agentID string, wtsSessionID uint32, logonAt time.Time) string {
	name := fmt.Sprintf("%s:%d:%s", agentID, wtsSessionID, logonAt.UTC().Format(time.RFC3339))
	return uuidV5(risklensNamespace, name)
}

func uuidV5(namespace [16]byte, name string) string {
	h := sha1.New()
	_, _ = h.Write(namespace[:])
	_, _ = h.Write([]byte(name))
	sum := h.Sum(nil)
	var u [16]byte
	copy(u[:], sum[:16])
	u[6] = (u[6] & 0x0f) | 0x50
	u[8] = (u[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", u[0:4], u[4:6], u[6:8], u[8:10], u[10:])
}
