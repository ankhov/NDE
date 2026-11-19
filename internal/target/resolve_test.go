package target

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveGuarded_LiteralPublic(t *testing.T) {
	ips, err := ResolveGuarded(context.Background(), "8.8.8.8", SSRFOptions{})
	require.NoError(t, err)
	require.Len(t, ips, 1)
	require.Equal(t, "8.8.8.8", ips[0].String())
}

func TestResolveGuarded_LoopbackBlocked(t *testing.T) {
	_, err := ResolveGuarded(context.Background(), "127.0.0.1", SSRFOptions{})
	require.ErrorIs(t, err, ErrForbiddenHost)

	// AllowPrivate lifts the guard.
	ips, err := ResolveGuarded(context.Background(), "127.0.0.1", SSRFOptions{AllowPrivate: true})
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1", ips[0].String())
}

func TestResolveGuarded_PrivateRangesBlocked(t *testing.T) {
	for _, host := range []string{"10.0.0.1", "172.16.0.1", "192.168.1.1", "169.254.169.254"} {
		_, err := ResolveGuarded(context.Background(), host, SSRFOptions{})
		require.ErrorIs(t, err, ErrForbiddenHost, "host %s should be blocked", host)
	}
}

func TestResolveGuarded_MetadataEndpointBlocked(t *testing.T) {
	// The cloud metadata endpoint must always be blocked without AllowPrivate
	// (SPEC §9.3). This is the single most important SSRF guard.
	_, err := ResolveGuarded(context.Background(), "169.254.169.254", SSRFOptions{})
	require.ErrorIs(t, err, ErrForbiddenHost)
}
