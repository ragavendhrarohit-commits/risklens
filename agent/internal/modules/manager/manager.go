package manager

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"risklens/agent/internal/modules/interfaces"
	"risklens/agent/internal/modules/registry"
)

var (
	ErrAlreadyStarted = errors.New("modules already started")
	ErrNilContext     = errors.New("context is required")
)

const rollbackTimeout = 5 * time.Second

type Snapshot struct {
	Name    string
	Version string
	Status  interfaces.Status
	Health  interfaces.Health
}

type Manager struct {
	registry *registry.Registry
	logger   *slog.Logger

	mu      sync.Mutex
	started []interfaces.Module
	running bool
}

func New(reg *registry.Registry, logger *slog.Logger) *Manager {
	if reg == nil {
		reg = registry.New()
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		registry: reg,
		logger:   logger.With("component", "modules"),
	}
}

func (m *Manager) StartAll(ctx context.Context) error {
	if ctx == nil {
		return ErrNilContext
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	modules := m.registry.Modules()
	for _, module := range modules {
		if err := registry.Validate(module); err != nil {
			return err
		}
	}

	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return ErrAlreadyStarted
	}
	m.mu.Unlock()

	started := make([]interfaces.Module, 0, len(modules))
	for _, module := range modules {
		if err := ctx.Err(); err != nil {
			m.rollback(started)
			return err
		}

		m.logger.Info("module starting", "module", module.Name(), "version", module.Version())
		if err := module.Start(ctx); err != nil {
			m.logger.Error("module start failed", "module", module.Name(), "error", err)
			m.rollback(started)
			return fmt.Errorf("start %s: %w", module.Name(), err)
		}
		m.logger.Info("module started", "module", module.Name(), "version", module.Version())
		started = append(started, module)
	}

	m.mu.Lock()
	m.started = started
	m.running = true
	m.mu.Unlock()
	return nil
}

func (m *Manager) StopAll(ctx context.Context) error {
	if ctx == nil {
		return ErrNilContext
	}

	m.mu.Lock()
	started := append([]interfaces.Module(nil), m.started...)
	m.mu.Unlock()

	err := m.stopStarted(ctx, started)

	m.mu.Lock()
	m.started = nil
	m.running = false
	m.mu.Unlock()
	return err
}

func (m *Manager) Status() []Snapshot {
	modules := m.registry.Modules()
	out := make([]Snapshot, 0, len(modules))
	for _, module := range modules {
		out = append(out, Snapshot{
			Name:    module.Name(),
			Version: module.Version(),
			Status:  module.Status(),
			Health:  module.Health(),
		})
	}
	return out
}

func (m *Manager) rollback(started []interfaces.Module) {
	ctx, cancel := context.WithTimeout(context.Background(), rollbackTimeout)
	defer cancel()
	_ = m.stopStarted(ctx, started)
}

func (m *Manager) stopStarted(ctx context.Context, started []interfaces.Module) error {
	var errs []error
	for i := len(started) - 1; i >= 0; i-- {
		module := started[i]
		m.logger.Info("module stopping", "module", module.Name())
		if err := module.Stop(ctx); err != nil {
			m.logger.Error("module stop failed", "module", module.Name(), "error", err)
			errs = append(errs, fmt.Errorf("stop %s: %w", module.Name(), err))
			continue
		}
		m.logger.Info("module stopped", "module", module.Name())
	}
	return errors.Join(errs...)
}
