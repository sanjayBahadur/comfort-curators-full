package audit

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func EnsureSchema(ctx context.Context, db *pgxpool.Pool) error {
	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS audit_events (
			id TEXT PRIMARY KEY,
			event_type TEXT NOT NULL,
			tenant_id TEXT,
			actor_id TEXT NOT NULL,
			action TEXT NOT NULL,
			resource_type TEXT NOT NULL DEFAULT '',
			resource_id TEXT NOT NULL DEFAULT '',
			previous_state JSONB,
			new_state JSONB,
			metadata JSONB,
			correlation_id TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("audit: create audit_events table: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_audit_events_tenant
			ON audit_events(tenant_id)
	`); err != nil {
		return fmt.Errorf("audit: create audit_events tenant index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_audit_events_actor
			ON audit_events(actor_id)
	`); err != nil {
		return fmt.Errorf("audit: create audit_events actor index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_audit_events_type
			ON audit_events(event_type)
	`); err != nil {
		return fmt.Errorf("audit: create audit_events event_type index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_audit_events_created_at
			ON audit_events(created_at)
	`); err != nil {
		return fmt.Errorf("audit: create audit_events created_at index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE OR REPLACE FUNCTION audit_no_update_delete()
		RETURNS TRIGGER AS $$
		BEGIN
			RAISE EXCEPTION 'audit_events are immutable: UPDATE and DELETE are not allowed';
		END;
		$$ LANGUAGE plpgsql
	`); err != nil {
		return fmt.Errorf("audit: create immutable trigger function: %w", err)
	}

	if _, err := db.Exec(ctx, `
		DROP TRIGGER IF EXISTS audit_events_no_update ON audit_events
	`); err != nil {
		return fmt.Errorf("audit: drop old trigger: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE TRIGGER audit_events_no_update
		BEFORE UPDATE OR DELETE ON audit_events
		FOR EACH STATEMENT
		EXECUTE FUNCTION audit_no_update_delete()
	`); err != nil {
		return fmt.Errorf("audit: create immutable trigger: %w", err)
	}

	return nil
}
