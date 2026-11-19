// Package sqlite — measurement repository (SPEC §6, §9.1).
//
// These methods on *DB implement storage.MeasurementWriter and
// storage.MeasurementReader. Writes are batched inside a single transaction
// (SPEC §9.1 "batch writes"): a scheduler tick produces one measurement per
// target, but the same path handles bulk back-fills from analytics without
// per-row commit overhead or lock churn.
//
// Storage conventions (0001_init.sql): timestamp is INTEGER Unix-ms UTC;
// latency_ms is REAL and NULL when there is no value (timeout / insufficient);
// metadata is JSON TEXT. The mapping lives here so the rest of the domain
// works with typed measurement.Measurement values only.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ankhov/NDE/internal/measurement"
)

// WriteMeasurements batch-inserts measurements in one transaction. A nil
// latency is stored as SQL NULL (not 0) so the "no value" state round-trips.
// Metadata is serialised to JSON; a nil map is stored as NULL.
func (d *DB) WriteMeasurements(ctx context.Context, ms []measurement.Measurement) error {
	if len(ms) == 0 {
		return nil
	}
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("storage: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op once committed

	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO measurements
  (target_id, timestamp, measurement_type, latency_ms, success, error_code, metadata)
VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("storage: prepare insert: %w", err)
	}
	defer stmt.Close()

	for _, m := range ms {
		var lat sql.NullFloat64
		if m.Latency != nil {
			lat = sql.NullFloat64{Float64: *m.Latency, Valid: true}
		}
		var meta sql.NullString
		if len(m.Metadata) > 0 {
			b, err := json.Marshal(m.Metadata)
			if err != nil {
				return fmt.Errorf("storage: marshal metadata: %w", err)
			}
			meta = sql.NullString{String: string(b), Valid: true}
		}
		if _, err := stmt.ExecContext(ctx,
			m.TargetID,
			m.Timestamp.UnixMilli(),
			string(m.Type),
			lat,
			boolToInt(m.Success),
			m.ErrorCode,
			meta,
		); err != nil {
			return fmt.Errorf("storage: insert measurement: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("storage: commit measurements: %w", err)
	}
	return nil
}

// QueryMeasurements reads measurements matching f. Filters are AND-combined;
// an empty filter returns the most recent measurements (ordered newest-first).
// A zero Limit defaults to 1000 to bound unbounded scans.
func (d *DB) QueryMeasurements(ctx context.Context, f measurement.QueryFilter) ([]measurement.Measurement, error) {
	var (
		clauses []string
		args    []any
	)
	if len(f.TargetIDs) > 0 {
		clauses = append(clauses, "target_id IN ("+placeholders(len(f.TargetIDs))+")")
		for _, id := range f.TargetIDs {
			args = append(args, id)
		}
	}
	if len(f.Types) > 0 {
		clauses = append(clauses, "measurement_type IN ("+placeholders(len(f.Types))+")")
		for _, t := range f.Types {
			args = append(args, string(t))
		}
	}
	if !f.From.IsZero() {
		clauses = append(clauses, "timestamp >= ?")
		args = append(args, f.From.UnixMilli())
	}
	if !f.To.IsZero() {
		clauses = append(clauses, "timestamp <= ?")
		args = append(args, f.To.UnixMilli())
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 1000
	}

	q := `SELECT target_id, timestamp, measurement_type, latency_ms, success, error_code, metadata
	      FROM measurements`
	if len(clauses) > 0 {
		q += " WHERE " + strings.Join(clauses, " AND ")
	}
	q += " ORDER BY timestamp DESC LIMIT ?"
	args = append(args, limit)

	rows, err := d.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("storage: query measurements: %w", err)
	}
	defer rows.Close()

	out := make([]measurement.Measurement, 0, limit)
	for rows.Next() {
		var (
			targetID    string
			tsMS        int64
			mtype       string
			lat         sql.NullFloat64
			successInt  int
			errorCode   string
			meta        sql.NullString
		)
		if err := rows.Scan(&targetID, &tsMS, &mtype, &lat, &successInt, &errorCode, &meta); err != nil {
			return nil, fmt.Errorf("storage: scan measurement: %w", err)
		}
		m := measurement.Measurement{
			TargetID:  targetID,
			Timestamp: time.UnixMilli(tsMS).UTC(),
			Type:      measurement.Type(mtype),
			Success:   successInt != 0,
			ErrorCode: errorCode,
		}
		if lat.Valid {
			v := lat.Float64
			m.Latency = &v
		}
		if meta.Valid && meta.String != "" {
			var md map[string]any
			if err := json.Unmarshal([]byte(meta.String), &md); err == nil {
				m.Metadata = md
			}
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// placeholders returns "?,?,?" for n parameters.
func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat("?,", n-1) + "?"
}

// boolToInt converts a bool to the INTEGER 0/1 convention used by the schema.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
