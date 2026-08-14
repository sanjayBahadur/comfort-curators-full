package onboarding

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func EnsureSchema(ctx context.Context, db *pgxpool.Pool) error {
	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS onboarding_cases (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			property_id TEXT NOT NULL,
			owner_authority_id TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'in_progress',
			portfolio JSONB,
			goals JSONB,
			service_preferences JSONB,
			budgets JSONB,
			contacts JSONB NOT NULL DEFAULT '[]',
			photographs JSONB NOT NULL DEFAULT '[]',
			amenities JSONB NOT NULL DEFAULT '[]',
			safety JSONB,
			furnishing JSONB,
			remediation JSONB,
			fit_score_inputs JSONB,
			version INTEGER NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("onboarding: create cases table: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_onboarding_cases_tenant
			ON onboarding_cases(tenant_id, status)
	`); err != nil {
		return fmt.Errorf("onboarding: create cases tenant index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_onboarding_cases_property
			ON onboarding_cases(property_id)
	`); err != nil {
		return fmt.Errorf("onboarding: create cases property index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS onboarding_evidence (
			id TEXT PRIMARY KEY,
			case_id TEXT NOT NULL,
			tenant_id TEXT NOT NULL,
			kind TEXT NOT NULL,
			content_hash TEXT NOT NULL,
			object_ref TEXT NOT NULL,
			captured_by TEXT NOT NULL,
			captured_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("onboarding: create evidence table: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_onboarding_evidence_case
			ON onboarding_evidence(case_id)
	`); err != nil {
		return fmt.Errorf("onboarding: create evidence case index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS onboarding_inspections (
			id TEXT PRIMARY KEY,
			case_id TEXT NOT NULL,
			tenant_id TEXT NOT NULL,
			property_id TEXT NOT NULL,
			performed_at TIMESTAMPTZ NOT NULL,
			inspected_by TEXT NOT NULL,
			evidence_hash TEXT NOT NULL,
			evidence_ref TEXT NOT NULL,
			findings TEXT NOT NULL,
			overall_status TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("onboarding: create inspections table: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_onboarding_inspections_case
			ON onboarding_inspections(case_id)
	`); err != nil {
		return fmt.Errorf("onboarding: create inspections case index: %w", err)
	}

	// Inspection evidence is immutable. No application role has an update or
	// delete path for inspection records, and the database enforces it so the
	// invariant holds even against direct SQL.
	if _, err := db.Exec(ctx, `
		CREATE OR REPLACE FUNCTION onboarding_inspection_immutable() RETURNS trigger AS $$
		BEGIN
			RAISE EXCEPTION 'onboarding inspection evidence is immutable';
		END;
		$$ LANGUAGE plpgsql
	`); err != nil {
		return fmt.Errorf("onboarding: create inspection immutable function: %w", err)
	}

	if _, err := db.Exec(ctx, `
		DROP TRIGGER IF EXISTS onboarding_inspections_no_update_delete ON onboarding_inspections
	`); err != nil {
		return fmt.Errorf("onboarding: drop inspection immutability trigger: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE TRIGGER onboarding_inspections_no_update_delete
			BEFORE UPDATE OR DELETE ON onboarding_inspections
			FOR EACH ROW EXECUTE FUNCTION onboarding_inspection_immutable()
	`); err != nil {
		return fmt.Errorf("onboarding: create inspection immutability trigger: %w", err)
	}

	return nil
}
