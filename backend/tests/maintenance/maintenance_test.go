package maintenance_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"comfort-curators-backend/internal/maintenance"
	"comfort-curators-backend/internal/platform/audit"
	"comfort-curators-backend/internal/platform/testdb"

	"github.com/jackc/pgx/v5/pgxpool"
)

func mtnPostgresAvailable() bool {
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

func mtnDBConnString() string {
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
	name := testdb.MustName()
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, pass, host, port, name)
}

func mtnPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if !mtnPostgresAvailable() {
		t.Skip("PostgreSQL not available for maintenance integration test")
	}
	pool, err := pgxpool.New(context.Background(), mtnDBConnString())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := maintenance.EnsureSchema(context.Background(), pool); err != nil {
		t.Fatalf("ensure maintenance schema: %v", err)
	}
	if err := audit.EnsureSchema(context.Background(), pool); err != nil {
		t.Fatalf("ensure audit schema: %v", err)
	}
	for _, table := range []string{
		"warranty_records",
		"vendor_work_orders",
		"maintenance_approvals",
		"maintenance_estimates",
		"maintenance_requests",
		"audit_events",
	} {
		if _, err := pool.Exec(context.Background(), "TRUNCATE "+table+" CASCADE"); err != nil {
			t.Fatalf("truncate %s: %v", table, err)
		}
	}
	return pool
}

func newMtnService(t *testing.T, pool *pgxpool.Pool) *maintenance.Service {
	t.Helper()
	return maintenance.NewService(pool).
		WithAudit(audit.NewAuditStore(pool))
}

// fullPipeline drives a request from report through an approved estimate and
// vendor assignment, returning the work order. The estimate is approved so
// callers may start the work immediately; the risk level and vendor can be
// chosen per test.
func fullPipeline(t *testing.T, svc *maintenance.Service, tenantID string, riskLevel string, vendorID string) *maintenance.VendorWorkOrder {
	t.Helper()
	ctx := context.Background()

	req, err := svc.CreateRequest(ctx, tenantID, maintenance.CreateRequestParams{
		PropertyID: "prop-1",
		Title:      "AC compressor fault",
		Category:   maintenance.CategorySpecialist,
		Priority:   maintenance.PriorityHigh,
		RiskLevel:  riskLevel,
		Notes:      "warm air from vents",
	}, "ops-1")
	if err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}

	req, err = svc.TriageRequest(ctx, tenantID, req.ID, maintenance.TriageRequestParams{
		Category:  maintenance.CategorySpecialist,
		Priority:  maintenance.PriorityHigh,
		RiskLevel: riskLevel,
		Notes:     "condenser fault",
	}, "ops-2")
	if err != nil {
		t.Fatalf("TriageRequest: %v", err)
	}

	est, err := svc.CreateEstimate(ctx, tenantID, req.ID, maintenance.CreateEstimateParams{
		AmountMinorUnits: 25000,
		Currency:         "INR",
		Scope:            "replace compressor, drain and recharge",
	}, "ops-3")
	if err != nil {
		t.Fatalf("CreateEstimate: %v", err)
	}

	_, err = svc.SubmitEstimate(ctx, tenantID, est.ID, "ops-3")
	if err != nil {
		t.Fatalf("SubmitEstimate: %v", err)
	}

	_, err = svc.DecideEstimate(ctx, tenantID, est.ID, maintenance.DecideEstimateParams{
		ActorID:  "ops-approver",
		Decision: maintenance.ApprovalDecisionApproved,
		Reason:   "within budget",
	})
	if err != nil {
		t.Fatalf("DecideEstimate approve: %v", err)
	}

	wo, err := svc.AssignVendorWorkOrder(ctx, tenantID, req.ID, maintenance.AssignVendorWorkOrderParams{
		VendorID: vendorID,
		Scope:    "replace compressor, drain and recharge",
	}, "ops-4")
	if err != nil {
		t.Fatalf("AssignVendorWorkOrder: %v", err)
	}
	return wo
}

func TestUnapprovedEstimateCannotStart(t *testing.T) {
	pool := mtnPool(t)
	svc := newMtnService(t, pool)
	tenantID := "tenant-mtn-start"

	ctx := context.Background()
	req, err := svc.CreateRequest(ctx, tenantID, maintenance.CreateRequestParams{
		PropertyID: "prop-1",
		Title:      "leaking tap",
		Category:   maintenance.CategoryRoutine,
		RiskLevel:  maintenance.RiskLevelStandard,
	}, "ops-1")
	if err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}
	_, err = svc.TriageRequest(ctx, tenantID, req.ID, maintenance.TriageRequestParams{
		Category:  maintenance.CategoryRoutine,
		Priority:  maintenance.PriorityNormal,
		RiskLevel: maintenance.RiskLevelStandard,
	}, "ops-2")
	if err != nil {
		t.Fatalf("TriageRequest: %v", err)
	}
	est, err := svc.CreateEstimate(ctx, tenantID, req.ID, maintenance.CreateEstimateParams{
		AmountMinorUnits: 1500,
		Currency:         "INR",
		Scope:            "replace washer",
	}, "ops-3")
	if err != nil {
		t.Fatalf("CreateEstimate: %v", err)
	}
	_, err = svc.SubmitEstimate(ctx, tenantID, est.ID, "ops-3")
	if err != nil {
		t.Fatalf("SubmitEstimate: %v", err)
	}
	wo, err := svc.AssignVendorWorkOrder(ctx, tenantID, req.ID, maintenance.AssignVendorWorkOrderParams{
		VendorID: "vendor-1",
		Scope:    "replace washer",
	}, "ops-4")
	if err != nil {
		t.Fatalf("AssignVendorWorkOrder: %v", err)
	}

	_, err = svc.StartWorkOrder(ctx, tenantID, wo.ID, "ops-5")
	if !errors.Is(err, maintenance.ErrEstimateNotApproved) {
		t.Fatalf("starting on an unapproved estimate must fail with ErrEstimateNotApproved, got %v", err)
	}

	still, err := svc.GetWorkOrder(ctx, tenantID, wo.ID)
	if err != nil {
		t.Fatalf("GetWorkOrder: %v", err)
	}
	if still.Status != maintenance.WorkOrderStatusAssigned {
		t.Fatalf("work order must remain assigned, got %s", still.Status)
	}
}

func TestApprovedEstimateAllowsStart(t *testing.T) {
	pool := mtnPool(t)
	svc := newMtnService(t, pool)
	tenantID := "tenant-mtn-start-ok"

	wo := fullPipeline(t, svc, tenantID, maintenance.RiskLevelStandard, "vendor-1")

	started, err := svc.StartWorkOrder(context.Background(), tenantID, wo.ID, "ops-5")
	if err != nil {
		t.Fatalf("StartWorkOrder with approved estimate: %v", err)
	}
	if started.Status != maintenance.WorkOrderStatusInProgress {
		t.Fatalf("expected in_progress, got %s", started.Status)
	}
	if started.StartedAt == nil {
		t.Fatal("started_at must be set")
	}
}

func TestEstimateApprovalControls(t *testing.T) {
	pool := mtnPool(t)
	svc := newMtnService(t, pool)
	tenantID := "tenant-mtn-approval"

	ctx := context.Background()
	req, err := svc.CreateRequest(ctx, tenantID, maintenance.CreateRequestParams{
		PropertyID: "prop-1",
		Title:      "electrical trip",
		Category:   maintenance.CategorySpecialist,
		RiskLevel:  maintenance.RiskLevelHigh,
	}, "ops-1")
	if err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}
	_, err = svc.TriageRequest(ctx, tenantID, req.ID, maintenance.TriageRequestParams{
		Category:  maintenance.CategorySpecialist,
		Priority:  maintenance.PriorityUrgent,
		RiskLevel: maintenance.RiskLevelHigh,
	}, "ops-2")
	if err != nil {
		t.Fatalf("TriageRequest: %v", err)
	}
	est, err := svc.CreateEstimate(ctx, tenantID, req.ID, maintenance.CreateEstimateParams{
		AmountMinorUnits: 80000,
		Currency:         "INR",
		Scope:            "rewire kitchen circuit",
	}, "ops-3")
	if err != nil {
		t.Fatalf("CreateEstimate: %v", err)
	}
	_, err = svc.SubmitEstimate(ctx, tenantID, est.ID, "ops-3")
	if err != nil {
		t.Fatalf("SubmitEstimate: %v", err)
	}

	if _, err := svc.DecideEstimate(ctx, tenantID, est.ID, maintenance.DecideEstimateParams{
		ActorID:   "ai-1",
		IsAIActor: true,
		Decision:  maintenance.ApprovalDecisionApproved,
	}); !errors.Is(err, maintenance.ErrAICannotApprove) {
		t.Fatalf("AI actor must not approve an estimate, got %v", err)
	}

	if _, err := svc.DecideEstimate(ctx, tenantID, est.ID, maintenance.DecideEstimateParams{
		ActorID:  "ops-3",
		Decision: maintenance.ApprovalDecisionApproved,
	}); !errors.Is(err, maintenance.ErrSelfApprovalDenied) {
		t.Fatalf("preparer must not approve own estimate, got %v", err)
	}

	approved, err := svc.DecideEstimate(ctx, tenantID, est.ID, maintenance.DecideEstimateParams{
		ActorID:  "ops-approver",
		Decision: maintenance.ApprovalDecisionApproved,
		Reason:   "authorised specialist work",
	})
	if err != nil {
		t.Fatalf("DecideEstimate approve: %v", err)
	}
	if approved.Status != maintenance.EstimateStatusApproved {
		t.Fatalf("expected approved, got %s", approved.Status)
	}

	approvals, err := svc.GetApprovals(ctx, tenantID, req.ID)
	if err != nil {
		t.Fatalf("GetApprovals: %v", err)
	}
	if len(approvals) != 1 {
		t.Fatalf("expected exactly one recorded approval, got %d", len(approvals))
	}
	if approvals[0].Decision != maintenance.ApprovalDecisionApproved || approvals[0].ActorID != "ops-approver" {
		t.Fatalf("unexpected approval record: %+v", approvals[0])
	}
}

func TestEstimateIsPreservedAfterSubmission(t *testing.T) {
	pool := mtnPool(t)
	svc := newMtnService(t, pool)
	tenantID := "tenant-mtn-preserve"

	ctx := context.Background()
	req, err := svc.CreateRequest(ctx, tenantID, maintenance.CreateRequestParams{
		PropertyID: "prop-1",
		Title:      "gutter repair",
		Category:   maintenance.CategoryRoutine,
		RiskLevel:  maintenance.RiskLevelStandard,
	}, "ops-1")
	if err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}
	_, err = svc.TriageRequest(ctx, tenantID, req.ID, maintenance.TriageRequestParams{
		Category:  maintenance.CategoryRoutine,
		Priority:  maintenance.PriorityLow,
		RiskLevel: maintenance.RiskLevelStandard,
	}, "ops-2")
	if err != nil {
		t.Fatalf("TriageRequest: %v", err)
	}
	est, err := svc.CreateEstimate(ctx, tenantID, req.ID, maintenance.CreateEstimateParams{
		AmountMinorUnits: 5000,
		Currency:         "INR",
		Scope:            "clear and reseal gutters",
	}, "ops-3")
	if err != nil {
		t.Fatalf("CreateEstimate: %v", err)
	}

	submitted, err := svc.SubmitEstimate(ctx, tenantID, est.ID, "ops-3")
	if err != nil {
		t.Fatalf("SubmitEstimate: %v", err)
	}
	if submitted.Status != maintenance.EstimateStatusPendingApproval {
		t.Fatalf("expected pending_approval, got %s", submitted.Status)
	}

	if _, err := svc.SubmitEstimate(ctx, tenantID, est.ID, "ops-3"); !errors.Is(err, maintenance.ErrEstimateImmutable) {
		t.Fatalf("resubmitting a preserved estimate must fail with ErrEstimateImmutable, got %v", err)
	}
}

func TestVendorSeesOnlyAssignedScope(t *testing.T) {
	pool := mtnPool(t)
	svc := newMtnService(t, pool)
	tenantID := "tenant-mtn-vendor"
	ctx := context.Background()

	wo1 := fullPipeline(t, svc, tenantID, maintenance.RiskLevelStandard, "vendor-1")
	wo2 := fullPipeline(t, svc, tenantID, maintenance.RiskLevelStandard, "vendor-2")

	orders, err := svc.ListVendorWorkOrders(ctx, tenantID, "vendor-1")
	if err != nil {
		t.Fatalf("ListVendorWorkOrders: %v", err)
	}
	if len(orders) != 1 || orders[0].ID != wo1.ID {
		t.Fatalf("vendor-1 must see exactly its own work order %s, got %+v", wo1.ID, orders)
	}

	other, err := svc.ListVendorWorkOrders(ctx, tenantID, "vendor-9")
	if err != nil {
		t.Fatalf("ListVendorWorkOrders: %v", err)
	}
	if len(other) != 0 {
		t.Fatalf("an unassigned vendor must see no work orders, got %d", len(other))
	}

	got, err := svc.GetVendorWorkOrder(ctx, tenantID, "vendor-1", wo1.ID)
	if err != nil {
		t.Fatalf("GetVendorWorkOrder for assigned scope: %v", err)
	}
	if got.ID != wo1.ID {
		t.Fatalf("expected %s, got %s", wo1.ID, got.ID)
	}

	if _, err := svc.GetVendorWorkOrder(ctx, tenantID, "vendor-1", wo2.ID); !errors.Is(err, maintenance.ErrVendorScopeDenied) {
		t.Fatalf("vendor-1 reading another vendor's order must fail with ErrVendorScopeDenied, got %v", err)
	}
}

func TestHighRiskActorCannotSelfVerify(t *testing.T) {
	pool := mtnPool(t)
	svc := newMtnService(t, pool)
	tenantID := "tenant-mtn-high-risk"
	ctx := context.Background()

	wo := fullPipeline(t, svc, tenantID, maintenance.RiskLevelHigh, "vendor-1")

	started, err := svc.StartWorkOrder(ctx, tenantID, wo.ID, "ops-5")
	if err != nil {
		t.Fatalf("StartWorkOrder: %v", err)
	}
	if started.Status != maintenance.WorkOrderStatusInProgress {
		t.Fatalf("expected in_progress, got %s", started.Status)
	}

	evidence := maintenance.ComputeEvidenceHash([]byte("completion photo of rewire with tester result"))
	completed, err := svc.CompleteWorkOrder(ctx, tenantID, wo.ID, maintenance.CompleteWorkOrderParams{
		CompletedBy:           "vendor-1",
		CompletionEvidenceRef: evidence,
	})
	if err != nil {
		t.Fatalf("CompleteWorkOrder: %v", err)
	}
	if completed.Status != maintenance.WorkOrderStatusCompleted {
		t.Fatalf("expected completed, got %s", completed.Status)
	}
	if completed.CompletionEvidenceRef != evidence {
		t.Fatalf("completion evidence must be preserved, got %q", completed.CompletionEvidenceRef)
	}

	if _, err := svc.VerifyWorkOrder(ctx, tenantID, wo.ID, "vendor-1"); !errors.Is(err, maintenance.ErrSelfVerificationDenied) {
		t.Fatalf("the performing vendor must not self-verify high-risk work, got %v", err)
	}

	verified, err := svc.VerifyWorkOrder(ctx, tenantID, wo.ID, "ops-verifier")
	if err != nil {
		t.Fatalf("independent verifier must succeed: %v", err)
	}
	if verified.Status != maintenance.WorkOrderStatusVerified {
		t.Fatalf("expected verified, got %s", verified.Status)
	}
	if verified.VerifiedBy != "ops-verifier" {
		t.Fatalf("expected verifier ops-verifier, got %s", verified.VerifiedBy)
	}
}

func TestStandardRiskAllowsDirectVerification(t *testing.T) {
	pool := mtnPool(t)
	svc := newMtnService(t, pool)
	tenantID := "tenant-mtn-standard"
	ctx := context.Background()

	wo := fullPipeline(t, svc, tenantID, maintenance.RiskLevelStandard, "vendor-1")

	if _, err := svc.StartWorkOrder(ctx, tenantID, wo.ID, "ops-5"); err != nil {
		t.Fatalf("StartWorkOrder: %v", err)
	}
	evidence := maintenance.ComputeEvidenceHash([]byte("completion photo of washer replacement"))
	if _, err := svc.CompleteWorkOrder(ctx, tenantID, wo.ID, maintenance.CompleteWorkOrderParams{
		CompletedBy:           "vendor-1",
		CompletionEvidenceRef: evidence,
	}); err != nil {
		t.Fatalf("CompleteWorkOrder: %v", err)
	}

	verified, err := svc.VerifyWorkOrder(ctx, tenantID, wo.ID, "ops-verifier")
	if err != nil {
		t.Fatalf("independent verifier for standard work must succeed: %v", err)
	}
	if verified.Status != maintenance.WorkOrderStatusVerified {
		t.Fatalf("expected verified, got %s", verified.Status)
	}
}

func TestCompletionRequiresEvidence(t *testing.T) {
	pool := mtnPool(t)
	svc := newMtnService(t, pool)
	tenantID := "tenant-mtn-evidence"
	ctx := context.Background()

	wo := fullPipeline(t, svc, tenantID, maintenance.RiskLevelStandard, "vendor-1")

	if _, err := svc.StartWorkOrder(ctx, tenantID, wo.ID, "ops-5"); err != nil {
		t.Fatalf("StartWorkOrder: %v", err)
	}

	if _, err := svc.CompleteWorkOrder(ctx, tenantID, wo.ID, maintenance.CompleteWorkOrderParams{
		CompletedBy: "vendor-1",
	}); !errors.Is(err, maintenance.ErrCompletionEvidenceRequired) {
		t.Fatalf("completion without evidence must fail, got %v", err)
	}

	if _, err := svc.CompleteWorkOrder(ctx, tenantID, wo.ID, maintenance.CompleteWorkOrderParams{
		CompletedBy:           "vendor-1",
		CompletionEvidenceRef: "not-a-sha256",
	}); err == nil {
		t.Fatal("completion with malformed evidence must fail")
	}

	if _, err := svc.CompleteWorkOrder(ctx, tenantID, wo.ID, maintenance.CompleteWorkOrderParams{
		CompletedBy:           "some-other-vendor",
		CompletionEvidenceRef: maintenance.ComputeEvidenceHash([]byte("photo")),
	}); err == nil {
		t.Fatal("only the assigned vendor may complete the work")
	}
}

func TestWarrantyHistoryIsRetainedForVerifiedWork(t *testing.T) {
	pool := mtnPool(t)
	svc := newMtnService(t, pool)
	tenantID := "tenant-mtn-warranty"
	ctx := context.Background()

	wo := fullPipeline(t, svc, tenantID, maintenance.RiskLevelHigh, "vendor-1")

	if _, err := svc.StartWorkOrder(ctx, tenantID, wo.ID, "ops-5"); err != nil {
		t.Fatalf("StartWorkOrder: %v", err)
	}
	if _, err := svc.CompleteWorkOrder(ctx, tenantID, wo.ID, maintenance.CompleteWorkOrderParams{
		CompletedBy:           "vendor-1",
		CompletionEvidenceRef: maintenance.ComputeEvidenceHash([]byte("completion evidence")),
	}); err != nil {
		t.Fatalf("CompleteWorkOrder: %v", err)
	}
	if _, err := svc.VerifyWorkOrder(ctx, tenantID, wo.ID, "ops-verifier"); err != nil {
		t.Fatalf("VerifyWorkOrder: %v", err)
	}

	expiry := time.Now().Add(365 * 24 * time.Hour)
	record, err := svc.RecordWarranty(ctx, tenantID, wo.ID, maintenance.RecordWarrantyParams{
		Provider:  "CoolAir Services",
		Coverage:  "parts and labour, 12 months",
		ExpiresAt: &expiry,
	}, "ops-verifier")
	if err != nil {
		t.Fatalf("RecordWarranty: %v", err)
	}
	if record.Provider != "CoolAir Services" {
		t.Fatalf("expected provider CoolAir Services, got %s", record.Provider)
	}

	warranties, err := svc.ListWarranties(ctx, tenantID, "prop-1")
	if err != nil {
		t.Fatalf("ListWarranties: %v", err)
	}
	if len(warranties) != 1 || warranties[0].ID != record.ID {
		t.Fatalf("warranty history must retain the record, got %+v", warranties)
	}

	closed, err := svc.GetWorkOrder(ctx, tenantID, wo.ID)
	if err != nil {
		t.Fatalf("GetWorkOrder: %v", err)
	}
	if closed.Status != maintenance.WorkOrderStatusClosed {
		t.Fatalf("expected work order closed after warranty, got %s", closed.Status)
	}
}

func TestWarrantyOnlyForVerifiedWork(t *testing.T) {
	pool := mtnPool(t)
	svc := newMtnService(t, pool)
	tenantID := "tenant-mtn-warranty-gate"

	wo := fullPipeline(t, svc, tenantID, maintenance.RiskLevelStandard, "vendor-1")

	if _, err := svc.RecordWarranty(context.Background(), tenantID, wo.ID, maintenance.RecordWarrantyParams{
		Provider: "Dummy",
		Coverage: "none",
	}, "ops-verifier"); err == nil {
		t.Fatal("warranty must only be recorded for verified work")
	}
}

func TestCrossTenantIsolation(t *testing.T) {
	pool := mtnPool(t)
	svc := newMtnService(t, pool)

	wo := fullPipeline(t, svc, "tenant-a", maintenance.RiskLevelStandard, "vendor-1")

	if _, err := svc.GetWorkOrder(context.Background(), "tenant-b", wo.ID); !errors.Is(err, maintenance.ErrWorkOrderNotFound) {
		t.Fatalf("cross-tenant read must not disclose the work order, got %v", err)
	}
	if _, err := svc.ListVendorWorkOrders(context.Background(), "tenant-b", "vendor-1"); err != nil {
		t.Fatalf("ListVendorWorkOrders for another tenant: %v", err)
	}
}
