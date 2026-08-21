package activity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
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
	ModuleName         = "activity"
	ModuleVersion      = "1.0.0"
	SessionsPath       = "/api/v1/sessions"
	ApplicationsPath   = "/api/v1/applications"
	userAgent          = "RiskLensAgent"
	DefaultInterval    = 15 * time.Second
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
	StateDir   string
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
		logger:    logger.With("component", "activity.client"),
	}
}

func (c *Client) UploadSessions(ctx context.Context, req SessionUpload) (*Result, error) {
	kind := sessionPayloadKind(req.Sessions)
	c.logger.Info("REPRO Payload sent to backend",
		"path", SessionsPath,
		"kind", kind,
		"payload", jsonDump(req),
	)
	if kind == "SHUTDOWN" {
		c.logger.Info("REPRO Shutdown payload", "payload", jsonDump(req))
	}
	if kind == "HEARTBEAT_OR_LOGIN_OPEN" {
		c.logger.Info("REPRO Heartbeat payload", "payload", jsonDump(req))
	}
	c.logger.Info("POST /api/v1/sessions",
		"agent_id", req.AgentID,
		"sessions", len(req.Sessions),
		"payload_status", sessionStatuses(req.Sessions),
	)
	result, err := c.upload(ctx, SessionsPath, req)
	if result != nil {
		c.logger.Info("POST /api/v1/sessions response",
			"http_status", result.HTTPStatusCode,
			"stored", result.Stored,
			"retry", err != nil,
			"error", errString(err),
		)
	}
	return result, err
}

func sessionStatuses(items []SessionItem) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, item.Status)
	}
	return strings.Join(parts, ",")
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (c *Client) UploadApplications(ctx context.Context, req ApplicationUpload) (*Result, error) {
	return c.upload(ctx, ApplicationsPath, req)
}

func (c *Client) upload(ctx context.Context, path string, body any) (*Result, error) {
	result := &Result{}
	txReq, err := c.transport.BuildRequest(request.MethodPOST, path, body)
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
			return result, fmt.Errorf("%w: server did not accept activity upload", ErrMalformedResponse)
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

	identity  IdentityProvider
	collector Collector
	config    Config
	client    *Client
	logger    *slog.Logger
	clock     Clock
	stateFile string

	scheduler *scheduler

	mu         sync.RWMutex
	processMu  sync.Mutex
	closing    atomic.Bool
	snapshot   Snapshot
	state      persistedState
	queue      uploadQueue
	queueFile  string
	events     []SessionEvent
	eventsFile string
	notifyHwnd atomic.Uintptr
	notifyOnce sync.Once
	notifyDone chan struct{}
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
		clock:      time.Now,
		stateFile:  statePath(cfg.StateDir),
		queueFile:  queuePath(cfg.StateDir),
		eventsFile: eventsPath(cfg.StateDir),
		snapshot:   Snapshot{State: StateIdle},
	}
}

func (m *Module) WithClock(clock Clock) *Module {
	if clock != nil {
		m.clock = clock
	}
	return m
}

func (m *Module) Start(ctx context.Context) error {
	m.MarkStarting()
	m.logger.Info("Activity collection started")

	if !m.config.Enabled {
		m.setSnapshot(Snapshot{State: StateSkipped, LastAttemptAt: m.now()})
		m.logger.Info("Activity collection disabled by configuration")
		m.MarkRunning()
		return nil
	}
	if _, ok := m.identity.Current(); !ok {
		m.setSnapshot(Snapshot{State: StateFailed, LastAttemptAt: m.now(), LastError: "identity not available"})
		m.logger.Error("Activity collection failed", "reason", "identity not available")
		m.MarkFailed()
		return nil
	}

	if loaded, err := loadState(m.stateFile); err == nil {
		cleaned := sanitizeState(loaded)
		m.mu.Lock()
		m.state = cleaned
		m.mu.Unlock()
		if cleaned.hasSession() != loaded.hasSession() || cleaned.SessionID != loaded.SessionID {
			if saveErr := saveState(m.stateFile, cleaned); saveErr != nil {
				m.logger.Warn("Activity state file write failed", "error", saveErr)
			}
		}
	} else {
		m.logger.Warn("Activity state file unreadable", "error", err)
	}
	m.mu.Lock()
	m.queue = loadQueue(m.queueFile)
	m.events = loadEvents(m.eventsFile)
	m.queue.dropUploadedClosed(m.state)
	m.mu.Unlock()
	m.persistQueue()

	m.drainQueue(ctx)

	m.setSnapshot(Snapshot{State: StateReady, LastAttemptAt: m.now()})
	m.installLogoffHook()
	m.startSessionNotify()
	m.scheduler = newScheduler(m.config.Interval, m.cycle)
	m.scheduler.Start(ctx)
	m.logger.Info("Activity scheduler running", "interval", m.config.Interval.String())
	m.MarkRunning()
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	m.MarkStopping()
	m.stopSessionNotify()
	if m.scheduler != nil {
		m.scheduler.Stop()
		m.scheduler = nil
	}
	m.MarkStopped()
	return nil
}

func (m *Module) ActivityStatus() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.snapshot
}

func (m *Module) cycle(ctx context.Context) {
	m.processMu.Lock()
	defer m.processMu.Unlock()
	if m.closing.Load() {
		m.logger.Info("cycle skipped; shutdown owns the current session")
		return
	}

	id, ok := m.identity.Current()
	if !ok {
		m.updateSnapshot(func(s *Snapshot) {
			s.State = StateFailed
			s.LastAttemptAt = m.now()
			s.LastError = "identity not available"
		})
		return
	}

	m.drainQueue(ctx)

	m.updateSnapshot(func(s *Snapshot) {
		s.State = StateCollecting
		s.LastAttemptAt = m.now()
		s.LastError = ""
	})

	obs, collectErr := m.collector.Collect(ctx)
	m.mu.Lock()
	prev := m.state
	m.mu.Unlock()

	now := m.now()
	beforeFile := m.activityFileJSON()
	beforeMem := jsonDump(prev)
	result := step(prev, obs, id.AgentID, now)
	login := result.Next.hasSession() && prev.SessionID != result.Next.SessionID
	if login {
		m.repro("LOGIN DETECTED")
		m.repro("Previous session_id", "session_id", prev.SessionID)
		m.repro("Current session_id", "session_id", result.Next.SessionID)
		m.repro("activity.json before", "contents", beforeFile)
		m.repro("memory before", "contents", beforeMem)
	}
	m.applyResult(ctx, id, now, result, collectErr)
	if login {
		m.mu.RLock()
		afterMem := jsonDump(m.state)
		m.mu.RUnlock()
		m.repro("activity.json after", "contents", m.activityFileJSON())
		m.repro("memory after", "contents", afterMem)
	}
}

// FlushClose uploads a closed session for the current Windows logon.
// Used on WTS_SESSION_LOGOFF, CTRL_LOGOFF_EVENT, and Windows shutdown so
// logout time and duration are stored before this process is terminated.
// Agent restart must not call this.
func (m *Module) FlushClose(ctx context.Context) error {
	if m.client == nil || !m.config.Enabled {
		return nil
	}
	id, ok := m.identity.Current()
	if !ok {
		return nil
	}
	m.closing.Store(true)

	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
	}

	done := make(chan struct{})
	go func() {
		m.processMu.Lock()
		defer m.processMu.Unlock()
		m.drainQueue(ctx)
		m.persistCloseBestEffort(id)
		m.drainQueue(ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		m.logger.Warn("Activity logoff upload timed out; close is queued for retry")
	}

	m.mu.Lock()
	empty := !m.state.hasSession()
	m.mu.Unlock()
	if empty {
		m.closing.Store(false)
	}
	return nil
}

func (m *Module) persistCloseBestEffort(id idmodel.Identity) {
	beforeFile := m.activityFileJSON()
	m.mu.Lock()
	prev := m.state
	m.mu.Unlock()
	m.repro("SHUTDOWN")
	m.repro("Current session_id", "session_id", prev.SessionID)
	m.repro("Previous session_id", "session_id", prev.LastClosedSessionID)
	m.repro("activity.json before", "contents", beforeFile)
	m.repro("memory before", "contents", jsonDump(prev))
	if !prev.hasSession() {
		m.repro("activity.json after", "contents", m.activityFileJSON())
		return
	}
	if prev.alreadyUploadedClosed(prev.SessionID) {
		m.replaceState(rememberClosed(prev, prev.SessionID, prev.LogonAt))
		m.repro("Shutdown payload", "payload", "skipped; already uploaded closed")
		m.repro("activity.json after", "contents", m.activityFileJSON())
		return
	}
	now := m.now()
	result := step(prev, Observation{}, id.AgentID, now)
	if len(result.Sessions) > 1 {
		kept := make([]SessionItem, 0, 1)
		for _, item := range result.Sessions {
			if item.SessionID == prev.SessionID && item.Status == StatusClosed {
				kept = append(kept, item)
			}
		}
		result.Sessions = kept
	}
	if len(result.Sessions) == 0 {
		m.mu.RLock()
		queued := m.queue.containsSessionID(prev.SessionID)
		uploaded := m.state.alreadyUploadedClosed(prev.SessionID)
		m.mu.RUnlock()
		if uploaded || queued || prev.alreadyClosedLocally(prev.SessionID) {
			m.replaceState(rememberClosed(prev, prev.SessionID, prev.LogonAt))
			m.repro("Shutdown payload", "payload", "skipped; queued or locally closed")
			m.repro("activity.json after", "contents", m.activityFileJSON())
			return
		}
		closed, app, hasApp := closeSession(prev, now)
		apps := []ActivityItem{}
		if hasApp {
			apps = []ActivityItem{app}
		}
		m.repro("Shutdown payload", "payload", jsonDump([]SessionItem{closed}))
		m.enqueueUploads(id, now, []SessionItem{closed}, apps, 0)
		m.replaceState(rememberClosed(prev, prev.SessionID, prev.LogonAt))
		m.repro("activity.json after", "contents", m.activityFileJSON())
		return
	}
	m.enqueueUploads(id, now, result.Sessions, result.Apps, 0)
	m.replaceState(result.Next)
	logoff := now
	if len(result.Sessions) > 0 && result.Sessions[0].LogoffAt != nil {
		logoff = *result.Sessions[0].LogoffAt
	}
	m.logger.Info("Queue persisted",
		"session_id", prev.SessionID,
		"logon_at", prev.LogonAt,
		"logout_at", logoff,
		"queue_len", m.QueueLen(),
		"closed", hasClosedSession(result),
		"retry", m.QueueLen() > 0,
	)
	m.logger.Info("------------------------------------------")
	m.logger.Info("SHUTDOWN")
	m.logger.Info("Current Session", "session_id", prev.SessionID)
	m.logger.Info("------------------------------------------")
	m.repro("Shutdown payload", "payload", jsonDump(result.Sessions))
	m.repro("activity.json after", "contents", m.activityFileJSON())
	m.mu.RLock()
	afterMem := jsonDump(m.state)
	m.mu.RUnlock()
	m.repro("memory after", "contents", afterMem)
}

func (m *Module) applyResult(ctx context.Context, id idmodel.Identity, now time.Time, result stepResult, collectErr error) {
	if !result.Changed && len(result.Sessions) == 0 && len(result.Apps) == 0 {
		m.updateSnapshot(func(s *Snapshot) {
			s.State = StateReady
			s.SessionsFound = 0
			s.ActivitiesFound = 0
			s.Cycles++
			if collectErr != nil {
				s.State = StatePartial
				s.LastError = collectErr.Error()
			}
		})
		return
	}

	closedItems, openedItems := partitionSessions(result.Sessions)
	stoppedApps, runningApps := partitionApps(result.Apps)

	m.mu.RLock()
	currentID := m.state.SessionID
	currentJSON, _ := json.Marshal(m.state)
	prevState := m.state
	m.mu.RUnlock()

	switching := result.Next.hasSession() && currentID != "" && currentID != result.Next.SessionID
	openingWithoutLocal := result.Next.hasSession() && currentID == ""
	if switching {
		if len(closedItems) == 0 {
			closedAt := result.Next.LogonAt
			if closedAt.IsZero() {
				closedAt = now
			}
			closed, app, hasApp := closeSession(prevState, closedAt)
			closedItems = []SessionItem{closed}
			if hasApp {
				stoppedApps = append([]ActivityItem{app}, stoppedApps...)
			}
			m.logger.Info("Decision taken", "decision", "SYNTHESIZE_CLOSE_THEN_OPEN")
		} else {
			m.logger.Info("Decision taken", "decision", "CLOSE_THEN_OPEN")
		}
		m.commitCloseThenOpen(ctx, id, now, closedItems, openedItems, stoppedApps, runningApps, result.Next, collectErr)
		return
	}
	if openingWithoutLocal {
		m.logger.Info("------------------------------------------")
		m.logger.Info("LOGIN DETECTED")
		m.logger.Info("Incoming session_id", "session_id", result.Next.SessionID)
		m.logger.Info("Current session_id from activity.json", "session_id", currentID)
		m.logger.Info("Current active session in memory", "session_id", currentID)
		m.logger.Info("activity.json", "contents", string(currentJSON))
		m.logger.Info("Decision taken", "decision", "OPEN_ONLY_NO_LOCAL_PREVIOUS")
		m.logger.Info("------------------------------------------")
	}

	result.Sessions = append(closedItems, openedItems...)
	result.Apps = append(stoppedApps, runningApps...)
	m.persistAndUpload(ctx, id, now, result, collectErr)
}

func (m *Module) commitCloseThenOpen(
	ctx context.Context,
	id idmodel.Identity,
	now time.Time,
	closedItems, openedItems []SessionItem,
	stoppedApps, runningApps []ActivityItem,
	next persistedState,
	collectErr error,
) {
	prevID := ""
	prevLogon := time.Time{}
	logoutAt := now
	m.mu.RLock()
	if m.state.hasSession() {
		prevID = m.state.SessionID
		prevLogon = m.state.LogonAt
	}
	m.mu.RUnlock()
	if len(closedItems) > 0 {
		prevID = closedItems[0].SessionID
		prevLogon = closedItems[0].LogonAt
		if closedItems[0].LogoffAt != nil {
			logoutAt = *closedItems[0].LogoffAt
		}
	}
	contents, _ := json.Marshal(m.state)
	m.logger.Info("------------------------------------------")
	m.logger.Info("LOGIN DETECTED")
	m.logger.Info("Incoming session_id", "session_id", next.SessionID)
	m.logger.Info("Current session_id from activity.json", "session_id", prevID)
	m.logger.Info("Current active session in memory", "session_id", prevID)
	m.logger.Info("activity.json", "contents", string(contents))
	m.logger.Info("previous session found",
		"session_id", prevID,
		"logon_at", prevLogon,
	)
	m.logger.Info("Incoming Session", "session_id", next.SessionID)
	m.logger.Info("Decision taken", "decision", "CLOSE_THEN_OPEN")
	m.logger.Info("------------------------------------------")

	m.enqueueUploads(id, now, closedItems, stoppedApps, 0)
	m.mu.Lock()
	cur := m.state
	m.mu.Unlock()
	closedState := rememberClosed(cur, prevID, prevLogon)
	m.replaceState(closedState)
	m.logger.Info("previous session closed",
		"session_id", prevID,
		"logon_at", prevLogon,
		"logout_at", logoutAt,
	)

	m.drainQueue(ctx)
	m.logger.Info("previous session close upload status",
		"session_id", prevID,
		"logout_at", logoutAt,
		"queue_len", m.QueueLen(),
		"retry", m.QueueLen() > 0,
	)

	m.mu.Lock()
	curAfter := m.state
	m.mu.Unlock()
	next.LastClosedSessionID = prevID
	next.LastClosedLogonAt = prevLogon
	next = sanitizeState(preserveClosedMeta(curAfter, next))

	m.replaceState(next)
	m.logger.Info("new session created",
		"session_id", next.SessionID,
		"logon_at", next.LogonAt,
	)
	m.logger.Info("Rows updated", "count", len(closedItems))
	m.logger.Info("Rows inserted", "count", len(openedItems))

	openResult := stepResult{
		Next:     next,
		Sessions: openedItems,
		Apps:     runningApps,
		Changed:  true,
	}
	m.persistAndUpload(ctx, id, now, openResult, collectErr)
}

func partitionSessions(items []SessionItem) (closed, opened []SessionItem) {
	for _, item := range items {
		if item.Status == StatusClosed {
			closed = append(closed, item)
			continue
		}
		opened = append(opened, item)
	}
	if len(opened) > 1 {
		opened = opened[len(opened)-1:]
	}
	return closed, opened
}

func partitionApps(items []ActivityItem) (stopped, running []ActivityItem) {
	for _, item := range items {
		if item.StoppedAt != nil {
			stopped = append(stopped, item)
			continue
		}
		running = append(running, item)
	}
	return stopped, running
}

func (m *Module) persistAndUpload(ctx context.Context, id idmodel.Identity, now time.Time, result stepResult, collectErr error) {
	m.mu.Lock()
	cur := m.state
	m.mu.Unlock()
	result.Next = preserveClosedMeta(cur, result.Next)
	result.Sessions = filterUnsentSessions(result.Next, result.Sessions)
	currentID := result.Next.SessionID
	if currentID == "" {
		currentID = cur.SessionID
	}
	kept := result.Sessions[:0]
	for _, item := range result.Sessions {
		if item.Status != StatusClosed && currentID != "" && item.SessionID != currentID {
			m.logger.Error("HEARTBEAT dropped non-current session",
				"session_id", item.SessionID,
				"current_session_id", currentID,
			)
			continue
		}
		kept = append(kept, item)
	}
	result.Sessions = kept

	m.updateSnapshot(func(s *Snapshot) {
		s.State = StateUploading
		s.SessionsFound = len(result.Sessions)
		s.ActivitiesFound = len(result.Apps)
		if collectErr != nil {
			s.LastError = collectErr.Error()
		}
	})

	closed := hasClosedSession(result)
	if result.Changed || closed || !result.Next.hasSession() {
		m.replaceState(result.Next)
	}

	if len(result.Sessions) == 0 && len(result.Apps) == 0 {
		m.updateSnapshot(func(s *Snapshot) {
			s.State = StateReady
			s.SessionsFound = 0
			s.ActivitiesFound = 0
			s.Cycles++
			s.LastAttemptAt = m.now()
			if collectErr != nil {
				s.State = StatePartial
				s.LastError = collectErr.Error()
			}
		})
		return
	}

	var uploadErr error
	var lastResult *Result
	uploaded := 0
	sessionStored := 0
	if len(result.Sessions) > 0 {
		lastResult, uploadErr = m.client.UploadSessions(ctx, SessionUpload{
			AgentID:     id.AgentID,
			InstallID:   id.InstallID,
			CollectedAt: now,
			Sessions:    result.Sessions,
		})
		if uploadErr == nil {
			uploaded = 1
			if lastResult != nil {
				sessionStored = lastResult.Stored
			}
			m.ackClosedSessionIDs(closedSessionIDs(result.Sessions))
		}
	}
	if uploadErr == nil && len(result.Apps) > 0 {
		lastResult, uploadErr = m.client.UploadApplications(ctx, ApplicationUpload{
			AgentID:     id.AgentID,
			InstallID:   id.InstallID,
			CollectedAt: now,
			Activities:  result.Apps,
		})
	}

	if uploadErr != nil {
		m.enqueueUploads(id, now, result.Sessions, result.Apps, uploaded)
	}

	m.updateSnapshot(func(s *Snapshot) {
		s.LastAttemptAt = m.now()
		s.Cycles++
		if lastResult != nil {
			s.HTTPStatusCode = lastResult.HTTPStatusCode
			s.ServerVersion = lastResult.ServerVersion
			s.RequestID = lastResult.RequestID
			s.ErrorCode = lastResult.ErrorCode
		}
		if uploadErr != nil {
			s.State = StateFailed
			s.LastError = uploadErr.Error()
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
	if uploadErr != nil {
		if lastResult != nil && lastResult.ErrorCode == "multiple_active_sessions" {
			contents, _ := json.Marshal(result.Next)
			m.logger.Error("FATAL: more than one ACTIVE session after login",
				"error_code", lastResult.ErrorCode,
				"http_status", lastResult.HTTPStatusCode,
				"incoming_session_id", currentID,
				"activity_json", string(contents),
			)
		}
		m.logger.Warn("Activity upload failed", "error", uploadErr, "retry", true)
		return
	}
	m.logger.Info("Activity upload success",
		"sessions", len(result.Sessions),
		"activities", len(result.Apps),
		"payload_status", sessionStatuses(result.Sessions),
		"rows_written", sessionStored,
	)
}

var _ interfaces.Module = (*Module)(nil)

func (m *Module) now() time.Time { return m.clock().UTC() }

func (m *Module) replaceState(state persistedState) {
	state = sanitizeState(state)
	m.mu.Lock()
	m.state = state
	path := m.stateFile
	m.mu.Unlock()
	if err := saveState(path, state); err != nil {
		m.logger.Warn("Activity state file write failed", "error", err)
	}
}

func (m *Module) ackClosedSessionIDs(ids []string) {
	if len(ids) == 0 {
		return
	}
	m.mu.Lock()
	m.state = sanitizeState(withUploadedClosed(m.state, ids...))
	m.queue.dropUploadedClosed(m.state)
	state := m.state
	path := m.stateFile
	m.mu.Unlock()
	if err := saveState(path, state); err != nil {
		m.logger.Warn("Activity state file write failed", "error", err)
	}
	m.persistQueue()
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
