// Package scheduler drives periodic measurement collection (SPEC §3.3, §9.1).
//
// It owns one goroutine per enabled target, ticking at the target's
// CheckInterval. Concurrency across ticks is bounded by a semaphore sized to
// cfg.MaxParallelChecks so a hundred targets cannot starve the machine.
//
// The scheduler is a core.Module: Start prepares a root context, Stop cancels
// it and waits for every runner to exit within the shutdown deadline. Targets
// are added/removed at runtime (AddTarget/RemoveTarget/UpdateTarget) so the
// Targets page can hot-edit the monitored set without a restart.
//
// Pause/Resume gate collection without tearing down runners: the dashboard's
// "Start/Stop monitoring" button maps to these. MonitoringStarted /
// MonitoringStopped events are published here so the GUI bridge (and only the
// GUI bridge) needs to forward them to the frontend (SPEC §8).
package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ankhov/NDE/internal/collector"
	"github.com/ankhov/NDE/internal/events"
	"github.com/ankhov/NDE/internal/measurement"
	"github.com/ankhov/NDE/internal/storage"
)

// ErrNotStarted is returned when a target operation is attempted before Start.
var ErrNotStarted = errors.New("scheduler: not started")

// ErrInvalidTarget is returned when AddTarget receives a target with an empty ID.
var ErrInvalidTarget = errors.New("scheduler: target id is required")

// Scheduler runs periodic collections. It is safe for concurrent use.
type Scheduler struct {
	reg         collector.Registry
	repo        storage.MeasurementWriter
	bus         events.Bus
	log         *slog.Logger
	maxParallel int

	sem    chan struct{}
	paused atomic.Bool

	mu        sync.Mutex
	runners   map[string]*runner
	rootCtx   context.Context
	rootCancel context.CancelFunc
	wg        sync.WaitGroup
	started   bool
}

// runner holds the per-target cancellation handle and a done signal.
type runner struct {
	target measurement.Target
	cancel context.CancelFunc
	done   chan struct{}
}

// New constructs a Scheduler. maxParallel must be >= 1; callers usually pass
// cfg.MaxParallelChecks.
func New(reg collector.Registry, repo storage.MeasurementWriter, bus events.Bus, maxParallel int, log *slog.Logger) *Scheduler {
	if maxParallel < 1 {
		maxParallel = 1
	}
	if log == nil {
		log = slog.Default()
	}
	return &Scheduler{
		reg:         reg,
		repo:        repo,
		bus:         bus,
		log:         log,
		maxParallel: maxParallel,
		sem:         make(chan struct{}, maxParallel),
		runners:     make(map[string]*runner),
	}
}

// Name implements core.Module.
func (s *Scheduler) Name() string { return "scheduler" }

// Start prepares the root context. It does not spawn runners — targets are
// added separately via AddTarget (typically the app loads them from the
// TargetStore right after Start). Implements core.Module.
func (s *Scheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return nil
	}
	s.rootCtx, s.rootCancel = context.WithCancel(ctx)
	s.started = true
	s.log.Info("scheduler started", "max_parallel", s.maxParallel)
	return nil
}

// Stop cancels the root context and waits for runners to exit. It returns
// nil even if the wait times out — runners exit on their own once cancelled,
// and a shutdown timeout should not surface as a hard error.
func (s *Scheduler) Stop(ctx context.Context) error {
	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return nil
	}
	s.started = false
	cancel := s.rootCancel
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	waitDone := make(chan struct{})
	go func() { s.wg.Wait(); close(waitDone) }()
	select {
	case <-waitDone:
	case <-ctx.Done():
		s.log.Warn("scheduler stop timed out, runners will exit in flight")
	}

	s.mu.Lock()
	s.runners = make(map[string]*runner)
	s.rootCtx, s.rootCancel = nil, nil
	s.mu.Unlock()
	s.log.Info("scheduler stopped")
	return nil
}

// AddTarget starts a runner for t. If a runner for t.ID already exists it is
// stopped and replaced (so UpdateTarget is just AddTarget again). Disabled
// targets are not run but are accepted as a no-op so callers need not filter.
func (s *Scheduler) AddTarget(t measurement.Target) error {
	if t.ID == "" {
		return ErrInvalidTarget
	}
	s.mu.Lock()
	if !s.started || s.rootCtx == nil {
		s.mu.Unlock()
		return ErrNotStarted
	}
	if old, ok := s.runners[t.ID]; ok {
		s.mu.Unlock()
		s.stopRunner(old)
		s.mu.Lock()
	}
	if !t.Enabled {
		s.mu.Unlock()
		return nil
	}
	rctx, cancel := context.WithCancel(s.rootCtx)
	r := &runner{target: t, cancel: cancel, done: make(chan struct{})}
	s.runners[t.ID] = r
	s.mu.Unlock()

	s.wg.Add(1)
	go s.run(rctx, r)
	s.log.Info("target scheduled", "id", t.ID, "host", t.Host, "interval", t.CheckInterval)
	return nil
}

// RemoveTarget stops and removes the runner for id. Missing ids are a no-op.
func (s *Scheduler) RemoveTarget(id string) {
	s.mu.Lock()
	r, ok := s.runners[id]
	if ok {
		delete(s.runners, id)
	}
	s.mu.Unlock()
	if ok {
		s.stopRunner(r)
		s.log.Info("target removed", "id", id)
	}
}

// UpdateTarget replaces the runner for t with a fresh one bound to the new
// fields. It is AddTarget with a clear name.
func (s *Scheduler) UpdateTarget(t measurement.Target) error { return s.AddTarget(t) }

// HasTarget reports whether a runner exists for id.
func (s *Scheduler) HasTarget(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.runners[id]
	return ok
}

// Pause suspends collection without tearing down runners.
// MonitoringStopped is published so the GUI can reflect the state.
func (s *Scheduler) Pause() {
	if s.paused.CompareAndSwap(false, true) {
		s.bus.Publish(context.Background(), events.Event{Name: events.MonitoringStopped})
		s.log.Info("monitoring paused")
	}
}

// Resume restarts collection. MonitoringStarted is published.
func (s *Scheduler) Resume() {
	if s.paused.CompareAndSwap(true, false) {
		s.bus.Publish(context.Background(), events.Event{Name: events.MonitoringStarted})
		s.log.Info("monitoring resumed")
	}
}

// IsPaused reports whether collection is suspended.
func (s *Scheduler) IsPaused() bool { return s.paused.Load() }

// run is the per-target loop. The first tick fires immediately so the
// dashboard shows data right after a target is added.
func (s *Scheduler) run(ctx context.Context, r *runner) {
	defer s.wg.Done()
	defer close(r.done)
	timer := time.NewTimer(0) // fire immediately
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			if !s.paused.Load() {
				s.tick(ctx, r.target)
			}
			timer.Reset(r.target.CheckInterval)
		}
	}
}

// tick acquires the concurrency semaphore and runs one collection.
func (s *Scheduler) tick(ctx context.Context, t measurement.Target) {
	select {
	case s.sem <- struct{}{}:
	case <-ctx.Done():
		return
	}
	defer func() { <-s.sem }()

	coll, ok := s.reg.Get(t.Protocol)
	if !ok {
		// No collector for this type yet (e.g. icmp before Phase 3). Record the
		// tick honestly so the UI can distinguish "not collected" from "stale".
		s.persist(ctx, measurement.Measurement{
			TargetID:  t.ID,
			Timestamp: time.Now().UTC(),
			Type:      t.Protocol,
			ErrorCode: measurement.CodeUnsupported,
		})
		return
	}

	checkCtx, cancel := context.WithTimeout(ctx, t.Timeout)
	defer cancel()
	m, err := coll.Collect(checkCtx, t)
	if err != nil {
		// Collector itself failed (not a target failure). Do not fabricate a
		// measurement; log and let the next tick retry.
		s.log.Error("collector error", "target", t.ID, "collector", coll.Name(), "err", err)
		return
	}
	s.persist(ctx, m)
}

// persist writes one measurement and publishes MeasurementNew. A write
// failure is logged but does not stop the scheduler.
func (s *Scheduler) persist(ctx context.Context, m measurement.Measurement) {
	if err := s.repo.WriteMeasurements(ctx, []measurement.Measurement{m}); err != nil {
		s.log.Error("persist measurement", "target", m.TargetID, "err", err)
		return
	}
	s.bus.Publish(ctx, events.Event{Name: events.MeasurementNew, Payload: m})
}

// stopRunner cancels a runner and waits for its goroutine to exit (bounded).
func (s *Scheduler) stopRunner(r *runner) {
	r.cancel()
	select {
	case <-r.done:
	case <-time.After(2 * time.Second):
		s.log.Warn("runner did not stop in time", "id", r.target.ID)
	}
}
