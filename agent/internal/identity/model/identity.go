package model

import "time"

const SchemaVersion = 1

type Status string

const (
	StatusActive Status = "active"
)

// Identity is the durable local agent record. It must never contain
// secrets, tokens, passwords, or network endpoints.
type Identity struct {
	SchemaVersion   int       `json:"schema_version"`
	AgentID         string    `json:"agent_id"`
	InstallID       string    `json:"install_id"`
	AgentVersion    string    `json:"agent_version"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	Status          Status    `json:"status"`
	DisplayName     string    `json:"display_name"`
	MachineName     string    `json:"machine_name"`
	OperatingSystem string    `json:"operating_system"`
}

func (id Identity) Clone() Identity {
	return id
}
