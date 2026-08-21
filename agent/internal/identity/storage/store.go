package storage

import (
	"errors"

	"risklens/agent/internal/identity/model"
)

var (
	ErrNotFound = errors.New("identity not found")
	ErrCorrupt  = errors.New("identity file corrupt")
)

// Store is the persistence port. FileStore is the current implementation;
// another backend can be added later without changing the manager.
type Store interface {
	Load() (model.Identity, error)
	Save(id model.Identity) error
	Delete() error
}
