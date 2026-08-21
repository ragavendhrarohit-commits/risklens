package activity

import (
	"encoding/json"
	"os"
	"strings"
)

func (m *Module) repro(msg string, args ...any) {
	if m == nil || m.logger == nil {
		return
	}
	m.logger.Info("REPRO "+msg, args...)
}

func jsonDump(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return err.Error()
	}
	return string(data)
}

func (m *Module) activityFileJSON() string {
	if m == nil || strings.TrimSpace(m.stateFile) == "" {
		return ""
	}
	data, err := os.ReadFile(m.stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return "{}"
		}
		return "UNREADABLE: " + err.Error()
	}
	return string(data)
}

func sessionPayloadKind(items []SessionItem) string {
	hasClosed := false
	hasActive := false
	for _, item := range items {
		switch item.Status {
		case StatusClosed:
			hasClosed = true
		default:
			hasActive = true
		}
	}
	switch {
	case hasClosed && hasActive:
		return "LOGIN_CLOSE_THEN_OPEN"
	case hasClosed:
		return "SHUTDOWN"
	case hasActive:
		return "HEARTBEAT_OR_LOGIN_OPEN"
	default:
		return "EMPTY"
	}
}
