// Package measurement — canonical error codes, metadata keys and status
// derivation shared by collectors, storage, analytics and diagnostics
// (SPEC §4.2, §4.4, §7).
//
// Error codes are short, stable strings persisted in measurements.error_code.
// They are NOT Go errors: a non-empty code means the network target failed
// (Success=false), which is a normal measurement outcome, not a collector
// fault. Collectors return a Go error only when they themselves are broken
// (collector.Collect contract).
package measurement

// ErrorCode constants. The empty string means "no error" (Success=true).
// Keep these lowercase, snake_case and stable — analytics and diagnostics
// switch on them (SPEC §4.4).
const (
	// CodeTimeout: the check exceeded the target's configured timeout.
	CodeTimeout = "timeout"
	// CodeRefused: the target actively refused the connection (RST / ICMP
	// unreachable). The host is up; the port/service is not.
	CodeRefused = "refused"
	// CodeUnreachable: no route to the host, or the host did not respond at
	// all (no RST). Includes DNS resolution failure for TCP/HTTP targets.
	CodeUnreachable = "unreachable"
	// CodeDNSError: the DNS exchange failed at the protocol level (bad
	// response, SERVFAIL, malformed packet) — distinct from "the resolver
	// is unreachable" (CodeTimeout/CodeUnreachable).
	CodeDNSError = "dns_error"
	// CodeHTTPStatus: an HTTP response was received but its status code was
	// outside the success range (2xx). metadata.status_code holds the code.
	CodeHTTPStatus = "http_status"
	// CodeHTTPError: the HTTP transport failed before any response
	// (connection reset, TLS handshake failure, EOF).
	CodeHTTPError = "http_error"
	// CodeSSRFBlocked: the resolved IP was rejected by the SSRF guard
	// (SPEC §9.3). metadata.resolved_ip holds the rejected address.
	CodeSSRFBlocked = "ssrf_blocked"
	// CodeInvalidConfig: the target is structurally unusable for this
	// collector (e.g. TCP target with port 0). The target should be fixed,
	// not retried blindly.
	CodeInvalidConfig = "invalid_config"
	// CodeUnsupported: no collector is registered for this measurement type
	// yet (e.g. icmp / traceroute before Phase 3). The scheduler records the
	// tick so the UI can show "not yet collected" instead of silently doing
	// nothing.
	CodeUnsupported = "unsupported"
)

// Canonical metadata keys. Using constants keeps collectors, storage and
// diagnostics aligned on field names without a shared schema struct.
const (
	MetaStatusCode  = "status_code" // HTTP response status (int)
	MetaDNSRcode    = "dns_rcode"   // DNS response code (int)
	MetaResolvedIP  = "resolved_ip" // IP the collector actually contacted
	MetaQueryName   = "query_name"  // DNS question name probed
	MetaAnswer      = "answer"      // first DNS answer (string)
	MetaPort        = "port"        // TCP/UDP port contacted
	MetaFailureMS   = "failure_ms"  // wall time elapsed before a failure
)

// Status classifies a measurement for the UI and for metric_aggregates.status
// (SPEC §6 gap-fix C3). It is derived, never stored as a separate field, so
// the rule lives in exactly one place.
type Status string

const (
	// StatusOK: the check succeeded and produced a latency value.
	StatusOK = "ok"
	// StatusInsufficient: the check succeeded but produced no latency value
	// (e.g. a traceroute hop that did not respond). Not an error.
	StatusInsufficient = "insufficient_data"
	// StatusError: the check failed (Success=false).
	StatusError = "error"
)

// Status derives the measurement status from its fields (SPEC §4.2).
//
//	Success && Latency != nil -> ok
//	Success && Latency == nil -> insufficient_data
//	!Success                  -> error
func (m Measurement) Status() Status {
	switch {
	case !m.Success:
		return StatusError
	case m.Latency != nil:
		return StatusOK
	default:
		return StatusInsufficient
	}
}

// IsTerminalError reports whether the error code represents a target-side
// failure that should NOT be retried within the same interval (e.g. SSRF
// block or invalid config). Transient codes (timeout, refused, unreachable)
// are not terminal — the next interval may succeed.
func IsTerminalError(code string) bool {
	switch code {
	case CodeSSRFBlocked, CodeInvalidConfig, CodeUnsupported:
		return true
	}
	return false
}
