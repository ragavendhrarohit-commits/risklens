package device

import (
	"testing"
	"time"

	"risklens/agent/internal/modules/interfaces"
)

func TestInfoIsEmpty(t *testing.T) {
	if !(Info{}).IsEmpty() {
		t.Fatal("zero Info should be empty")
	}
	if (Info{MachineName: "X"}).IsEmpty() {
		t.Fatal("Info with MachineName should not be empty")
	}
}

func TestInfoMemoryAndDiskHelpers(t *testing.T) {
	info := Info{
		MemoryBytes:   2 * 1024 * 1024 * 1024,
		DiskSizeBytes: 4 * 1024 * 1024 * 1024,
		DiskFreeBytes: 1 * 1024 * 1024 * 1024,
	}
	if info.MemoryGiB() != 2 {
		t.Fatalf("MemoryGiB = %v", info.MemoryGiB())
	}
	if info.DiskSizeGiB() != 4 {
		t.Fatalf("DiskSizeGiB = %v", info.DiskSizeGiB())
	}
	if info.DiskFreeGiB() != 1 {
		t.Fatalf("DiskFreeGiB = %v", info.DiskFreeGiB())
	}
	if (Info{}).MemoryGiB() != 0 || (Info{}).DiskSizeGiB() != 0 {
		t.Fatal("zero helpers should return 0")
	}
}

func TestInfoComparableOmitsCollectedAt(t *testing.T) {
	info := fullInfo()
	got := info.Comparable()
	if !got.CollectedAt.IsZero() {
		t.Fatal("comparable payload must omit CollectedAt")
	}
	if got.MachineName != info.MachineName || got.DiskFreeBytes != info.DiskFreeBytes {
		t.Fatalf("comparable dropped OS facts: %+v", got)
	}
	if info.CollectedAt.IsZero() {
		t.Fatal("original CollectedAt must be unchanged")
	}
}

func TestInfoForTransportRestoresCollectedAt(t *testing.T) {
	info := fullInfo().Comparable()
	sample := time.Date(2026, 8, 20, 11, 0, 0, 0, time.UTC)
	got := info.ForTransport(sample)
	if !got.CollectedAt.Equal(sample) {
		t.Fatalf("CollectedAt = %v", got.CollectedAt)
	}
	full := fullInfo()
	if !full.ForTransport(sample).CollectedAt.Equal(full.CollectedAt) {
		t.Fatal("existing CollectedAt must win")
	}
}

func TestSnapshotCollectionHealth(t *testing.T) {
	if (Snapshot{State: StateCollected}).CollectionHealth() != interfaces.HealthHealthy {
		t.Fatal("collected must be healthy")
	}
	if (Snapshot{State: StatePartial}).CollectionHealth() != interfaces.HealthDegraded {
		t.Fatal("partial must be degraded")
	}
	if (Snapshot{State: StateFailed}).CollectionHealth() != interfaces.HealthUnhealthy {
		t.Fatal("failed must be unhealthy")
	}
}

func TestClassifyState(t *testing.T) {
	full := fullInfo()
	if got := classifyState(full, nil); got != StateCollected {
		t.Fatalf("full => %q", got)
	}

	partial := Info{MachineName: "A", Hostname: "A", AgentVersion: "1"}
	if got := classifyState(partial, nil); got != StatePartial {
		t.Fatalf("partial => %q", got)
	}
	if got := classifyState(partial, errSample); got != StatePartial {
		t.Fatalf("partial with err => %q", got)
	}
	if got := classifyState(Info{}, errSample); got != StateFailed {
		t.Fatalf("empty => %q", got)
	}
}

func TestCompletenessCountsPresentFields(t *testing.T) {
	present, total := completeness(fullInfo())
	if present != total {
		t.Fatalf("present=%d total=%d", present, total)
	}
	present, total = completeness(Info{})
	if present != 0 || total == 0 {
		t.Fatalf("empty present=%d total=%d", present, total)
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("  ", "", "alpha", "beta"); got != "alpha" {
		t.Fatalf("got %q", got)
	}
	if got := firstNonEmpty("", "  "); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestTruncateTime(t *testing.T) {
	in := time.Date(2026, 8, 14, 12, 34, 56, 789, time.FixedZone("x", 3600))
	out := truncateTime(in)
	if out.Location() != time.UTC {
		t.Fatalf("location = %v", out.Location())
	}
	if out.Nanosecond() != 0 {
		t.Fatalf("nanos = %d", out.Nanosecond())
	}
	if !truncateTime(time.Time{}).IsZero() {
		t.Fatal("zero should stay zero")
	}
}

func TestSanitizeHardwareField(t *testing.T) {
	cases := map[string]string{
		"Dell Inc.":              "Dell Inc.",
		"  ":                     "",
		"To Be Filled By O.E.M.": "",
		"Default string":         "",
		"System Serial Number":   "",
		"None":                   "",
		"N/A":                    "",
		"Latitude 5520":          "Latitude 5520",
	}
	for in, want := range cases {
		if got := sanitizeHardwareField(in); got != want {
			t.Errorf("sanitize(%q)=%q want %q", in, got, want)
		}
	}
}

// errSample is a package-level sentinel used by classify tests.
var errSample = errString("sample")

type errString string

func (e errString) Error() string { return string(e) }
