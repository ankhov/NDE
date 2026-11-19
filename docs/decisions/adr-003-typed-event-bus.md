# ADR-003: Typed in-process event bus

**Status:** Accepted
**Date:** 2026-11-19
**Context:** SPEC §8 defines nine backend↔frontend events. Collectors,
analytics and diagnostics must produce events without coupling to the UI or to
Wails.

**Decision:** Provide a typed event bus in `internal/events`:
- `EventName` constants matching SPEC §8 exactly (`measurement:new`, …).
- `Event{Name, Payload any, At time.Time}`; subscribers assert the concrete
  payload type they expect for a given name.
- `Bus` interface with `Subscribe(name, handler) -> unsubscribe` and
  `Publish(ctx, event)`.
- Delivery is **synchronous** on the publisher's goroutine; handlers must be
  non-blocking (dispatch long work themselves). This keeps ordering
  deterministic and avoids unbounded goroutine creation.

The Wails GUI layer (`cmd/app/gui.go`) subscribes to the bus and re-emits via
`gui.Event.Emit`. In tests, in-memory subscribers assert behaviour.

**Consequences:** Producers are Wails-free and headless-testable. Ordering is
predictable. A slow handler could delay a producer — mitigated by the
"non-blocking handler" rule and by Phase 2/3 dispatching slow work to worker
goroutines where needed.

**Alternatives considered:** Channel-based async bus (rejected — adds
backpressure complexity for little benefit in a monolith); direct Wails event
emission from producers (rejected — couples domain to Wails).
