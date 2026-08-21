package device

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"risklens/agent/internal/modules/interfaces"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type stubCollector struct {
	info Info
	err  error
	mu   sync.Mutex
	n    int
}

func (s *stubCollector) Collect(ctx context.Context) (Info, error) {
	s.mu.Lock()
	s.n++
	s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Info{}, err
	}
	return s.info, s.err
}

func (s *stubCollector) calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.n
}

func fullInfo() Info {
	return Info{
		MachineName:     "DESKTOP-01",
		Hostname:        "DESKTOP-01",
		OperatingSystem: "Windows 11 Pro",
		OSVersion:       "10.0 (23H2)",
		WindowsBuild:    "22631",
		CPUModel:        "AMD Ryzen 7 5800X",
		CPUCores:        8,
		MemoryBytes:     16 * 1024 * 1024 * 1024,
		DiskSizeBytes:   512 * 1024 * 1024 * 1024,
		DiskFreeBytes:   200 * 1024 * 1024 * 1024,
		Architecture:    "x86_64",
		SerialNumber:    "SN-123",
		Manufacturer:    "ExampleCorp",
		Model:           "ExampleBook",
		BootTime:        time.Date(2026, 8, 14, 1, 0, 0, 0, time.UTC),
		CurrentUser:     `EXAMPLE\alice`,
		AgentVersion:    "0.1.0",
		CollectedAt:     time.Date(2026, 8, 14, 2, 0, 0, 0, time.UTC),
	}
}

func TestNewRejectsNilCollector(t *testing.T) {
	if _, err := NewWithCollector(Config{Enabled: true}, nil, discardLogger()); err == nil {
		t.Fatal("expected error")
	}
}

func TestNewDefaultNameAndVersion(t *testing.T) {
	m, err := NewWithCollector(Config{Enabled: true}, &stubCollector{info: fullInfo()}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	if m.Name() != ModuleName || m.Version() != ModuleVersion {
		t.Fatalf("identity = %s %s", m.Name(), m.Version())
	}
	var _ interfaces.Module = m
}

func TestStartDisabledLeavesNotCollected(t *testing.T) {
	collector := &stubCollector{info: fullInfo()}
	m, err := NewWithCollector(Config{Enabled: false}, collector, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if collector.calls() != 0 {
		t.Fatalf("collector called %d times", collector.calls())
	}
	if m.Inventory().State != StateNotCollected {
		t.Fatalf("state = %q", m.Inventory().State)
	}
	if m.Status() != interfaces.StatusRunning {
		t.Fatalf("status = %q", m.Status())
	}
}

func TestStartSuccessfulCollection(t *testing.T) {
	info := fullInfo()
	m, err := NewWithCollector(Config{Enabled: true}, &stubCollector{info: info}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	snap := m.Inventory()
	if snap.State != StateCollected {
		t.Fatalf("state = %q", snap.State)
	}
	if snap.Info != info {
		t.Fatalf("info mismatch: %+v", snap.Info)
	}
	if m.Info().MachineName != "DESKTOP-01" {
		t.Fatalf("Info() = %+v", m.Info())
	}
	if m.Status() != interfaces.StatusRunning {
		t.Fatalf("status = %q", m.Status())
	}
	if m.Health() != interfaces.HealthHealthy {
		t.Fatalf("health = %q", m.Health())
	}
}

func TestStartPartialCollectionMarksDegraded(t *testing.T) {
	partial := Info{
		MachineName:     "HOST",
		Hostname:        "HOST",
		OperatingSystem: "Windows 11 Pro",
		Architecture:    "x86_64",
		AgentVersion:    "0.1.0",
		CollectedAt:     time.Now().UTC(),
	}
	m, err := NewWithCollector(
		Config{Enabled: true},
		&stubCollector{info: partial, err: errors.New("cpu registry missing")},
		discardLogger(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	snap := m.Inventory()
	if snap.State != StatePartial {
		t.Fatalf("state = %q", snap.State)
	}
	if snap.LastError == "" {
		t.Fatal("expected LastError")
	}
	if m.Status() != interfaces.StatusRunning {
		t.Fatalf("status = %q", m.Status())
	}
	if m.Health() != interfaces.HealthDegraded {
		t.Fatalf("health = %q", m.Health())
	}
}

func TestStartEmptyCollectionMarksFailed(t *testing.T) {
	m, err := NewWithCollector(
		Config{Enabled: true},
		&stubCollector{err: errors.New("everything failed")},
		discardLogger(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if m.Inventory().State != StateFailed {
		t.Fatalf("state = %q", m.Inventory().State)
	}
	if m.Status() != interfaces.StatusFailed {
		t.Fatalf("status = %q", m.Status())
	}
}

func TestStartNeverReturnsErrorToModuleManager(t *testing.T) {
	cases := []*stubCollector{
		{info: fullInfo()},
		{info: Info{MachineName: "X"}, err: errors.New("partial")},
		{err: errors.New("total failure")},
	}
	for i, collector := range cases {
		m, err := NewWithCollector(Config{Enabled: true}, collector, discardLogger())
		if err != nil {
			t.Fatal(err)
		}
		if err := m.Start(context.Background()); err != nil {
			t.Fatalf("case %d: Start returned %v", i, err)
		}
	}
}

func TestStartHonoursCancelledContext(t *testing.T) {
	collector := &stubCollector{info: fullInfo()}
	m, err := NewWithCollector(Config{Enabled: true}, collector, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	// Collector itself sees the cancelled context and returns an error with empty info.
	if m.Inventory().State != StateFailed {
		t.Fatalf("state = %q", m.Inventory().State)
	}
}

func TestStopIsNoOp(t *testing.T) {
	m, err := NewWithCollector(Config{Enabled: false}, &stubCollector{}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := m.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if m.Status() != interfaces.StatusStopped {
		t.Fatalf("status = %q", m.Status())
	}
}

func TestInventoryIsSafeForConcurrentReaders(t *testing.T) {
	m, err := NewWithCollector(Config{Enabled: true}, &stubCollector{info: fullInfo()}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = m.Inventory()
			_ = m.Info()
			_ = m.Status()
			_ = m.Health()
		}()
	}
	wg.Wait()
}

func TestInfoRemainsAvailableAfterStop(t *testing.T) {
	m, err := NewWithCollector(Config{Enabled: true}, &stubCollector{info: fullInfo()}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	_ = m.Stop(context.Background())
	if m.Info().MachineName != "DESKTOP-01" {
		t.Fatalf("inventory lost after Stop: %+v", m.Info())
	}
}

type sequenceCollector struct {
	mu    sync.Mutex
	steps []collectStep
	n     int
}

type collectStep struct {
	info Info
	err  error
}

func (s *sequenceCollector) Collect(ctx context.Context) (Info, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.n++
	if err := ctx.Err(); err != nil {
		return Info{}, err
	}
	i := s.n - 1
	if i >= len(s.steps) {
		return Info{}, errors.New("sequence exhausted")
	}
	return s.steps[i].info, s.steps[i].err
}

func (s *sequenceCollector) calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.n
}

func TestStartSkipStartCollectDoesNotCollect(t *testing.T) {
	collector := &stubCollector{info: fullInfo()}
	m, err := NewWithCollector(Config{Enabled: true, SkipStartCollect: true}, collector, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if collector.calls() != 0 {
		t.Fatalf("Start must not Refresh when SkipStartCollect, calls=%d", collector.calls())
	}
	if m.Inventory().State != StateNotCollected {
		t.Fatalf("state = %q", m.Inventory().State)
	}
	if m.Status() != interfaces.StatusRunning {
		t.Fatalf("status = %q", m.Status())
	}
	if m.CollectionHealth() != interfaces.HealthUnknown {
		t.Fatalf("collection health = %q", m.CollectionHealth())
	}
}

func TestStartStillCollectsOnce(t *testing.T) {
	collector := &stubCollector{info: fullInfo()}
	m, err := NewWithCollector(Config{Enabled: true}, collector, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if collector.calls() != 1 {
		t.Fatalf("Start must collect once, calls=%d", collector.calls())
	}
}

func TestRefreshMatchesStartSnapshot(t *testing.T) {
	info := fullInfo()
	viaStart, err := NewWithCollector(Config{Enabled: true}, &stubCollector{info: info}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	if err := viaStart.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	viaRefresh, err := NewWithCollector(Config{Enabled: true}, &stubCollector{info: info}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	got := viaRefresh.Refresh(context.Background())

	want := viaStart.Inventory()
	if got.State != want.State || got.Info != want.Info || got.LastError != want.LastError {
		t.Fatalf("Refresh snapshot %+v != Start snapshot %+v", got, want)
	}
	if got.CollectedAt != want.CollectedAt {
		t.Fatalf("CollectedAt Refresh=%v Start=%v", got.CollectedAt, want.CollectedAt)
	}
}

func TestCollectorCollectCanBeCalledRepeatedly(t *testing.T) {
	collector := &stubCollector{info: fullInfo()}
	ctx := context.Background()

	first, err := collector.Collect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	second, err := collector.Collect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if collector.calls() != 2 {
		t.Fatalf("calls=%d", collector.calls())
	}
	if first != second || first != fullInfo() {
		t.Fatalf("Collect results drifted: %+v vs %+v", first, second)
	}
}

func TestRefreshMultipleConsecutiveCallsSucceed(t *testing.T) {
	first := fullInfo()
	second := fullInfo()
	second.MachineName = "DESKTOP-02"
	second.Hostname = "DESKTOP-02"
	second.DiskFreeBytes = 100
	second.CollectedAt = time.Date(2026, 8, 14, 3, 0, 0, 0, time.UTC)

	collector := &sequenceCollector{steps: []collectStep{
		{info: first},
		{info: second},
		{info: second},
	}}
	m, err := NewWithCollector(Config{Enabled: true}, collector, discardLogger())
	if err != nil {
		t.Fatal(err)
	}

	a := m.Refresh(context.Background())
	b := m.Refresh(context.Background())
	c := m.Refresh(context.Background())
	if collector.calls() != 3 {
		t.Fatalf("calls=%d", collector.calls())
	}
	if a.State != StateCollected || b.State != StateCollected || c.State != StateCollected {
		t.Fatalf("states = %s %s %s", a.State, b.State, c.State)
	}
	if a.Info.MachineName != "DESKTOP-01" || b.Info.MachineName != "DESKTOP-02" || c.Info.MachineName != "DESKTOP-02" {
		t.Fatalf("names = %q %q %q", a.Info.MachineName, b.Info.MachineName, c.Info.MachineName)
	}
}

func TestRefreshReturnsFreshSnapshotEachCall(t *testing.T) {
	first := fullInfo()
	second := fullInfo()
	second.Hostname = "fresh-host"
	second.DiskFreeBytes = 1
	second.CollectedAt = time.Date(2026, 8, 14, 4, 0, 0, 0, time.UTC)

	collector := &sequenceCollector{steps: []collectStep{{info: first}, {info: second}}}
	m, err := NewWithCollector(Config{Enabled: true}, collector, discardLogger())
	if err != nil {
		t.Fatal(err)
	}

	a := m.Refresh(context.Background())
	b := m.Refresh(context.Background())
	if a.Info == b.Info {
		t.Fatal("expected a fresh Info on the second Refresh")
	}
	if m.Inventory().Info != second {
		t.Fatalf("module still holds first snapshot: %+v", m.Inventory().Info)
	}
	if a.CollectedAt == b.CollectedAt {
		t.Fatal("CollectedAt must come from the current collect")
	}
}

func TestRefreshDoesNotMutatePreviousSnapshots(t *testing.T) {
	first := fullInfo()
	second := fullInfo()
	second.MachineName = "DESKTOP-02"
	second.Hostname = "DESKTOP-02"

	collector := &sequenceCollector{steps: []collectStep{{info: first}, {info: second}}}
	m, err := NewWithCollector(Config{Enabled: true}, collector, discardLogger())
	if err != nil {
		t.Fatal(err)
	}

	held := m.Refresh(context.Background())
	_ = m.Refresh(context.Background())

	if held.Info.MachineName != "DESKTOP-01" {
		t.Fatalf("held snapshot mutated to %q", held.Info.MachineName)
	}

	held.Info.MachineName = "MUTATED"
	held.LastError = "mutated"
	if m.Inventory().Info.MachineName != "DESKTOP-02" {
		t.Fatalf("mutating a copy changed module state: %+v", m.Inventory().Info)
	}
	if m.Inventory().LastError != "" {
		t.Fatalf("mutating a copy changed LastError: %q", m.Inventory().LastError)
	}
}

func TestRefreshDoesNotRetainStaleFields(t *testing.T) {
	full := fullInfo()
	sparse := Info{
		Hostname:     "only-host",
		AgentVersion: "0.1.0",
		CollectedAt:  time.Date(2026, 8, 14, 5, 0, 0, 0, time.UTC),
	}

	collector := &sequenceCollector{steps: []collectStep{
		{info: full},
		{info: sparse, err: errors.New("most probes failed")},
	}}
	m, err := NewWithCollector(Config{Enabled: true}, collector, discardLogger())
	if err != nil {
		t.Fatal(err)
	}

	_ = m.Refresh(context.Background())
	got := m.Refresh(context.Background())
	if got.Info.MachineName != "" {
		t.Fatalf("stale MachineName retained: %q", got.Info.MachineName)
	}
	if got.Info.MemoryBytes != 0 || got.Info.CPUModel != "" {
		t.Fatalf("stale hardware retained: %+v", got.Info)
	}
	if got.Info.Hostname != "only-host" {
		t.Fatalf("hostname = %q", got.Info.Hostname)
	}
	if got.LastError == "" {
		t.Fatal("expected LastError from the second collect")
	}
}

func TestRefreshErrorDoesNotCorruptLaterCollection(t *testing.T) {
	ok := fullInfo()
	collector := &sequenceCollector{steps: []collectStep{
		{err: errors.New("total failure")},
		{info: ok},
		{err: errors.New("failed again")},
		{info: ok},
	}}
	m, err := NewWithCollector(Config{Enabled: true}, collector, discardLogger())
	if err != nil {
		t.Fatal(err)
	}

	failed := m.Refresh(context.Background())
	if failed.State != StateFailed || failed.LastError == "" {
		t.Fatalf("first refresh = %+v", failed)
	}

	recovered := m.Refresh(context.Background())
	if recovered.State != StateCollected || recovered.Info != ok || recovered.LastError != "" {
		t.Fatalf("error leaked into next snapshot: %+v", recovered)
	}

	failedAgain := m.Refresh(context.Background())
	if failedAgain.State != StateFailed {
		t.Fatalf("state = %q", failedAgain.State)
	}

	recoveredAgain := m.Refresh(context.Background())
	if recoveredAgain.State != StateCollected || recoveredAgain.Info != ok {
		t.Fatalf("second recovery failed: %+v", recoveredAgain)
	}
}

func TestRefreshReplacesSingleSnapshot(t *testing.T) {
	collector := &stubCollector{info: fullInfo()}
	m, err := NewWithCollector(Config{Enabled: true}, collector, discardLogger())
	if err != nil {
		t.Fatal(err)
	}

	const rounds = 256
	var last Snapshot
	for i := 0; i < rounds; i++ {
		next := fullInfo()
		next.CPUCores = i + 1
		next.CollectedAt = time.Date(2026, 8, 14, 6, 0, i, 0, time.UTC)
		collector.mu.Lock()
		collector.info = next
		collector.mu.Unlock()
		last = m.Refresh(context.Background())
	}
	if collector.calls() != rounds {
		t.Fatalf("calls=%d", collector.calls())
	}
	if last.Info.CPUCores != rounds {
		t.Fatalf("last cores = %d", last.Info.CPUCores)
	}
	if m.Inventory().Info.CPUCores != rounds {
		t.Fatalf("module stored more than the last snapshot: %+v", m.Inventory().Info)
	}
}

func TestRefreshSetsCollectionHealthWithoutChangingStatus(t *testing.T) {
	m, err := NewWithCollector(Config{Enabled: true, SkipStartCollect: true}, &stubCollector{info: fullInfo()}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if m.Status() != interfaces.StatusRunning {
		t.Fatalf("status = %q", m.Status())
	}
	_ = m.Refresh(context.Background())
	if m.CollectionHealth() != interfaces.HealthHealthy {
		t.Fatalf("collection health = %q", m.CollectionHealth())
	}
	if m.Health() != interfaces.HealthHealthy {
		t.Fatalf("module health = %q", m.Health())
	}
	if m.Status() != interfaces.StatusRunning {
		t.Fatal("Refresh must not change Status")
	}
}

func TestRefreshFailureDoesNotMarkCollectorFailedStatus(t *testing.T) {
	m, err := NewWithCollector(
		Config{Enabled: true, SkipStartCollect: true},
		&stubCollector{err: errors.New("probe failed")},
		discardLogger(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	_ = m.Refresh(context.Background())
	if m.CollectionHealth() != interfaces.HealthUnhealthy {
		t.Fatalf("collection health = %q", m.CollectionHealth())
	}
	if m.Status() != interfaces.StatusRunning {
		t.Fatalf("HTTP-unrelated collect fail must keep Status Running, got %q", m.Status())
	}
}
