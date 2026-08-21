package registration

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	txerrors "risklens/agent/internal/transport/errors"
	"risklens/agent/internal/transport/request"
	"risklens/agent/internal/transport/response"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func sampleRequest() Request {
	return Request{
		AgentID:         "9b5d7f3e-1234-4abc-9def-0123456789ab",
		InstallID:       "install-1",
		AgentVersion:    "0.1.0",
		MachineName:     "TEST-HOST",
		OperatingSystem: "windows",
		DisplayName:     "Test",
		CreatedAt:       time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC),
	}
}

func successBody() string {
	return `{"accepted":true,"agent_id":"9b5d7f3e-1234-4abc-9def-0123456789ab",` +
		`"registration_status":"registered","server_version":"0.1.0",` +
		`"server_time":"2026-08-14T01:02:03Z"}`
}

// ------------------------------------------------------------- construction

func TestNewClientRejectsEmptyURL(t *testing.T) {
	if _, err := NewClient("", time.Second, "0.1.0", nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestNewClientRejectsBadScheme(t *testing.T) {
	if _, err := NewClient("ftp://example.com", time.Second, "0.1.0", nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestNewClientAcceptsZeroTimeout(t *testing.T) {
	// The transport supplies the default; the module no longer owns one.
	if _, err := NewClient("http://localhost:1", 0, "0.1.0", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildUserAgentIncludesVersion(t *testing.T) {
	if got := buildUserAgent("1.2.3"); got != "RiskLensAgent/1.2.3" {
		t.Errorf("user agent = %q", got)
	}
	if got := buildUserAgent(""); got != "RiskLensAgent" {
		t.Errorf("user agent = %q", got)
	}
}

// ----------------------------------------------------- transport interaction

func TestRegisterIssuesPostToRegistrationEndpoint(t *testing.T) {
	transport := respondWith(response.StatusCreated, successBody())
	client := NewClientWithTransport(transport, testLogger())

	if _, err := client.Register(context.Background(), sampleRequest()); err != nil {
		t.Fatalf("Register: %v", err)
	}

	sent := transport.request()
	if sent.Method != request.MethodPOST {
		t.Errorf("method = %q", sent.Method)
	}
	if sent.Path != EndpointPath {
		t.Errorf("path = %q, want %q", sent.Path, EndpointPath)
	}
	if transport.callCount() != 1 {
		t.Errorf("calls = %d, want exactly one (no retry in this sprint)", transport.callCount())
	}
}

func TestRegisterPassesContextThrough(t *testing.T) {
	type ctxKey string
	const key ctxKey = "marker"

	transport := respondWith(response.StatusCreated, successBody())
	client := NewClientWithTransport(transport, testLogger())

	ctx := context.WithValue(context.Background(), key, "present")
	if _, err := client.Register(ctx, sampleRequest()); err != nil {
		t.Fatal(err)
	}
	if transport.lastCtx.Value(key) != "present" {
		t.Error("caller context was not propagated to the transport")
	}
}

func TestRegisterSendsRequestBodyUnchanged(t *testing.T) {
	transport := respondWith(response.StatusCreated, successBody())
	client := NewClientWithTransport(transport, testLogger())

	want := sampleRequest()
	if _, err := client.Register(context.Background(), want); err != nil {
		t.Fatal(err)
	}

	got, ok := transport.request().Body.(Request)
	if !ok {
		t.Fatalf("body type = %T, want registration.Request", transport.request().Body)
	}
	if got != want {
		t.Errorf("body = %+v, want %+v", got, want)
	}
}

func TestRegisterBodyMatchesBackendWireFormat(t *testing.T) {
	// The backend rejects unknown fields, so the JSON key set is a contract.
	encoded, err := json.Marshal(sampleRequest())
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}

	want := []string{
		"agent_id", "install_id", "agent_version",
		"machine_name", "operating_system", "display_name", "created_at",
	}
	for _, key := range want {
		if _, exists := decoded[key]; !exists {
			t.Errorf("payload missing %q", key)
		}
	}
	if len(decoded) != len(want) {
		t.Errorf("payload has %d keys, want %d: %v", len(decoded), len(want), decoded)
	}
}

// -------------------------------------------------------------- status paths

func TestRegisterSuccess201(t *testing.T) {
	transport := respondWith(response.StatusCreated, successBody())
	client := NewClientWithTransport(transport, testLogger())

	result, err := client.Register(context.Background(), sampleRequest())
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if result.HTTPStatusCode != response.StatusCreated {
		t.Errorf("status = %d", result.HTTPStatusCode)
	}
	if result.ServerVersion != "0.1.0" {
		t.Errorf("server_version = %q", result.ServerVersion)
	}
	if result.ServerTime.IsZero() {
		t.Error("server_time not parsed")
	}
}

func TestRegister409IsDuplicate(t *testing.T) {
	transport := respondWith(response.StatusConflict,
		`{"error":{"code":"duplicate_agent","message":"already registered","request_id":"rid-1"}}`)
	client := NewClientWithTransport(transport, testLogger())

	result, err := client.Register(context.Background(), sampleRequest())
	if !errors.Is(err, ErrDuplicate) {
		t.Fatalf("expected ErrDuplicate, got %v", err)
	}
	if result.HTTPStatusCode != response.StatusConflict {
		t.Errorf("status = %d", result.HTTPStatusCode)
	}
	if result.ErrorCode != "duplicate_agent" {
		t.Errorf("error_code = %q", result.ErrorCode)
	}
	if result.RequestID != "rid-1" {
		t.Errorf("request_id = %q", result.RequestID)
	}
}

func TestRegister422IsValidation(t *testing.T) {
	transport := respondWith(response.StatusUnprocessableEntity,
		`{"error":{"code":"validation_error","message":"bad body","request_id":"rid-2"}}`)
	client := NewClientWithTransport(transport, testLogger())

	_, err := client.Register(context.Background(), sampleRequest())
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
	}
}

func TestRegister400IsValidation(t *testing.T) {
	transport := respondWith(response.StatusBadRequest, `{"error":{"code":"bad_request"}}`)
	client := NewClientWithTransport(transport, testLogger())

	if _, err := client.Register(context.Background(), sampleRequest()); !errors.Is(err, ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
	}
}

func TestRegister500IsServerError(t *testing.T) {
	transport := respondWith(response.StatusInternalServerError,
		`{"error":{"code":"internal_error","message":"boom","request_id":"rid-3"}}`)
	client := NewClientWithTransport(transport, testLogger())

	result, err := client.Register(context.Background(), sampleRequest())
	if !errors.Is(err, ErrServer) {
		t.Fatalf("expected ErrServer, got %v", err)
	}
	if result.HTTPStatusCode != 500 {
		t.Errorf("status = %d", result.HTTPStatusCode)
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention status: %v", err)
	}
}

func TestRegister503IsServerError(t *testing.T) {
	transport := respondWith(response.StatusServiceUnavailable, ``)
	client := NewClientWithTransport(transport, testLogger())

	if _, err := client.Register(context.Background(), sampleRequest()); !errors.Is(err, ErrServer) {
		t.Fatalf("expected ErrServer, got %v", err)
	}
}

func TestRegisterUnexpectedStatus(t *testing.T) {
	transport := respondWith(response.StatusNotFound, `{"error":{"code":"not_found"}}`)
	client := NewClientWithTransport(transport, testLogger())

	_, err := client.Register(context.Background(), sampleRequest())
	if !errors.Is(err, ErrUnexpectedStatus) {
		t.Fatalf("expected ErrUnexpectedStatus, got %v", err)
	}
}

// ------------------------------------------------------------ response shape

func TestRegisterMalformedSuccessBody(t *testing.T) {
	transport := respondWith(response.StatusCreated, "this is not json")
	client := NewClientWithTransport(transport, testLogger())

	if _, err := client.Register(context.Background(), sampleRequest()); !errors.Is(err, ErrMalformedResponse) {
		t.Fatalf("expected ErrMalformedResponse, got %v", err)
	}
}

func TestRegisterEmptySuccessBody(t *testing.T) {
	transport := respondWith(response.StatusCreated, "")
	client := NewClientWithTransport(transport, testLogger())

	if _, err := client.Register(context.Background(), sampleRequest()); !errors.Is(err, ErrMalformedResponse) {
		t.Fatalf("expected ErrMalformedResponse, got %v", err)
	}
}

func TestRegisterRejectsUnacceptedResponse(t *testing.T) {
	transport := respondJSON(t, response.StatusCreated, SuccessResponse{
		Accepted:           false,
		RegistrationStatus: "pending",
	})
	client := NewClientWithTransport(transport, testLogger())

	if _, err := client.Register(context.Background(), sampleRequest()); !errors.Is(err, ErrMalformedResponse) {
		t.Fatalf("expected ErrMalformedResponse, got %v", err)
	}
}

func TestRegisterRejectsWrongRegistrationStatus(t *testing.T) {
	transport := respondJSON(t, response.StatusCreated, SuccessResponse{
		Accepted:           true,
		RegistrationStatus: "quarantined",
	})
	client := NewClientWithTransport(transport, testLogger())

	if _, err := client.Register(context.Background(), sampleRequest()); !errors.Is(err, ErrMalformedResponse) {
		t.Fatalf("expected ErrMalformedResponse, got %v", err)
	}
}

func TestRegisterToleratesUnparsableErrorEnvelope(t *testing.T) {
	// A proxy returning HTML on 409 must still yield ErrDuplicate.
	transport := respondWith(response.StatusConflict, `<html>Conflict</html>`)
	client := NewClientWithTransport(transport, testLogger())

	result, err := client.Register(context.Background(), sampleRequest())
	if !errors.Is(err, ErrDuplicate) {
		t.Fatalf("expected ErrDuplicate, got %v", err)
	}
	if result.ErrorCode != "" {
		t.Errorf("error_code = %q, want empty", result.ErrorCode)
	}
}

// ---------------------------------------------------- transport error mapping

func TestTransportTimeoutBecomesErrTransport(t *testing.T) {
	transport := failWith(txerrors.New(txerrors.KindTimeout, "send", nil))
	client := NewClientWithTransport(transport, testLogger())

	if _, err := client.Register(context.Background(), sampleRequest()); !errors.Is(err, ErrTransport) {
		t.Fatalf("expected ErrTransport, got %v", err)
	}
}

func TestTransportConnectionFailureBecomesErrTransport(t *testing.T) {
	transport := failWith(txerrors.New(txerrors.KindConnection, "send", nil))
	client := NewClientWithTransport(transport, testLogger())

	if _, err := client.Register(context.Background(), sampleRequest()); !errors.Is(err, ErrTransport) {
		t.Fatalf("expected ErrTransport, got %v", err)
	}
}

func TestTransportCancellationBecomesErrTransport(t *testing.T) {
	transport := failWith(txerrors.New(txerrors.KindCanceled, "send", nil))
	client := NewClientWithTransport(transport, testLogger())

	if _, err := client.Register(context.Background(), sampleRequest()); !errors.Is(err, ErrTransport) {
		t.Fatalf("expected ErrTransport, got %v", err)
	}
}

func TestOversizedResponseBecomesMalformed(t *testing.T) {
	// A reply that arrived but is unusable is a response problem, not a
	// connectivity problem, so it must not be reported as ErrTransport.
	transport := failWith(txerrors.New(txerrors.KindInvalidResponse, "read response", nil))
	client := NewClientWithTransport(transport, testLogger())

	if _, err := client.Register(context.Background(), sampleRequest()); !errors.Is(err, ErrMalformedResponse) {
		t.Fatalf("expected ErrMalformedResponse, got %v", err)
	}
}

func TestTransportDecodeFailureBecomesMalformed(t *testing.T) {
	transport := failWith(txerrors.New(txerrors.KindDecode, "read response", nil))
	client := NewClientWithTransport(transport, testLogger())

	if _, err := client.Register(context.Background(), sampleRequest()); !errors.Is(err, ErrMalformedResponse) {
		t.Fatalf("expected ErrMalformedResponse, got %v", err)
	}
}

func TestBuildRequestFailureIsReported(t *testing.T) {
	transport := respondWith(response.StatusCreated, successBody())
	transport.buildErr = errors.New("cannot build")
	client := NewClientWithTransport(transport, testLogger())

	_, err := client.Register(context.Background(), sampleRequest())
	if err == nil {
		t.Fatal("expected an error")
	}
	if transport.callCount() != 0 {
		t.Error("Send must not run when BuildRequest fails")
	}
}
