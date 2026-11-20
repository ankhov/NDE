// Package bindings — CollectorService reports which measurement types have a
// live collector registered, so the UI can show "ICMP: ready" / "ready to
// collect" per type rather than discovering support only when a tick fails.
//
// It is read-only and never drives collection; the scheduler still resolves a
// collector per tick through the registry. The service reads the same Registry
// the scheduler uses, so the two never disagree.
package bindings

import (
	"context"

	"github.com/ankhov/NDE/internal/collector"
	"github.com/ankhov/NDE/internal/measurement"
)

// CollectorRegistry is the subset of collector.Registry the service needs.
type CollectorRegistry interface {
	Get(t measurement.Type) (collector.Collector, bool)
	All() []collector.Collector
}

// CollectorService exposes collector availability to the UI.
type CollectorService struct {
	reg CollectorRegistry
}

// NewCollectorService returns a CollectorService backed by reg.
func NewCollectorService(reg CollectorRegistry) *CollectorService {
	return &CollectorService{reg: reg}
}

// Compile-time check that collector.Registry satisfies CollectorRegistry.
var _ CollectorRegistry = (collector.Registry)(nil)

// CollectorStatusDTO is the UI shape for one measurement type's collector
// availability.
type CollectorStatusDTO struct {
	// Type is the measurement type name (icmp/tcp/dns/http/traceroute).
	Type string `json:"type"`
	// Ready reports whether a collector is registered for this type.
	Ready bool `json:"ready"`
	// CollectorName is the registered collector's Name() when ready, else "".
	CollectorName string `json:"collectorName"`
}

// Status returns the availability of every canonical measurement type, in
// canonical order. A type with no registered collector is reported as
// Ready=false so the UI can flag it as "unsupported".
func (s *CollectorService) Status(_ context.Context) ([]CollectorStatusDTO, error) {
	out := make([]CollectorStatusDTO, 0, len(measurement.AllTypes))
	for _, t := range measurement.AllTypes {
		dto := CollectorStatusDTO{Type: string(t)}
		if c, ok := s.reg.Get(t); ok {
			dto.Ready = true
			dto.CollectorName = c.Name()
		}
		out = append(out, dto)
	}
	return out, nil
}

// ReadyTypes returns the list of measurement type names that have a registered
// collector. Convenient for the UI to gate target creation options.
func (s *CollectorService) ReadyTypes(_ context.Context) ([]string, error) {
	out := make([]string, 0, len(measurement.AllTypes))
	for _, t := range measurement.AllTypes {
		if _, ok := s.reg.Get(t); ok {
			out = append(out, string(t))
		}
	}
	return out, nil
}
