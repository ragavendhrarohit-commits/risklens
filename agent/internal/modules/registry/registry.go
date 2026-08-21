package registry

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"risklens/agent/internal/modules/interfaces"
)

var (
	ErrNilModule      = errors.New("module is nil")
	ErrDuplicate      = errors.New("duplicate module")
	ErrInvalidName    = errors.New("module name is required")
	ErrInvalidVersion = errors.New("module version is required")
)

type Registry struct {
	mu      sync.RWMutex
	modules []interfaces.Module
}

func New() *Registry {
	return &Registry{}
}

func (r *Registry) Register(module interfaces.Module) error {
	if r == nil {
		return errors.New("registry is nil")
	}
	if err := Validate(module); err != nil {
		return err
	}

	name := strings.TrimSpace(module.Name())
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.modules {
		if strings.EqualFold(strings.TrimSpace(existing.Name()), name) {
			return fmt.Errorf("%w: %s", ErrDuplicate, name)
		}
	}
	r.modules = append(r.modules, module)
	return nil
}

func (r *Registry) Modules() []interfaces.Module {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]interfaces.Module, len(r.modules))
	copy(out, r.modules)
	return out
}

func (r *Registry) Get(name string) (interfaces.Module, bool) {
	if r == nil {
		return nil, false
	}
	name = strings.TrimSpace(name)
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, module := range r.modules {
		if strings.EqualFold(strings.TrimSpace(module.Name()), name) {
			return module, true
		}
	}
	return nil, false
}

func (r *Registry) Len() int {
	if r == nil {
		return 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.modules)
}

func Validate(module interfaces.Module) error {
	if module == nil {
		return ErrNilModule
	}
	if strings.TrimSpace(module.Name()) == "" {
		return ErrInvalidName
	}
	if strings.TrimSpace(module.Version()) == "" {
		return ErrInvalidVersion
	}
	return nil
}
