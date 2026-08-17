package reporting

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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

type ReportingStore struct {
	pool *pgxpool.Pool
}

func NewReportingStore(pool *pgxpool.Pool) *ReportingStore {
	return &ReportingStore{pool: pool}
}

// --- Report snapshots ---

// UpsertSnapshot stores one snapshot per (tenant, kind, property, period).
// Rebuilding an existing snapshot replaces its data and increments its
// version, so a rebuild is idempotent.
func (s *ReportingStore) UpsertSnapshot(ctx context.Context, snap *ReportSnapshot) error {
	var existingID string
	err := s.pool.QueryRow(ctx, `
		SELECT id FROM report_snapshots
		WHERE tenant_id = $1 AND kind = $2 AND property_id = $3
		  AND period_start IS NOT DISTINCT FROM $4
		  AND period_end IS NOT DISTINCT FROM $5
	`, snap.TenantID, snap.Kind, snap.PropertyID, snap.PeriodStart, snap.PeriodEnd).Scan(&existingID)

	switch {
	case err == pgx.ErrNoRows:
		if snap.ID == "" {
			snap.ID = newID("rpt")
		}
		snap.Version = 1
		snap.CreatedAt = time.Now().UTC()
		_, err := s.pool.Exec(ctx, `
			INSERT INTO report_snapshots (
				id, tenant_id, property_id, kind, period_start, period_end,
				source_count, source_hash, data, built_at, version, created_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		`, snap.ID, snap.TenantID, snap.PropertyID, snap.Kind, snap.PeriodStart, snap.PeriodEnd,
			snap.SourceCount, snap.SourceHash, snap.Data, snap.BuiltAt, snap.Version, snap.CreatedAt)
		return err
	case err != nil:
		return fmt.Errorf("lookup report snapshot: %w", err)
	}

	err = s.pool.QueryRow(ctx, `
		UPDATE report_snapshots
		SET data = $1, source_count = $2, source_hash = $3, built_at = $4,
			version = version + 1
		WHERE id = $5 AND tenant_id = $6
		RETURNING version
	`, snap.Data, snap.SourceCount, snap.SourceHash, snap.BuiltAt, existingID, snap.TenantID).Scan(&snap.Version)
	if err != nil {
		return fmt.Errorf("update report snapshot: %w", err)
	}
	snap.ID = existingID
	return nil
}

func (s *ReportingStore) GetSnapshot(ctx context.Context, tenantID, snapshotID string) (*ReportSnapshot, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, property_id, kind, period_start, period_end,
			source_count, source_hash, data, built_at, version, created_at
		FROM report_snapshots
		WHERE id = $1 AND tenant_id = $2
	`, snapshotID, tenantID)
	return scanSnapshot(row)
}

func (s *ReportingStore) ListSnapshots(ctx context.Context, tenantID string) ([]ReportSnapshot, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, property_id, kind, period_start, period_end,
			source_count, source_hash, data, built_at, version, created_at
		FROM report_snapshots
		WHERE tenant_id = $1
		ORDER BY built_at DESC, id ASC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list report snapshots: %w", err)
	}
	defer rows.Close()
	var result []ReportSnapshot
	for rows.Next() {
		snap, err := scanSnapshot(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *snap)
	}
	return result, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanSnapshot(row scanner) (*ReportSnapshot, error) {
	var snap ReportSnapshot
	err := row.Scan(
		&snap.ID, &snap.TenantID, &snap.PropertyID, &snap.Kind,
		&snap.PeriodStart, &snap.PeriodEnd,
		&snap.SourceCount, &snap.SourceHash, &snap.Data,
		&snap.BuiltAt, &snap.Version, &snap.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrSnapshotNotFound
		}
		return nil, fmt.Errorf("scan report snapshot: %w", err)
	}
	return &snap, nil
}

// --- Worker metric observations ---

func (s *ReportingStore) InsertMetricObservation(ctx context.Context, o *MetricObservation) error {
	if o.ID == "" {
		o.ID = newID("mtr")
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO metric_observations (
			id, tenant_id, property_id, worker_id, metric_kind, value, unit,
			period_start, period_end, source_ref, recorded_by, recorded_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	`, o.ID, o.TenantID, o.PropertyID, o.WorkerID, o.MetricKind, o.Value, o.Unit,
		o.PeriodStart, o.PeriodEnd, o.SourceRef, o.RecordedBy, o.RecordedAt)
	if err != nil {
		return fmt.Errorf("insert metric observation: %w", err)
	}
	return nil
}

// ListMetricObservations returns worker metrics in chronological order. It is
// deliberately never ordered or aggregated by value: worker metrics are
// development feedback and must not be turned into ranking or discipline.
func (s *ReportingStore) ListMetricObservations(ctx context.Context, tenantID, propertyID, workerID, metricKind string) ([]MetricObservation, error) {
	query := `
		SELECT id, tenant_id, property_id, worker_id, metric_kind, value, unit,
			period_start, period_end, source_ref, recorded_by, recorded_at
		FROM metric_observations
		WHERE tenant_id = $1`
	args := []any{tenantID}
	if propertyID != "" {
		args = append(args, propertyID)
		query += fmt.Sprintf(" AND property_id = $%d", len(args))
	}
	if workerID != "" {
		args = append(args, workerID)
		query += fmt.Sprintf(" AND worker_id = $%d", len(args))
	}
	if metricKind != "" {
		args = append(args, metricKind)
		query += fmt.Sprintf(" AND metric_kind = $%d", len(args))
	}
	query += " ORDER BY recorded_at ASC, id ASC"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list metric observations: %w", err)
	}
	defer rows.Close()
	var result []MetricObservation
	for rows.Next() {
		o, err := scanMetricObservation(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *o)
	}
	return result, rows.Err()
}

func scanMetricObservation(row scanner) (*MetricObservation, error) {
	var o MetricObservation
	err := row.Scan(
		&o.ID, &o.TenantID, &o.PropertyID, &o.WorkerID, &o.MetricKind,
		&o.Value, &o.Unit, &o.PeriodStart, &o.PeriodEnd, &o.SourceRef,
		&o.RecordedBy, &o.RecordedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan metric observation: %w", err)
	}
	return &o, nil
}

// --- Source queries used by the projections ---

// scopeArgs builds the tenant, property and period filter for a source query.
// period filters use NULL to mean unbounded, so an empty period and an
// explicit window are both handled.
func scopeArgs(tenantID, propertyID string, start, end *time.Time) (string, []any) {
	filter := " tenant_id = $1"
	args := []any{tenantID}
	if propertyID != "" {
		args = append(args, propertyID)
		filter += fmt.Sprintf(" AND property_id = $%d", len(args))
	}
	if start != nil {
		args = append(args, *start)
		filter += fmt.Sprintf(" AND created_at >= $%d", len(args))
	}
	if end != nil {
		args = append(args, *end)
		filter += fmt.Sprintf(" AND created_at < $%d", len(args))
	}
	return filter, args
}

func (s *ReportingStore) ListChargesForReport(ctx context.Context, tenantID, propertyID string, start, end *time.Time) ([]ChargeSourceRow, error) {
	filter, args := scopeArgs(tenantID, propertyID, start, end)
	rows, err := s.pool.Query(ctx, `
		SELECT id, charge_type, status, amount_minor_units, currency
		FROM charges
		WHERE`+filter+`
		ORDER BY created_at ASC, id ASC
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("list charges for report: %w", err)
	}
	defer rows.Close()
	var result []ChargeSourceRow
	for rows.Next() {
		var c ChargeSourceRow
		if err := rows.Scan(&c.ID, &c.ChargeType, &c.Status, &c.AmountMinorUnits, &c.Currency); err != nil {
			return nil, fmt.Errorf("scan charge for report: %w", err)
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

func (s *ReportingStore) ListCreditsForReport(ctx context.Context, tenantID, propertyID string, start, end *time.Time) ([]CreditSourceRow, error) {
	filter, args := scopeArgs(tenantID, propertyID, start, end)
	rows, err := s.pool.Query(ctx, `
		SELECT id, credit_type, status, amount_minor_units, currency
		FROM credits
		WHERE`+filter+`
		ORDER BY created_at ASC, id ASC
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("list credits for report: %w", err)
	}
	defer rows.Close()
	var result []CreditSourceRow
	for rows.Next() {
		var c CreditSourceRow
		if err := rows.Scan(&c.ID, &c.CreditType, &c.Status, &c.AmountMinorUnits, &c.Currency); err != nil {
			return nil, fmt.Errorf("scan credit for report: %w", err)
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

func (s *ReportingStore) ListRecoveriesForReport(ctx context.Context, tenantID, propertyID string, start, end *time.Time) ([]RecoverySourceRow, error) {
	filter, args := scopeArgs(tenantID, propertyID, start, end)
	rows, err := s.pool.Query(ctx, `
		SELECT id, status, rework_cost_minor, currency
		FROM service_recoveries
		WHERE`+filter+`
		ORDER BY created_at ASC, id ASC
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("list recoveries for report: %w", err)
	}
	defer rows.Close()
	var result []RecoverySourceRow
	for rows.Next() {
		var r RecoverySourceRow
		if err := rows.Scan(&r.ID, &r.Status, &r.ReworkCostMinor, &r.Currency); err != nil {
			return nil, fmt.Errorf("scan recovery for report: %w", err)
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// ListIncidentTicketsForReport returns incident tickets and the count of open
// ones for a property. Incident tickets are owner-visible exceptions.
func (s *ReportingStore) ListIncidentTicketsForReport(ctx context.Context, tenantID, propertyID string, start, end *time.Time) (int, []TicketSourceRow, error) {
	filter, args := scopeArgs(tenantID, propertyID, start, end)
	rows, err := s.pool.Query(ctx, `
		SELECT id, property_id, type, status, COALESCE(severity, ''), reason, created_at
		FROM tickets
		WHERE`+filter+` AND type = 'incident'
		ORDER BY created_at DESC, id ASC
	`, args...)
	if err != nil {
		return 0, nil, fmt.Errorf("list incident tickets for report: %w", err)
	}
	defer rows.Close()
	var result []TicketSourceRow
	open := 0
	for rows.Next() {
		var t TicketSourceRow
		if err := rows.Scan(&t.ID, &t.PropertyID, &t.Type, &t.Status, &t.Severity, &t.Reason, &t.CreatedAt); err != nil {
			return 0, nil, fmt.Errorf("scan incident ticket for report: %w", err)
		}
		result = append(result, t)
		if ClassifyTicketException(t.Type, t.Status).OwnerVisible {
			open++
		}
	}
	if err := rows.Err(); err != nil {
		return 0, nil, err
	}
	return open, result, nil
}

func (s *ReportingStore) CountClosedTicketsForReport(ctx context.Context, tenantID, propertyID string, start, end *time.Time) (int, error) {
	filter, args := scopeArgs(tenantID, propertyID, start, end)
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM tickets
		WHERE`+filter+` AND status = 'closed'
	`, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count closed tickets for report: %w", err)
	}
	return count, nil
}

func (s *ReportingStore) CountOpenRecoveriesForReport(ctx context.Context, tenantID, propertyID string, start, end *time.Time) (int, error) {
	filter, args := scopeArgs(tenantID, propertyID, start, end)
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM service_recoveries
		WHERE`+filter+` AND status = 'open'
	`, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count open recoveries for report: %w", err)
	}
	return count, nil
}

// CountInventoryMovementsForReport counts append-only stock movements across
// the property's stock locations (INV ledger).
func (s *ReportingStore) CountInventoryMovementsForReport(ctx context.Context, tenantID, propertyID string, start, end *time.Time) (int, error) {
	filter := " m.tenant_id = $1"
	args := []any{tenantID}
	if propertyID != "" {
		args = append(args, propertyID)
		filter += fmt.Sprintf(" AND l.property_id = $%d", len(args))
	}
	if start != nil {
		args = append(args, *start)
		filter += fmt.Sprintf(" AND m.created_at >= $%d", len(args))
	}
	if end != nil {
		args = append(args, *end)
		filter += fmt.Sprintf(" AND m.created_at < $%d", len(args))
	}
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM inventory_movements m
		JOIN stock_locations l ON l.id = m.location_id
		WHERE`+filter, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count inventory movements for report: %w", err)
	}
	return count, nil
}

// ListOwnerExceptionRows reads the tenant-scoped exception records that can
// surface to the owner and classifies each one. Rows that are internal noise
// (routine operational work, closed records) keep OwnerVisible=false and are
// filtered out by the caller.
func (s *ReportingStore) ListOwnerExceptionRows(ctx context.Context, tenantID, propertyID string, start, end *time.Time) ([]OwnerException, error) {
	result := []OwnerException{}

	// Incident tickets: only active incidents are owner-visible.
	incidents := []TicketSourceRow{}
	filter, args := scopeArgs(tenantID, propertyID, start, end)
	rows, err := s.pool.Query(ctx, `
		SELECT id, property_id, type, status, COALESCE(severity, ''), reason, created_at
		FROM tickets
		WHERE`+filter+` AND type = 'incident'
		ORDER BY created_at DESC, id ASC
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("list incident exceptions: %w", err)
	}
	for rows.Next() {
		var t TicketSourceRow
		if err := rows.Scan(&t.ID, &t.PropertyID, &t.Type, &t.Status, &t.Severity, &t.Reason, &t.CreatedAt); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan incident exception: %w", err)
		}
		incidents = append(incidents, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, t := range incidents {
		class := ClassifyTicketException(t.Type, t.Status)
		if !class.OwnerVisible {
			continue
		}
		result = append(result, OwnerException{
			Source:       ExceptionSourceIncident,
			SourceID:     t.ID,
			PropertyID:   t.PropertyID,
			Label:        class.Label,
			Summary:      t.Reason,
			Severity:     t.Severity,
			Status:       t.Status,
			OccurredAt:   t.CreatedAt,
			OwnerVisible: true,
		})
	}

	// Service recoveries: only open recoveries are owner-visible.
	filter, args = scopeArgs(tenantID, propertyID, start, end)
	recoveryRows, err := s.pool.Query(ctx, `
		SELECT id, property_id, COALESCE(severity, ''), original_reason, status, created_at
		FROM service_recoveries
		WHERE`+filter+`
		ORDER BY created_at DESC, id ASC
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("list recovery exceptions: %w", err)
	}
	type recoveryRow struct {
		ID, PropertyID, Severity, Reason, Status string
		CreatedAt                                time.Time
	}
	var recoveries []recoveryRow
	for recoveryRows.Next() {
		var r recoveryRow
		if err := recoveryRows.Scan(&r.ID, &r.PropertyID, &r.Severity, &r.Reason, &r.Status, &r.CreatedAt); err != nil {
			recoveryRows.Close()
			return nil, fmt.Errorf("scan recovery exception: %w", err)
		}
		recoveries = append(recoveries, r)
	}
	recoveryRows.Close()
	if err := recoveryRows.Err(); err != nil {
		return nil, err
	}
	for _, r := range recoveries {
		class := ClassifyRecoveryException(r.Status)
		if !class.OwnerVisible {
			continue
		}
		result = append(result, OwnerException{
			Source:       ExceptionSourceServiceRecovery,
			SourceID:     r.ID,
			PropertyID:   r.PropertyID,
			Label:        class.Label,
			Summary:      r.Reason,
			Severity:     r.Severity,
			Status:       r.Status,
			OccurredAt:   r.CreatedAt,
			OwnerVisible: true,
		})
	}

	// Reconciliation exceptions: only open financial exceptions are
	// owner-visible.
	filter, args = scopeArgs(tenantID, propertyID, start, end)
	finRows, err := s.pool.Query(ctx, `
		SELECT id, property_id, exception_type, description, status, created_at
		FROM reconciliation_exceptions
		WHERE`+filter+`
		ORDER BY created_at DESC, id ASC
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("list financial exceptions: %w", err)
	}
	type finRow struct {
		ID, PropertyID, ExceptionType, Description, Status string
		CreatedAt                                          time.Time
	}
	var fins []finRow
	for finRows.Next() {
		var f finRow
		if err := finRows.Scan(&f.ID, &f.PropertyID, &f.ExceptionType, &f.Description, &f.Status, &f.CreatedAt); err != nil {
			finRows.Close()
			return nil, fmt.Errorf("scan financial exception: %w", err)
		}
		fins = append(fins, f)
	}
	finRows.Close()
	if err := finRows.Err(); err != nil {
		return nil, err
	}
	for _, f := range fins {
		class := ClassifyFinancialException(f.Status)
		if !class.OwnerVisible {
			continue
		}
		result = append(result, OwnerException{
			Source:       ExceptionSourceFinancial,
			SourceID:     f.ID,
			PropertyID:   f.PropertyID,
			Label:        class.Label,
			Summary:      fmt.Sprintf("%s: %s", f.ExceptionType, f.Description),
			Status:       f.Status,
			OccurredAt:   f.CreatedAt,
			OwnerVisible: true,
		})
	}

	return result, nil
}

// --- Source queries for new read models ---

// CountComplianceHolds counts active compliance items with critical severity
// and a hold_id (active holds) for a property.
func (s *ReportingStore) CountComplianceHolds(ctx context.Context, tenantID, propertyID string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM compliance_items
		WHERE tenant_id = $1 AND property_id = $2 AND status = 'active' AND hold_id IS NOT NULL
	`, tenantID, propertyID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count compliance holds: %w", err)
	}
	return count, nil
}

// CountPendingRenewals counts compliance items nearing expiry with renewal
// warnings issued for a property.
func (s *ReportingStore) CountPendingRenewals(ctx context.Context, tenantID, propertyID string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM compliance_renewal_warnings
		WHERE tenant_id = $1 AND property_id = $2 AND acknowledged_at IS NULL
	`, tenantID, propertyID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count pending renewals: %w", err)
	}
	return count, nil
}

// GetOnboardingStatus returns the onboarding status for a property.
func (s *ReportingStore) GetOnboardingStatus(ctx context.Context, tenantID, propertyID string) (string, error) {
	var status string
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(status, 'not_started')
		FROM onboarding_cases
		WHERE tenant_id = $1 AND property_id = $2
		ORDER BY created_at DESC LIMIT 1
	`, tenantID, propertyID).Scan(&status)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "not_started", nil
		}
		return "", fmt.Errorf("get onboarding status: %w", err)
	}
	return status, nil
}

// CountTicketsByStatus counts tickets for a property with an optional status filter.
func (s *ReportingStore) CountTicketsByStatus(ctx context.Context, tenantID, propertyID string, start, end *time.Time, status string) (int, error) {
	filter, args := scopeArgs(tenantID, propertyID, start, end)
	query := `SELECT COUNT(*) FROM tickets WHERE` + filter
	if status != "" {
		args = append(args, status)
		query += fmt.Sprintf(" AND status = $%d", len(args))
	}
	var count int
	err := s.pool.QueryRow(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count tickets: %w", err)
	}
	return count, nil
}

// CountCompletedChecklistItems counts checklist items with status
// 'completed'. ticket_checklist_items carries no property_id of its own --
// it is scoped to a property only through its parent ticket -- so this
// joins tickets directly rather than reusing scopeArgs, which assumes a
// flat property_id column on the target table.
func (s *ReportingStore) CountCompletedChecklistItems(ctx context.Context, tenantID, propertyID string, start, end *time.Time) (int, error) {
	args := []any{tenantID}
	query := `
		SELECT COUNT(*)
		FROM ticket_checklist_items tci
		JOIN tickets t ON t.id = tci.ticket_id AND t.tenant_id = tci.tenant_id
		WHERE tci.tenant_id = $1 AND tci.status = 'completed'
	`
	if propertyID != "" {
		args = append(args, propertyID)
		query += fmt.Sprintf(" AND t.property_id = $%d", len(args))
	}
	if start != nil {
		args = append(args, *start)
		query += fmt.Sprintf(" AND tci.created_at >= $%d", len(args))
	}
	if end != nil {
		args = append(args, *end)
		query += fmt.Sprintf(" AND tci.created_at < $%d", len(args))
	}
	var count int
	err := s.pool.QueryRow(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count completed checklists: %w", err)
	}
	return count, nil
}

// CountStockLocations counts stock locations for a property.
func (s *ReportingStore) CountStockLocations(ctx context.Context, tenantID, propertyID string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM stock_locations
		WHERE tenant_id = $1 AND property_id = $2
	`, tenantID, propertyID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count stock locations: %w", err)
	}
	return count, nil
}

// CountInventoryMovementsDetailed returns total movements and total consumed quantity.
func (s *ReportingStore) CountInventoryMovementsDetailed(ctx context.Context, tenantID, propertyID string) (movements int, consumed int64, err error) {
	err = s.pool.QueryRow(ctx, `
		SELECT COUNT(*), COALESCE(SUM(CASE WHEN m.movement_type = 'consume' THEN m.quantity ELSE 0 END), 0)
		FROM inventory_movements m
		JOIN stock_locations l ON l.id = m.location_id
		WHERE m.tenant_id = $1 AND l.property_id = $2
	`, tenantID, propertyID).Scan(&movements, &consumed)
	if err != nil {
		return 0, 0, fmt.Errorf("count inventory movements detailed: %w", err)
	}
	return movements, consumed, nil
}

// CountAdjustments counts inventory adjustment entries.
func (s *ReportingStore) CountAdjustments(ctx context.Context, tenantID, propertyID string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM inventory_movements m
		JOIN stock_locations l ON l.id = m.location_id
		WHERE m.tenant_id = $1 AND l.property_id = $2 AND m.movement_type = 'adjustment'
	`, tenantID, propertyID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count adjustments: %w", err)
	}
	return count, nil
}

// CountPendingInventoryCounts counts inventory count records in draft status.
func (s *ReportingStore) CountPendingInventoryCounts(ctx context.Context, tenantID, propertyID string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM inventory_counts ic
		JOIN stock_locations l ON l.id = ic.location_id
		WHERE ic.tenant_id = $1 AND l.property_id = $2 AND ic.status = 'draft'
	`, tenantID, propertyID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count pending inventory counts: %w", err)
	}
	return count, nil
}

// CountMakerCheckerRequests returns a map of status -> count for maker-checker requests.
func (s *ReportingStore) CountMakerCheckerRequests(ctx context.Context, tenantID, propertyID string) (map[string]int, error) {
	var rows []struct {
		Status string
		Count  int
	}
	q := `
		SELECT status, COUNT(*)
		FROM maker_checker_requests
		WHERE tenant_id = $1 AND property_id = $2
		GROUP BY status
	`
	r, err := s.pool.Query(ctx, q, tenantID, propertyID)
	if err != nil {
		return nil, fmt.Errorf("count maker-checker requests: %w", err)
	}
	defer r.Close()
	for r.Next() {
		var row struct {
			Status string
			Count  int
		}
		if err := r.Scan(&row.Status, &row.Count); err != nil {
			return nil, fmt.Errorf("scan maker-checker count: %w", err)
		}
		rows = append(rows, row)
	}
	if err := r.Err(); err != nil {
		return nil, err
	}
	result := make(map[string]int)
	for _, row := range rows {
		result[row.Status] = row.Count
	}
	return result, nil
}

// CountDocuments counts total documents for a property.
func (s *ReportingStore) CountDocuments(ctx context.Context, tenantID, propertyID string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM documents
		WHERE tenant_id = $1 AND property_id = $2
	`, tenantID, propertyID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count documents: %w", err)
	}
	return count, nil
}

// CountExpiredDocuments counts documents that have an expires_at before now.
func (s *ReportingStore) CountExpiredDocuments(ctx context.Context, tenantID, propertyID string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM documents
		WHERE tenant_id = $1 AND property_id = $2
		  AND expires_at IS NOT NULL AND expires_at < NOW()
	`, tenantID, propertyID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count expired documents: %w", err)
	}
	return count, nil
}

// CountPendingDocumentReviews counts document reviews with pending status.
func (s *ReportingStore) CountPendingDocumentReviews(ctx context.Context, tenantID, propertyID string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM document_reviews dr
		JOIN documents d ON d.id = dr.document_id
		WHERE dr.tenant_id = $1 AND d.property_id = $2 AND dr.status = 'pending'
	`, tenantID, propertyID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count pending document reviews: %w", err)
	}
	return count, nil
}

// CountCompletedSubmissionPackets counts submission packets with status 'submitted'.
func (s *ReportingStore) CountCompletedSubmissionPackets(ctx context.Context, tenantID, propertyID string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM submission_packets
		WHERE tenant_id = $1 AND property_id = $2 AND status = 'submitted'
	`, tenantID, propertyID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count submitted packets: %w", err)
	}
	return count, nil
}

// AggregateTimeEntries returns total work minutes, travel minutes, overtime count,
// and distinct worker count for time_entries linked to a property.
func (s *ReportingStore) AggregateTimeEntries(ctx context.Context, tenantID, propertyID string) (work int64, travel int64, overtime int, workers int, err error) {
	err = s.pool.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(work_minutes), 0),
			COALESCE(SUM(travel_minutes), 0),
			COALESCE(SUM(CASE WHEN overtime_flag THEN 1 ELSE 0 END), 0),
			COUNT(DISTINCT worker_id)
		FROM time_entries
		WHERE tenant_id = $1
		  AND ticket_id IN (SELECT id FROM tickets WHERE tenant_id = $1 AND property_id = $2)
	`, tenantID, propertyID).Scan(&work, &travel, &overtime, &workers)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("aggregate time entries: %w", err)
	}
	return work, travel, overtime, workers, nil
}

// AggregateExpenses returns total minor units of expenses linked to a property.
func (s *ReportingStore) AggregateExpenses(ctx context.Context, tenantID, propertyID string) (int64, error) {
	var total int64
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(minor_units), 0)
		FROM expenses
		WHERE tenant_id = $1
		  AND ticket_id IN (SELECT id FROM tickets WHERE tenant_id = $1 AND property_id = $2)
	`, tenantID, propertyID).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("aggregate expenses: %w", err)
	}
	return total, nil
}
