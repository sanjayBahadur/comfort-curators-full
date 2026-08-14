package inventory

import (
	"testing"
	"time"
)

func TestValidLocationType(t *testing.T) {
	if !ValidLocationType(LocationTypeCentral) {
		t.Fatal("central must be a valid location type")
	}
	if !ValidLocationType(LocationTypeInTransit) {
		t.Fatal("in_transit must be a valid location type")
	}
	if !ValidLocationType(LocationTypeWorkerKit) {
		t.Fatal("worker_kit must be a valid location type")
	}
	if !ValidLocationType(LocationTypeProperty) {
		t.Fatal("property must be a valid location type")
	}
	if ValidLocationType("invalid") {
		t.Fatal("invalid must not be a valid location type")
	}
}

func TestValidMovementType(t *testing.T) {
	for _, mt := range []string{
		MovementTypeReceive, MovementTypeIssue,
		MovementTypeTransferIn, MovementTypeTransferOut,
		MovementTypeAdjustment, MovementTypeReturn,
		MovementTypeConsumption, MovementTypeExpiry,
	} {
		if !ValidMovementType(mt) {
			t.Fatalf("%s must be a valid movement type", mt)
		}
	}
	if ValidMovementType("bogus") {
		t.Fatal("bogus must not be a valid movement type")
	}
}

func TestValidCountStatus(t *testing.T) {
	for _, s := range []string{
		CountStatusDraft, CountStatusInProgress,
		CountStatusReviewed, CountStatusReconciled,
	} {
		if !ValidCountStatus(s) {
			t.Fatalf("%s must be a valid count status", s)
		}
	}
	if ValidCountStatus("bogus") {
		t.Fatal("bogus must not be a valid count status")
	}
}

func TestComputeBalance(t *testing.T) {
	if b := ComputeBalance(nil); b != 0 {
		t.Fatalf("nil movements must give balance 0, got %d", b)
	}

	movements := []InventoryMovement{
		{Quantity: 100},
		{Quantity: 50},
		{Quantity: -30},
		{Quantity: -20},
	}
	bal := ComputeBalance(movements)
	if bal != 100 {
		t.Fatalf("expected balance 100, got %d", bal)
	}
}

func TestIsNegativeStock(t *testing.T) {
	negative, explained := IsNegativeStock(100, -80, MovementTypeIssue)
	if negative {
		t.Fatal("100-80=20 must not be negative")
	}
	if !explained {
		t.Fatal("non-negative result must be explained")
	}

	negative, explained = IsNegativeStock(10, -30, MovementTypeIssue)
	if !negative {
		t.Fatal("10-30=-20 must be negative")
	}
	if explained {
		t.Fatal("issue causing negative must be unexplained")
	}

	negative, explained = IsNegativeStock(10, -30, MovementTypeAdjustment)
	if !negative {
		t.Fatal("10-30=-20 must be negative")
	}
	if !explained {
		t.Fatal("adjustment causing negative must be explained (attributable)")
	}

	negative, explained = IsNegativeStock(0, 50, MovementTypeReceive)
	if negative {
		t.Fatal("0+50=50 must not be negative")
	}
}

func TestIsExpired(t *testing.T) {
	now := time.Now().UTC()

	if IsExpired(nil, now) {
		t.Fatal("nil expiry must not be expired")
	}

	zero := time.Time{}
	if IsExpired(&zero, now) {
		t.Fatal("zero expiry must not be expired")
	}

	past := now.Add(-24 * time.Hour)
	if !IsExpired(&past, now) {
		t.Fatal("past expiry must be expired")
	}

	future := now.Add(24 * time.Hour)
	if IsExpired(&future, now) {
		t.Fatal("future expiry must not be expired")
	}
}
