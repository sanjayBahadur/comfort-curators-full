package catalog

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// EnsureSchema creates the catalog module's owned tables. The tables match the
// protected catalog ownership list: catalog_items, catalog_claim_evidence,
// package_templates, property_package_versions, and property_package_items.
func EnsureSchema(ctx context.Context, db *pgxpool.Pool) error {
	ddl := []string{
		`CREATE TABLE IF NOT EXISTS catalog_items (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			sku TEXT NOT NULL,
			name TEXT NOT NULL,
			category TEXT NOT NULL,
			brand TEXT NOT NULL DEFAULT '',
			pack_size TEXT NOT NULL DEFAULT '',
			unit_cost_minor_units BIGINT NOT NULL,
			unit_cost_currency TEXT NOT NULL,
			owner_price_minor_units BIGINT NOT NULL,
			owner_price_currency TEXT NOT NULL,
			tax_class TEXT NOT NULL DEFAULT '',
			supplier TEXT NOT NULL DEFAULT '',
			country_of_origin TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'active',
			shelf_life_rule TEXT NOT NULL DEFAULT '',
			substitution_group TEXT NOT NULL DEFAULT '',
			operational_suitability TEXT NOT NULL DEFAULT '',
			label TEXT NOT NULL,
			version INT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (tenant_id, sku)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_items_tenant
		 ON catalog_items(tenant_id, status)`,

		`CREATE TABLE IF NOT EXISTS catalog_claim_evidence (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			catalog_item_id TEXT NOT NULL,
			claim_type TEXT NOT NULL,
			claim_statement TEXT NOT NULL DEFAULT '',
			evidence_ref TEXT NOT NULL,
			evidence_retained_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_claim_evidence_item
		 ON catalog_claim_evidence(tenant_id, catalog_item_id)`,

		`CREATE TABLE IF NOT EXISTS package_templates (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'active',
			items JSONB NOT NULL DEFAULT '[]'::jsonb,
			version INT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_package_templates_tenant
		 ON package_templates(tenant_id, status)`,

		`CREATE TABLE IF NOT EXISTS property_package_versions (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			property_id TEXT NOT NULL,
			version_number INT NOT NULL,
			status TEXT NOT NULL DEFAULT 'draft',
			effective_date TIMESTAMPTZ NOT NULL,
			monthly_budget_limit_minor_units BIGINT,
			substitution_policy TEXT NOT NULL DEFAULT 'owner_approval',
			require_approval_for_price_increase BOOLEAN NOT NULL DEFAULT FALSE,
			require_approval_for_new_sku BOOLEAN NOT NULL DEFAULT FALSE,
			setup_cost_minor_units BIGINT NOT NULL DEFAULT 0,
			monthly_cost_minor_units BIGINT NOT NULL DEFAULT 0,
			monthly_consumption_units BIGINT NOT NULL DEFAULT 0,
			currency TEXT NOT NULL,
			review_summary JSONB NOT NULL,
			created_by TEXT NOT NULL DEFAULT '',
			activated_at TIMESTAMPTZ,
			version INT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (tenant_id, property_id, version_number)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_package_versions_property
		 ON property_package_versions(tenant_id, property_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_package_versions_property_number
		 ON property_package_versions(tenant_id, property_id, version_number)`,

		`CREATE TABLE IF NOT EXISTS property_package_items (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			package_version_id TEXT NOT NULL,
			catalog_item_id TEXT NOT NULL,
			sku TEXT NOT NULL,
			name TEXT NOT NULL DEFAULT '',
			label TEXT NOT NULL DEFAULT '',
			substitution_group TEXT NOT NULL DEFAULT '',
			quantity INT NOT NULL,
			order_index INT NOT NULL DEFAULT 0,
			expected_monthly_consumption INT NOT NULL DEFAULT 0,
			setup_cost_minor_units BIGINT NOT NULL DEFAULT 0,
			monthly_cost_minor_units BIGINT NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_package_items_version
		 ON property_package_items(tenant_id, package_version_id, order_index)`,
	}

	for _, stmt := range ddl {
		if _, err := db.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("catalog schema: %w", err)
		}
	}

	return nil
}
