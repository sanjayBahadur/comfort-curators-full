package consumer

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// EnsureSchema creates the consumer module's owned tables:
// consumer_disclosures, consumer_acceptances, and consumer_history_exports.
// The history export reads tenant-scoped rows from the catalog, billing and
// operations tables that already exist in the monolith.
func EnsureSchema(ctx context.Context, db *pgxpool.Pool) error {
	ddl := []string{
		`CREATE TABLE IF NOT EXISTS consumer_disclosures (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			property_id TEXT NOT NULL DEFAULT '',
			resource_type TEXT NOT NULL,
			resource_id TEXT NOT NULL,
			price_minor_units BIGINT NOT NULL DEFAULT 0,
			tax_minor_units BIGINT NOT NULL DEFAULT 0,
			currency TEXT NOT NULL,
			recurrence TEXT NOT NULL DEFAULT 'one_time',
			recurrence_amount_minor_units BIGINT,
			substitution_policy TEXT NOT NULL DEFAULT '',
			cancellation_policy TEXT NOT NULL DEFAULT '',
			refund_policy TEXT NOT NULL DEFAULT '',
			seller TEXT NOT NULL DEFAULT '',
			country_of_origin TEXT NOT NULL DEFAULT '',
			grievance_contact TEXT NOT NULL DEFAULT '',
			recurring_cost_visible BOOLEAN NOT NULL DEFAULT FALSE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_consumer_disclosures_tenant
		 ON consumer_disclosures(tenant_id, resource_type)`,
		`CREATE INDEX IF NOT EXISTS idx_consumer_disclosures_resource
		 ON consumer_disclosures(tenant_id, resource_type, resource_id)`,

		`CREATE TABLE IF NOT EXISTS consumer_acceptances (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			property_id TEXT NOT NULL DEFAULT '',
			disclosure_id TEXT NOT NULL,
			resource_type TEXT NOT NULL,
			resource_id TEXT NOT NULL,
			accepted_by TEXT NOT NULL DEFAULT '',
			accepted_at TIMESTAMPTZ NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_consumer_acceptances_tenant
		 ON consumer_acceptances(tenant_id, resource_type, resource_id)`,

		`CREATE TABLE IF NOT EXISTS consumer_history_exports (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			property_id TEXT NOT NULL DEFAULT '',
			requested_by TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'completed',
			data JSONB NOT NULL DEFAULT '{}',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_consumer_history_exports_tenant
		 ON consumer_history_exports(tenant_id)`,
	}

	for _, stmt := range ddl {
		if _, err := db.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("consumer schema: %w", err)
		}
	}

	return nil
}
