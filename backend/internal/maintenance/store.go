package maintenance

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

const requestColumns = `id, tenant_id, property_id, title, category, priority, risk_level, status,
	reported_by, triaged_by, triaged_at, estimate_id, notes, version, created_at, updated_at`

func scanRequest(row pgx.Row) (*MaintenanceRequest, error) {
	var r MaintenanceRequest
	err := row.Scan(
		&r.ID, &r.TenantID, &r.PropertyID, &r.Title, &r.Category, &r.Priority, &r.RiskLevel, &r.Status,
		&r.ReportedBy, &r.TriagedBy, &r.TriagedAt, &r.EstimateID, &r.Notes, &r.Version, &r.CreatedAt, &r.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRequestNotFound
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (st *Store) InsertRequest(ctx context.Context, q querier, r *MaintenanceRequest) error {
	r.ID = newID("mtn")
	_, err := q.Exec(ctx, `
		INSERT INTO maintenance_requests (id, tenant_id, property_id, title, category, priority, risk_level,
			status, reported_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, r.ID, r.TenantID, r.PropertyID, r.Title, r.Category, r.Priority, r.RiskLevel, r.Status, r.ReportedBy)
	return err
}

func (st *Store) GetRequest(ctx context.Context, tenantID, requestID string) (*MaintenanceRequest, error) {
	return scanRequest(st.pool.QueryRow(ctx, `
		SELECT `+requestColumns+`
		FROM maintenance_requests
		WHERE id = $1 AND tenant_id = $2
	`, requestID, tenantID))
}

func (st *Store) ListRequests(ctx context.Context, tenantID, propertyID string) ([]MaintenanceRequest, error) {
	query := `SELECT ` + requestColumns + ` FROM maintenance_requests WHERE tenant_id = $1`
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

	var out []MaintenanceRequest
	for rows.Next() {
		r, err := scanRequest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

func (st *Store) TriageRequest(ctx context.Context, q querier, tenantID, requestID string, params TriageRequestParams, triagedBy string, triagedAt time.Time) (*MaintenanceRequest, error) {
	return scanRequest(q.QueryRow(ctx, `
		UPDATE maintenance_requests
		SET category = $3, priority = $4, risk_level = $5, notes = $6,
		    status = $7, triaged_by = $8, triaged_at = $9,
		    updated_at = NOW(), version = version + 1
		WHERE id = $1 AND tenant_id = $2 AND status = 'reported'
		RETURNING `+requestColumns+`
	`, requestID, tenantID, params.Category, params.Priority, params.RiskLevel, params.Notes,
		RequestStatusTriaged, triagedBy, triagedAt))
}

func (st *Store) UpdateRequestStatus(ctx context.Context, q querier, tenantID, requestID, status, estimateID string) (*MaintenanceRequest, error) {
	return scanRequest(q.QueryRow(ctx, `
		UPDATE maintenance_requests
		SET status = $3, estimate_id = CASE WHEN $4 = '' THEN estimate_id ELSE $4 END,
		    updated_at = NOW(), version = version + 1
		WHERE id = $1 AND tenant_id = $2
		RETURNING `+requestColumns+`
	`, requestID, tenantID, status, estimateID))
}

// SetRequestEstimate points a request at its current estimate without changing
// the request status. The linked estimate is what the start gate evaluates.
func (st *Store) SetRequestEstimate(ctx context.Context, q querier, tenantID, requestID, estimateID string) error {
	_, err := q.Exec(ctx, `
		UPDATE maintenance_requests
		SET estimate_id = $3, updated_at = NOW(), version = version + 1
		WHERE id = $1 AND tenant_id = $2
	`, requestID, tenantID, estimateID)
	return err
}

const estimateColumns = `id, tenant_id, request_id, property_id, prepared_by,
	amount_minor_units, currency, scope, status, submitted_at,
	approved_by, approved_at, rejected_by, rejected_at,
	version, created_at, updated_at`

func scanEstimate(row pgx.Row) (*MaintenanceEstimate, error) {
	var e MaintenanceEstimate
	err := row.Scan(
		&e.ID, &e.TenantID, &e.RequestID, &e.PropertyID, &e.PreparedBy,
		&e.AmountMinorUnits, &e.Currency, &e.Scope, &e.Status, &e.SubmittedAt,
		&e.ApprovedBy, &e.ApprovedAt, &e.RejectedBy, &e.RejectedAt,
		&e.Version, &e.CreatedAt, &e.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrEstimateNotFound
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (st *Store) InsertEstimate(ctx context.Context, q querier, e *MaintenanceEstimate) error {
	e.ID = newID("mte")
	_, err := q.Exec(ctx, `
		INSERT INTO maintenance_estimates (id, tenant_id, request_id, property_id, prepared_by,
			amount_minor_units, currency, scope, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, e.ID, e.TenantID, e.RequestID, e.PropertyID, e.PreparedBy,
		e.AmountMinorUnits, e.Currency, e.Scope, e.Status)
	return err
}

func (st *Store) GetEstimate(ctx context.Context, tenantID, estimateID string) (*MaintenanceEstimate, error) {
	return scanEstimate(st.pool.QueryRow(ctx, `
		SELECT `+estimateColumns+`
		FROM maintenance_estimates
		WHERE id = $1 AND tenant_id = $2
	`, estimateID, tenantID))
}

func (st *Store) ListEstimates(ctx context.Context, tenantID, requestID string) ([]MaintenanceEstimate, error) {
	query := `SELECT ` + estimateColumns + ` FROM maintenance_estimates WHERE tenant_id = $1`
	var args []any
	args = append(args, tenantID)
	if requestID != "" {
		query += ` AND request_id = $2`
		args = append(args, requestID)
	}
	query += ` ORDER BY created_at DESC`

	rows, err := st.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []MaintenanceEstimate
	for rows.Next() {
		e, err := scanEstimate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

func (st *Store) SubmitEstimate(ctx context.Context, q querier, tenantID, estimateID string, submittedAt time.Time) (*MaintenanceEstimate, error) {
	return scanEstimate(q.QueryRow(ctx, `
		UPDATE maintenance_estimates
		SET status = $3, submitted_at = $4, updated_at = NOW(), version = version + 1
		WHERE id = $1 AND tenant_id = $2 AND status = 'draft'
		RETURNING `+estimateColumns+`
	`, estimateID, tenantID, EstimateStatusPendingApproval, submittedAt))
}

func (st *Store) DecideEstimate(ctx context.Context, q querier, tenantID, estimateID string, params DecideEstimateParams) (*MaintenanceEstimate, error) {
	var status, actorCol, atCol string
	switch params.Decision {
	case ApprovalDecisionApproved:
		status = EstimateStatusApproved
		actorCol = "approved_by"
		atCol = "approved_at"
	case ApprovalDecisionRejected:
		status = EstimateStatusRejected
		actorCol = "rejected_by"
		atCol = "rejected_at"
	default:
		return nil, fmt.Errorf("%w: decision %q", ErrInvalidApproval, params.Decision)
	}

	return scanEstimate(q.QueryRow(ctx, `
		UPDATE maintenance_estimates
		SET status = $3, `+actorCol+` = $4, `+atCol+` = NOW(), updated_at = NOW(), version = version + 1
		WHERE id = $1 AND tenant_id = $2 AND status = 'pending_approval'
		RETURNING `+estimateColumns+`
	`, estimateID, tenantID, status, params.ActorID))
}

const approvalColumns = `id, tenant_id, request_id, estimate_id, actor_id, decision, reason, is_ai_actor, created_at`

func scanApproval(row pgx.Row) (*MaintenanceApproval, error) {
	var a MaintenanceApproval
	err := row.Scan(
		&a.ID, &a.TenantID, &a.RequestID, &a.EstimateID, &a.ActorID, &a.Decision, &a.Reason, &a.IsAIActor, &a.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrApprovalNotFound
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (st *Store) InsertApproval(ctx context.Context, q querier, a *MaintenanceApproval) error {
	a.ID = newID("mta")
	_, err := q.Exec(ctx, `
		INSERT INTO maintenance_approvals (id, tenant_id, request_id, estimate_id, actor_id, decision, reason, is_ai_actor)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, a.ID, a.TenantID, a.RequestID, a.EstimateID, a.ActorID, a.Decision, a.Reason, a.IsAIActor)
	return err
}

func (st *Store) ListApprovals(ctx context.Context, tenantID, requestID string) ([]MaintenanceApproval, error) {
	query := `SELECT ` + approvalColumns + ` FROM maintenance_approvals WHERE tenant_id = $1`
	var args []any
	args = append(args, tenantID)
	if requestID != "" {
		query += ` AND request_id = $2`
		args = append(args, requestID)
	}
	query += ` ORDER BY created_at DESC`

	rows, err := st.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []MaintenanceApproval
	for rows.Next() {
		a, err := scanApproval(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

const workOrderColumns = `id, tenant_id, request_id, estimate_id, property_id, vendor_id, scope, risk_level, status,
	assigned_by, assigned_at, started_at, completed_by, completed_at, completion_evidence_ref,
	verified_by, verified_at, version, created_at, updated_at`

func scanWorkOrder(row pgx.Row) (*VendorWorkOrder, error) {
	var wo VendorWorkOrder
	err := row.Scan(
		&wo.ID, &wo.TenantID, &wo.RequestID, &wo.EstimateID, &wo.PropertyID, &wo.VendorID, &wo.Scope, &wo.RiskLevel, &wo.Status,
		&wo.AssignedBy, &wo.AssignedAt, &wo.StartedAt, &wo.CompletedBy, &wo.CompletedAt, &wo.CompletionEvidenceRef,
		&wo.VerifiedBy, &wo.VerifiedAt, &wo.Version, &wo.CreatedAt, &wo.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrWorkOrderNotFound
	}
	if err != nil {
		return nil, err
	}
	return &wo, nil
}

func (st *Store) InsertWorkOrder(ctx context.Context, q querier, wo *VendorWorkOrder) error {
	wo.ID = newID("mwo")
	_, err := q.Exec(ctx, `
		INSERT INTO vendor_work_orders (id, tenant_id, request_id, estimate_id, property_id, vendor_id, scope, risk_level, status, assigned_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, wo.ID, wo.TenantID, wo.RequestID, wo.EstimateID, wo.PropertyID, wo.VendorID, wo.Scope, wo.RiskLevel, wo.Status, wo.AssignedBy)
	return err
}

func (st *Store) GetWorkOrder(ctx context.Context, tenantID, workOrderID string) (*VendorWorkOrder, error) {
	return scanWorkOrder(st.pool.QueryRow(ctx, `
		SELECT `+workOrderColumns+`
		FROM vendor_work_orders
		WHERE id = $1 AND tenant_id = $2
	`, workOrderID, tenantID))
}

func (st *Store) GetVendorWorkOrder(ctx context.Context, tenantID, vendorID, workOrderID string) (*VendorWorkOrder, error) {
	wo, err := scanWorkOrder(st.pool.QueryRow(ctx, `
		SELECT `+workOrderColumns+`
		FROM vendor_work_orders
		WHERE id = $1 AND tenant_id = $2 AND vendor_id = $3
	`, workOrderID, tenantID, vendorID))
	if errors.Is(err, ErrWorkOrderNotFound) {
		return nil, ErrVendorScopeDenied
	}
	return wo, err
}

func (st *Store) ListWorkOrders(ctx context.Context, tenantID, requestID string) ([]VendorWorkOrder, error) {
	query := `SELECT ` + workOrderColumns + ` FROM vendor_work_orders WHERE tenant_id = $1`
	var args []any
	args = append(args, tenantID)
	if requestID != "" {
		query += ` AND request_id = $2`
		args = append(args, requestID)
	}
	query += ` ORDER BY created_at DESC`

	rows, err := st.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []VendorWorkOrder
	for rows.Next() {
		wo, err := scanWorkOrder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *wo)
	}
	return out, rows.Err()
}

// ListVendorWorkOrders returns only work orders assigned to the given vendor.
// A vendor never sees work scoped to another vendor.
func (st *Store) ListVendorWorkOrders(ctx context.Context, tenantID, vendorID string) ([]VendorWorkOrder, error) {
	rows, err := st.pool.Query(ctx, `
		SELECT `+workOrderColumns+`
		FROM vendor_work_orders
		WHERE tenant_id = $1 AND vendor_id = $2
		ORDER BY created_at DESC
	`, tenantID, vendorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []VendorWorkOrder
	for rows.Next() {
		wo, err := scanWorkOrder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *wo)
	}
	return out, rows.Err()
}

func (st *Store) UpdateWorkOrderStatus(ctx context.Context, q querier, tenantID, workOrderID string, status string) (*VendorWorkOrder, error) {
	return scanWorkOrder(q.QueryRow(ctx, `
		UPDATE vendor_work_orders
		SET status = $3, updated_at = NOW(), version = version + 1
		WHERE id = $1 AND tenant_id = $2
		RETURNING `+workOrderColumns+`
	`, workOrderID, tenantID, status))
}

func (st *Store) StartWorkOrder(ctx context.Context, q querier, tenantID, workOrderID string, startedAt time.Time) (*VendorWorkOrder, error) {
	return scanWorkOrder(q.QueryRow(ctx, `
		UPDATE vendor_work_orders
		SET status = $3, started_at = $4, updated_at = NOW(), version = version + 1
		WHERE id = $1 AND tenant_id = $2 AND status = 'assigned'
		RETURNING `+workOrderColumns+`
	`, workOrderID, tenantID, WorkOrderStatusInProgress, startedAt))
}

func (st *Store) CompleteWorkOrder(ctx context.Context, q querier, tenantID, workOrderID string, params CompleteWorkOrderParams, completedAt time.Time) (*VendorWorkOrder, error) {
	return scanWorkOrder(q.QueryRow(ctx, `
		UPDATE vendor_work_orders
		SET status = $3, completed_by = $4, completed_at = $5, completion_evidence_ref = $6,
		    updated_at = NOW(), version = version + 1
		WHERE id = $1 AND tenant_id = $2 AND status = 'in_progress'
		RETURNING `+workOrderColumns+`
	`, workOrderID, tenantID, WorkOrderStatusCompleted, params.CompletedBy, completedAt, params.CompletionEvidenceRef))
}

func (st *Store) VerifyWorkOrder(ctx context.Context, q querier, tenantID, workOrderID, verifierID string, verifiedAt time.Time) (*VendorWorkOrder, error) {
	return scanWorkOrder(q.QueryRow(ctx, `
		UPDATE vendor_work_orders
		SET status = $3, verified_by = $4, verified_at = $5,
		    updated_at = NOW(), version = version + 1
		WHERE id = $1 AND tenant_id = $2 AND status = 'completed'
		RETURNING `+workOrderColumns+`
	`, workOrderID, tenantID, WorkOrderStatusVerified, verifierID, verifiedAt))
}

const warrantyColumns = `id, tenant_id, work_order_id, property_id, vendor_id, provider, coverage, expires_at, status, recorded_by, created_at`

func scanWarranty(row pgx.Row) (*WarrantyRecord, error) {
	var w WarrantyRecord
	err := row.Scan(
		&w.ID, &w.TenantID, &w.WorkOrderID, &w.PropertyID, &w.VendorID, &w.Provider, &w.Coverage, &w.ExpiresAt, &w.Status, &w.RecordedBy, &w.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrWarrantyNotFound
	}
	if err != nil {
		return nil, err
	}
	return &w, nil
}

func (st *Store) InsertWarranty(ctx context.Context, q querier, w *WarrantyRecord) error {
	w.ID = newID("wty")
	_, err := q.Exec(ctx, `
		INSERT INTO warranty_records (id, tenant_id, work_order_id, property_id, vendor_id, provider, coverage, expires_at, status, recorded_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, w.ID, w.TenantID, w.WorkOrderID, w.PropertyID, w.VendorID, w.Provider, w.Coverage, w.ExpiresAt, w.Status, w.RecordedBy)
	return err
}

func (st *Store) GetWarranty(ctx context.Context, tenantID, warrantyID string) (*WarrantyRecord, error) {
	return scanWarranty(st.pool.QueryRow(ctx, `
		SELECT `+warrantyColumns+`
		FROM warranty_records
		WHERE id = $1 AND tenant_id = $2
	`, warrantyID, tenantID))
}

func (st *Store) ListWarranties(ctx context.Context, tenantID, propertyID string) ([]WarrantyRecord, error) {
	query := `SELECT ` + warrantyColumns + ` FROM warranty_records WHERE tenant_id = $1`
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

	var out []WarrantyRecord
	for rows.Next() {
		w, err := scanWarranty(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *w)
	}
	return out, rows.Err()
}

func marshalJSON(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("{}")
	}
	return data
}
