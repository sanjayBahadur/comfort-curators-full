package fleet

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func EnsureSchema(ctx context.Context, db *pgxpool.Pool) error {
	ddl := []string{
		`CREATE TABLE IF NOT EXISTS fleet_assets (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			model TEXT NOT NULL,
			serial_number TEXT NOT NULL,
			rated_motor_power_watts INT NOT NULL,
			maximum_design_speed_kmh INT NOT NULL,
			design_speed_evidence_ref TEXT NOT NULL DEFAULT '',
			compliance_document_ref TEXT NOT NULL DEFAULT '',
			battery_serial TEXT NOT NULL DEFAULT '',
			charger TEXT NOT NULL DEFAULT '',
			purchase_date TIMESTAMPTZ NOT NULL,
			warranty_expires_at TIMESTAMPTZ DEFAULT NULL,
			warranty_terms TEXT NOT NULL DEFAULT '',
			assigned_custodian_id TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'available',
			version INT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_fleet_assets_tenant
		 ON fleet_assets(tenant_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_fleet_assets_custodian
		 ON fleet_assets(tenant_id, assigned_custodian_id)`,

		`CREATE TABLE IF NOT EXISTS fleet_batteries (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			asset_id TEXT NOT NULL,
			battery_serial TEXT NOT NULL,
			health_status TEXT NOT NULL DEFAULT 'ok',
			cycle_count INT NOT NULL DEFAULT 0,
			last_service_at TIMESTAMPTZ DEFAULT NULL,
			next_service_due_at TIMESTAMPTZ DEFAULT NULL,
			status TEXT NOT NULL DEFAULT 'active',
			version INT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_fleet_batteries_asset
		 ON fleet_batteries(tenant_id, asset_id)`,

		`CREATE TABLE IF NOT EXISTS fleet_custody_events (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			asset_id TEXT NOT NULL,
			event_type TEXT NOT NULL,
			from_worker_id TEXT NOT NULL DEFAULT '',
			to_worker_id TEXT NOT NULL DEFAULT '',
			condition TEXT NOT NULL DEFAULT '',
			accessories TEXT NOT NULL DEFAULT '',
			acknowledged_by TEXT NOT NULL DEFAULT '',
			acknowledged_at TIMESTAMPTZ DEFAULT NULL,
			notes TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_fleet_custody_events_asset
		 ON fleet_custody_events(tenant_id, asset_id, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_fleet_custody_events_worker
		 ON fleet_custody_events(tenant_id, to_worker_id, created_at)`,

		`CREATE TABLE IF NOT EXISTS fleet_inspections (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			asset_id TEXT NOT NULL,
			worker_id TEXT NOT NULL,
			inspection_type TEXT NOT NULL DEFAULT 'pre_use',
			result TEXT NOT NULL,
			damage_reported BOOLEAN NOT NULL DEFAULT FALSE,
			damage_description TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_fleet_inspections_asset
		 ON fleet_inspections(tenant_id, asset_id, created_at)`,

		`CREATE TABLE IF NOT EXISTS fleet_maintenance (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			asset_id TEXT NOT NULL,
			kind TEXT NOT NULL,
			title TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			due_at TIMESTAMPTZ DEFAULT NULL,
			completed_at TIMESTAMPTZ DEFAULT NULL,
			status TEXT NOT NULL DEFAULT 'open',
			service_provider TEXT NOT NULL DEFAULT '',
			performed_by TEXT NOT NULL DEFAULT '',
			notes TEXT NOT NULL DEFAULT '',
			version INT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_fleet_maintenance_asset
		 ON fleet_maintenance(tenant_id, asset_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_fleet_maintenance_overdue
		 ON fleet_maintenance(tenant_id, status, due_at)`,

		`CREATE TABLE IF NOT EXISTS fleet_incidents (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			asset_id TEXT NOT NULL,
			kind TEXT NOT NULL DEFAULT '',
			severity TEXT NOT NULL,
			description TEXT NOT NULL,
			reported_by TEXT NOT NULL,
			safety_ticket_id TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'open',
			reviewed_by TEXT NOT NULL DEFAULT '',
			reviewed_at TIMESTAMPTZ DEFAULT NULL,
			resolution TEXT NOT NULL DEFAULT '',
			version INT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_fleet_incidents_asset
		 ON fleet_incidents(tenant_id, asset_id, status)`,

		`CREATE TABLE IF NOT EXISTS fleet_tracking_events (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			asset_id TEXT NOT NULL,
			worker_id TEXT NOT NULL,
			custody_event_id TEXT NOT NULL DEFAULT '',
			latitude DOUBLE PRECISION NOT NULL,
			longitude DOUBLE PRECISION NOT NULL,
			captured_at TIMESTAMPTZ NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_fleet_tracking_events_worker
		 ON fleet_tracking_events(tenant_id, worker_id, captured_at)`,
		`CREATE INDEX IF NOT EXISTS idx_fleet_tracking_events_asset
		 ON fleet_tracking_events(tenant_id, asset_id, captured_at)`,
	}

	for _, stmt := range ddl {
		if _, err := db.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("fleet schema: %w", err)
		}
	}

	return nil
}
