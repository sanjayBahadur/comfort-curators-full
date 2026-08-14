package procurement

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newID(prefix string) string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(b[:])
}

type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

const supplierColumns = `id, tenant_id, name, contact_info, status, created_by, approved_by, approved_at, version, created_at, updated_at`

func scanSupplier(row pgx.Row) (*Supplier, error) {
	var s Supplier
	err := row.Scan(
		&s.ID, &s.TenantID, &s.Name, &s.ContactInfo, &s.Status,
		&s.CreatedBy, &s.ApprovedBy, &s.ApprovedAt, &s.Version,
		&s.CreatedAt, &s.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSupplierNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (st *Store) InsertSupplier(ctx context.Context, q querier, s *Supplier) error {
	s.ID = newID("sup")
	_, err := q.Exec(ctx, `
		INSERT INTO suppliers (id, tenant_id, name, contact_info, status, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, s.ID, s.TenantID, s.Name, s.ContactInfo, s.Status, s.CreatedBy)
	return err
}

func (st *Store) GetSupplier(ctx context.Context, tenantID, supplierID string) (*Supplier, error) {
	return scanSupplier(st.pool.QueryRow(ctx, `
		SELECT `+supplierColumns+`
		FROM suppliers
		WHERE id = $1 AND tenant_id = $2
	`, supplierID, tenantID))
}

func (st *Store) ListSuppliers(ctx context.Context, tenantID string) ([]Supplier, error) {
	rows, err := st.pool.Query(ctx, `
		SELECT `+supplierColumns+`
		FROM suppliers
		WHERE tenant_id = $1
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Supplier
	for rows.Next() {
		s, err := scanSupplier(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

func (st *Store) ApproveSupplier(ctx context.Context, q querier, tenantID, supplierID, approvedBy string, approvedAt time.Time) (*Supplier, error) {
	return scanSupplier(q.QueryRow(ctx, `
		UPDATE suppliers
		SET status = $3, approved_by = $4, approved_at = $5, updated_at = NOW(), version = version + 1
		WHERE id = $1 AND tenant_id = $2 AND status = 'pending_approval'
		RETURNING `+supplierColumns+`
	`, supplierID, tenantID, SupplierStatusActive, approvedBy, approvedAt))
}

func (st *Store) RejectSupplier(ctx context.Context, q querier, tenantID, supplierID string) (*Supplier, error) {
	return scanSupplier(q.QueryRow(ctx, `
		UPDATE suppliers
		SET status = $3, updated_at = NOW(), version = version + 1
		WHERE id = $1 AND tenant_id = $2 AND status = 'pending_approval'
		RETURNING `+supplierColumns+`
	`, supplierID, tenantID, SupplierStatusDisabled))
}

const supplierItemColumns = `id, tenant_id, supplier_id, catalog_item_id, supplier_sku,
	unit_cost_minor_units, unit_cost_currency, lead_time_days, minimum_order_quantity, is_preferred,
	version, created_at, updated_at`

func scanSupplierItem(row pgx.Row) (*SupplierItem, error) {
	var si SupplierItem
	err := row.Scan(
		&si.ID, &si.TenantID, &si.SupplierID, &si.CatalogItemID, &si.SupplierSKU,
		&si.UnitCostMinorUnits, &si.UnitCostCurrency, &si.LeadTimeDays, &si.MinimumOrderQuantity, &si.IsPreferred,
		&si.Version, &si.CreatedAt, &si.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSupplierItemNotFound
	}
	if err != nil {
		return nil, err
	}
	return &si, nil
}

func (st *Store) InsertSupplierItem(ctx context.Context, q querier, si *SupplierItem) error {
	si.ID = newID("spi")
	_, err := q.Exec(ctx, `
		INSERT INTO supplier_items (id, tenant_id, supplier_id, catalog_item_id, supplier_sku,
			unit_cost_minor_units, unit_cost_currency, lead_time_days, minimum_order_quantity, is_preferred)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, si.ID, si.TenantID, si.SupplierID, si.CatalogItemID, si.SupplierSKU,
		si.UnitCostMinorUnits, si.UnitCostCurrency, si.LeadTimeDays, si.MinimumOrderQuantity, si.IsPreferred)
	return err
}

func (st *Store) GetSupplierItem(ctx context.Context, tenantID, itemID string) (*SupplierItem, error) {
	return scanSupplierItem(st.pool.QueryRow(ctx, `
		SELECT `+supplierItemColumns+`
		FROM supplier_items
		WHERE id = $1 AND tenant_id = $2
	`, itemID, tenantID))
}

func (st *Store) ListSupplierItems(ctx context.Context, tenantID, supplierID string) ([]SupplierItem, error) {
	rows, err := st.pool.Query(ctx, `
		SELECT `+supplierItemColumns+`
		FROM supplier_items
		WHERE tenant_id = $1 AND supplier_id = $2
		ORDER BY created_at ASC
	`, tenantID, supplierID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SupplierItem
	for rows.Next() {
		si, err := scanSupplierItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *si)
	}
	return out, rows.Err()
}

func (st *Store) GetSupplierItemByCatalog(ctx context.Context, tenantID, supplierID, catalogItemID string) (*SupplierItem, error) {
	return scanSupplierItem(st.pool.QueryRow(ctx, `
		SELECT `+supplierItemColumns+`
		FROM supplier_items
		WHERE tenant_id = $1 AND supplier_id = $2 AND catalog_item_id = $3
	`, tenantID, supplierID, catalogItemID))
}

const requisitionColumns = `id, tenant_id, property_id, status, created_by, approved_by, rejected_by,
	total_cost_minor_units, currency, notes, new_supplier_ids, version, created_at, updated_at`

func scanRequisition(row pgx.Row) (*Requisition, error) {
	var r Requisition
	var newSupplierJSON []byte
	err := row.Scan(
		&r.ID, &r.TenantID, &r.PropertyID, &r.Status, &r.CreatedBy,
		&r.ApprovedBy, &r.RejectedBy, &r.TotalCostMinorUnits, &r.Currency, &r.Notes,
		&newSupplierJSON, &r.Version, &r.CreatedAt, &r.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRequisitionNotFound
	}
	if err != nil {
		return nil, err
	}
	if len(newSupplierJSON) > 0 {
		json.Unmarshal(newSupplierJSON, &r.NewSupplierIDs)
	}
	return &r, nil
}

func (st *Store) InsertRequisition(ctx context.Context, q querier, r *Requisition) error {
	r.ID = newID("req")
	newSupplierJSON, _ := json.Marshal(r.NewSupplierIDs)
	if newSupplierJSON == nil {
		newSupplierJSON = []byte("[]")
	}
	_, err := q.Exec(ctx, `
		INSERT INTO requisitions (id, tenant_id, property_id, status, created_by,
			total_cost_minor_units, currency, notes, new_supplier_ids)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, r.ID, r.TenantID, r.PropertyID, r.Status, r.CreatedBy,
		r.TotalCostMinorUnits, r.Currency, r.Notes, newSupplierJSON)
	return err
}

func (st *Store) GetRequisition(ctx context.Context, tenantID, requisitionID string) (*Requisition, error) {
	return scanRequisition(st.pool.QueryRow(ctx, `
		SELECT `+requisitionColumns+`
		FROM requisitions
		WHERE id = $1 AND tenant_id = $2
	`, requisitionID, tenantID))
}

func (st *Store) ListRequisitions(ctx context.Context, tenantID, propertyID string) ([]Requisition, error) {
	query := `SELECT ` + requisitionColumns + ` FROM requisitions WHERE tenant_id = $1`
	var args []any
	args = append(args, tenantID)
	if propertyID != "" {
		query += ` AND property_id = $2`
		args = append(args, propertyID)
	}
	query += ` ORDER BY created_at DESC`

	rows, err := st.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Requisition
	for rows.Next() {
		r, err := scanRequisition(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

func (st *Store) UpdateRequisitionStatus(ctx context.Context, q querier, tenantID, requisitionID, status, approvedBy, rejectedBy string) (*Requisition, error) {
	return scanRequisition(q.QueryRow(ctx, `
		UPDATE requisitions
		SET status = $3, approved_by = $4, rejected_by = $5, updated_at = NOW(), version = version + 1
		WHERE id = $1 AND tenant_id = $2
		RETURNING `+requisitionColumns+`
	`, requisitionID, tenantID, status, approvedBy, rejectedBy))
}

func (st *Store) UpdateRequisitionTotal(ctx context.Context, q querier, tenantID, requisitionID string, total int64, currency string) (*Requisition, error) {
	return scanRequisition(q.QueryRow(ctx, `
		UPDATE requisitions
		SET total_cost_minor_units = $3, currency = $4, updated_at = NOW(), version = version + 1
		WHERE id = $1 AND tenant_id = $2
		RETURNING `+requisitionColumns+`
	`, requisitionID, tenantID, total, currency))
}

const requisitionItemColumns = `id, tenant_id, requisition_id, catalog_item_id, supplier_item_id,
	quantity, unit_cost_minor_units, unit_cost_currency, line_total_minor_units, created_at`

func scanRequisitionItem(row pgx.Row) (*RequisitionItem, error) {
	var ri RequisitionItem
	err := row.Scan(
		&ri.ID, &ri.TenantID, &ri.RequisitionID, &ri.CatalogItemID, &ri.SupplierItemID,
		&ri.Quantity, &ri.UnitCostMinorUnits, &ri.UnitCostCurrency, &ri.LineTotalMinorUnits, &ri.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRequisitionItemNotFound
	}
	if err != nil {
		return nil, err
	}
	return &ri, nil
}

func (st *Store) InsertRequisitionItem(ctx context.Context, q querier, ri *RequisitionItem) error {
	ri.ID = newID("rqi")
	_, err := q.Exec(ctx, `
		INSERT INTO requisition_items (id, tenant_id, requisition_id, catalog_item_id, supplier_item_id,
			quantity, unit_cost_minor_units, unit_cost_currency, line_total_minor_units)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, ri.ID, ri.TenantID, ri.RequisitionID, ri.CatalogItemID, ri.SupplierItemID,
		ri.Quantity, ri.UnitCostMinorUnits, ri.UnitCostCurrency, ri.LineTotalMinorUnits)
	return err
}

func (st *Store) ListRequisitionItems(ctx context.Context, tenantID, requisitionID string) ([]RequisitionItem, error) {
	rows, err := st.pool.Query(ctx, `
		SELECT `+requisitionItemColumns+`
		FROM requisition_items
		WHERE tenant_id = $1 AND requisition_id = $2
		ORDER BY created_at ASC
	`, tenantID, requisitionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RequisitionItem
	for rows.Next() {
		ri, err := scanRequisitionItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *ri)
	}
	return out, rows.Err()
}

func (st *Store) GetRequisitionItem(ctx context.Context, tenantID, itemID string) (*RequisitionItem, error) {
	return scanRequisitionItem(st.pool.QueryRow(ctx, `
		SELECT `+requisitionItemColumns+`
		FROM requisition_items
		WHERE id = $1 AND tenant_id = $2
	`, itemID, tenantID))
}

const approvalColumns = `id, tenant_id, requisition_id, actor_id, decision, reason, is_ai_actor, created_at`

func scanApproval(row pgx.Row) (*RequisitionApproval, error) {
	var a RequisitionApproval
	err := row.Scan(
		&a.ID, &a.TenantID, &a.RequisitionID, &a.ActorID, &a.Decision,
		&a.Reason, &a.IsAIActor, &a.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrApprovalNotFound
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (st *Store) InsertApproval(ctx context.Context, q querier, a *RequisitionApproval) error {
	a.ID = newID("apr")
	_, err := q.Exec(ctx, `
		INSERT INTO requisition_approvals (id, tenant_id, requisition_id, actor_id, decision, reason, is_ai_actor)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, a.ID, a.TenantID, a.RequisitionID, a.ActorID, a.Decision, a.Reason, a.IsAIActor)
	return err
}

func (st *Store) ListApprovals(ctx context.Context, tenantID, requisitionID string) ([]RequisitionApproval, error) {
	rows, err := st.pool.Query(ctx, `
		SELECT `+approvalColumns+`
		FROM requisition_approvals
		WHERE tenant_id = $1 AND requisition_id = $2
		ORDER BY created_at DESC
	`, tenantID, requisitionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RequisitionApproval
	for rows.Next() {
		a, err := scanApproval(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

const purchaseOrderColumns = `id, tenant_id, requisition_id, supplier_id, status,
	ordered_by, total_minor_units, currency, order_date, expected_delivery,
	version, created_at, updated_at`

func scanPurchaseOrder(row pgx.Row) (*PurchaseOrder, error) {
	var po PurchaseOrder
	err := row.Scan(
		&po.ID, &po.TenantID, &po.RequisitionID, &po.SupplierID, &po.Status,
		&po.OrderedBy, &po.TotalMinorUnits, &po.Currency, &po.OrderDate, &po.ExpectedDelivery,
		&po.Version, &po.CreatedAt, &po.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPurchaseOrderNotFound
	}
	if err != nil {
		return nil, err
	}
	return &po, nil
}

func (st *Store) InsertPurchaseOrder(ctx context.Context, q querier, po *PurchaseOrder) error {
	po.ID = newID("po")
	_, err := q.Exec(ctx, `
		INSERT INTO purchase_orders (id, tenant_id, requisition_id, supplier_id, status,
			ordered_by, total_minor_units, currency, order_date, expected_delivery)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, po.ID, po.TenantID, po.RequisitionID, po.SupplierID, po.Status,
		po.OrderedBy, po.TotalMinorUnits, po.Currency, po.OrderDate, nullTime(po.ExpectedDelivery))
	return err
}

func (st *Store) GetPurchaseOrder(ctx context.Context, tenantID, poID string) (*PurchaseOrder, error) {
	return scanPurchaseOrder(st.pool.QueryRow(ctx, `
		SELECT `+purchaseOrderColumns+`
		FROM purchase_orders
		WHERE id = $1 AND tenant_id = $2
	`, poID, tenantID))
}

func (st *Store) ListPurchaseOrders(ctx context.Context, tenantID, requisitionID string) ([]PurchaseOrder, error) {
	query := `SELECT ` + purchaseOrderColumns + ` FROM purchase_orders WHERE tenant_id = $1`
	var args []any
	args = append(args, tenantID)
	if requisitionID != "" {
		query += ` AND requisition_id = $2`
		args = append(args, requisitionID)
	}
	query += ` ORDER BY created_at DESC`

	rows, err := st.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PurchaseOrder
	for rows.Next() {
		po, err := scanPurchaseOrder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *po)
	}
	return out, rows.Err()
}

func (st *Store) UpdatePurchaseOrderStatus(ctx context.Context, q querier, tenantID, poID, status string) (*PurchaseOrder, error) {
	return scanPurchaseOrder(q.QueryRow(ctx, `
		UPDATE purchase_orders
		SET status = $3, updated_at = NOW(), version = version + 1
		WHERE id = $1 AND tenant_id = $2
		RETURNING `+purchaseOrderColumns+`
	`, poID, tenantID, status))
}

const purchaseOrderItemColumns = `id, tenant_id, purchase_order_id, requisition_item_id, catalog_item_id,
	quantity, unit_cost_minor_units, currency, line_total_minor_units, created_at`

func scanPurchaseOrderItem(row pgx.Row) (*PurchaseOrderItem, error) {
	var poi PurchaseOrderItem
	err := row.Scan(
		&poi.ID, &poi.TenantID, &poi.PurchaseOrderID, &poi.RequisitionItemID, &poi.CatalogItemID,
		&poi.Quantity, &poi.UnitCostMinorUnits, &poi.Currency, &poi.LineTotalMinorUnits, &poi.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPurchaseOrderNotFound
	}
	if err != nil {
		return nil, err
	}
	return &poi, nil
}

func (st *Store) InsertPurchaseOrderItem(ctx context.Context, q querier, poi *PurchaseOrderItem) error {
	poi.ID = newID("poi")
	_, err := q.Exec(ctx, `
		INSERT INTO purchase_order_items (id, tenant_id, purchase_order_id, requisition_item_id, catalog_item_id,
			quantity, unit_cost_minor_units, currency, line_total_minor_units)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, poi.ID, poi.TenantID, poi.PurchaseOrderID, poi.RequisitionItemID, poi.CatalogItemID,
		poi.Quantity, poi.UnitCostMinorUnits, poi.Currency, poi.LineTotalMinorUnits)
	return err
}

func (st *Store) ListPurchaseOrderItems(ctx context.Context, tenantID, poID string) ([]PurchaseOrderItem, error) {
	rows, err := st.pool.Query(ctx, `
		SELECT `+purchaseOrderItemColumns+`
		FROM purchase_order_items
		WHERE tenant_id = $1 AND purchase_order_id = $2
		ORDER BY created_at ASC
	`, tenantID, poID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PurchaseOrderItem
	for rows.Next() {
		poi, err := scanPurchaseOrderItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *poi)
	}
	return out, rows.Err()
}

const goodsReceiptColumns = `id, tenant_id, purchase_order_id, received_by, status,
	condition, condition_notes, evidence_ref, received_at,
	version, created_at, updated_at`

func scanGoodsReceipt(row pgx.Row) (*GoodsReceipt, error) {
	var gr GoodsReceipt
	err := row.Scan(
		&gr.ID, &gr.TenantID, &gr.PurchaseOrderID, &gr.ReceivedBy, &gr.Status,
		&gr.Condition, &gr.ConditionNotes, &gr.EvidenceRef, &gr.ReceivedAt,
		&gr.Version, &gr.CreatedAt, &gr.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrGoodsReceiptNotFound
	}
	if err != nil {
		return nil, err
	}
	return &gr, nil
}

func (st *Store) InsertGoodsReceipt(ctx context.Context, q querier, gr *GoodsReceipt) error {
	gr.ID = newID("rcp")
	_, err := q.Exec(ctx, `
		INSERT INTO goods_receipts (id, tenant_id, purchase_order_id, received_by, status,
			condition, condition_notes, evidence_ref, received_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, gr.ID, gr.TenantID, gr.PurchaseOrderID, gr.ReceivedBy, gr.Status,
		gr.Condition, gr.ConditionNotes, gr.EvidenceRef, gr.ReceivedAt)
	return err
}

func (st *Store) GetGoodsReceipt(ctx context.Context, tenantID, receiptID string) (*GoodsReceipt, error) {
	return scanGoodsReceipt(st.pool.QueryRow(ctx, `
		SELECT `+goodsReceiptColumns+`
		FROM goods_receipts
		WHERE id = $1 AND tenant_id = $2
	`, receiptID, tenantID))
}

func (st *Store) ListGoodsReceipts(ctx context.Context, tenantID, poID string) ([]GoodsReceipt, error) {
	query := `SELECT ` + goodsReceiptColumns + ` FROM goods_receipts WHERE tenant_id = $1`
	var args []any
	args = append(args, tenantID)
	if poID != "" {
		query += ` AND purchase_order_id = $2`
		args = append(args, poID)
	}
	query += ` ORDER BY created_at DESC`

	rows, err := st.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []GoodsReceipt
	for rows.Next() {
		gr, err := scanGoodsReceipt(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *gr)
	}
	return out, rows.Err()
}

const goodsReceiptItemColumns = `id, tenant_id, goods_receipt_id, purchase_order_item_id, catalog_item_id,
	quantity_ordered, quantity_received, created_at`

func scanGoodsReceiptItem(row pgx.Row) (*GoodsReceiptItem, error) {
	var gri GoodsReceiptItem
	err := row.Scan(
		&gri.ID, &gri.TenantID, &gri.GoodsReceiptID, &gri.PurchaseOrderItemID, &gri.CatalogItemID,
		&gri.QuantityOrdered, &gri.QuantityReceived, &gri.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrGoodsReceiptNotFound
	}
	if err != nil {
		return nil, err
	}
	return &gri, nil
}

func (st *Store) InsertGoodsReceiptItem(ctx context.Context, q querier, gri *GoodsReceiptItem) error {
	gri.ID = newID("gri")
	_, err := q.Exec(ctx, `
		INSERT INTO goods_receipt_items (id, tenant_id, goods_receipt_id, purchase_order_item_id, catalog_item_id,
			quantity_ordered, quantity_received)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, gri.ID, gri.TenantID, gri.GoodsReceiptID, gri.PurchaseOrderItemID, gri.CatalogItemID,
		gri.QuantityOrdered, gri.QuantityReceived)
	return err
}

func (st *Store) ListGoodsReceiptItems(ctx context.Context, tenantID, receiptID string) ([]GoodsReceiptItem, error) {
	rows, err := st.pool.Query(ctx, `
		SELECT `+goodsReceiptItemColumns+`
		FROM goods_receipt_items
		WHERE tenant_id = $1 AND goods_receipt_id = $2
		ORDER BY created_at ASC
	`, tenantID, receiptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []GoodsReceiptItem
	for rows.Next() {
		gri, err := scanGoodsReceiptItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *gri)
	}
	return out, rows.Err()
}

const rebateColumns = `id, tenant_id, supplier_id, purchase_order_id, description,
	amount_minor_units, currency, status, offered_at, settled_at,
	version, created_at, updated_at`

func scanRebate(row pgx.Row) (*SupplierRebate, error) {
	var r SupplierRebate
	err := row.Scan(
		&r.ID, &r.TenantID, &r.SupplierID, &r.PurchaseOrderID, &r.Description,
		&r.AmountMinorUnits, &r.Currency, &r.Status, &r.OfferedAt, &r.SettledAt,
		&r.Version, &r.CreatedAt, &r.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRebateNotFound
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (st *Store) InsertRebate(ctx context.Context, q querier, r *SupplierRebate) error {
	r.ID = newID("reb")
	_, err := q.Exec(ctx, `
		INSERT INTO supplier_rebates (id, tenant_id, supplier_id, purchase_order_id, description,
			amount_minor_units, currency, status, offered_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, r.ID, r.TenantID, r.SupplierID, r.PurchaseOrderID, r.Description,
		r.AmountMinorUnits, r.Currency, r.Status, r.OfferedAt)
	return err
}

func (st *Store) GetRebate(ctx context.Context, tenantID, rebateID string) (*SupplierRebate, error) {
	return scanRebate(st.pool.QueryRow(ctx, `
		SELECT `+rebateColumns+`
		FROM supplier_rebates
		WHERE id = $1 AND tenant_id = $2
	`, rebateID, tenantID))
}

func (st *Store) ListRebates(ctx context.Context, tenantID, supplierID string) ([]SupplierRebate, error) {
	query := `SELECT ` + rebateColumns + ` FROM supplier_rebates WHERE tenant_id = $1`
	var args []any
	args = append(args, tenantID)
	if supplierID != "" {
		query += ` AND supplier_id = $2`
		args = append(args, supplierID)
	}
	query += ` ORDER BY created_at DESC`

	rows, err := st.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SupplierRebate
	for rows.Next() {
		r, err := scanRebate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

func (st *Store) UpdateRebateStatus(ctx context.Context, q querier, tenantID, rebateID, status string) (*SupplierRebate, error) {
	return scanRebate(q.QueryRow(ctx, `
		UPDATE supplier_rebates
		SET status = $3, settled_at = CASE WHEN $3 = 'settled' THEN NOW() ELSE settled_at END,
		    updated_at = NOW(), version = version + 1
		WHERE id = $1 AND tenant_id = $2
		RETURNING `+rebateColumns+`
	`, rebateID, tenantID, status))
}

func nullTime(t *time.Time) *time.Time {
	if t == nil || t.IsZero() {
		return nil
	}
	return t
}

func marshalJSON(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("{}")
	}
	return data
}
