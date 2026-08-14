package access

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func EnsureSchema(ctx context.Context, db *pgxpool.Pool) error {
	ddl := []string{
		`CREATE TABLE IF NOT EXISTS property_access_secrets (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			property_id TEXT NOT NULL,
			secret_type TEXT NOT NULL,
			label TEXT NOT NULL DEFAULT '',
			encrypted_value TEXT NOT NULL,
			encryption_key_id TEXT NOT NULL DEFAULT '',
			metadata TEXT NOT NULL DEFAULT '',
			version INT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_property_access_secrets_property
		 ON property_access_secrets(tenant_id, property_id, secret_type)`,

		`CREATE TABLE IF NOT EXISTS access_grants (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			property_id TEXT NOT NULL,
			secret_id TEXT NOT NULL,
			grantee_id TEXT NOT NULL,
			granter_id TEXT NOT NULL,
			window_start TIMESTAMPTZ NOT NULL,
			window_end TIMESTAMPTZ NOT NULL,
			reason TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'active',
			acknowledged_at TIMESTAMPTZ DEFAULT NULL,
			returned_at TIMESTAMPTZ DEFAULT NULL,
			revoked_at TIMESTAMPTZ DEFAULT NULL,
			revoked_by TEXT NOT NULL DEFAULT '',
			revoke_reason TEXT NOT NULL DEFAULT '',
			is_emergency BOOLEAN NOT NULL DEFAULT FALSE,
			emergency_reason TEXT NOT NULL DEFAULT '',
			version INT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_access_grants_tenant
		 ON access_grants(tenant_id, property_id, grantee_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_access_grants_window
		 ON access_grants(window_start, window_end)`,

		`CREATE TABLE IF NOT EXISTS access_disclosures (
			id TEXT PRIMARY KEY,
			grant_id TEXT NOT NULL,
			tenant_id TEXT NOT NULL,
			property_id TEXT NOT NULL,
			secret_id TEXT NOT NULL,
			requestor_id TEXT NOT NULL,
			result TEXT NOT NULL,
			denial_reason TEXT NOT NULL DEFAULT '',
			disclosed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_access_disclosures_grant
		 ON access_disclosures(grant_id, tenant_id)`,
		`CREATE INDEX IF NOT EXISTS idx_access_disclosures_time
		 ON access_disclosures(tenant_id, disclosed_at)`,

		`CREATE TABLE IF NOT EXISTS access_custody_events (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			property_id TEXT NOT NULL,
			grant_id TEXT DEFAULT NULL,
			secret_id TEXT DEFAULT NULL,
			event_type TEXT NOT NULL,
			actor_id TEXT NOT NULL,
			grantee_id TEXT DEFAULT NULL,
			reason TEXT NOT NULL DEFAULT '',
			metadata TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_access_custody_events_tenant
		 ON access_custody_events(tenant_id, property_id, created_at)`,

		`CREATE TABLE IF NOT EXISTS access_holds (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			property_id TEXT NOT NULL,
			reason TEXT NOT NULL DEFAULT '',
			placed_by TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'active',
			released_at TIMESTAMPTZ DEFAULT NULL,
			released_by TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_access_holds_tenant
		 ON access_holds(tenant_id, property_id, status)`,
	}

	for _, stmt := range ddl {
		if _, err := db.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("access schema: %w", err)
		}
	}

	return nil
}
