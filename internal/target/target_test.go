package target

import (
	"testing"
	"time"

	"github.com/ankhov/NDE/internal/measurement"
)

func goodTarget() measurement.Target {
	return measurement.Target{
		ID:                "t1",
		Name:              "Gateway",
		Host:              "1.1.1.1",
		Protocol:          measurement.TypeICMP,
		Port:              0,
		CheckInterval:     10 * time.Second,
		Timeout:           2 * time.Second,
		Enabled:           true,
		WarningThreshold:  100,
		CriticalThreshold: 300,
		Group:             measurement.GroupExternal,
	}
}

func TestValidateAcceptsGood(t *testing.T) {
	if err := Validate(goodTarget()); err != nil {
		t.Fatalf("good target rejected: %v", err)
	}
}

func TestValidateRejectsBad(t *testing.T) {
	cases := map[string]func(measurement.Target) measurement.Target{
		"empty name":      func(t measurement.Target) measurement.Target { t.Name = ""; return t },
		"empty host":      func(t measurement.Target) measurement.Target { t.Host = ""; return t },
		"bad port":        func(t measurement.Target) measurement.Target { t.Port = 70000; return t },
		"bad protocol":    func(t measurement.Target) measurement.Target { t.Protocol = "bogus"; return t },
		"bad group":       func(t measurement.Target) measurement.Target { t.Group = "bogus"; return t },
		"zero interval":   func(t measurement.Target) measurement.Target { t.CheckInterval = 0; return t },
		"zero timeout":    func(t measurement.Target) measurement.Target { t.Timeout = 0; return t },
		"thresholds":      func(t measurement.Target) measurement.Target { t.WarningThreshold = 500; t.CriticalThreshold = 100; return t },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			if err := Validate(mutate(goodTarget())); err == nil {
				t.Fatalf("expected validation error for %s", name)
			}
		})
	}
}

func TestSSRFGuardBlocksPrivate(t *testing.T) {
	for _, h := range []string{"127.0.0.1", "10.0.0.1", "192.168.1.1", "172.16.0.1", "169.254.1.1", "localhost", "::1"} {
		if err := SSRFGuard(h, SSRFOptions{}); err == nil {
			t.Errorf("expected forbidden for %q", h)
		}
	}
}

func TestSSRFGuardAllowsPublicAndHostnames(t *testing.T) {
	for _, h := range []string{"1.1.1.1", "8.8.8.8", "example.com", "1.1.1.1:53"} {
		if err := SSRFGuard(h, SSRFOptions{}); err != nil {
			t.Errorf("expected allowed for %q, got %v", h, err)
		}
	}
}

func TestSSRFGuardAllowPrivate(t *testing.T) {
	if err := SSRFGuard("127.0.0.1", SSRFOptions{AllowPrivate: true}); err != nil {
		t.Errorf("allowprivate should permit loopback: %v", err)
	}
}

func TestHostPort(t *testing.T) {
	if got := HostPort("1.1.1.1", 53); got != "1.1.1.1:53" {
		t.Errorf("ipv4 hostport: %q", got)
	}
	if got := HostPort("::1", 53); got != "[::1]:53" {
		t.Errorf("ipv6 hostport: %q", got)
	}
}
