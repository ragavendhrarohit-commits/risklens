package pipeline

// Health is per-collector status. It is not Windows service state
// and MUST NOT be used as Fleet Online/Offline.
type Health string

const (
	HealthHealthy  Health = "healthy"
	HealthWarning  Health = "warning"
	HealthError    Health = "error"
	HealthDisabled Health = "disabled"
)
