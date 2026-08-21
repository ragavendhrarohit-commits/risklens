package registration

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	idmodel "risklens/agent/internal/identity/model"
	"risklens/agent/internal/modules/base"
	"risklens/agent/internal/modules/interfaces"
	txinterfaces "risklens/agent/internal/transport/interfaces"
)

const (
	// ModuleName is the stable identifier used by the module registry.
	ModuleName = "registration"

	// ModuleVersion is the registration module's own semver, independent of
	// the agent version reported inside the request payload.
	ModuleVersion = "1.0.0"
)

// IdentityProvider is the read-only view the module needs from the Identity
// Manager. The module never opens identity storage.
type IdentityProvider interface {
	Current() (idmodel.Identity, bool)
}

// Config captures the settings the module reads from the Configuration
// Manager. The module never opens YAML directly.
type Config struct {
	Enabled    bool
	BackendURL string
	Timeout    time.Duration
}

// Clock is injectable for deterministic tests.
type Clock func() time.Time

// Module is the RiskLens Agent Registration Module. It implements the
// modules/interfaces.Module contract.
type Module struct {
	*base.Module

	identity IdentityProvider
	config   Config
	client   *Client
	logger   *slog.Logger
	clock    Clock

	mu       sync.RWMutex
	snapshot Snapshot
}

// New builds the module. When registration is disabled it still returns a
// working module that reports state=NotRegistered on Start; callers do not
// have to conditionally omit the module.
//
// When Enabled=true but BackendURL is empty or malformed, New returns an
// error so the caller can log and skip. This is the fail-loud path for
// operator misconfiguration.
func New(cfg Config, identity IdentityProvider, agentVersion string, logger *slog.Logger) (*Module, error) {
	if identity == nil {
		return nil, errors.New("identity provider is required")
	}
	if logger == nil {
		logger = slog.Default()
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

// NewWithTransport builds the module around a caller-supplied transport
// instead of letting it construct its own. This is how the agent will share
// one transport across Registration, Heartbeat, and Event Upload once those
// modules exist, and how tests exercise the module without a socket.
func NewWithTransport(cfg Config, identity IdentityProvider, transport txinterfaces.Transport, logger *slog.Logger) (*Module, error) {
	if identity == nil {
		return nil, errors.New("identity provider is required")
	}
	if transport == nil {
		return nil, errors.New("transport is required")
	}
	if logger == nil {
		logger = slog.Default()
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
		snapshot: Snapshot{State: StateNotRegistered},
	}
}

// WithClock overrides the clock used for LastAttemptAt. Test-only.
func (m *Module) WithClock(clock Clock) *Module {
	if clock != nil {
		m.clock = clock
	}
	return m
}

// Start runs a single registration attempt. It never returns an error: a
// failed registration marks the module Failed and keeps the service alive.
// This matches the "Do not crash the service" requirement.
func (m *Module) Start(ctx context.Context) error {
	m.MarkStarting()
	m.logger.Info("Registration started")

	if !m.config.Enabled {
		m.setSnapshot(Snapshot{State: StateNotRegistered, LastAttemptAt: m.now()})
		m.logger.Info("Registration disabled by configuration")
		m.MarkRunning()
		return nil
	}

	id, ok := m.identity.Current()
	if !ok {
		snap := Snapshot{State: StateFailed, LastAttemptAt: m.now(), LastError: "identity not available"}
		m.setSnapshot(snap)
		m.logger.Error("Registration failed", "reason", "identity not available")
		m.MarkFailed()
		return nil
	}

	m.setSnapshot(Snapshot{State: StateRegistering, LastAttemptAt: m.now()})
	m.logger.Info("Registration in progress",
		"agent_id", id.AgentID,
		"install_id", id.InstallID,
		"agent_version", id.AgentVersion,
		"backend_url", m.config.BackendURL,
	)

	req := RequestFromIdentity(id)
	result, err := m.client.Register(ctx, req)

	snap := Snapshot{LastAttemptAt: m.now()}
	if result != nil {
		snap.HTTPStatusCode = result.HTTPStatusCode
		snap.ServerVersion = result.ServerVersion
		snap.ServerTime = result.ServerTime
		snap.RequestID = result.RequestID
		snap.ErrorCode = result.ErrorCode
	}

	if err == nil {
		snap.State = StateRegistered
		m.setSnapshot(snap)
		m.logger.Info("Registration success",
			"agent_id", id.AgentID,
			"server_version", snap.ServerVersion,
			"server_time", snap.ServerTime,
			"http_status", snap.HTTPStatusCode,
		)
		m.MarkRunning()
		return nil
	}

	// 409 means the backend already knows this agent; from the agent's
	// perspective that is a successful registered state.
	if errors.Is(err, ErrDuplicate) {
		snap.State = StateRegistered
		m.setSnapshot(snap)
		m.logger.Info("Registration success (already registered)",
			"agent_id", id.AgentID,
			"http_status", snap.HTTPStatusCode,
			"error_code", snap.ErrorCode,
		)
		m.MarkRunning()
		return nil
	}

	snap.State = StateFailed
	snap.LastError = err.Error()
	m.setSnapshot(snap)
	m.logger.Error("Registration failed",
		"agent_id", id.AgentID,
		"http_status", snap.HTTPStatusCode,
		"error_code", snap.ErrorCode,
		"error", err,
	)
	m.MarkFailed()
	return nil
}

// Stop is a no-op for this module today; there is no background loop yet.
// The synchronous Start attempt is already bounded by the client timeout
// and the parent context.
func (m *Module) Stop(ctx context.Context) error {
	m.MarkStopping()
	m.MarkStopped()
	return nil
}

// RegistrationStatus returns the current registration snapshot.
func (m *Module) RegistrationStatus() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.snapshot
}

// Ensure Module implements the framework interface at compile time.
var _ interfaces.Module = (*Module)(nil)

func (m *Module) now() time.Time {
	return m.clock().UTC()
}

func (m *Module) setSnapshot(s Snapshot) {
	m.mu.Lock()
	m.snapshot = s
	m.mu.Unlock()
}
