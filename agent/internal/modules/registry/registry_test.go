package registry

import (
	"context"
	"errors"
	"testing"

	"risklens/agent/internal/modules/base"
	"risklens/agent/internal/modules/interfaces"
)

type stub struct {
	*base.Module
}

func (s *stub) Start(context.Context) error { return nil }
func (s *stub) Stop(context.Context) error  { return nil }

func newStub(name, version string) interfaces.Module {
	return &stub{Module: base.New(name, version)}
}

func TestRegisterAndOrder(t *testing.T) {
	reg := New()
	if err := reg.Register(newStub("alpha", "1.0.0")); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(newStub("beta", "1.0.0")); err != nil {
		t.Fatal(err)
	}
	if reg.Len() != 2 {
		t.Fatalf("Len = %d", reg.Len())
	}
	mods := reg.Modules()
	if mods[0].Name() != "alpha" || mods[1].Name() != "beta" {
		t.Fatalf("order = %s, %s", mods[0].Name(), mods[1].Name())
	}
	if _, ok := reg.Get("BETA"); !ok {
		t.Fatal("Get is case-insensitive")
	}
}

func TestDuplicateRejected(t *testing.T) {
	reg := New()
	if err := reg.Register(newStub("eventlog", "1.0.0")); err != nil {
		t.Fatal(err)
	}
	err := reg.Register(newStub("EventLog", "2.0.0"))
	if !errors.Is(err, ErrDuplicate) {
		t.Fatalf("expected ErrDuplicate, got %v", err)
	}
	if reg.Len() != 1 {
		t.Fatalf("Len = %d", reg.Len())
	}
}

func TestNilAndInvalidRejected(t *testing.T) {
	reg := New()
	if err := reg.Register(nil); !errors.Is(err, ErrNilModule) {
		t.Fatalf("nil: %v", err)
	}
	if err := reg.Register(newStub(" ", "1.0.0")); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("empty name: %v", err)
	}
	if err := reg.Register(newStub("net", " ")); !errors.Is(err, ErrInvalidVersion) {
		t.Fatalf("empty version: %v", err)
	}
}

func TestModulesReturnsCopy(t *testing.T) {
	reg := New()
	if err := reg.Register(newStub("alpha", "1.0.0")); err != nil {
		t.Fatal(err)
	}
	mods := reg.Modules()
	mods[0] = newStub("mutated", "9.9.9")
	got := reg.Modules()
	if got[0].Name() != "alpha" {
		t.Fatal("Modules must return a copy")
	}
}
