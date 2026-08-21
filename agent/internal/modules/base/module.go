package base

import (
	"sync"

	"risklens/agent/internal/modules/interfaces"
)

// Module is embeddable state for future collectors. It does not implement
// Start or Stop; those belong to each concrete module.
type Module struct {
	name    string
	version string

	mu     sync.RWMutex
	status interfaces.Status
	health interfaces.Health
}

func New(name, version string) *Module {
	return &Module{
		name:    name,
		version: version,
		status:  interfaces.StatusRegistered,
		health:  interfaces.HealthUnknown,
	}
}

func (m *Module) Name() string {
	return m.name
}

func (m *Module) Version() string {
	return m.version
}

func (m *Module) Status() interfaces.Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

func (m *Module) Health() interfaces.Health {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.health
}

func (m *Module) SetStatus(status interfaces.Status) {
	m.mu.Lock()
	m.status = status
	m.mu.Unlock()
}

func (m *Module) SetHealth(health interfaces.Health) {
	m.mu.Lock()
	m.health = health
	m.mu.Unlock()
}

func (m *Module) MarkStarting() {
	m.mu.Lock()
	m.status = interfaces.StatusStarting
	m.health = interfaces.HealthUnknown
	m.mu.Unlock()
}

func (m *Module) MarkRunning() {
	m.mu.Lock()
	m.status = interfaces.StatusRunning
	m.health = interfaces.HealthHealthy
	m.mu.Unlock()
}

func (m *Module) MarkStopping() {
	m.mu.Lock()
	m.status = interfaces.StatusStopping
	m.mu.Unlock()
}

func (m *Module) MarkStopped() {
	m.mu.Lock()
	m.status = interfaces.StatusStopped
	m.health = interfaces.HealthUnknown
	m.mu.Unlock()
}

func (m *Module) MarkFailed() {
	m.mu.Lock()
	m.status = interfaces.StatusFailed
	m.health = interfaces.HealthUnhealthy
	m.mu.Unlock()
}
