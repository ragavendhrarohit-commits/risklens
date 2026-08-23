package heartbeat

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	idmodel "risklens/agent/internal/identity/model"
	"risklens/agent/internal/modules/base"
	"risklens/agent/internal/modules/health"
	"risklens/agent/internal/modules/interfaces"
	"risklens/agent/internal/modules/network"
	txinterfaces "risklens/agent/internal/transport/interfaces"
)

const (
	ModuleName    = "heartbeat"
	ModuleVersion = "1.0.0"

	DefaultInterval = 30 * time.Second
)

// IdentityProvider is the read-only view needed from the Identity Manager.
type IdentityProvider interface {
	Current() (idmodel.Identity, bool)
}

// Config is read from the Configuration Manager.
type Config struct {
	Enabled    bool
	BackendURL string
	Timeout    time.Duration
	Interval   time.Duration
}

type Clock func() time.Time

// Module periodically posts heartbeats using the Transport interface.
type Module struct {
	*base.Module

	identity IdentityProvider
	config   Config
	client   *Client
	logger   *slog.Logger
	clock    Clock

	scheduler *Scheduler

	mu       sync.RWMutex
	snapshot Snapshot
}

func New(cfg Config, identity IdentityProvider, agentVersion string, logger *slog.Logger) (*Module, error) {
	if identity == nil {
		return nil, errors.New("identity provider is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.Interval <= 0 {
		cfg.Interval = DefaultInterval
	}

	var client *Client
	if cfg.Enabled {
		c, err := NewClient(cfg.BackendURL, cfg.Timeout, agentVersion, logger)
		if err != nil {
			return nil, err
		}
		client = c
	}
	return newModule(cfg, identity, client, logger), nil
}

func NewWithTransport(
	cfg Config,
	identity IdentityProvider,
	transport txinterfaces.Transport,
	logger *slog.Logger,
) (*Module, error) {
	if identity == nil {
		return nil, errors.New("identity provider is required")
	}
	if transport == nil {
		return nil, errors.New("transport is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.Interval <= 0 {
		cfg.Interval = DefaultInterval
	}
	return newModule(cfg, identity, NewClientWithTransport(transport, logger), logger), nil
}

func newModule(cfg Config, identity IdentityProvider, client *Client, logger *slog.Logger) *Module {
	return &Module{
		Module:   base.New(ModuleName, ModuleVersion),
		identity: identity,
		config:   cfg,
		client:   client,
		logger:   logger.With("module", ModuleName),
		clock:    time.Now,
		snapshot: Snapshot{State: StateIdle},
	}
}

func (m *Module) WithClock(clock Clock) *Module {
	if clock != nil {
		m.clock = clock
	}
	return m
}

// Start begins the heartbeat scheduler. It never returns an error to the manager.
func (m *Module) Start(ctx context.Context) error {
	m.MarkStarting()
	m.logger.Info("Heartbeat started")

	if !m.config.Enabled {
		m.setSnapshot(Snapshot{State: StateSkipped, LastAttemptAt: m.now()})
		m.logger.Info("Heartbeat disabled by configuration")
		m.MarkRunning()
		return nil
	}

	if _, ok := m.identity.Current(); !ok {
		snap := Snapshot{State: StateFailed, LastAttemptAt: m.now(), LastError: "identity not available"}
		m.setSnapshot(snap)
		m.logger.Error("Heartbeat failed", "reason", "identity not available")
		m.MarkFailed()
		return nil
	}

	m.setSnapshot(Snapshot{State: StateRunning, LastAttemptAt: m.now()})
	m.scheduler = NewScheduler(m.config.Interval, m.sendOnce)
	m.scheduler.Start(ctx)
	m.logger.Info("Heartbeat scheduler running",
		"interval", m.config.Interval.String(),
		"backend_url", m.config.BackendURL,
	)
	m.MarkRunning()
	return nil
}

// Stop cancels the scheduler and waits for in-flight work to finish.
func (m *Module) Stop(ctx context.Context) error {
	m.MarkStopping()
	if m.scheduler != nil {
		m.scheduler.Stop()
		m.scheduler = nil
	}
	m.MarkStopped()
	return nil
}

// HeartbeatStatus returns the latest diagnostic snapshot.
func (m *Module) HeartbeatStatus() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.snapshot
}

func (m *Module) sendOnce(ctx context.Context) {
	id, ok := m.identity.Current()
	if !ok {
		m.updateSnapshot(func(s *Snapshot) {
			s.State = StateFailed
			s.LastAttemptAt = m.now()
			s.LastError = "identity not available"
		})
		m.logger.Error("Heartbeat skipped", "reason", "identity not available")
		return
	}

	req := RequestFromIdentity(id, m.now(), network.Collect(ctx), health.Collect())
	if m.logger != nil {
		body, _ := json.Marshal(req)
		m.logger.Info("REPRO Heartbeat payload", "payload", string(body))
	}
	m.updateSnapshot(func(s *Snapshot) {
		s.LastAttemptAt = m.now()
		s.LastError = ""
		s.ErrorCode = ""
	})

	result, err := m.client.Send(ctx, req)
	m.updateSnapshot(func(s *Snapshot) {
		s.LastAttemptAt = m.now()
		if result != nil {
			s.HTTPStatusCode = result.HTTPStatusCode
			s.ServerVersion = result.ServerVersion
			s.ServerTime = result.ServerTime
			s.RequestID = result.RequestID
			s.ErrorCode = result.ErrorCode
		}
		if err != nil {
			s.State = StateFailed
			s.LastError = err.Error()
			return
		}
		s.State = StateRunning
		s.LastSuccessAt = m.now()
		s.BeatsSent++
		s.LastError = ""
	})

	if err != nil {
		m.logger.Warn("Heartbeat failed",
			"agent_id", id.AgentID,
			"error", err,
		)
		return
	}
	m.logger.Info("Heartbeat success",
		"agent_id", id.AgentID,
		"http_status", result.HTTPStatusCode,
	)
}

var _ interfaces.Module = (*Module)(nil)

func (m *Module) now() time.Time {
	return m.clock().UTC()
}

func (m *Module) setSnapshot(s Snapshot) {
	m.mu.Lock()
	m.snapshot = s
	m.mu.Unlock()
}

func (m *Module) updateSnapshot(fn func(*Snapshot)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	fn(&m.snapshot)
}
