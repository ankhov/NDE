// Package collector — traceroute collector (Phase 3).
//
// It discovers the IP path to the target by shelling out to the OS traceroute
// binary: `traceroute` (unix) / `tracert` (Windows). As with the ICMP
// collector, delegating to the OS binary avoids raw-socket privilege issues
// (traceroute on Linux uses cap_net_raw; tracert uses IcmpSendEcho with
// decreasing TTL). The collector resolves through ResolveGuarded first and
// traces the screened IP literal, so SSRF holds at request time.
//
// The Measurement carries the end-to-end RTT (the latency to the destination
// hop, when it answered) as Latency, and the full hop list in Metadata so the
// UI can render the path. Hops that did not respond ("* * *" / "Request timed
// out") are recorded with a nil RTT and do not fail the check; the check is an
// error only when the destination never answered.
//
// The process runner is injectable (CommandRunner, shared with icmp.go) so
// tests parse canned traceroute output without spawning a process.
package collector

import (
	"context"
	"errors"
	"log/slog"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/ankhov/NDE/internal/measurement"
	"github.com/ankhov/NDE/internal/target"
)

// TracerouteOptions configures the traceroute collector.
type TracerouteOptions struct {
	// AllowPrivate permits tracing loopback/private/link-local addresses.
	AllowPrivate bool
	// MaxHops caps the number of hops the OS tool probes (default 30).
	MaxHops int
}

// Traceroute discovers the IP path to a target via the OS traceroute binary.
type Traceroute struct {
	opts TracerouteOptions
	run  CommandRunner
	log  *slog.Logger
}

// NewTraceroute returns a Traceroute collector with the default exec runner.
func NewTraceroute(opts TracerouteOptions) *Traceroute {
	if opts.MaxHops <= 0 {
		opts.MaxHops = 30
	}
	return &Traceroute{opts: opts, run: defaultRunner, log: slog.Default()}
}

// WithRunner replaces the OS command runner (for tests).
func (c *Traceroute) WithRunner(r CommandRunner) *Traceroute { c.run = r; return c }

// WithLogger sets the collector logger.
func (c *Traceroute) WithLogger(l *slog.Logger) *Traceroute { c.log = l; return c }

// Name returns the collector identifier.
func (c *Traceroute) Name() string { return "traceroute" }

// Supports reports whether the collector handles the given type.
func (c *Traceroute) Supports(t measurement.Type) bool { return t == measurement.TypeTraceroute }

// Collect runs one traceroute.
func (c *Traceroute) Collect(ctx context.Context, t measurement.Target) (measurement.Measurement, error) {
	res := measurement.Measurement{
		TargetID:  t.ID,
		Timestamp: time.Now().UTC(),
		Type:      measurement.TypeTraceroute,
	}
	if t.Host == "" {
		res.ErrorCode = measurement.CodeInvalidConfig
		return res, nil
	}
	deadline := t.Timeout
	if deadline <= 0 {
		deadline = 10 * time.Second
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

	name, args := tracerouteArgs(ip.String(), c.opts.MaxHops, deadline)
	// Traceroute can take a while; allow the tool its own time but cap with the
	// context so a hung target cannot block the scheduler indefinitely.
	runCtx, cancel := context.WithTimeout(ctx, deadline+2*time.Second)
	defer cancel()
	out, runErr := c.run(runCtx, name, args...)

	hops, reached := parseTraceroute(out)
	res.Metadata = map[string]any{
		measurement.MetaResolvedIP: ip.String(),
		measurement.MetaHopCount:   len(hops),
		measurement.MetaHops:       hops,
		measurement.MetaReachedDest: reached,
	}
	if len(hops) > 0 {
		res.Metadata[measurement.MetaFinalHop] = hops[len(hops)-1]["ip"]
	}

	switch {
	case reached:
		// Destination answered: set Latency to the destination hop's RTT.
		if l, ok := hopLatency(hops[len(hops)-1]); ok {
			res.Success = true
			res.Latency = &l
		} else {
			// Reached but the last hop had no parseable RTT — still a success
			// with insufficient latency data.
			res.Success = true
		}
	case isTracerouteTimeout(out, runErr):
		res.ErrorCode = measurement.CodeTimeout
	case isTracerouteUnreachable(out):
		res.ErrorCode = measurement.CodeUnreachable
	default:
		res.ErrorCode = measurement.CodeUnreachable
		if runErr != nil {
			c.log.Warn("traceroute failed", "target", t.ID, "host", ip, "err", runErr, "out", string(out))
		}
	}
	return res, nil
}

// tracerouteArgs builds the OS-specific traceroute command.
// Windows:  tracert -d -h <maxHops> -w <ms> <host>
// Unix:     traceroute -n -q 1 -w <sec> -m <maxHops> <host>
func tracerouteArgs(host string, maxHops int, timeout time.Duration) (string, []string) {
	if maxHops <= 0 {
		maxHops = 30
	}
	if runtime.GOOS == "windows" {
		ms := int(timeout / time.Millisecond)
		if ms <= 0 {
			ms = 1000
		}
		return "tracert", []string{"-d", "-h", strconv.Itoa(maxHops), "-w", strconv.Itoa(ms), host}
	}
	secs := int(timeout / time.Second)
	if secs <= 0 {
		secs = 2
	}
	return "traceroute", []string{"-n", "-q", "1", "-w", strconv.Itoa(secs), "-m", strconv.Itoa(maxHops), host}
}

// hop is a parsed traceroute hop: {hop:int, ip:string, rtt_ms:*float64}.
type hop = map[string]any

// unixHopRe matches a numeric traceroute hop line:
//   " 1  192.168.1.1  0.042 ms"  /  " 2  *  *  *"
// Group 1 = hop number, group 2 = ip (or "*"), group 3 = first RTT in ms.
var unixHopRe = regexp.MustCompile(`^\s*(\d+)\s+(\S+)(?:\s+([0-9]+(?:\.[0-9]+)?)\s*ms)?`)

// winHopRe matches a Windows tracert hop line:
//   "  1     1 ms     1 ms     1 ms  192.168.1.1"
//   "  2     *        *        *     Request timed out."
// Group 1 = hop number, group 2 = first RTT (digits) or "*".
var winHopRe = regexp.MustCompile(`^\s*(\d+)\s+([0-9]+|<\d+|\*)\s*ms`)

// parseTraceroute walks the output line by line and returns the hop list and
// whether the destination was reached. A hop whose IP equals the traced IP and
// has a latency value marks the destination as reached.
func parseTraceroute(out []byte) (hops []hop, reached bool) {
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if h, ok := parseUnixHop(line); ok {
			hops = append(hops, h)
			continue
		}
		if h, ok := parseWindowsHop(line); ok {
			hops = append(hops, h)
			continue
		}
	}
	// The destination is reached when the LAST hop's IP is a real address (not
	// "*") and it is not a timeout marker. Some tools echo the target host in
	// the first header line; we rely on the hop lines alone.
	if len(hops) > 0 {
		last := hops[len(hops)-1]
		if ip, _ := last["ip"].(string); ip != "" && ip != "*" && !isTimeoutHop(last) {
			reached = true
		}
	}
	return hops, reached
}

// parseUnixHop parses one unix traceroute hop line. The hop's address token is
// an IP literal (contains "." for IPv4 / ":" for IPv6) or "*"; a bare integer
// means this is actually a Windows tracert RTT column, so the line is rejected
// here and left for parseWindowsHop.
func parseUnixHop(line string) (hop, bool) {
	m := unixHopRe.FindStringSubmatch(line)
	if m == nil {
		return nil, false
	}
	addr := m[2]
	if addr != "*" && !contains(addr, ".") && !contains(addr, ":") {
		return nil, false // bare number — Windows RTT column, not a unix hop
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return nil, false
	}
	h := hop{"hop": n, "ip": addr}
	if m[3] != "" {
		if v, err := strconv.ParseFloat(m[3], 64); err == nil {
			h["rtt_ms"] = v
		}
	}
	return h, true
}

// parseWindowsHop parses one Windows tracert hop line.
func parseWindowsHop(line string) (hop, bool) {
	m := winHopRe.FindStringSubmatch(line)
	if m == nil {
		return nil, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return nil, false
	}
	h := hop{"hop": n}
	rtt := m[2]
	if rtt == "*" {
		h["ip"] = "*"
		return h, true
	}
	// RTT present; the IP is the last token on the line that is not "ms".
	h["rtt_ms"] = parseWinRTT(rtt)
	// Extract the trailing IP (the token after the three "ms" columns).
	h["ip"] = extractWindowsHopIP(line)
	return h, true
}

// parseWinRTT turns "<1" / "12" into a float64 ms value.
func parseWinRTT(s string) float64 {
	s = strings.TrimPrefix(s, "<")
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		return v
	}
	return 0
}

// extractWindowsHopIP returns the hop IP from a tracert line (the token after
// the three RTT columns), or "*"/"" when the hop timed out.
func extractWindowsHopIP(line string) string {
	// Drop everything up to and including the third "ms".
	idx := 0
	for i := 0; i < 3; i++ {
		j := strings.Index(line[idx:], "ms")
		if j < 0 {
			return ""
		}
		idx += j + 2
	}
	rest := strings.TrimSpace(line[idx:])
	if rest == "" {
		return ""
	}
	if contains(rest, "timed out") {
		return "*"
	}
	// The IP is the first whitespace-delimited token.
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// isTimeoutHop reports whether a hop represents a non-responding hop ("*").
func isTimeoutHop(h hop) bool {
	ip, _ := h["ip"].(string)
	return ip == "*" || ip == ""
}

// hopLatency returns the RTT of a hop and ok=false when it has no value.
func hopLatency(h hop) (float64, bool) {
	v, ok := h["rtt_ms"]
	if !ok {
		return 0, false
	}
	f, ok := v.(float64)
	if !ok {
		return 0, false
	}
	return f, true
}

// isTracerouteTimeout reports whether the output indicates the probe timed out
// before completing (Windows "Request timed out" on every hop, or the process
// was killed by the context deadline).
func isTracerouteTimeout(out []byte, runErr error) bool {
	s := string(out)
	if contains(s, "Request timed out") && !contains(s, "ms") {
		return true
	}
	if runErr != nil && (contains(runErr.Error(), "signal: killed") || contains(runErr.Error(), "deadline exceeded")) {
		return true
	}
	return false
}

// isTracerouteUnreachable reports whether the output indicates a network-layer
// unreachable condition (no route, unknown host).
func isTracerouteUnreachable(out []byte) bool {
	s := string(out)
	return contains(s, "Network is unreachable") ||
		contains(s, "Name or service not known") ||
		contains(s, "unknown host") ||
		contains(s, "Temporary failure in name resolution") ||
		contains(s, "Destination Host Unreachable")
}
