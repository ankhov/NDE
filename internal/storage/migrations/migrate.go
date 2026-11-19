// Package migrations embeds SQL migration files and applies them to a SQLite
// database in order, tracking applied versions in a schema_migrations table.
//
// This is a small, dependency-free migrator (no external tool) so the binary
// stays self-contained (SPEC §6 "Миграции базы данных"). Each migration file
// runs in its own transaction; a failure rolls back and aborts.
package migrations

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed *.sql
var migrationFS embed.FS

// Migrate applies all pending embedded migrations to db.
func Migrate(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		applied_at INTEGER NOT NULL
	);`); err != nil {
		return fmt.Errorf("migrations: create schema_migrations: %w", err)
	}

	applied, err := readAppliedVersions(ctx, db)
	if err != nil {
		return err
	}

	files, err := migrationFiles()
	if err != nil {
		return err
	}

	for _, name := range files {
		version, err := versionFromName(name)
		if err != nil {
			return fmt.Errorf("migrations: %s: %w", name, err)
		}
		if applied[version] {
			continue
		}
		if err := applyFile(ctx, db, name, version); err != nil {
			return fmt.Errorf("migrations: apply %s: %w", name, err)
		}
	}
	return nil
}

// Version returns the highest applied migration version, or 0 if none.
func Version(ctx context.Context, db *sql.DB) (int, error) {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		applied_at INTEGER NOT NULL
	);`); err != nil {
		return 0, err
	}
	row := db.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_migrations")
	var v int
	if err := row.Scan(&v); err != nil {
		return 0, err
	}
	return v, nil
}

func readAppliedVersions(ctx context.Context, db *sql.DB) (map[int]bool, error) {
	rows, err := db.QueryContext(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("migrations: read applied: %w", err)
	}
	defer rows.Close()
	out := make(map[int]bool)
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out[v] = true
	}
	return out, rows.Err()
}

func migrationFiles() ([]string, error) {
	entries, err := migrationFS.ReadDir(".")
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		files = append(files, e.Name())
	}
	sort.Strings(files)
	return files, nil
}

func versionFromName(name string) (int, error) {
	i := 0
	for i < len(name) && name[i] >= '0' && name[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0, fmt.Errorf("no leading version digits")
	}
	return strconv.Atoi(name[:i])
}

func applyFile(ctx context.Context, db *sql.DB, name string, version int) error {
	content, err := migrationFS.ReadFile(name)
	if err != nil {
		return err
	}
	stmts := splitStatements(string(content))

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for _, s := range stmts {
		if _, err := tx.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("statement [%s]: %w", firstLine(s), err)
		}
	}
	if _, err := tx.ExecContext(
		ctx,
		"INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)",
		version, time.Now().UTC().UnixMilli(),
	); err != nil {
		return err
	}
	return tx.Commit()
}

// splitStatements splits a SQL script into individual statements, handling
// line comments (--), block comments (/* */) and single-quoted string
// literals. A trailing statement without a semicolon is also returned.
func splitStatements(src string) []string {
	var (
		out      []string
		sb       strings.Builder
		inStr    bool
		inLine   bool
		inBlock  bool
	)
	for i := 0; i < len(src); i++ {
		c := src[i]
		switch {
		case inLine:
			sb.WriteByte(c)
			if c == '\n' {
				inLine = false
			}
		case inBlock:
			sb.WriteByte(c)
			if c == '*' && i+1 < len(src) && src[i+1] == '/' {
				sb.WriteByte('/')
				inBlock = false
				i++
			}
		case inStr:
			sb.WriteByte(c)
			if c == '\'' {
				inStr = false
			}
		case c == '\'' && !inStr:
			inStr = true
			sb.WriteByte(c)
		case c == '-' && i+1 < len(src) && src[i+1] == '-':
			inLine = true
			sb.WriteByte('-')
			sb.WriteByte('-')
			i++
		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			inBlock = true
			sb.WriteByte('/')
			sb.WriteByte('*')
			i++
		case c == ';':
			s := stripComments(sb.String())
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
			sb.Reset()
		default:
			sb.WriteByte(c)
		}
	}
	if s := stripComments(sb.String()); s != "" {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// stripComments removes -- line comments and /* */ block comments from a
// statement so that comment-only fragments become empty (and are skipped).
func stripComments(s string) string {
	var sb strings.Builder
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == '-' && i+1 < len(s) && s[i+1] == '-':
			// skip to end of line
			for i < len(s) && s[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < len(s) && s[i+1] == '*':
			i += 2
			for i+1 < len(s) && !(s[i] == '*' && s[i+1] == '/') {
				i++
			}
			i += 2
			continue
		default:
			sb.WriteByte(c)
			i++
		}
	}
	return sb.String()
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
