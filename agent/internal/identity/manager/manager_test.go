package manager

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"risklens/agent/internal/identity/generator"
	"risklens/agent/internal/identity/storage"
)

func testInfo() generator.Info {
	return generator.Info{
		AgentVersion:    "0.1.0",
		DisplayName:     "RiskLens Endpoint Agent",
		MachineName:     "HOST1",
		OperatingSystem: "windows/amd64",
	}
}

func newMgr(t *testing.T) (*Manager, *storage.FileStore) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "identity.json")
	store, err := storage.NewFileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	mgr, err := New(store, testInfo(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return mgr, store
}

func TestLoadOrCreatePersists(t *testing.T) {
	mgr, store := newMgr(t)
	first, err := mgr.LoadOrCreate()
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.Validate(first); err != nil {
		t.Fatal(err)
	}
	current, ok := mgr.Current()
	if !ok || current.AgentID != first.AgentID {
		t.Fatalf("current = %+v ok=%v", current, ok)
	}

	mgr2, err := New(store, testInfo(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	second, err := mgr2.LoadOrCreate()
	if err != nil {
		t.Fatal(err)
	}
	if second.AgentID != first.AgentID || second.InstallID != first.InstallID {
		t.Fatalf("identity regenerated: first=%s second=%s", first.AgentID, second.AgentID)
	}
}

func TestMissingFileCreatesOnce(t *testing.T) {
	mgr, _ := newMgr(t)
	_, err := mgr.Load()
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("Load missing: %v", err)
	}
	created, err := mgr.Create()
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := mgr.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.AgentID != created.AgentID {
		t.Fatal("load after create changed agent_id")
	}
}

func TestCorruptFileDoesNotRegenerate(t *testing.T) {
	mgr, store := newMgr(t)
	if _, err := mgr.Create(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.Path(), []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := mgr.LoadOrCreate()
	if !errors.Is(err, storage.ErrCorrupt) {
		t.Fatalf("expected corrupt, got %v", err)
	}
}

func TestResetCreatesNewIdentity(t *testing.T) {
	mgr, _ := newMgr(t)
	first, err := mgr.LoadOrCreate()
	if err != nil {
		t.Fatal(err)
	}
	second, err := mgr.Reset()
	if err != nil {
		t.Fatal(err)
	}
	if second.AgentID == first.AgentID || second.InstallID == first.InstallID {
		t.Fatal("reset reused identity")
	}
	current, ok := mgr.Current()
	if !ok || current.AgentID != second.AgentID {
		t.Fatal("current after reset is wrong")
	}
	reloaded, err := mgr.Load()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.AgentID != second.AgentID {
		t.Fatal("persisted identity after reset is wrong")
	}
}

func TestConcurrentCurrentReads(t *testing.T) {
	mgr, _ := newMgr(t)
	id, err := mgr.LoadOrCreate()
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, ok := mgr.Current()
			if !ok {
				errCh <- errors.New("current missing")
				return
			}
			if got.AgentID != id.AgentID {
				errCh <- errors.New("agent_id changed during concurrent read")
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}
}

func TestNewRequiresStore(t *testing.T) {
	_, err := New(nil, testInfo(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSaveRejectsInvalid(t *testing.T) {
	mgr, _ := newMgr(t)
	id, err := mgr.Create()
	if err != nil {
		t.Fatal(err)
	}
	id.AgentID = "bad"
	if err := mgr.Save(id); err == nil {
		t.Fatal("expected validation error")
	}
}
