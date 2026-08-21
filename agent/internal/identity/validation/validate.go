package validation

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"risklens/agent/internal/identity/model"
)

var (
	ErrInvalidUUID         = errors.New("invalid uuid")
	ErrMissingField        = errors.New("required field missing")
	ErrIncompatibleVersion = errors.New("incompatible identity schema version")
	ErrInvalidStatus       = errors.New("invalid identity status")
)

var uuidRE = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func Validate(id model.Identity) error {
	if id.SchemaVersion <= 0 {
		return fmt.Errorf("%w: schema_version is required", ErrMissingField)
	}
	if id.SchemaVersion > model.SchemaVersion {
		return fmt.Errorf("%w: got %d, supported %d", ErrIncompatibleVersion, id.SchemaVersion, model.SchemaVersion)
	}
	if err := requireUUID("agent_id", id.AgentID); err != nil {
		return err
	}
	if err := requireUUID("install_id", id.InstallID); err != nil {
		return err
	}
	if id.AgentID == id.InstallID {
		return fmt.Errorf("%w: agent_id and install_id must differ", ErrInvalidUUID)
	}
	if strings.TrimSpace(id.AgentVersion) == "" {
		return fmt.Errorf("%w: agent_version", ErrMissingField)
	}
	if id.CreatedAt.IsZero() {
		return fmt.Errorf("%w: created_at", ErrMissingField)
	}
	if id.UpdatedAt.IsZero() {
		return fmt.Errorf("%w: updated_at", ErrMissingField)
	}
	if id.UpdatedAt.Before(id.CreatedAt) {
		return fmt.Errorf("%w: updated_at is before created_at", ErrMissingField)
	}
	if strings.TrimSpace(string(id.Status)) == "" {
		return fmt.Errorf("%w: status", ErrMissingField)
	}
	if id.Status != model.StatusActive {
		return fmt.Errorf("%w: %s", ErrInvalidStatus, id.Status)
	}
	if strings.TrimSpace(id.DisplayName) == "" {
		return fmt.Errorf("%w: display_name", ErrMissingField)
	}
	if strings.TrimSpace(id.MachineName) == "" {
		return fmt.Errorf("%w: machine_name", ErrMissingField)
	}
	if strings.TrimSpace(id.OperatingSystem) == "" {
		return fmt.Errorf("%w: operating_system", ErrMissingField)
	}
	return nil
}

func IsUUID(value string) bool {
	return uuidRE.MatchString(strings.TrimSpace(value))
}

func requireUUID(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%w: %s", ErrMissingField, field)
	}
	if !IsUUID(value) {
		return fmt.Errorf("%w: %s", ErrInvalidUUID, field)
	}
	return nil
}
