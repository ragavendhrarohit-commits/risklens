package health

// Snapshot is live endpoint resource usage collected with each heartbeat.
type Snapshot struct {
	CPUPercent    float64      `json:"cpu_percent"`
	CPUModel      string       `json:"cpu_model,omitempty"`
	LogicalCPUs   int          `json:"logical_cpus,omitempty"`
	PhysicalCores int          `json:"physical_cores,omitempty"`
	Memory        MemoryStats  `json:"memory"`
	Disks         []DiskStats  `json:"disks"`
}

// MemoryStats is physical RAM usage at collection time.
type MemoryStats struct {
	Total   uint64  `json:"total"`
	Used    uint64  `json:"used"`
	Free    uint64  `json:"free"`
	Percent float64 `json:"percent"`
}

// DiskStats is one fixed local volume (for example C:).
type DiskStats struct {
	Name    string  `json:"name"`
	Total   uint64  `json:"total"`
	Used    uint64  `json:"used"`
	Free    uint64  `json:"free"`
	Percent float64 `json:"percent"`
}

// Collect gathers live health metrics for the current platform.
func Collect() Snapshot {
	return collect()
}
