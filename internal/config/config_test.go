package config

import (
	"testing"
)

func TestDefaultIsValid(t *testing.T) {
	if err := Default().Validate(); err != nil {
		t.Fatalf("default config invalid: %v", err)
	}
}

func TestLoadRejectsBadEnv(t *testing.T) {
	t.Setenv("NDE_MAX_PARALLEL", "0")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for NDE_MAX_PARALLEL=0")
	}
}

func TestLoadRejectsBadLogLevel(t *testing.T) {
	t.Setenv("NDE_LOG_LEVEL", "verbose")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for invalid log level")
	}
}

func TestLoadAppliesOverrides(t *testing.T) {
	t.Setenv("NDE_DB_PATH", "/tmp/nde-test.db")
	t.Setenv("NDE_MAX_PARALLEL", "16")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.DBPath != "/tmp/nde-test.db" {
		t.Errorf("db path not overridden: %q", cfg.DBPath)
	}
	if cfg.MaxParallelChecks != 16 {
		t.Errorf("max parallel not overridden: %d", cfg.MaxParallelChecks)
	}
}

func TestValidateShutsdownTimeoutPositive(t *testing.T) {
	cfg := Default()
	cfg.ShutdownTimeout = 0
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for zero shutdown timeout")
	}
}
