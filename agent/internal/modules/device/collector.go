package device

import (
	"context"
	"strings"
	"time"
)

// Collector gathers static device inventory. Implementations must never dial
// the network. A partial result with an error is preferred over a total
// failure when at least one useful field was obtained.
type Collector interface {
	Collect(ctx context.Context) (Info, error)
}

// completeness rates how much of the required inventory is present. Used to
// decide between Collected, Partial, and Failed after a collection attempt.
func completeness(info Info) (present, total int) {
	checks := []bool{
		info.MachineName != "",
		info.Hostname != "",
		info.OperatingSystem != "",
		info.OSVersion != "",
		info.WindowsBuild != "",
		info.CPUModel != "",
		info.CPUCores > 0,
		info.MemoryBytes > 0,
		info.DiskSizeBytes > 0,
		info.DiskFreeBytes > 0 || info.DiskSizeBytes > 0, // free can be 0 on a full disk
		info.Architecture != "",
		info.Manufacturer != "" || info.Model != "" || info.SerialNumber != "",
		!info.BootTime.IsZero(),
		info.CurrentUser != "",
		info.AgentVersion != "",
	}
	total = len(checks)
	for _, ok := range checks {
		if ok {
			present++
		}
	}
	return present, total
}

// classifyState maps a collection result onto the inventory state machine.
func classifyState(info Info, err error) State {
	present, total := completeness(info)
	switch {
	case present == 0:
		return StateFailed
	case err != nil && present < total:
		return StatePartial
	case present < total:
		return StatePartial
	default:
		return StateCollected
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func truncateTime(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	return t.UTC().Truncate(time.Second)
}

// sanitizeHardwareField drops placeholder BIOS/SMBIOS strings that OEMs leave
// in firmware when a real value was never programmed.
func sanitizeHardwareField(value string) string {
	value = strings.TrimSpace(value)
	switch strings.ToLower(value) {
	case "", "none", "default string", "to be filled by o.e.m.", "to be filled by o.e.m",
		"system serial number", "system manufacturer", "system product name",
		"o.e.m", "oem", "n/a", "na":
		return ""
	default:
		return value
	}
}
