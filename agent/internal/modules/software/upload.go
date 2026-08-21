package software

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	idmodel "risklens/agent/internal/identity/model"
	"risklens/agent/internal/modules/base"
	"risklens/agent/internal/modules/interfaces"
	txclient "risklens/agent/internal/transport/client"
	txerrors "risklens/agent/internal/transport/errors"
	txinterfaces "risklens/agent/internal/transport/interfaces"
	"risklens/agent/internal/transport/request"
	"risklens/agent/internal/transport/response"
)

const (
	ModuleName      = "software"
	ModuleVersion   = "1.0.0"
	EndpointPath    = "/api/v1/software"
	userAgent       = "RiskLensAgent"
	DefaultInterval = 15 * time.Minute
)

var (
	ErrUnknownAgent      = errors.New("unknown agent")
	ErrValidation        = errors.New("request validation error")
	ErrServer            = errors.New("server error")
	ErrUnexpectedStatus  = errors.New("unexpected status")
	ErrMalformedResponse = errors.New("malformed response")
	ErrTransport         = errors.New("transport error")
)

// IdentityProvider is the read-only view from the Identity Manager.
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

// UploadRequest is the POST body for /api/v1/software.
type UploadRequest = Inventory

// SuccessResponse mirrors the backend upload acknowledgement.
type SuccessResponse struct {
	Accepted        bool      `json:"accepted"`
	AgentID         string    `json:"agent_id"`
	Stored          int       `json:"stored"`
	InventoryStatus string    `json:"inventory_status"`
	ServerVersion   string    `json:"server_version"`
	ServerTime      time.Time `json:"server_time"`
}

type errorEnvelope struct {
	Error struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
	} `json:"error"`
}

// Result captures diagnostics from one upload attempt.
type Result struct {
	HTTPStatusCode int
	ServerVersion  string
	ServerTime     time.Time
	RequestID      string
	ErrorCode      string
	ErrorMessage   string
	Stored         int
}

// Client uploads software inventory through Transport only.
type Client struct {
	transport txinterfaces.Transport
	logger    *slog.Logger
}

func NewClient(baseURL string, timeout time.Duration, agentVersion string, logger *slog.Logger) (*Client, error) {
	if logger == nil {
		logger = slog.Default()
	}
	ua := userAgent
	if agentVersion != "" {
		ua = userAgent + "/" + agentVersion
	}
	transport, err := txclient.New(txclient.Config{
		BaseURL:   baseURL,
		Timeout:   timeout,
		UserAgent: ua,
		Logger:    logger,
	})
	if err != nil {
		return nil, err
	}
	return NewClientWithTransport(transport, logger), nil
}

func NewClientWithTransport(transport txinterfaces.Transport, logger *slog.Logger) *Client {
	if logger == nil {
		logger = slog.Default()
	}
	return &Client{
		transport: transport,
		logger:    logger.With("component", "software.client"),
	}
}

// Upload POSTs the collected inventory to /api/v1/software.
func (c *Client) Upload(ctx context.Context, req UploadRequest) (*Result, error) {
	result := &Result{}
	txReq, err := c.transport.BuildRequest(request.MethodPOST, EndpointPath, req)
	if err != nil {
		return result, fmt.Errorf("build request: %w", err)
	}
	resp, err := c.transport.Send(ctx, txReq)
	if err != nil {
		return result, translateTransportError(err)
	}
	result.HTTPStatusCode = resp.StatusCode

	switch resp.StatusCode {
	case response.StatusOK, response.StatusCreated, response.StatusAccepted:
		var success SuccessResponse
		if err := c.transport.DecodeResponse(resp, &success); err != nil {
			return result, fmt.Errorf("%w: %v", ErrMalformedResponse, err)
		}
		if !success.Accepted || success.InventoryStatus != "stored" {
			return result, fmt.Errorf("%w: server did not accept software inventory", ErrMalformedResponse)
		}
		result.ServerVersion = success.ServerVersion
		result.ServerTime = success.ServerTime
		result.Stored = success.Stored
		return result, nil
	case response.StatusNotFound:
		parseErrorEnvelope(resp.Body, result)
		return result, ErrUnknownAgent
	case response.StatusBadRequest, response.StatusUnprocessableEntity:
		parseErrorEnvelope(resp.Body, result)
		return result, fmt.Errorf("%w: %s", ErrValidation, safeMessage(result))
	default:
		parseErrorEnvelope(resp.Body, result)
		if resp.IsServerError() {
			return result, fmt.Errorf("%w: HTTP %d %s", ErrServer, resp.StatusCode, safeMessage(result))
		}
		return result, fmt.Errorf("%w: HTTP %d %s", ErrUnexpectedStatus, resp.StatusCode, safeMessage(result))
	}
}

func translateTransportError(err error) error {
	switch {
	case errors.Is(err, txerrors.ErrTimeout):
		return fmt.Errorf("%w: timeout: %v", ErrTransport, err)
	case errors.Is(err, txerrors.ErrInvalidResponse), errors.Is(err, txerrors.ErrDecode):
		return fmt.Errorf("%w: %v", ErrMalformedResponse, err)
	default:
		return fmt.Errorf("%w: %v", ErrTransport, err)
	}
}

func parseErrorEnvelope(body []byte, result *Result) {
	var envelope errorEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return
	}
	result.ErrorCode = envelope.Error.Code
	result.ErrorMessage = envelope.Error.Message
	result.RequestID = envelope.Error.RequestID
}

func safeMessage(result *Result) string {
	if result.ErrorMessage != "" {
		return result.ErrorMessage
	}
	if result.ErrorCode != "" {
		return result.ErrorCode
	}
	return ""
}

// Module collects installed applications on a schedule and uploads them.
type Module struct {
	*base.Module

	identity  IdentityProvider
	collector Collector
	config    Config
	client    *Client
	logger    *slog.Logger
	clock     Clock

	scheduler *scheduler

	mu       sync.RWMutex
	snapshot Snapshot
	last     []Application
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
	return newModule(cfg, identity, &WindowsCollector{}, client, logger), nil
}

func NewWithDeps(
	cfg Config,
	identity IdentityProvider,
	collector Collector,
	transport txinterfaces.Transport,
	logger *slog.Logger,
) (*Module, error) {
	if identity == nil {
		return nil, errors.New("identity provider is required")
	}
	if collector == nil {
		return nil, errors.New("collector is required")
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
	return newModule(cfg, identity, collector, NewClientWithTransport(transport, logger), logger), nil
}

func newModule(cfg Config, identity IdentityProvider, collector Collector, client *Client, logger *slog.Logger) *Module {
	return &Module{
		Module:    base.New(ModuleName, ModuleVersion),
		identity:  identity,
		collector: collector,
		config:    cfg,
		client:    client,
		logger:    logger.With("module", ModuleName),
		clock:     time.Now,
		snapshot:  Snapshot{State: StateIdle},
	}
}

func (m *Module) WithClock(clock Clock) *Module {
	if clock != nil {
		m.clock = clock
	}
	return m
}

// Start begins the collect+upload scheduler. It never returns an error to the manager.
func (m *Module) Start(ctx context.Context) error {
	m.MarkStarting()
	m.logger.Info("Software inventory started")

	if !m.config.Enabled {
		m.setSnapshot(Snapshot{State: StateSkipped, LastAttemptAt: m.now()})
		m.logger.Info("Software inventory disabled by configuration")
		m.MarkRunning()
		return nil
	}
	if _, ok := m.identity.Current(); !ok {
		m.setSnapshot(Snapshot{State: StateFailed, LastAttemptAt: m.now(), LastError: "identity not available"})
		m.logger.Error("Software inventory failed", "reason", "identity not available")
		m.MarkFailed()
		return nil
	}

	m.setSnapshot(Snapshot{State: StateReady, LastAttemptAt: m.now()})
	m.scheduler = newScheduler(m.config.Interval, m.cycle)
	m.scheduler.Start(ctx)
	m.logger.Info("Software inventory scheduler running", "interval", m.config.Interval.String())
	m.MarkRunning()
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	m.MarkStopping()
	if m.scheduler != nil {
		m.scheduler.Stop()
		m.scheduler = nil
	}
	m.MarkStopped()
	return nil
}

// Applications returns the last collected inventory snapshot.
func (m *Module) Applications() []Application {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Application, len(m.last))
	copy(out, m.last)
	return out
}

func (m *Module) SoftwareStatus() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.snapshot
}

func (m *Module) cycle(ctx context.Context) {
	id, ok := m.identity.Current()
	if !ok {
		m.updateSnapshot(func(s *Snapshot) {
			s.State = StateFailed
			s.LastAttemptAt = m.now()
			s.LastError = "identity not available"
		})
		return
	}

	m.updateSnapshot(func(s *Snapshot) {
		s.State = StateCollecting
		s.LastAttemptAt = m.now()
		s.LastError = ""
	})

	apps, collectErr := m.collector.Collect(ctx)
	apps = dedupe(apps)
	m.mu.Lock()
	m.last = apps
	m.mu.Unlock()

	if len(apps) == 0 && collectErr != nil {
		m.updateSnapshot(func(s *Snapshot) {
			s.State = StateFailed
			s.ApplicationsFound = 0
			s.LastError = collectErr.Error()
		})
		m.logger.Error("Software inventory collection failed", "error", collectErr)
		return
	}

	m.updateSnapshot(func(s *Snapshot) {
		s.State = StateUploading
		s.ApplicationsFound = len(apps)
		if collectErr != nil {
			s.State = StatePartial
			s.LastError = collectErr.Error()
		}
	})

	req := Inventory{
		AgentID:      id.AgentID,
		InstallID:    id.InstallID,
		CollectedAt:  m.now(),
		Applications: apps,
	}
	result, err := m.client.Upload(ctx, req)
	m.updateSnapshot(func(s *Snapshot) {
		s.LastAttemptAt = m.now()
		s.Cycles++
		if result != nil {
			s.HTTPStatusCode = result.HTTPStatusCode
			s.ServerVersion = result.ServerVersion
			s.RequestID = result.RequestID
			s.ErrorCode = result.ErrorCode
		}
		if err != nil {
			s.State = StateFailed
			s.LastError = err.Error()
			return
		}
		if collectErr != nil {
			s.State = StatePartial
		} else {
			s.State = StateReady
			s.LastError = ""
		}
		s.LastSuccessAt = m.now()
	})
	if err != nil {
		m.logger.Warn("Software inventory upload failed", "error", err, "applications", len(apps))
		return
	}
	m.logger.Info("Software inventory upload success",
		"applications", len(apps),
		"stored", result.Stored,
		"http_status", result.HTTPStatusCode,
	)
}

var _ interfaces.Module = (*Module)(nil)

func (m *Module) now() time.Time { return m.clock().UTC() }

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

// scheduler mirrors the heartbeat/users ticker pattern (no overlapping cycles).
type scheduler struct {
	interval time.Duration
	beat     func(context.Context)
	mu       sync.Mutex
	running  bool
	cancel   context.CancelFunc
	done     chan struct{}
	inflight sync.Mutex
}

func newScheduler(interval time.Duration, beat func(context.Context)) *scheduler {
	return &scheduler{interval: interval, beat: beat}
}

func (s *scheduler) Start(parent context.Context) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	s.done = make(chan struct{})
	s.running = true
	s.mu.Unlock()
	go s.loop(ctx)
}

func (s *scheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	cancel := s.cancel
	done := s.done
	s.mu.Unlock()
	cancel()
	<-done
	s.mu.Lock()
	s.running = false
	s.cancel = nil
	s.done = nil
	s.mu.Unlock()
}

func (s *scheduler) loop(ctx context.Context) {
	defer close(s.done)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	s.run(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.run(ctx)
		}
	}
}

func (s *scheduler) run(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	if !s.inflight.TryLock() {
		return
	}
	defer s.inflight.Unlock()
	if ctx.Err() != nil || s.beat == nil {
		return
	}
	s.beat(ctx)
}
