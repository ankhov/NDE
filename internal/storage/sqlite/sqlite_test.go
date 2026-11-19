package sqlite

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ankhov/NDE/internal/storage/migrations"
)

// expectedTables mirrors SPEC §6 plus the documented gap-fix tables and the
// migrator's bookkeeping table.
var expectedTables = []string{
	"targets",
	"measurements",
	"network_interfaces",
	"routes",
	"route_hops",
	"anomalies",
	"anomaly_measurements",
	"diagnostic_events",
	"diagnostic_evidence",
	"diagnostic_recommendations",
	"metric_aggregates",
	"notifications",
	"settings",
	"schema_migrations",
}

func TestOpenAndMigrate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nde.db")

	db, err := Open(Options{Path: path})
	require.NoError(t, err)
	defer db.Close()

	require.NoError(t, db.Migrate(context.Background()))

	// --- all expected tables exist ---
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table'")
	require.NoError(t, err)
	got := map[string]bool{}
	for rows.Next() {
		var n string
		require.NoError(t, rows.Scan(&n))
		got[strings.ToLower(n)] = true
	}
	require.NoError(t, rows.Err())
	rows.Close()

	for _, want := range expectedTables {
		if !got[want] {
			t.Errorf("missing table: %s", want)
		}
	}

	// --- WAL mode (SPEC §2.3) ---
	var mode string
	require.NoError(t, db.QueryRow("PRAGMA journal_mode").Scan(&mode))
	assert.Equal(t, "wal", strings.ToLower(mode), "expected WAL journal mode")

	// --- foreign keys on (SPEC §6 integrity) ---
	var fk int
	require.NoError(t, db.QueryRow("PRAGMA foreign_keys").Scan(&fk))
	assert.Equal(t, 1, fk, "expected foreign_keys = ON")

	// --- migration version recorded ---
	v, err := db.MigrationVersion(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, v)

	// --- time-series index present (SPEC §6) ---
	var name string
	err = db.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='index' AND name='idx_measurements_target_time'",
	).Scan(&name)
	require.NoError(t, err, "idx_measurements_target_time missing")
	assert.Equal(t, "idx_measurements_target_time", name)

	// --- re-running migrate is idempotent ---
	require.NoError(t, db.Migrate(context.Background()))
	v2, err := db.MigrationVersion(context.Background())
	require.NoError(t, err)
	assert.Equal(t, v, v2, "migrate should be idempotent")
}

func TestMigrateViaMigrationsPackage(t *testing.T) {
	// The migrator can be applied directly through the migrations package,
	// decoupled from the sqlite wrapper.
	dir := t.TempDir()
	path := filepath.Join(dir, "nde2.db")
	db, err := Open(Options{Path: path})
	require.NoError(t, err)
	defer db.Close()

	require.NoError(t, migrations.Migrate(context.Background(), db.DB))
	v, err := migrations.Version(context.Background(), db.DB)
	require.NoError(t, err)
	assert.Equal(t, 1, v)
}
