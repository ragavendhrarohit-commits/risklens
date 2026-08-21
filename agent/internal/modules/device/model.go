package device

import "time"

// Info is the static inventory snapshot for the local Windows computer.
// Fields that could not be collected are left empty (or zero) rather than
// inventing values. Consumers must treat empty strings and zero numbers as
// "unavailable", not as meaningful zeros.
type Info struct {
	MachineName     string
	Hostname        string
	OperatingSystem string
	OSVersion       string
	WindowsBuild    string
	CPUModel        string
	CPUCores        int
	MemoryBytes     uint64
	DiskSizeBytes   uint64
	DiskFreeBytes   uint64
	Architecture    string
	SerialNumber    string
	Manufacturer    string
	Model           string
	BootTime        time.Time
	CurrentUser     string
	AgentVersion    string

	// CollectedAt is when this inventory sample was taken.
	CollectedAt time.Time
}

// Comparable returns inventory facts used for fingerprinting.
// CollectedAt is a sample clock, not an OS fact, so it is cleared.
func (i Info) Comparable() Info {
	out := i
	out.CollectedAt = time.Time{}
	return out
}

// ForTransport returns the inventory sent to the backend.
// If CollectedAt is unset, sampleTime from the pipeline envelope is used.
func (i Info) ForTransport(sampleTime time.Time) Info {
	out := i
	if out.CollectedAt.IsZero() && !sampleTime.IsZero() {
		out.CollectedAt = sampleTime.UTC()
	}
	return out
}

// MemoryGiB returns installed memory in gibibytes, or 0 when unknown.
func (i Info) MemoryGiB() float64 {
	if i.MemoryBytes == 0 {
		return 0
	}
	return float64(i.MemoryBytes) / (1024 * 1024 * 1024)
}

// DiskSizeGiB returns total disk capacity in gibibytes, or 0 when unknown.
func (i Info) DiskSizeGiB() float64 {
	if i.DiskSizeBytes == 0 {
		return 0
	}
	return float64(i.DiskSizeBytes) / (1024 * 1024 * 1024)
}

// DiskFreeGiB returns free disk space in gibibytes, or 0 when unknown.
func (i Info) DiskFreeGiB() float64 {
	if i.DiskFreeBytes == 0 {
		return 0
	}
	return float64(i.DiskFreeBytes) / (1024 * 1024 * 1024)
}

// IsEmpty reports whether no inventory fields were collected at all.
func (i Info) IsEmpty() bool {
	return i.MachineName == "" &&
		i.Hostname == "" &&
		i.OperatingSystem == "" &&
		i.OSVersion == "" &&
		i.WindowsBuild == "" &&
		i.CPUModel == "" &&
		i.CPUCores == 0 &&
		i.MemoryBytes == 0 &&
		i.DiskSizeBytes == 0 &&
		i.DiskFreeBytes == 0 &&
		i.Architecture == "" &&
		i.SerialNumber == "" &&
		i.Manufacturer == "" &&
		i.Model == "" &&
		i.BootTime.IsZero() &&
		i.CurrentUser == "" &&
		i.AgentVersion == ""
}
