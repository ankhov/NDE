package collector

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ankhov/NDE/internal/measurement"
)

// fakeRunner returns a canned CommandRunner that yields the given output and
// error regardless of the command invoked.
func fakeRunner(out []byte, err error) CommandRunner {
	return func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return out, err
	}
}

func TestICMP_Success_LinuxOutput(t *testing.T) {
	out := []byte("PING 127.0.0.1 (127.0.0.1) 56(84) bytes of data.\n" +
		"64 bytes from 127.0.0.1: icmp_seq=1 ttl=64 time=0.042 ms\n\n" +
		"--- 127.0.0.1 ping statistics ---\n1 packets transmitted, 1 received, 0% packet loss")
	c := NewICMP(ICMPOptions{AllowPrivate: true}).WithRunner(fakeRunner(out, nil))

	m, err := c.Collect(context.Background(), measurement.Target{
		ID: "t1", Host: "127.0.0.1", Protocol: measurement.TypeICMP, Timeout: 2 * time.Second,
	})
	require.NoError(t, err)
	require.True(t, m.Success, "code=%q", m.ErrorCode)
	require.NotNil(t, m.Latency)
	require.InDelta(t, 0.042, *m.Latency, 0.001)
	require.Equal(t, "127.0.0.1", m.Metadata[measurement.MetaResolvedIP])
}

func TestICMP_Success_WindowsOutput(t *testing.T) {
	out := []byte("Pinging 127.0.0.1 with 32 bytes of data:\n" +
		"Reply from 127.0.0.1: bytes=32 time<1ms TTL=128\n\n" +
		"Ping statistics for 127.0.0.1: Packets: Sent = 1, Received = 1, Lost = 0")
	c := NewICMP(ICMPOptions{AllowPrivate: true}).WithRunner(fakeRunner(out, nil))

	m, err := c.Collect(context.Background(), measurement.Target{
		ID: "t2", Host: "127.0.0.1", Protocol: measurement.TypeICMP, Timeout: 2 * time.Second,
	})
	require.NoError(t, err)
	require.True(t, m.Success)
	require.NotNil(t, m.Latency)
	// "time<1ms" parses to 1.0
	require.InDelta(t, 1.0, *m.Latency, 0.001)
}

func TestICMP_Success_WindowsMillisOutput(t *testing.T) {
	out := []byte("Reply from 1.1.1.1: bytes=32 time=12ms TTL=58\n")
	c := NewICMP(ICMPOptions{AllowPrivate: false}).WithRunner(fakeRunner(out, nil))

	m, err := c.Collect(context.Background(), measurement.Target{
		ID: "t3", Host: "1.1.1.1", Protocol: measurement.TypeICMP, Timeout: 2 * time.Second,
	})
	require.NoError(t, err)
	require.True(t, m.Success)
	require.InDelta(t, 12.0, *m.Latency, 0.001)
}

func TestICMP_Timeout(t *testing.T) {
	// Windows "Request timed out." — no time= marker.
	out := []byte("Pinging 192.0.2.1 with 32 bytes of data:\nRequest timed out.\n")
	c := NewICMP(ICMPOptions{AllowPrivate: true}).WithRunner(fakeRunner(out, nil))

	m, err := c.Collect(context.Background(), measurement.Target{
		ID: "t4", Host: "192.0.2.1", Protocol: measurement.TypeICMP, Timeout: time.Second,
	})
	require.NoError(t, err)
	require.False(t, m.Success)
	require.Equal(t, measurement.CodeTimeout, m.ErrorCode)
	require.Nil(t, m.Latency)
}

func TestICMP_PacketLoss_Timeout(t *testing.T) {
	// Linux 100% packet loss with exit status 1 — transient, classify as timeout.
	out := []byte("--- 127.0.0.1 ping statistics ---\n1 packets transmitted, 0 received, 100% packet loss\n")
	c := NewICMP(ICMPOptions{AllowPrivate: true}).WithRunner(fakeRunner(out, errors.New("exit status 1")))

	m, err := c.Collect(context.Background(), measurement.Target{
		ID: "t5", Host: "127.0.0.1", Protocol: measurement.TypeICMP, Timeout: time.Second,
	})
	require.NoError(t, err)
	require.False(t, m.Success)
	require.Equal(t, measurement.CodeTimeout, m.ErrorCode)
}

func TestICMP_Unreachable(t *testing.T) {
	out := []byte("PING 10.255.255.1 (10.255.255.1) 56(84) bytes of data.\n" +
		"From 10.0.0.1 icmp_seq=1 Destination Host Unreachable\n")
	c := NewICMP(ICMPOptions{AllowPrivate: true}).WithRunner(fakeRunner(out, nil))

	m, err := c.Collect(context.Background(), measurement.Target{
		ID: "t6", Host: "10.255.255.1", Protocol: measurement.TypeICMP, Timeout: time.Second,
	})
	require.NoError(t, err)
	require.False(t, m.Success)
	require.Equal(t, measurement.CodeUnreachable, m.ErrorCode)
}

func TestICMP_SSRFBlocked(t *testing.T) {
	c := NewICMP(ICMPOptions{AllowPrivate: false}).WithRunner(fakeRunner(nil, nil))

	m, err := c.Collect(context.Background(), measurement.Target{
		ID: "t7", Host: "127.0.0.1", Protocol: measurement.TypeICMP, Timeout: time.Second,
	})
	require.NoError(t, err)
	require.False(t, m.Success)
	require.Equal(t, measurement.CodeSSRFBlocked, m.ErrorCode)
}

func TestICMP_InvalidConfig(t *testing.T) {
	c := NewICMP(ICMPOptions{AllowPrivate: true}).WithRunner(fakeRunner(nil, nil))
	m, err := c.Collect(context.Background(), measurement.Target{
		ID: "t8", Host: "", Protocol: measurement.TypeICMP, Timeout: time.Second,
	})
	require.NoError(t, err)
	require.False(t, m.Success)
	require.Equal(t, measurement.CodeInvalidConfig, m.ErrorCode)
}

func TestICMP_NameAndSupports(t *testing.T) {
	c := NewICMP(ICMPOptions{})
	require.Equal(t, "icmp", c.Name())
	require.True(t, c.Supports(measurement.TypeICMP))
	require.False(t, c.Supports(measurement.TypeTCP))
}

func TestParsePingRTT_Formats(t *testing.T) {
	cases := []struct {
		in   string
		want float64
		ok   bool
	}{
		{"time=0.042 ms", 0.042, true},
		{"time=42 ms", 42, true},
		{"time<1ms", 1, true},
		{"time=12.5ms", 12.5, true},
		{"Request timed out.", 0, false},
		{"", 0, false},
	}
	for _, tc := range cases {
		got, ok := parsePingRTT([]byte(tc.in))
		require.Equal(t, tc.ok, ok, "input=%q", tc.in)
		if ok {
			require.InDelta(t, tc.want, got, 0.0001, "input=%q", tc.in)
		}
	}
}
