package registration

import (
	"context"
	"testing"
	"time"

	idmodel "risklens/agent/internal/identity/model"
	"risklens/agent/internal/modules/interfaces"
	txerrors "risklens/agent/internal/transport/errors"
	"risklens/agent/internal/transport/response"
)

type stubIdentity struct {
	id idmodel.Identity
	ok bool
}

func (s stubIdentity) Current() (idmodel.Identity, bool) {
	return s.id, s.ok
}

func sampleIdentity() idmodel.Identity {
	return idmodel.Identity{
		SchemaVersion:   1,
		AgentID:         "9b5d7f3e-1234-4abc-9def-0123456789ab",
		InstallID:       "install-1",
		AgentVersion:    "0.1.0",
		CreatedAt:       time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC),
		UpdatedAt:       time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC),
		Status:          idmodel.StatusActive,
		DisplayName:     "Test Host",
		MachineName:     "TEST-HOST",
		OperatingSystem: "windows",
	}
}

func enabledConfig() Config {
	return Config{Enabled: true, BackendURL: "http://backend.invalid", Timeout: time.Second}
}

// moduleWith builds an enabled module wired to a fake transport.
func moduleWith(t *testing.T, transport *fakeTransport) *Module {
	t.Helper()
	m, err := NewWithTransport(
		enabledConfig(),
		stubIdentity{id: sampleIdentity(), ok: true},
		transport,
		testLogger(),
	)
	if err != nil {
		t.Fatalf("NewWithTransport: %v", err)
	}
	return m
}

// ------------------------------------------------------------- construction

func TestNewRejectsNilIdentity(t *testing.T) {
	if _, err := New(Config{Enabled: false}, nil, "0.1.0", testLogger()); err == nil {
		t.Fatal("expected error")
	}
}

func TestNewRejectsEnabledWithBadURL(t *testing.T) {
	_, err := New(
		Config{Enabled: true, BackendURL: "not-a-url", Timeout: time.Second},
		stubIdentity{id: sampleIdentity(), ok: true},
		"0.1.0",
		testLogger(),
	)
	if err == nil {
		t.Fatal("expected error for bad backend URL")
	}
}

func TestNewAllowsDisabledWithoutURL(t *testing.T) {
	m, err := New(
		Config{Enabled: false},
		stubIdentity{id: sampleIdentity(), ok: true},
		"0.1.0",
		testLogger(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Name() != ModuleName || m.Version() != ModuleVersion {
		t.Fatalf("module identity wrong: %s %s", m.Name(), m.Version())
	}
	if _, ok := interfaces.Module(m).(*Module); !ok {
		t.Fatal("module does not satisfy interfaces.Module")
	}
}

func TestNewWithTransportRejectsNilTransport(t *testing.T) {
	_, err := NewWithTransport(
		enabledConfig(),
		stubIdentity{id: sampleIdentity(), ok: true},
		nil,
		testLogger(),
	)
	if err == nil {
		t.Fatal("expected error for nil transport")
	}
}

func TestNewWithTransportRejectsNilIdentity(t *testing.T) {
	if _, err := NewWithTransport(enabledConfig(), nil, respondWith(201, successBody()), testLogger()); err == nil {
		t.Fatal("expected error for nil identity")
	}
}

func TestNewWithTransportDoesNotRequireBackendURL(t *testing.T) {
	// The URL belongs to the transport now, so the module no longer needs it.
	m, err := NewWithTransport(
		Config{Enabled: true},
		stubIdentity{id: sampleIdentity(), ok: true},
		respondWith(response.StatusCreated, successBody()),
		testLogger(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if m.RegistrationStatus().State != StateRegistered {
		t.Fatalf("state = %q", m.RegistrationStatus().State)
	}
}

// ------------------------------------------------------------------ lifecycle

func TestStartDisabledSetsNotRegistered(t *testing.T) {
	transport := respondWith(response.StatusCreated, successBody())
	m, err := NewWithTransport(
		Config{Enabled: false},
		stubIdentity{id: sampleIdentity(), ok: true},
		transport,
		testLogger(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if snap := m.RegistrationStatus(); snap.State != StateNotRegistered {
		t.Fatalf("state = %q", snap.State)
	}
	if m.Status() != interfaces.StatusRunning {
		t.Fatalf("module status = %q", m.Status())
	}
	if transport.callCount() != 0 {
		t.Error("disabled module must not contact the backend")
	}
}

func TestStartNoIdentityMarksFailed(t *testing.T) {
	transport := respondWith(response.StatusCreated, successBody())
	m, err := NewWithTransport(enabledConfig(), stubIdentity{ok: false}, transport, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if snap := m.RegistrationStatus(); snap.State != StateFailed {
		t.Fatalf("state = %q", snap.State)
	}
	if m.Status() != interfaces.StatusFailed {
		t.Fatalf("module status = %q", m.Status())
	}
	if transport.callCount() != 0 {
		t.Error("must not contact the backend without an identity")
	}
}

func TestStopIsNoOp(t *testing.T) {
	m, err := New(Config{Enabled: false}, stubIdentity{id: sampleIdentity(), ok: true}, "0.1.0", testLogger())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := m.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if m.Status() != interfaces.StatusStopped {
		t.Fatalf("status = %q", m.Status())
	}
}

// ------------------------------------------------------------ backend results

func TestStartSuccessfulRegistration(t *testing.T) {
	transport := respondWith(response.StatusCreated, successBody())
	m := moduleWith(t, transport)

	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	snap := m.RegistrationStatus()
	if snap.State != StateRegistered {
		t.Fatalf("state = %q", snap.State)
	}
	if snap.HTTPStatusCode != response.StatusCreated {
		t.Fatalf("status = %d", snap.HTTPStatusCode)
	}
	if snap.ServerVersion != "0.1.0" {
		t.Fatalf("server_version = %q", snap.ServerVersion)
	}
	if snap.LastAttemptAt.IsZero() {
		t.Fatal("last_attempt_at is zero")
	}
	if m.Status() != interfaces.StatusRunning {
		t.Fatalf("module status = %q", m.Status())
	}
	if m.Health() != interfaces.HealthHealthy {
		t.Fatalf("module health = %q", m.Health())
	}
}

func TestStartDuplicateIsRegistered(t *testing.T) {
	transport := respondWith(response.StatusConflict,
		`{"error":{"code":"duplicate_agent","message":"already","request_id":"rid-a"}}`)
	m := moduleWith(t, transport)

	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	snap := m.RegistrationStatus()
	if snap.State != StateRegistered {
		t.Fatalf("state = %q", snap.State)
	}
	if snap.ErrorCode != "duplicate_agent" {
		t.Fatalf("error_code = %q", snap.ErrorCode)
	}
	if snap.RequestID != "rid-a" {
		t.Fatalf("request_id = %q", snap.RequestID)
	}
	if m.Status() != interfaces.StatusRunning {
		t.Fatalf("module status = %q", m.Status())
	}
}

func TestStartValidationErrorMarksFailed(t *testing.T) {
	transport := respondWith(response.StatusUnprocessableEntity,
		`{"error":{"code":"validation_error","message":"bad"}}`)
	m := moduleWith(t, transport)

	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	snap := m.RegistrationStatus()
	if snap.State != StateFailed {
		t.Fatalf("state = %q", snap.State)
	}
	if snap.ErrorCode != "validation_error" {
		t.Fatalf("error_code = %q", snap.ErrorCode)
	}
	if m.Status() != interfaces.StatusFailed {
		t.Fatalf("module status = %q", m.Status())
	}
}

func TestStartServerErrorMarksFailed(t *testing.T) {
	m := moduleWith(t, respondWith(response.StatusInternalServerError, ``))

	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	snap := m.RegistrationStatus()
	if snap.State != StateFailed {
		t.Fatalf("state = %q", snap.State)
	}
	if snap.HTTPStatusCode != 500 {
		t.Fatalf("http_status = %d", snap.HTTPStatusCode)
	}
}

func TestStartBackendUnavailableMarksFailed(t *testing.T) {
	m := moduleWith(t, failWith(txerrors.New(txerrors.KindConnection, "send", nil)))

	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	snap := m.RegistrationStatus()
	if snap.State != StateFailed {
		t.Fatalf("state = %q", snap.State)
	}
	if snap.LastError == "" {
		t.Fatal("expected LastError to be populated")
	}
}

func TestStartTimeoutMarksFailed(t *testing.T) {
	m := moduleWith(t, failWith(txerrors.New(txerrors.KindTimeout, "send", nil)))

	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if m.RegistrationStatus().State != StateFailed {
		t.Fatalf("state = %q", m.RegistrationStatus().State)
	}
}

func TestStartMalformedResponseMarksFailed(t *testing.T) {
	m := moduleWith(t, respondWith(response.StatusCreated, "not-json"))

	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if m.RegistrationStatus().State != StateFailed {
		t.Fatalf("state = %q", m.RegistrationStatus().State)
	}
}

func TestStartNeverReturnsErrorToTheModuleManager(t *testing.T) {
	// The service must survive every backend outcome.
	transports := []*fakeTransport{
		respondWith(response.StatusCreated, successBody()),
		respondWith(response.StatusConflict, `{}`),
		respondWith(response.StatusUnprocessableEntity, `{}`),
		respondWith(response.StatusInternalServerError, ``),
		respondWith(response.StatusNotFound, ``),
		respondWith(response.StatusCreated, "garbage"),
		failWith(txerrors.New(txerrors.KindTimeout, "send", nil)),
		failWith(txerrors.New(txerrors.KindConnection, "send", nil)),
		failWith(txerrors.New(txerrors.KindCanceled, "send", nil)),
	}
	for i, transport := range transports {
		m := moduleWith(t, transport)
		if err := m.Start(context.Background()); err != nil {
			t.Errorf("case %d: Start returned %v, want nil", i, err)
		}
	}
}

// ------------------------------------------------------------------- payload

func TestStartUsesIdentityFields(t *testing.T) {
	transport := respondWith(response.StatusCreated, successBody())
	id := sampleIdentity()
	m := moduleWith(t, transport)

	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	got, ok := transport.request().Body.(Request)
	if !ok {
		t.Fatalf("body type = %T", transport.request().Body)
	}
	if got.AgentID != id.AgentID {
		t.Errorf("agent_id: got %q want %q", got.AgentID, id.AgentID)
	}
	if got.InstallID != id.InstallID {
		t.Errorf("install_id: got %q want %q", got.InstallID, id.InstallID)
	}
	if got.MachineName != id.MachineName {
		t.Errorf("machine_name: got %q want %q", got.MachineName, id.MachineName)
	}
	if got.OperatingSystem != id.OperatingSystem {
		t.Errorf("operating_system: got %q want %q", got.OperatingSystem, id.OperatingSystem)
	}
	if got.AgentVersion != id.AgentVersion {
		t.Errorf("agent_version: got %q want %q", got.AgentVersion, id.AgentVersion)
	}
	if got.DisplayName != id.DisplayName {
		t.Errorf("display_name: got %q want %q", got.DisplayName, id.DisplayName)
	}
	if !got.CreatedAt.Equal(id.CreatedAt.UTC()) {
		t.Errorf("created_at: got %v want %v", got.CreatedAt, id.CreatedAt)
	}
}

func TestStartUsesInjectedClock(t *testing.T) {
	frozen := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	m := moduleWith(t, respondWith(response.StatusCreated, successBody()))
	m.WithClock(func() time.Time { return frozen })

	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !m.RegistrationStatus().LastAttemptAt.Equal(frozen) {
		t.Fatalf("last_attempt_at = %v", m.RegistrationStatus().LastAttemptAt)
	}
}

func TestStartIssuesExactlyOneAttempt(t *testing.T) {
	// Retry is explicitly out of scope for this sprint.
	transport := failWith(txerrors.New(txerrors.KindConnection, "send", nil))
	m := moduleWith(t, transport)

	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if transport.callCount() != 1 {
		t.Fatalf("calls = %d, want 1", transport.callCount())
	}
}
