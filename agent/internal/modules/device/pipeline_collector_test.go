package device

import (
	"context"
	"errors"
	"testing"
	"time"

	"risklens/agent/internal/fingerprint"
	"risklens/agent/internal/pipeline"
)

func TestNewPipelineCollectorRejectsNil(t *testing.T) {
	if _, err := NewPipelineCollector(nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestPipelineCollectorKind(t *testing.T) {
	m, err := NewWithCollector(Config{Enabled: true}, &stubCollector{info: fullInfo()}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewPipelineCollector(m)
	if err != nil {
		t.Fatal(err)
	}
	if c.Kind() != "device" {
		t.Fatalf("kind = %q", c.Kind())
	}
}

func TestPipelineCollectorCallsRefresh(t *testing.T) {
	stub := &stubCollector{info: fullInfo()}
	m, err := NewWithCollector(Config{Enabled: true}, stub, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewPipelineCollector(m)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := c.Collect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if stub.calls() != 1 {
		t.Fatalf("Refresh/Collect calls = %d, want 1", stub.calls())
	}
}

func TestPipelineCollectorReturnsPipelineSnapshotMatchingRefresh(t *testing.T) {
	stub := &stubCollector{info: fullInfo()}
	m, err := NewWithCollector(Config{Enabled: true}, stub, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewPipelineCollector(m)
	if err != nil {
		t.Fatal(err)
	}

	out, err := c.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	dev := m.Inventory()

	if out.Kind != "device" || out.SchemaVersion != pipeline.SchemaVersionV1 {
		t.Fatalf("envelope = %+v", out)
	}
	if !out.CollectedAt.Equal(dev.SampleTime()) {
		t.Fatalf("CollectedAt adapter=%v sample=%v", out.CollectedAt, dev.SampleTime())
	}
	info, ok := out.Payload.(Info)
	if !ok {
		t.Fatalf("payload type %T", out.Payload)
	}
	if info != dev.ComparablePayload() {
		t.Fatalf("payload != Device comparable payload\nadapter %+v\nwant %+v", info, dev.ComparablePayload())
	}
	if out.CollectError != dev.LastError {
		t.Fatalf("CollectError = %q LastError = %q", out.CollectError, dev.LastError)
	}
	if out.Partial {
		t.Fatal("full collect must not be partial")
	}
}

func TestPipelineCollectorDoesNotFingerprint(t *testing.T) {
	m, err := NewWithCollector(Config{Enabled: true}, &stubCollector{info: fullInfo()}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewPipelineCollector(m)
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !out.Fingerprint.Empty() {
		t.Fatalf("adapter must not hash, fingerprint = %s", out.Fingerprint)
	}
}

func TestPipelineCollectorDoesNotUpload(t *testing.T) {
	stub := &stubCollector{info: fullInfo()}
	m, err := NewWithCollector(Config{Enabled: true}, stub, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewPipelineCollector(m)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Collect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if stub.calls() != 1 {
		t.Fatalf("only Refresh may run, calls = %d", stub.calls())
	}
}

func TestPipelineCollectorPropagatesCollectionErrors(t *testing.T) {
	partial := Info{
		MachineName:     "HOST",
		Hostname:        "HOST",
		OperatingSystem: "Windows 11 Pro",
		Architecture:    "x86_64",
		AgentVersion:    "0.1.0",
		CollectedAt:     time.Date(2026, 8, 14, 2, 0, 0, 0, time.UTC),
	}
	m, err := NewWithCollector(
		Config{Enabled: true},
		&stubCollector{info: partial, err: errors.New("cpu registry missing")},
		discardLogger(),
	)
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewPipelineCollector(m)
	if err != nil {
		t.Fatal(err)
	}

	out, collectErr := c.Collect(context.Background())
	if collectErr == nil {
		t.Fatal("expected collection error")
	}
	if collectErr.Error() != "cpu registry missing" {
		t.Fatalf("err = %v", collectErr)
	}
	if !out.Partial {
		t.Fatal("expected partial pipeline snapshot")
	}
	info := out.Payload.(Info)
	if info != m.Inventory().ComparablePayload() {
		t.Fatal("partial payload must match Device comparable payload")
	}
	if out.CollectError != m.Inventory().LastError {
		t.Fatalf("CollectError = %q", out.CollectError)
	}
}

func TestPipelineCollectorFailedCollectStillMatchesRefresh(t *testing.T) {
	m, err := NewWithCollector(
		Config{Enabled: true},
		&stubCollector{err: errors.New("everything failed")},
		discardLogger(),
	)
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewPipelineCollector(m)
	if err != nil {
		t.Fatal(err)
	}

	out, collectErr := c.Collect(context.Background())
	if collectErr == nil {
		t.Fatal("expected error")
	}
	dev := m.Inventory()
	if collectErr.Error() != dev.LastError {
		t.Fatalf("err = %v last = %q", collectErr, dev.LastError)
	}
	got := out.Payload.(Info)
	if got != dev.ComparablePayload() {
		t.Fatalf("failed payload mismatch: %+v vs %+v", got, dev.ComparablePayload())
	}
}

func TestPipelineCollectorDoesNotMutateDeviceSnapshot(t *testing.T) {
	m, err := NewWithCollector(Config{Enabled: true}, &stubCollector{info: fullInfo()}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewPipelineCollector(m)
	if err != nil {
		t.Fatal(err)
	}

	out, err := c.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	before := m.Inventory()

	info := out.Payload.(Info)
	info.MachineName = "MUTATED"
	out.Payload = info
	out.CollectError = "mutated"
	out.Partial = true

	after := m.Inventory()
	if after.Info != before.Info || after.LastError != before.LastError || after.State != before.State {
		t.Fatalf("device snapshot mutated: %+v", after)
	}
	if after.Info.MachineName != "DESKTOP-01" {
		t.Fatalf("MachineName = %q", after.Info.MachineName)
	}
}

func TestPipelineCollectorPayloadFingerprintIgnoresCollectedAt(t *testing.T) {
	first := fullInfo()
	first.CollectedAt = time.Date(2026, 8, 14, 1, 0, 0, 0, time.UTC)
	second := first
	second.CollectedAt = time.Date(2026, 8, 14, 2, 0, 0, 0, time.UTC)

	m, err := NewWithCollector(Config{Enabled: true}, &sequenceCollector{
		steps: []collectStep{{info: first}, {info: second}},
	}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewPipelineCollector(m)
	if err != nil {
		t.Fatal(err)
	}

	out1, err := c.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	out2, err := c.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	fp1 := fingerprint.Of(out1.Kind, out1.SchemaVersion, out1.Partial, out1.Payload)
	fp2 := fingerprint.Of(out2.Kind, out2.SchemaVersion, out2.Partial, out2.Payload)
	if fp1.Empty() || !fp1.Equal(fp2) {
		t.Fatalf("fingerprints differ: %s vs %s", fp1, fp2)
	}
	if out1.CollectedAt.Equal(out2.CollectedAt) {
		t.Fatal("envelope CollectedAt should still change")
	}
}
