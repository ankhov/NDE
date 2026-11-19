// Package config loads application configuration.
//
// Bootstrap precedence: defaults < environment variables. Runtime settings
// (UI-configurable thresholds, retention, notifications) live in the
// `settings` table and are layered on top in later phases; this package only
// owns the static bootstrap config needed before the database is open.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Config is the bootstrap application configuration.
type Config struct {
	// DBPath is the SQLite database file path.
	DBPath string
	// LogFile is the optional structured log file path.
	LogFile string
	// LogLevel: debug | info | warn | error.
	LogLevel string
	// LogFormat: json | text.
	LogFormat string
	// MaxParallelChecks bounds concurrent collector executions (SPEC §9.1).
	MaxParallelChecks int
	// RetentionDays: raw measurements are retained this many days (SPEC §6).
	RetentionDays int
	// AggregateDays: aggregated statistics are retained this many days.
	AggregateDays int
	// ShutdownTimeout bounds graceful shutdown.
	ShutdownTimeout time.Duration
}

// Default returns a sane default config. The database/log live in the
// per-user OS config dir (e.g. %APPDATA%\nde on Windows), falling back to
// ./db for development.
func Default() Config {
	dataDir := defaultDataDir()
	return Config{
		DBPath:            filepath.Join(dataDir, "nde.db"),
		LogFile:           filepath.Join(dataDir, "nde.log"),
		LogLevel:          "info",
		LogFormat:         "json",
		MaxParallelChecks: 8,
		RetentionDays:     7,
		AggregateDays:     365,
		ShutdownTimeout:   5 * time.Second,
	}
}

func defaultDataDir() string {
	if d, err := os.UserConfigDir(); err == nil && d != "" {
		return filepath.Join(d, "nde")
	}
	return filepath.Join(".", "db")
}

// Load applies environment-variable overrides on top of Default and validates
// the result. Recognised variables: NDE_DB_PATH, NDE_LOG_FILE, NDE_LOG_LEVEL,
// NDE_LOG_FORMAT, NDE_MAX_PARALLEL, NDE_RETENTION_DAYS, NDE_SHUTDOWN_TIMEOUT_MS.
func Load() (Config, error) {
	cfg := Default()

	if v := os.Getenv("NDE_DB_PATH"); v != "" {
		cfg.DBPath = v
	}
	if v := os.Getenv("NDE_LOG_FILE"); v != "" {
		cfg.LogFile = v
	}
	if v := os.Getenv("NDE_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	if v := os.Getenv("NDE_LOG_FORMAT"); v != "" {
		cfg.LogFormat = v
	}
	if v := os.Getenv("NDE_MAX_PARALLEL"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return cfg, fmt.Errorf("config: invalid NDE_MAX_PARALLEL %q", v)
		}
		cfg.MaxParallelChecks = n
	}
	if v := os.Getenv("NDE_RETENTION_DAYS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return cfg, fmt.Errorf("config: invalid NDE_RETENTION_DAYS %q", v)
		}
		cfg.RetentionDays = n
	}
	if v := os.Getenv("NDE_SHUTDOWN_TIMEOUT_MS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 100 {
			return cfg, fmt.Errorf("config: invalid NDE_SHUTDOWN_TIMEOUT_MS %q", v)
		}
		cfg.ShutdownTimeout = time.Duration(n) * time.Millisecond
	}

	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// Validate checks the config for internal consistency.
func (c Config) Validate() error {
	if c.DBPath == "" {
		return fmt.Errorf("config: db path is required")
	}
	if c.MaxParallelChecks < 1 {
		return fmt.Errorf("config: max parallel checks must be >= 1")
	}
	if c.RetentionDays < 1 {
		return fmt.Errorf("config: retention days must be >= 1")
	}
	if c.ShutdownTimeout <= 0 {
		return fmt.Errorf("config: shutdown timeout must be positive")
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("config: invalid log level %q", c.LogLevel)
	}
	switch c.LogFormat {
	case "json", "text":
	default:
		return fmt.Errorf("config: invalid log format %q", c.LogFormat)
	}
	return nil
}
