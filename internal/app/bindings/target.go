// Package bindings — TargetService exposes target CRUD to the UI.
//
// Every mutation flows through target.Validate + SSRFGuard (SPEC §9.3) before
// it reaches storage, and through the scheduler so the running monitored set
// updates live — no restart needed. The service never fabricates targets
// (SPEC rule 15/16).
package bindings

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	pkgerrors "github.com/ankhov/NDE/internal/errors"
	"github.com/ankhov/NDE/internal/measurement"
	"github.com/ankhov/NDE/internal/scheduler"
	"github.com/ankhov/NDE/internal/storage"
	"github.com/ankhov/NDE/internal/target"
)

// TargetScheduler is the subset of *scheduler.Scheduler the service needs.
// Defining it here keeps the service unit-testable with a fake.
type TargetScheduler interface {
	AddTarget(measurement.Target) error
	RemoveTarget(id string)
	UpdateTarget(measurement.Target) error
}

// TargetService handles target CRUD and live scheduling.
type TargetService struct {
	store storage.TargetStore
	sched TargetScheduler
	log   *slog.Logger
}

// NewTargetService returns a TargetService backed by store and sched.
// sched may be nil (headless tests); in that case mutations still persist but
// do not affect a running scheduler.
func NewTargetService(store storage.TargetStore, sched TargetScheduler, log *slog.Logger) *TargetService {
	if log == nil {
		log = slog.Default()
	}
	return &TargetService{store: store, sched: sched, log: log}
}

// Compile-time check that *scheduler.Scheduler satisfies TargetScheduler.
var _ TargetScheduler = (*scheduler.Scheduler)(nil)

// List returns every configured target.
func (s *TargetService) List(ctx context.Context) ([]TargetDTO, error) {
	ts, err := s.store.ListTargets(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: list targets: %v", pkgerrors.ErrStorage, err)
	}
	out := make([]TargetDTO, len(ts))
	for i, t := range ts {
		out[i] = toTargetDTO(t)
	}
	return out, nil
}

// Add validates and persists a new target, then schedules it. The ID is
// generated server-side (UUID v4).
func (s *TargetService) Add(ctx context.Context, in TargetInputDTO) (TargetDTO, error) {
	t := toTarget(uuid.NewString(), in)
	if err := s.validateAndGuard(t); err != nil {
		return TargetDTO{}, err
	}
	if err := s.store.UpsertTarget(ctx, t); err != nil {
		return TargetDTO{}, fmt.Errorf("%w: upsert target: %v", pkgerrors.ErrStorage, err)
	}
	if s.sched != nil {
		if err := s.sched.AddTarget(t); err != nil {
			s.log.Warn("schedule new target", "id", t.ID, "err", err)
		}
	}
	s.log.Info("target added", "id", t.ID, "host", t.Host, "protocol", t.Protocol)
	return toTargetDTO(t), nil
}

// Delete removes a target from the scheduler and storage.
func (s *TargetService) Delete(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("%w: id is required", pkgerrors.ErrInvalidInput)
	}
	if s.sched != nil {
		s.sched.RemoveTarget(id)
	}
	if err := s.store.DeleteTarget(ctx, id); err != nil {
		return fmt.Errorf("%w: delete target: %v", pkgerrors.ErrStorage, err)
	}
	s.log.Info("target deleted", "id", id)
	return nil
}

// SetEnabled toggles a target's enabled flag and reschedules it. Disabling
// stops collection; enabling starts it.
func (s *TargetService) SetEnabled(ctx context.Context, id string, enabled bool) (TargetDTO, error) {
	t, err := s.store.GetTarget(ctx, id)
	if err != nil {
		return TargetDTO{}, fmt.Errorf("%w: get target: %v", pkgerrors.ErrNotFound, err)
	}
	t.Enabled = enabled
	if err := s.store.UpsertTarget(ctx, t); err != nil {
		return TargetDTO{}, fmt.Errorf("%w: upsert target: %v", pkgerrors.ErrStorage, err)
	}
	if s.sched != nil {
		if err := s.sched.UpdateTarget(t); err != nil {
			s.log.Warn("reschedule target", "id", t.ID, "err", err)
		}
	}
	s.log.Info("target toggled", "id", id, "enabled", enabled)
	return toTargetDTO(t), nil
}

// validateAndGuard runs the domain validator and the SSRF literal screen.
func (s *TargetService) validateAndGuard(t measurement.Target) error {
	if err := target.Validate(t); err != nil {
		return fmt.Errorf("%w: %v", pkgerrors.ErrInvalidInput, err)
	}
	if err := target.SSRFGuard(t.Host, target.SSRFOptions{AllowPrivate: false}); err != nil {
		return fmt.Errorf("%w: %v", pkgerrors.ErrInvalidInput, err)
	}
	return nil
}
