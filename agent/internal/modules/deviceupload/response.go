package deviceupload

import (
	"encoding/json"
	"time"
)

// SuccessResponse mirrors the backend DeviceInventoryResponse.
type SuccessResponse struct {
	Accepted        bool      `json:"accepted"`
	AgentID         string    `json:"agent_id"`
	Created         bool      `json:"created"`
	InventoryStatus string    `json:"inventory_status"`
	ServerVersion   string    `json:"server_version"`
	ServerTime      time.Time `json:"server_time"`
	ReceivedAt      time.Time `json:"received_at"`
}

// ErrorEnvelope mirrors the backend error envelope.
type ErrorEnvelope struct {
	Error ErrorPayload `json:"error"`
}

type ErrorPayload struct {
	Code      string          `json:"code"`
	Message   string          `json:"message"`
	Details   json.RawMessage `json:"details,omitempty"`
	RequestID string          `json:"request_id,omitempty"`
}
