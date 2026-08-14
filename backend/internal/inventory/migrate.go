package inventory

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func EnsureSchema(ctx context.Context, db *pgxpool.Pool) error {
	ddl := []string{
		`CREATE TABLE IF NOT EXISTS stock_locations (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			property_id TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL,
			location_type TEXT NOT NULL,
			version BIGINT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_stock_locations_tenant
		 ON stock_locations(tenant_id, location_type)`,
		`CREATE INDEX IF NOT EXISTS idx_stock_locations_property
		 ON stock_locations(tenant_id, property_id)`,

		`CREATE TABLE IF NOT EXISTS inventory_movements (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			location_id TEXT NOT NULL,
			catalog_item_id TEXT NOT NULL,
			movement_type TEXT NOT NULL,
			quantity BIGINT NOT NULL,
			reference_type TEXT NOT NULL DEFAULT '',
			reference_id TEXT NOT NULL DEFAULT '',
			reason TEXT NOT NULL DEFAULT '',
			actor_id TEXT NOT NULL DEFAULT '',
			expires_at TIMESTAMPTZ DEFAULT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_inventory_movements_balance
		 ON inventory_movements(tenant_id, location_id, catalog_item_id, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_inventory_movements_location
		 ON inventory_movements(tenant_id, location_id, created_at)`,

		`CREATE TABLE IF NOT EXISTS inventory_counts (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			location_id TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'draft',
			counted_by TEXT NOT NULL DEFAULT '',
			reviewed_by TEXT NOT NULL DEFAULT '',
			version BIGINT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_inventory_counts_location
		 ON inventory_counts(tenant_id, location_id)`,

		`CREATE TABLE IF NOT EXISTS inventory_count_lines (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			count_id TEXT NOT NULL,
			catalog_item_id TEXT NOT NULL,
			expected_quantity BIGINT NOT NULL DEFAULT 0,
			counted_quantity BIGINT NOT NULL DEFAULT 0,
			variance BIGINT NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_inventory_count_lines_count
		 ON inventory_count_lines(tenant_id, count_id)`,
	}

	for _, stmt := range ddl {
		if _, err := db.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("inventory schema: %w", err)
		}
	}

	if _, err := db.Exec(ctx, `
		CREATE OR REPLACE FUNCTION inventory_movements_immutable()
		RETURNS TRIGGER AS $$
		BEGIN
			RAISE EXCEPTION 'inventory_movements are immutable: UPDATE and DELETE are not allowed';
		END;
		$$ LANGUAGE plpgsql
	`); err != nil {
		return fmt.Errorf("inventory schema: create immutable trigger function: %w", err)
	}

	if _, err := db.Exec(ctx, `
		DROP TRIGGER IF EXISTS inventory_movements_no_update ON inventory_movements
	`); err != nil {
		return fmt.Errorf("inventory schema: drop old trigger: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE TRIGGER inventory_movements_no_update
		BEFORE UPDATE OR DELETE ON inventory_movements
		FOR EACH STATEMENT
		EXECUTE FUNCTION inventory_movements_immutable()
	`); err != nil {
		return fmt.Errorf("inventory schema: create immutable trigger: %w", err)
	}

	return nil
}
