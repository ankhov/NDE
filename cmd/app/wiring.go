// Package main — monitoring pipeline wiring shared by the headless and GUI
// entry points (SPEC §3.1, §3.3).
//
// Both entry points build the same domain pipeline — collector registry,
// scheduler, target seeding, target load — so behaviour is identical in
// either mode; only the event loop (signal wait vs. Wails Run) and the
// registered UI services differ. This file has no build tag and imports no
// Wails code, so it compiles into the headless binary too.
package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/ankhov/NDE/internal/app/bindings"
	"github.com/ankhov/NDE/internal/app/seed"
	"github.com/ankhov/NDE/internal/collector"
	"github.com/ankhov/NDE/internal/config"
	"github.com/ankhov/NDE/internal/core"
	"github.com/ankhov/NDE/internal/scheduler"
	"github.com/ankhov/NDE/internal/storage/sqlite"
)

// wireMonitoring builds the collector registry and scheduler, registers the
// scheduler as a core module, and seeds default targets on first run. It MUST
// be called before core.App.Start so the scheduler participates in ordered
// module startup/shutdown.
func wireMonitoring(ctx context.Context, cfg config.Config, log *slog.Logger, db *sqlite.DB, app *core.App) (*scheduler.Scheduler, collector.Registry, error) {
	reg := collector.NewDefaultRegistry(collector.DefaultOptions{}, log)
	sched := scheduler.New(reg, db, app.Bus(), cfg.MaxParallelChecks, log)
	app.Register(sched)

	if n, err := seed.SeedDefaults(ctx, db, log); err != nil {
		return nil, nil, fmt.Errorf("seed defaults: %w", err)
	} else if n > 0 {
		log.Info("default targets seeded", "count", n)
	}
	return sched, reg, nil
}

// loadTargetsIntoScheduler reads all targets from storage and schedules them.
// Call AFTER core.App.Start: the scheduler's Start must have run so AddTarget
// has a root context to derive from. Returns the number of targets scheduled.
func loadTargetsIntoScheduler(ctx context.Context, db *sqlite.DB, sched *scheduler.Scheduler, log *slog.Logger) int {
	ts, err := db.ListTargets(ctx)
	if err != nil {
		log.Error("load targets into scheduler", "err", err)
		return 0
	}
	scheduled := 0
	for _, t := range ts {
		if err := sched.AddTarget(t); err != nil {
			log.Warn("schedule target", "id", t.ID, "host", t.Host, "err", err)
			continue
		}
		scheduled++
	}
	log.Info("targets scheduled", "total", len(ts), "scheduled", scheduled)
	return scheduled
}

// services bundles the UI-facing bindings constructed for the GUI entry point.
type services struct {
	app         *bindings.AppService
	target      *bindings.TargetService
	measurement *bindings.MeasurementService
	monitoring  *bindings.MonitoringService
	collector   *bindings.CollectorService
}

// newServices constructs the UI-facing services bound to the live scheduler,
// registry and storage. Used only by the GUI entry point.
func newServices(version string, db *sqlite.DB, sched *scheduler.Scheduler, reg collector.Registry, log *slog.Logger) services {
	return services{
		app:         bindings.NewAppService(version),
		target:      bindings.NewTargetService(db, sched, log),
		measurement: bindings.NewMeasurementService(db, db, log),
		monitoring:  bindings.NewMonitoringService(sched),
		collector:   bindings.NewCollectorService(reg),
	}
}
