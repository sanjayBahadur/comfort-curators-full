package reporting

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// EnsureSchema creates the reporting module's owned tables:
// report_snapshots (rebuildable read models computed from source
// transactions) and metric_observations (append-only worker development
// metrics). Both are projections; they are rebuildable and never become
// transaction authority (contracts/database/table_ownership.yaml).
func EnsureSchema(ctx context.Context, db *pgxpool.Pool) error {
	ddl := []string{
		`CREATE TABLE IF NOT EXISTS report_snapshots (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			property_id TEXT NOT NULL DEFAULT '',
			kind TEXT NOT NULL,
			period_start TIMESTAMPTZ DEFAULT NULL,
			period_end TIMESTAMPTZ DEFAULT NULL,
			source_count BIGINT NOT NULL DEFAULT 0,
			source_hash TEXT NOT NULL DEFAULT '',
			data JSONB NOT NULL DEFAULT '{}',
			built_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			version BIGINT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		// One snapshot per (tenant, kind, property, period). A NULL period
		// (all-time projection) is folded onto -infinity/+infinity so the
		// rebuild of an all-time projection is idempotent too.
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_report_snapshots_key
		 ON report_snapshots(tenant_id, kind, property_id,
			COALESCE(period_start, TIMESTAMPTZ '-infinity'),
			COALESCE(period_end, TIMESTAMPTZ 'infinity'))`,
		`CREATE INDEX IF NOT EXISTS idx_report_snapshots_tenant
		 ON report_snapshots(tenant_id, kind, property_id)`,

		`CREATE TABLE IF NOT EXISTS metric_observations (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			property_id TEXT NOT NULL DEFAULT '',
			worker_id TEXT NOT NULL,
			metric_kind TEXT NOT NULL,
			value BIGINT NOT NULL DEFAULT 0,
			unit TEXT NOT NULL DEFAULT '',
			period_start TIMESTAMPTZ DEFAULT NULL,
			period_end TIMESTAMPTZ DEFAULT NULL,
			source_ref TEXT NOT NULL DEFAULT '',
			recorded_by TEXT NOT NULL DEFAULT '',
			recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_metric_observations_tenant
		 ON metric_observations(tenant_id, property_id, worker_id, recorded_at)`,
	}

	for _, stmt := range ddl {
		if _, err := db.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("reporting schema: %w", err)
		}
	}

	return nil
}
