package storage

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"risklens/agent/internal/identity/model"
)

func sample(t *testing.T) model.Identity {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Millisecond)
	return model.Identity{
		SchemaVersion:   model.SchemaVersion,
		AgentID:         "550e8400-e29b-41d4-a716-446655440000",
		InstallID:       "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
		AgentVersion:    "0.1.0",
		CreatedAt:       now,
		UpdatedAt:       now,
		Status:          model.StatusActive,
		DisplayName:     "RiskLens Endpoint Agent",
		MachineName:     "HOST1",
		OperatingSystem: "windows/amd64",
	}
}

func TestFileStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.json")
	store, err := NewFileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	want := sample(t)
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.AgentID != want.AgentID || got.InstallID != want.InstallID {
		t.Fatalf("got %+v", got)
	}
}

func TestFileStoreMissing(t *testing.T) {
	store, err := NewFileStore(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Load()
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestFileStoreCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.json")
	if err := os.WriteFile(path, []byte("{not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewFileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Load()
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("expected ErrCorrupt, got %v", err)
	}
}

func TestFileStoreIgnoresUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.json")
	raw := `{
  "schema_version": 1,
  "agent_id": "550e8400-e29b-41d4-a716-446655440000",
  "install_id": "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
  "agent_version": "0.1.0",
  "created_at": "2026-01-01T00:00:00Z",
  "updated_at": "2026-01-01T00:00:00Z",
  "status": "active",
  "display_name": "RiskLens Endpoint Agent",
  "machine_name": "HOST1",
  "operating_system": "windows/amd64",
  "future_field": "ok"
}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewFileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	id, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if id.AgentID != "550e8400-e29b-41d4-a716-446655440000" {
		t.Fatalf("agent_id = %s", id.AgentID)
	}
}

func TestIdentityJSONHasNoSecrets(t *testing.T) {
	data, err := json.Marshal(sample(t))
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ToLower(string(data))
	for _, banned := range []string{"password", "token", "secret", "api_key", "authorization", "http://", "https://"} {
		if strings.Contains(text, banned) {
			t.Fatalf("identity json contains %q: %s", banned, text)
		}
	}
}

func TestRelativePathRejected(t *testing.T) {
	if _, err := NewFileStore("identity.json"); err == nil {
		t.Fatal("expected error")
	}
}
