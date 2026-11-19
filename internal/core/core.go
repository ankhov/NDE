// Package core is the Application Core (SPEC §3.1).
//
// It owns application lifecycle: configuration, logging, the event bus, a
// module registry and ordered, bounded graceful shutdown. Domain modules
// (collectors, scheduler, metrics, analytics, diagnostics, reporting) are
// registered as core.Module implementations and started/stopped in order.
//
// The Core has no dependency on Wails or on any concrete collector; it can be
// started headless and exercised in tests.
package core

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/ankhov/NDE/internal/config"
	"github.com/ankhov/NDE/internal/events"
)

// Module is a pluggable application component with an explicit lifecycle.
// Start is called in registration order; Stop in reverse order.
type Module interface {
	Name() string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

// ErrAlreadyStarted is returned when Start is called on an already-running App.
var ErrAlreadyStarted = errors.New("core: app already started")

// App is the Application Core.
type App struct {
	cfg             config.Config
	log             *slog.Logger
	bus             events.Bus
	modules         []Module
	shutdownTimeout time.Duration

	mu      sync.Mutex
	started bool
}

// Option configures an App at construction.
type Option func(*App)

// WithLogger sets the logger (defaults to slog.Default()).
func WithLogger(l *slog.Logger) Option { return func(a *App) { a.log = l } }

// WithBus sets the event bus (defaults to events.NewBus()).
func WithBus(b events.Bus) Option { return func(a *App) { a.bus = b } }

// WithModule registers a module.
func WithModule(m Module) Option {
	return func(a *App) { a.modules = append(a.modules, m) }
}

// New constructs an App from cfg + options.
func New(cfg config.Config, opts ...Option) *App {
	a := &App{
		cfg:             cfg,
		shutdownTimeout: cfg.ShutdownTimeout,
	}
	if a.shutdownTimeout <= 0 {
		a.shutdownTimeout = 5 * time.Second
	}
	for _, o := range opts {
		o(a)
	}
	if a.log == nil {
		a.log = slog.Default()
	}
	if a.bus == nil {
		a.bus = events.NewBus()
	}
	return a
}

// Config returns the bootstrap configuration.
func (a *App) Config() config.Config { return a.cfg }

// Logger returns the application logger.
func (a *App) Logger() *slog.Logger { return a.log }

// Bus returns the event bus.
func (a *App) Bus() events.Bus { return a.bus }

// Modules returns the registered modules (registration order).
func (a *App) Modules() []Module {
	a.mu.Lock()
	defer a.mu.Unlock()
	cp := make([]Module, len(a.modules))
	copy(cp, a.modules)
	return cp
}

// Register adds a module after construction (useful for wiring in main).
// It panics if the app has already started.
func (a *App) Register(m Module) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.started {
		panic("core: cannot register modules after Start")
	}
	a.modules = append(a.modules, m)
}

// Start starts every registered module in registration order.
// A second Start returns ErrAlreadyStarted.
func (a *App) Start(ctx context.Context) error {
	a.mu.Lock()
	if a.started {
		a.mu.Unlock()
		return ErrAlreadyStarted
	}
	a.started = true
	modules := append([]Module(nil), a.modules...)
	a.mu.Unlock()

	for _, m := range modules {
		a.log.Info("module starting", "module", m.Name())
		if err := m.Start(ctx); err != nil {
			a.log.Error("module start failed", "module", m.Name(), "err", err)
			// Attempt to roll back already-started modules.
			_ = a.Shutdown(ctx)
			return err
		}
	}
	return nil
}

// Shutdown stops every module in reverse registration order, bounded by the
// configured shutdown timeout. Returns the first non-cancellation error.
func (a *App) Shutdown(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, a.shutdownTimeout)
	defer cancel()

	a.mu.Lock()
	started := a.started
	a.started = false
	modules := append([]Module(nil), a.modules...)
	a.mu.Unlock()

	if !started {
		return nil
	}

	var firstErr error
	for i := len(modules) - 1; i >= 0; i-- {
		m := modules[i]
		a.log.Info("module stopping", "module", m.Name())
		if err := m.Stop(ctx); err != nil && !errors.Is(err, context.Canceled) {
			a.log.Error("module stop failed", "module", m.Name(), "err", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}
