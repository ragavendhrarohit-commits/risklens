package base

import (
	"sync"
	"testing"

	"risklens/agent/internal/modules/interfaces"
)

func TestInitialState(t *testing.T) {
	m := New("eventlog", "1.0.0")
	if m.Name() != "eventlog" {
		t.Fatalf("Name = %q", m.Name())
	}
	if m.Version() != "1.0.0" {
		t.Fatalf("Version = %q", m.Version())
	}
	if m.Status() != interfaces.StatusRegistered {
		t.Fatalf("Status = %q", m.Status())
	}
	if m.Health() != interfaces.HealthUnknown {
		t.Fatalf("Health = %q", m.Health())
	}
}

func TestLifecycleMarks(t *testing.T) {
	m := New("proc", "0.1.0")
	m.MarkStarting()
	if m.Status() != interfaces.StatusStarting {
		t.Fatalf("starting status = %q", m.Status())
	}
	m.MarkRunning()
	if m.Status() != interfaces.StatusRunning || m.Health() != interfaces.HealthHealthy {
		t.Fatalf("running status=%q health=%q", m.Status(), m.Health())
	}
	m.MarkStopping()
	if m.Status() != interfaces.StatusStopping {
		t.Fatalf("stopping status = %q", m.Status())
	}
	m.MarkStopped()
	if m.Status() != interfaces.StatusStopped {
		t.Fatalf("stopped status = %q", m.Status())
	}
	m.MarkFailed()
	if m.Status() != interfaces.StatusFailed || m.Health() != interfaces.HealthUnhealthy {
		t.Fatalf("failed status=%q health=%q", m.Status(), m.Health())
	}
}

func TestConcurrentStatus(t *testing.T) {
	m := New("safe", "1.0.0")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.MarkRunning()
			_ = m.Status()
			_ = m.Health()
			m.MarkStopped()
		}()
	}
	wg.Wait()
}
