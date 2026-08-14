package fleet

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

type FleetStore struct {
	pool *pgxpool.Pool
}

func NewFleetStore(pool *pgxpool.Pool) *FleetStore {
	return &FleetStore{pool: pool}
}

const assetColumns = `id, tenant_id, model, serial_number, rated_motor_power_watts,
	maximum_design_speed_kmh, design_speed_evidence_ref, compliance_document_ref,
	battery_serial, charger, purchase_date, warranty_expires_at, warranty_terms,
	assigned_custodian_id, status, version, created_at, updated_at`

func scanAsset(row pgx.Row) (*FleetAsset, error) {
	var a FleetAsset
	var warrantyExpiresAt *time.Time
	err := row.Scan(
		&a.ID, &a.TenantID, &a.Model, &a.SerialNumber,
		&a.RatedMotorPowerWatts, &a.MaximumDesignSpeedKmh,
		&a.DesignSpeedEvidenceRef, &a.ComplianceDocumentRef,
		&a.BatterySerial, &a.Charger, &a.PurchaseDate,
		&warrantyExpiresAt, &a.WarrantyTerms,
		&a.AssignedCustodianID, &a.Status, &a.Version,
		&a.CreatedAt, &a.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrAssetNotFound
	}
	if err != nil {
		return nil, err
	}
	if warrantyExpiresAt != nil {
		a.WarrantyExpiresAt = warrantyExpiresAt
	}
	return &a, nil
}

func (s *FleetStore) InsertAsset(ctx context.Context, q querier, a *FleetAsset) error {
	a.ID = newID("ast")
	_, err := q.Exec(ctx, `
		INSERT INTO fleet_assets (
			id, tenant_id, model, serial_number, rated_motor_power_watts,
			maximum_design_speed_kmh, design_speed_evidence_ref, compliance_document_ref,
			battery_serial, charger, purchase_date, warranty_expires_at, warranty_terms,
			assigned_custodian_id, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`, a.ID, a.TenantID, a.Model, a.SerialNumber, a.RatedMotorPowerWatts,
		a.MaximumDesignSpeedKmh, a.DesignSpeedEvidenceRef, a.ComplianceDocumentRef,
		a.BatterySerial, a.Charger, a.PurchaseDate, nullTime(a.WarrantyExpiresAt),
		a.WarrantyTerms, a.AssignedCustodianID, a.Status)
	return err
}

func (s *FleetStore) GetAsset(ctx context.Context, tenantID, assetID string) (*FleetAsset, error) {
	return scanAsset(s.pool.QueryRow(ctx, `
		SELECT `+assetColumns+`
		FROM fleet_assets
		WHERE id = $1 AND tenant_id = $2
	`, assetID, tenantID))
}

func (s *FleetStore) GetAssetByCustodian(ctx context.Context, tenantID, workerID, assetID string) (*FleetAsset, error) {
	return scanAsset(s.pool.QueryRow(ctx, `
		SELECT `+assetColumns+`
		FROM fleet_assets
		WHERE id = $1 AND tenant_id = $2 AND assigned_custodian_id = $3
	`, assetID, tenantID, workerID))
}

func (s *FleetStore) GetActiveCustodyAsset(ctx context.Context, tenantID, workerID string) (*FleetAsset, error) {
	return scanAsset(s.pool.QueryRow(ctx, `
		SELECT `+assetColumns+`
		FROM fleet_assets
		WHERE tenant_id = $1 AND assigned_custodian_id = $2
		ORDER BY updated_at DESC
		LIMIT 1
	`, tenantID, workerID))
}

func (s *FleetStore) ListAssets(ctx context.Context, tenantID string) ([]FleetAsset, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+assetColumns+`
		FROM fleet_assets
		WHERE tenant_id = $1
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []FleetAsset
	for rows.Next() {
		a, err := scanAsset(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

func (s *FleetStore) FreezeAsset(ctx context.Context, q querier, tenantID, assetID string) (*FleetAsset, error) {
	return scanAsset(q.QueryRow(ctx, `
		UPDATE fleet_assets
		SET status = $3, updated_at = NOW(), version = version + 1
		WHERE id = $1 AND tenant_id = $2
		RETURNING `+assetColumns+`
	`, assetID, tenantID, AssetStatusFrozen))
}

func (s *FleetStore) UnfreezeAsset(ctx context.Context, q querier, tenantID, assetID string) (*FleetAsset, error) {
	return scanAsset(q.QueryRow(ctx, `
		UPDATE fleet_assets
		SET status = $3, updated_at = NOW(), version = version + 1
		WHERE id = $1 AND tenant_id = $2 AND status = $4
		RETURNING `+assetColumns+`
	`, assetID, tenantID, AssetStatusAvailable, AssetStatusFrozen))
}

func (s *FleetStore) SetCustodian(ctx context.Context, q querier, tenantID, assetID, custodianID string) (*FleetAsset, error) {
	return scanAsset(q.QueryRow(ctx, `
		UPDATE fleet_assets
		SET assigned_custodian_id = $3, status = $4, updated_at = NOW(), version = version + 1
		WHERE id = $1 AND tenant_id = $2
		RETURNING `+assetColumns+`
	`, assetID, tenantID, custodianID, AssetStatusAssigned))
}

func (s *FleetStore) ClearCustodian(ctx context.Context, q querier, tenantID, assetID string) (*FleetAsset, error) {
	return scanAsset(q.QueryRow(ctx, `
		UPDATE fleet_assets
		SET assigned_custodian_id = '',
			status = CASE WHEN status = 'assigned' THEN 'available' ELSE status END,
			updated_at = NOW(), version = version + 1
		WHERE id = $1 AND tenant_id = $2
		RETURNING `+assetColumns+`
	`, assetID, tenantID))
}

func (s *FleetStore) InsertBattery(ctx context.Context, q querier, b *FleetBattery) error {
	b.ID = newID("bat")
	_, err := q.Exec(ctx, `
		INSERT INTO fleet_batteries (
			id, tenant_id, asset_id, battery_serial, health_status,
			cycle_count, last_service_at, next_service_due_at, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, b.ID, b.TenantID, b.AssetID, b.BatterySerial, b.HealthStatus,
		b.CycleCount, nullTime(b.LastServiceAt), nullTime(b.NextServiceDueAt), b.Status)
	return err
}

func (s *FleetStore) GetBattery(ctx context.Context, tenantID, batteryID string) (*FleetBattery, error) {
	var b FleetBattery
	var lastServiceAt, nextServiceDueAt *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, asset_id, battery_serial, health_status,
			cycle_count, last_service_at, next_service_due_at, status,
			version, created_at, updated_at
		FROM fleet_batteries
		WHERE id = $1 AND tenant_id = $2
	`, batteryID, tenantID).Scan(
		&b.ID, &b.TenantID, &b.AssetID, &b.BatterySerial, &b.HealthStatus,
		&b.CycleCount, &lastServiceAt, &nextServiceDueAt, &b.Status,
		&b.Version, &b.CreatedAt, &b.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrBatteryNotFound
	}
	if err != nil {
		return nil, err
	}
	if lastServiceAt != nil {
		b.LastServiceAt = lastServiceAt
	}
	if nextServiceDueAt != nil {
		b.NextServiceDueAt = nextServiceDueAt
	}
	return &b, nil
}

func (s *FleetStore) InsertCustodyEvent(ctx context.Context, q querier, e *FleetCustodyEvent) error {
	e.ID = newID("cst")
	_, err := q.Exec(ctx, `
		INSERT INTO fleet_custody_events (
			id, tenant_id, asset_id, event_type,
			from_worker_id, to_worker_id, condition, accessories,
			acknowledged_by, acknowledged_at, notes
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, e.ID, e.TenantID, e.AssetID, e.EventType,
		e.FromWorkerID, e.ToWorkerID, e.Condition, e.Accessories,
		e.AcknowledgedBy, nullTime(e.AcknowledgedAt), e.Notes)
	return err
}

func (s *FleetStore) LatestCustodyEvent(ctx context.Context, tenantID, assetID string) (*FleetCustodyEvent, error) {
	return scanCustodyEvent(s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, asset_id, event_type,
			from_worker_id, to_worker_id, condition, accessories,
			acknowledged_by, acknowledged_at, notes, created_at
		FROM fleet_custody_events
		WHERE tenant_id = $1 AND asset_id = $2
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`, tenantID, assetID))
}

func (s *FleetStore) ListCustodyEvents(ctx context.Context, tenantID, assetID string) ([]FleetCustodyEvent, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, asset_id, event_type,
			from_worker_id, to_worker_id, condition, accessories,
			acknowledged_by, acknowledged_at, notes, created_at
		FROM fleet_custody_events
		WHERE tenant_id = $1 AND asset_id = $2
		ORDER BY created_at ASC
	`, tenantID, assetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []FleetCustodyEvent
	for rows.Next() {
		e, err := scanCustodyEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

func scanCustodyEvent(row pgx.Row) (*FleetCustodyEvent, error) {
	var e FleetCustodyEvent
	var acknowledgedAt *time.Time
	err := row.Scan(
		&e.ID, &e.TenantID, &e.AssetID, &e.EventType,
		&e.FromWorkerID, &e.ToWorkerID, &e.Condition, &e.Accessories,
		&e.AcknowledgedBy, &acknowledgedAt, &e.Notes, &e.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrCustodyNotFound
	}
	if err != nil {
		return nil, err
	}
	if acknowledgedAt != nil {
		e.AcknowledgedAt = acknowledgedAt
	}
	return &e, nil
}

func (s *FleetStore) InsertInspection(ctx context.Context, q querier, i *FleetInspection) error {
	i.ID = newID("ins")
	_, err := q.Exec(ctx, `
		INSERT INTO fleet_inspections (
			id, tenant_id, asset_id, worker_id, inspection_type,
			result, damage_reported, damage_description
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, i.ID, i.TenantID, i.AssetID, i.WorkerID, i.InspectionType,
		i.Result, i.DamageReported, i.DamageDescription)
	return err
}

func (s *FleetStore) LatestInspection(ctx context.Context, tenantID, assetID string) (*FleetInspection, error) {
	var i FleetInspection
	err := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, asset_id, worker_id, inspection_type,
			result, damage_reported, damage_description, created_at
		FROM fleet_inspections
		WHERE tenant_id = $1 AND asset_id = $2
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`, tenantID, assetID).Scan(
		&i.ID, &i.TenantID, &i.AssetID, &i.WorkerID, &i.InspectionType,
		&i.Result, &i.DamageReported, &i.DamageDescription, &i.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrAssetNotFound
	}
	return &i, err
}

func (s *FleetStore) InsertMaintenanceRecord(ctx context.Context, q querier, r *FleetMaintenanceRecord) error {
	r.ID = newID("mnt")
	_, err := q.Exec(ctx, `
		INSERT INTO fleet_maintenance (
			id, tenant_id, asset_id, kind, title, description,
			due_at, completed_at, status, service_provider,
			performed_by, notes
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, r.ID, r.TenantID, r.AssetID, r.Kind, r.Title, r.Description,
		nullTime(r.DueAt), nullTime(r.CompletedAt), r.Status, r.ServiceProvider,
		r.PerformedBy, r.Notes)
	return err
}

func (s *FleetStore) GetMaintenanceRecord(ctx context.Context, tenantID, recordID string) (*FleetMaintenanceRecord, error) {
	return scanMaintenanceRecord(s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, asset_id, kind, title, description,
			due_at, completed_at, status, service_provider,
			performed_by, notes, version, created_at, updated_at
		FROM fleet_maintenance
		WHERE id = $1 AND tenant_id = $2
	`, recordID, tenantID))
}

func (s *FleetStore) CompleteMaintenanceRecord(ctx context.Context, q querier, tenantID, recordID string, completedAt time.Time, performedBy, notes string) (*FleetMaintenanceRecord, error) {
	return scanMaintenanceRecord(q.QueryRow(ctx, `
		UPDATE fleet_maintenance
		SET status = $3, completed_at = $4, performed_by = $5, notes = $6,
			updated_at = NOW(), version = version + 1
		WHERE id = $1 AND tenant_id = $2 AND status = 'open'
		RETURNING id, tenant_id, asset_id, kind, title, description,
			due_at, completed_at, status, service_provider,
			performed_by, notes, version, created_at, updated_at
	`, recordID, tenantID, ItemStatusCompleted, completedAt, performedBy, notes))
}

func (s *FleetStore) ListOverdueSafetyItems(ctx context.Context, tenantID, assetID string, now time.Time) ([]FleetMaintenanceRecord, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, asset_id, kind, title, description,
			due_at, completed_at, status, service_provider,
			performed_by, notes, version, created_at, updated_at
		FROM fleet_maintenance
		WHERE tenant_id = $1 AND asset_id = $2 AND status = 'open'
			AND due_at IS NOT NULL AND due_at < $3
		ORDER BY due_at ASC
	`, tenantID, assetID, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []FleetMaintenanceRecord
	for rows.Next() {
		r, err := scanMaintenanceRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

func scanMaintenanceRecord(row pgx.Row) (*FleetMaintenanceRecord, error) {
	var r FleetMaintenanceRecord
	var dueAt, completedAt *time.Time
	err := row.Scan(
		&r.ID, &r.TenantID, &r.AssetID, &r.Kind, &r.Title, &r.Description,
		&dueAt, &completedAt, &r.Status, &r.ServiceProvider,
		&r.PerformedBy, &r.Notes, &r.Version, &r.CreatedAt, &r.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSafetyItemNotFound
	}
	if err != nil {
		return nil, err
	}
	if dueAt != nil {
		r.DueAt = dueAt
	}
	if completedAt != nil {
		r.CompletedAt = completedAt
	}
	return &r, nil
}

func (s *FleetStore) InsertIncident(ctx context.Context, q querier, i *FleetIncident) error {
	i.ID = newID("inc")
	_, err := q.Exec(ctx, `
		INSERT INTO fleet_incidents (
			id, tenant_id, asset_id, kind, severity, description,
			reported_by, safety_ticket_id, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, i.ID, i.TenantID, i.AssetID, i.Kind, i.Severity, i.Description,
		i.ReportedBy, i.SafetyTicketID, i.Status)
	return err
}

func (s *FleetStore) GetIncident(ctx context.Context, tenantID, incidentID string) (*FleetIncident, error) {
	return scanIncident(s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, asset_id, kind, severity, description,
			reported_by, safety_ticket_id, status,
			reviewed_by, reviewed_at, resolution,
			version, created_at, updated_at
		FROM fleet_incidents
		WHERE id = $1 AND tenant_id = $2
	`, incidentID, tenantID))
}

func (s *FleetStore) ListOpenIncidents(ctx context.Context, tenantID, assetID string) ([]FleetIncident, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, asset_id, kind, severity, description,
			reported_by, safety_ticket_id, status,
			reviewed_by, reviewed_at, resolution,
			version, created_at, updated_at
		FROM fleet_incidents
		WHERE tenant_id = $1 AND asset_id = $2 AND status = 'open'
		ORDER BY created_at DESC
	`, tenantID, assetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []FleetIncident
	for rows.Next() {
		i, err := scanIncident(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *i)
	}
	return out, rows.Err()
}

func (s *FleetStore) CountOpenIncidents(ctx context.Context, q querier, tenantID, assetID string) (int, error) {
	var count int
	err := q.QueryRow(ctx, `
		SELECT COUNT(1)
		FROM fleet_incidents
		WHERE tenant_id = $1 AND asset_id = $2 AND status = 'open'
	`, tenantID, assetID).Scan(&count)
	return count, err
}

func (s *FleetStore) ResolveIncident(ctx context.Context, q querier, tenantID, incidentID string, resolution, reviewedBy string, reviewedAt time.Time) (*FleetIncident, error) {
	return scanIncident(q.QueryRow(ctx, `
		UPDATE fleet_incidents
		SET status = $3, resolution = $4, reviewed_by = $5, reviewed_at = $6,
			updated_at = NOW(), version = version + 1
		WHERE id = $1 AND tenant_id = $2 AND status = 'open'
		RETURNING id, tenant_id, asset_id, kind, severity, description,
			reported_by, safety_ticket_id, status,
			reviewed_by, reviewed_at, resolution,
			version, created_at, updated_at
	`, incidentID, tenantID, IncidentStatusResolved, resolution, reviewedBy, reviewedAt))
}

func scanIncident(row pgx.Row) (*FleetIncident, error) {
	var i FleetIncident
	var reviewedAt *time.Time
	err := row.Scan(
		&i.ID, &i.TenantID, &i.AssetID, &i.Kind, &i.Severity, &i.Description,
		&i.ReportedBy, &i.SafetyTicketID, &i.Status,
		&i.ReviewedBy, &reviewedAt, &i.Resolution,
		&i.Version, &i.CreatedAt, &i.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrIncidentNotFound
	}
	if err != nil {
		return nil, err
	}
	if reviewedAt != nil {
		i.ReviewedAt = reviewedAt
	}
	return &i, nil
}

func (s *FleetStore) InsertTrackingEvent(ctx context.Context, q querier, e *FleetTrackingEvent) error {
	e.ID = newID("trk")
	_, err := q.Exec(ctx, `
		INSERT INTO fleet_tracking_events (
			id, tenant_id, asset_id, worker_id, custody_event_id,
			latitude, longitude, captured_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, e.ID, e.TenantID, e.AssetID, e.WorkerID, e.CustodyEventID,
		e.Latitude, e.Longitude, e.CapturedAt)
	return err
}

func (s *FleetStore) CountTrackingEvents(ctx context.Context, tenantID, workerID string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(1)
		FROM fleet_tracking_events
		WHERE tenant_id = $1 AND worker_id = $2
	`, tenantID, workerID).Scan(&count)
	return count, err
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
