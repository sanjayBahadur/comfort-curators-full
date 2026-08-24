package procurement_test

import (
	"context"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"comfort-curators-backend/internal/platform/audit"
	"comfort-curators-backend/internal/platform/testdb"
	"comfort-curators-backend/internal/procurement"

	"github.com/jackc/pgx/v5/pgxpool"
)

func procPostgresAvailable() bool {
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

func procDBConnString() string {
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

func procPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if !procPostgresAvailable() {
		t.Skip("PostgreSQL not available for procurement integration test")
	}
	pool, err := pgxpool.New(context.Background(), procDBConnString())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := procurement.EnsureSchema(context.Background(), pool); err != nil {
		t.Fatalf("ensure procurement schema: %v", err)
	}
	if err := audit.EnsureSchema(context.Background(), pool); err != nil {
		t.Fatalf("ensure audit schema: %v", err)
	}
	for _, table := range []string{
		"supplier_rebates",
		"goods_receipt_items",
		"goods_receipts",
		"purchase_order_items",
		"purchase_orders",
		"requisition_approvals",
		"requisition_items",
		"requisitions",
		"supplier_items",
		"suppliers",
		"audit_events",
	} {
		if _, err := pool.Exec(context.Background(), "TRUNCATE "+table+" CASCADE"); err != nil {
			t.Fatalf("truncate %s: %v", table, err)
		}
	}
	return pool
}

func newProcService(t *testing.T, pool *pgxpool.Pool) *procurement.Service {
	t.Helper()
	return procurement.NewService(pool).
		WithAudit(audit.NewAuditStore(pool))
}

func TestCreateSupplier(t *testing.T) {
	pool := procPool(t)
	svc := newProcService(t, pool)

	tenantID := "tenant-test-1"
	actorID := "user-1"

	supplier, err := svc.CreateSupplier(context.Background(), tenantID, procurement.CreateSupplierParams{
		Name:        "Test Supplier Inc.",
		ContactInfo: "contact@testsupplier.com",
	}, actorID)
	if err != nil {
		t.Fatalf("CreateSupplier: %v", err)
	}

	if supplier.Status != procurement.SupplierStatusPendingApproval {
		t.Errorf("expected status pending_approval, got %s", supplier.Status)
	}
	if supplier.CreatedBy != actorID {
		t.Errorf("expected created_by %s, got %s", actorID, supplier.CreatedBy)
	}
}

func TestSupplierRequiresHumanApproval(t *testing.T) {
	pool := procPool(t)
	svc := newProcService(t, pool)

	tenantID := "tenant-test-2"
	actorID := "user-1"

	supplier, err := svc.CreateSupplier(context.Background(), tenantID, procurement.CreateSupplierParams{
		Name: "New Supplier Co.",
	}, actorID)
	if err != nil {
		t.Fatalf("CreateSupplier: %v", err)
	}

	if !supplier.IsNew() {
		t.Errorf("new supplier should be pending_approval")
	}
	if supplier.Status != procurement.SupplierStatusPendingApproval {
		t.Errorf("expected pending_approval, got %s", supplier.Status)
	}

	approved, err := svc.ApproveSupplier(context.Background(), tenantID, supplier.ID, "approver-1")
	if err != nil {
		t.Fatalf("ApproveSupplier: %v", err)
	}
	if approved.Status != procurement.SupplierStatusActive {
		t.Errorf("expected active after approval, got %s", approved.Status)
	}
}

func TestRejectSupplier(t *testing.T) {
	pool := procPool(t)
	svc := newProcService(t, pool)

	tenantID := "tenant-test-3"
	actorID := "user-1"

	supplier, err := svc.CreateSupplier(context.Background(), tenantID, procurement.CreateSupplierParams{
		Name: "Bad Supplier",
	}, actorID)
	if err != nil {
		t.Fatalf("CreateSupplier: %v", err)
	}

	rejected, err := svc.RejectSupplier(context.Background(), tenantID, supplier.ID, "approver-1")
	if err != nil {
		t.Fatalf("RejectSupplier: %v", err)
	}
	if rejected.Status != procurement.SupplierStatusDisabled {
		t.Errorf("expected disabled after rejection, got %s", rejected.Status)
	}
}

func TestCreateSupplierItem(t *testing.T) {
	pool := procPool(t)
	svc := newProcService(t, pool)

	tenantID := "tenant-test-4"
	actorID := "user-1"

	supplier, err := svc.CreateSupplier(context.Background(), tenantID, procurement.CreateSupplierParams{
		Name: "Item Supplier",
	}, actorID)
	if err != nil {
		t.Fatalf("CreateSupplier: %v", err)
	}

	_, err = svc.ApproveSupplier(context.Background(), tenantID, supplier.ID, "approver-1")
	if err != nil {
		t.Fatalf("ApproveSupplier: %v", err)
	}

	si, err := svc.CreateSupplierItem(context.Background(), tenantID, supplier.ID, procurement.CreateSupplierItemParams{
		CatalogItemID:        "cat-001",
		SupplierSKU:          "SKU-001",
		UnitCostMinorUnits:   15000,
		UnitCostCurrency:     "INR",
		LeadTimeDays:         7,
		MinimumOrderQuantity: 10,
	}, actorID)
	if err != nil {
		t.Fatalf("CreateSupplierItem: %v", err)
	}
	if si.UnitCostMinorUnits != 15000 {
		t.Errorf("expected unit_cost 15000, got %d", si.UnitCostMinorUnits)
	}
	if si.LeadTimeDays != 7 {
		t.Errorf("expected lead_time 7, got %d", si.LeadTimeDays)
	}
}

func TestCreateSupplierItemInactiveSupplier(t *testing.T) {
	pool := procPool(t)
	svc := newProcService(t, pool)

	tenantID := "tenant-test-5"
	actorID := "user-1"

	supplier, err := svc.CreateSupplier(context.Background(), tenantID, procurement.CreateSupplierParams{
		Name: "Pending Supplier",
	}, actorID)
	if err != nil {
		t.Fatalf("CreateSupplier: %v", err)
	}

	_, err = svc.CreateSupplierItem(context.Background(), tenantID, supplier.ID, procurement.CreateSupplierItemParams{
		CatalogItemID:      "cat-002",
		UnitCostMinorUnits: 100,
		UnitCostCurrency:   "INR",
	}, actorID)
	if err == nil {
		t.Fatal("expected error creating item for inactive supplier")
	}
}

func TestCreateRequisition(t *testing.T) {
	pool := procPool(t)
	svc := newProcService(t, pool)

	tenantID := "tenant-test-6"
	actorID := "user-1"

	supplier, err := svc.CreateSupplier(context.Background(), tenantID, procurement.CreateSupplierParams{
		Name: "Req Supplier",
	}, actorID)
	if err != nil {
		t.Fatalf("CreateSupplier: %v", err)
	}
	_, err = svc.ApproveSupplier(context.Background(), tenantID, supplier.ID, "approver-1")
	if err != nil {
		t.Fatalf("ApproveSupplier: %v", err)
	}

	si, err := svc.CreateSupplierItem(context.Background(), tenantID, supplier.ID, procurement.CreateSupplierItemParams{
		CatalogItemID:        "cat-req-001",
		SupplierSKU:          "SKU-REQ-001",
		UnitCostMinorUnits:   500,
		UnitCostCurrency:     "INR",
		LeadTimeDays:         3,
		MinimumOrderQuantity: 5,
	}, actorID)
	if err != nil {
		t.Fatalf("CreateSupplierItem: %v", err)
	}

	req, err := svc.CreateRequisition(context.Background(), tenantID, procurement.CreateRequisitionParams{
		PropertyID: "prop-1",
		Notes:      "Test requisition",
		Items: []procurement.RequisitionItemInput{
			{SupplierItemID: si.ID, Quantity: 10},
		},
	}, "creator-1", false)
	if err != nil {
		t.Fatalf("CreateRequisition: %v", err)
	}

	if req.Status != procurement.RequisitionStatusDraft {
		t.Errorf("expected draft, got %s", req.Status)
	}
	if req.TotalCostMinorUnits != 5000 {
		t.Errorf("expected total_cost 5000, got %d", req.TotalCostMinorUnits)
	}
}

func TestAICannotCreateRequisition(t *testing.T) {
	pool := procPool(t)
	svc := newProcService(t, pool)

	tenantID := "tenant-test-7"

	_, err := svc.CreateRequisition(context.Background(), tenantID, procurement.CreateRequisitionParams{
		Items: []procurement.RequisitionItemInput{
			{SupplierItemID: "some-id", Quantity: 1},
		},
	}, "ai-actor", true)

	if err == nil {
		t.Fatal("expected error: AI cannot create requisition")
	}
	if err.Error() != procurement.ErrAICannotApprove.Error() {
		t.Errorf("expected ErrAICannotApprove, got: %v", err)
	}
}

func TestAICannotApproveRequisition(t *testing.T) {
	pool := procPool(t)
	svc := newProcService(t, pool)

	tenantID := "tenant-test-8"
	actorID := "user-1"

	supplier, err := svc.CreateSupplier(context.Background(), tenantID, procurement.CreateSupplierParams{
		Name: "AI Test Supplier",
	}, actorID)
	if err != nil {
		t.Fatalf("CreateSupplier: %v", err)
	}
	_, err = svc.ApproveSupplier(context.Background(), tenantID, supplier.ID, "approver-1")
	if err != nil {
		t.Fatalf("ApproveSupplier: %v", err)
	}

	si, err := svc.CreateSupplierItem(context.Background(), tenantID, supplier.ID, procurement.CreateSupplierItemParams{
		CatalogItemID:      "cat-ai-001",
		UnitCostMinorUnits: 100,
		UnitCostCurrency:   "INR",
	}, actorID)
	if err != nil {
		t.Fatalf("CreateSupplierItem: %v", err)
	}

	req, err := svc.CreateRequisition(context.Background(), tenantID, procurement.CreateRequisitionParams{
		Items: []procurement.RequisitionItemInput{
			{SupplierItemID: si.ID, Quantity: 1},
		},
	}, "creator-1", false)
	if err != nil {
		t.Fatalf("CreateRequisition: %v", err)
	}

	_, err = svc.ApproveRequisition(context.Background(), tenantID, req.ID, procurement.ApproveRequisitionParams{
		ActorID:   "ai-approver",
		IsAIActor: true,
	})
	if err == nil {
		t.Fatal("expected error: AI cannot approve requisition")
	}
}

func TestSelfApprovalDenied(t *testing.T) {
	pool := procPool(t)
	svc := newProcService(t, pool)

	tenantID := "tenant-test-9"
	actorID := "user-1"

	supplier, err := svc.CreateSupplier(context.Background(), tenantID, procurement.CreateSupplierParams{
		Name: "Self Approval Supplier",
	}, actorID)
	if err != nil {
		t.Fatalf("CreateSupplier: %v", err)
	}
	_, err = svc.ApproveSupplier(context.Background(), tenantID, supplier.ID, "approver-1")
	if err != nil {
		t.Fatalf("ApproveSupplier: %v", err)
	}

	si, err := svc.CreateSupplierItem(context.Background(), tenantID, supplier.ID, procurement.CreateSupplierItemParams{
		CatalogItemID:      "cat-self-001",
		UnitCostMinorUnits: 100,
		UnitCostCurrency:   "INR",
	}, actorID)
	if err != nil {
		t.Fatalf("CreateSupplierItem: %v", err)
	}

	req, err := svc.CreateRequisition(context.Background(), tenantID, procurement.CreateRequisitionParams{
		Items: []procurement.RequisitionItemInput{
			{SupplierItemID: si.ID, Quantity: 1},
		},
	}, "creator-self", false)
	if err != nil {
		t.Fatalf("CreateRequisition: %v", err)
	}

	_, err = svc.ApproveRequisition(context.Background(), tenantID, req.ID, procurement.ApproveRequisitionParams{
		ActorID: "creator-self",
	})
	if err == nil {
		t.Fatal("expected error: creator cannot approve own requisition")
	}
}

func TestNewSupplierRequiresHumanApprovalForRequisition(t *testing.T) {
	pool := procPool(t)
	svc := newProcService(t, pool)

	tenantID := "tenant-test-10"
	actorID := "user-1"

	newSupplier, err := svc.CreateSupplier(context.Background(), tenantID, procurement.CreateSupplierParams{
		Name: "Unapproved New Supplier",
	}, actorID)
	if err != nil {
		t.Fatalf("CreateSupplier: %v", err)
	}

	_, err = svc.CreateSupplierItem(context.Background(), tenantID, newSupplier.ID, procurement.CreateSupplierItemParams{
		CatalogItemID:      "cat-new-001",
		UnitCostMinorUnits: 200,
		UnitCostCurrency:   "INR",
	}, actorID)
	if err == nil {
		t.Fatal("expected error creating item for unapproved supplier")
	}
}

func TestRequisitionApprovalAndPurchaseOrder(t *testing.T) {
	pool := procPool(t)
	svc := newProcService(t, pool)

	tenantID := "tenant-test-11"
	actorID := "user-1"

	supplier, err := svc.CreateSupplier(context.Background(), tenantID, procurement.CreateSupplierParams{
		Name: "PO Supplier",
	}, actorID)
	if err != nil {
		t.Fatalf("CreateSupplier: %v", err)
	}
	_, err = svc.ApproveSupplier(context.Background(), tenantID, supplier.ID, "approver-1")
	if err != nil {
		t.Fatalf("ApproveSupplier: %v", err)
	}

	si, err := svc.CreateSupplierItem(context.Background(), tenantID, supplier.ID, procurement.CreateSupplierItemParams{
		CatalogItemID:        "cat-po-001",
		UnitCostMinorUnits:   250,
		UnitCostCurrency:     "INR",
		LeadTimeDays:         5,
		MinimumOrderQuantity: 2,
	}, actorID)
	if err != nil {
		t.Fatalf("CreateSupplierItem: %v", err)
	}

	req, err := svc.CreateRequisition(context.Background(), tenantID, procurement.CreateRequisitionParams{
		PropertyID: "prop-po-1",
		Notes:      "Need supplies",
		Items: []procurement.RequisitionItemInput{
			{SupplierItemID: si.ID, Quantity: 4},
		},
	}, "creator-1", false)
	if err != nil {
		t.Fatalf("CreateRequisition: %v", err)
	}

	approved, err := svc.ApproveRequisition(context.Background(), tenantID, req.ID, procurement.ApproveRequisitionParams{
		ActorID: "approver-2",
	})
	if err != nil {
		t.Fatalf("ApproveRequisition: %v", err)
	}
	if approved.Status != procurement.RequisitionStatusApproved {
		t.Errorf("expected approved, got %s", approved.Status)
	}

	po, err := svc.CreatePurchaseOrder(context.Background(), tenantID, req.ID, procurement.CreatePurchaseOrderParams{
		SupplierID: supplier.ID,
		OrderedBy:  "orderer-1",
	}, actorID)
	if err != nil {
		t.Fatalf("CreatePurchaseOrder: %v", err)
	}
	if po.Status != procurement.PurchaseOrderStatusDraft {
		t.Errorf("expected draft, got %s", po.Status)
	}

	issued, err := svc.IssuePurchaseOrder(context.Background(), tenantID, po.ID, "issuer-1")
	if err != nil {
		t.Fatalf("IssuePurchaseOrder: %v", err)
	}
	if issued.Status != procurement.PurchaseOrderStatusIssued {
		t.Errorf("expected issued, got %s", issued.Status)
	}
}

func TestGoodsReceiptLinksOrderAndEvidence(t *testing.T) {
	pool := procPool(t)
	svc := newProcService(t, pool)

	tenantID := "tenant-test-12"
	actorID := "user-1"

	supplier, err := svc.CreateSupplier(context.Background(), tenantID, procurement.CreateSupplierParams{
		Name: "Receipt Supplier",
	}, actorID)
	if err != nil {
		t.Fatalf("CreateSupplier: %v", err)
	}
	_, err = svc.ApproveSupplier(context.Background(), tenantID, supplier.ID, "approver-1")
	if err != nil {
		t.Fatalf("ApproveSupplier: %v", err)
	}

	si, err := svc.CreateSupplierItem(context.Background(), tenantID, supplier.ID, procurement.CreateSupplierItemParams{
		CatalogItemID:        "cat-rec-001",
		UnitCostMinorUnits:   300,
		UnitCostCurrency:     "INR",
		MinimumOrderQuantity: 1,
	}, actorID)
	if err != nil {
		t.Fatalf("CreateSupplierItem: %v", err)
	}

	req, err := svc.CreateRequisition(context.Background(), tenantID, procurement.CreateRequisitionParams{
		Items: []procurement.RequisitionItemInput{
			{SupplierItemID: si.ID, Quantity: 5},
		},
	}, "creator-1", false)
	if err != nil {
		t.Fatalf("CreateRequisition: %v", err)
	}

	_, err = svc.ApproveRequisition(context.Background(), tenantID, req.ID, procurement.ApproveRequisitionParams{
		ActorID: "approver-2",
	})
	if err != nil {
		t.Fatalf("ApproveRequisition: %v", err)
	}

	po, err := svc.CreatePurchaseOrder(context.Background(), tenantID, req.ID, procurement.CreatePurchaseOrderParams{
		SupplierID: supplier.ID,
		OrderedBy:  "orderer-1",
	}, actorID)
	if err != nil {
		t.Fatalf("CreatePurchaseOrder: %v", err)
	}

	_, err = svc.IssuePurchaseOrder(context.Background(), tenantID, po.ID, "issuer-1")
	if err != nil {
		t.Fatalf("IssuePurchaseOrder: %v", err)
	}

	poItems, err := svc.GetPurchaseOrder(context.Background(), tenantID, po.ID)
	if err != nil {
		t.Fatalf("GetPurchaseOrder: %v", err)
	}
	if len(poItems.Items) == 0 {
		t.Fatal("expected items in purchase order")
	}

	gr, err := svc.ReceiveGoods(context.Background(), tenantID, po.ID, procurement.CreateGoodsReceiptParams{
		ReceivedBy:     "receiver-1",
		Condition:      procurement.ConditionGood,
		ConditionNotes: "All items in good condition",
		EvidenceRef:    "s3://evidence/receipt-photo-001.jpg",
		Items: []procurement.GoodsReceiptItemInput{
			{PurchaseOrderItemID: poItems.Items[0].ID, QuantityReceived: 5},
		},
	}, actorID)
	if err != nil {
		t.Fatalf("ReceiveGoods: %v", err)
	}

	if gr.PurchaseOrderID != po.ID {
		t.Errorf("receipt must link to purchase order; expected %s, got %s", po.ID, gr.PurchaseOrderID)
	}
	if gr.EvidenceRef != "s3://evidence/receipt-photo-001.jpg" {
		t.Errorf("evidence ref mismatch: got %s", gr.EvidenceRef)
	}
	if gr.Status != procurement.ReceiptStatusReceived {
		t.Errorf("expected received status for good condition, got %s", gr.Status)
	}
}

func TestGoodsReceiptDamagedCondition(t *testing.T) {
	pool := procPool(t)
	svc := newProcService(t, pool)

	tenantID := "tenant-test-13"
	actorID := "user-1"

	supplier, err := svc.CreateSupplier(context.Background(), tenantID, procurement.CreateSupplierParams{
		Name: "Damaged Supplier",
	}, actorID)
	if err != nil {
		t.Fatalf("CreateSupplier: %v", err)
	}
	_, err = svc.ApproveSupplier(context.Background(), tenantID, supplier.ID, "approver-1")
	if err != nil {
		t.Fatalf("ApproveSupplier: %v", err)
	}

	si, err := svc.CreateSupplierItem(context.Background(), tenantID, supplier.ID, procurement.CreateSupplierItemParams{
		CatalogItemID:      "cat-dmg-001",
		UnitCostMinorUnits: 100,
		UnitCostCurrency:   "INR",
	}, actorID)
	if err != nil {
		t.Fatalf("CreateSupplierItem: %v", err)
	}

	req, err := svc.CreateRequisition(context.Background(), tenantID, procurement.CreateRequisitionParams{
		Items: []procurement.RequisitionItemInput{
			{SupplierItemID: si.ID, Quantity: 3},
		},
	}, "creator-1", false)
	if err != nil {
		t.Fatalf("CreateRequisition: %v", err)
	}

	_, err = svc.ApproveRequisition(context.Background(), tenantID, req.ID, procurement.ApproveRequisitionParams{
		ActorID: "approver-2",
	})
	if err != nil {
		t.Fatalf("ApproveRequisition: %v", err)
	}

	po, err := svc.CreatePurchaseOrder(context.Background(), tenantID, req.ID, procurement.CreatePurchaseOrderParams{
		SupplierID: supplier.ID,
		OrderedBy:  "orderer-1",
	}, actorID)
	if err != nil {
		t.Fatalf("CreatePurchaseOrder: %v", err)
	}
	_, err = svc.IssuePurchaseOrder(context.Background(), tenantID, po.ID, "issuer-1")
	if err != nil {
		t.Fatalf("IssuePurchaseOrder: %v", err)
	}

	poItems, err := svc.GetPurchaseOrder(context.Background(), tenantID, po.ID)
	if err != nil {
		t.Fatalf("GetPurchaseOrder: %v", err)
	}

	gr, err := svc.ReceiveGoods(context.Background(), tenantID, po.ID, procurement.CreateGoodsReceiptParams{
		ReceivedBy:     "receiver-1",
		Condition:      procurement.ConditionDamaged,
		ConditionNotes: "2 of 3 items damaged",
		EvidenceRef:    "s3://evidence/damage-photo-001.jpg",
		Items: []procurement.GoodsReceiptItemInput{
			{PurchaseOrderItemID: poItems.Items[0].ID, QuantityReceived: 1},
		},
	}, actorID)
	if err != nil {
		t.Fatalf("ReceiveGoods: %v", err)
	}

	if gr.Status != procurement.ReceiptStatusQuarantined {
		t.Errorf("expected quarantined for damaged condition, got %s", gr.Status)
	}
}

func TestRebates(t *testing.T) {
	pool := procPool(t)
	svc := newProcService(t, pool)

	tenantID := "tenant-test-14"

	rebate, err := svc.CreateRebate(context.Background(), tenantID, "", "", procurement.CreateRebateParams{
		Description:      "Volume discount rebate",
		AmountMinorUnits: 50000,
		Currency:         "INR",
	}, "actor-1")
	if err != nil {
		t.Fatalf("CreateRebate: %v", err)
	}

	if rebate.Status != procurement.RebateStatusOffered {
		t.Errorf("expected offered, got %s", rebate.Status)
	}
	if rebate.AmountMinorUnits != 50000 {
		t.Errorf("expected 50000, got %d", rebate.AmountMinorUnits)
	}
}

func TestApproveRequisitionWithNewSupplierBlocked(t *testing.T) {
	pool := procPool(t)
	svc := newProcService(t, pool)

	tenantID := "tenant-test-15"
	actorID := "user-1"

	pendingSupplier, err := svc.CreateSupplier(context.Background(), tenantID, procurement.CreateSupplierParams{
		Name: "Pending New Supplier",
	}, actorID)
	if err != nil {
		t.Fatalf("CreateSupplier: %v", err)
	}

	_, err = svc.CreateSupplierItem(context.Background(), tenantID, pendingSupplier.ID, procurement.CreateSupplierItemParams{
		CatalogItemID:      "cat-block-001",
		UnitCostMinorUnits: 500,
		UnitCostCurrency:   "INR",
	}, actorID)
	if err == nil {
		t.Fatalf("expected error: cannot create item for inactive supplier")
	}
}

func TestCalculateReorderBasis(t *testing.T) {
	pool := procPool(t)
	svc := newProcService(t, pool)

	tenantID := "tenant-test-16"
	actorID := "user-1"

	supplier, err := svc.CreateSupplier(context.Background(), tenantID, procurement.CreateSupplierParams{
		Name: "Reorder Supplier",
	}, actorID)
	if err != nil {
		t.Fatalf("CreateSupplier: %v", err)
	}
	_, err = svc.ApproveSupplier(context.Background(), tenantID, supplier.ID, "approver-1")
	if err != nil {
		t.Fatalf("ApproveSupplier: %v", err)
	}

	si, err := svc.CreateSupplierItem(context.Background(), tenantID, supplier.ID, procurement.CreateSupplierItemParams{
		CatalogItemID:        "cat-reorder-001",
		UnitCostMinorUnits:   200,
		UnitCostCurrency:     "INR",
		LeadTimeDays:         7,
		MinimumOrderQuantity: 10,
	}, actorID)
	if err != nil {
		t.Fatalf("CreateSupplierItem: %v", err)
	}

	stockLevels := []procurement.StockLevel{
		{CatalogItemID: "cat-reorder-001", Balance: 5},
	}

	basis := svc.CalculateReorderBasis(context.Background(), tenantID, "loc-1", stockLevels, []procurement.SupplierItem{*si}, 7, 3)

	if len(basis) == 0 {
		t.Fatal("expected reorder basis calculation")
	}
	if basis[0].CurrentStock != 5 {
		t.Errorf("expected current_stock 5, got %d", basis[0].CurrentStock)
	}
}
