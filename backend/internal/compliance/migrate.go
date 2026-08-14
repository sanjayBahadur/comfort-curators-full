package compliance

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func EnsureSchema(ctx context.Context, db *pgxpool.Pool) error {
	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS compliance_items (
			id TEXT PRIMARY KEY,
			property_id TEXT NOT NULL,
			tenant_id TEXT NOT NULL,
			kind TEXT NOT NULL,
			severity TEXT NOT NULL,
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			effective_date TIMESTAMPTZ NOT NULL,
			expiry_date TIMESTAMPTZ NOT NULL,
			status TEXT NOT NULL DEFAULT 'active',
			evidence_ids JSONB NOT NULL DEFAULT '[]',
			renewed_from_id TEXT,
			hold_id TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("compliance: create compliance_items table: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_compliance_items_property
			ON compliance_items(property_id)
	`); err != nil {
		return fmt.Errorf("compliance: create compliance_items property index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_compliance_items_tenant
			ON compliance_items(tenant_id)
	`); err != nil {
		return fmt.Errorf("compliance: create compliance_items tenant index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_compliance_items_expiry
			ON compliance_items(status, expiry_date)
	`); err != nil {
		return fmt.Errorf("compliance: create compliance_items expiry index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS compliance_renewal_warnings (
			id TEXT PRIMARY KEY,
			item_id TEXT NOT NULL,
			property_id TEXT NOT NULL,
			tenant_id TEXT NOT NULL,
			days_before_expiry INTEGER NOT NULL,
			issued_at TIMESTAMPTZ NOT NULL,
			acknowledged_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("compliance: create compliance_renewal_warnings table: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_renewal_warnings_item
			ON compliance_renewal_warnings(item_id)
	`); err != nil {
		return fmt.Errorf("compliance: create renewal_warnings item index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_renewal_warnings_property
			ON compliance_renewal_warnings(property_id)
	`); err != nil {
		return fmt.Errorf("compliance: create renewal_warnings property index: %w", err)
	}

	return nil
}
