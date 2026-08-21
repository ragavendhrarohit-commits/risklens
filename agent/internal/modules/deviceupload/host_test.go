package deviceupload

import (
	"context"
	"sync"
	"testing"
	"time"

	"risklens/agent/internal/modules/device"
	"risklens/agent/internal/pipeline"
	"risklens/agent/internal/transport/response"
)

type countingProbe struct {
	mu          sync.Mutex
	n           int
	info        device.Info
	hold        time.Duration
	inflight    int
	maxInflight int
}

func (c *countingProbe) Collect(ctx context.Context) (device.Info, error) {
	c.mu.Lock()
	c.n++
	c.inflight++
	if c.inflight > c.maxInflight {
		c.maxInflight = c.inflight
	}
	hold := c.hold
	info := c.info
	c.mu.Unlock()

	if hold > 0 {
		timer := time.NewTimer(hold)
		defer timer.Stop()
		select {
		case <-ctx.Done():
		case <-timer.C:
		}
	}

	c.mu.Lock()
	c.inflight--
	c.mu.Unlock()
	return info, nil
}

func (c *countingProbe) calls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}

func (c *countingProbe) max() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.maxInflight
}

func (c *countingProbe) setInfo(info device.Info) {
	c.mu.Lock()
	c.info = info
	c.mu.Unlock()
}

func testHostConfig() pipeline.Config {
	return pipeline.Config{
		Interval: 25 * time.Millisecond,
		Jitter:   0,
		Timeout:  time.Second,
	}
}

func TestNewPipelineHostRejectsNilDependencies(t *testing.T) {
	cfg := pipeline.Config{Interval: time.Hour}
	dev, err := device.NewWithCollector(device.Config{Enabled: true}, &countingProbe{info: sampleInfo()}, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	id := stubIdentity{id: sampleIdentity(), ok: true}
	client := NewClientWithTransport(respond(response.StatusCreated, successBody(true)), testLogger())

	if _, err := NewPipelineHost(cfg, nil, id, client, testLogger(), nil); err == nil {
		t.Fatal("expected error for nil refresher")
	}
	if _, err := NewPipelineHost(cfg, dev, nil, client, testLogger(), nil); err == nil {
		t.Fatal("expected error for nil identity")
	}
	if _, err := NewPipelineHost(cfg, dev, id, nil, testLogger(), nil); err == nil {
		t.Fatal("expected error for nil client")
	}
}

func TestPipelineHostBuildsWithInjectedCollaboratorsAndConfig(t *testing.T) {
	cfg := testHostConfig()
	dev, err := device.NewWithCollector(device.Config{Enabled: true}, &countingProbe{info: sampleInfo()}, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	client := NewClientWithTransport(respond(response.StatusCreated, successBody(true)), testLogger())
	host, err := NewPipelineHost(cfg, dev, stubIdentity{id: sampleIdentity(), ok: true}, client, testLogger(), nil)
	if err != nil {
		t.Fatal(err)
	}

	if host.Pipeline() == nil {
		t.Fatal("pipeline is nil")
	}
	if host.Pipeline().Kind() != "device" {
		t.Fatalf("kind = %q", host.Pipeline().Kind())
	}

	col, ok := host.Collector().(*device.PipelineCollector)
	if !ok || col == nil {
		t.Fatalf("collector type %T", host.Collector())
	}
	if host.Collector().Kind() != "device" {
		t.Fatalf("collector kind = %q", host.Collector().Kind())
	}

	up, ok := host.Uploader().(*Uploader)
	if !ok || up == nil {
		t.Fatalf("uploader type %T", host.Uploader())
	}

	got := host.Config()
	if got.Interval != cfg.Interval || got.Jitter != cfg.Jitter || got.Timeout != cfg.Timeout {
		t.Fatalf("config = %+v want %+v", got, cfg)
	}
}

func TestPipelineHostDoesNotExecute(t *testing.T) {
	probe := &countingProbe{info: sampleInfo()}
	dev, err := device.NewWithCollector(device.Config{Enabled: true}, probe, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	transport := respond(response.StatusCreated, successBody(true))
	client := NewClientWithTransport(transport, testLogger())
	cfg := testHostConfig()

	host, err := NewPipelineHost(cfg, dev, stubIdentity{id: sampleIdentity(), ok: true}, client, testLogger(), nil)
	if err != nil {
		t.Fatal(err)
	}

	if probe.calls() != 0 {
		t.Fatalf("collects = %d, construction must not Refresh", probe.calls())
	}
	if transport.callCount() != 0 {
		t.Fatalf("uploads = %d, construction must not POST", transport.callCount())
	}
	if host.Pipeline().Health() != pipeline.HealthDisabled {
		t.Fatalf("health = %s, Start must not have run", host.Pipeline().Health())
	}
	if !host.Pipeline().LastSuccess().Empty() {
		t.Fatal("last success must be empty")
	}
	if host.Pipeline().Pending() {
		t.Fatal("pending must be false")
	}

	time.Sleep(80 * time.Millisecond)

	if probe.calls() != 0 {
		t.Fatalf("collects after wait = %d, scheduler must not be running", probe.calls())
	}
	if transport.callCount() != 0 {
		t.Fatalf("uploads after wait = %d", transport.callCount())
	}
	if host.Pipeline().Health() != pipeline.HealthDisabled {
		t.Fatalf("health after wait = %s", host.Pipeline().Health())
	}
}

func TestPipelineHostRunOnceOneCollectOnePostWithoutScheduler(t *testing.T) {
	probe := &countingProbe{info: sampleInfo()}
	dev, err := device.NewWithCollector(device.Config{Enabled: true}, probe, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	transport := respond(response.StatusCreated, successBody(true))
	client := NewClientWithTransport(transport, testLogger())
	cfg := testHostConfig()

	host, err := NewPipelineHost(cfg, dev, stubIdentity{id: sampleIdentity(), ok: true}, client, testLogger(), nil)
	if err != nil {
		t.Fatal(err)
	}

	host.RunOnce(context.Background())

	if probe.calls() != 1 {
		t.Fatalf("collects = %d, want 1", probe.calls())
	}
	if transport.callCount() != 1 {
		t.Fatalf("posts = %d, want 1", transport.callCount())
	}
	if _, ok := host.Collector().(*device.PipelineCollector); !ok {
		t.Fatalf("collector = %T", host.Collector())
	}
	if _, ok := host.Uploader().(*Uploader); !ok {
		t.Fatalf("uploader = %T", host.Uploader())
	}

	time.Sleep(80 * time.Millisecond)
	if probe.calls() != 1 {
		t.Fatalf("scheduler started: collects = %d", probe.calls())
	}
	if transport.callCount() != 1 {
		t.Fatalf("scheduler started: posts = %d", transport.callCount())
	}
}

func TestPipelineHostStartImmediateAndStopsCleanly(t *testing.T) {
	info := sampleInfo()
	info.CollectedAt = time.Time{}
	probe := &countingProbe{info: info}
	dev, err := device.NewWithCollector(device.Config{Enabled: true}, probe, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	transport := respond(response.StatusCreated, successBody(true))
	client := NewClientWithTransport(transport, testLogger())
	host, err := NewPipelineHost(testHostConfig(), dev, stubIdentity{id: sampleIdentity(), ok: true}, client, testLogger(), nil)
	if err != nil {
		t.Fatal(err)
	}

	host.Start(context.Background())
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if transport.callCount() >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if transport.callCount() != 1 {
		t.Fatalf("posts = %d, want 1 immediate upload", transport.callCount())
	}

	deadline = time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if probe.calls() >= 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if probe.calls() < 2 {
		t.Fatalf("collects = %d, scheduler must tick", probe.calls())
	}
	if transport.callCount() != 1 {
		t.Fatalf("posts = %d, unchanged must skip", transport.callCount())
	}

	host.Stop()
	collects := probe.calls()
	posts := transport.callCount()
	time.Sleep(80 * time.Millisecond)
	if probe.calls() != collects || transport.callCount() != posts {
		t.Fatal("scheduler continued after Stop")
	}
}

func TestPipelineHostRejectsInvalidPipelineConfig(t *testing.T) {
	dev, err := device.NewWithCollector(device.Config{Enabled: true}, &countingProbe{info: sampleInfo()}, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	client := NewClientWithTransport(respond(response.StatusCreated, successBody(true)), testLogger())
	_, err = NewPipelineHost(
		pipeline.Config{Interval: 0},
		dev,
		stubIdentity{id: sampleIdentity(), ok: true},
		client,
		testLogger(),
		nil,
	)
	if err == nil {
		t.Fatal("expected invalid interval from pipeline.New")
	}
}
