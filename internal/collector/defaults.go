// Package collector — default registry factory.
//
// NewDefaultRegistry returns a Registry pre-populated with all five
// collectors: TCP, DNS, HTTP, ICMP and traceroute. ICMP shells out to the OS
// ping binary (IcmpSendEcho on Windows, cap_net_raw/UDP-ICMP on unix per
// ADR-002); traceroute shells out to traceroute/tracert. A target of any of
// the five measurement types now has a live collector, so the scheduler no
// longer records CodeUnsupported for these types.
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

// NewDefaultRegistry returns a Registry with all five collectors registered,
// all sharing the same AllowPrivate policy and a single logger. Phase 3 adds
// the ICMP and traceroute collectors, completing the five-type collector set
// (icmp/tcp/dns/http/traceroute).
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
	reg.Register(NewICMP(ICMPOptions{AllowPrivate: opts.AllowPrivate}).WithLogger(log))
	reg.Register(NewTraceroute(TracerouteOptions{AllowPrivate: opts.AllowPrivate}).WithLogger(log))
	return reg
}
