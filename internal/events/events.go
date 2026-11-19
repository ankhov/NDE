// Package events provides a typed in-process event bus (SPEC §8).
//
// The bus decouples producers (collectors, analytics, diagnostics) from
// consumers (UI, notifications). The Wails binding layer (Phase 5/6) subscribes
// to the bus and re-emits events to the frontend, so the domain never imports
// Wails.
//
// Handlers are invoked synchronously on the publisher's goroutine. They must
// be non-blocking; long work must be dispatched to a goroutine by the handler.
// This keeps event ordering deterministic and avoids unbounded goroutine
// creation.
package events

import (
	"context"
	"sync"
	"time"
)

// EventName is a typed event channel name (SPEC §8).
type EventName string

// Event names defined by SPEC §8.
const (
	MeasurementNew              EventName = "measurement:new"
	AnomalyDetected             EventName = "anomaly:detected"
	AnomalyResolved             EventName = "anomaly:resolved"
	DiagnosticCreated           EventName = "diagnostic:created"
	DiagnosticUpdated           EventName = "diagnostic:updated"
	TargetStatusChanged         EventName = "target:status_changed"
	NetworkConfigurationChanged EventName = "network:configuration_changed"
	MonitoringStarted           EventName = "monitoring:started"
	MonitoringStopped           EventName = "monitoring:stopped"
)

// Event is a typed message on the bus. Payload is an arbitrary typed struct;
// subscribers assert the concrete type they expect for a given EventName.
type Event struct {
	Name    EventName
	Payload any
	At      time.Time
}

// Handler processes one event.
type Handler func(Event)

// Bus is the pub/sub contract.
type Bus interface {
	// Subscribe registers h for events of the given name and returns an
	// unsubscribe function.
	Subscribe(name EventName, h Handler) (unsubscribe func())
	// Publish delivers e to all current subscribers of e.Name synchronously.
	Publish(ctx context.Context, e Event)
}

// NewBus returns a concurrency-safe in-memory Bus.
func NewBus() Bus {
	return &bus{subs: make(map[EventName][]Handler)}
}

type bus struct {
	mu   sync.RWMutex
	subs map[EventName][]Handler
}

func (b *bus) Subscribe(name EventName, h Handler) func() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subs[name] = append(b.subs[name], h)
	idx := len(b.subs[name]) - 1
	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		subs := b.subs[name]
		if idx >= 0 && idx < len(subs) {
			subs[idx] = nil // tombstone; filtered on publish
		}
	}
}

func (b *bus) Publish(_ context.Context, e Event) {
	if e.At.IsZero() {
		e.At = time.Now().UTC()
	}
	b.mu.RLock()
	subs := append([]Handler(nil), b.subs[e.Name]...)
	b.mu.RUnlock()
	for _, h := range subs {
		if h == nil {
			continue
		}
		h(e)
	}
}
