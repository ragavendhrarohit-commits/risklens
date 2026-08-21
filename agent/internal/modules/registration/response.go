package registration

import (
	"encoding/json"
	"time"
)

// SuccessResponse mirrors the backend AgentRegistrationResponse.
type SuccessResponse struct {
	Accepted           bool      `json:"accepted"`
	AgentID            string    `json:"agent_id"`
	RegistrationStatus string    `json:"registration_status"`
	ServerVersion      string    `json:"server_version"`
	ServerTime         time.Time `json:"server_time"`
}

// ErrorEnvelope mirrors the backend error envelope produced by
// app/api/errors.py: `{"error": {"code","message","details","request_id"}}`.
type ErrorEnvelope struct {
	Error ErrorPayload `json:"error"`
}

type ErrorPayload struct {
	Code      string          `json:"code"`
	Message   string          `json:"message"`
	Details   json.RawMessage `json:"details,omitempty"`
	RequestID string          `json:"request_id,omitempty"`
}
