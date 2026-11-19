// Package bindings — DTOs that cross the Wails boundary.
//
// The domain models (measurement.Target / Measurement) use time.Duration for
// intervals and would serialize as nanosecond int64s; the UI wants
// milliseconds and camelCase JSON. These DTOs are thin transport shapes —
// no behaviour, no duplicated business logic (SPEC rule 8) — with explicit
// conversions to/from the domain models in this file.
package bindings

import (
	"time"

	"github.com/ankhov/NDE/internal/measurement"
)

// TargetDTO is the UI shape for a monitoring target.
type TargetDTO struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Host              string `json:"host"`
	Protocol          string `json:"protocol"`
	Port              int    `json:"port"`
	CheckIntervalMS   int64  `json:"checkInterval"`
	TimeoutMS         int64  `json:"timeout"`
	Enabled           bool   `json:"enabled"`
	WarningThreshold  float64 `json:"warningThreshold"`
	CriticalThreshold float64 `json:"criticalThreshold"`
	Group             string `json:"group"`
}

// TargetInputDTO is what the UI sends to create a target. ID is generated
// server-side.
type TargetInputDTO struct {
	Name              string `json:"name"`
	Host              string `json:"host"`
	Protocol          string `json:"protocol"`
	Port              int    `json:"port"`
	CheckIntervalMS   int64  `json:"checkInterval"`
	TimeoutMS         int64  `json:"timeout"`
	Enabled           bool   `json:"enabled"`
	WarningThreshold  float64 `json:"warningThreshold"`
	CriticalThreshold float64 `json:"criticalThreshold"`
	Group             string `json:"group"`
}

// MeasurementDTO is the UI shape for a single measurement.
type MeasurementDTO struct {
	TargetID  string         `json:"targetId"`
	Timestamp string         `json:"timestamp"` // RFC3339 UTC
	Type      string         `json:"type"`
	Latency   *float64       `json:"latency"` // null = no value
	Success   bool           `json:"success"`
	ErrorCode string         `json:"errorCode"`
	Metadata  map[string]any `json:"metadata"`
}

// StatsDTO is the dashboard summary, computed from recent measurements.
type StatsDTO struct {
	AvgRTT        *float64 `json:"avgRtt"`        // ms, null if no data
	P95RTT        *float64 `json:"p95Rtt"`        // ms, null if no data
	Jitter        *float64 `json:"jitter"`       // ms, mean abs. delta, null if <2 samples
	PacketLoss    float64  `json:"packetLoss"`    // percent (0..100)
	SampleCount   int      `json:"sampleCount"`
	ActiveTargets int      `json:"activeTargets"`
}

// toTargetDTO converts a domain Target to its transport shape.
func toTargetDTO(t measurement.Target) TargetDTO {
	return TargetDTO{
		ID: t.ID, Name: t.Name, Host: t.Host,
		Protocol: string(t.Protocol), Port: t.Port,
		CheckIntervalMS:   t.CheckInterval.Milliseconds(),
		TimeoutMS:         t.Timeout.Milliseconds(),
		Enabled:           t.Enabled,
		WarningThreshold:  t.WarningThreshold,
		CriticalThreshold: t.CriticalThreshold,
		Group:             string(t.Group),
	}
}

// toTarget converts a TargetInputDTO to a domain Target with the given id.
func toTarget(id string, in TargetInputDTO) measurement.Target {
	return measurement.Target{
		ID: id, Name: in.Name, Host: in.Host,
		Protocol: measurement.Type(in.Protocol), Port: in.Port,
		CheckInterval: time.Duration(in.CheckIntervalMS) * time.Millisecond,
		Timeout:       time.Duration(in.TimeoutMS) * time.Millisecond,
		Enabled:       in.Enabled,
		WarningThreshold:  in.WarningThreshold,
		CriticalThreshold: in.CriticalThreshold,
		Group:          measurement.TargetGroup(in.Group),
	}
}

// toMeasurementDTO converts a domain Measurement to its transport shape.
func toMeasurementDTO(m measurement.Measurement) MeasurementDTO {
	return MeasurementDTO{
		TargetID:  m.TargetID,
		Timestamp: m.Timestamp.Format(time.RFC3339Nano),
		Type:      string(m.Type),
		Latency:   m.Latency,
		Success:   m.Success,
		ErrorCode: m.ErrorCode,
		Metadata:  m.Metadata,
	}
}
