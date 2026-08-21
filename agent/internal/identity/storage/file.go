package storage

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"risklens/agent/internal/identity/model"
)

const maxIdentityBytes = 64 << 10

type FileStore struct {
	path string
}

func NewFileStore(path string) (*FileStore, error) {
	if path == "" {
		return nil, fmt.Errorf("identity path is required")
	}
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("identity path must be absolute")
	}
	return &FileStore{path: filepath.Clean(path)}, nil
}

func DefaultPath() string {
	base := os.Getenv("PROGRAMDATA")
	if base == "" {
		base = `C:\ProgramData`
	}
	return filepath.Join(base, "RiskLens", "state", "identity.json")
}

func (s *FileStore) Path() string {
	return s.path
}

func (s *FileStore) Load() (model.Identity, error) {
	f, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return model.Identity{}, ErrNotFound
		}
		return model.Identity{}, fmt.Errorf("%w: %w", ErrCorrupt, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return model.Identity{}, fmt.Errorf("%w: %w", ErrCorrupt, err)
	}
	if info.IsDir() {
		return model.Identity{}, fmt.Errorf("%w: path is a directory", ErrCorrupt)
	}
	if info.Size() > maxIdentityBytes {
		return model.Identity{}, fmt.Errorf("%w: file exceeds %d bytes", ErrCorrupt, maxIdentityBytes)
	}

	data, err := io.ReadAll(io.LimitReader(f, maxIdentityBytes+1))
	if err != nil {
		return model.Identity{}, fmt.Errorf("%w: %w", ErrCorrupt, err)
	}
	if len(data) == 0 || len(data) > maxIdentityBytes {
		return model.Identity{}, fmt.Errorf("%w: invalid file size", ErrCorrupt)
	}

	var id model.Identity
	if err := json.Unmarshal(data, &id); err != nil {
		return model.Identity{}, fmt.Errorf("%w: %w", ErrCorrupt, err)
	}
	return id, nil
}

func (s *FileStore) Save(id model.Identity) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(id, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(s.path)
		if err := os.Rename(tmp, s.path); err != nil {
			_ = os.Remove(tmp)
			return err
		}
	}
	return nil
}

func (s *FileStore) Delete() error {
	err := os.Remove(s.path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	_ = os.Remove(s.path + ".tmp")
	return nil
}
