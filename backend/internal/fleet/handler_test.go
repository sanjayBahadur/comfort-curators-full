package fleet

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

func TestAssetView(t *testing.T) {
	now := time.Now().UTC()
	past := now.AddDate(-1, 0, 0)
	future := now.AddDate(1, 0, 0)

	a := &FleetAsset{
		ID:                     "ast-test",
		TenantID:               "tenant-1",
		Model:                  "Velo City 250",
		SerialNumber:           "SN-001",
		RatedMotorPowerWatts:   250,
		MaximumDesignSpeedKmh:  25,
		DesignSpeedEvidenceRef: "evidence:ds",
		ComplianceDocumentRef:  "evidence:comp",
		BatterySerial:          "BAT-001",
		Charger:                "CHG-001",
		PurchaseDate:           past,
		WarrantyExpiresAt:      &future,
		WarrantyTerms:          "24 month",
		AssignedCustodianID:    "worker-1",
		Status:                 AssetStatusAssigned,
		Version:                2,
		CreatedAt:              now,
		UpdatedAt:              now,
	}

	v := assetView(a)
	if v["id"] != "ast-test" {
		t.Fatalf("expected id ast-test, got %v", v["id"])
	}
	if v["status"] != AssetStatusAssigned {
		t.Fatalf("expected status assigned, got %v", v["status"])
	}
	if _, ok := v["warranty_expires_at"]; !ok {
		t.Fatal("warranty_expires_at must be present when set")
	}

	a2 := &FleetAsset{
		ID:     "ast-no-warranty",
		Status: AssetStatusAvailable,
	}
	v2 := assetView(a2)
	if _, ok := v2["warranty_expires_at"]; ok {
		t.Fatal("warranty_expires_at must be absent when nil")
	}
}

func TestCustodyEventView(t *testing.T) {
	now := time.Now().UTC()
	e := &FleetCustodyEvent{
		ID:             "cst-1",
		TenantID:       "tenant-1",
		AssetID:        "ast-1",
		EventType:      CustodyEventTypeHandover,
		FromWorkerID:   "w1",
		ToWorkerID:     "w2",
		Condition:      "good",
		Accessories:    "helmet",
		AcknowledgedBy: "w2",
		AcknowledgedAt: &now,
	}

	v := custodyEventView(e)
	if v["id"] != "cst-1" {
		t.Fatalf("expected id cst-1, got %v", v["id"])
	}
	if v["event_type"] != CustodyEventTypeHandover {
		t.Fatalf("expected handover, got %v", v["event_type"])
	}

	e2 := &FleetCustodyEvent{ID: "cst-2"}
	v2 := custodyEventView(e2)
	if _, ok := v2["acknowledged_at"]; ok {
		t.Fatal("acknowledged_at must be absent when nil")
	}
}

func TestIncidentView(t *testing.T) {
	now := time.Now().UTC()
	i := &FleetIncident{
		ID:             "inc-1",
		TenantID:       "tenant-1",
		AssetID:        "ast-1",
		Kind:           "collision",
		Severity:       IncidentSeverityHigh,
		Description:    "fork damaged",
		ReportedBy:     "worker-9",
		SafetyTicketID: "ticket-101",
		Status:         IncidentStatusOpen,
		ReviewedBy:     "reviewer-1",
		ReviewedAt:     &now,
		Resolution:     "replaced fork",
		Version:        1,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	v := incidentView(i)
	if v["id"] != "inc-1" {
		t.Fatalf("expected id inc-1, got %v", v["id"])
	}
	if v["severity"] != IncidentSeverityHigh {
		t.Fatalf("expected severity high, got %v", v["severity"])
	}
	if _, ok := v["reviewed_at"]; !ok {
		t.Fatal("reviewed_at must be present when set")
	}
}

func TestSafetyItemView(t *testing.T) {
	now := time.Now().UTC()
	due := now.Add(24 * time.Hour)
	r := &FleetMaintenanceRecord{
		ID:          "mnt-1",
		TenantID:    "tenant-1",
		AssetID:     "ast-1",
		Kind:        SafetyKindBrake,
		Title:       "brake service",
		Status:      ItemStatusOpen,
		DueAt:       &due,
		PerformedBy: "tech-1",
		Version:     1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	v := safetyItemView(r)
	if v["id"] != "mnt-1" {
		t.Fatalf("expected id mnt-1, got %v", v["id"])
	}
	if v["kind"] != SafetyKindBrake {
		t.Fatalf("expected brake, got %v", v["kind"])
	}
	if _, ok := v["due_at"]; !ok {
		t.Fatal("due_at must be present")
	}
}

func TestInspectionView(t *testing.T) {
	now := time.Now().UTC()
	i := &FleetInspection{
		ID:                "ins-1",
		TenantID:          "tenant-1",
		AssetID:           "ast-1",
		WorkerID:          "worker-1",
		InspectionType:    InspectionTypePreUse,
		Result:            InspectionResultPass,
		DamageReported:    false,
		DamageDescription: "",
		CreatedAt:         now,
	}

	v := inspectionView(i)
	if v["id"] != "ins-1" {
		t.Fatalf("expected id ins-1, got %v", v["id"])
	}
	if v["result"] != InspectionResultPass {
		t.Fatalf("expected pass, got %v", v["result"])
	}
}

func TestTrackingEventView(t *testing.T) {
	now := time.Now().UTC()
	e := &FleetTrackingEvent{
		ID:             "trk-1",
		TenantID:       "tenant-1",
		AssetID:        "ast-1",
		WorkerID:       "worker-1",
		CustodyEventID: "cst-1",
		Latitude:       28.6139,
		Longitude:      77.2090,
		CapturedAt:     now,
		CreatedAt:      now,
	}

	v := trackingEventView(e)
	if v["id"] != "trk-1" {
		t.Fatalf("expected id trk-1, got %v", v["id"])
	}
	if v["latitude"] != 28.6139 {
		t.Fatalf("expected latitude 28.6139, got %v", v["latitude"])
	}
}

func TestDispatchEligibilityPurePolicyIntegration(t *testing.T) {
	now := time.Now().UTC()
	asset := &FleetAsset{
		ID:     "ast-dispatch-test",
		Status: AssetStatusAvailable,
	}

	block := EvaluateDispatch(now, asset, nil, nil, nil)
	if !block.Allowed {
		t.Fatal("clean asset must allow dispatch")
	}

	past := now.Add(-24 * time.Hour)
	overdue := []FleetMaintenanceRecord{
		{
			ID:     "mnt-overdue",
			Kind:   SafetyKindBrake,
			Status: ItemStatusOpen,
			DueAt:  &past,
		},
	}
	block = EvaluateDispatch(now, asset, overdue, nil, nil)
	if block.Allowed {
		t.Fatal("overdue safety item must block dispatch")
	}

	frozen := &FleetAsset{
		ID:     "ast-frozen",
		Status: AssetStatusFrozen,
	}
	block = EvaluateDispatch(now, frozen, nil, nil, nil)
	if block.Allowed {
		t.Fatal("frozen asset must block dispatch")
	}

	incidents := []FleetIncident{
		{
			ID:          "inc-open",
			Severity:    IncidentSeverityHigh,
			Description: "brake failure",
			Status:      IncidentStatusOpen,
		},
	}
	block = EvaluateDispatch(now, asset, nil, incidents, nil)
	if block.Allowed {
		t.Fatal("open incident must block dispatch")
	}

	damagedInspection := &FleetInspection{
		InspectionType:    InspectionTypePreUse,
		Result:            InspectionResultFail,
		DamageReported:    true,
		DamageDescription: "light broken",
	}
	block = EvaluateDispatch(now, asset, nil, nil, damagedInspection)
	if block.Allowed {
		t.Fatal("damaged inspection must block dispatch")
	}
}
