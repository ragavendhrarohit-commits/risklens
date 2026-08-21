package activity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	eventsFileName = "session_events.json"
	maxEvents      = 200
)

// SessionEvent is a local timeline record for Windows session-change notifications.
// It is not a Fleet API payload (Sprint 13 timeline remains the existing stub).
type SessionEvent struct {
	Type         string    `json:"type"`
	WTSSessionID uint32    `json:"wts_session_id"`
	SessionID    string    `json:"session_id,omitempty"`
	At           time.Time `json:"at"`
}

func eventsPath(stateDir string) string {
	if strings.TrimSpace(stateDir) == "" {
		return filepath.Join(os.TempDir(), "RiskLens", eventsFileName)
	}
	return filepath.Join(stateDir, eventsFileName)
}

func loadEvents(path string) []SessionEvent {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var events []SessionEvent
	if err := json.Unmarshal(data, &events); err != nil {
		return nil
	}
	return events
}

func saveEvents(path string, events []SessionEvent) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if len(events) > maxEvents {
		events = events[len(events)-maxEvents:]
	}
	data, err := json.MarshalIndent(events, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (m *Module) recordEvent(eventType string, wtsSessionID uint32, sessionID string) {
	if m == nil || strings.TrimSpace(eventType) == "" {
		return
	}
	event := SessionEvent{
		Type:         eventType,
		WTSSessionID: wtsSessionID,
		SessionID:    sessionID,
		At:           m.now(),
	}
	m.mu.Lock()
	m.events = append(m.events, event)
	if len(m.events) > maxEvents {
		m.events = append([]SessionEvent(nil), m.events[len(m.events)-maxEvents:]...)
	}
	snapshot := append([]SessionEvent(nil), m.events...)
	path := m.eventsFile
	m.mu.Unlock()
	if err := saveEvents(path, snapshot); err != nil && m.logger != nil {
		m.logger.Warn("Session event file write failed", "error", err)
	}
}

// SessionEvents returns a copy of locally recorded session-change events.
func (m *Module) SessionEvents() []SessionEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]SessionEvent, len(m.events))
	copy(out, m.events)
	return out
}

func (m *Module) lastEventType() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.events) == 0 {
		return ""
	}
	return m.events[len(m.events)-1].Type
}
