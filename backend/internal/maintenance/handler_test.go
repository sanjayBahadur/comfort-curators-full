package maintenance

import (
	"net/http"
	"testing"
	"time"
)

func TestHandlerNew(t *testing.T) {
	svc := &Service{}
	h := NewHandler(svc)
	if h == nil || h.svc != svc {
		t.Fatal("handler must wrap the service")
	}
}

func TestRegisterRoutes(t *testing.T) {
	svc := &Service{}
	h := NewHandler(svc)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
}

func TestMapError(t *testing.T) {
	cases := []struct {
		err       error
		wantCode  string
		wantState int
	}{
		{ErrRequestNotFound, "NOT_FOUND", http.StatusNotFound},
		{ErrEstimateNotFound, "NOT_FOUND", http.StatusNotFound},
		{ErrWorkOrderNotFound, "NOT_FOUND", http.StatusNotFound},
		{ErrWarrantyNotFound, "NOT_FOUND", http.StatusNotFound},
		{ErrSelfVerificationDenied, "FORBIDDEN", http.StatusForbidden},
		{ErrIndependentVerificationNeeded, "FORBIDDEN", http.StatusForbidden},
		{ErrVendorScopeDenied, "FORBIDDEN", http.StatusForbidden},
		{ErrAICannotApprove, "FORBIDDEN", http.StatusForbidden},
		{ErrSelfApprovalDenied, "FORBIDDEN", http.StatusForbidden},
		{ErrEstimateNotApproved, "ESTIMATE_NOT_APPROVED", http.StatusConflict},
		{ErrRequestNotApproved, "REQUEST_NOT_APPROVED", http.StatusConflict},
		{ErrCompletionEvidenceRequired, "EVIDENCE_REQUIRED", http.StatusUnprocessableEntity},
		{ErrInvalidRequest, "VALIDATION_ERROR", http.StatusUnprocessableEntity},
	}
	for _, c := range cases {
		status, code := mapError(c.err)
		if code != c.wantCode || status != c.wantState {
			t.Fatalf("mapError(%v) = (%d, %s), want (%d, %s)", c.err, status, code, c.wantState, c.wantCode)
		}
	}
}

func TestRequestView(t *testing.T) {
	now := time.Now().UTC()
	r := &MaintenanceRequest{
		ID:         "mtn-1",
		TenantID:   "tenant-1",
		PropertyID: "prop-1",
		Title:      "AC leak",
		Category:   CategorySpecialist,
		Priority:   PriorityHigh,
		RiskLevel:  RiskLevelHigh,
		Status:     RequestStatusTriaged,
		ReportedBy: "ops-1",
		TriagedBy:  "ops-2",
		TriagedAt:  &now,
		EstimateID: "mte-1",
		Notes:      "condenser fault",
		Version:    3,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	v := requestView(r)
	if v["id"] != "mtn-1" {
		t.Fatalf("expected id mtn-1, got %v", v["id"])
	}
	if v["property_id"] != "prop-1" {
		t.Fatalf("expected property_id prop-1, got %v", v["property_id"])
	}
	if v["risk_level"] != RiskLevelHigh {
		t.Fatalf("expected risk_level high, got %v", v["risk_level"])
	}
	if _, ok := v["triaged_at"]; !ok {
		t.Fatal("triaged_at must be present when set")
	}

	r2 := &MaintenanceRequest{ID: "mtn-2", Status: RequestStatusReported}
	v2 := requestView(r2)
	if _, ok := v2["triaged_at"]; ok {
		t.Fatal("triaged_at must be absent when nil")
	}
}

func TestVendorWorkOrderViewIsScopeLimited(t *testing.T) {
	now := time.Now().UTC()
	wo := &VendorWorkOrder{
		ID:                    "mwo-1",
		TenantID:              "tenant-1",
		RequestID:             "mtn-1",
		EstimateID:            "mte-1",
		PropertyID:            "prop-1",
		VendorID:              "vendor-1",
		Scope:                 "replace compressor, drain and recharge",
		RiskLevel:             RiskLevelHigh,
		Status:                WorkOrderStatusAssigned,
		AssignedBy:            "ops-1",
		AssignedAt:            now,
		CompletionEvidenceRef: "",
		Version:               1,
		CreatedAt:             now,
		UpdatedAt:             now,
	}

	v := vendorWorkOrderView(wo)
	if v["scope"] != "replace compressor, drain and recharge" {
		t.Fatalf("vendor view must expose the assigned scope, got %v", v["scope"])
	}
	if v["status"] != WorkOrderStatusAssigned {
		t.Fatalf("vendor view must expose execution state, got %v", v["status"])
	}
	if _, ok := v["tenant_id"]; ok {
		t.Fatal("vendor view must not expose tenant_id")
	}
	if _, ok := v["vendor_id"]; ok {
		t.Fatal("vendor view must not leak the vendor_id")
	}
	if _, ok := v["assigned_by"]; ok {
		t.Fatal("vendor view must not expose assigned_by")
	}
}

func TestEstimateView(t *testing.T) {
	now := time.Now().UTC()
	e := &MaintenanceEstimate{
		ID:               "mte-1",
		TenantID:         "tenant-1",
		RequestID:        "mtn-1",
		PropertyID:       "prop-1",
		PreparedBy:       "ops-1",
		AmountMinorUnits: 25000,
		Currency:         "INR",
		Scope:            "condenser replacement",
		Status:           EstimateStatusApproved,
		ApprovedBy:       "ops-2",
		ApprovedAt:       &now,
		Version:          2,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	v := estimateView(e)
	if v["amount_minor_units"] != int64(25000) {
		t.Fatalf("expected amount_minor_units 25000, got %v", v["amount_minor_units"])
	}
	if v["currency"] != "INR" {
		t.Fatalf("expected currency INR, got %v", v["currency"])
	}
	if v["status"] != EstimateStatusApproved {
		t.Fatalf("expected approved, got %v", v["status"])
	}
}

func TestWarrantyView(t *testing.T) {
	now := time.Now().UTC()
	future := now.Add(365 * 24 * time.Hour)
	w := &WarrantyRecord{
		ID:          "wty-1",
		TenantID:    "tenant-1",
		WorkOrderID: "mwo-1",
		PropertyID:  "prop-1",
		VendorID:    "vendor-1",
		Provider:    "CoolAir Services",
		Coverage:    "parts and labour, 12 months",
		ExpiresAt:   &future,
		Status:      WarrantyStatusActive,
		RecordedBy:  "ops-2",
		CreatedAt:   now,
	}

	v := warrantyView(w)
	if v["provider"] != "CoolAir Services" {
		t.Fatalf("expected provider CoolAir Services, got %v", v["provider"])
	}
	if v["status"] != WarrantyStatusActive {
		t.Fatalf("expected status active, got %v", v["status"])
	}
	if _, ok := v["expires_at"]; !ok {
		t.Fatal("expires_at must be present when set")
	}
}
