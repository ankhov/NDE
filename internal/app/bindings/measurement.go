// Package bindings — MeasurementService exposes measurement reads and the
// dashboard summary to the UI. It computes Avg/P95/Jitter/Packet-loss from
// raw measurements with plain Go (no SQL aggregation yet — the
// metric_aggregates table is populated in Phase 4 by the metrics pipeline).
package bindings

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"time"

	pkgerrors "github.com/ankhov/NDE/internal/errors"
	"github.com/ankhov/NDE/internal/measurement"
	"github.com/ankhov/NDE/internal/storage"
)

// MeasurementService reads measurements and computes summaries.
type MeasurementService struct {
	reader storage.MeasurementReader
	targets storage.TargetStore
	log    *slog.Logger
}

// NewMeasurementService returns a MeasurementService.
func NewMeasurementService(reader storage.MeasurementReader, targets storage.TargetStore, log *slog.Logger) *MeasurementService {
	if log == nil {
		log = slog.Default()
	}
	return &MeasurementService{reader: reader, targets: targets, log: log}
}

// Recent returns the latest measurements across all targets, newest first.
func (s *MeasurementService) Recent(ctx context.Context, limit int) ([]MeasurementDTO, error) {
	if limit <= 0 || limit > 1000 {
		limit = 50
	}
	ms, err := s.reader.QueryMeasurements(ctx, measurement.QueryFilter{Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("%w: query measurements: %v", pkgerrors.ErrStorage, err)
	}
	return toMeasurementDTOs(ms), nil
}

// ByTarget returns the latest measurements for one target, newest first.
func (s *MeasurementService) ByTarget(ctx context.Context, targetID string, limit int) ([]MeasurementDTO, error) {
	if targetID == "" {
		return nil, fmt.Errorf("%w: targetId is required", pkgerrors.ErrInvalidInput)
	}
	if limit <= 0 || limit > 1000 {
		limit = 50
	}
	ms, err := s.reader.QueryMeasurements(ctx, measurement.QueryFilter{
		TargetIDs: []string{targetID}, Limit: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: query measurements: %v", pkgerrors.ErrStorage, err)
	}
	return toMeasurementDTOs(ms), nil
}

// Timeline returns measurements for one target oldest-first so a chart renders
// left-to-right. Default window: the last 100 samples.
func (s *MeasurementService) Timeline(ctx context.Context, targetID string, limit int) ([]MeasurementDTO, error) {
	ms, err := s.ByTarget(ctx, targetID, limit)
	if err != nil {
		return nil, err
	}
	// ByTarget returns newest-first; reverse for chronological order.
	for i, j := 0, len(ms)-1; i < j; i, j = i+1, j-1 {
		ms[i], ms[j] = ms[j], ms[i]
	}
	return ms, nil
}

// Stats computes the dashboard summary over the last `window` (default 5 min).
// Latency statistics use only measurements that produced a latency value
// (Success with non-nil latency); packet loss counts every failed check.
func (s *MeasurementService) Stats(ctx context.Context) (StatsDTO, error) {
	const window = 5 * time.Minute
	ms, err := s.reader.QueryMeasurements(ctx, measurement.QueryFilter{
		From:  time.Now().UTC().Add(-window),
		Limit: 1000,
	})
	if err != nil {
		return StatsDTO{}, fmt.Errorf("%w: query measurements: %v", pkgerrors.ErrStorage, err)
	}

	stats := StatsDTO{SampleCount: len(ms)}
	if len(ms) == 0 {
		stats.ActiveTargets = s.countActiveTargets(ctx)
		return stats, nil
	}

	lats := make([]float64, 0, len(ms))
	failures := 0
	for _, m := range ms {
		if !m.Success {
			failures++
		}
		if m.Latency != nil {
			lats = append(lats, *m.Latency)
		}
	}
	stats.PacketLoss = percent(failures, len(ms))

	if len(lats) > 0 {
		avg := mean(lats)
		stats.AvgRTT = &avg
		p95 := percentile(lats, 0.95)
		stats.P95RTT = &p95
	}
	if len(lats) >= 2 {
		j := jitter(lats)
		stats.Jitter = &j
	}
	stats.ActiveTargets = s.countActiveTargets(ctx)
	return stats, nil
}

// countActiveTargets returns the number of enabled configured targets.
func (s *MeasurementService) countActiveTargets(ctx context.Context) int {
	ts, err := s.targets.ListTargets(ctx)
	if err != nil {
		return 0
	}
	n := 0
	for _, t := range ts {
		if t.Enabled {
			n++
		}
	}
	return n
}

func toMeasurementDTOs(ms []measurement.Measurement) []MeasurementDTO {
	out := make([]MeasurementDTO, len(ms))
	for i, m := range ms {
		out[i] = toMeasurementDTO(m)
	}
	return out
}

// mean returns the arithmetic mean. The caller guarantees len(xs) > 0.
func mean(xs []float64) float64 {
	var s float64
	for _, v := range xs {
		s += v
	}
	return s / float64(len(xs))
}

// jitter is the mean absolute difference between consecutive samples.
func jitter(xs []float64) float64 {
	if len(xs) < 2 {
		return 0
	}
	var s float64
	for i := 1; i < len(xs); i++ {
		s += abs(xs[i] - xs[i-1])
	}
	return s / float64(len(xs)-1)
}

// percentile returns the q-quantile (0..1) of xs using the nearest-rank
// method (ceil(q*N)-th value, 1-indexed). The caller guarantees len(xs) > 0.
func percentile(xs []float64, q float64) float64 {
	sorted := append([]float64(nil), xs...)
	sort.Float64s(sorted)
	idx := int(math.Ceil(q*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func percent(part, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(part) / float64(total) * 100.0
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
