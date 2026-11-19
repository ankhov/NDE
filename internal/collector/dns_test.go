package collector

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"

	"github.com/ankhov/NDE/internal/measurement"
)

// startLocalDNS starts a UDP DNS server on 127.0.0.1 that answers A queries
// for "example.com." with 127.0.0.1 and SERVFAILs anything else. It returns
// the "host:port" address and a shutdown function.
func startLocalDNS(t *testing.T, rcode int, answer string) (addr string, shutdown func()) {
	t.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	addr = conn.LocalAddr().String()

	handler := dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)
		if rcode == dns.RcodeSuccess {
			rr, err := dns.NewRR("example.com. 60 IN A " + answer)
			require.NoError(t, err)
			m.Answer = append(m.Answer, rr)
		} else {
			m.Rcode = rcode
		}
		_ = w.WriteMsg(m)
	})
	srv := &dns.Server{Net: "udp", PacketConn: conn, Handler: handler}
	go func() { _ = srv.ActivateAndServe() }()
	return addr, func() { _ = srv.Shutdown() }
}

func hostPort(addr string) (host string, port int) {
	h, p, err := net.SplitHostPort(addr)
	if err != nil {
		return addr, 53
	}
	port, _ = strconv.Atoi(p)
	return h, port
}

func TestDNS_Success(t *testing.T) {
	addr, shutdown := startLocalDNS(t, dns.RcodeSuccess, "127.0.0.1")
	defer shutdown()
	host, port := hostPort(addr)

	c := NewDNS(DNSOptions{AllowPrivate: true, ProbeName: "example.com."})
	m, err := c.Collect(context.Background(), measurement.Target{
		ID: "d1", Host: host, Port: port, Protocol: measurement.TypeDNS,
		Timeout: 2 * time.Second,
	})
	require.NoError(t, err)
	require.True(t, m.Success, "code=%q", m.ErrorCode)
	require.NotNil(t, m.Latency)
	require.Equal(t, 0, m.Metadata[measurement.MetaDNSRcode])
	require.Equal(t, "127.0.0.1", m.Metadata[measurement.MetaAnswer])
}

func TestDNS_ServerFailure(t *testing.T) {
	addr, shutdown := startLocalDNS(t, dns.RcodeServerFailure, "")
	defer shutdown()
	host, port := hostPort(addr)

	c := NewDNS(DNSOptions{AllowPrivate: true, ProbeName: "example.com."})
	m, err := c.Collect(context.Background(), measurement.Target{
		ID: "d2", Host: host, Port: port, Protocol: measurement.TypeDNS,
		Timeout: 2 * time.Second,
	})
	require.NoError(t, err)
	require.False(t, m.Success)
	require.Equal(t, measurement.CodeDNSError, m.ErrorCode)
	require.Equal(t, dns.RcodeServerFailure, m.Metadata[measurement.MetaDNSRcode])
}

func TestDNS_SSRFBlocked(t *testing.T) {
	c := NewDNS(DNSOptions{AllowPrivate: false, ProbeName: "example.com."})
	m, err := c.Collect(context.Background(), measurement.Target{
		ID: "d3", Host: "127.0.0.1", Port: 53, Protocol: measurement.TypeDNS,
		Timeout: time.Second,
	})
	require.NoError(t, err)
	require.False(t, m.Success)
	require.Equal(t, measurement.CodeSSRFBlocked, m.ErrorCode)
}

func TestDNS_DefaultProbe(t *testing.T) {
	c := NewDNS(DNSOptions{AllowPrivate: true})
	require.Equal(t, "example.com.", c.opts.ProbeName)
	require.Equal(t, uint16(dns.TypeA), c.opts.ProbeType)
}
