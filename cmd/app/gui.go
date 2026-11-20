//go:build gui

// GUI entry point (build with -tags=gui). Runs the Wails v3 desktop
// application. Shares bootstrap with the headless entry point; the only
// difference is the event loop (Wails Run vs. signal wait) and that bound
// services + embedded assets are registered for the frontend.
//
// Build:  go build -tags=gui ./cmd/app
// (Requires the frontend to be built first: (cd frontend && npm run build).)
package main

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/ankhov/NDE/frontend"
	"github.com/ankhov/NDE/internal/config"
	"github.com/ankhov/NDE/internal/core"
	"github.com/ankhov/NDE/internal/events"
	"github.com/ankhov/NDE/internal/logging"
	"github.com/ankhov/NDE/internal/storage/sqlite"
)

func runApp() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	log, err := logging.New(logging.Config{
		Level:  cfg.LogLevel,
		Format: cfg.LogFormat,
		File:   cfg.LogFile,
	})
	if err != nil {
		return fmt.Errorf("init logging: %w", err)
	}
	slog.SetDefault(log)

	db, err := sqlite.Open(sqlite.Options{Path: cfg.DBPath})
	if err != nil {
		return fmt.Errorf("open storage: %w", err)
	}
	if err := db.Migrate(context.Background()); err != nil {
		return fmt.Errorf("migrate storage: %w", err)
	}
	v, _ := db.MigrationVersion(context.Background())
	log.Info("storage ready", "db", cfg.DBPath, "migration_version", v)

	app := core.New(cfg, core.WithLogger(log))

	// Wire the monitoring pipeline (collectors + scheduler + seed) before the
	// GUI is constructed so the services bind to a live scheduler and registry.
	sched, reg, err := wireMonitoring(context.Background(), cfg, log, db, app)
	if err != nil {
		_ = db.Close()
		return fmt.Errorf("wire monitoring: %w", err)
	}
	svcs := newServices(buildVersion, db, sched, reg, log)

	// Embedded Vite production assets (frontend/dist).
	distFS, err := fs.Sub(frontend.Dist, "dist")
	if err != nil {
		return fmt.Errorf("embed dist: %w", err)
	}

	gui := application.New(application.Options{
		Name:        "Network Diagnostics Engine",
		Description: "Continuous network monitoring and explainable diagnostics",
		Logger:      log,
		Services: []application.Service{
			application.NewService(svcs.app),
			application.NewService(svcs.target),
			application.NewService(svcs.measurement),
			application.NewService(svcs.monitoring),
			application.NewService(svcs.collector),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(distFS),
		},
	})

	// Bridge the internal event bus to Wails frontend events (SPEC §8).
	// Producers stay decoupled from Wails; only this layer re-emits.
	bridgeEvents := []events.EventName{
		events.MeasurementNew, events.AnomalyDetected, events.AnomalyResolved,
		events.DiagnosticCreated, events.DiagnosticUpdated,
		events.TargetStatusChanged, events.NetworkConfigurationChanged,
		events.MonitoringStarted, events.MonitoringStopped,
	}
	for _, name := range bridgeEvents {
		n := name
		app.Bus().Subscribe(n, func(e events.Event) {
			gui.Event.Emit(string(n), e.Payload)
		})
	}

	// Graceful shutdown: stop the core and close the DB when the GUI quits.
	gui.OnShutdown(func() {
		log.Info("gui shutdown")
		app.Bus().Publish(context.Background(), events.Event{Name: events.MonitoringStopped})
		shutCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		_ = app.Shutdown(shutCtx)
		_ = db.Close()
	})

	// Start the Application Core before the GUI event loop.
	if err := app.Start(context.Background()); err != nil {
		_ = db.Close()
		return fmt.Errorf("start core: %w", err)
	}
	loadTargetsIntoScheduler(context.Background(), db, sched, log)
	app.Bus().Publish(context.Background(), events.Event{Name: events.MonitoringStarted})

	gui.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:      "main",
		Title:     "Network Diagnostics Engine",
		Width:     1280,
		Height:    800,
		MinWidth:  720,
		MinHeight: 480,
	})

	log.Info("nde gui starting", "version", buildVersion)
	return gui.Run()
}
