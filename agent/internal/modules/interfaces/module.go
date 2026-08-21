package interfaces

import "context"

type Status string

const (
	StatusRegistered Status = "registered"
	StatusStarting   Status = "starting"
	StatusRunning    Status = "running"
	StatusStopping   Status = "stopping"
	StatusStopped    Status = "stopped"
	StatusFailed     Status = "failed"
)

type Health string

const (
	HealthUnknown   Health = "unknown"
	HealthHealthy   Health = "healthy"
	HealthDegraded  Health = "degraded"
	HealthUnhealthy Health = "unhealthy"
)

// Module is the contract every RiskLens agent component must implement.
// Sprint 2 provides only this framework. Collectors are not implemented here.
type Module interface {
	Name() string
	Version() string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Status() Status
	Health() Health
}
