package device

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"risklens/agent/internal/modules/base"
	"risklens/agent/internal/modules/interfaces"
)

const (
	// ModuleName is the stable identifier used by the module registry.
	ModuleName = "device"

	// ModuleVersion is the device module's own semver.
	ModuleVersion = "1.0.0"
)

// Config captures settings the module may read from the Configuration
// Manager. The module never opens YAML directly.
type Config struct {
	// Enabled defaults to true when the zero value is used by callers that
	// construct Config{} intentionally for "run with defaults".
	Enabled bool
	// SkipStartCollect, when true, leaves collection to Pipeline.RunOnce.
	// Refresh is unchanged; Start simply does not call it.
	SkipStartCollect bool
}

// Clock is injectable for deterministic tests.
type Clock func() time.Time

// Module is the RiskLens Device Information Collector. It implements the
// modules/interfaces.Module contract and stores inventory in memory only.
type Module struct {
	*base.Module

	config    Config
	collector Collector
	logger    *slog.Logger
	clock     Clock

	mu               sync.RWMutex
	snapshot         Snapshot
	collectionHealth interfaces.Health
}

// New builds the device module with the platform Windows collector.
func New(cfg Config, agentVersion string, logger *slog.Logger) (*Module, error) {
	return NewWithCollector(cfg, &WindowsCollector{AgentVersion: agentVersion}, logger)
}

// NewWithCollector builds the module around a caller-supplied collector.
// Tests use this to inject fixtures without touching Windows APIs.
func NewWithCollector(cfg Config, collector Collector, logger *slog.Logger) (*Module, error) {
	if collector == nil {
		return nil, errors.New("collector is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Module{
		Module:           base.New(ModuleName, ModuleVersion),
		config:           cfg,
		collector:        collector,
		logger:           logger.With("module", ModuleName),
		clock:            time.Now,
		snapshot:         Snapshot{State: StateNotCollected},
		collectionHealth: interfaces.HealthUnknown,
	}, nil
}

// WithClock overrides the clock used for CollectedAt bookkeeping. Test-only.
func (m *Module) WithClock(clock Clock) *Module {
	if clock != nil {
		m.clock = clock
	}
	return m
}

// Start runs a single inventory collection. It never returns an error to the
// module manager: a failed collection marks the module Failed (or keeps it
// Running for Partial) and leaves the service alive.
//
// Production still collects once. Refresh exists so later pipeline ticks can
// recollect without changing this Start contract.
func (m *Module) Start(ctx context.Context) error {
	m.MarkStarting()
	m.logger.Info("Device inventory collection started")

	if !m.config.Enabled {
		m.setSnapshot(Snapshot{State: StateNotCollected})
		m.logger.Info("Device inventory collection disabled by configuration")
		m.MarkRunning()
		return nil
	}

	if m.config.SkipStartCollect {
		m.logger.Info("Device inventory collection deferred to pipeline")
		m.MarkRunning()
		m.SetHealth(interfaces.HealthUnknown)
		return nil
	}

	snap := m.refresh(ctx)

	switch snap.State {
	case StateCollected:
		m.logger.Info("Device inventory collected",
			"machine_name", snap.Info.MachineName,
			"hostname", snap.Info.Hostname,
			"os", snap.Info.OperatingSystem,
			"os_version", snap.Info.OSVersion,
			"build", snap.Info.WindowsBuild,
			"arch", snap.Info.Architecture,
			"cpu_cores", snap.Info.CPUCores,
			"memory_bytes", snap.Info.MemoryBytes,
			"manufacturer", snap.Info.Manufacturer,
			"model", snap.Info.Model,
		)
		m.MarkRunning()
	case StatePartial:
		m.logger.Warn("Device inventory partially collected",
			"machine_name", snap.Info.MachineName,
			"hostname", snap.Info.Hostname,
			"os", snap.Info.OperatingSystem,
			"error", snap.LastError,
		)
		m.MarkRunning()
		m.SetHealth(interfaces.HealthDegraded)
	default:
		m.logger.Error("Device inventory collection failed", "error", snap.LastError)
		m.MarkFailed()
	}
	return nil
}

// Refresh collects inventory again and replaces the in-memory snapshot.
// Each call runs a full collect; previous Info is discarded, not merged.
// Refresh updates collection health only. It does not schedule, upload, or
// change module Status.
func (m *Module) Refresh(ctx context.Context) Snapshot {
	if !m.config.Enabled {
		snap := Snapshot{State: StateNotCollected}
		m.setSnapshot(snap)
		m.setCollectionHealth(snap.CollectionHealth())
		return snap
	}
	return m.refresh(ctx)
}

func (m *Module) refresh(ctx context.Context) Snapshot {
	m.setSnapshot(Snapshot{State: StateCollecting, CollectedAt: m.now()})

	info, err := m.collector.Collect(ctx)
	if info.CollectedAt.IsZero() {
		info.CollectedAt = m.now()
	}

	state := classifyState(info, err)
	snap := Snapshot{
		State:       state,
		CollectedAt: info.CollectedAt,
		Info:        info,
	}
	if err != nil {
		snap.LastError = err.Error()
	}

	m.setSnapshot(snap)
	m.setCollectionHealth(snap.CollectionHealth())
	return snap
}

// Stop is a no-op: collection is synchronous and there is no background loop.
func (m *Module) Stop(ctx context.Context) error {
	m.MarkStopping()
	m.MarkStopped()
	return nil
}

// Inventory returns the current inventory snapshot.
func (m *Module) Inventory() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.snapshot
}

// Info returns the collected device information, or a zero Info when none
// has been collected yet. Other modules use this as the read API.
func (m *Module) Info() Info {
	return m.Inventory().Info
}

// CollectionHealth is collector-only. Upload/HTTP failures must not change it.
func (m *Module) CollectionHealth() interfaces.Health {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.collectionHealth
}

func (m *Module) setCollectionHealth(h interfaces.Health) {
	m.mu.Lock()
	m.collectionHealth = h
	m.mu.Unlock()
	if h != interfaces.HealthUnknown {
		m.SetHealth(h)
	}
}

// Ensure Module implements the framework interface at compile time.
var _ interfaces.Module = (*Module)(nil)

func (m *Module) now() time.Time {
	return truncateTime(m.clock())
}

func (m *Module) setSnapshot(s Snapshot) {
	m.mu.Lock()
	m.snapshot = s
	m.mu.Unlock()
}
