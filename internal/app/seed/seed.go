// Package seed writes the default monitoring target set on first run
// (SPEC §4.5). The seeder runs only when the targets table is empty, so a
// user who deletes a default target will not have it re-created on the next
// launch. Every default target is an external, SSRF-safe endpoint validated
// through the same target.Validate / SSRFGuard path as user-supplied targets
// — there is no privileged seed path.
package seed

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ankhov/NDE/internal/measurement"
	"github.com/ankhov/NDE/internal/storage/sqlite"
	"github.com/ankhov/NDE/internal/target"
)

// DefaultTargets is the first-run target set. IDs are stable so a later,
// richer seeder (Phase 3: gateway detection, local DNS) can extend rather than
// duplicate them.
var DefaultTargets = []measurement.Target{
	{
		ID: "default-cf-tcp", Name: "Cloudflare 1.1.1.1 (TCP 443)",
		Host: "1.1.1.1", Protocol: measurement.TypeTCP, Port: 443,
		CheckInterval: 60 * time.Second, Timeout: 3 * time.Second,
		Enabled: true, WarningThreshold: 150, CriticalThreshold: 400,
		Group: measurement.GroupExternal,
	},
	{
		ID: "default-cf-dns", Name: "Cloudflare DNS resolver",
		Host: "1.1.1.1", Protocol: measurement.TypeDNS, Port: 53,
		CheckInterval: 60 * time.Second, Timeout: 3 * time.Second,
		Enabled: true, WarningThreshold: 100, CriticalThreshold: 300,
		Group: measurement.GroupDNS,
	},
	{
		ID: "default-google-dns", Name: "Google DNS resolver",
		Host: "8.8.8.8", Protocol: measurement.TypeDNS, Port: 53,
		CheckInterval: 60 * time.Second, Timeout: 3 * time.Second,
		Enabled: true, WarningThreshold: 100, CriticalThreshold: 300,
		Group: measurement.GroupDNS,
	},
	{
		ID: "default-example-http", Name: "example.com (HTTP)",
		Host: "https://example.com", Protocol: measurement.TypeHTTP, Port: 0,
		CheckInterval: 60 * time.Second, Timeout: 5 * time.Second,
		Enabled: true, WarningThreshold: 300, CriticalThreshold: 1000,
		Group: measurement.GroupExternal,
	},
}

// SeedDefaults inserts DefaultTargets when the targets table is empty. It
// validates and SSRF-screens each target first; a rejected default is skipped
// with a warning rather than aborting the seed (a future Phase 3 seeder may
// add local targets that fail the external-only guard).
func SeedDefaults(ctx context.Context, db *sqlite.DB, log *slog.Logger) (int, error) {
	count, err := db.CountTargets(ctx)
	if err != nil {
		return 0, fmt.Errorf("seed: count targets: %w", err)
	}
	if count > 0 {
		return 0, nil
	}
	if log == nil {
		log = slog.Default()
	}
	inserted := 0
	for _, t := range DefaultTargets {
		if err := target.Validate(t); err != nil {
			log.Warn("seed: skipping invalid default target", "id", t.ID, "err", err)
			continue
		}
		if err := target.SSRFGuard(t.Host, target.SSRFOptions{AllowPrivate: false}); err != nil {
			log.Warn("seed: skipping SSRF-blocked default target", "id", t.ID, "err", err)
			continue
		}
		if err := db.UpsertTarget(ctx, t); err != nil {
			return inserted, fmt.Errorf("seed: upsert %s: %w", t.ID, err)
		}
		inserted++
	}
	log.Info("seed: default targets written", "count", inserted)
	return inserted, nil
}
