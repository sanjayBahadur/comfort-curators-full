package compliance

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

type ComplianceStore struct {
	pool *pgxpool.Pool
}

func NewComplianceStore(pool *pgxpool.Pool) *ComplianceStore {
	return &ComplianceStore{pool: pool}
}

func (s *ComplianceStore) InsertItem(ctx context.Context, q querier, item *ComplianceItem) error {
	if item.ID == "" {
		item.ID = newID("cmp")
	}
	evidenceJSON, err := json.Marshal(item.EvidenceIDs)
	if err != nil {
		return fmt.Errorf("marshal evidence ids: %w", err)
	}
	_, err = q.Exec(ctx, `
		INSERT INTO compliance_items (
			id, property_id, tenant_id, kind, severity, name, description,
			effective_date, expiry_date, status, evidence_ids, renewed_from_id,
			hold_id, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
	`,
		item.ID, item.PropertyID, item.TenantID, item.Kind, item.Severity,
		item.Name, item.Description, item.EffectiveDate, item.ExpiryDate,
		item.Status, evidenceJSON, item.RenewedFromID, item.HoldID,
		item.CreatedAt, item.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert compliance item: %w", err)
	}
	return nil
}

func (s *ComplianceStore) GetItem(ctx context.Context, tenantID, itemID string) (*ComplianceItem, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, property_id, tenant_id, kind, severity, name, description,
			effective_date, expiry_date, status, evidence_ids, renewed_from_id,
			hold_id, created_at, updated_at
		FROM compliance_items
		WHERE id = $1 AND tenant_id = $2
	`, itemID, tenantID)
	return scanComplianceItem(row)
}

func (s *ComplianceStore) ListItems(ctx context.Context, tenantID, propertyID string) ([]ComplianceItem, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, property_id, tenant_id, kind, severity, name, description,
			effective_date, expiry_date, status, evidence_ids, renewed_from_id,
			hold_id, created_at, updated_at
		FROM compliance_items
		WHERE tenant_id = $1 AND property_id = $2
		ORDER BY expiry_date ASC
	`, tenantID, propertyID)
	if err != nil {
		return nil, fmt.Errorf("list compliance items: %w", err)
	}
	defer rows.Close()

	var items []ComplianceItem
	for rows.Next() {
		item, err := scanComplianceItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

func (s *ComplianceStore) ListActiveItems(ctx context.Context, now time.Time) ([]ComplianceItem, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, property_id, tenant_id, kind, severity, name, description,
			effective_date, expiry_date, status, evidence_ids, renewed_from_id,
			hold_id, created_at, updated_at
		FROM compliance_items
		WHERE status = $1
		ORDER BY expiry_date ASC
	`, ItemStatusActive)
	if err != nil {
		return nil, fmt.Errorf("list active compliance items: %w", err)
	}
	defer rows.Close()

	var items []ComplianceItem
	for rows.Next() {
		item, err := scanComplianceItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

func (s *ComplianceStore) UpdateItem(ctx context.Context, q querier, item *ComplianceItem) error {
	evidenceJSON, err := json.Marshal(item.EvidenceIDs)
	if err != nil {
		return fmt.Errorf("marshal evidence ids: %w", err)
	}
	tag, err := q.Exec(ctx, `
		UPDATE compliance_items
		SET status = $4, evidence_ids = $5, renewed_from_id = $6,
			hold_id = $7, description = $8, updated_at = $9
		WHERE id = $1 AND property_id = $2 AND tenant_id = $3
	`,
		item.ID, item.PropertyID, item.TenantID, item.Status,
		evidenceJSON, item.RenewedFromID, item.HoldID, item.Description,
		item.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("update compliance item: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrComplianceItemNotFound
	}
	return nil
}

func (s *ComplianceStore) InsertRenewalWarning(ctx context.Context, w *ComplianceRenewalWarning) error {
	if w.ID == "" {
		w.ID = newID("crw")
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO compliance_renewal_warnings (
			id, item_id, property_id, tenant_id, days_before_expiry,
			issued_at, acknowledged_at, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`,
		w.ID, w.ItemID, w.PropertyID, w.TenantID, w.DaysBeforeExpiry,
		w.IssuedAt, w.Acknowledged, w.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert renewal warning: %w", err)
	}
	return nil
}

func (s *ComplianceStore) HasActiveWarningForItemInWindow(ctx context.Context, itemID string, daysBeforeExpiry int) (bool, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM compliance_renewal_warnings
		WHERE item_id = $1 AND days_before_expiry = $2 AND acknowledged_at IS NULL
	`, itemID, daysBeforeExpiry).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check active warnings: %w", err)
	}
	return count > 0, nil
}

func (s *ComplianceStore) ListRenewalWarnings(ctx context.Context, tenantID, propertyID string) ([]ComplianceRenewalWarning, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, item_id, property_id, tenant_id, days_before_expiry,
			issued_at, acknowledged_at, created_at
		FROM compliance_renewal_warnings
		WHERE tenant_id = $1 AND property_id = $2
		ORDER BY issued_at DESC
	`, tenantID, propertyID)
	if err != nil {
		return nil, fmt.Errorf("list renewal warnings: %w", err)
	}
	defer rows.Close()

	var warnings []ComplianceRenewalWarning
	for rows.Next() {
		var w ComplianceRenewalWarning
		if err := rows.Scan(
			&w.ID, &w.ItemID, &w.PropertyID, &w.TenantID,
			&w.DaysBeforeExpiry, &w.IssuedAt, &w.Acknowledged, &w.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan renewal warning: %w", err)
		}
		warnings = append(warnings, w)
	}
	return warnings, rows.Err()
}

func (s *ComplianceStore) AcknowledgeWarning(ctx context.Context, warningID string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE compliance_renewal_warnings
		SET acknowledged_at = NOW()
		WHERE id = $1
	`, warningID)
	if err != nil {
		return fmt.Errorf("acknowledge warning: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrComplianceRenewalNotFound
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanComplianceItem(row scanner) (*ComplianceItem, error) {
	var item ComplianceItem
	var evidenceJSON []byte
	var renewedFromID, holdID *string
	err := row.Scan(
		&item.ID, &item.PropertyID, &item.TenantID, &item.Kind, &item.Severity,
		&item.Name, &item.Description, &item.EffectiveDate, &item.ExpiryDate,
		&item.Status, &evidenceJSON, &renewedFromID, &holdID,
		&item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrComplianceItemNotFound
		}
		return nil, fmt.Errorf("scan compliance item: %w", err)
	}
	if err := json.Unmarshal(evidenceJSON, &item.EvidenceIDs); err != nil {
		item.EvidenceIDs = []string{}
	}
	item.RenewedFromID = renewedFromID
	item.HoldID = holdID
	return &item, nil
}
