// Package collector defines the Collector contract (SPEC §3.3, §7) and an
// in-memory Registry.
//
// Collectors perform a single network check and return a unified
// measurement.Measurement. They MUST NOT contain diagnostic or analytics
// logic (SPEC §3.3, rule 9). All Collect calls accept a context.Context and
// must respect its deadline (SPEC rule 10).
package collector

import (
	"context"
	"sync"

	"github.com/ankhov/NDE/internal/measurement"
)

// Collector performs a single network check of a given type.
type Collector interface {
	// Name is a short, unique identifier for logging and metrics.
	Name() string
	// Supports reports whether this collector can handle the given type.
	Supports(t measurement.Type) bool
	// Collect runs one check against target and returns a unified measurement.
	// A non-nil error means the collector itself failed (not that the network
	// target is down); target-side failures are encoded in the Measurement
	// (Success=false, ErrorCode set).
	Collect(ctx context.Context, target measurement.Target) (measurement.Measurement, error)
}

// Registry maps measurement types to collectors.
type Registry interface {
	Register(c Collector)
	Get(t measurement.Type) (Collector, bool)
	All() []Collector
}

// NewRegistry returns a concurrency-safe in-memory Registry.
func NewRegistry() Registry {
	return &registry{}
}

type registry struct {
	mu  sync.RWMutex
	all []Collector
}

func (r *registry) Register(c Collector) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.all = append(r.all, c)
}

func (r *registry) Get(t measurement.Type) (Collector, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, c := range r.all {
		if c.Supports(t) {
			return c, true
		}
	}
	return nil, false
}

func (r *registry) All() []Collector {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cp := make([]Collector, len(r.all))
	copy(cp, r.all)
	return cp
}
