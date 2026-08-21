package deviceupload

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"risklens/agent/internal/modules/device"
	"risklens/agent/internal/pipeline"
	"risklens/agent/internal/transport/request"
	"risklens/agent/internal/transport/response"
)

func TestNewUploaderRejectsNilCollaborators(t *testing.T) {
	client := NewClientWithTransport(respond(response.StatusCreated, successBody(true)), testLogger())
	if _, err := NewUploader(nil, client); err == nil {
		t.Fatal("expected error for nil identity")
	}
	if _, err := NewUploader(stubIdentity{id: sampleIdentity(), ok: true}, nil); err == nil {
		t.Fatal("expected error for nil client")
	}
}

func TestUploaderCallsExistingClientOnce(t *testing.T) {
	transport := respond(response.StatusCreated, successBody(true))
	client := NewClientWithTransport(transport, testLogger())
	u, err := NewUploader(stubIdentity{id: sampleIdentity(), ok: true}, client)
	if err != nil {
		t.Fatal(err)
	}

	err = u.Upload(context.Background(), pipeline.Snapshot{
		Kind:          "device",
		SchemaVersion: pipeline.SchemaVersionV1,
		Payload:       sampleInfo(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if transport.callCount() != 1 {
		t.Fatalf("Send calls = %d, want 1", transport.callCount())
	}
}

func TestUploaderPayloadMatchesRequestFromInventory(t *testing.T) {
	info := sampleInfo()
	id := sampleIdentity()
	want := RequestFromInventory(id.AgentID, id.InstallID, info)

	transport := respond(response.StatusCreated, successBody(true))
	client := NewClientWithTransport(transport, testLogger())
	u, err := NewUploader(stubIdentity{id: id, ok: true}, client)
	if err != nil {
		t.Fatal(err)
	}

	if err := u.Upload(context.Background(), pipeline.Snapshot{Payload: info}); err != nil {
		t.Fatal(err)
	}

	if transport.lastReq.Method != request.MethodPOST {
		t.Fatalf("method = %s", transport.lastReq.Method)
	}
	if transport.lastReq.Path != EndpointPath {
		t.Fatalf("path = %s", transport.lastReq.Path)
	}
	if transport.lastReq.Headers != nil && len(transport.lastReq.Headers) != 0 {
		t.Fatalf("adapter must not add headers: %+v", transport.lastReq.Headers)
	}

	got, ok := transport.lastReq.Body.(Request)
	if !ok {
		t.Fatalf("body type %T", transport.lastReq.Body)
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if string(wantJSON) != string(gotJSON) {
		t.Fatalf("JSON mismatch\nwant %s\ngot  %s", wantJSON, gotJSON)
	}
}

func TestUploaderUsesDeviceTransportPayload(t *testing.T) {
	info := sampleInfo().Comparable()
	sample := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	want := RequestFromInventory(sampleIdentity().AgentID, sampleIdentity().InstallID, info.ForTransport(sample))

	transport := respond(response.StatusCreated, successBody(true))
	u, err := NewUploader(stubIdentity{id: sampleIdentity(), ok: true}, NewClientWithTransport(transport, testLogger()))
	if err != nil {
		t.Fatal(err)
	}
	if err := u.Upload(context.Background(), pipeline.Snapshot{Payload: info, CollectedAt: sample}); err != nil {
		t.Fatal(err)
	}
	got := transport.lastReq.Body.(Request)
	if !got.CollectedAt.Equal(want.CollectedAt) {
		t.Fatalf("collected_at = %v want %v", got.CollectedAt, want.CollectedAt)
	}
}

func TestUploaderPropagatesClientErrorUnchanged(t *testing.T) {
	transport := respond(response.StatusNotFound, `{"error":{"code":"unknown_agent","message":"nope"}}`)
	client := NewClientWithTransport(transport, testLogger())
	u, err := NewUploader(stubIdentity{id: sampleIdentity(), ok: true}, client)
	if err != nil {
		t.Fatal(err)
	}

	err = u.Upload(context.Background(), pipeline.Snapshot{Payload: sampleInfo()})
	if !errors.Is(err, ErrUnknownAgent) {
		t.Fatalf("got %v", err)
	}
	if transport.callCount() != 1 {
		t.Fatalf("calls = %d", transport.callCount())
	}
}

func TestUploaderDoesNotRetry(t *testing.T) {
	transport := respond(response.StatusInternalServerError, `{"error":{"code":"server"}}`)
	client := NewClientWithTransport(transport, testLogger())
	u, err := NewUploader(stubIdentity{id: sampleIdentity(), ok: true}, client)
	if err != nil {
		t.Fatal(err)
	}

	err = u.Upload(context.Background(), pipeline.Snapshot{Payload: sampleInfo()})
	if err == nil {
		t.Fatal("expected error")
	}
	if transport.callCount() != 1 {
		t.Fatalf("retries are forbidden, calls = %d", transport.callCount())
	}
}

func TestUploaderDoesNotModifySnapshot(t *testing.T) {
	transport := respond(response.StatusCreated, successBody(true))
	client := NewClientWithTransport(transport, testLogger())
	u, err := NewUploader(stubIdentity{id: sampleIdentity(), ok: true}, client)
	if err != nil {
		t.Fatal(err)
	}

	info := sampleInfo()
	snap := pipeline.Snapshot{
		Kind:          "device",
		SchemaVersion: pipeline.SchemaVersionV1,
		CollectedAt:   time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC),
		Fingerprint:   pipeline.Fingerprint{Value: "do-not-touch"},
		Payload:       info,
		Partial:       true,
		CollectError:  "cpu missing",
	}
	before := snap

	if err := u.Upload(context.Background(), snap); err != nil {
		t.Fatal(err)
	}

	if snap.Kind != before.Kind || snap.SchemaVersion != before.SchemaVersion ||
		!snap.CollectedAt.Equal(before.CollectedAt) || !snap.Fingerprint.Equal(before.Fingerprint) ||
		snap.Partial != before.Partial || snap.CollectError != before.CollectError {
		t.Fatalf("envelope mutated: %+v", snap)
	}
	got := snap.Payload.(device.Info)
	if got != info {
		t.Fatalf("payload mutated: %+v", got)
	}
}

func TestUploaderRejectsWrongPayloadWithoutHTTP(t *testing.T) {
	transport := respond(response.StatusCreated, successBody(true))
	client := NewClientWithTransport(transport, testLogger())
	u, err := NewUploader(stubIdentity{id: sampleIdentity(), ok: true}, client)
	if err != nil {
		t.Fatal(err)
	}

	err = u.Upload(context.Background(), pipeline.Snapshot{Payload: "not-device-info"})
	if !errors.Is(err, ErrInvalidPayload) {
		t.Fatalf("got %v", err)
	}
	if transport.callCount() != 0 {
		t.Fatal("must not Send")
	}
}

func TestUploaderRejectsMissingIdentityWithoutHTTP(t *testing.T) {
	transport := respond(response.StatusCreated, successBody(true))
	client := NewClientWithTransport(transport, testLogger())
	u, err := NewUploader(stubIdentity{ok: false}, client)
	if err != nil {
		t.Fatal(err)
	}

	err = u.Upload(context.Background(), pipeline.Snapshot{Payload: sampleInfo()})
	if !errors.Is(err, ErrIdentityUnavailable) {
		t.Fatalf("got %v", err)
	}
	if transport.callCount() != 0 {
		t.Fatal("must not Send")
	}
}

func TestUploaderRejectsEmptyNameWithoutHTTP(t *testing.T) {
	transport := respond(response.StatusCreated, successBody(true))
	client := NewClientWithTransport(transport, testLogger())
	u, err := NewUploader(stubIdentity{id: sampleIdentity(), ok: true}, client)
	if err != nil {
		t.Fatal(err)
	}

	info := sampleInfo()
	info.MachineName = ""
	info.Hostname = ""
	err = u.Upload(context.Background(), pipeline.Snapshot{Payload: info})
	if !errors.Is(err, ErrEmptyInventory) {
		t.Fatalf("got %v", err)
	}
	if transport.callCount() != 0 {
		t.Fatal("must not Send")
	}
}
