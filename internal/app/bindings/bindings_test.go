package bindings

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ankhov/NDE/internal/measurement"
)

// fakeStore implements storage.TargetStore + storage.MeasurementReader.
type fakeStore struct {
	mu  sync.Mutex
	tg  map[string]measurement.Target
	ms  []measurement.Measurement
}

func newFakeStore() *fakeStore { return &fakeStore{tg: map[string]measurement.Target{}} }

func (f *fakeStore) UpsertTarget(_ context.Context, t measurement.Target) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tg[t.ID] = t
	return nil
}
func (f *fakeStore) DeleteTarget(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.tg[id]; !ok {
		return errNotFound{}
	}
	delete(f.tg, id)
	return nil
}
func (f *fakeStore) ListTargets(_ context.Context) ([]measurement.Target, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]measurement.Target, 0, len(f.tg))
	for _, t := range f.tg {
		out = append(out, t)
	}
	return out, nil
}
func (f *fakeStore) GetTarget(_ context.Context, id string) (measurement.Target, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.tg[id]
	if !ok {
		return measurement.Target{}, errNotFound{}
	}
	return t, nil
}
func (f *fakeStore) QueryMeasurements(_ context.Context, flt measurement.QueryFilter) ([]measurement.Measurement, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]measurement.Measurement, 0)
	for _, m := range f.ms {
		if len(flt.TargetIDs) > 0 && !containsStr(flt.TargetIDs, m.TargetID) {
			continue
		}
		if len(flt.Types) > 0 && !containsType(flt.Types, m.Type) {
			continue
		}
		if !flt.From.IsZero() && m.Timestamp.Before(flt.From) {
			continue
		}
		if !flt.To.IsZero() && m.Timestamp.After(flt.To) {
			continue
		}
		out = append(out, m)
	}
	// newest first
	sortDesc(out)
	limit := flt.Limit
	if limit <= 0 {
		limit = 1000
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

type errNotFound struct{}

func (errNotFound) Error() string { return "not found" }

// fakeScheduler implements TargetScheduler + MonitoringScheduler.
type fakeScheduler struct {
	mu      sync.Mutex
	added   []string
	removed []string
	paused  bool
}

func (f *fakeScheduler) AddTarget(t measurement.Target) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.added = append(f.added, t.ID)
	return nil
}
func (f *fakeScheduler) RemoveTarget(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removed = append(f.removed, id)
}
func (f *fakeScheduler) UpdateTarget(t measurement.Target) error { return f.AddTarget(t) }
func (f *fakeScheduler) Pause()                                  { f.mu.Lock(); f.paused = true; f.mu.Unlock() }
func (f *fakeScheduler) Resume()                                 { f.mu.Lock(); f.paused = false; f.mu.Unlock() }
func (f *fakeScheduler) IsPaused() bool                          { f.mu.Lock(); defer f.mu.Unlock(); return f.paused }

func TestTargetService_AddListDelete(t *testing.T) {
	store := newFakeStore()
	sched := &fakeScheduler{}
	svc := NewTargetService(store, sched, nil)
	ctx := context.Background()

	dto, err := svc.Add(ctx, TargetInputDTO{
		Name: "Test", Host: "1.1.1.1", Protocol: "tcp", Port: 443,
		CheckIntervalMS: 30000, TimeoutMS: 3000, Enabled: true,
		WarningThreshold: 150, CriticalThreshold: 400, Group: "external",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, dto.ID)
	require.Contains(t, sched.added, dto.ID)

	list, err := svc.List(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 1)

	require.NoError(t, svc.Delete(ctx, dto.ID))
	require.Contains(t, sched.removed, dto.ID)
	list, _ = svc.List(ctx)
	assert.Empty(t, list)
}

func TestTargetService_AddRejectsPrivateHost(t *testing.T) {
	store := newFakeStore()
	svc := NewTargetService(store, &fakeScheduler{}, nil)
	_, err := svc.Add(context.Background(), TargetInputDTO{
		Name: "Bad", Host: "127.0.0.1", Protocol: "tcp", Port: 80,
		CheckIntervalMS: 30000, TimeoutMS: 3000, Enabled: true, Group: "external",
	})
	assert.Error(t, err)
}

func TestTargetService_SetEnabled(t *testing.T) {
	store := newFakeStore()
	// Seed a target directly.
	require.NoError(t, store.UpsertTarget(context.Background(), measurement.Target{
		ID: "t-1", Name: "x", Host: "1.1.1.1", Protocol: measurement.TypeTCP,
		Port: 443, CheckInterval: time.Second, Timeout: time.Second,
		Enabled: true, Group: measurement.GroupExternal,
	}))
	sched := &fakeScheduler{}
	svc := NewTargetService(store, sched, nil)

	dto, err := svc.SetEnabled(context.Background(), "t-1", false)
	require.NoError(t, err)
	assert.False(t, dto.Enabled)
	// UpdateTarget (== AddTarget) must have been called to reschedule.
	require.Contains(t, sched.added, "t-1")
}

func TestMeasurementService_StatsAndTimeline(t *testing.T) {
	store := newFakeStore()
	svc := NewMeasurementService(store, store, nil)

	require.NoError(t, store.UpsertTarget(context.Background(), measurement.Target{
		ID: "t-1", Name: "x", Host: "1.1.1.1", Protocol: measurement.TypeTCP,
		Port: 443, CheckInterval: time.Second, Timeout: time.Second,
		Enabled: true, Group: measurement.GroupExternal,
	}))

	now := time.Now().UTC()
	lats := []float64{10, 20, 30, 40, 50}
	for i, lat := range lats {
		l := lat
		store.ms = append(store.ms, measurement.Measurement{
			TargetID: "t-1", Timestamp: now.Add(-time.Duration(i) * time.Second),
			Type: measurement.TypeTCP, Latency: &l, Success: true,
		})
	}
	// one failure
	store.ms = append(store.ms, measurement.Measurement{
		TargetID: "t-1", Timestamp: now, Type: measurement.TypeTCP,
		ErrorCode: measurement.CodeTimeout,
	})

	stats, err := svc.Stats(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 6, stats.SampleCount)
	require.NotNil(t, stats.AvgRTT)
	assert.InDelta(t, 30.0, *stats.AvgRTT, 0.001) // mean of 10..50 = 30
	require.NotNil(t, stats.P95RTT)
	assert.InDelta(t, 50.0, *stats.P95RTT, 0.001) // nearest-rank p95 of {10,20,30,40,50}
	require.NotNil(t, stats.Jitter)
	assert.InDelta(t, 10.0, *stats.Jitter, 0.001) // mean abs delta = 10
	assert.InDelta(t, 100.0/6.0, stats.PacketLoss, 0.01)
	assert.Equal(t, 1, stats.ActiveTargets)

	// Timeline returns oldest-first (RFC3339 sorts lexically == chronologically).
	tl, err := svc.Timeline(context.Background(), "t-1", 100)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(tl), 5)
	assert.True(t, tl[0].Timestamp <= tl[len(tl)-1].Timestamp)
}

func TestMonitoringService_Toggle(t *testing.T) {
	sched := &fakeScheduler{}
	svc := NewMonitoringService(sched)
	assert.True(t, svc.IsActive(context.Background()))
	require.NoError(t, svc.Stop(context.Background()))
	assert.False(t, svc.IsActive(context.Background()))
	require.NoError(t, svc.Start(context.Background()))
	assert.True(t, svc.IsActive(context.Background()))
}

// helpers
func containsStr(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
func containsType(xs []measurement.Type, t measurement.Type) bool {
	for _, x := range xs {
		if x == t {
			return true
		}
	}
	return false
}
func sortDesc(ms []measurement.Measurement) {
	for i := 0; i < len(ms); i++ {
		for j := i + 1; j < len(ms); j++ {
			if ms[j].Timestamp.After(ms[i].Timestamp) {
				ms[i], ms[j] = ms[j], ms[i]
			}
		}
	}
}
