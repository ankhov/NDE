package measurement

import "testing"

func TestStatus_Derivation(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	cases := []struct {
		name string
		m    Measurement
		want Status
	}{
		{"ok with latency", Measurement{Success: true, Latency: f(12.3)}, StatusOK},
		{"insufficient no latency", Measurement{Success: true}, StatusInsufficient},
		{"error failed", Measurement{Success: false, ErrorCode: CodeTimeout}, StatusError},
		{"error failed but latency set (shouldn't normally happen) -> error wins", Measurement{Success: false, Latency: f(1)}, StatusError},
	}
	for _, c := range cases {
		if got := c.m.Status(); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestIsTerminalError(t *testing.T) {
	if !IsTerminalError(CodeSSRFBlocked) {
		t.Error("SSRF block should be terminal")
	}
	if !IsTerminalError(CodeInvalidConfig) {
		t.Error("invalid config should be terminal")
	}
	for _, c := range []string{CodeTimeout, CodeRefused, CodeUnreachable, CodeHTTPStatus, ""} {
		if IsTerminalError(c) {
			t.Errorf("code %q should not be terminal", c)
		}
	}
}

func TestLatencyMS(t *testing.T) {
	m := Measurement{}
	if _, ok := m.LatencyMS(); ok {
		t.Error("nil latency should report ok=false")
	}
	v := 42.5
	m.Latency = &v
	got, ok := m.LatencyMS()
	if !ok || got != 42.5 {
		t.Errorf("got %v ok=%v want 42.5 ok=true", got, ok)
	}
}
