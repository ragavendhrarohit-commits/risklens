package validation

import (
	"errors"
	"testing"
	"time"

	"risklens/agent/internal/identity/model"
)

func validID() model.Identity {
	now := time.Now().UTC()
	return model.Identity{
		SchemaVersion:   model.SchemaVersion,
		AgentID:         "550e8400-e29b-41d4-a716-446655440000",
		InstallID:       "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
		AgentVersion:    "0.1.0",
		CreatedAt:       now.Add(-time.Hour),
		UpdatedAt:       now,
		Status:          model.StatusActive,
		DisplayName:     "RiskLens Endpoint Agent",
		MachineName:     "HOST1",
		OperatingSystem: "windows/amd64",
	}
}

func TestValidateOK(t *testing.T) {
	if err := Validate(validID()); err != nil {
		t.Fatal(err)
	}
}

func TestValidateMissingAndInvalid(t *testing.T) {
	id := validID()
	id.AgentID = "not-a-uuid"
	if err := Validate(id); !errors.Is(err, ErrInvalidUUID) {
		t.Fatalf("bad uuid: %v", err)
	}

	id = validID()
	id.AgentID = ""
	if err := Validate(id); !errors.Is(err, ErrMissingField) {
		t.Fatalf("missing agent_id: %v", err)
	}

	id = validID()
	id.SchemaVersion = 99
	if err := Validate(id); !errors.Is(err, ErrIncompatibleVersion) {
		t.Fatalf("future schema: %v", err)
	}

	id = validID()
	id.SchemaVersion = 0
	if err := Validate(id); !errors.Is(err, ErrMissingField) {
		t.Fatalf("schema 0: %v", err)
	}

	id = validID()
	id.Status = "unknown"
	if err := Validate(id); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("status: %v", err)
	}

	id = validID()
	id.AgentID = id.InstallID
	if err := Validate(id); !errors.Is(err, ErrInvalidUUID) {
		t.Fatalf("same ids: %v", err)
	}
}
