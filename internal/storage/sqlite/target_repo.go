// Package sqlite — target repository (SPEC §4.5, §6).
//
// These methods on *DB implement storage.TargetStore. UpsertTarget uses an
// ON CONFLICT DO UPDATE so re-adding an existing target (e.g. from the seeder
// or the UI) updates its mutable fields without resetting created_at. Delete
// cascades through measurements via the schema's ON DELETE CASCADE.
package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/ankhov/NDE/internal/measurement"
)

const targetUpsertSQL = `
INSERT INTO targets
  (id, name, host, protocol, port, check_interval_ms, timeout_ms,
   enabled, warning_threshold, critical_threshold, target_group, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  name              = excluded.name,
  host              = excluded.host,
  protocol          = excluded.protocol,
  port              = excluded.port,
  check_interval_ms = excluded.check_interval_ms,
  timeout_ms        = excluded.timeout_ms,
  enabled           = excluded.enabled,
  warning_threshold = excluded.warning_threshold,
  critical_threshold= excluded.critical_threshold,
  target_group      = excluded.target_group`

// UpsertTarget inserts or updates a target. created_at is assigned here (the
// domain Target has no created_at field) and preserved across updates.
func (d *DB) UpsertTarget(ctx context.Context, t measurement.Target) error {
	_, err := d.ExecContext(ctx, targetUpsertSQL,
		t.ID,
		t.Name,
		t.Host,
		string(t.Protocol),
		t.Port,
		t.CheckInterval.Milliseconds(),
		t.Timeout.Milliseconds(),
		boolToInt(t.Enabled),
		t.WarningThreshold,
		t.CriticalThreshold,
		string(t.Group),
		time.Now().UTC().UnixMilli(),
	)
	if err != nil {
		return fmt.Errorf("storage: upsert target: %w", err)
	}
	return nil
}

// DeleteTarget removes a target. Measurements are cascade-deleted by the
// schema's foreign key.
func (d *DB) DeleteTarget(ctx context.Context, id string) error {
	res, err := d.ExecContext(ctx, `DELETE FROM targets WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("storage: delete target: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("storage: target %s not found", id)
	}
	return nil
}

// ListTargets returns all targets ordered by name.
func (d *DB) ListTargets(ctx context.Context) ([]measurement.Target, error) {
	rows, err := d.QueryContext(ctx, `
SELECT id, name, host, protocol, port, check_interval_ms, timeout_ms,
       enabled, warning_threshold, critical_threshold, target_group
FROM targets ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("storage: list targets: %w", err)
	}
	defer rows.Close()
	out := make([]measurement.Target, 0)
	for rows.Next() {
		t, err := scanTarget(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// GetTarget returns a single target by id. It returns an error wrapping
// sql.ErrNoRows when the target does not exist.
func (d *DB) GetTarget(ctx context.Context, id string) (measurement.Target, error) {
	row := d.QueryRowContext(ctx, `
SELECT id, name, host, protocol, port, check_interval_ms, timeout_ms,
       enabled, warning_threshold, critical_threshold, target_group
FROM targets WHERE id = ?`, id)
	t, err := scanTarget(row)
	if err != nil {
		return t, fmt.Errorf("storage: get target: %w", err)
	}
	return t, nil
}

// scanner abstracts *sql.Row and *sql.Rows so scanTarget serves both paths.
type scanner interface {
	Scan(dest ...any) error
}

func scanTarget(s scanner) (measurement.Target, error) {
	var (
		t           measurement.Target
		protocol    string
		group       string
		intervalMS  int64
		timeoutMS   int64
		enabledInt  int
	)
	err := s.Scan(
		&t.ID, &t.Name, &t.Host, &protocol, &t.Port,
		&intervalMS, &timeoutMS, &enabledInt,
		&t.WarningThreshold, &t.CriticalThreshold, &group,
	)
	if err != nil {
		return t, err
	}
	t.Protocol = measurement.Type(protocol)
	t.Group = measurement.TargetGroup(group)
	t.CheckInterval = time.Duration(intervalMS) * time.Millisecond
	t.Timeout = time.Duration(timeoutMS) * time.Millisecond
	t.Enabled = enabledInt != 0
	return t, nil
}

// CountTargets returns the number of configured targets. Used by the seeder
// to decide whether defaults need to be written on a first run.
func (d *DB) CountTargets(ctx context.Context) (int, error) {
	var n int
	err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM targets`).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("storage: count targets: %w", err)
	}
	return n, nil
}

// Compile-time assertions that *DB satisfies the storage repository contract.
var (
	_ interface {
		WriteMeasurements(context.Context, []measurement.Measurement) error
	} = (*DB)(nil)
)
