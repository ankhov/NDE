package measurement

import "testing"

func TestLatencyMSAbsentWhenNil(t *testing.T) {
	m := Measurement{Type: TypeICMP}
	if _, ok := m.LatencyMS(); ok {
		t.Fatal("expected ok=false when latency is nil")
	}
}

func TestLatencyMSPresentWhenSet(t *testing.T) {
	v := 12.5
	m := Measurement{Type: TypeTCP, Latency: &v}
	got, ok := m.LatencyMS()
	if !ok || got != 12.5 {
		t.Fatalf("expected 12.5 ok=true, got %v ok=%v", got, ok)
	}
}

func TestWithLatencyIsImmutable(t *testing.T) {
	m := Measurement{Type: TypeHTTP}
	cp := m.WithLatency(7)
	if m.Latency != nil {
		t.Fatal("original mutated")
	}
	got, ok := cp.LatencyMS()
	if !ok || got != 7 {
		t.Fatalf("copy latency mismatch: %v ok=%v", got, ok)
	}
}

func TestTypeValid(t *testing.T) {
	for _, ty := range AllTypes {
		if !ty.Valid() {
			t.Errorf("AllTypes member not valid: %s", ty)
		}
	}
	if Type("bogus").Valid() {
		t.Error("bogus type reported valid")
	}
}

func TestTargetGroupValid(t *testing.T) {
	for _, g := range []TargetGroup{GroupLocal, GroupExternal, GroupDNS, GroupCustom} {
		if !g.Valid() {
			t.Errorf("group not valid: %s", g)
		}
	}
	if TargetGroup("bogus").Valid() {
		t.Error("bogus group reported valid")
	}
}
