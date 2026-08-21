package activity

import (
	"context"
	"encoding/json"
	"time"
)

// Native WTS session-change codes (wtsapi32 WM_WTSSESSION_CHANGE / SERVICE_CONTROL_SESSIONCHANGE).
const (
	WTSConsoleConnect    = 0x1
	WTSConsoleDisconnect = 0x2
	WTSRemoteConnect     = 0x3
	WTSRemoteDisconnect  = 0x4
	WTSSessionLogon      = 0x5
	WTSSessionLogoff     = 0x6
	WTSSessionLock       = 0x7
	WTSSessionUnlock     = 0x8
	WTSSessionTerminate  = 0xB
)

const (
	EventLogon             = "logon"
	EventLogoff            = "logoff"
	EventLock              = "lock"
	EventUnlock            = "unlock"
	EventRemoteConnect     = "remote_connect"
	EventRemoteDisconnect  = "remote_disconnect"
	EventConsoleConnect    = "console_connect"
	EventConsoleDisconnect = "console_disconnect"
	EventShutdown          = "shutdown"
)

func eventName(eventType uint32) string {
	switch eventType {
	case WTSConsoleConnect:
		return EventConsoleConnect
	case WTSConsoleDisconnect:
		return EventConsoleDisconnect
	case WTSRemoteConnect:
		return EventRemoteConnect
	case WTSRemoteDisconnect:
		return EventRemoteDisconnect
	case WTSSessionLogon:
		return EventLogon
	case WTSSessionLogoff:
		return EventLogoff
	case WTSSessionLock:
		return EventLock
	case WTSSessionUnlock:
		return EventUnlock
	case WTSSessionTerminate:
		return EventLogoff
	default:
		return ""
	}
}

// OnWindowsSessionChange is the service-layer callback for SERVICE_CONTROL_SESSIONCHANGE.
func (m *Module) OnWindowsSessionChange(eventType uint32, sessionID uint32) {
	m.handleSessionChange("scm", eventType, sessionID)
}

// OnWindowsShutdown flushes open sessions on SERVICE_CONTROL_SHUTDOWN / PRESHUTDOWN.
// It persists the close locally first, then best-effort uploads. It must not
// be used for a normal agent/service restart (that is not a user logout).
func (m *Module) OnWindowsShutdown(ctx context.Context) {
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
	}
	m.recordEvent(EventShutdown, 0, "")
	_ = m.CloseSession(ctx, "shutdown")
}

// HandleSessionChange applies one native Windows session notification.
func (m *Module) HandleSessionChange(eventType uint32, wtsSessionID uint32) {
	m.handleSessionChange("wts", eventType, wtsSessionID)
}

func (m *Module) handleSessionChange(source string, eventType uint32, wtsSessionID uint32) {
	if m == nil || !m.config.Enabled {
		return
	}
	name := eventName(eventType)
	if name == "" {
		if m.logger != nil {
			m.logger.Info("Session notification received",
				"source", source,
				"event_type", eventType,
				"wts_session_id", wtsSessionID,
			)
		}
		return
	}
	sid := ""
	var stateJSON []byte
	m.mu.RLock()
	if m.state.hasSession() {
		sid = m.state.SessionID
	}
	stateJSON, _ = json.Marshal(m.state)
	m.mu.RUnlock()
	if m.logger != nil {
		m.logger.Info("Session notification received",
			"source", source,
			"event", name,
			"event_type", eventType,
			"wts_session_id", wtsSessionID,
			"session_id", sid,
		)
	}
	m.recordEvent(name, wtsSessionID, sid)

	switch eventType {
	case WTSSessionLogon:
		if m.logger != nil {
			m.logger.Info("LOGIN DETECTED")
			m.logger.Info("Incoming WTS session", "wts_session_id", wtsSessionID)
			m.logger.Info("Current session_id from activity.json", "session_id", sid)
			m.logger.Info("Current active session in memory", "session_id", sid)
			m.logger.Info("activity.json", "contents", string(stateJSON))
		}
		m.cycleNow()
	case WTSSessionLogoff, WTSSessionTerminate:
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		_ = m.CloseSession(ctx, name)
	case WTSRemoteConnect, WTSConsoleConnect:
		m.cycleNow()
	case WTSSessionLock, WTSSessionUnlock, WTSRemoteDisconnect, WTSConsoleDisconnect:
		// Timeline only. The Windows session remains active.
	}
}

// CloseSession is the single logoff/shutdown close path.
func (m *Module) CloseSession(ctx context.Context, reason string) error {
	if m != nil && m.logger != nil {
		m.mu.RLock()
		sid := m.state.SessionID
		logon := m.state.LogonAt
		m.mu.RUnlock()
		m.logger.Info("CloseSession invoked",
			"reason", reason,
			"session_id", sid,
			"logon_at", logon,
			"logout_at", m.now(),
		)
	}
	return m.FlushClose(ctx)
}

func (m *Module) cycleNow() {
	timeout := m.config.Timeout
	if timeout <= 0 {
		timeout = DefaultInterval
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	m.cycle(ctx)
}

func hasClosedSession(result stepResult) bool {
	for _, item := range result.Sessions {
		if item.Status == StatusClosed {
			return true
		}
	}
	return false
}
