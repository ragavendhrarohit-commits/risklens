package manager

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"risklens/agent/internal/identity/generator"
	"risklens/agent/internal/identity/model"
	"risklens/agent/internal/identity/storage"
	"risklens/agent/internal/identity/validation"
)

type Manager struct {
	store  storage.Store
	info   generator.Info
	logger *slog.Logger

	mu      sync.RWMutex
	current *model.Identity
}

func New(store storage.Store, info generator.Info, logger *slog.Logger) (*Manager, error) {
	if store == nil {
		return nil, errors.New("identity store is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		store:  store,
		info:   info,
		logger: logger.With("component", "identity"),
	}, nil
}

func (m *Manager) Load() (model.Identity, error) {
	id, err := m.store.Load()
	if err != nil {
		return model.Identity{}, err
	}
	if err := m.Validate(id); err != nil {
		return model.Identity{}, err
	}
	refreshed := generator.ApplyInfo(id, m.info)
	if err := m.Validate(refreshed); err != nil {
		return model.Identity{}, err
	}
	if !refreshed.UpdatedAt.Equal(id.UpdatedAt) {
		if err := m.store.Save(refreshed); err != nil {
			return model.Identity{}, err
		}
	}
	m.setCurrent(refreshed)
	return refreshed, nil
}

func (m *Manager) Create() (model.Identity, error) {
	id, err := generator.NewIdentity(m.info)
	if err != nil {
		return model.Identity{}, err
	}
	if err := m.Validate(id); err != nil {
		return model.Identity{}, err
	}
	if err := m.store.Save(id); err != nil {
		return model.Identity{}, err
	}
	m.setCurrent(id)
	m.logger.Info("identity created", "agent_id", id.AgentID, "install_id", id.InstallID)
	return id, nil
}

func (m *Manager) Save(id model.Identity) error {
	if err := m.Validate(id); err != nil {
		return err
	}
	if err := m.store.Save(id); err != nil {
		return err
	}
	m.setCurrent(id)
	return nil
}

func (m *Manager) Reset() (model.Identity, error) {
	if err := m.store.Delete(); err != nil {
		return model.Identity{}, fmt.Errorf("reset identity: %w", err)
	}
	m.mu.Lock()
	m.current = nil
	m.mu.Unlock()
	m.logger.Info("identity reset")
	return m.Create()
}

func (m *Manager) Validate(id model.Identity) error {
	return validation.Validate(id)
}

func (m *Manager) Current() (model.Identity, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.current == nil {
		return model.Identity{}, false
	}
	return m.current.Clone(), true
}

func (m *Manager) LoadOrCreate() (model.Identity, error) {
	id, err := m.Load()
	if err == nil {
		m.logger.Info("identity loaded", "agent_id", id.AgentID, "install_id", id.InstallID)
		return id, nil
	}
	if !errors.Is(err, storage.ErrNotFound) {
		return model.Identity{}, err
	}
	return m.Create()
}

func (m *Manager) setCurrent(id model.Identity) {
	clone := id.Clone()
	m.mu.Lock()
	m.current = &clone
	m.mu.Unlock()
}
