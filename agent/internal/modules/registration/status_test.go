package registration

import "testing"

func TestStateConstantsAreStable(t *testing.T) {
	if StateNotRegistered != "not_registered" {
		t.Fatalf("not_registered=%q", StateNotRegistered)
	}
	if StateRegistering != "registering" {
		t.Fatalf("registering=%q", StateRegistering)
	}
	if StateRegistered != "registered" {
		t.Fatalf("registered=%q", StateRegistered)
	}
	if StateFailed != "failed" {
		t.Fatalf("failed=%q", StateFailed)
	}
}

func TestSnapshotZeroValueIsSafe(t *testing.T) {
	var snap Snapshot
	if snap.State != "" {
		t.Fatalf("zero snapshot has state %q", snap.State)
	}
	if snap.HTTPStatusCode != 0 {
		t.Fatalf("zero snapshot has status %d", snap.HTTPStatusCode)
	}
}
