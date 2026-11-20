// Package collector — ICMP echo collector (Phase 3).
//
// It measures round-trip time of a single ICMP echo to the target by shelling
// out to the operating-system ping binary: `ping` (unix) / `ping.exe`
// (Windows). Delegating to the OS binary keeps the collector unprivileged and
// cross-platform without a syscall wrapper: on Windows ping.exe calls
// IcmpSendEcho internally (ADR-002), on Linux/macOS ping uses cap_net_raw or
// the UDP-ICMP trick. The collector still resolves through ResolveGuarded and
// pings the screened IP literal, so the SSRF screen holds at request time.
//
// The process runner is injectable (CommandRunner) so unit tests parse canned
// ping output without spawning a process or needing ICMP privileges.
package collector

import (
	"context"
	"errors"
	"log/slog"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"time"

	"github.com/ankhov/NDE/internal/measurement"
	"github.com/ankhov/NDE/internal/target"
)

// CommandRunner runs an OS command and returns its combined stdout+stderr. It
// is the seam collectors shell out through, so tests can inject canned output.
// A non-nil error means the command failed to start or returned a non-zero
// exit (the output may still contain useful diagnostic text).
type CommandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

// defaultRunner runs the command via exec.CommandContext and returns combined
// output. A non-zero exit is reported as an error carrying the output.
func defaultRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, err
	}
	return out, nil
}

// ICMPOptions configures the ICMP collector.
type ICMPOptions struct {
	// AllowPrivate permits pinging loopback/private/link-local addresses.
	// Default false: user-supplied targets must not reach internal hosts.
	AllowPrivate bool
	// Count is the number of echo requests (default 1). Extra samples are
	// averaged; a single sample is the common case for a periodic check.
	Count int
}

// ICMP measures ICMP echo RTT via the OS ping binary.
type ICMP struct {
	opts ICMPOptions
	run  CommandRunner
	log  *slog.Logger
}

// NewICMP returns an ICMP collector with the default exec runner.
func NewICMP(opts ICMPOptions) *ICMP {
	if opts.Count <= 0 {
		opts.Count = 1
	}
	return &ICMP{opts: opts, run: defaultRunner, log: slog.Default()}
}

// WithRunner replaces the OS command runner (for tests).
func (c *ICMP) WithRunner(r CommandRunner) *ICMP { c.run = r; return c }

// WithLogger sets the collector logger.
func (c *ICMP) WithLogger(l *slog.Logger) *ICMP { c.log = l; return c }

// Name returns the collector identifier.
func (c *ICMP) Name() string { return "icmp" }

// Supports reports whether the collector handles the given type.
func (c *ICMP) Supports(t measurement.Type) bool { return t == measurement.TypeICMP }

// Collect runs one ICMP echo check.
func (c *ICMP) Collect(ctx context.Context, t measurement.Target) (measurement.Measurement, error) {
	res := measurement.Measurement{
		TargetID:  t.ID,
		Timestamp: time.Now().UTC(),
		Type:      measurement.TypeICMP,
	}
	if t.Host == "" {
		res.ErrorCode = measurement.CodeInvalidConfig
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

	name, args := pingArgs(ip.String(), c.opts.Count, deadline)
	runCtx, cancel := context.WithTimeout(ctx, deadline+500*time.Millisecond)
	defer cancel()
	start := time.Now()
	out, runErr := c.run(runCtx, name, args...)
	elapsed := time.Since(start)

	res.Metadata = map[string]any{measurement.MetaResolvedIP: ip.String()}

	lat, ok := parsePingRTT(out)
	switch {
	case ok:
		// Got a reply with a latency value.
		res.Success = true
		res.Latency = &lat
	case isPingTimeout(out, runErr):
		res.ErrorCode = measurement.CodeTimeout
		res.Metadata[measurement.MetaFailureMS] = roundMS(elapsed)
	case isPingUnreachable(out):
		res.ErrorCode = measurement.CodeUnreachable
		res.Metadata[measurement.MetaFailureMS] = roundMS(elapsed)
	default:
		// No reply and no recognised failure marker — treat as unreachable.
		res.ErrorCode = measurement.CodeUnreachable
		res.Metadata[measurement.MetaFailureMS] = roundMS(elapsed)
		if runErr != nil {
			c.log.Warn("icmp ping failed", "target", t.ID, "host", ip, "err", runErr, "out", string(out))
		}
	}
	return res, nil
}

// pingArgs builds the OS-specific ping command for a single (or counted) echo.
// Windows:  ping -n <count> -w <ms> <host>
// Unix:     ping -c <count> -W <sec> <host>   (Linux -W is seconds)
func pingArgs(host string, count int, timeout time.Duration) (string, []string) {
	if count <= 0 {
		count = 1
	}
	if runtime.GOOS == "windows" {
		ms := int(timeout / time.Millisecond)
		if ms <= 0 {
			ms = 1000
		}
		return "ping", []string{"-n", strconv.Itoa(count), "-w", strconv.Itoa(ms), host}
	}
	secs := int(timeout / time.Second)
	if secs <= 0 {
		secs = 1
	}
	return "ping", []string{"-c", strconv.Itoa(count), "-W", strconv.Itoa(secs), host}
}

// rttRe matches the per-packet latency line in ping output:
//   unix:   "time=0.042 ms"  /  "time=42.0 ms"
//   windows:"time<1ms"       /  "time=12ms"
// The captured group is the numeric latency. "<" is accepted as a separator
// so sub-millisecond Windows replies ("time<1ms") still yield a value.
var rttRe = regexp.MustCompile(`time[=<]\s*([0-9]+(?:\.[0-9]+)?)\s*ms`)

// parsePingRTT extracts the first latency value from ping output. ok=false
// when no "time=...ms" / "time<...ms" marker is present (no reply received).
func parsePingRTT(out []byte) (float64, bool) {
	m := rttRe.FindSubmatch(out)
	if m == nil {
		return 0, false
	}
	v, err := strconv.ParseFloat(string(m[1]), 64)
	if err != nil {
		return 0, false
	}
	// Windows "time<1ms" means "less than 1 ms" — report the bound as-is so
	// the UI shows a real (sub-millisecond) value rather than a sentinel.
	return v, true
}

// isPingTimeout reports whether the output indicates a request timeout with no
// reply (Windows "Request timed out." / unix "0% packets received", "100%
// packet loss") — a transient condition worth a retry.
func isPingTimeout(out []byte, runErr error) bool {
	s := string(out)
	if contains(s, "timed out") || contains(s, "Request timed out") {
		return true
	}
	if contains(s, "100% packet loss") || contains(s, "0% packets received") || contains(s, "0 received") {
		return true
	}
	// Exit code 1 from iputils ping = "no reply" (distinct from exit 2 = error).
	if runErr != nil && contains(runErr.Error(), "exit status 1") {
		return true
	}
	return false
}

// isPingUnreachable reports whether the output indicates the host is
// unreachable at the network layer (no route, unknown host, DNS failure) — a
// more persistent condition than a timeout.
func isPingUnreachable(out []byte) bool {
	s := string(out)
	return contains(s, "Destination Host Unreachable") ||
		contains(s, "Destination Net Unreachable") ||
		contains(s, "Name or service not known") ||
		contains(s, "unknown host") ||
		contains(s, "could not find host") ||
		contains(s, "Temporary failure in name resolution")
}
