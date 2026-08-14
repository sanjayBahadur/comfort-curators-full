package consumer

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
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

type ConsumerStore struct {
	pool *pgxpool.Pool
}

func NewConsumerStore(pool *pgxpool.Pool) *ConsumerStore {
	return &ConsumerStore{pool: pool}
}

// --- Disclosures ---

func (s *ConsumerStore) InsertDisclosure(ctx context.Context, q querier, d *Disclosure) error {
	if d.ID == "" {
		d.ID = newID("cnd")
	}
	_, err := q.Exec(ctx, `
		INSERT INTO consumer_disclosures (
			id, tenant_id, property_id, resource_type, resource_id,
			price_minor_units, tax_minor_units, currency, recurrence,
			recurrence_amount_minor_units, substitution_policy, cancellation_policy,
			refund_policy, seller, country_of_origin, grievance_contact,
			recurring_cost_visible, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
	`, d.ID, d.TenantID, d.PropertyID, d.ResourceType, d.ResourceID,
		d.PriceMinorUnits, d.TaxMinorUnits, d.Currency, d.Recurrence,
		d.RecurrenceAmountMinorUnits, d.SubstitutionPolicy, d.CancellationPolicy,
		d.RefundPolicy, d.Seller, d.CountryOfOrigin, d.GrievanceContact,
		d.RecurringCostVisible, d.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert disclosure: %w", err)
	}
	return nil
}

func (s *ConsumerStore) GetDisclosure(ctx context.Context, tenantID, disclosureID string) (*Disclosure, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, property_id, resource_type, resource_id,
			price_minor_units, tax_minor_units, currency, recurrence,
			recurrence_amount_minor_units, substitution_policy, cancellation_policy,
			refund_policy, seller, country_of_origin, grievance_contact,
			recurring_cost_visible, created_at
		FROM consumer_disclosures
		WHERE id = $1 AND tenant_id = $2
	`, disclosureID, tenantID)
	return scanDisclosure(row)
}

func (s *ConsumerStore) ListDisclosures(ctx context.Context, tenantID string) ([]Disclosure, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, property_id, resource_type, resource_id,
			price_minor_units, tax_minor_units, currency, recurrence,
			recurrence_amount_minor_units, substitution_policy, cancellation_policy,
			refund_policy, seller, country_of_origin, grievance_contact,
			recurring_cost_visible, created_at
		FROM consumer_disclosures
		WHERE tenant_id = $1
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list disclosures: %w", err)
	}
	defer rows.Close()
	var result []Disclosure
	for rows.Next() {
		d, err := scanDisclosure(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *d)
	}
	return result, rows.Err()
}

// --- Acceptances ---

func (s *ConsumerStore) InsertAcceptance(ctx context.Context, q querier, a *Acceptance) error {
	if a.ID == "" {
		a.ID = newID("cna")
	}
	_, err := q.Exec(ctx, `
		INSERT INTO consumer_acceptances (
			id, tenant_id, property_id, disclosure_id, resource_type,
			resource_id, accepted_by, accepted_at, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`, a.ID, a.TenantID, a.PropertyID, a.DisclosureID, a.ResourceType,
		a.ResourceID, a.AcceptedBy, a.AcceptedAt, a.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert acceptance: %w", err)
	}
	return nil
}

func (s *ConsumerStore) GetAcceptance(ctx context.Context, tenantID, acceptanceID string) (*Acceptance, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, property_id, disclosure_id, resource_type,
			resource_id, accepted_by, accepted_at, created_at
		FROM consumer_acceptances
		WHERE id = $1 AND tenant_id = $2
	`, acceptanceID, tenantID)
	return scanAcceptance(row)
}

// --- History exports ---

func (s *ConsumerStore) InsertHistoryExport(ctx context.Context, q querier, e *HistoryExport) error {
	if e.ID == "" {
		e.ID = newID("cnx")
	}
	_, err := q.Exec(ctx, `
		INSERT INTO consumer_history_exports (
			id, tenant_id, property_id, requested_by, status, data, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7)
	`, e.ID, e.TenantID, e.PropertyID, e.RequestedBy, e.Status, e.Data, e.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert history export: %w", err)
	}
	return nil
}

func (s *ConsumerStore) GetHistoryExport(ctx context.Context, tenantID, exportID string) (*HistoryExport, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, property_id, requested_by, status, data, created_at
		FROM consumer_history_exports
		WHERE id = $1 AND tenant_id = $2
	`, exportID, tenantID)
	return scanHistoryExport(row)
}

func (s *ConsumerStore) ListHistoryExports(ctx context.Context, tenantID string) ([]HistoryExport, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, property_id, requested_by, status, data, created_at
		FROM consumer_history_exports
		WHERE tenant_id = $1
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list history exports: %w", err)
	}
	defer rows.Close()
	var result []HistoryExport
	for rows.Next() {
		e, err := scanHistoryExport(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *e)
	}
	return result, rows.Err()
}

// CollectHistory gathers owner order, invoice, package and service history for
// a tenant (and optionally a property) using tenant-scoped queries only, so
// another tenant's rows can never enter the export (CON-006).
func (s *ConsumerStore) CollectHistory(ctx context.Context, tenantID, propertyID string) (json.RawMessage, error) {
	data := HistoryExportData{
		Packages: []ExportedPackage{},
		Invoices: []ExportedInvoice{},
		Orders:   []ExportedOrder{},
		Services: []ExportedService{},
	}

	propertyFilter := ""
	args := []any{tenantID}
	if propertyID != "" {
		propertyFilter = " AND property_id = $2"
		args = append(args, propertyID)
	}

	packageRows, err := s.pool.Query(ctx, `
		SELECT id, property_id, version_number, status, effective_date,
			currency, setup_cost_minor_units, monthly_cost_minor_units, created_at
		FROM property_package_versions
		WHERE tenant_id = $1`+propertyFilter+`
		ORDER BY created_at DESC
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("collect package history: %w", err)
	}
	defer packageRows.Close()
	for packageRows.Next() {
		var p ExportedPackage
		if err := packageRows.Scan(
			&p.ID, &p.PropertyID, &p.VersionNumber, &p.Status, &p.EffectiveDate,
			&p.Currency, &p.SetupCostMinorUnits, &p.MonthlyCostMinorUnits, &p.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan package history: %w", err)
		}
		data.Packages = append(data.Packages, p)
	}
	if err := packageRows.Err(); err != nil {
		return nil, fmt.Errorf("package history rows: %w", err)
	}

	invoiceRows, err := s.pool.Query(ctx, `
		SELECT id, property_id, period_start, period_end, total_minor_units,
			currency, status, created_at
		FROM invoices
		WHERE tenant_id = $1`+propertyFilter+`
		ORDER BY created_at DESC
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("collect invoice history: %w", err)
	}
	defer invoiceRows.Close()
	for invoiceRows.Next() {
		var inv ExportedInvoice
		if err := invoiceRows.Scan(
			&inv.ID, &inv.PropertyID, &inv.PeriodStart, &inv.PeriodEnd,
			&inv.TotalMinorUnits, &inv.Currency, &inv.Status, &inv.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan invoice history: %w", err)
		}
		data.Invoices = append(data.Invoices, inv)
	}
	if err := invoiceRows.Err(); err != nil {
		return nil, fmt.Errorf("invoice history rows: %w", err)
	}

	orderRows, err := s.pool.Query(ctx, `
		SELECT id, property_id, order_id, charge_type, amount_minor_units,
			currency, status, created_at
		FROM charges
		WHERE tenant_id = $1 AND order_id != ''`+propertyFilter+`
		ORDER BY created_at DESC
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("collect order history: %w", err)
	}
	defer orderRows.Close()
	for orderRows.Next() {
		var o ExportedOrder
		if err := orderRows.Scan(
			&o.ID, &o.PropertyID, &o.OrderID, &o.ChargeType,
			&o.AmountMinorUnits, &o.Currency, &o.Status, &o.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan order history: %w", err)
		}
		data.Orders = append(data.Orders, o)
	}
	if err := orderRows.Err(); err != nil {
		return nil, fmt.Errorf("order history rows: %w", err)
	}

	serviceRows, err := s.pool.Query(ctx, `
		SELECT id, property_id, type, status, created_at
		FROM tickets
		WHERE tenant_id = $1`+propertyFilter+`
		ORDER BY created_at DESC
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("collect service history: %w", err)
	}
	defer serviceRows.Close()
	for serviceRows.Next() {
		var svc ExportedService
		if err := serviceRows.Scan(&svc.ID, &svc.PropertyID, &svc.Type, &svc.Status, &svc.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan service history: %w", err)
		}
		data.Services = append(data.Services, svc)
	}
	if err := serviceRows.Err(); err != nil {
		return nil, fmt.Errorf("service history rows: %w", err)
	}

	raw, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshal history export: %w", err)
	}
	return raw, nil
}

// --- Scanner helpers ---

type scanner interface {
	Scan(dest ...any) error
}

func scanDisclosure(row scanner) (*Disclosure, error) {
	var d Disclosure
	err := row.Scan(
		&d.ID, &d.TenantID, &d.PropertyID, &d.ResourceType, &d.ResourceID,
		&d.PriceMinorUnits, &d.TaxMinorUnits, &d.Currency, &d.Recurrence,
		&d.RecurrenceAmountMinorUnits, &d.SubstitutionPolicy, &d.CancellationPolicy,
		&d.RefundPolicy, &d.Seller, &d.CountryOfOrigin, &d.GrievanceContact,
		&d.RecurringCostVisible, &d.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrDisclosureNotFound
		}
		return nil, fmt.Errorf("scan disclosure: %w", err)
	}
	return &d, nil
}

func scanAcceptance(row scanner) (*Acceptance, error) {
	var a Acceptance
	err := row.Scan(
		&a.ID, &a.TenantID, &a.PropertyID, &a.DisclosureID, &a.ResourceType,
		&a.ResourceID, &a.AcceptedBy, &a.AcceptedAt, &a.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrAcceptanceNotFound
		}
		return nil, fmt.Errorf("scan acceptance: %w", err)
	}
	return &a, nil
}

func scanHistoryExport(row scanner) (*HistoryExport, error) {
	var e HistoryExport
	err := row.Scan(
		&e.ID, &e.TenantID, &e.PropertyID, &e.RequestedBy, &e.Status,
		&e.Data, &e.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrExportNotFound
		}
		return nil, fmt.Errorf("scan history export: %w", err)
	}
	return &e, nil
}
