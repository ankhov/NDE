package collector

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ankhov/NDE/internal/measurement"
)

// freePort returns a TCP port that is currently free by opening a listener and
// closing it. Two callers racing for the same port is possible but extremely
// unlikely in a single test binary.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// acceptOnce listens on a TCP address, accepts one connection and closes it.
// The TCP collector only needs the handshake to complete; it never sends data.
func acceptOnce(t *testing.T, addr string) {
	t.Helper()
	l, err := net.Listen("tcp", addr)
	require.NoError(t, err)
	go func() {
		defer l.Close()
		c, err := l.Accept()
		if err != nil {
			return
		}
		_ = c.Close()
	}()
}

func TestTCP_Success(t *testing.T) {
	port := freePort(t)
	acceptOnce(t, net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	c := NewTCP(TCPOptions{AllowPrivate: true})

	m, err := c.Collect(context.Background(), measurement.Target{
		ID: "t1", Host: "127.0.0.1", Port: port, Protocol: measurement.TypeTCP,
		Timeout: 2 * time.Second,
	})
	require.NoError(t, err)
	require.True(t, m.Success, "expected success, code=%q", m.ErrorCode)
	require.NotNil(t, m.Latency)
	require.Equal(t, measurement.TypeTCP, m.Type)
	require.Equal(t, "127.0.0.1", m.Metadata[measurement.MetaResolvedIP])
}

func TestTCP_RefusedOrUnreachable(t *testing.T) {
	port := freePort(t) // listener already closed -> closed port
	c := NewTCP(TCPOptions{AllowPrivate: true})

	m, err := c.Collect(context.Background(), measurement.Target{
		ID: "t2", Host: "127.0.0.1", Port: port, Protocol: measurement.TypeTCP,
		Timeout: 2 * time.Second,
	})
	require.NoError(t, err)
	require.False(t, m.Success)
	require.Contains(t, []string{measurement.CodeRefused, measurement.CodeUnreachable}, m.ErrorCode)
	require.Nil(t, m.Latency)
}

func TestTCP_SSRFBlocked(t *testing.T) {
	port := freePort(t)
	acceptOnce(t, net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	c := NewTCP(TCPOptions{AllowPrivate: false}) // guard active

	m, err := c.Collect(context.Background(), measurement.Target{
		ID: "t3", Host: "127.0.0.1", Port: port, Protocol: measurement.TypeTCP,
		Timeout: 2 * time.Second,
	})
	require.NoError(t, err)
	require.False(t, m.Success)
	require.Equal(t, measurement.CodeSSRFBlocked, m.ErrorCode)
}

func TestTCP_InvalidPort(t *testing.T) {
	c := NewTCP(TCPOptions{AllowPrivate: true})
	m, err := c.Collect(context.Background(), measurement.Target{
		ID: "t4", Host: "127.0.0.1", Port: 0, Protocol: measurement.TypeTCP,
		Timeout: time.Second,
	})
	require.NoError(t, err)
	require.False(t, m.Success)
	require.Equal(t, measurement.CodeInvalidConfig, m.ErrorCode)
}
