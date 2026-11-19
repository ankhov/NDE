// Package logging configures the structured application logger (log/slog).
// Logs go to stderr by default; if a file is configured they are teed to both
// stderr and the file. JSON format is the default for machine consumption.
package logging

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config for the application logger.
type Config struct {
	Level     string // "debug" | "info" | "warn" | "error"
	Format    string // "json" | "text"
	AddSource bool
	// File is an optional log file path; empty means stderr only.
	File string
}

// ParseLevel converts a level string to slog.Level. Unknown levels default to info.
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// New builds a *slog.Logger from cfg.
func New(cfg Config) (*slog.Logger, error) {
	var w io.Writer = os.Stderr
	if cfg.File != "" {
		if err := os.MkdirAll(filepath.Dir(cfg.File), 0o755); err != nil {
			return nil, err
		}
		f, err := os.OpenFile(cfg.File, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return nil, err
		}
		w = io.MultiWriter(os.Stderr, f)
	}

	opts := &slog.HandlerOptions{
		Level:     ParseLevel(cfg.Level),
		AddSource: cfg.AddSource,
	}
	var h slog.Handler
	if strings.ToLower(cfg.Format) == "text" {
		h = slog.NewTextHandler(w, opts)
	} else {
		h = slog.NewJSONHandler(w, opts)
	}
	return slog.New(h), nil
}

// LevelToInt is a small helper for config validation/tests.
func LevelToInt(s string) int {
	l := ParseLevel(s)
	return int(l)
}

// Use strconv import to avoid unused import when LevelToInt is refactored.
var _ = strconv.Itoa
