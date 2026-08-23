package heartbeat

import (
	"strings"
	"time"

	"risklens/agent/internal/identity/model"
	"risklens/agent/internal/modules/health"
	"risklens/agent/internal/modules/network"
)

// Request is the heartbeat payload. It never includes device inventory.
type Request struct {
	AgentID   string           `json:"agent_id"`
	InstallID string           `json:"install_id"`
	Timestamp time.Time        `json:"timestamp"`
	PublicIP  string           `json:"public_ip,omitempty"`
	PrivateIP string           `json:"private_ip,omitempty"`
	Hostname  string           `json:"hostname,omitempty"`
	Health    *health.Snapshot `json:"health,omitempty"`
}

// RequestFromIdentity builds a heartbeat from local identity + current UTC time.
// IDs are never generated here.
func RequestFromIdentity(id model.Identity, at time.Time, endpoint network.EndpointNetwork, snapshot health.Snapshot) Request {
	req := Request{
		AgentID:   id.AgentID,
		InstallID: id.InstallID,
		Timestamp: at.UTC(),
		PublicIP:  strings.TrimSpace(endpoint.PublicIP),
		PrivateIP: strings.TrimSpace(endpoint.PrivateIP),
		Hostname:  strings.TrimSpace(endpoint.Hostname),
	}
	if snapshot.Memory.Total > 0 || snapshot.CPUPercent > 0 || len(snapshot.Disks) > 0 || snapshot.CPUModel != "" {
		copySnap := snapshot
		req.Health = &copySnap
	}
	return req
}
