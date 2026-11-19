package collector

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ankhov/NDE/internal/measurement"
)

func TestHTTP_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewHTTP(HTTPOptions{AllowPrivate: true})
	m, err := c.Collect(context.Background(), measurement.Target{
		ID: "h1", Host: srv.URL, Protocol: measurement.TypeHTTP, Timeout: 2 * time.Second,
	})
	require.NoError(t, err)
	require.True(t, m.Success, "code=%q", m.ErrorCode)
	require.NotNil(t, m.Latency)
	require.Equal(t, http.StatusOK, m.Metadata[measurement.MetaStatusCode])
}

func TestHTTP_NonSuccessStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewHTTP(HTTPOptions{AllowPrivate: true})
	m, err := c.Collect(context.Background(), measurement.Target{
		ID: "h2", Host: srv.URL, Protocol: measurement.TypeHTTP, Timeout: 2 * time.Second,
	})
	require.NoError(t, err)
	require.False(t, m.Success)
	require.Equal(t, measurement.CodeHTTPStatus, m.ErrorCode)
	require.NotNil(t, m.Latency, "latency should still be set: a response arrived")
	require.Equal(t, http.StatusInternalServerError, m.Metadata[measurement.MetaStatusCode])
}

func TestHTTP_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewHTTP(HTTPOptions{AllowPrivate: true})
	m, err := c.Collect(context.Background(), measurement.Target{
		ID: "h3", Host: srv.URL, Protocol: measurement.TypeHTTP, Timeout: 80 * time.Millisecond,
	})
	require.NoError(t, err)
	require.False(t, m.Success)
	require.Equal(t, measurement.CodeTimeout, m.ErrorCode)
}

func TestHTTP_SSRFBlocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewHTTP(HTTPOptions{AllowPrivate: false})
	m, err := c.Collect(context.Background(), measurement.Target{
		ID: "h4", Host: srv.URL, Protocol: measurement.TypeHTTP, Timeout: time.Second,
	})
	require.NoError(t, err)
	require.False(t, m.Success)
	require.Equal(t, measurement.CodeSSRFBlocked, m.ErrorCode)
}

func TestHTTP_AddsScheme(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	// Strip the leading "http://" so the collector must add it back.
	host := srv.URL[len("http://"):]

	c := NewHTTP(HTTPOptions{AllowPrivate: true})
	m, err := c.Collect(context.Background(), measurement.Target{
		ID: "h5", Host: host, Protocol: measurement.TypeHTTP, Timeout: 2 * time.Second,
	})
	require.NoError(t, err)
	require.True(t, m.Success, "code=%q", m.ErrorCode)
}
