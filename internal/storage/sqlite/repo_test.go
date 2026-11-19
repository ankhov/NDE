package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ankhov/NDE/internal/measurement"
)

// newTestDB returns a migrated *DB backed by a per-test temp file.
func newTestDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	db, err := Open(Options{Path: filepath.Join(dir, "nde.db")})
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.Migrate(context.Background()))
	return db
}

func TestTargetRepo_UpsertListGetDelete(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	tg := measurement.Target{
		ID: "t-1", Name: "Cloudflare DNS", Host: "1.1.1.1",
		Protocol: measurement.TypeDNS, Port: 53,
		CheckInterval: 30 * time.Second, Timeout: 2 * time.Second,
		Enabled: true, WarningThreshold: 100, CriticalThreshold: 250,
		Group: measurement.GroupExternal,
	}
	require.NoError(t, db.UpsertTarget(ctx, tg))

	got, err := db.GetTarget(ctx, "t-1")
	require.NoError(t, err)
	assert.Equal(t, tg.Name, got.Name)
	assert.Equal(t, tg.Host, got.Host)
	assert.Equal(t, measurement.TypeDNS, got.Protocol)
	assert.Equal(t, 53, got.Port)
	assert.Equal(t, 30*time.Second, got.CheckInterval)
	assert.Equal(t, 2*time.Second, got.Timeout)
	assert.True(t, got.Enabled)
	assert.Equal(t, measurement.GroupExternal, got.Group)

	list, err := db.ListTargets(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 1)

	// Upsert on same id updates fields without creating a duplicate.
	tg.Name = "Cloudflare DNS (1.1.1.1)"
	tg.Enabled = false
	require.NoError(t, db.UpsertTarget(ctx, tg))
	list, err = db.ListTargets(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 1, "upsert must not duplicate")
	assert.False(t, list[0].Enabled)
	assert.Equal(t, "Cloudflare DNS (1.1.1.1)", list[0].Name)

	n, err := db.CountTargets(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	require.NoError(t, db.DeleteTarget(ctx, "t-1"))
	n, _ = db.CountTargets(ctx)
	assert.Equal(t, 0, n)

	// Delete of a missing target is an error.
	err = db.DeleteTarget(ctx, "missing")
	assert.Error(t, err)
}

func TestMeasurementRepo_WriteAndQuery(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	require.NoError(t, db.UpsertTarget(ctx, measurement.Target{
		ID: "t-1", Name: "x", Host: "1.1.1.1", Protocol: measurement.TypeTCP,
		Port: 443, CheckInterval: time.Second, Timeout: time.Second,
		Enabled: true, Group: measurement.GroupExternal,
	}))

	now := time.Now().UTC().Truncate(time.Millisecond)
	lat := 12.5
	lat2 := 13.7
	ms := []measurement.Measurement{
		{TargetID: "t-1", Timestamp: now, Type: measurement.TypeTCP, Latency: &lat, Success: true,
			Metadata: map[string]any{"resolved_ip": "1.1.1.1"}},
		{TargetID: "t-1", Timestamp: now.Add(-2 * time.Second), Type: measurement.TypeTCP, Latency: &lat2, Success: true},
		{TargetID: "t-1", Timestamp: now.Add(-4 * time.Second), Type: measurement.TypeTCP, ErrorCode: measurement.CodeTimeout},
	}
	require.NoError(t, db.WriteMeasurements(ctx, ms))

	// Empty batch is a no-op.
	require.NoError(t, db.WriteMeasurements(ctx, nil))

	got, err := db.QueryMeasurements(ctx, measurement.QueryFilter{TargetIDs: []string{"t-1"}, Limit: 10})
	require.NoError(t, err)
	assert.Len(t, got, 3)
	// Newest first.
	assert.Equal(t, now.UnixMilli(), got[0].Timestamp.UnixMilli())
	// Latency round-trips.
	v, ok := got[0].LatencyMS()
	require.True(t, ok)
	assert.InDelta(t, 12.5, v, 0.001)
	// Failure keeps nil latency.
	assert.Nil(t, got[2].Latency)
	assert.Equal(t, measurement.CodeTimeout, got[2].ErrorCode)
	// Metadata round-trips.
	assert.Equal(t, "1.1.1.1", got[0].Metadata["resolved_ip"])
}

func TestMeasurementRepo_FilterByTypeAndTime(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	require.NoError(t, db.UpsertTarget(ctx, measurement.Target{
		ID: "t-1", Name: "x", Host: "1.1.1.1", Protocol: measurement.TypeDNS,
		Port: 53, CheckInterval: time.Second, Timeout: time.Second,
		Enabled: true, Group: measurement.GroupDNS,
	}))

	base := time.Now().UTC().Add(-10 * time.Minute)
	lat := 5.0
	for i, typ := range []measurement.Type{measurement.TypeTCP, measurement.TypeDNS, measurement.TypeHTTP} {
		require.NoError(t, db.WriteMeasurements(ctx, []measurement.Measurement{{
			TargetID: "t-1", Timestamp: base.Add(time.Duration(i) * time.Minute),
			Type: typ, Latency: &lat, Success: true,
		}}))
	}

	// Filter by type.
	got, err := db.QueryMeasurements(ctx, measurement.QueryFilter{Types: []measurement.Type{measurement.TypeDNS}})
	require.NoError(t, err)
	assert.Len(t, got, 1)
	assert.Equal(t, measurement.TypeDNS, got[0].Type)

	// Filter by time window: include the two newest (base+1m, base+2m),
	// exclude the oldest (base+0m). From sits exactly on the i=1 boundary.
	got, err = db.QueryMeasurements(ctx, measurement.QueryFilter{
		From: base.Add(time.Minute), To: time.Now().UTC(),
	})
	require.NoError(t, err)
	assert.Len(t, got, 2)
}

func TestMeasurementRepo_CascadeDelete(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	require.NoError(t, db.UpsertTarget(ctx, measurement.Target{
		ID: "t-1", Name: "x", Host: "1.1.1.1", Protocol: measurement.TypeTCP,
		Port: 443, CheckInterval: time.Second, Timeout: time.Second,
		Enabled: true, Group: measurement.GroupExternal,
	}))
	lat := 1.0
	require.NoError(t, db.WriteMeasurements(ctx, []measurement.Measurement{{
		TargetID: "t-1", Timestamp: time.Now().UTC(), Type: measurement.TypeTCP, Latency: &lat, Success: true,
	}}))
	require.NoError(t, db.DeleteTarget(ctx, "t-1"))
	got, err := db.QueryMeasurements(ctx, measurement.QueryFilter{TargetIDs: []string{"t-1"}})
	require.NoError(t, err)
	assert.Empty(t, got, "measurements must cascade-delete with the target")
}
