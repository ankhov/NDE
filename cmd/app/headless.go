//go:build !gui

// Headless entry point (default build). Runs the Application Core until a
// SIGINT/SIGTERM is received, then shuts down gracefully. Used by tests, CI
// and any non-GUI run (e.g. a service/daemon mode later).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

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

	// Storage: open + migrate before the core starts so every module sees a
	// ready schema. Migration failures are fatal at startup.
	db, err := sqlite.Open(sqlite.Options{Path: cfg.DBPath})
	if err != nil {
		return fmt.Errorf("open storage: %w", err)
	}
	defer func() {
		if cerr := db.Close(); cerr != nil {
			log.Error("close storage", "err", cerr)
		}
	}()

	if err := db.Migrate(context.Background()); err != nil {
		return fmt.Errorf("migrate storage: %w", err)
	}
	v, _ := db.MigrationVersion(context.Background())
	log.Info("storage ready", "db", cfg.DBPath, "migration_version", v)

	app := core.New(cfg, core.WithLogger(log))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Wire the monitoring pipeline (collectors + scheduler + seed) before
	// Start so the scheduler is part of ordered module lifecycle.
	sched, err := wireMonitoring(ctx, cfg, log, db, app)
	if err != nil {
		return fmt.Errorf("wire monitoring: %w", err)
	}

	log.Info("nde starting", "version", buildVersion)
	if err := app.Start(ctx); err != nil {
		return fmt.Errorf("start core: %w", err)
	}
	loadTargetsIntoScheduler(ctx, db, sched, log)
	app.Bus().Publish(ctx, events.Event{Name: events.MonitoringStarted})
	log.Info("nde running", "pid", os.Getpid())

	<-ctx.Done()
	log.Info("nde shutdown signal received")
	stop() // restore default signal behaviour so a second signal force-exits

	app.Bus().Publish(ctx, events.Event{Name: events.MonitoringStopped})

	shutCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := app.Shutdown(shutCtx); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("nde shutdown error", "err", err)
		return fmt.Errorf("shutdown: %w", err)
	}
	log.Info("nde stopped")
	return nil
}
