// Package collector — DNS collector (SPEC §3.3, §4.4, §4.6).
//
// It measures the round-trip time of a single DNS exchange against
// target.Host (the resolver address, e.g. "1.1.1.1"). The question name is a
// fixed reachability probe (default "example.com", type A) so the measurement
// answers "how fast does this resolver answer?" — independent of the queried
// domain's own TTL. The probe name is configurable per-collector.
//
// miekg/dns is used because it gives precise RTT, lets us target a specific
// resolver (net.LookupHost cannot), and exposes the response code needed for
// diagnostics. It is pure Go and cross-compiles to Windows without CGO.
package collector

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/miekg/dns"

	"github.com/ankhov/NDE/internal/measurement"
	"github.com/ankhov/NDE/internal/target"
)

// DNSOptions configures the DNS collector.
type DNSOptions struct {
	// AllowPrivate permits querying resolvers on private/loopback addresses
	// (e.g. a home router at 192.168.1.1). Default false.
	AllowPrivate bool
	// ProbeName is the question name queried to measure resolver latency.
	// Default "example.com". It must be a name the resolver can answer; a
	// root/hint name would measure cache-miss behaviour, which is not what
	// the dashboard's "DNS RTT" tile intends.
	ProbeName string
	// ProbeType is the DNS query type (dns.TypeA by default).
	ProbeType uint16
}

// DNS measures DNS exchange RTT against a configured resolver.
type DNS struct {
	opts DNSOptions
	log  *slog.Logger
}

// NewDNS returns a DNS collector.
func NewDNS(opts DNSOptions) *DNS {
	if opts.ProbeName == "" {
		opts.ProbeName = "example.com."
	}
	if opts.ProbeType == 0 {
		opts.ProbeType = dns.TypeA
	}
	return &DNS{opts: opts, log: slog.Default()}
}

// WithLogger sets the collector logger.
func (c *DNS) WithLogger(l *slog.Logger) *DNS { c.log = l; return c }

// Name returns the collector identifier.
func (c *DNS) Name() string { return "dns" }

// Supports reports whether the collector handles the given type.
func (c *DNS) Supports(t measurement.Type) bool { return t == measurement.TypeDNS }

// Collect runs one DNS exchange.
func (c *DNS) Collect(ctx context.Context, t measurement.Target) (measurement.Measurement, error) {
	res := measurement.Measurement{
		TargetID:  t.ID,
		Timestamp: time.Now().UTC(),
		Type:      measurement.TypeDNS,
	}
	port := t.Port
	if port <= 0 {
		port = 53
	}
	deadline := t.Timeout
	if deadline <= 0 {
		deadline = 3 * time.Second
	}

	// The resolver address is itself screened: a user target pointing at a
	// private resolver is blocked unless explicitly allowed.
	if err := target.SSRFGuard(t.Host, target.SSRFOptions{AllowPrivate: c.opts.AllowPrivate}); err != nil {
		res.ErrorCode = measurement.CodeSSRFBlocked
		res.Metadata = map[string]any{measurement.MetaResolvedIP: t.Host}
		return res, nil
	}
	addr := target.HostPort(t.Host, port)

	q := new(dns.Msg)
	q.SetQuestion(dns.Fqdn(c.opts.ProbeName), c.opts.ProbeType)
	q.RecursionDesired = true

	client := &dns.Client{
		Net:     "udp",
		Timeout: deadline,
		UDPSize: 4096,
	}

	// miekg/dns Exchange honours its own Timeout but not a caller context; wrap
	// it so a cancelled scheduler context cannot leave a query in flight.
	done := make(chan dnsResult, 1)
	go func() {
		r, rtt, err := client.Exchange(q, addr)
		done <- dnsResult{r: r, rtt: rtt, err: err}
	}()
	select {
	case <-ctx.Done():
		res.ErrorCode = measurement.CodeTimeout
		return res, nil
	case out := <-done:
		return c.fromExchange(res, out), nil
	}
}

type dnsResult struct {
	r   *dns.Msg
	rtt time.Duration
	err error
}

// fromExchange translates a miekg/dns outcome into a measurement.
func (c *DNS) fromExchange(res measurement.Measurement, o dnsResult) measurement.Measurement {
	if o.err != nil {
		res.ErrorCode = classifyDNSError(o.err)
		return res
	}
	if o.r == nil {
		res.ErrorCode = measurement.CodeDNSError
		return res
	}
	rcode := o.r.Rcode
	res.Metadata = map[string]any{
		measurement.MetaDNSRcode: rcode,
		measurement.MetaQueryName: c.opts.ProbeName,
	}
	if o.r.Rcode != dns.RcodeSuccess {
		res.ErrorCode = measurement.CodeDNSError
		return res
	}
	if len(o.r.Answer) > 0 {
		if a, ok := o.r.Answer[0].(*dns.A); ok {
			res.Metadata[measurement.MetaAnswer] = a.A.String()
		}
	}
	lat := roundMS(o.rtt)
	res.Success = true
	res.Latency = &lat
	return res
}

// classifyDNSError maps a dns exchange error to a measurement code. Timeouts
// and context deadlines are CodeTimeout; anything else (network unreachable,
// malformed packet, server failure seen as an error) is CodeDNSError — a
// non-success rcode is handled separately in fromExchange.
func classifyDNSError(err error) string {
	var ne interface{ Timeout() bool }
	if errors.As(err, &ne) && ne.Timeout() {
		return measurement.CodeTimeout
	}
	s := err.Error()
	if contains(s, "no such host") || contains(s, "unreachable") {
		return measurement.CodeUnreachable
	}
	return measurement.CodeDNSError
}
