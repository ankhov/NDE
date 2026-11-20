package collector

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ankhov/NDE/internal/measurement"
)

func TestTraceroute_Success_LinuxOutput(t *testing.T) {
	out := []byte(`traceroute to 1.1.1.1 (1.1.1.1), 30 hops max, 60 byte packets
 1  192.168.1.1  0.523 ms
 2  10.0.0.1  4.210 ms
 3  1.1.1.1  8.452 ms
`)
	c := NewTraceroute(TracerouteOptions{AllowPrivate: false}).WithRunner(fakeRunner(out, nil))

	m, err := c.Collect(context.Background(), measurement.Target{
		ID: "tr1", Host: "1.1.1.1", Protocol: measurement.TypeTraceroute, Timeout: 5 * time.Second,
	})
	require.NoError(t, err)
	require.True(t, m.Success, "code=%q", m.ErrorCode)
	require.NotNil(t, m.Latency)
	require.InDelta(t, 8.452, *m.Latency, 0.001)

	require.Equal(t, 3, m.Metadata[measurement.MetaHopCount])
	require.Equal(t, true, m.Metadata[measurement.MetaReachedDest])
	require.Equal(t, "1.1.1.1", m.Metadata[measurement.MetaFinalHop])
	hops, ok := m.Metadata[measurement.MetaHops].([]map[string]any)
	require.True(t, ok)
	require.Len(t, hops, 3)
	require.Equal(t, 1, hops[0]["hop"])
	require.Equal(t, "192.168.1.1", hops[0]["ip"])
}

func TestTraceroute_Success_WindowsOutput(t *testing.T) {
	out := []byte(`Tracing route to 1.1.1.1 over a maximum of 30 hops:

  1     1 ms     1 ms     1 ms  192.168.1.1
  2     4 ms     4 ms     4 ms  10.0.0.1
  3     8 ms     8 ms     8 ms  1.1.1.1

Trace complete.
`)
	c := NewTraceroute(TracerouteOptions{AllowPrivate: false}).WithRunner(fakeRunner(out, nil))

	m, err := c.Collect(context.Background(), measurement.Target{
		ID: "tr2", Host: "1.1.1.1", Protocol: measurement.TypeTraceroute, Timeout: 5 * time.Second,
	})
	require.NoError(t, err)
	require.True(t, m.Success)
	require.NotNil(t, m.Latency)
	require.InDelta(t, 8.0, *m.Latency, 0.001)
	require.Equal(t, 3, m.Metadata[measurement.MetaHopCount])
	require.Equal(t, true, m.Metadata[measurement.MetaReachedDest])
}

func TestTraceroute_HopsWithTimeouts_Linux(t *testing.T) {
	// Hop 2 did not respond ("*"); hop 3 (destination) did -> reached, success.
	out := []byte(`traceroute to 1.1.1.1 (1.1.1.1), 30 hops max
 1  192.168.1.1  0.4 ms
 2  *  *  *
 3  1.1.1.1  7.2 ms
`)
	c := NewTraceroute(TracerouteOptions{AllowPrivate: false}).WithRunner(fakeRunner(out, nil))

	m, err := c.Collect(context.Background(), measurement.Target{
		ID: "tr3", Host: "1.1.1.1", Protocol: measurement.TypeTraceroute, Timeout: 5 * time.Second,
	})
	require.NoError(t, err)
	require.True(t, m.Success)
	require.NotNil(t, m.Latency)
	require.InDelta(t, 7.2, *m.Latency, 0.001)

	hops := m.Metadata[measurement.MetaHops].([]map[string]any)
	require.Len(t, hops, 3)
	require.Equal(t, "*", hops[1]["ip"])
	_, hasRtt := hops[1]["rtt_ms"]
	require.False(t, hasRtt, "timeout hop must not carry an RTT")
}

func TestTraceroute_DestinationNotReached(t *testing.T) {
	// Last hop is "*" -> destination never answered -> error (unreachable).
	out := []byte(`traceroute to 203.0.113.1 (203.0.113.1), 30 hops max
 1  192.168.1.1  0.4 ms
 2  10.0.0.1  5.1 ms
 3  *  *  *
`)
	c := NewTraceroute(TracerouteOptions{AllowPrivate: true}).WithRunner(fakeRunner(out, nil))

	m, err := c.Collect(context.Background(), measurement.Target{
		ID: "tr4", Host: "203.0.113.1", Protocol: measurement.TypeTraceroute, Timeout: 5 * time.Second,
	})
	require.NoError(t, err)
	require.False(t, m.Success)
	require.Nil(t, m.Latency)
	require.Equal(t, measurement.CodeUnreachable, m.ErrorCode)
	require.Equal(t, false, m.Metadata[measurement.MetaReachedDest])
}

func TestTraceroute_SSRFBlocked(t *testing.T) {
	c := NewTraceroute(TracerouteOptions{AllowPrivate: false}).WithRunner(fakeRunner(nil, nil))
	m, err := c.Collect(context.Background(), measurement.Target{
		ID: "tr5", Host: "127.0.0.1", Protocol: measurement.TypeTraceroute, Timeout: time.Second,
	})
	require.NoError(t, err)
	require.False(t, m.Success)
	require.Equal(t, measurement.CodeSSRFBlocked, m.ErrorCode)
}

func TestTraceroute_InvalidConfig(t *testing.T) {
	c := NewTraceroute(TracerouteOptions{AllowPrivate: true}).WithRunner(fakeRunner(nil, nil))
	m, err := c.Collect(context.Background(), measurement.Target{
		ID: "tr6", Host: "", Protocol: measurement.TypeTraceroute, Timeout: time.Second,
	})
	require.NoError(t, err)
	require.False(t, m.Success)
	require.Equal(t, measurement.CodeInvalidConfig, m.ErrorCode)
}

func TestTraceroute_NameAndSupports(t *testing.T) {
	c := NewTraceroute(TracerouteOptions{})
	require.Equal(t, "traceroute", c.Name())
	require.True(t, c.Supports(measurement.TypeTraceroute))
	require.False(t, c.Supports(measurement.TypeICMP))
}

func TestParseTraceroute_EmptyOutput(t *testing.T) {
	hops, reached := parseTraceroute(nil)
	require.Empty(t, hops)
	require.False(t, reached)
}

func TestParseTraceroute_MixedFormatsIgnoredHeaders(t *testing.T) {
	// Header lines must not parse as hops; only numbered hop lines count.
	out := []byte(`traceroute to 1.1.1.1 (1.1.1.1), 30 hops max, 60 byte packets
 1  192.168.1.1  0.5 ms
 2  1.1.1.1  9.0 ms
`)
	hops, reached := parseTraceroute(out)
	require.Len(t, hops, 2)
	require.True(t, reached)
}
