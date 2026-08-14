package fleet_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"comfort-curators-backend/internal/fleet"
	"comfort-curators-backend/internal/platform/audit"

	"github.com/jackc/pgx/v5/pgxpool"
)

func fleetPostgresAvailable() bool {
	host := os.Getenv("CC_DB_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("CC_DB_PORT")
	if port == "" {
		port = "5432"
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func fleetDBConnString() string {
	host := os.Getenv("CC_DB_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("CC_DB_PORT")
	if port == "" {
		port = "5432"
	}
	user := os.Getenv("CC_DB_USER")
	if user == "" {
		user = "ccuser"
	}
	pass := os.Getenv("CC_DB_PASS")
	if pass == "" {
		pass = "ccpass"
	}
	name := os.Getenv("CC_DB_NAME")
	if name == "" {
		name = "comfort_curators"
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, pass, host, port, name)
}

func fleetPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if !fleetPostgresAvailable() {
		t.Skip("PostgreSQL not available for fleet integration test")
	}
	pool, err := pgxpool.New(context.Background(), fleetDBConnString())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := fleet.EnsureSchema(context.Background(), pool); err != nil {
		t.Fatalf("ensure fleet schema: %v", err)
	}
	if err := audit.EnsureSchema(context.Background(), pool); err != nil {
		t.Fatalf("ensure audit schema: %v", err)
	}
	for _, table := range []string{
		"fleet_tracking_events",
		"fleet_incidents",
		"fleet_maintenance",
		"fleet_inspections",
		"fleet_custody_events",
		"fleet_batteries",
		"fleet_assets",
		"audit_events",
	} {
		if _, err := pool.Exec(context.Background(), "TRUNCATE "+table+" CASCADE"); err != nil {
			t.Fatalf("truncate %s: %v", table, err)
		}
	}
	return pool
}

func newFleetService(t *testing.T) *fleet.Service {
	t.Helper()
	pool := fleetPool(t)
	return fleet.NewService(pool).WithAudit(audit.NewAuditStore(pool))
}

func createCompliantAsset(t *testing.T, svc *fleet.Service, tenantID string) *fleet.FleetAsset {
	t.Helper()
	asset, _, err := svc.CreateAsset(context.Background(), tenantID, fleet.CreateAssetParams{
		Model:                  "Velo City 250",
		SerialNumber:           "SN-" + tenantID,
		RatedMotorPowerWatts:   250,
		MaximumDesignSpeedKmh:  25,
		DesignSpeedEvidenceRef: "evidence:design-speed-250w",
		ComplianceDocumentRef:  "evidence:compliance-cert",
		BatterySerial:          "BAT-" + tenantID,
		Charger:                "CHG-" + tenantID,
		PurchaseDate:           time.Now().UTC().AddDate(-1, 0, 0),
		WarrantyExpiresAt:      timePtr(time.Now().UTC().AddDate(1, 0, 0)),
		WarrantyTerms:          "24 month frame and battery warranty",
	}, "actor-ops-1")
	if err != nil {
		t.Fatalf("create asset: %v", err)
	}
	return asset
}

func timePtr(t time.Time) *time.Time {
	return &t
}

// --- Pure policy tests ----------------------------------------------------

func TestIsSafetyItemOverdue(t *testing.T) {
	now := time.Date(2026, time.July, 10, 12, 0, 0, 0, time.UTC)

	past := now.Add(-48 * time.Hour)
	future := now.Add(48 * time.Hour)

	open := fleet.ItemStatusOpen
	completed := fleet.ItemStatusCompleted

	if !fleet.IsSafetyItemOverdue(now, &fleet.FleetMaintenanceRecord{
		Status: open,
		DueAt:  &past,
	}) {
		t.Fatal("an open safety item with a past due date must be overdue")
	}

	if fleet.IsSafetyItemOverdue(now, &fleet.FleetMaintenanceRecord{
		Status: open,
		DueAt:  &future,
	}) {
		t.Fatal("an open safety item with a future due date must not be overdue")
	}

	if fleet.IsSafetyItemOverdue(now, &fleet.FleetMaintenanceRecord{
		Status: completed,
		DueAt:  &past,
	}) {
		t.Fatal("a completed safety item must never be overdue")
	}

	if fleet.IsSafetyItemOverdue(now, &fleet.FleetMaintenanceRecord{
		Status: open,
	}) {
		t.Fatal("an open safety item without a due date must not be overdue")
	}
}

func TestEvaluateDispatchOverdueSafetyItemBlocksDispatch(t *testing.T) {
	now := time.Now().UTC()
	asset := &fleet.FleetAsset{
		ID:     "ast-unit-1",
		Status: fleet.AssetStatusAvailable,
	}
	overdue := []fleet.FleetMaintenanceRecord{
		{
			ID:     "mnt-overdue",
			Kind:   fleet.SafetyKindBrake,
			Title:  "brake pads worn",
			Status: fleet.ItemStatusOpen,
			DueAt:  timePtr(now.Add(-24 * time.Hour)),
		},
	}

	block := fleet.EvaluateDispatch(now, asset, overdue, nil, nil)
	if block.Allowed {
		t.Fatal("an overdue safety item must block dispatch")
	}
	found := false
	for _, r := range block.Reasons {
		if r.Code == fleet.DispatchBlockSafetyOverdue {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a SAFETY_ITEM_OVERDUE block reason, got %+v", block.Reasons)
	}

	cleared := []fleet.FleetMaintenanceRecord{
		{
			ID:     "mnt-completed",
			Kind:   fleet.SafetyKindBrake,
			Status: fleet.ItemStatusCompleted,
			DueAt:  timePtr(now.Add(-24 * time.Hour)),
		},
	}
	if block := fleet.EvaluateDispatch(now, asset, cleared, nil, nil); !block.Allowed {
		t.Fatalf("dispatch must be allowed once the overdue item is completed, got %+v", block.Reasons)
	}
}

func TestEvaluateDispatchIncidentFreezesAsset(t *testing.T) {
	now := time.Now().UTC()
	frozen := &fleet.FleetAsset{
		ID:     "ast-unit-2",
		Status: fleet.AssetStatusFrozen,
	}
	block := fleet.EvaluateDispatch(now, frozen, nil, nil, nil)
	if block.Allowed {
		t.Fatal("a frozen asset must not be dispatchable")
	}
	hasFrozen := false
	for _, r := range block.Reasons {
		if r.Code == fleet.DispatchBlockFrozen {
			hasFrozen = true
		}
	}
	if !hasFrozen {
		t.Fatalf("expected ASSET_FROZEN reason, got %+v", block.Reasons)
	}

	available := &fleet.FleetAsset{
		ID:     "ast-unit-3",
		Status: fleet.AssetStatusAvailable,
	}
	incidents := []fleet.FleetIncident{
		{
			ID:          "inc-1",
			Severity:    fleet.IncidentSeverityHigh,
			Description: "brake failure on route",
			Status:      fleet.IncidentStatusOpen,
		},
	}
	block = fleet.EvaluateDispatch(now, available, nil, incidents, nil)
	if block.Allowed {
		t.Fatal("an open incident must block dispatch")
	}
	hasIncident := false
	for _, r := range block.Reasons {
		if r.Code == fleet.DispatchBlockIncidentPending {
			hasIncident = true
		}
	}
	if !hasIncident {
		t.Fatalf("expected INCIDENT_PENDING_REVIEW reason, got %+v", block.Reasons)
	}
}

func TestEvaluateDispatchPreUseInspectionBlocksWhenNotPassing(t *testing.T) {
	now := time.Now().UTC()
	asset := &fleet.FleetAsset{
		ID:     "ast-unit-4",
		Status: fleet.AssetStatusAvailable,
	}

	damaged := &fleet.FleetInspection{
		InspectionType:    fleet.InspectionTypePreUse,
		Result:            fleet.InspectionResultFail,
		DamageReported:    true,
		DamageDescription: "front light not working",
	}
	block := fleet.EvaluateDispatch(now, asset, nil, nil, damaged)
	if block.Allowed {
		t.Fatal("a pre-use inspection that reports damage must block dispatch")
	}
	hasInspection := false
	for _, r := range block.Reasons {
		if r.Code == fleet.DispatchBlockInspectionFailed {
			hasInspection = true
		}
	}
	if !hasInspection {
		t.Fatalf("expected PRE_USE_INSPECTION_NOT_PASSED reason, got %+v", block.Reasons)
	}

	passing := &fleet.FleetInspection{
		InspectionType: fleet.InspectionTypePreUse,
		Result:         fleet.InspectionResultPass,
	}
	if block := fleet.EvaluateDispatch(now, asset, nil, nil, passing); !block.Allowed {
		t.Fatalf("a passing pre-use inspection must allow dispatch, got %+v", block.Reasons)
	}
}

// --- Integration: overdue safety item blocks dispatch ----------------------

func TestFleetOverdueSafetyItemBlocksDispatch(t *testing.T) {
	svc := newFleetService(t)
	ctx := context.Background()
	tenantID := "tenant-fleet-overdue"
	asset := createCompliantAsset(t, svc, tenantID)

	now := time.Now().UTC()

	block, err := svc.DispatchEligibility(ctx, tenantID, asset.ID, now)
	if err != nil {
		t.Fatalf("dispatch eligibility: %v", err)
	}
	if !block.Allowed {
		t.Fatalf("a freshly created compliant asset must be dispatchable, got %+v", block.Reasons)
	}

	item, err := svc.ScheduleSafetyItem(ctx, tenantID, asset.ID, fleet.SafetyItemParams{
		Kind:        fleet.SafetyKindBrake,
		Title:       "brake inspection",
		Description: "annual brake service",
		DueAt:       now.Add(-48 * time.Hour),
	}, "actor-ops-1")
	if err != nil {
		t.Fatalf("schedule safety item: %v", err)
	}

	block, err = svc.DispatchEligibility(ctx, tenantID, asset.ID, now)
	if err != nil {
		t.Fatalf("dispatch eligibility: %v", err)
	}
	if block.Allowed {
		t.Fatal("an overdue brake safety item must block dispatch")
	}

	found := false
	for _, r := range block.Reasons {
		if r.Code == fleet.DispatchBlockSafetyOverdue && r.Message != "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected an overdue safety item reason, got %+v", block.Reasons)
	}

	overdue, err := svc.ListOverdueSafetyItems(ctx, tenantID, asset.ID, now)
	if err != nil {
		t.Fatalf("list overdue items: %v", err)
	}
	if len(overdue) != 1 || overdue[0].ID != item.ID {
		t.Fatalf("expected exactly one overdue item, got %+v", overdue)
	}

	if _, err := svc.CompleteSafetyItem(ctx, tenantID, item.ID, fleet.CompleteSafetyItemParams{
		CompletedAt: now,
		PerformedBy: "technician-1",
		Notes:       "brakes replaced",
	}, "actor-ops-1"); err != nil {
		t.Fatalf("complete safety item: %v", err)
	}

	block, err = svc.DispatchEligibility(ctx, tenantID, asset.ID, now)
	if err != nil {
		t.Fatalf("dispatch eligibility: %v", err)
	}
	if !block.Allowed {
		t.Fatalf("dispatch must be allowed after the overdue item is completed, got %+v", block.Reasons)
	}
}

// --- Integration: incident freezes asset until reviewed --------------------

func TestFleetIncidentFreezesAssetUntilReviewed(t *testing.T) {
	svc := newFleetService(t)
	ctx := context.Background()
	tenantID := "tenant-fleet-incident"
	asset := createCompliantAsset(t, svc, tenantID)

	incident, err := svc.RecordIncident(ctx, tenantID, asset.ID, fleet.IncidentParams{
		Kind:           "collision",
		Severity:       fleet.IncidentSeverityHigh,
		Description:    "worker hit a pothole; front fork damaged",
		SafetyTicketID: "ticket-vehicle-101",
	}, "worker-9")
	if err != nil {
		t.Fatalf("record incident: %v", err)
	}
	if incident.Status != fleet.IncidentStatusOpen {
		t.Fatalf("new incident must be open, got %q", incident.Status)
	}
	if incident.SafetyTicketID != "ticket-vehicle-101" {
		t.Fatalf("incident must link the safety ticket, got %q", incident.SafetyTicketID)
	}

	reloaded, err := svc.GetAsset(ctx, tenantID, asset.ID)
	if err != nil {
		t.Fatalf("reload asset: %v", err)
	}
	if reloaded.Status != fleet.AssetStatusFrozen {
		t.Fatalf("recording an incident must freeze the asset, got status %q", reloaded.Status)
	}

	block, err := svc.DispatchEligibility(ctx, tenantID, asset.ID, time.Now().UTC())
	if err != nil {
		t.Fatalf("dispatch eligibility: %v", err)
	}
	if block.Allowed {
		t.Fatal("a frozen asset with an open incident must not be dispatchable")
	}

	open, err := svc.ListOpenIncidents(ctx, tenantID, asset.ID)
	if err != nil {
		t.Fatalf("list open incidents: %v", err)
	}
	if len(open) != 1 {
		t.Fatalf("expected one open incident, got %d", len(open))
	}

	// A review without a resolution is rejected.
	if _, err := svc.ReviewIncident(ctx, tenantID, incident.ID, fleet.ReviewIncidentParams{}, "reviewer-1"); !errors.Is(err, fleet.ErrIncidentRequiresResolution) {
		t.Fatalf("review without resolution must be rejected, got %v", err)
	}

	resolved, err := svc.ReviewIncident(ctx, tenantID, incident.ID, fleet.ReviewIncidentParams{
		Resolution: "front fork replaced; asset passed post-repair pre-use check",
	}, "reviewer-1")
	if err != nil {
		t.Fatalf("review incident: %v", err)
	}
	if resolved.Status != fleet.IncidentStatusResolved {
		t.Fatalf("reviewed incident must be resolved, got %q", resolved.Status)
	}
	if resolved.ReviewedBy != "reviewer-1" || resolved.ReviewedAt == nil {
		t.Fatalf("reviewed incident must record reviewer and time: %+v", resolved)
	}

	reloaded, err = svc.GetAsset(ctx, tenantID, asset.ID)
	if err != nil {
		t.Fatalf("reload asset after review: %v", err)
	}
	if reloaded.Status != fleet.AssetStatusAvailable {
		t.Fatalf("asset must be unfrozen after the incident is reviewed, got status %q", reloaded.Status)
	}

	block, err = svc.DispatchEligibility(ctx, tenantID, asset.ID, time.Now().UTC())
	if err != nil {
		t.Fatalf("dispatch eligibility after review: %v", err)
	}
	if !block.Allowed {
		t.Fatalf("dispatch must be allowed after the incident is reviewed, got %+v", block.Reasons)
	}
}

// --- Integration: off-duty tracking is not collected -----------------------

func TestFleetOffDutyTrackingIsNotCollected(t *testing.T) {
	svc := newFleetService(t)
	ctx := context.Background()
	tenantID := "tenant-fleet-tracking"
	asset := createCompliantAsset(t, svc, tenantID)
	workerID := "worker-curator-1"

	// Off duty from the start: nothing may be collected.
	status, err := svc.GetTrackingStatus(ctx, tenantID, workerID)
	if err != nil {
		t.Fatalf("tracking status: %v", err)
	}
	if status.Tracking {
		t.Fatal("tracking must be disabled for a worker with no active custody")
	}

	_, err = svc.CollectLocation(ctx, tenantID, workerID, fleet.LocationParams{
		AssetID:   asset.ID,
		Latitude:  28.6139,
		Longitude: 77.2090,
	}, "worker-curator-1")
	if !errors.Is(err, fleet.ErrOffDutyTrackingDisabled) {
		t.Fatalf("off-duty location collection must be refused with ErrOffDutyTrackingDisabled, got %v", err)
	}

	// On duty: handover enables tracking.
	if _, err := svc.Handover(ctx, tenantID, asset.ID, fleet.CustodyParams{
		ToWorkerID:     workerID,
		Condition:      "good",
		Accessories:    "helmet, lock",
		AcknowledgedBy: workerID,
	}, "actor-ops-1"); err != nil {
		t.Fatalf("handover: %v", err)
	}

	status, err = svc.GetTrackingStatus(ctx, tenantID, workerID)
	if err != nil {
		t.Fatalf("tracking status after handover: %v", err)
	}
	if !status.Tracking || status.AssetID != asset.ID {
		t.Fatalf("tracking must be enabled on the handed-over asset, got %+v", status)
	}

	event, err := svc.CollectLocation(ctx, tenantID, workerID, fleet.LocationParams{
		AssetID:   asset.ID,
		Latitude:  28.6139,
		Longitude: 77.2090,
	}, "worker-curator-1")
	if err != nil {
		t.Fatalf("on-duty location collection must succeed, got %v", err)
	}
	if event.WorkerID != workerID || event.AssetID != asset.ID {
		t.Fatalf("tracking event must be attributed to the on-duty worker and asset: %+v", event)
	}

	count, err := svc.CountTrackingEvents(ctx, tenantID, workerID)
	if err != nil {
		t.Fatalf("count tracking events: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly one tracking event while on duty, got %d", count)
	}

	// Off duty again: returning the asset shuts tracking off automatically.
	if _, err := svc.Return(ctx, tenantID, asset.ID, fleet.CustodyParams{
		FromWorkerID: workerID,
		Condition:    "good",
	}, "actor-ops-1"); err != nil {
		t.Fatalf("return: %v", err)
	}

	status, err = svc.GetTrackingStatus(ctx, tenantID, workerID)
	if err != nil {
		t.Fatalf("tracking status after return: %v", err)
	}
	if status.Tracking {
		t.Fatal("tracking must be automatically disabled after the asset is returned")
	}

	_, err = svc.CollectLocation(ctx, tenantID, workerID, fleet.LocationParams{
		AssetID:   asset.ID,
		Latitude:  28.6139,
		Longitude: 77.2090,
	}, "worker-curator-1")
	if !errors.Is(err, fleet.ErrOffDutyTrackingDisabled) {
		t.Fatalf("off-duty location collection must be refused after return, got %v", err)
	}

	count, err = svc.CountTrackingEvents(ctx, tenantID, workerID)
	if err != nil {
		t.Fatalf("count tracking events after refusal: %v", err)
	}
	if count != 1 {
		t.Fatalf("off-duty refusal must never persist a location sample, got %d rows", count)
	}
}

// --- Integration: 250 W and design-speed evidence --------------------------

func TestFleetAssetComplianceValidation(t *testing.T) {
	svc := newFleetService(t)
	ctx := context.Background()
	tenantID := "tenant-fleet-compliance"

	base := fleet.CreateAssetParams{
		Model:                  "Trial 500",
		SerialNumber:           "SN-500",
		RatedMotorPowerWatts:   250,
		MaximumDesignSpeedKmh:  25,
		DesignSpeedEvidenceRef: "evidence:design-speed",
		ComplianceDocumentRef:  "evidence:compliance",
		BatterySerial:          "BAT-500",
		PurchaseDate:           time.Now().UTC(),
	}

	if _, _, err := svc.CreateAsset(ctx, tenantID, base, "actor-ops-1"); err != nil {
		t.Fatalf("compliant asset must be created: %v", err)
	}

	overpowered := base
	overpowered.SerialNumber = "SN-600"
	overpowered.RatedMotorPowerWatts = 300
	if _, _, err := svc.CreateAsset(ctx, tenantID, overpowered, "actor-ops-1"); !errors.Is(err, fleet.ErrPowerLimitExceeded) {
		t.Fatalf("a 300 W motor must be rejected, got %v", err)
	}

	tooFast := base
	tooFast.SerialNumber = "SN-FAST"
	tooFast.MaximumDesignSpeedKmh = 40
	if _, _, err := svc.CreateAsset(ctx, tenantID, tooFast, "actor-ops-1"); !errors.Is(err, fleet.ErrDesignSpeedLimitExceeded) {
		t.Fatalf("a 40 km/h design speed must be rejected, got %v", err)
	}

	noEvidence := base
	noEvidence.SerialNumber = "SN-NOEV"
	noEvidence.DesignSpeedEvidenceRef = ""
	if _, _, err := svc.CreateAsset(ctx, tenantID, noEvidence, "actor-ops-1"); !errors.Is(err, fleet.ErrComplianceEvidenceRequired) {
		t.Fatalf("an asset without design-speed evidence must be rejected, got %v", err)
	}
}

// --- Integration: tenant scoping fails closed ------------------------------

func TestFleetCrossTenantFailsClosed(t *testing.T) {
	svc := newFleetService(t)
	ctx := context.Background()
	asset := createCompliantAsset(t, svc, "tenant-fleet-cross-a")

	other := fleet.NewService(fleetPool(t)).WithAudit(audit.NewAuditStore(fleetPool(t)))

	if _, err := other.GetAsset(ctx, "tenant-fleet-cross-b", asset.ID); !errors.Is(err, fleet.ErrAssetNotFound) {
		t.Fatalf("cross-tenant asset read must fail closed with ErrAssetNotFound, got %v", err)
	}
	if _, err := other.DispatchEligibility(ctx, "tenant-fleet-cross-b", asset.ID, time.Now().UTC()); !errors.Is(err, fleet.ErrAssetNotFound) {
		t.Fatalf("cross-tenant dispatch eligibility must fail closed, got %v", err)
	}
	if _, err := other.RecordIncident(ctx, "tenant-fleet-cross-b", asset.ID, fleet.IncidentParams{
		Severity:       fleet.IncidentSeverityLow,
		Description:    "cross tenant incident",
		SafetyTicketID: "ticket-x",
	}, "actor-b"); !errors.Is(err, fleet.ErrAssetNotFound) {
		t.Fatalf("cross-tenant incident must fail closed, got %v", err)
	}
}

// --- Integration: pre-use inspection and custody ---------------------------

func TestFleetPreUseInspectionDamageBlocksDispatch(t *testing.T) {
	svc := newFleetService(t)
	ctx := context.Background()
	tenantID := "tenant-fleet-preuse"
	asset := createCompliantAsset(t, svc, tenantID)
	now := time.Now().UTC()

	if _, err := svc.RecordInspection(ctx, tenantID, asset.ID, fleet.InspectionParams{
		WorkerID:       "worker-curator-1",
		InspectionType: fleet.InspectionTypePreUse,
		Result:         fleet.InspectionResultPass,
	}, "worker-curator-1"); err != nil {
		t.Fatalf("record passing pre-use inspection: %v", err)
	}

	block, err := svc.DispatchEligibility(ctx, tenantID, asset.ID, now)
	if err != nil {
		t.Fatalf("dispatch eligibility: %v", err)
	}
	if !block.Allowed {
		t.Fatalf("a passing pre-use inspection must not block dispatch, got %+v", block.Reasons)
	}

	if _, err := svc.RecordInspection(ctx, tenantID, asset.ID, fleet.InspectionParams{
		WorkerID:          "worker-curator-1",
		InspectionType:    fleet.InspectionTypePreUse,
		Result:            fleet.InspectionResultFail,
		DamageReported:    true,
		DamageDescription: "rear brake not engaging",
	}, "worker-curator-1"); err != nil {
		t.Fatalf("record damaged pre-use inspection: %v", err)
	}

	block, err = svc.DispatchEligibility(ctx, tenantID, asset.ID, now)
	if err != nil {
		t.Fatalf("dispatch eligibility after damage: %v", err)
	}
	if block.Allowed {
		t.Fatal("reported damage in the latest pre-use inspection must block dispatch")
	}

	// Reporting damage without a description is rejected.
	if _, err := svc.RecordInspection(ctx, tenantID, asset.ID, fleet.InspectionParams{
		WorkerID:       "worker-curator-1",
		InspectionType: fleet.InspectionTypePreUse,
		Result:         fleet.InspectionResultFail,
		DamageReported: true,
	}, "worker-curator-1"); err == nil {
		t.Fatal("damage without a description must be rejected")
	}
}

func TestFleetCustodyEventsRecordHandoverAndReturn(t *testing.T) {
	svc := newFleetService(t)
	ctx := context.Background()
	tenantID := "tenant-fleet-custody"
	asset := createCompliantAsset(t, svc, tenantID)

	if _, err := svc.Handover(ctx, tenantID, asset.ID, fleet.CustodyParams{
		ToWorkerID:     "worker-curator-2",
		Condition:      "excellent",
		Accessories:    "helmet, lock, charger",
		AcknowledgedBy: "worker-curator-2",
	}, "actor-ops-1"); err != nil {
		t.Fatalf("handover: %v", err)
	}

	reloaded, err := svc.GetAsset(ctx, tenantID, asset.ID)
	if err != nil {
		t.Fatalf("reload asset: %v", err)
	}
	if reloaded.AssignedCustodianID != "worker-curator-2" || reloaded.Status != fleet.AssetStatusAssigned {
		t.Fatalf("handover must assign the custodian, got custodian=%q status=%q", reloaded.AssignedCustodianID, reloaded.Status)
	}

	// A second handover by a different worker must fail.
	if _, err := svc.Handover(ctx, tenantID, asset.ID, fleet.CustodyParams{
		FromWorkerID: "worker-other",
		ToWorkerID:   "worker-curator-3",
	}, "actor-ops-1"); !errors.Is(err, fleet.ErrCustodyEventInvalid) {
		t.Fatalf("handover by a non-custodian worker must be rejected, got %v", err)
	}

	if _, err := svc.Return(ctx, tenantID, asset.ID, fleet.CustodyParams{
		FromWorkerID: "worker-curator-2",
		Condition:    "good",
	}, "actor-ops-1"); err != nil {
		t.Fatalf("return: %v", err)
	}

	reloaded, err = svc.GetAsset(ctx, tenantID, asset.ID)
	if err != nil {
		t.Fatalf("reload asset after return: %v", err)
	}
	if reloaded.AssignedCustodianID != "" || reloaded.Status != fleet.AssetStatusAvailable {
		t.Fatalf("return must clear the custodian, got custodian=%q status=%q", reloaded.AssignedCustodianID, reloaded.Status)
	}

	if _, err := svc.Return(ctx, tenantID, asset.ID, fleet.CustodyParams{
		FromWorkerID: "worker-curator-2",
	}, "actor-ops-1"); !errors.Is(err, fleet.ErrNoActiveCustody) {
		t.Fatalf("a second return must fail with ErrNoActiveCustody, got %v", err)
	}

	events, err := svc.ListCustodyEvents(ctx, tenantID, asset.ID)
	if err != nil {
		t.Fatalf("list custody events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected handover and return events, got %d", len(events))
	}
	if events[0].EventType != fleet.CustodyEventTypeHandover || events[1].EventType != fleet.CustodyEventTypeReturn {
		t.Fatalf("custody ledger must preserve handover then return: %+v", events)
	}
}
