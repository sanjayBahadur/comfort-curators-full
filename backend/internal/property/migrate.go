package property

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func EnsureSchema(ctx context.Context, db *pgxpool.Pool) error {
	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS properties (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			owner_authority_id TEXT NOT NULL,
			service_address JSONB NOT NULL,
			geolocation_zone TEXT NOT NULL,
			timezone TEXT NOT NULL DEFAULT 'Asia/Kolkata',
			emergency_contacts JSONB NOT NULL DEFAULT '[]',
			access_method TEXT NOT NULL,
			maximum_occupancy INTEGER NOT NULL,
			state TEXT NOT NULL,
			owner_contract_accepted BOOLEAN NOT NULL DEFAULT false,
			compliance_complete BOOLEAN NOT NULL DEFAULT false,
			mandatory_fields_set BOOLEAN NOT NULL DEFAULT false,
			version INTEGER NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("property: create properties table: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_properties_tenant
			ON properties(tenant_id, state)
	`); err != nil {
		return fmt.Errorf("property: create properties tenant index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS property_compliance_holds (
			id TEXT PRIMARY KEY,
			property_id TEXT NOT NULL,
			tenant_id TEXT NOT NULL,
			kind TEXT NOT NULL,
			severity TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'open',
			reason TEXT NOT NULL,
			expires_at TIMESTAMPTZ,
			exception_by TEXT,
			exception_at TIMESTAMPTZ,
			exception_expires_at TIMESTAMPTZ,
			resolved_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("property: create compliance holds table: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_property_holds_property
			ON property_compliance_holds(property_id)
	`); err != nil {
		return fmt.Errorf("property: create holds property index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_property_holds_tenant
			ON property_compliance_holds(tenant_id)
	`); err != nil {
		return fmt.Errorf("property: create holds tenant index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS property_transitions (
			id TEXT PRIMARY KEY,
			property_id TEXT NOT NULL,
			tenant_id TEXT NOT NULL,
			from_state TEXT NOT NULL,
			to_state TEXT NOT NULL,
			actor_id TEXT NOT NULL,
			reason TEXT NOT NULL,
			from_version INTEGER NOT NULL,
			to_version INTEGER NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("property: create transitions table: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_property_transitions_property
			ON property_transitions(property_id, created_at)
	`); err != nil {
		return fmt.Errorf("property: create transitions index: %w", err)
	}

	// owner_authority_grants links an acting user to the owner authorities
	// they control. An actor is granted an authority the first time they
	// create a property under it; OwnerAuthorities resolvers consult this
	// table to decide whether an owner subject may reach a given property's
	// scoped records (finance, documents, contracts, reporting).
	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS owner_authority_grants (
			tenant_id TEXT NOT NULL,
			actor_id TEXT NOT NULL,
			authority_id TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (actor_id, authority_id)
		)
	`); err != nil {
		return fmt.Errorf("property: create owner authority grants table: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_owner_authority_grants_actor
			ON owner_authority_grants(actor_id)
	`); err != nil {
		return fmt.Errorf("property: create owner authority grants actor index: %w", err)
	}

	return nil
}
