// Package collector — default registry factory (SPEC §3.3).
//
// NewDefaultRegistry returns a Registry pre-populated with the TCP, DNS and
// HTTP collectors. ICMP and traceroute land in Phase 3 (ICMP needs the
// Windows unprivileged syscall, ADR-002; traceroute reuses ICMP + the route
// tables) — so a target of those types has no collector yet and the scheduler
// records a measurement with CodeUnsupported rather than skipping silently.
package collector

import "log/slog"

// DefaultOptions configures all collectors built by NewDefaultRegistry.
type DefaultOptions struct {
	// AllowPrivate lifts the SSRF guard for private/loopback targets across
	// all collectors. Used only when seeding trusted local targets (e.g. the
	// default gateway) — never for user-supplied external targets.
	AllowPrivate bool
	// DNSProbeName overrides the DNS collector's question name.
	DNSProbeName string
}

// NewDefaultRegistry returns a Registry with the Phase 2 collectors
// registered, all sharing the same AllowPrivate policy and a single logger.
func NewDefaultRegistry(opts DefaultOptions, log *slog.Logger) Registry {
	if log == nil {
		log = slog.Default()
	}
	reg := NewRegistry()
	reg.Register((&TCP{}).WithLogger(log))
	reg.Register(NewDNS(DNSOptions{
		AllowPrivate: opts.AllowPrivate,
		ProbeName:    opts.DNSProbeName,
	}).WithLogger(log))
	reg.Register(NewHTTP(HTTPOptions{AllowPrivate: opts.AllowPrivate}).WithLogger(log))
	return reg
}
