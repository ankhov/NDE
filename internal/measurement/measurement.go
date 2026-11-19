// Package measurement defines the unified measurement and target models used
// across collectors, storage, analytics and diagnostics (SPEC §4.5, §6, §7).
//
// These types are pure data: they carry no behaviour that depends on network
// IO or storage. Collectors, the scheduler, metrics, analytics and diagnostics
// all consume the same models, so business logic is not duplicated.
package measurement

import "time"

// Type identifies the kind of network check that produced a measurement.
type Type string

// Supported measurement types (SPEC §3.3, §4.1, §4.4, §4.6).
const (
	TypeICMP       Type = "icmp"
	TypeTCP        Type = "tcp"
	TypeDNS        Type = "dns"
	TypeHTTP       Type = "http"
	TypeTraceroute Type = "traceroute"
)

// AllTypes lists every measurement type in canonical order.
var AllTypes = []Type{TypeICMP, TypeTCP, TypeDNS, TypeHTTP, TypeTraceroute}

// Valid reports whether t is a known measurement type.
func (t Type) Valid() bool {
	for _, x := range AllTypes {
		if t == x {
			return true
		}
	}
	return false
}

// Measurement is the unified result of a single network check (SPEC §7).
//
// Latency is *float64 (milliseconds): a nil pointer means "no value" —
// timeout, insufficient data, or a non-latency result — never a sentinel
// number such as 0 or -1. This makes the insufficient_data state explicit
// (SPEC §4.2).
type Measurement struct {
	TargetID  string
	Timestamp time.Time
	Type      Type
	Latency   *float64
	Success   bool
	ErrorCode string
	Metadata  map[string]any
}

// LatencyMS returns the latency in milliseconds and ok=false when it is
// absent (nil). Callers must handle ok=false rather than treating 0 as a
// real measurement.
func (m Measurement) LatencyMS() (value float64, ok bool) {
	if m.Latency == nil {
		return 0, false
	}
	return *m.Latency, true
}

// WithLatency returns a copy of m with the latency pointer set to v.
func (m Measurement) WithLatency(v float64) Measurement {
	cp := m
	cp.Latency = &v
	return cp
}

// TargetGroup categorises targets for grouping in the UI (SPEC §4.5).
type TargetGroup string

const (
	GroupLocal   TargetGroup = "local"
	GroupExternal TargetGroup = "external"
	GroupDNS      TargetGroup = "dns"
	GroupCustom   TargetGroup = "custom"
)

// Valid reports whether g is a known target group.
func (g TargetGroup) Valid() bool {
	switch g {
	case GroupLocal, GroupExternal, GroupDNS, GroupCustom:
		return true
	}
	return false
}

// Target is a monitored endpoint (SPEC §4.5).
type Target struct {
	ID                string
	Name              string
	Host              string
	Protocol          Type
	Port              int
	CheckInterval     time.Duration
	Timeout           time.Duration
	Enabled           bool
	WarningThreshold  float64
	CriticalThreshold float64
	Group             TargetGroup
}

// QueryFilter constrains a measurement query (storage.MeasurementReader).
// All time fields are UTC.
type QueryFilter struct {
	TargetIDs []string
	Types     []Type
	From      time.Time
	To        time.Time
	Limit     int
}
