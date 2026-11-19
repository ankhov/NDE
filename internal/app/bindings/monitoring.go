// Package bindings — MonitoringService maps the dashboard's Start/Stop button
// to scheduler pause/resume. The scheduler publishes MonitoringStarted /
// MonitoringStopped itself, so this service only flips the gate; the GUI event
// bridge forwards those events to the frontend (SPEC §8).
package bindings

import (
	"context"

	"github.com/ankhov/NDE/internal/scheduler"
)

// MonitoringScheduler is the subset of *scheduler.Scheduler the service needs.
type MonitoringScheduler interface {
	Pause()
	Resume()
	IsPaused() bool
}

// MonitoringService controls the global collection gate.
type MonitoringService struct {
	sched MonitoringScheduler
}

// NewMonitoringService returns a MonitoringService bound to sched.
func NewMonitoringService(sched MonitoringScheduler) *MonitoringService {
	return &MonitoringService{sched: sched}
}

// Compile-time check that *scheduler.Scheduler satisfies MonitoringScheduler.
var _ MonitoringScheduler = (*scheduler.Scheduler)(nil)

// Start resumes collection. It is idempotent: resuming an already-active
// scheduler is a no-op.
func (s *MonitoringService) Start(_ context.Context) error {
	s.sched.Resume()
	return nil
}

// Stop pauses collection without tearing down runners.
func (s *MonitoringService) Stop(_ context.Context) error {
	s.sched.Pause()
	return nil
}

// IsActive reports whether collection is running.
func (s *MonitoringService) IsActive(_ context.Context) bool {
	return !s.sched.IsPaused()
}
