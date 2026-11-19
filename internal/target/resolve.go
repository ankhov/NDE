// Package target — resolution-time SSRF guard (SPEC §9.3).
//
// Validate/SSRFGuard check the host literal only. But a hostname like
// "evil.example" can resolve to 169.254.169.254 (the cloud metadata endpoint)
// or 127.0.0.1 at request time. ResolveGuarded resolves the hostname and
// applies the same private-range screening to every returned address, so a
// target whose hostname resolves to a private IP is blocked before any dial.
//
// Collectors call ResolveGuarded immediately before connecting and dial the
// returned (screened) IP directly, which closes the TOCTOU window that would
// exist if the collector let the transport re-resolve the hostname itself.
package target

import (
	"context"
	"fmt"
	"net"
)

// ResolveGuarded resolves host to IP addresses and rejects the resolution if
// any returned address is private and opts.AllowPrivate is false. A literal
// IP is returned as a single-element slice without DNS traffic. The error is
// wrapped ErrForbiddenHost when the guard rejects the host, so callers can
// distinguish SSRF blocks from real resolution failures.
func ResolveGuarded(ctx context.Context, host string, opts SSRFOptions) ([]net.IP, error) {
	h := stripPort(host)
	if h == "" {
		return nil, ErrEmptyHost
	}
	if ip := net.ParseIP(h); ip != nil {
		if !opts.AllowPrivate && isPrivate(ip) {
			return nil, fmt.Errorf("%w: %s", ErrForbiddenHost, ip)
		}
		return []net.IP{ip}, nil
	}
	// Hostname: resolve and screen every answer.
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, h)
	if err != nil {
		return nil, fmt.Errorf("%w: resolve %s: %v", ErrUnreachable, h, err)
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("%w: %s resolved to no addresses", ErrUnreachable, h)
	}
	out := make([]net.IP, 0, len(addrs))
	for _, a := range addrs {
		if !opts.AllowPrivate && isPrivate(a.IP) {
			return nil, fmt.Errorf("%w: %s resolves to private %s", ErrForbiddenHost, h, a.IP)
		}
		out = append(out, a.IP)
	}
	return out, nil
}

// ErrUnreachable is returned by ResolveGuarded when DNS resolution fails or
// returns no usable address. It is distinct from ErrForbiddenHost (an SSRF
// rejection) so collectors can map it to the right measurement error code.
var ErrUnreachable = fmt.Errorf("target: host is unreachable or unresolvable")
