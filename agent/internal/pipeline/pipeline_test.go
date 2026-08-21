package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strconv"
	"sync"
	"testing"
	"time"

	"risklens/agent/internal/fingerprint"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func sampleSnapshot(kind, fp string, payload any) Snapshot {
	return Snapshot{
		Kind:          kind,
		SchemaVersion: SchemaVersionV1,
		CollectedAt:   time.Date(2026, 8, 20, 7, 0, 0, 0, time.UTC),
		Fingerprint:   Fingerprint{Value: fp},
		Payload:       payload,
	}
}

func computedFP(kind string, payload any) Fingerprint {
	return fingerprint.Of(kind, SchemaVersionV1, false, payload)
}

type fakeCollector struct {
	kind string
	mu   sync.Mutex
	n    int
	fn   func(ctx context.Context, n int) (Snapshot, error)
}

func (f *fakeCollector) Kind() string { return f.kind }

func (f *fakeCollector) Collect(ctx context.Context) (Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	f.mu.Lock()
	n := f.n
	f.n++
	fn := f.fn
	f.mu.Unlock()
	if fn == nil {
		return Snapshot{}, errors.New("no collect fn")
	}
	return fn(ctx, n)
}

func (f *fakeCollector) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.n
}

type fakeUploader struct {
	mu    sync.Mutex
	calls int
	snaps []Snapshot
	errs  []error
	block time.Duration
}

func (f *fakeUploader) Upload(ctx context.Context, snap Snapshot) error {
	if f.block > 0 {
		timer := time.NewTimer(f.block)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	i := f.calls
	f.calls++
	f.snaps = append(f.snaps, snap)
	if i < len(f.errs) {
		return f.errs[i]
	}
	return nil
}

func (f *fakeUploader) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func waitUploads(t *testing.T, u *fakeUploader, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if u.count() >= n {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("uploads = %d, want >= %d", u.count(), n)
}

func TestNewRejectsMissingCollaborators(t *testing.T) {
	col := &fakeCollector{kind: "test"}
	up := &fakeUploader{}
	if _, err := New(Config{Interval: time.Second}, nil, up, testLogger()); !errors.Is(err, ErrNilCollector) {
		t.Fatalf("err = %v", err)
	}
	if _, err := New(Config{Interval: time.Second}, col, nil, testLogger()); !errors.Is(err, ErrNilUploader) {
		t.Fatalf("err = %v", err)
	}
	if _, err := New(Config{Interval: 0}, col, up, testLogger()); err == nil {
		t.Fatal("expected invalid interval")
	}
}

func TestFingerprintEqualAndEmpty(t *testing.T) {
	var empty Fingerprint
	if !empty.Empty() {
		t.Fatal("zero fingerprint should be empty")
	}
	a := Fingerprint{Value: "abc"}
	if !a.Equal(Fingerprint{Value: "abc"}) || a.Equal(Fingerprint{Value: "xyz"}) {
		t.Fatal("equal mismatch")
	}
}

func TestSnapshotJSONRoundTrip(t *testing.T) {
	in := sampleSnapshot("device", "fp1", map[string]any{"hostname": "pc"})
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out Snapshot
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out.Kind != "device" || !out.Fingerprint.Equal(Fingerprint{Value: "fp1"}) || out.SchemaVersion != SchemaVersionV1 {
		t.Fatalf("round trip = %+v", out)
	}
}

func TestSnapshotUploadable(t *testing.T) {
	ok := sampleSnapshot("k", "", "payload")
	if !ok.Uploadable() {
		t.Fatal("expected uploadable")
	}
	missingPayload := ok
	missingPayload.Payload = nil
	if missingPayload.Uploadable() {
		t.Fatal("nil payload must not be uploadable")
	}
}

func TestFirstTickUploads(t *testing.T) {
	col := &fakeCollector{kind: "alpha", fn: func(context.Context, int) (Snapshot, error) {
		return sampleSnapshot("alpha", "fp-a", "v1"), nil
	}}
	up := &fakeUploader{}
	p, err := New(Config{Interval: time.Hour}, col, up, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	if p.Health() != HealthDisabled {
		t.Fatalf("health before start = %s", p.Health())
	}
	p.Start(context.Background())
	defer p.Stop()
	waitUploads(t, up, 1)
	if p.Health() != HealthHealthy {
		t.Fatalf("health = %s", p.Health())
	}
	if !p.LastSuccess().Equal(computedFP("alpha", "v1")) {
		t.Fatalf("last success = %s", p.LastSuccess())
	}
}

func TestUnchangedFingerprintSkipsUpload(t *testing.T) {
	col := &fakeCollector{kind: "alpha", fn: func(context.Context, int) (Snapshot, error) {
		return sampleSnapshot("alpha", "same", "payload"), nil
	}}
	up := &fakeUploader{}
	p, err := New(Config{Interval: 25 * time.Millisecond}, col, up, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	p.Start(context.Background())
	defer p.Stop()
	waitUploads(t, up, 1)
	time.Sleep(80 * time.Millisecond)
	if up.count() != 1 {
		t.Fatalf("uploads = %d, want 1 (unchanged skipped)", up.count())
	}
}

func TestChangedFingerprintUploadsAgain(t *testing.T) {
	col := &fakeCollector{kind: "alpha", fn: func(_ context.Context, n int) (Snapshot, error) {
		fp := "first"
		if n > 0 {
			fp = "second"
		}
		return sampleSnapshot("alpha", fp, fp), nil
	}}
	up := &fakeUploader{}
	p, err := New(Config{Interval: 25 * time.Millisecond}, col, up, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	p.Start(context.Background())
	defer p.Stop()
	waitUploads(t, up, 2)
}

func TestEmptyCollectDoesNotUpload(t *testing.T) {
	col := &fakeCollector{kind: "alpha", fn: func(context.Context, int) (Snapshot, error) {
		return Snapshot{Kind: "alpha"}, errors.New("nothing collected")
	}}
	up := &fakeUploader{}
	p, err := New(Config{Interval: time.Hour}, col, up, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	p.Start(context.Background())
	time.Sleep(40 * time.Millisecond)
	p.Stop()
	if up.count() != 0 {
		t.Fatalf("uploads = %d", up.count())
	}
	if p.Health() != HealthDisabled {
		t.Fatalf("health after stop = %s", p.Health())
	}
}

func TestEmptyCollectHealthErrorWhileRunning(t *testing.T) {
	started := make(chan struct{})
	col := &fakeCollector{kind: "alpha", fn: func(context.Context, int) (Snapshot, error) {
		select {
		case <-started:
		default:
			close(started)
		}
		return Snapshot{}, errors.New("empty")
	}}
	up := &fakeUploader{}
	p, err := New(Config{Interval: time.Hour}, col, up, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	p.Start(context.Background())
	<-started
	time.Sleep(20 * time.Millisecond)
	if p.Health() != HealthError {
		t.Fatalf("health = %s", p.Health())
	}
	p.Stop()
}

func TestPartialSnapshotUploads(t *testing.T) {
	col := &fakeCollector{kind: "alpha", fn: func(context.Context, int) (Snapshot, error) {
		s := sampleSnapshot("alpha", "partial-fp", "half")
		s.Partial = true
		s.CollectError = "cpu missing"
		return s, errors.New("cpu missing")
	}}
	up := &fakeUploader{}
	p, err := New(Config{Interval: time.Hour}, col, up, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	p.Start(context.Background())
	defer p.Stop()
	waitUploads(t, up, 1)
	if p.Health() != HealthWarning {
		t.Fatalf("health = %s", p.Health())
	}
}

func TestUploadFailureRetriesSameFingerprint(t *testing.T) {
	col := &fakeCollector{kind: "alpha", fn: func(context.Context, int) (Snapshot, error) {
		return sampleSnapshot("alpha", "stable", "payload"), nil
	}}
	up := &fakeUploader{errs: []error{errors.New("timeout")}}
	p, err := New(Config{Interval: 25 * time.Millisecond}, col, up, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	p.Start(context.Background())
	defer p.Stop()
	waitUploads(t, up, 2)
	if !p.LastSuccess().Equal(computedFP("alpha", "payload")) {
		t.Fatalf("last success = %q", p.LastSuccess())
	}
	if p.Pending() {
		t.Fatal("pending should clear after successful retry")
	}
}

func TestPipelineDoesNotDeepEqualPayload(t *testing.T) {
	col := &fakeCollector{kind: "alpha", fn: func(_ context.Context, n int) (Snapshot, error) {
		s := sampleSnapshot("alpha", "ignored", "same-payload")
		s.CollectedAt = time.Date(2026, 8, 20, 7, 0, n, 0, time.UTC)
		return s, nil
	}}
	up := &fakeUploader{}
	p, err := New(Config{Interval: 25 * time.Millisecond}, col, up, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	p.Start(context.Background())
	defer p.Stop()
	waitUploads(t, up, 1)
	time.Sleep(80 * time.Millisecond)
	if up.count() != 1 {
		t.Fatalf("CollectedAt change uploaded %d times", up.count())
	}
}

func TestPanicIsRecoveredAndKindKeepsRunning(t *testing.T) {
	col := &fakeCollector{kind: "alpha", fn: func(_ context.Context, n int) (Snapshot, error) {
		if n == 0 {
			panic("collector boom")
		}
		return sampleSnapshot("alpha", "after-panic", "ok"), nil
	}}
	up := &fakeUploader{}
	p, err := New(Config{Interval: 25 * time.Millisecond}, col, up, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	p.Start(context.Background())
	defer p.Stop()
	waitUploads(t, up, 1)
}

func TestTwoPipelinesAreIsolated(t *testing.T) {
	slow := &fakeCollector{kind: "slow", fn: func(context.Context, int) (Snapshot, error) {
		return sampleSnapshot("slow", "slow-fp", "s"), nil
	}}
	fast := &fakeCollector{kind: "fast", fn: func(_ context.Context, n int) (Snapshot, error) {
		return sampleSnapshot("fast", "fast-"+strconv.Itoa(n), n), nil
	}}
	slowUp := &fakeUploader{block: 120 * time.Millisecond}
	fastUp := &fakeUploader{}

	slowPipe, err := New(Config{Interval: time.Hour}, slow, slowUp, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	fastPipe, err := New(Config{Interval: 20 * time.Millisecond}, fast, fastUp, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	slowPipe.Start(ctx)
	fastPipe.Start(ctx)
	defer slowPipe.Stop()
	defer fastPipe.Stop()

	waitUploads(t, fastUp, 3)
	waitUploads(t, slowUp, 1)
	if slowPipe.Health() == HealthDisabled {
		t.Fatal("slow pipeline should be running")
	}
}

func TestContextCancelDoesNotUpload(t *testing.T) {
	started := make(chan struct{})
	col := &fakeCollector{kind: "alpha", fn: func(ctx context.Context, _ int) (Snapshot, error) {
		close(started)
		<-ctx.Done()
		return Snapshot{}, ctx.Err()
	}}
	up := &fakeUploader{}
	p, err := New(Config{Interval: time.Hour}, col, up, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.Start(ctx)
	<-started
	cancel()
	p.Stop()
	if up.count() != 0 {
		t.Fatalf("uploads = %d", up.count())
	}
}

func TestKindsDoNotShareState(t *testing.T) {
	aCol := &fakeCollector{kind: "a", fn: func(context.Context, int) (Snapshot, error) {
		return sampleSnapshot("a", "fp-a", "a"), nil
	}}
	bCol := &fakeCollector{kind: "b", fn: func(context.Context, int) (Snapshot, error) {
		return Snapshot{Kind: "b"}, errors.New("empty")
	}}
	aUp := &fakeUploader{}
	bUp := &fakeUploader{}
	aPipe, err := New(Config{Interval: time.Hour}, aCol, aUp, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	bPipe, err := New(Config{Interval: time.Hour}, bCol, bUp, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	aPipe.Start(ctx)
	bPipe.Start(ctx)
	defer aPipe.Stop()
	defer bPipe.Stop()
	waitUploads(t, aUp, 1)
	time.Sleep(30 * time.Millisecond)
	if aPipe.Health() != HealthHealthy {
		t.Fatalf("a health = %s", aPipe.Health())
	}
	if bUp.count() != 0 {
		t.Fatal("b must not upload")
	}
}

func TestRunOnceExecutesExactlyOneCycle(t *testing.T) {
	col := &fakeCollector{kind: "alpha", fn: func(_ context.Context, n int) (Snapshot, error) {
		return sampleSnapshot("alpha", "fp-"+strconv.Itoa(n), n), nil
	}}
	up := &fakeUploader{}
	p, err := New(Config{Interval: 25 * time.Millisecond}, col, up, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	p.RunOnce(context.Background())

	if col.calls() != 1 {
		t.Fatalf("collects = %d, want 1", col.calls())
	}
	if up.count() != 1 {
		t.Fatalf("uploads = %d, want 1", up.count())
	}
	if !p.LastSuccess().Equal(computedFP("alpha", 0)) {
		t.Fatalf("last success = %s", p.LastSuccess())
	}
	if p.Health() != HealthHealthy {
		t.Fatalf("health = %s", p.Health())
	}
}

func TestRunOnceNeverStartsScheduler(t *testing.T) {
	col := &fakeCollector{kind: "alpha", fn: func(_ context.Context, n int) (Snapshot, error) {
		return sampleSnapshot("alpha", "fp-"+strconv.Itoa(n), n), nil
	}}
	up := &fakeUploader{}
	p, err := New(Config{Interval: 25 * time.Millisecond}, col, up, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	p.RunOnce(context.Background())
	time.Sleep(80 * time.Millisecond)

	if col.calls() != 1 {
		t.Fatalf("collects = %d, want 1 (scheduler must not be running)", col.calls())
	}
	if up.count() != 1 {
		t.Fatalf("uploads = %d, want 1", up.count())
	}
}

func TestRunOnceSecondCallSkipsUnchangedFingerprint(t *testing.T) {
	col := &fakeCollector{kind: "alpha", fn: func(context.Context, int) (Snapshot, error) {
		return sampleSnapshot("alpha", "same", "payload"), nil
	}}
	up := &fakeUploader{}
	p, err := New(Config{Interval: time.Hour}, col, up, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	p.RunOnce(context.Background())
	p.RunOnce(context.Background())

	if col.calls() != 2 {
		t.Fatalf("collects = %d, want 2", col.calls())
	}
	if up.count() != 1 {
		t.Fatalf("uploads = %d, want 1", up.count())
	}
}

func TestRunOnceDoesNotPreventLaterStart(t *testing.T) {
	col := &fakeCollector{kind: "alpha", fn: func(_ context.Context, n int) (Snapshot, error) {
		return sampleSnapshot("alpha", "fp-"+strconv.Itoa(n), n), nil
	}}
	up := &fakeUploader{}
	p, err := New(Config{Interval: 25 * time.Millisecond}, col, up, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	p.RunOnce(context.Background())
	p.Start(context.Background())
	defer p.Stop()
	waitUploads(t, up, 2)
}

func TestSchedulerInvokesRunOnceOnEachTick(t *testing.T) {
	col := &fakeCollector{kind: "alpha", fn: func(_ context.Context, n int) (Snapshot, error) {
		return sampleSnapshot("alpha", "fp-"+strconv.Itoa(n), n), nil
	}}
	up := &fakeUploader{}
	p, err := New(Config{Interval: 25 * time.Millisecond}, col, up, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	p.Start(context.Background())
	defer p.Stop()
	waitUploads(t, up, 2)
	if col.calls() < 2 {
		t.Fatalf("collects = %d, scheduler must invoke RunOnce each tick", col.calls())
	}
}

func TestPipelineIgnoresCollectorSuppliedFingerprint(t *testing.T) {
	col := &fakeCollector{kind: "alpha", fn: func(_ context.Context, n int) (Snapshot, error) {
		return sampleSnapshot("alpha", "collector-fp-"+strconv.Itoa(n), "same-payload"), nil
	}}
	up := &fakeUploader{}
	p, err := New(Config{Interval: time.Hour}, col, up, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	p.RunOnce(context.Background())
	p.RunOnce(context.Background())

	if up.count() != 1 {
		t.Fatalf("uploads = %d, pipeline must not use Snapshot.Fingerprint", up.count())
	}
	if !p.LastSuccess().Equal(computedFP("alpha", "same-payload")) {
		t.Fatalf("last success = %s", p.LastSuccess())
	}
}

func TestPipelineFingerprintIgnoresCollectedAt(t *testing.T) {
	col := &fakeCollector{kind: "alpha", fn: func(_ context.Context, n int) (Snapshot, error) {
		s := sampleSnapshot("alpha", "ignored", map[string]any{"hostname": "pc"})
		s.CollectedAt = time.Date(2026, 8, 20, 7, 0, n, 0, time.UTC)
		s.CollectError = "transient-" + strconv.Itoa(n)
		return s, nil
	}}
	up := &fakeUploader{}
	p, err := New(Config{Interval: time.Hour}, col, up, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	p.RunOnce(context.Background())
	p.RunOnce(context.Background())

	if up.count() != 1 {
		t.Fatalf("uploads = %d, CollectedAt/CollectError must not change fingerprint", up.count())
	}
}
