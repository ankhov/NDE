// Package target validates monitoring targets and guards against SSRF
// (SPEC §9.3): user-supplied HTTP/TCP targets must not reach loopback,
// private, link-local, multicast or reserved addresses.
//
// Validation here covers literals only; DNS-resolution-time guarding is
// performed by collectors at request time (Phase 2) so that a target whose
// hostname resolves to a private IP is still blocked.
package target

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/ankhov/NDE/internal/measurement"
)

// Validation errors.
var (
	ErrEmptyName      = errors.New("target: name must not be empty")
	ErrEmptyHost      = errors.New("target: host must not be empty")
	ErrInvalidPort    = errors.New("target: port out of range")
	ErrInvalidProtocol = errors.New("target: unsupported protocol")
	ErrInvalidGroup   = errors.New("target: unsupported group")
	ErrInvalidInterval = errors.New("target: check interval must be positive")
	ErrInvalidTimeout = errors.New("target: timeout must be positive")
	ErrThresholdRange = errors.New("target: thresholds must be >= 0 and warning <= critical")
	ErrForbiddenHost  = errors.New("target: host is forbidden (SSRF guard)")
)

// Validate checks a target's fields. It does NOT perform DNS resolution.
func Validate(t measurement.Target) error {
	if strings.TrimSpace(t.Name) == "" {
		return ErrEmptyName
	}
	if strings.TrimSpace(t.Host) == "" {
		return ErrEmptyHost
	}
	if t.Port < 0 || t.Port > 65535 {
		return ErrInvalidPort
	}
	if !t.Protocol.Valid() {
		return fmt.Errorf("%w: %s", ErrInvalidProtocol, t.Protocol)
	}
	if !t.Group.Valid() {
		return fmt.Errorf("%w: %s", ErrInvalidGroup, t.Group)
	}
	if t.CheckInterval <= 0 {
		return ErrInvalidInterval
	}
	if t.Timeout <= 0 {
		return ErrInvalidTimeout
	}
	if t.WarningThreshold < 0 || t.CriticalThreshold < 0 ||
		t.WarningThreshold > t.CriticalThreshold {
		return ErrThresholdRange
	}
	return nil
}

// SSRFOptions controls how strictly a host is screened.
type SSRFOptions struct {
	// AllowPrivate permits loopback/private/link-local addresses. Use only for
	// explicitly trusted local targets (e.g. the default gateway).
	AllowPrivate bool
}

// SSRFGuard examines a host literal. A literal IP is rejected if it falls in
// a private/loopback/link-local/multicast/unreserved range and AllowPrivate is
// false. The hostname "localhost" is treated as private. Bare hostnames are
// accepted at this stage (resolution-time check is the collector's job).
func SSRFGuard(host string, opts SSRFOptions) error {
	h := stripPort(strings.TrimSpace(host))
	if h == "" {
		return ErrEmptyHost
	}
	if strings.EqualFold(h, "localhost") {
		if !opts.AllowPrivate {
			return ErrForbiddenHost
		}
		return nil
	}
	if ip := net.ParseIP(h); ip != nil {
		if !opts.AllowPrivate && isPrivate(ip) {
			return ErrForbiddenHost
		}
		return nil
	}
	// Hostname — defer to resolution-time guard in the collector.
	return nil
}

// stripPort removes a trailing ":port" from a host literal.
func stripPort(h string) string {
	// Preserve IPv6 brackets content: [::1]:port -> ::1
	if i := strings.LastIndex(h, ":"); i >= 0 {
		// If it looks like an unbracketed IPv6, do not strip (contains multiple colons).
		if strings.Count(h, ":") == 1 {
			return h[:i]
		}
		if strings.HasPrefix(h, "[") {
			end := strings.Index(h, "]")
			if end > 0 {
				return h[1:end]
			}
		}
	}
	return h
}

// isPrivate reports whether an IP is in a range that user targets must not
// reach without explicit opt-in.
func isPrivate(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	// Additional reserved ranges not covered by the helpers above.
	if v4 := ip.To4(); v4 != nil {
		switch {
		case v4[0] == 0: // 0.0.0.0/8
			return true
		case v4[0] == 127: // 127.0.0.0/8 (already via IsLoopback, kept for clarity)
			return true
		case v4[0] == 169 && v4[1] == 254: // 169.254.0.0/16
			return true
		case v4[0] >= 224: // 224.0.0.0/4 multicast + 240.0.0.0/4 reserved
			return true
		}
	}
	return false
}

// HostPort joins host and port into a "host:port" string suitable for dialing,
// bracketing IPv6 literals.
func HostPort(host string, port int) string {
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		return "[" + host + "]:" + strconv.Itoa(port)
	}
	return host + ":" + strconv.Itoa(port)
}

// IntervalLabel renders a check interval for logs/UI in a compact form.
func IntervalLabel(d time.Duration) string {
	if d <= 0 {
		return "—"
	}
	return d.String()
}
