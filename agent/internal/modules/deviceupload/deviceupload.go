package deviceupload

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	idmodel "risklens/agent/internal/identity/model"
	"risklens/agent/internal/modules/base"
	"risklens/agent/internal/modules/device"
	"risklens/agent/internal/modules/interfaces"
	"risklens/agent/internal/pipeline"
	txinterfaces "risklens/agent/internal/transport/interfaces"
)

const (
	ModuleName    = "deviceupload"
	ModuleVersion = "1.0.0"
)

// IdentityProvider is the read-only view needed from the Identity Manager.
type IdentityProvider interface {
	Current() (idmodel.Identity, bool)
}

// InventorySource is the Device Module surface needed to host the pipeline.
type InventorySource interface {
	device.InventoryRefresher
	Inventory() device.Snapshot
	Info() device.Info
	CollectionHealth() interfaces.Health
}

// Config is read from the Configuration Manager.
type Config struct {
	Enabled    bool
	BackendURL string
	Timeout    time.Duration
	// Interval is the Pipeline scheduler period. Zero means 5 minutes.
	// YAML validation enforces 1m–1h; tests may inject a shorter value.
	Interval time.Duration
}

type Clock func() time.Time

// Module uploads the Device Module's inventory to the backend.
type Module struct {
	*base.Module

	identity  IdentityProvider
	inventory InventorySource
	config    Config
	client    *Client
	host      *PipelineHost
	logger    *slog.Logger
	clock     Clock

	mu           sync.RWMutex
	snapshot     Snapshot
	uploadHealth interfaces.Health
}

func New(
	cfg Config,
	identity IdentityProvider,
	inventory InventorySource,
	agentVersion string,
	logger *slog.Logger,
) (*Module, error) {
	if identity == nil {
		return nil, errors.New("identity provider is required")
	}
	if inventory == nil {
		return nil, errors.New("inventory source is required")
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
	return newModule(cfg, identity, inventory, client, logger), nil
}

func NewWithTransport(
	cfg Config,
	identity IdentityProvider,
	inventory InventorySource,
	transport txinterfaces.Transport,
	logger *slog.Logger,
) (*Module, error) {
	if identity == nil {
		return nil, errors.New("identity provider is required")
	}
	if inventory == nil {
		return nil, errors.New("inventory source is required")
	}
	if transport == nil {
		return nil, errors.New("transport is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return newModule(cfg, identity, inventory, NewClientWithTransport(transport, logger), logger), nil
}

func newModule(
	cfg Config,
	identity IdentityProvider,
	inventory InventorySource,
	client *Client,
	logger *slog.Logger,
) *Module {
	return &Module{
		Module:       base.New(ModuleName, ModuleVersion),
		identity:     identity,
		inventory:    inventory,
		config:       cfg,
		client:       client,
		logger:       logger.With("module", ModuleName),
		clock:        time.Now,
		snapshot:     Snapshot{State: StateNotUploaded},
		uploadHealth: interfaces.HealthUnknown,
	}
}

func (m *Module) WithClock(clock Clock) *Module {
	if clock != nil {
		m.clock = clock
	}
	return m
}

// Start launches the Device Pipeline scheduler. The first tick runs immediately.
// It never returns an error to the module manager.
func (m *Module) Start(ctx context.Context) error {
	m.MarkStarting()
	m.logger.Info("Device inventory upload started")

	if !m.config.Enabled {
		m.setSnapshot(Snapshot{State: StateSkipped, LastAttemptAt: m.now()})
		m.logger.Info("Device inventory upload disabled by configuration")
		m.MarkRunning()
		return nil
	}

	id, ok := m.identity.Current()
	if !ok {
		snap := Snapshot{State: StateFailed, LastAttemptAt: m.now(), LastError: "identity not available"}
		m.setSnapshot(snap)
		m.logger.Error("Device inventory upload failed", "reason", "identity not available")
		m.MarkFailed()
		return nil
	}

	if m.client == nil {
		snap := Snapshot{State: StateFailed, LastAttemptAt: m.now(), LastError: "upload client not available"}
		m.setSnapshot(snap)
		m.MarkFailed()
		return nil
	}

	interval := m.config.Interval
	if interval <= 0 {
		interval = 5 * time.Minute
	}

	host, err := NewPipelineHost(
		pipeline.Config{Interval: interval, Timeout: m.config.Timeout},
		m.inventory,
		m.identity,
		m.client,
		m.logger,
		m,
	)
	if err != nil {
		snap := Snapshot{State: StateFailed, LastAttemptAt: m.now(), LastError: err.Error()}
		m.setSnapshot(snap)
		m.logger.Error("Device inventory pipeline host failed", "error", err)
		m.MarkFailed()
		return nil
	}
	m.host = host

	m.setSnapshot(Snapshot{State: StateUploading, LastAttemptAt: m.now()})
	m.logger.Info("Device inventory pipeline started",
		"agent_id", id.AgentID,
		"interval", interval.String(),
		"backend_url", m.config.BackendURL,
	)

	m.MarkRunning()
	host.Start(ctx)
	return nil
}

// CollectionHealth is the Device collector's health. HTTP failures do not change it.
func (m *Module) CollectionHealth() interfaces.Health {
	if m == nil || m.inventory == nil {
		return interfaces.HealthUnknown
	}
	return m.inventory.CollectionHealth()
}

// UploadHealth is upload/HTTP health only.
func (m *Module) UploadHealth() interfaces.Health {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.uploadHealth
}

// OnCollect records collector health after a pipeline collect. Upload is unchanged.
func (m *Module) OnCollect() {
	m.recomputeOverall()
}

// OnUpload records upload health. Collection success plus HTTP failure is
// Warning (degraded overall), not a failed collector.
func (m *Module) OnUpload(err error) {
	h := interfaces.HealthHealthy
	if err != nil {
		h = interfaces.HealthDegraded
	}
	m.mu.Lock()
	m.uploadHealth = h
	m.mu.Unlock()
	m.recomputeOverall()
}

func (m *Module) recomputeOverall() {
	collect := m.CollectionHealth()
	upload := m.UploadHealth()
	overall := worstHealth(collect, upload)
	if overall != interfaces.HealthUnknown {
		m.SetHealth(overall)
	}
}

func worstHealth(a, b interfaces.Health) interfaces.Health {
	rank := func(h interfaces.Health) int {
		switch h {
		case interfaces.HealthUnhealthy:
			return 3
		case interfaces.HealthDegraded:
			return 2
		case interfaces.HealthHealthy:
			return 1
		default:
			return 0
		}
	}
	if rank(a) >= rank(b) {
		return a
	}
	return b
}

func (m *Module) Stop(ctx context.Context) error {
	m.MarkStopping()
	if m.host != nil {
		m.host.Stop()
		m.host = nil
	}
	m.MarkStopped()
	return nil
}

func (m *Module) UploadStatus() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.snapshot
}

var _ interfaces.Module = (*Module)(nil)
var _ HealthObserver = (*Module)(nil)

func (m *Module) now() time.Time {
	return m.clock().UTC()
}

func (m *Module) setSnapshot(s Snapshot) {
	m.mu.Lock()
	m.snapshot = s
	m.mu.Unlock()
}
