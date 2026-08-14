package contracts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func EnsureSchema(ctx context.Context, db *pgxpool.Pool) error {
	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS service_contracts (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			property_id TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'draft',
			current_version INTEGER NOT NULL DEFAULT 0,
			version INTEGER NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("contracts: create service_contracts table: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_service_contracts_tenant
			ON service_contracts(tenant_id, status)
	`); err != nil {
		return fmt.Errorf("contracts: create service_contracts tenant index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_service_contracts_property
			ON service_contracts(property_id)
	`); err != nil {
		return fmt.Errorf("contracts: create service_contracts property index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS service_contract_versions (
			id TEXT PRIMARY KEY,
			agreement_id TEXT NOT NULL,
			tenant_id TEXT NOT NULL,
			version_number INTEGER NOT NULL,
			content_hash TEXT NOT NULL,
			terms JSONB NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (agreement_id, version_number)
		)
	`); err != nil {
		return fmt.Errorf("contracts: create service_contract_versions table: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_service_contract_versions_agreement
			ON service_contract_versions(agreement_id)
	`); err != nil {
		return fmt.Errorf("contracts: create service_contract_versions agreement index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS contract_acceptances (
			id TEXT PRIMARY KEY,
			agreement_id TEXT NOT NULL,
			tenant_id TEXT NOT NULL,
			version_number INTEGER NOT NULL,
			content_hash TEXT NOT NULL,
			accepted_by TEXT NOT NULL,
			accepted_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("contracts: create contract_acceptances table: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_contract_acceptances_agreement
			ON contract_acceptances(agreement_id)
	`); err != nil {
		return fmt.Errorf("contracts: create contract_acceptances agreement index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS fee_rules (
			id TEXT PRIMARY KEY,
			rule_version TEXT NOT NULL,
			currency TEXT NOT NULL,
			service_tier TEXT NOT NULL,
			percentage_basis_points BIGINT NOT NULL,
			minimum_monthly_fee_minor_units BIGINT NOT NULL DEFAULT 0,
			setup_fee_minor_units BIGINT NOT NULL DEFAULT 0,
			effective_from TEXT,
			effective_to TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (rule_version, currency, service_tier)
		)
	`); err != nil {
		return fmt.Errorf("contracts: create fee_rules table: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_fee_rules_tier
			ON fee_rules(service_tier, currency, rule_version)
	`); err != nil {
		return fmt.Errorf("contracts: create fee_rules tier index: %w", err)
	}

	// Agreement version records are immutable. No application role has an
	// update or delete path for them, and the database enforces it so the
	// invariant holds even against direct SQL.
	if _, err := db.Exec(ctx, `
		CREATE OR REPLACE FUNCTION contracts_version_immutable() RETURNS trigger AS $$
		BEGIN
			RAISE EXCEPTION 'service agreement versions are immutable';
		END;
		$$ LANGUAGE plpgsql
	`); err != nil {
		return fmt.Errorf("contracts: create version immutable function: %w", err)
	}

	if _, err := db.Exec(ctx, `
		DROP TRIGGER IF EXISTS service_contract_versions_no_update_delete ON service_contract_versions
	`); err != nil {
		return fmt.Errorf("contracts: drop version immutability trigger: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE TRIGGER service_contract_versions_no_update_delete
			BEFORE UPDATE OR DELETE ON service_contract_versions
			FOR EACH ROW EXECUTE FUNCTION contracts_version_immutable()
	`); err != nil {
		return fmt.Errorf("contracts: create version immutability trigger: %w", err)
	}

	// Accepted agreement terms are immutable: once a contract is accepted its
	// version rows are locked at the database, so the accepted contract cannot
	// mutate through any path.
	if _, err := db.Exec(ctx, `
		CREATE OR REPLACE FUNCTION contracts_accepted_version_immutable() RETURNS trigger AS $$
		BEGIN
			IF EXISTS (
				SELECT 1 FROM service_contracts
				WHERE id = NEW.agreement_id AND status = 'accepted'
			) THEN
				RAISE EXCEPTION 'accepted service agreement is immutable';
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql
	`); err != nil {
		return fmt.Errorf("contracts: create accepted version immutable function: %w", err)
	}

	if _, err := db.Exec(ctx, `
		DROP TRIGGER IF EXISTS service_contract_versions_no_insert_accepted ON service_contract_versions
	`); err != nil {
		return fmt.Errorf("contracts: drop accepted version insert trigger: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE TRIGGER service_contract_versions_no_insert_accepted
			BEFORE INSERT ON service_contract_versions
			FOR EACH ROW EXECUTE FUNCTION contracts_accepted_version_immutable()
	`); err != nil {
		return fmt.Errorf("contracts: create accepted version insert trigger: %w", err)
	}

	return nil
}
