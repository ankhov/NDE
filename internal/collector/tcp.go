// Package collector — TCP-connect collector (SPEC §3.3, §4.4).
//
// It measures the round-trip time of completing a TCP three-way handshake to
// (target.Host, target.Port). On Windows this is the primary latency signal
// until the unprivileged ICMP path lands in Phase 3 (ADR-002): it needs no
// raw sockets and works for any reachable TCP endpoint.
//
// The collector resolves target.Host through the SSRF guard and dials the
// screened IP directly, so a hostname that resolves to a private address is
// blocked before the connection (SPEC §9.3).
package collector

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"strconv"
	"time"

	"github.com/ankhov/NDE/internal/measurement"
	"github.com/ankhov/NDE/internal/target"
)

// TCPOptions configures the TCP collector.
type TCPOptions struct {
	// AllowPrivate permits dialing loopback/private/link-local addresses.
	// Default false: user-supplied targets must not reach internal hosts.
	AllowPrivate bool
}

// TCP measures TCP handshake RTT.
type TCP struct {
	opts TCPOptions
	log  *slog.Logger
}

// NewTCP returns a TCP collector.
func NewTCP(opts TCPOptions) *TCP {
	return &TCP{opts: opts, log: slog.Default()}
}

// WithLogger sets the collector logger.
func (c *TCP) WithLogger(l *slog.Logger) *TCP { c.log = l; return c }

// Name returns the collector identifier.
func (c *TCP) Name() string { return "tcp" }

// Supports reports whether the collector handles the given type.
func (c *TCP) Supports(t measurement.Type) bool { return t == measurement.TypeTCP }

// Collect runs one TCP-connect check.
func (c *TCP) Collect(ctx context.Context, t measurement.Target) (measurement.Measurement, error) {
	res := measurement.Measurement{
		TargetID:  t.ID,
		Timestamp: time.Now().UTC(),
		Type:      measurement.TypeTCP,
	}
	if t.Port <= 0 {
		res.ErrorCode = measurement.CodeInvalidConfig
		res.Metadata = map[string]any{measurement.MetaPort: t.Port}
		return res, nil
	}
	deadline := t.Timeout
	if deadline <= 0 {
		deadline = 3 * time.Second
	}

	ips, err := target.ResolveGuarded(ctx, t.Host, target.SSRFOptions{AllowPrivate: c.opts.AllowPrivate})
	if err != nil {
		if errors.Is(err, target.ErrForbiddenHost) {
			res.ErrorCode = measurement.CodeSSRFBlocked
			res.Metadata = map[string]any{measurement.MetaResolvedIP: t.Host}
			return res, nil
		}
		res.ErrorCode = measurement.CodeUnreachable
		return res, nil
	}
	ip := ips[0]
	addr := net.JoinHostPort(ip.String(), strconv.Itoa(t.Port))

	start := time.Now()
	d := net.Dialer{Timeout: deadline}
	conn, derr := d.DialContext(ctx, "tcp", addr)
	elapsed := time.Since(start)

	if derr != nil {
		res.ErrorCode = classifyDialError(derr)
		res.Metadata = map[string]any{
			measurement.MetaResolvedIP: ip.String(),
			measurement.MetaPort:       t.Port,
			measurement.MetaFailureMS:  roundMS(elapsed),
		}
		return res, nil
	}
	_ = conn.Close()

	lat := roundMS(elapsed)
	res.Success = true
	res.Latency = &lat
	res.Metadata = map[string]any{
		measurement.MetaResolvedIP: ip.String(),
		measurement.MetaPort:       t.Port,
	}
	return res, nil
}

// classifyDialError maps a net.OpError to a stable measurement error code.
// Timeouts and deadline-exceeded become CodeTimeout; connection-refused
// becomes CodeRefused; everything else (no route, unresolved at dial time,
// reset) collapses to CodeUnreachable — the host did not complete the
// handshake.
func classifyDialError(err error) string {
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return measurement.CodeTimeout
	}
	if isConnectionRefused(err) {
		return measurement.CodeRefused
	}
	return measurement.CodeUnreachable
}

// roundMS returns a duration in milliseconds rounded to 3 decimal places.
func roundMS(d time.Duration) float64 {
	return float64(d.Microseconds()) / 1000.0
}

func isConnectionRefused(err error) bool {
	// "connection refused" surfaces as a syscall ECONNREFUSED on both platforms.
	s := err.Error()
	return contains(s, "refused") || contains(s, "connection reset")
}

// contains is a tiny strings.Contains alias kept local to avoid importing
// strings in this file.
func contains(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
