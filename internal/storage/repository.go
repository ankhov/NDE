// Package storage defines repository interfaces used by the domain layer.
//
// Domain modules depend on these interfaces, never on the concrete SQLite
// implementation (internal/storage/sqlite), so the entire domain is testable
// with in-memory fakes (SPEC §3, rule 14). The Migrator is implemented in
// Phase 1; the Repository implementation grows across Phases 3–5.
package storage

import (
	"context"

	"github.com/ankhov/NDE/internal/measurement"
)

// MeasurementWriter batch-inserts measurements (SPEC §9.1: batch writes).
type MeasurementWriter interface {
	WriteMeasurements(ctx context.Context, ms []measurement.Measurement) error
}

// MeasurementReader queries measurements by filter.
type MeasurementReader interface {
	QueryMeasurements(ctx context.Context, f measurement.QueryFilter) ([]measurement.Measurement, error)
}

// TargetStore persists monitoring targets (SPEC §4.5).
type TargetStore interface {
	UpsertTarget(ctx context.Context, t measurement.Target) error
	DeleteTarget(ctx context.Context, id string) error
	ListTargets(ctx context.Context) ([]measurement.Target, error)
	GetTarget(ctx context.Context, id string) (measurement.Target, error)
}

// Migrator applies database schema migrations. Implemented in Phase 1.
type Migrator interface {
	Migrate(ctx context.Context) error
}

// Closer releases storage resources (database handle, etc.).
type Closer interface {
	Close() error
}

// Repository composes the storage interfaces. It is the contract every domain
// module talks to. Phase 1 declares it; the SQLite implementation is extended
// across Phases 3–5 (anomalies, diagnostics, aggregates).
type Repository interface {
	MeasurementWriter
	MeasurementReader
	TargetStore
}
