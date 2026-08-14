package billing

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

const chargeColumns = `id, tenant_id, property_id, charge_type,
	amount_minor_units, currency, reason, data,
	contract_rule_id, evidence_id, ticket_id, order_id, approval_id,
	idempotency_key, status, version, created_at, updated_at`

func scanCharge(row pgx.Row) (*Charge, error) {
	var c Charge
	err := row.Scan(
		&c.ID, &c.TenantID, &c.PropertyID, &c.ChargeType,
		&c.AmountMinorUnits, &c.Currency, &c.Reason, &c.Data,
		&c.ContractRuleID, &c.EvidenceID, &c.TicketID, &c.OrderID, &c.ApprovalID,
		&c.IdempotencyKey, &c.Status, &c.Version, &c.CreatedAt, &c.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrChargeNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (st *Store) GetChargeByIdempotencyKey(ctx context.Context, q querier, tenantID, idempotencyKey string) (*Charge, error) {
	return scanCharge(q.QueryRow(ctx, `
		SELECT `+chargeColumns+`
		FROM charges
		WHERE tenant_id = $1 AND idempotency_key = $2
	`, tenantID, idempotencyKey))
}

func (st *Store) InsertCharge(ctx context.Context, q querier, c *Charge) error {
	c.ID = newID("chg")
	_, err := q.Exec(ctx, `
		INSERT INTO charges (id, tenant_id, property_id, charge_type,
			amount_minor_units, currency, reason, data,
			contract_rule_id, evidence_id, ticket_id, order_id, approval_id,
			idempotency_key, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`, c.ID, c.TenantID, c.PropertyID, c.ChargeType,
		c.AmountMinorUnits, c.Currency, c.Reason, c.Data,
		c.ContractRuleID, c.EvidenceID, c.TicketID, c.OrderID, c.ApprovalID,
		c.IdempotencyKey, c.Status)
	return err
}

func (st *Store) GetCharge(ctx context.Context, tenantID, chargeID string) (*Charge, error) {
	return scanCharge(st.pool.QueryRow(ctx, `
		SELECT `+chargeColumns+`
		FROM charges
		WHERE id = $1 AND tenant_id = $2
	`, chargeID, tenantID))
}

func (st *Store) ListCharges(ctx context.Context, tenantID, propertyID string) ([]Charge, error) {
	query := `SELECT ` + chargeColumns + ` FROM charges WHERE tenant_id = $1`
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

	var out []Charge
	for rows.Next() {
		c, err := scanCharge(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func (st *Store) UpdateChargeStatus(ctx context.Context, q querier, tenantID, chargeID, status string) (*Charge, error) {
	return scanCharge(q.QueryRow(ctx, `
		UPDATE charges
		SET status = $3, updated_at = NOW(), version = version + 1
		WHERE id = $1 AND tenant_id = $2
		RETURNING `+chargeColumns+`
	`, chargeID, tenantID, status))
}

const invoiceColumns = `id, tenant_id, property_id, period_start, period_end,
	total_minor_units, currency, status, idempotency_key,
	version, created_at, updated_at`

func scanInvoice(row pgx.Row) (*Invoice, error) {
	var inv Invoice
	err := row.Scan(
		&inv.ID, &inv.TenantID, &inv.PropertyID, &inv.PeriodStart, &inv.PeriodEnd,
		&inv.TotalMinorUnits, &inv.Currency, &inv.Status, &inv.IdempotencyKey,
		&inv.Version, &inv.CreatedAt, &inv.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvoiceNotFound
	}
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

func (st *Store) GetInvoiceByIdempotencyKey(ctx context.Context, q querier, tenantID, idempotencyKey string) (*Invoice, error) {
	return scanInvoice(q.QueryRow(ctx, `
		SELECT `+invoiceColumns+`
		FROM invoices
		WHERE tenant_id = $1 AND idempotency_key = $2
	`, tenantID, idempotencyKey))
}

func (st *Store) InsertInvoice(ctx context.Context, q querier, inv *Invoice) error {
	inv.ID = newID("inv")
	_, err := q.Exec(ctx, `
		INSERT INTO invoices (id, tenant_id, property_id, period_start, period_end,
			total_minor_units, currency, status, idempotency_key)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, inv.ID, inv.TenantID, inv.PropertyID, inv.PeriodStart, inv.PeriodEnd,
		inv.TotalMinorUnits, inv.Currency, inv.Status, inv.IdempotencyKey)
	return err
}

func (st *Store) GetInvoice(ctx context.Context, tenantID, invoiceID string) (*Invoice, error) {
	return scanInvoice(st.pool.QueryRow(ctx, `
		SELECT `+invoiceColumns+`
		FROM invoices
		WHERE id = $1 AND tenant_id = $2
	`, invoiceID, tenantID))
}

func (st *Store) ListInvoices(ctx context.Context, tenantID, propertyID string) ([]Invoice, error) {
	query := `SELECT ` + invoiceColumns + ` FROM invoices WHERE tenant_id = $1`
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

	var out []Invoice
	for rows.Next() {
		inv, err := scanInvoice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *inv)
	}
	return out, rows.Err()
}

func (st *Store) UpdateInvoiceStatus(ctx context.Context, q querier, tenantID, invoiceID, status string) (*Invoice, error) {
	return scanInvoice(q.QueryRow(ctx, `
		UPDATE invoices
		SET status = $3, updated_at = NOW(), version = version + 1
		WHERE id = $1 AND tenant_id = $2
		RETURNING `+invoiceColumns+`
	`, invoiceID, tenantID, status))
}

const invoiceLineColumns = `id, invoice_id, tenant_id, charge_type, description,
	amount_minor_units, currency, contract_rule_id, ticket_id,
	order_id, adjustment_id, created_at`

func scanInvoiceLine(row pgx.Row) (*InvoiceLine, error) {
	var line InvoiceLine
	err := row.Scan(
		&line.ID, &line.InvoiceID, &line.TenantID, &line.ChargeType, &line.Description,
		&line.AmountMinorUnits, &line.Currency, &line.ContractRuleID, &line.TicketID,
		&line.OrderID, &line.AdjustmentID, &line.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("invoice line not found")
	}
	if err != nil {
		return nil, err
	}
	return &line, nil
}

func (st *Store) InsertInvoiceLine(ctx context.Context, q querier, line *InvoiceLine) error {
	line.ID = newID("ivl")
	_, err := q.Exec(ctx, `
		INSERT INTO invoice_lines (id, invoice_id, tenant_id, charge_type, description,
			amount_minor_units, currency, contract_rule_id, ticket_id,
			order_id, adjustment_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, line.ID, line.InvoiceID, line.TenantID, line.ChargeType, line.Description,
		line.AmountMinorUnits, line.Currency, line.ContractRuleID, line.TicketID,
		line.OrderID, line.AdjustmentID)
	return err
}

func (st *Store) ListInvoiceLines(ctx context.Context, tenantID, invoiceID string) ([]InvoiceLine, error) {
	rows, err := st.pool.Query(ctx, `
		SELECT `+invoiceLineColumns+`
		FROM invoice_lines
		WHERE tenant_id = $1 AND invoice_id = $2
		ORDER BY created_at ASC
	`, tenantID, invoiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []InvoiceLine
	for rows.Next() {
		line, err := scanInvoiceLine(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *line)
	}
	return out, rows.Err()
}

const creditColumns = `id, tenant_id, property_id, credit_type,
	amount_minor_units, currency, reason, original_entry_id, original_entry_type, data,
	idempotency_key, status, version, created_at, updated_at`

func scanCredit(row pgx.Row) (*Credit, error) {
	var c Credit
	err := row.Scan(
		&c.ID, &c.TenantID, &c.PropertyID, &c.CreditType,
		&c.AmountMinorUnits, &c.Currency, &c.Reason, &c.OriginalEntryID, &c.OriginalEntryType, &c.Data,
		&c.IdempotencyKey, &c.Status, &c.Version, &c.CreatedAt, &c.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrCreditNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (st *Store) GetCreditByIdempotencyKey(ctx context.Context, q querier, tenantID, idempotencyKey string) (*Credit, error) {
	return scanCredit(q.QueryRow(ctx, `
		SELECT `+creditColumns+`
		FROM credits
		WHERE tenant_id = $1 AND idempotency_key = $2
	`, tenantID, idempotencyKey))
}

func (st *Store) InsertCredit(ctx context.Context, q querier, c *Credit) error {
	c.ID = newID("crd")
	_, err := q.Exec(ctx, `
		INSERT INTO credits (id, tenant_id, property_id, credit_type,
			amount_minor_units, currency, reason, original_entry_id, original_entry_type, data,
			idempotency_key, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, c.ID, c.TenantID, c.PropertyID, c.CreditType,
		c.AmountMinorUnits, c.Currency, c.Reason, c.OriginalEntryID, c.OriginalEntryType, c.Data,
		c.IdempotencyKey, c.Status)
	return err
}

func (st *Store) GetCredit(ctx context.Context, tenantID, creditID string) (*Credit, error) {
	return scanCredit(st.pool.QueryRow(ctx, `
		SELECT `+creditColumns+`
		FROM credits
		WHERE id = $1 AND tenant_id = $2
	`, creditID, tenantID))
}

func (st *Store) ListCredits(ctx context.Context, tenantID, propertyID string) ([]Credit, error) {
	query := `SELECT ` + creditColumns + ` FROM credits WHERE tenant_id = $1`
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

	var out []Credit
	for rows.Next() {
		c, err := scanCredit(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

const subledgerColumns = `id, tenant_id, property_id, entry_type,
	amount_minor_units, currency, reference_type, reference_id,
	description, created_at`

func scanSubledgerEntry(row pgx.Row) (*OperationalSubledgerEntry, error) {
	var e OperationalSubledgerEntry
	err := row.Scan(
		&e.ID, &e.TenantID, &e.PropertyID, &e.EntryType,
		&e.AmountMinorUnits, &e.Currency, &e.ReferenceType, &e.ReferenceID,
		&e.Description, &e.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSubledgerEntryNotFound
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (st *Store) InsertSubledgerEntry(ctx context.Context, q querier, entry *OperationalSubledgerEntry) error {
	entry.ID = newID("sled")
	_, err := q.Exec(ctx, `
		INSERT INTO operational_subledger_entries
			(id, tenant_id, property_id, entry_type, amount_minor_units,
			 currency, reference_type, reference_id, description)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, entry.ID, entry.TenantID, entry.PropertyID, entry.EntryType,
		entry.AmountMinorUnits, entry.Currency, entry.ReferenceType, entry.ReferenceID,
		entry.Description)
	return err
}

func (st *Store) ListSubledgerEntries(ctx context.Context, tenantID, propertyID string) ([]OperationalSubledgerEntry, error) {
	query := `SELECT ` + subledgerColumns + ` FROM operational_subledger_entries WHERE tenant_id = $1`
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

	var out []OperationalSubledgerEntry
	for rows.Next() {
		e, err := scanSubledgerEntry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

func (st *Store) ListSubledgerEntriesByPeriod(ctx context.Context, tenantID string, periodStart, periodEnd *time.Time) ([]OperationalSubledgerEntry, error) {
	query := `SELECT ` + subledgerColumns + ` FROM operational_subledger_entries WHERE tenant_id = $1`
	var args []any
	args = append(args, tenantID)
	argIdx := 2
	if periodStart != nil {
		query += fmt.Sprintf(` AND created_at >= $%d`, argIdx)
		args = append(args, *periodStart)
		argIdx++
	}
	if periodEnd != nil {
		query += fmt.Sprintf(` AND created_at <= $%d`, argIdx)
		args = append(args, *periodEnd)
		argIdx++
	}
	query += ` ORDER BY created_at ASC`

	rows, err := st.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []OperationalSubledgerEntry
	for rows.Next() {
		e, err := scanSubledgerEntry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

const exportColumns = `id, tenant_id, period_start, period_end,
	format, status, requested_by, result_ref,
	version, created_at, updated_at`

func scanAccountingExport(row pgx.Row) (*AccountingExport, error) {
	var e AccountingExport
	err := row.Scan(
		&e.ID, &e.TenantID, &e.PeriodStart, &e.PeriodEnd,
		&e.Format, &e.Status, &e.RequestedBy, &e.ResultRef,
		&e.Version, &e.CreatedAt, &e.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrAccountingExportNotFound
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (st *Store) InsertAccountingExport(ctx context.Context, q querier, e *AccountingExport) error {
	e.ID = newID("aex")
	_, err := q.Exec(ctx, `
		INSERT INTO accounting_exports
			(id, tenant_id, period_start, period_end, format, status, requested_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, e.ID, e.TenantID, e.PeriodStart, e.PeriodEnd, e.Format, e.Status, e.RequestedBy)
	return err
}

func (st *Store) GetAccountingExport(ctx context.Context, tenantID, exportID string) (*AccountingExport, error) {
	return scanAccountingExport(st.pool.QueryRow(ctx, `
		SELECT `+exportColumns+`
		FROM accounting_exports
		WHERE id = $1 AND tenant_id = $2
	`, exportID, tenantID))
}

func (st *Store) ListAccountingExports(ctx context.Context, tenantID string) ([]AccountingExport, error) {
	rows, err := st.pool.Query(ctx, `
		SELECT `+exportColumns+`
		FROM accounting_exports
		WHERE tenant_id = $1
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AccountingExport
	for rows.Next() {
		e, err := scanAccountingExport(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

const approvalColumns = `id, tenant_id, request_id, approver_id, decision, reason, created_at`

func scanFinancialApproval(row pgx.Row) (*FinancialApproval, error) {
	var a FinancialApproval
	err := row.Scan(
		&a.ID, &a.TenantID, &a.RequestID, &a.ApproverID, &a.Decision, &a.Reason, &a.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrFinancialApprovalNotFound
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (st *Store) InsertFinancialApproval(ctx context.Context, q querier, a *FinancialApproval) error {
	a.ID = newID("fap")
	_, err := q.Exec(ctx, `
		INSERT INTO financial_approvals
			(id, tenant_id, request_id, approver_id, decision, reason)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, a.ID, a.TenantID, a.RequestID, a.ApproverID, a.Decision, a.Reason)
	return err
}

func (st *Store) GetFinancialApproval(ctx context.Context, tenantID, approvalID string) (*FinancialApproval, error) {
	return scanFinancialApproval(st.pool.QueryRow(ctx, `
		SELECT `+approvalColumns+`
		FROM financial_approvals
		WHERE id = $1 AND tenant_id = $2
	`, approvalID, tenantID))
}

func (st *Store) ListFinancialApprovals(ctx context.Context, tenantID, requestID string) ([]FinancialApproval, error) {
	query := `SELECT ` + approvalColumns + ` FROM financial_approvals WHERE tenant_id = $1`
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

	var out []FinancialApproval
	for rows.Next() {
		a, err := scanFinancialApproval(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
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

const requestColumns = `id, tenant_id, request_type, property_id, status,
	created_by, submitted_by, approved_by, rejected_by,
	payload, idempotency_key, requires_verification,
	version, created_at, updated_at`

func scanMakerCheckerRequest(row pgx.Row) (*MakerCheckerRequest, error) {
	var r MakerCheckerRequest
	err := row.Scan(
		&r.ID, &r.TenantID, &r.RequestType, &r.PropertyID, &r.Status,
		&r.CreatedBy, &r.SubmittedBy, &r.ApprovedBy, &r.RejectedBy,
		&r.Payload, &r.IdempotencyKey, &r.RequiresVerification,
		&r.Version, &r.CreatedAt, &r.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrMakerCheckerRequestNotFound
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (st *Store) GetMakerCheckerRequestByIdempotencyKey(ctx context.Context, q querier, tenantID, idempotencyKey string) (*MakerCheckerRequest, error) {
	return scanMakerCheckerRequest(q.QueryRow(ctx, `
		SELECT `+requestColumns+`
		FROM maker_checker_requests
		WHERE tenant_id = $1 AND idempotency_key = $2
	`, tenantID, idempotencyKey))
}

func (st *Store) InsertMakerCheckerRequest(ctx context.Context, q querier, r *MakerCheckerRequest) error {
	r.ID = newID("mcr")
	_, err := q.Exec(ctx, `
		INSERT INTO maker_checker_requests
			(id, tenant_id, request_type, property_id, status,
			 created_by, submitted_by, approved_by, rejected_by,
			 payload, idempotency_key, requires_verification)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, r.ID, r.TenantID, r.RequestType, r.PropertyID, r.Status,
		r.CreatedBy, r.SubmittedBy, r.ApprovedBy, r.RejectedBy,
		r.Payload, r.IdempotencyKey, r.RequiresVerification)
	return err
}

func (st *Store) GetMakerCheckerRequest(ctx context.Context, tenantID, requestID string) (*MakerCheckerRequest, error) {
	return scanMakerCheckerRequest(st.pool.QueryRow(ctx, `
		SELECT `+requestColumns+`
		FROM maker_checker_requests
		WHERE id = $1 AND tenant_id = $2
	`, requestID, tenantID))
}

func (st *Store) UpdateMakerCheckerRequestStatus(ctx context.Context, q querier, tenantID, requestID, status, approvedBy, rejectedBy string) (*MakerCheckerRequest, error) {
	return scanMakerCheckerRequest(q.QueryRow(ctx, `
		UPDATE maker_checker_requests
		SET status = $3, approved_by = $4, rejected_by = $5,
		    updated_at = NOW(), version = version + 1
		WHERE id = $1 AND tenant_id = $2
		RETURNING `+requestColumns+`
	`, requestID, tenantID, status, approvedBy, rejectedBy))
}

func (st *Store) SubmitMakerCheckerRequest(ctx context.Context, q querier, tenantID, requestID, submittedBy string) (*MakerCheckerRequest, error) {
	return scanMakerCheckerRequest(q.QueryRow(ctx, `
		UPDATE maker_checker_requests
		SET status = $3, submitted_by = $4, updated_at = NOW(), version = version + 1
		WHERE id = $1 AND tenant_id = $2
		RETURNING `+requestColumns+`
	`, requestID, tenantID, RequestStatusPendingApproval, submittedBy))
}

func (st *Store) ListMakerCheckerRequests(ctx context.Context, tenantID, propertyID string) ([]MakerCheckerRequest, error) {
	query := `SELECT ` + requestColumns + ` FROM maker_checker_requests WHERE tenant_id = $1`
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

	var out []MakerCheckerRequest
	for rows.Next() {
		r, err := scanMakerCheckerRequest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

const bankVerificationColumns = `id, tenant_id, request_id, verification_token, status,
	verified_by, verified_at, expires_at, created_at`

func scanBankVerification(row pgx.Row) (*BankVerification, error) {
	var bv BankVerification
	err := row.Scan(
		&bv.ID, &bv.TenantID, &bv.RequestID, &bv.VerificationToken, &bv.Status,
		&bv.VerifiedBy, &bv.VerifiedAt, &bv.ExpiresAt, &bv.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrBankVerificationNotFound
	}
	if err != nil {
		return nil, err
	}
	return &bv, nil
}

func (st *Store) InsertBankVerification(ctx context.Context, q querier, bv *BankVerification) error {
	bv.ID = newID("bv")
	_, err := q.Exec(ctx, `
		INSERT INTO bank_verifications
			(id, tenant_id, request_id, verification_token, status, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, bv.ID, bv.TenantID, bv.RequestID, bv.VerificationToken, bv.Status, bv.ExpiresAt)
	return err
}

func (st *Store) GetBankVerification(ctx context.Context, tenantID, verificationID string) (*BankVerification, error) {
	return scanBankVerification(st.pool.QueryRow(ctx, `
		SELECT `+bankVerificationColumns+`
		FROM bank_verifications
		WHERE id = $1 AND tenant_id = $2
	`, verificationID, tenantID))
}

func (st *Store) GetBankVerificationByRequest(ctx context.Context, tenantID, requestID string) (*BankVerification, error) {
	return scanBankVerification(st.pool.QueryRow(ctx, `
		SELECT `+bankVerificationColumns+`
		FROM bank_verifications
		WHERE tenant_id = $1 AND request_id = $2
	`, tenantID, requestID))
}

func (st *Store) ConfirmBankVerification(ctx context.Context, q querier, tenantID, verificationID, verifiedBy string) (*BankVerification, error) {
	return scanBankVerification(q.QueryRow(ctx, `
		UPDATE bank_verifications
		SET status = $3, verified_by = $4, verified_at = NOW()
		WHERE id = $1 AND tenant_id = $2 AND status = $5
		RETURNING `+bankVerificationColumns+`
	`, verificationID, tenantID, BankVerificationStatusVerified, verifiedBy, BankVerificationStatusPending))
}

func (st *Store) ExpireBankVerification(ctx context.Context, q querier, tenantID, verificationID string) (*BankVerification, error) {
	return scanBankVerification(q.QueryRow(ctx, `
		UPDATE bank_verifications
		SET status = $3
		WHERE id = $1 AND tenant_id = $2
		RETURNING `+bankVerificationColumns+`
	`, verificationID, tenantID, BankVerificationStatusExpired))
}

const reconciliationExceptionColumns = `id, tenant_id, property_id, entry_id, entry_type,
	exception_type, description, status, recorded_by, resolved_by,
	resolved_at, created_at`

func scanReconciliationException(row pgx.Row) (*ReconciliationException, error) {
	var re ReconciliationException
	err := row.Scan(
		&re.ID, &re.TenantID, &re.PropertyID, &re.EntryID, &re.EntryType,
		&re.ExceptionType, &re.Description, &re.Status, &re.RecordedBy, &re.ResolvedBy,
		&re.ResolvedAt, &re.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrReconciliationExceptionNotFound
	}
	if err != nil {
		return nil, err
	}
	return &re, nil
}

func (st *Store) InsertReconciliationException(ctx context.Context, q querier, re *ReconciliationException) error {
	re.ID = newID("re")
	_, err := q.Exec(ctx, `
		INSERT INTO reconciliation_exceptions
			(id, tenant_id, property_id, entry_id, entry_type,
			 exception_type, description, status, recorded_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, re.ID, re.TenantID, re.PropertyID, re.EntryID, re.EntryType,
		re.ExceptionType, re.Description, re.Status, re.RecordedBy)
	return err
}

func (st *Store) GetReconciliationException(ctx context.Context, tenantID, exceptionID string) (*ReconciliationException, error) {
	return scanReconciliationException(st.pool.QueryRow(ctx, `
		SELECT `+reconciliationExceptionColumns+`
		FROM reconciliation_exceptions
		WHERE id = $1 AND tenant_id = $2
	`, exceptionID, tenantID))
}

func (st *Store) ResolveReconciliationException(ctx context.Context, q querier, tenantID, exceptionID, resolvedBy string) (*ReconciliationException, error) {
	return scanReconciliationException(q.QueryRow(ctx, `
		UPDATE reconciliation_exceptions
		SET status = $3, resolved_by = $4, resolved_at = NOW()
		WHERE id = $1 AND tenant_id = $2
		RETURNING `+reconciliationExceptionColumns+`
	`, exceptionID, tenantID, ExceptionStatusResolved, resolvedBy))
}

func (st *Store) ListReconciliationExceptions(ctx context.Context, tenantID, propertyID string) ([]ReconciliationException, error) {
	query := `SELECT ` + reconciliationExceptionColumns + ` FROM reconciliation_exceptions WHERE tenant_id = $1`
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

	var out []ReconciliationException
	for rows.Next() {
		re, err := scanReconciliationException(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *re)
	}
	return out, rows.Err()
}
