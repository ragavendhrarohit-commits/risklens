//go:build windows

package device

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"

	"risklens/agent/internal/modules/interfaces"
)

func TestWindowsCollectorCollectsCoreInventory(t *testing.T) {
	collector := &WindowsCollector{
		AgentVersion: "0.1.0-test",
		Now:          func() time.Time { return time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC) },
	}

	info, err := collector.Collect(context.Background())
	if info.IsEmpty() {
		t.Fatalf("expected inventory, got empty (err=%v)", err)
	}

	// These must always be obtainable on a real Windows host.
	required := map[string]string{
		"Hostname":        info.Hostname,
		"MachineName":     info.MachineName,
		"OperatingSystem": info.OperatingSystem,
		"Architecture":    info.Architecture,
		"AgentVersion":    info.AgentVersion,
	}
	for name, value := range required {
		if strings.TrimSpace(value) == "" {
			t.Errorf("%s is empty", name)
		}
	}
	if info.AgentVersion != "0.1.0-test" {
		t.Errorf("AgentVersion = %q", info.AgentVersion)
	}
	if info.CPUCores <= 0 {
		t.Errorf("CPUCores = %d", info.CPUCores)
	}
	if info.MemoryBytes == 0 {
		t.Error("MemoryBytes is 0")
	}
	if info.DiskSizeBytes == 0 {
		t.Error("DiskSizeBytes is 0")
	}
	if info.BootTime.IsZero() {
		t.Error("BootTime is zero")
	}
	if info.CollectedAt.IsZero() {
		t.Error("CollectedAt is zero")
	}

	// Architecture should match the process architecture on this host.
	if info.Architecture != "x86_64" && info.Architecture != "arm64" && info.Architecture != runtime.GOARCH {
		t.Errorf("Architecture = %q", info.Architecture)
	}
}

func TestWindowsCollectorHonoursCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (&WindowsCollector{AgentVersion: "0.1.0"}).Collect(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestWindowsCollectorMissingValuesDoNotPanic(t *testing.T) {
	// Pointing at a non-existent volume forces a disk fallback path while
	// every other probe still runs. The collector must return what it can.
	collector := &WindowsCollector{
		AgentVersion: "0.1.0",
		SystemDrive:  `Z:\risklens-missing-volume\`,
	}
	info, err := collector.Collect(context.Background())
	if info.IsEmpty() {
		t.Fatalf("expected partial inventory, got empty (err=%v)", err)
	}
	if info.DiskSizeBytes != 0 {
		t.Errorf("DiskSizeBytes = %d, want 0 for missing volume", info.DiskSizeBytes)
	}
	if err == nil {
		t.Fatal("expected a partial-collection error for the missing volume")
	}
	if !strings.Contains(err.Error(), "partial") && !strings.Contains(err.Error(), "GetDiskFreeSpaceEx") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDetectArchitectureFallback(t *testing.T) {
	arch := detectArchitecture()
	if arch == "" {
		t.Fatal("architecture empty")
	}
}

func TestEnsureTrailingSlash(t *testing.T) {
	cases := map[string]string{
		`C:`:         `C:\`,
		`C:\`:        `C:\`,
		`D:/`:        `D:/`,
		``:           `C:\`,
		`C:\Windows`: `C:\Windows\`,
	}
	for in, want := range cases {
		if got := ensureTrailingSlash(in); got != want {
			t.Errorf("ensureTrailingSlash(%q)=%q want %q", in, got, want)
		}
	}
}

func TestSystemDriveIsAbsolute(t *testing.T) {
	drive := systemDrive()
	if len(drive) < 3 || drive[1] != ':' {
		t.Fatalf("systemDrive = %q", drive)
	}
}

func TestModuleWithRealWindowsCollector(t *testing.T) {
	m, err := New(Config{Enabled: true}, "0.1.0", discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	snap := m.Inventory()
	switch snap.State {
	case StateCollected, StatePartial:
	default:
		t.Fatalf("state = %q err=%q", snap.State, snap.LastError)
	}
	if m.Info().Hostname == "" {
		t.Fatal("hostname missing from module inventory")
	}
	if m.Status() != interfaces.StatusRunning {
		t.Fatalf("status = %q", m.Status())
	}
}
