package software

import "time"

// Application is one installed Windows application entry.
type Application struct {
	Name             string `json:"application_name"`
	Version          string `json:"version"`
	Publisher        string `json:"publisher"`
	InstallDate      string `json:"install_date"`
	InstallLocation  string `json:"install_location"`
	EstimatedSize    int64  `json:"estimated_size"`
	Architecture     string `json:"architecture"`
	SystemComponent  bool   `json:"system_component"`
	WindowsComponent bool   `json:"windows_component"`
	RegistryPath     string `json:"registry_path"`
}

// Inventory is the collected set of installed applications for one agent.
type Inventory struct {
	AgentID      string        `json:"agent_id"`
	InstallID    string        `json:"install_id"`
	CollectedAt  time.Time     `json:"collected_at"`
	Applications []Application `json:"applications"`
}

const (
	ArchitectureX64 = "x64"
	ArchitectureX86 = "x86"
)

// DedupKey identifies a unique application entry within one agent's inventory.
// The backend upserts by (agent_id, application_name, version); the agent-side
// dedupe key mirrors that so we do not upload two rows that would collapse.
func (a Application) DedupKey() string {
	return a.Name + "\x00" + a.Version
}
