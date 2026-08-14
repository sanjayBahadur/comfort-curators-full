package maintenance

import (
	"errors"
	"testing"
)

func TestValidateStartReady(t *testing.T) {
	if err := ValidateStartReady(EstimateStatusApproved); err != nil {
		t.Fatalf("approved estimate must allow start, got %v", err)
	}

	for _, status := range []string{
		EstimateStatusDraft,
		EstimateStatusPendingApproval,
		EstimateStatusRejected,
		"",
	} {
		if err := ValidateStartReady(status); !errors.Is(err, ErrEstimateNotApproved) {
			t.Fatalf("estimate status %q must block start with ErrEstimateNotApproved, got %v", status, err)
		}
	}
}

func TestValidateVerifier(t *testing.T) {
	t.Run("standard risk may be verified by performer", func(t *testing.T) {
		if err := ValidateVerifier(RiskLevelStandard, "vendor-1", "vendor-1", "vendor-1"); err != nil {
			t.Fatalf("standard risk must not restrict verifier, got %v", err)
		}
		if err := ValidateVerifier(RiskLevelStandard, "ops-1", "vendor-1", "vendor-1"); err != nil {
			t.Fatalf("standard risk must accept independent verifier, got %v", err)
		}
	})

	t.Run("high risk rejects missing verifier", func(t *testing.T) {
		if err := ValidateVerifier(RiskLevelHigh, "", "vendor-1", "vendor-1"); !errors.Is(err, ErrIndependentVerificationNeeded) {
			t.Fatalf("high risk without verifier must fail with ErrIndependentVerificationNeeded, got %v", err)
		}
	})

	t.Run("high risk rejects the performing vendor", func(t *testing.T) {
		if err := ValidateVerifier(RiskLevelHigh, "vendor-1", "vendor-1", "vendor-1"); !errors.Is(err, ErrSelfVerificationDenied) {
			t.Fatalf("performer self-verification must fail with ErrSelfVerificationDenied, got %v", err)
		}
	})

	t.Run("high risk rejects another actor claiming the vendor identity", func(t *testing.T) {
		if err := ValidateVerifier(RiskLevelHigh, "vendor-1", "ops-9", "vendor-1"); !errors.Is(err, ErrSelfVerificationDenied) {
			t.Fatalf("assigned vendor verifying must fail with ErrSelfVerificationDenied, got %v", err)
		}
	})

	t.Run("high risk accepts independent verifier", func(t *testing.T) {
		if err := ValidateVerifier(RiskLevelHigh, "ops-1", "vendor-1", "vendor-1"); err != nil {
			t.Fatalf("independent verifier must be accepted, got %v", err)
		}
	})
}

func TestVendorVisibleOrders(t *testing.T) {
	orders := []VendorWorkOrder{
		{ID: "wo-1", VendorID: "vendor-1"},
		{ID: "wo-2", VendorID: "vendor-2"},
		{ID: "wo-3", VendorID: "vendor-1"},
	}

	visible := VendorVisibleOrders(orders, "vendor-1")
	if len(visible) != 2 {
		t.Fatalf("expected 2 visible orders for vendor-1, got %d", len(visible))
	}
	for _, o := range visible {
		if o.VendorID != "vendor-1" {
			t.Fatalf("vendor-1 must not see order %s scoped to %s", o.ID, o.VendorID)
		}
	}

	other := VendorVisibleOrders(orders, "vendor-2")
	if len(other) != 1 || other[0].ID != "wo-2" {
		t.Fatalf("vendor-2 must only see wo-2, got %+v", other)
	}

	none := VendorVisibleOrders(orders, "vendor-3")
	if len(none) != 0 {
		t.Fatalf("unassigned vendor must see nothing, got %d", len(none))
	}
}

func TestValidStatusHelpers(t *testing.T) {
	if !ValidRequestStatus(RequestStatusTriaged) {
		t.Fatal("triaged must be a valid request status")
	}
	if ValidRequestStatus("bogus") {
		t.Fatal("bogus must not be a valid request status")
	}
	if !ValidEstimateStatus(EstimateStatusApproved) {
		t.Fatal("approved must be a valid estimate status")
	}
	if !ValidWorkOrderStatus(WorkOrderStatusInProgress) {
		t.Fatal("in_progress must be a valid work order status")
	}
	if !ValidApprovalDecision(ApprovalDecisionRejected) {
		t.Fatal("rejected must be a valid approval decision")
	}
	if !ValidRiskLevel(RiskLevelHigh) {
		t.Fatal("high must be a valid risk level")
	}
	if ValidRiskLevel("extreme") {
		t.Fatal("extreme must not be a valid risk level")
	}
	if !ValidCurrency("INR") {
		t.Fatal("INR must be a valid currency")
	}
	if ValidCurrency("Rupee") {
		t.Fatal("Rupee must not be a valid currency")
	}
	if !ValidCategory(CategorySpecialist) {
		t.Fatal("specialist_vendor_request must be a valid category")
	}
	if !ValidPriority(PriorityUrgent) {
		t.Fatal("urgent must be a valid priority")
	}
}

func TestIsValidSHA256Hash(t *testing.T) {
	hash := ComputeEvidenceHash([]byte("before-and-after-photo"))
	if !IsValidSHA256Hash(hash) {
		t.Fatalf("computed hash %q must validate", hash)
	}
	if IsValidSHA256Hash("short") {
		t.Fatal("short string must not validate as sha256")
	}
	if IsValidSHA256Hash("zz" + hash[2:]) {
		t.Fatal("non-hex characters must not validate as sha256")
	}
}
