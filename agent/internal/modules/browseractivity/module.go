package browseractivity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
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
	ModuleName      = "browser_activity"
	ModuleVersion   = "1.0.0"
	BrowserPath     = "/api/v1/browser"
	userAgent       = "RiskLensAgent"
	DefaultInterval = 15 * time.Second
	DefaultTimeout  = 15 * time.Second
)

var (
	ErrUnknownAgent      = errors.New("unknown agent")
	ErrUnknownSession    = errors.New("unknown session")
	ErrValidation        = errors.New("request validation error")
	ErrServer            = errors.New("server error")
	ErrUnexpectedStatus  = errors.New("unexpected status")
	ErrMalformedResponse = errors.New("malformed response")
	ErrTransport         = errors.New("transport error")
)

type IdentityProvider interface {
	Current() (idmodel.Identity, bool)
}

type ForegroundSource func() (Foreground, bool)

type Config struct {
	Enabled             bool
	BackendURL          string
	Timeout             time.Duration
	Interval            time.Duration
	StateDir            string
	ChromiumExtensionID string
	FirefoxExtensionID  string
}

type Clock func() time.Time

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

type Result struct {
	HTTPStatusCode int
	ServerVersion  string
	ServerTime     time.Time
	RequestID      string
	ErrorCode      string
	ErrorMessage   string
	Stored         int
}

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
		logger:    logger.With("component", "browser_activity.client"),
	}
}

func (c *Client) Upload(ctx context.Context, req BrowserUpload) (*Result, error) {
	result := &Result{}
	txReq, err := c.transport.BuildRequest(request.MethodPOST, BrowserPath, req)
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
			return result, fmt.Errorf("%w: server did not accept browser activity upload", ErrMalformedResponse)
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
		if result.ErrorCode == "unknown_session" {
			return result, fmt.Errorf("%w: %s", ErrUnknownSession, safeMessage(result))
		}
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

type Module struct {
	*base.Module

	identity     IdentityProvider
	config       Config
	client       *Client
	logger       *slog.Logger
	clock        Clock
	foreground   ForegroundSource
	stateFile    string
	inboxFile    string
	activityFile string

	scheduler *scheduler

	mu       sync.RWMutex
	snapshot Snapshot
	state    persistedState
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
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
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

func NewWithDeps(
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
	m := &Module{
		Module:       base.New(ModuleName, ModuleVersion),
		identity:     identity,
		config:       cfg,
		client:       client,
		logger:       logger.With("module", ModuleName),
		clock:        time.Now,
		stateFile:    browserStatePath(cfg.StateDir),
		inboxFile:    inboxPath(cfg.StateDir),
		activityFile: activityJSONPath(cfg.StateDir),
		snapshot:     Snapshot{State: StateIdle},
	}
	m.foreground = func() (Foreground, bool) {
		return ReadForeground(m.activityFile)
	}
	return m
}

func (m *Module) WithClock(clock Clock) *Module {
	if clock != nil {
		m.clock = clock
	}
	return m
}

func (m *Module) WithForeground(src ForegroundSource) *Module {
	if src != nil {
		m.foreground = src
	}
	return m
}

func (m *Module) Start(ctx context.Context) error {
	m.MarkStarting()
	m.logger.Info("Browser activity started")

	if !m.config.Enabled {
		m.setSnapshot(Snapshot{State: StateSkipped, LastAttemptAt: m.now()})
		m.logger.Info("Browser activity disabled by configuration")
		m.MarkRunning()
		return nil
	}
	if _, ok := m.identity.Current(); !ok {
		m.setSnapshot(Snapshot{State: StateFailed, LastAttemptAt: m.now(), LastError: "identity not available"})
		m.logger.Error("Browser activity failed", "reason", "identity not available")
		m.MarkFailed()
		return nil
	}

	if loaded, err := loadBrowserState(m.stateFile); err == nil {
		m.mu.Lock()
		m.state = loaded
		m.mu.Unlock()
	}

	if exe, err := os.Executable(); err == nil {
		chromeJSON, firefoxJSON, werr := writeNativeManifests(
			m.config.StateDir,
			exe,
			m.config.ChromiumExtensionID,
			m.config.FirefoxExtensionID,
		)
		if werr != nil {
			m.logger.Warn("native messaging manifests not written", "error", werr)
		} else {
			registerNativeHost(m.logger, chromeJSON, firefoxJSON, m.config.ChromiumExtensionID)
		}
	}

	m.setSnapshot(Snapshot{State: StateReady, LastAttemptAt: m.now()})
	m.scheduler = newScheduler(m.config.Interval, m.cycle)
	m.scheduler.Start(ctx)
	m.logger.Info("Browser activity scheduler running", "interval", m.config.Interval.String())
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

func (m *Module) StatusSnapshot() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.snapshot
}

func (m *Module) cycle(ctx context.Context) {
	now := m.now()
	m.updateSnapshot(func(s *Snapshot) {
		s.State = StateCollecting
		s.LastAttemptAt = now
		s.Cycles++
	})

	samples, err := ReadInbox(m.inboxFile)
	if err != nil {
		m.logger.Warn("browser inbox unreadable", "error", err)
	} else if err := ClearInbox(m.inboxFile); err != nil {
		m.logger.Warn("browser inbox clear failed", "error", err)
	}

	fg, ok := m.foreground()
	if !ok {
		fg = Foreground{}
	}

	m.mu.Lock()
	state := m.state
	m.mu.Unlock()

	var pending []BrowserItem
	for i := range samples {
		sample := samples[i]
		result := step(state, fg, &sample, now)
		state = result.Next
		pending = append(pending, result.Items...)
	}
	result := step(state, fg, nil, now)
	state = result.Next
	pending = append(pending, result.Items...)

	m.mu.Lock()
	m.state = state
	m.mu.Unlock()
	if err := saveBrowserState(m.stateFile, state); err != nil {
		m.logger.Warn("browser state write failed", "error", err)
	}

	m.updateSnapshot(func(s *Snapshot) {
		s.SamplesFound = len(samples)
	})

	if len(pending) == 0 {
		m.updateSnapshot(func(s *Snapshot) { s.State = StateReady })
		return
	}

	id, ok := m.identity.Current()
	if !ok {
		m.updateSnapshot(func(s *Snapshot) {
			s.State = StateFailed
			s.LastError = "identity not available"
		})
		return
	}

	m.updateSnapshot(func(s *Snapshot) { s.State = StateUploading })
	uploadResult, err := m.client.Upload(ctx, BrowserUpload{
		AgentID:     id.AgentID,
		InstallID:   id.InstallID,
		CollectedAt: now,
		Activities:  pending,
	})
	if err != nil {
		m.logger.Warn("browser activity upload failed", "error", err, "items", len(pending))
		m.updateSnapshot(func(s *Snapshot) {
			s.State = StateFailed
			s.LastError = err.Error()
			if uploadResult != nil {
				s.HTTPStatusCode = uploadResult.HTTPStatusCode
			}
		})
		return
	}
	m.logger.Info("browser activity upload success", "items", len(pending), "stored", uploadResult.Stored)
	m.updateSnapshot(func(s *Snapshot) {
		s.State = StateReady
		s.LastSuccessAt = now
		s.Stored = uploadResult.Stored
		s.HTTPStatusCode = uploadResult.HTTPStatusCode
		s.LastError = ""
	})
}

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
	s.beat(ctx)
}

var _ interfaces.Module = (*Module)(nil)
