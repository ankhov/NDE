# ADR-002: Windows unprivileged ICMP

**Status:** Accepted (design; implementation in Phase 2)
**Date:** 2026-11-19
**Context:** SPEC §4.1 requires ICMP Echo RTT, but §9.3 requires minimal
system privileges. Raw ICMP sockets on Windows need administrator rights, and
`golang.org/x/net/icmp` defaults to raw sockets. This is the single biggest
technical risk (R1).

**Decision:** Implement ICMP per-platform behind the `Collector` interface:
- **Windows:** use the unprivileged `IcmpSendEcho` system API via
  `golang.org/x/sys/windows` syscall. No elevation required.
- **Linux:** use the UDP-based ICMP socket trick (requires `cap_net_raw` or
  running with appropriate privileges, but not full root).
- **Fallback:** TCP-connect RTT is always available without privileges and is
  used when ICMP is unavailable or blocked; this never blocks monitoring.

The Phase 1 `Collector` interface is already designed so the implementation
swap is contained in `internal/network/icmp`.

**Consequences:** No admin prompt on Windows; degraded (TCP) mode is explicit
and measurable, never silent. SPEC §4.3 ("don't treat missing ICMP as proof of
user-traffic loss") is enforced by separating ICMP packet loss from TCP/HTTP
errors at the metrics layer (Phase 3).

**Alternatives considered:** Require admin and use raw ICMP (rejected —
violates §9.3); ICMP only (rejected — fails closed on restricted networks).
