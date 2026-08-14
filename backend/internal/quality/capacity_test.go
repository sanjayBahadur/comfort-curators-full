package quality

import (
	"fmt"
	"testing"
)

func TestDefaultCapacityTarget(t *testing.T) {
	target := DefaultCapacityTarget()
	want := CapacityTarget{Properties: 50, Reservations: 1000, Workers: 100, Tickets: 50000, Movements: 100000}
	if target != want {
		t.Errorf("default capacity target = %+v, want %+v", target, want)
	}
	if err := target.Validate(); err != nil {
		t.Errorf("default capacity target must validate: %v", err)
	}
}

func TestCapacityTargetValidate(t *testing.T) {
	valid := CapacityTarget{Properties: 1, Reservations: 2, Workers: 3, Tickets: 4, Movements: 5}
	if err := valid.Validate(); err != nil {
		t.Errorf("valid target rejected: %v", err)
	}

	invalid := valid
	invalid.Movements = -1
	if err := invalid.Validate(); err == nil {
		t.Error("negative movements must be rejected")
	}

	invalid = valid
	invalid.Properties = -5
	if err := invalid.Validate(); err == nil {
		t.Error("negative properties must be rejected")
	}

	invalid = valid
	invalid.Properties = 0
	invalid.Tickets = 10
	if err := invalid.Validate(); err == nil {
		t.Error("tickets without properties must be rejected")
	}
}

func TestP95TargetConstant(t *testing.T) {
	if P95TargetMilliseconds != 500 {
		t.Errorf("NFR-003 p95 target must be 500ms, got %v", P95TargetMilliseconds)
	}
}

func TestSeedIDPrefixIsStablePerTenant(t *testing.T) {
	prefixA := seedIDPrefix("tenant-capacity-1")
	prefixB := seedIDPrefix("tenant-capacity-1")
	prefixC := seedIDPrefix("tenant-capacity-2")
	if prefixA != prefixB {
		t.Errorf("same tenant must produce the same prefix: %q vs %q", prefixA, prefixB)
	}
	if prefixA == prefixC {
		t.Errorf("different tenants must produce different prefixes: %q", prefixA)
	}
	if len(prefixA) != 8 {
		t.Errorf("expected 8-hex-char prefix, got %q", prefixA)
	}
}

func TestSeedIDPrefixLengthPreservesIDWidths(t *testing.T) {
	prefix := seedIDPrefix("tenant-capacity-x")
	id := fmt.Sprintf("%s-ticket-%06d", prefix, 49999)
	if len(id) != len(prefix)+14 {
		t.Errorf("ticket id %q has unexpected width", id)
	}
}
