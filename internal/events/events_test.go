package events

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestPubSubDeliversToSubscribers(t *testing.T) {
	bus := NewBus()
	var got int32
	unsub := bus.Subscribe(MeasurementNew, func(e Event) {
		if e.Name != MeasurementNew {
			t.Errorf("unexpected name %q", e.Name)
		}
		if _, ok := e.Payload.(int); !ok {
			t.Errorf("payload type mismatch: %T", e.Payload)
		}
		atomic.AddInt32(&got, 1)
	})

	bus.Subscribe(AnomalyDetected, func(Event) { t.Error("wrong channel delivered") })

	bus.Publish(context.Background(), Event{Name: MeasurementNew, Payload: 42})

	if atomic.LoadInt32(&got) != 1 {
		t.Fatalf("expected 1 delivery, got %d", got)
	}

	unsub()
	bus.Publish(context.Background(), Event{Name: MeasurementNew, Payload: 1})
	if atomic.LoadInt32(&got) != 1 {
		t.Fatalf("unsubscribe did not stop delivery, got %d", got)
	}
}

func TestPublishSetsTimestamp(t *testing.T) {
	bus := NewBus()
	var seen Event
	bus.Subscribe(AnomalyResolved, func(e Event) { seen = e })
	before := time.Now().UTC()
	bus.Publish(context.Background(), Event{Name: AnomalyResolved})
	if seen.At.Before(before) {
		t.Fatalf("timestamp not set: %v < %v", seen.At, before)
	}
	if seen.Name != AnomalyResolved {
		t.Fatalf("name mismatch: %q", seen.Name)
	}
}

func TestMultipleSubscribers(t *testing.T) {
	bus := NewBus()
	var n int32
	bus.Subscribe(DiagnosticCreated, func(Event) { atomic.AddInt32(&n, 1) })
	bus.Subscribe(DiagnosticCreated, func(Event) { atomic.AddInt32(&n, 1) })
	bus.Subscribe(DiagnosticUpdated, func(Event) { atomic.AddInt32(&n, 1) })

	bus.Publish(context.Background(), Event{Name: DiagnosticCreated})
	if atomic.LoadInt32(&n) != 2 {
		t.Fatalf("expected 2 deliveries, got %d", n)
	}
}

func TestEventNamesComplete(t *testing.T) {
	// SPEC §8 — every event name must exist as a constant.
	want := []EventName{
		MeasurementNew, AnomalyDetected, AnomalyResolved,
		DiagnosticCreated, DiagnosticUpdated, TargetStatusChanged,
		NetworkConfigurationChanged, MonitoringStarted, MonitoringStopped,
	}
	seen := map[EventName]bool{}
	for _, n := range want {
		seen[n] = true
	}
	if len(seen) != len(want) {
		t.Fatalf("duplicate event names in SPEC §8 set: %d vs %d", len(seen), len(want))
	}
}
