package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	cfgmodel "risklens/agent/internal/configuration/model"
	"risklens/agent/internal/identity/model"
	"risklens/agent/internal/modules/interfaces"
	"risklens/agent/internal/modules/manager"
	"risklens/agent/internal/modules/registry"
)

// Identity is a read-only view. App never opens identity storage.
type Identity interface {
	Current() (model.Identity, bool)
}

// Configuration is a read-only view. App never opens configuration files.
type Configuration interface {
	Current() (cfgmodel.Config, bool)
}

var ErrShutdownTimeout = errors.New("shutdown deadline exceeded")

type App struct {
	shutdownTimeout time.Duration
	logger          *slog.Logger
	manager         *manager.Manager
	identity        Identity
	configuration   Configuration
}

func New(shutdownTimeout time.Duration, logger *slog.Logger, modules ...interfaces.Module) (*App, error) {
	if logger == nil {
		return nil, errors.New("logger is required")
	}
	if shutdownTimeout <= 0 {
		shutdownTimeout = 15 * time.Second
	}

	reg := registry.New()
	for _, module := range modules {
		if err := reg.Register(module); err != nil {
			return nil, err
		}
	}

	return &App{
		shutdownTimeout: shutdownTimeout,
		logger:          logger.With("component", "app"),
		manager:         manager.New(reg, logger),
	}, nil
}

func (a *App) UseIdentity(identity Identity) {
	if a != nil {
		a.identity = identity
	}
}

func (a *App) UseConfiguration(configuration Configuration) {
	if a != nil {
		a.configuration = configuration
	}
}

func (a *App) Run(ctx context.Context) error {
	if ctx == nil {
		return errors.New("context is required")
	}
	if a.identity != nil {
		if current, ok := a.identity.Current(); ok {
			a.logger.Info("Service started", "agent_id", current.AgentID, "install_id", current.InstallID)
		} else {
			a.logger.Info("Service started")
		}
	} else {
		a.logger.Info("Service started")
	}

	if err := a.manager.StartAll(ctx); err != nil {
		return err
	}

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), a.shutdownTimeout)
	defer cancel()
	return a.Shutdown(shutdownCtx)
}

func (a *App) Shutdown(ctx context.Context) error {
	a.logger.Info("Service stopping")

	var err error
	if a.manager != nil {
		err = a.manager.StopAll(ctx)
	}
	if ctx != nil && ctx.Err() != nil {
		err = errors.Join(fmt.Errorf("%w: %w", ErrShutdownTimeout, ctx.Err()), err)
	}

	a.logger.Info("Service stopped")
	return err
}

func (a *App) ModuleStatus() []manager.Snapshot {
	if a == nil || a.manager == nil {
		return nil
	}
	return a.manager.Status()
}
