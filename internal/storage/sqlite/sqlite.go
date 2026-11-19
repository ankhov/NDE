// Package sqlite provides the SQLite storage backend (SPEC §2.3, §6).
//
// It uses modernc.org/sqlite (pure-Go, CGO-free) so the binary cross-compiles
// cleanly to Windows. The database is opened with WAL mode, a busy timeout,
// foreign-key enforcement and NORMAL synchronisation — the recommended setup
// for a concurrent desktop write workload (SPEC §2.3 "WAL mode", §9.1).
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // registers the "sqlite" driver

	"github.com/ankhov/NDE/internal/storage/migrations"
)

// DB wraps a *sql.DB with SQLite-specific configuration and migration support.
type DB struct {
	*sql.DB
}

// Options configures database open behaviour.
type Options struct {
	// Path is the database file path.
	Path string
	// BusyTimeoutMs is the SQLite busy timeout (default 5000).
	BusyTimeoutMs int
}

// Open opens (or creates) the database at opts.Path, applies PRAGMAs and
// returns a *DB. The parent directory is created if necessary.
func Open(opts Options) (*DB, error) {
	path := opts.Path
	if path == "" {
		return nil, fmt.Errorf("sqlite: empty path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("sqlite: create db dir: %w", err)
	}
	busy := opts.BusyTimeoutMs
	if busy <= 0 {
		busy = 5000
	}

	// modernc DSN: use the file path directly. Pragmas applied via Exec below
	// to stay explicit and driver-version independent.
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open: %w", err)
	}

	if err := applyPragmas(context.Background(), db, busy); err != nil {
		_ = db.Close()
		return nil, err
	}
	// Single writer connection is the safe SQLite concurrency model; reads
	// still multiplex. SetMaxOpenConns(1) avoids "database is locked" under
	// concurrent writers in WAL mode with our batch-write pattern.
	db.SetMaxOpenConns(1)

	return &DB{db}, nil
}

func applyPragmas(ctx context.Context, db *sql.DB, busyTimeoutMs int) error {
	pragmas := []string{
		fmt.Sprintf("PRAGMA busy_timeout = %d", busyTimeoutMs),
		"PRAGMA journal_mode = WAL",
		"PRAGMA foreign_keys = ON",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA temp_store = MEMORY",
	}
	for _, p := range pragmas {
		if _, err := db.ExecContext(ctx, p); err != nil {
			return fmt.Errorf("sqlite: pragma [%s]: %w", p, err)
		}
	}
	return nil
}

// Migrate applies all pending embedded migrations (SPEC §6).
func (d *DB) Migrate(ctx context.Context) error {
	return migrations.Migrate(ctx, d.DB)
}

// MigrationVersion returns the highest applied migration version.
func (d *DB) MigrationVersion(ctx context.Context) (int, error) {
	return migrations.Version(ctx, d.DB)
}

// Close closes the underlying database handle.
func (d *DB) Close() error {
	if d.DB == nil {
		return nil
	}
	return d.DB.Close()
}
