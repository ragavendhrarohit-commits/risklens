package heartbeat

import (
	"encoding/json"
	"time"
)

// SuccessResponse mirrors the backend heartbeat acceptance payload.
type SuccessResponse struct {
	Accepted        bool      `json:"accepted"`
	AgentID         string    `json:"agent_id"`
	HeartbeatStatus string    `json:"heartbeat_status"`
	ServerVersion   string    `json:"server_version"`
	ServerTime      time.Time `json:"server_time"`
	ReceivedAt      time.Time `json:"received_at"`
}

// ErrorEnvelope mirrors the platform error envelope.
type ErrorEnvelope struct {
	Error ErrorPayload `json:"error"`
}

type ErrorPayload struct {
	Code      string          `json:"code"`
	Message   string          `json:"message"`
	Details   json.RawMessage `json:"details,omitempty"`
	RequestID string          `json:"request_id,omitempty"`
}
