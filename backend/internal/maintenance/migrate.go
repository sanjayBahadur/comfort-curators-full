package maintenance

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// EnsureSchema creates the maintenance module's owned tables. The tables match
// the module ownership contract and follow tenant-scoped, property-scoped,
// versioned and append-only patterns.
func EnsureSchema(ctx context.Context, db *pgxpool.Pool) error {
	ddl := []string{
		`CREATE TABLE IF NOT EXISTS maintenance_requests (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			property_id TEXT NOT NULL DEFAULT '',
			title TEXT NOT NULL DEFAULT '',
			category TEXT NOT NULL DEFAULT '',
			priority TEXT NOT NULL DEFAULT 'normal',
			risk_level TEXT NOT NULL DEFAULT 'standard',
			status TEXT NOT NULL DEFAULT 'reported',
			reported_by TEXT NOT NULL DEFAULT '',
			triaged_by TEXT NOT NULL DEFAULT '',
			triaged_at TIMESTAMPTZ DEFAULT NULL,
			estimate_id TEXT NOT NULL DEFAULT '',
			notes TEXT NOT NULL DEFAULT '',
			version BIGINT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_maintenance_requests_tenant ON maintenance_requests(tenant_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_maintenance_requests_property ON maintenance_requests(tenant_id, property_id)`,

		`CREATE TABLE IF NOT EXISTS maintenance_estimates (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			request_id TEXT NOT NULL DEFAULT '',
			property_id TEXT NOT NULL DEFAULT '',
			prepared_by TEXT NOT NULL DEFAULT '',
			amount_minor_units BIGINT NOT NULL DEFAULT 0,
			currency TEXT NOT NULL DEFAULT 'INR',
			scope TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'draft',
			submitted_at TIMESTAMPTZ DEFAULT NULL,
			approved_by TEXT NOT NULL DEFAULT '',
			approved_at TIMESTAMPTZ DEFAULT NULL,
			rejected_by TEXT NOT NULL DEFAULT '',
			rejected_at TIMESTAMPTZ DEFAULT NULL,
			version BIGINT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_maintenance_estimates_request ON maintenance_estimates(tenant_id, request_id)`,

		`CREATE TABLE IF NOT EXISTS maintenance_approvals (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			request_id TEXT NOT NULL DEFAULT '',
			estimate_id TEXT NOT NULL DEFAULT '',
			actor_id TEXT NOT NULL DEFAULT '',
			decision TEXT NOT NULL DEFAULT '',
			reason TEXT NOT NULL DEFAULT '',
			is_ai_actor BOOLEAN NOT NULL DEFAULT false,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_maintenance_approvals_estimate ON maintenance_approvals(tenant_id, estimate_id)`,

		`CREATE TABLE IF NOT EXISTS vendor_work_orders (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			request_id TEXT NOT NULL DEFAULT '',
			estimate_id TEXT NOT NULL DEFAULT '',
			property_id TEXT NOT NULL DEFAULT '',
			vendor_id TEXT NOT NULL DEFAULT '',
			scope TEXT NOT NULL DEFAULT '',
			risk_level TEXT NOT NULL DEFAULT 'standard',
			status TEXT NOT NULL DEFAULT 'assigned',
			assigned_by TEXT NOT NULL DEFAULT '',
			assigned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			started_at TIMESTAMPTZ DEFAULT NULL,
			completed_by TEXT NOT NULL DEFAULT '',
			completed_at TIMESTAMPTZ DEFAULT NULL,
			completion_evidence_ref TEXT NOT NULL DEFAULT '',
			verified_by TEXT NOT NULL DEFAULT '',
			verified_at TIMESTAMPTZ DEFAULT NULL,
			version BIGINT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_vendor_work_orders_vendor ON vendor_work_orders(tenant_id, vendor_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_vendor_work_orders_request ON vendor_work_orders(tenant_id, request_id)`,

		`CREATE TABLE IF NOT EXISTS warranty_records (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			work_order_id TEXT NOT NULL DEFAULT '',
			property_id TEXT NOT NULL DEFAULT '',
			vendor_id TEXT NOT NULL DEFAULT '',
			provider TEXT NOT NULL DEFAULT '',
			coverage TEXT NOT NULL DEFAULT '',
			expires_at TIMESTAMPTZ DEFAULT NULL,
			status TEXT NOT NULL DEFAULT 'active',
			recorded_by TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_warranty_records_property ON warranty_records(tenant_id, property_id)`,
		`CREATE INDEX IF NOT EXISTS idx_warranty_records_work_order ON warranty_records(tenant_id, work_order_id)`,
	}

	for _, stmt := range ddl {
		if _, err := db.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("maintenance schema: %w", err)
		}
	}

	return nil
}
