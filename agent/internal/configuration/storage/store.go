package storage

import (
	"errors"

	"risklens/agent/internal/configuration/model"
)

var (
	ErrNotFound = errors.New("configuration not found")
	ErrCorrupt  = errors.New("configuration file corrupt")
)

// Store is the persistence port. YAML is the current adapter; remote,
// database, encrypted, or registry stores can be added later.
type Store interface {
	Load() (model.Config, error)
	Save(cfg model.Config) error
	Path() string
}
