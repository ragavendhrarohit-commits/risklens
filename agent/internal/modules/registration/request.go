package registration

import (
	"time"

	"risklens/agent/internal/identity/model"
)

// Request mirrors the backend AgentRegistrationRequest schema exactly.
// Field ordering and JSON tags must match backend/app/schemas/agents.py.
type Request struct {
	AgentID         string    `json:"agent_id"`
	InstallID       string    `json:"install_id"`
	AgentVersion    string    `json:"agent_version"`
	MachineName     string    `json:"machine_name"`
	OperatingSystem string    `json:"operating_system"`
	DisplayName     string    `json:"display_name,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// RequestFromIdentity builds a registration request from the loaded local
// identity. The module never generates ids; it only forwards them.
func RequestFromIdentity(id model.Identity) Request {
	return Request{
		AgentID:         id.AgentID,
		InstallID:       id.InstallID,
		AgentVersion:    id.AgentVersion,
		MachineName:     id.MachineName,
		OperatingSystem: id.OperatingSystem,
		DisplayName:     id.DisplayName,
		CreatedAt:       id.CreatedAt.UTC(),
	}
}
