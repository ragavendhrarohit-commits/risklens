package heartbeat

import (
	"time"

	"risklens/agent/internal/identity/model"
)

// Request is the heartbeat payload. It never includes device inventory.
type Request struct {
	AgentID   string    `json:"agent_id"`
	InstallID string    `json:"install_id"`
	Timestamp time.Time `json:"timestamp"`
}

// RequestFromIdentity builds a heartbeat from local identity + current UTC time.
// IDs are never generated here.
func RequestFromIdentity(id model.Identity, at time.Time) Request {
	return Request{
		AgentID:   id.AgentID,
		InstallID: id.InstallID,
		Timestamp: at.UTC(),
	}
}
