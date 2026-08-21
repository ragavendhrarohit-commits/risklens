package deviceupload

import (
	"time"

	"risklens/agent/internal/modules/device"
)

// Request mirrors the backend DeviceInventoryRequest schema.
type Request struct {
	AgentID         string     `json:"agent_id"`
	InstallID       string     `json:"install_id"`
	MachineName     string     `json:"machine_name"`
	Hostname        string     `json:"hostname"`
	OperatingSystem string     `json:"operating_system"`
	OSVersion       string     `json:"os_version"`
	WindowsBuild    string     `json:"windows_build"`
	Architecture    string     `json:"architecture"`
	CPUModel        string     `json:"cpu_model"`
	CPUCores        int        `json:"cpu_cores"`
	MemoryBytes     uint64     `json:"memory_bytes"`
	DiskTotalBytes  uint64     `json:"disk_total_bytes"`
	DiskFreeBytes   uint64     `json:"disk_free_bytes"`
	Manufacturer    string     `json:"manufacturer"`
	Model           string     `json:"model"`
	SerialNumber    string     `json:"serial_number"`
	BootTime        *time.Time `json:"boot_time,omitempty"`
	CurrentUser     string     `json:"current_user"`
	AgentVersion    string     `json:"agent_version"`
	CollectedAt     time.Time  `json:"collected_at"`
}

// RequestFromInventory builds an upload payload from identity + device inventory.
// It never recollects hardware data.
func RequestFromInventory(agentID, installID string, info device.Info) Request {
	req := Request{
		AgentID:         agentID,
		InstallID:       installID,
		MachineName:     firstNonEmpty(info.MachineName, info.Hostname),
		Hostname:        firstNonEmpty(info.Hostname, info.MachineName),
		OperatingSystem: info.OperatingSystem,
		OSVersion:       info.OSVersion,
		WindowsBuild:    info.WindowsBuild,
		Architecture:    info.Architecture,
		CPUModel:        info.CPUModel,
		CPUCores:        info.CPUCores,
		MemoryBytes:     info.MemoryBytes,
		DiskTotalBytes:  info.DiskSizeBytes,
		DiskFreeBytes:   info.DiskFreeBytes,
		Manufacturer:    info.Manufacturer,
		Model:           info.Model,
		SerialNumber:    info.SerialNumber,
		CurrentUser:     info.CurrentUser,
		AgentVersion:    info.AgentVersion,
		CollectedAt:     info.CollectedAt.UTC(),
	}
	if !info.BootTime.IsZero() {
		boot := info.BootTime.UTC()
		req.BootTime = &boot
	}
	if req.CollectedAt.IsZero() {
		req.CollectedAt = time.Now().UTC()
	}
	return req
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
