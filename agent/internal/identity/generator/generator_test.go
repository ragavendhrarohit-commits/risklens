package generator

import (
	"testing"

	"risklens/agent/internal/identity/validation"
)

func TestNewUUIDv4(t *testing.T) {
	seen := map[string]struct{}{}
	for i := 0; i < 8; i++ {
		id, err := NewUUIDv4()
		if err != nil {
			t.Fatal(err)
		}
		if !validation.IsUUID(id) {
			t.Fatalf("not a uuid: %s", id)
		}
		if _, ok := seen[id]; ok {
			t.Fatalf("duplicate uuid %s", id)
		}
		seen[id] = struct{}{}
	}
}

func TestNewIdentity(t *testing.T) {
	id, err := NewIdentity(Info{
		AgentVersion:    "0.1.0",
		DisplayName:     "RiskLens Endpoint Agent",
		MachineName:     "HOST1",
		OperatingSystem: "windows/amd64",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := validation.Validate(id); err != nil {
		t.Fatal(err)
	}
	if id.AgentID == id.InstallID {
		t.Fatal("agent_id and install_id must differ")
	}
	if id.AgentVersion != "0.1.0" || id.MachineName != "HOST1" {
		t.Fatalf("metadata = %+v", id)
	}
}

func TestApplyInfoDoesNotChangeIDs(t *testing.T) {
	id, err := NewIdentity(Info{AgentVersion: "0.1.0", DisplayName: "A", MachineName: "H", OperatingSystem: "windows/amd64"})
	if err != nil {
		t.Fatal(err)
	}
	next := ApplyInfo(id, Info{AgentVersion: "0.2.0", DisplayName: "A", MachineName: "H", OperatingSystem: "windows/amd64"})
	if next.AgentID != id.AgentID || next.InstallID != id.InstallID || !next.CreatedAt.Equal(id.CreatedAt) {
		t.Fatal("identity keys changed")
	}
	if next.AgentVersion != "0.2.0" {
		t.Fatalf("version = %s", next.AgentVersion)
	}
	if !next.UpdatedAt.After(id.UpdatedAt) && !next.UpdatedAt.Equal(id.UpdatedAt) {
		t.Fatal("updated_at should move when metadata changes")
	}
}
