package users

import (
	"time"
)

// Account is one local Windows user account snapshot.
type Account struct {
	Username         string     `json:"username"`
	SID              string     `json:"sid"`
	DisplayName      string     `json:"display_name"`
	AccountType      string     `json:"account_type"`
	Enabled          bool       `json:"enabled"`
	Builtin          bool       `json:"builtin"`
	PasswordRequired bool       `json:"password_required"`
	LastLogon        *time.Time `json:"last_logon,omitempty"`
	Source           string     `json:"source"`
}

// Inventory is the collected set of local accounts for one agent.
type Inventory struct {
	AgentID     string    `json:"agent_id"`
	InstallID   string    `json:"install_id"`
	CollectedAt time.Time `json:"collected_at"`
	Users       []Account `json:"users"`
}

const (
	SourceLocal        = "Local"
	AccountTypeLocal   = "Local"
	AccountTypeAdmin   = "Administrator"
	AccountTypeGuest   = "Guest"
	AccountTypeUnknown = "Unknown"
)
