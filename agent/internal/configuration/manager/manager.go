package manager

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"risklens/agent/internal/configuration/model"
	"risklens/agent/internal/configuration/storage"
	"risklens/agent/internal/configuration/validation"
	"risklens/agent/internal/configuration/watcher"
)

type Listener func(cfg model.Config)

type Manager struct {
	store    storage.Store
	logger   *slog.Logger
	interval time.Duration

	mu        sync.RWMutex
	current   *model.Config
	listeners []Listener
}

func New(store storage.Store, logger *slog.Logger) (*Manager, error) {
	return NewWithInterval(store, logger, 0)
}

func NewWithInterval(store storage.Store, logger *slog.Logger, interval time.Duration) (*Manager, error) {
	if store == nil {
		return nil, errors.New("configuration store is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		store:    store,
		logger:   logger.With("component", "configuration"),
		interval: interval,
	}, nil
}

func (m *Manager) Load() (model.Config, error) {
	cfg, err := m.store.Load()
	if err != nil {
		return model.Config{}, err
	}
	applyEnv(&cfg)
	if err := m.Validate(&cfg); err != nil {
		return model.Config{}, err
	}
	m.setCurrent(cfg)
	return cfg.Clone(), nil
}

func (m *Manager) Save(cfg model.Config) error {
	if err := m.Validate(&cfg); err != nil {
		return err
	}
	if err := m.store.Save(cfg); err != nil {
		return err
	}
	m.setCurrent(cfg)
	return nil
}

func (m *Manager) Reload() (model.Config, error) {
	cfg, err := m.Load()
	if err != nil {
		return model.Config{}, err
	}
	m.notify(cfg)
	return cfg, nil
}

func (m *Manager) Validate(cfg *model.Config) error {
	return validation.Validate(cfg)
}

func (m *Manager) Current() (model.Config, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.current == nil {
		return model.Config{}, false
	}
	return m.current.Clone(), true
}

func (m *Manager) Subscribe(fn Listener) (unsubscribe func()) {
	if fn == nil {
		return func() {}
	}
	m.mu.Lock()
	m.listeners = append(m.listeners, fn)
	idx := len(m.listeners) - 1
	m.mu.Unlock()
	return func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if idx >= 0 && idx < len(m.listeners) {
			m.listeners[idx] = nil
		}
	}
}

func (m *Manager) Watch(ctx context.Context) error {
	if ctx == nil {
		return errors.New("context is required")
	}
	w := watcher.New(m.store.Path(), m.interval)
	go func() {
		_ = w.Run(ctx)
	}()
	go m.consume(ctx, w.Events())
	return nil
}

func (m *Manager) consume(ctx context.Context, events <-chan struct{}) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-events:
			cfg, err := m.Load()
			if err != nil {
				m.logger.Warn("configuration reload skipped", "error", err)
				continue
			}
			m.logger.Info("configuration reloaded", "path", cfg.Path)
			m.notify(cfg)
		}
	}
}

func (m *Manager) setCurrent(cfg model.Config) {
	clone := cfg.Clone()
	m.mu.Lock()
	m.current = &clone
	m.mu.Unlock()
}

func (m *Manager) notify(cfg model.Config) {
	m.mu.RLock()
	listeners := append([]Listener(nil), m.listeners...)
	m.mu.RUnlock()
	snapshot := cfg.Clone()
	for _, fn := range listeners {
		if fn != nil {
			fn(snapshot)
		}
	}
}

func applyEnv(cfg *model.Config) {
	if v := os.Getenv(model.EnvLogLevel); v != "" {
		cfg.Logging.Level = v
	}
	if v := os.Getenv(model.EnvLogFile); v != "" {
		cfg.Logging.File = v
	}
	if v := os.Getenv(model.EnvShutdownTimeout); v != "" {
		cfg.General.ShutdownTimeoutRaw = v
	}
	if v := os.Getenv(model.EnvBackendURL); v != "" {
		cfg.Networking.BackendURL = v
	}
	if v := os.Getenv(model.EnvRegistrationEnabled); v != "" {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "on":
			cfg.Registration.Enabled = true
		case "0", "false", "no", "off":
			cfg.Registration.Enabled = false
		}
	}
	if v := os.Getenv(model.EnvRegistrationTimeout); v != "" {
		cfg.Registration.TimeoutRaw = v
	}
	if v := os.Getenv(model.EnvDeviceUploadEnabled); v != "" {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "on":
			cfg.DeviceUpload.Enabled = true
		case "0", "false", "no", "off":
			cfg.DeviceUpload.Enabled = false
		}
	}
	if v := os.Getenv(model.EnvDeviceUploadTimeout); v != "" {
		cfg.DeviceUpload.TimeoutRaw = v
	}
	if v := os.Getenv(model.EnvDeviceUploadInterval); v != "" {
		cfg.DeviceUpload.IntervalRaw = v
	}
	if v := os.Getenv(model.EnvHeartbeatEnabled); v != "" {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "on":
			cfg.Heartbeat.Enabled = true
		case "0", "false", "no", "off":
			cfg.Heartbeat.Enabled = false
		}
	}
	if v := os.Getenv(model.EnvHeartbeatTimeout); v != "" {
		cfg.Heartbeat.TimeoutRaw = v
	}
	if v := os.Getenv(model.EnvHeartbeatInterval); v != "" {
		cfg.Heartbeat.IntervalRaw = v
	}
	if v := os.Getenv(model.EnvUsersEnabled); v != "" {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "on":
			cfg.Users.Enabled = true
		case "0", "false", "no", "off":
			cfg.Users.Enabled = false
		}
	}
	if v := os.Getenv(model.EnvUsersTimeout); v != "" {
		cfg.Users.TimeoutRaw = v
	}
	if v := os.Getenv(model.EnvUsersInterval); v != "" {
		cfg.Users.IntervalRaw = v
	}
	if v := os.Getenv(model.EnvSoftwareEnabled); v != "" {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "on":
			cfg.Software.Enabled = true
		case "0", "false", "no", "off":
			cfg.Software.Enabled = false
		}
	}
	if v := os.Getenv(model.EnvSoftwareTimeout); v != "" {
		cfg.Software.TimeoutRaw = v
	}
	if v := os.Getenv(model.EnvSoftwareInterval); v != "" {
		cfg.Software.IntervalRaw = v
	}
	if v := os.Getenv(model.EnvActivityEnabled); v != "" {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "on":
			cfg.Activity.Enabled = true
		case "0", "false", "no", "off":
			cfg.Activity.Enabled = false
		}
	}
	if v := os.Getenv(model.EnvActivityTimeout); v != "" {
		cfg.Activity.TimeoutRaw = v
	}
	if v := os.Getenv(model.EnvActivityInterval); v != "" {
		cfg.Activity.IntervalRaw = v
	}
}

func ResolvePath(explicit string) string {
	path := explicit
	if path == "" {
		path = os.Getenv(model.EnvConfig)
	}
	if path == "" {
		path = model.DefaultPath()
	}
	if !filepath.IsAbs(path) {
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
	}
	return filepath.Clean(path)
}
