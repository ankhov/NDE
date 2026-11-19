package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ankhov/NDE/internal/collector"
	"github.com/ankhov/NDE/internal/events"
	"github.com/ankhov/NDE/internal/measurement"
)

// fakeRepo is an in-memory storage.MeasurementWriter for tests.
type fakeRepo struct {
	mu   sync.Mutex
	data []measurement.Measurement
}

func (r *fakeRepo) WriteMeasurements(_ context.Context, ms []measurement.Measurement) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data = append(r.data, ms...)
	return nil
}

func (r *fakeRepo) snapshot() []measurement.Measurement {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make([]measurement.Measurement, len(r.data))
	copy(cp, r.data)
	return cp
}

// fakeCollector always succeeds with a fixed latency and supports a single type.
type fakeCollector struct {
	typ     measurement.Type
	latency float64
	delay   time.Duration
}

func (c *fakeCollector) Name() string { return "fake-" + string(c.typ) }
func (c *fakeCollector) Supports(t measurement.Type) bool { return t == c.typ }
func (c *fakeCollector) Collect(ctx context.Context, t measurement.Target) (measurement.Measurement, error) {
	if c.delay > 0 {
		select {
		case <-time.After(c.delay):
		case <-ctx.Done():
			return measurement.Measurement{}, ctx.Err()
		}
	}
	lat := c.latency
	return measurement.Measurement{
		TargetID: t.ID, Timestamp: time.Now().UTC(), Type: c.typ,
		Latency: &lat, Success: true,
	}, nil
}

func newTestScheduler(t *testing.T, maxParallel int) (*Scheduler, *fakeRepo, events.Bus) {
	t.Helper()
	reg := collector.NewRegistry()
	reg.Register(&fakeCollector{typ: measurement.TypeTCP, latency: 5.0})
	reg.Register(&fakeCollector{typ: measurement.TypeDNS, latency: 8.0})
	repo := &fakeRepo{}
	bus := events.NewBus()
	s := New(reg, repo, bus, maxParallel, nil)
	require.NoError(t, s.Start(context.Background()))
	t.Cleanup(func() { _ = s.Stop(context.Background()) })
	return s, repo, bus
}

func TestScheduler_CollectsAndPublishes(t *testing.T) {
	s, repo, bus := newTestScheduler(t, 4)
	var got []measurement.Measurement
	var mu sync.Mutex
	bus.Subscribe(events.MeasurementNew, func(e events.Event) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, e.Payload.(measurement.Measurement))
	})

	require.NoError(t, s.AddTarget(measurement.Target{
		ID: "tcp-1", Host: "1.1.1.1", Protocol: measurement.TypeTCP,
		Port: 443, CheckInterval: 40 * time.Millisecond, Timeout: time.Second,
		Enabled: true, Group: measurement.GroupExternal,
	}))
	require.NoError(t, s.AddTarget(measurement.Target{
		ID: "dns-1", Host: "1.1.1.1", Protocol: measurement.TypeDNS,
		Port: 53, CheckInterval: 40 * time.Millisecond, Timeout: time.Second,
		Enabled: true, Group: measurement.GroupDNS,
	}))

	time.Sleep(180 * time.Millisecond)

	data := repo.snapshot()
	require.GreaterOrEqual(t, len(data), 4, "expected several measurements, got %d", len(data))
	mu.Lock()
	assert.GreaterOrEqual(t, len(got), 4, "expected several events")
	mu.Unlock()

	// Every measurement is one of the two target types.
	for _, m := range data {
		assert.Contains(t, []measurement.Type{measurement.TypeTCP, measurement.TypeDNS}, m.Type)
		assert.True(t, m.Success)
	}
}

func TestScheduler_PauseResume(t *testing.T) {
	s, repo, _ := newTestScheduler(t, 2)
	require.NoError(t, s.AddTarget(measurement.Target{
		ID: "tcp-1", Host: "1.1.1.1", Protocol: measurement.TypeTCP,
		Port: 443, CheckInterval: 30 * time.Millisecond, Timeout: time.Second,
		Enabled: true, Group: measurement.GroupExternal,
	}))
	time.Sleep(80 * time.Millisecond) // a few ticks
	before := len(repo.snapshot())

	s.Pause()
	assert.True(t, s.IsPaused())
	time.Sleep(120 * time.Millisecond)
	afterPause := len(repo.snapshot())
	assert.Equal(t, before, afterPause, "no measurements should be collected while paused")

	s.Resume()
	assert.False(t, s.IsPaused())
	time.Sleep(80 * time.Millisecond)
	afterResume := len(repo.snapshot())
	assert.Greater(t, afterResume, afterPause, "collection should resume")
}

func TestScheduler_UnsupportedTypeRecorded(t *testing.T) {
	s, repo, _ := newTestScheduler(t, 2)
	require.NoError(t, s.AddTarget(measurement.Target{
		ID: "icmp-1", Host: "1.1.1.1", Protocol: measurement.TypeICMP,
		Port: 0, CheckInterval: 30 * time.Millisecond, Timeout: time.Second,
		Enabled: true, Group: measurement.GroupExternal,
	}))
	time.Sleep(90 * time.Millisecond)
	data := repo.snapshot()
	require.NotEmpty(t, data)
	for _, m := range data {
		assert.Equal(t, measurement.CodeUnsupported, m.ErrorCode)
		assert.False(t, m.Success)
		assert.Equal(t, measurement.TypeICMP, m.Type)
	}
}

func TestScheduler_RemoveTarget(t *testing.T) {
	s, repo, _ := newTestScheduler(t, 2)
	require.NoError(t, s.AddTarget(measurement.Target{
		ID: "tcp-1", Host: "1.1.1.1", Protocol: measurement.TypeTCP,
		Port: 443, CheckInterval: 30 * time.Millisecond, Timeout: time.Second,
		Enabled: true, Group: measurement.GroupExternal,
	}))
	time.Sleep(70 * time.Millisecond)
	require.NotEmpty(t, repo.snapshot())

	s.RemoveTarget("tcp-1")
	assert.False(t, s.HasTarget("tcp-1"))
	n := len(repo.snapshot())
	time.Sleep(80 * time.Millisecond)
	assert.Equal(t, n, len(repo.snapshot()), "removed target must not keep producing")
}

func TestScheduler_UpdateTargetReplaces(t *testing.T) {
	s, _, _ := newTestScheduler(t, 2)
	require.NoError(t, s.AddTarget(measurement.Target{
		ID: "tcp-1", Host: "1.1.1.1", Protocol: measurement.TypeTCP,
		Port: 443, CheckInterval: 30 * time.Millisecond, Timeout: time.Second,
		Enabled: true, Group: measurement.GroupExternal,
	}))
	require.True(t, s.HasTarget("tcp-1"))
	// Update with a new interval — should replace, not duplicate.
	require.NoError(t, s.UpdateTarget(measurement.Target{
		ID: "tcp-1", Host: "1.1.1.1", Protocol: measurement.TypeTCP,
		Port: 443, CheckInterval: 60 * time.Millisecond, Timeout: time.Second,
		Enabled: true, Group: measurement.GroupExternal,
	}))
	require.True(t, s.HasTarget("tcp-1"))
}

func TestScheduler_DisabledTargetNotRun(t *testing.T) {
	s, repo, _ := newTestScheduler(t, 2)
	require.NoError(t, s.AddTarget(measurement.Target{
		ID: "tcp-1", Host: "1.1.1.1", Protocol: measurement.TypeTCP,
		Port: 443, CheckInterval: 20 * time.Millisecond, Timeout: time.Second,
		Enabled: false, Group: measurement.GroupExternal,
	}))
	time.Sleep(70 * time.Millisecond)
	assert.Empty(t, repo.snapshot(), "disabled target must not collect")
	assert.False(t, s.HasTarget("tcp-1"))
}

func TestScheduler_AddBeforeStartErrors(t *testing.T) {
	reg := collector.NewRegistry()
	s := New(reg, &fakeRepo{}, events.NewBus(), 2, nil)
	err := s.AddTarget(measurement.Target{
		ID: "x", Protocol: measurement.TypeTCP, CheckInterval: time.Second, Timeout: time.Second,
		Enabled: true, Group: measurement.GroupExternal,
	})
	assert.ErrorIs(t, err, ErrNotStarted)
}
