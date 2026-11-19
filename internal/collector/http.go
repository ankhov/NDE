// Package collector — HTTP collector (SPEC §3.3, §4.4).
//
// It issues a single GET against target.Host (a full URL, e.g.
// "https://example.com") and measures the round-trip to the first response
// byte. Latency is recorded for any completed response (2xx or otherwise);
// a non-2xx status is a target-side failure (Success=false, CodeHTTPStatus)
// with the status code in metadata, not a collector error. Transport
// failures (timeout, TLS, reset) map to CodeTimeout / CodeHTTPError.
//
// SSRF: the URL host is resolved and screened before the request. For Phase 2
// the transport is then allowed to re-resolve normally; the screen still
// blocks a private hostname because the same hostname would resolve to the
// same screened address. IP-pinned dialing (closing the tiny TOCTOU window)
// is tracked as a Phase 3 hardening (ADR TBD).
package collector

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ankhov/NDE/internal/measurement"
	"github.com/ankhov/NDE/internal/target"
)

// HTTPOptions configures the HTTP collector.
type HTTPOptions struct {
	// AllowPrivate permits requesting private/loopback URLs.
	AllowPrivate bool
	// UserAgent sent with the probe. Default "NDE/0.1 (+network diagnostics)".
	UserAgent string
}

// HTTP measures HTTP GET RTT.
type HTTP struct {
	opts HTTPOptions
	log  *slog.Logger
}

// NewHTTP returns an HTTP collector.
func NewHTTP(opts HTTPOptions) *HTTP {
	if opts.UserAgent == "" {
		opts.UserAgent = "NDE/0.1 (+network diagnostics)"
	}
	return &HTTP{opts: opts, log: slog.Default()}
}

// WithLogger sets the collector logger.
func (c *HTTP) WithLogger(l *slog.Logger) *HTTP { c.log = l; return c }

// Name returns the collector identifier.
func (c *HTTP) Name() string { return "http" }

// Supports reports whether the collector handles the given type.
func (c *HTTP) Supports(t measurement.Type) bool { return t == measurement.TypeHTTP }

// Collect runs one HTTP GET.
func (c *HTTP) Collect(ctx context.Context, t measurement.Target) (measurement.Measurement, error) {
	res := measurement.Measurement{
		TargetID:  t.ID,
		Timestamp: time.Now().UTC(),
		Type:      measurement.TypeHTTP,
	}
	raw := strings.TrimSpace(t.Host)
	if raw == "" {
		res.ErrorCode = measurement.CodeInvalidConfig
		return res, nil
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		res.ErrorCode = measurement.CodeInvalidConfig
		return res, nil
	}

	// Resolve and screen the URL host before any request. A private hostname
	// (or a hostname that resolves private) is blocked here.
	if _, err := target.ResolveGuarded(ctx, u.Hostname(), target.SSRFOptions{AllowPrivate: c.opts.AllowPrivate}); err != nil {
		if errors.Is(err, target.ErrForbiddenHost) {
			res.ErrorCode = measurement.CodeSSRFBlocked
			res.Metadata = map[string]any{measurement.MetaResolvedIP: u.Hostname()}
			return res, nil
		}
		res.ErrorCode = measurement.CodeUnreachable
		return res, nil
	}

	deadline := t.Timeout
	if deadline <= 0 {
		deadline = 5 * time.Second
	}
	reqCtx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, raw, nil)
	if err != nil {
		res.ErrorCode = measurement.CodeInvalidConfig
		return res, nil
	}
	req.Header.Set("User-Agent", c.opts.UserAgent)
	req.Header.Set("Accept", "*/*")

	client := &http.Client{
		Timeout: deadline,
		// Do not follow redirects: a redirect to a private internal host would
		// bypass the screen. Diagnostics can still see the 3xx via status_code.
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	start := time.Now()
	resp, err := client.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		res.ErrorCode = classifyHTTPError(err)
		res.Metadata = map[string]any{measurement.MetaFailureMS: roundMS(elapsed)}
		return res, nil
	}
	defer resp.Body.Close()
	// Drain a small amount so the connection can be reused; bound it so a
	// malicious huge body cannot stall the collector.
	_, _ = io.CopyN(io.Discard, resp.Body, 4096)

	res.Metadata = map[string]any{
		measurement.MetaStatusCode: resp.StatusCode,
	}
	lat := roundMS(elapsed)
	res.Latency = &lat
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		res.Success = true
	} else {
		res.ErrorCode = measurement.CodeHTTPStatus
	}
	return res, nil
}

// classifyHTTPError maps a transport error to a measurement code. Context
// deadline / timeout → CodeTimeout; TLS and connection-reset style failures →
// CodeHTTPError; connection-refused → CodeRefused (the host is up).
func classifyHTTPError(err error) string {
	var ne interface{ Timeout() bool }
	if errors.As(err, &ne) && ne.Timeout() {
		return measurement.CodeTimeout
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return measurement.CodeTimeout
	}
	s := err.Error()
	if contains(s, "refused") {
		return measurement.CodeRefused
	}
	if contains(s, "no such host") || contains(s, "unreachable") {
		return measurement.CodeUnreachable
	}
	return measurement.CodeHTTPError
}
