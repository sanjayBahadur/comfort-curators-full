package privacy

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func EnsureSchema(ctx context.Context, db *pgxpool.Pool) error {
	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS privacy_purposes (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			data_categories JSONB NOT NULL DEFAULT '[]',
			lawful_basis TEXT NOT NULL DEFAULT '',
			retention_period_days INTEGER NOT NULL DEFAULT 0,
			active BOOLEAN NOT NULL DEFAULT true,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("privacy: create privacy_purposes table: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_privacy_purposes_tenant
			ON privacy_purposes(tenant_id)
	`); err != nil {
		return fmt.Errorf("privacy: create purposes tenant index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS privacy_notices (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			actor_id TEXT NOT NULL,
			purpose_id TEXT NOT NULL DEFAULT '',
			notice_text TEXT NOT NULL DEFAULT '',
			version TEXT NOT NULL DEFAULT '1.0',
			language TEXT NOT NULL DEFAULT 'en',
			delivered_at TIMESTAMPTZ NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("privacy: create privacy_notices table: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_privacy_notices_tenant
			ON privacy_notices(tenant_id)
	`); err != nil {
		return fmt.Errorf("privacy: create notices tenant index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_privacy_notices_purpose
			ON privacy_notices(purpose_id)
	`); err != nil {
		return fmt.Errorf("privacy: create notices purpose index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS privacy_consents (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			actor_id TEXT NOT NULL,
			purpose_id TEXT NOT NULL,
			notice_id TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'active',
			lawful_basis TEXT NOT NULL DEFAULT 'consent',
			granted_at TIMESTAMPTZ NOT NULL,
			withdrawn_at TIMESTAMPTZ,
			expires_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("privacy: create privacy_consents table: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_privacy_consents_tenant
			ON privacy_consents(tenant_id)
	`); err != nil {
		return fmt.Errorf("privacy: create consents tenant index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_privacy_consents_purpose
			ON privacy_consents(purpose_id)
	`); err != nil {
		return fmt.Errorf("privacy: create consents purpose index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS privacy_rights_requests (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			actor_id TEXT NOT NULL,
			request_type TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			description TEXT NOT NULL DEFAULT '',
			related_data TEXT NOT NULL DEFAULT '',
			correction_data TEXT NOT NULL DEFAULT '',
			response_data TEXT NOT NULL DEFAULT '',
			block_reason TEXT NOT NULL DEFAULT '',
			reviewed_by TEXT NOT NULL DEFAULT '',
			reviewed_at TIMESTAMPTZ,
			completed_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("privacy: create privacy_rights_requests table: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_privacy_rights_tenant
			ON privacy_rights_requests(tenant_id)
	`); err != nil {
		return fmt.Errorf("privacy: create rights requests tenant index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_privacy_rights_status
			ON privacy_rights_requests(status)
	`); err != nil {
		return fmt.Errorf("privacy: create rights requests status index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS privacy_retention_records (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			actor_id TEXT NOT NULL DEFAULT '',
			record_type TEXT NOT NULL DEFAULT '',
			record_description TEXT NOT NULL DEFAULT '',
			lawful_basis TEXT NOT NULL DEFAULT '',
			retain_until TIMESTAMPTZ NOT NULL,
			status TEXT NOT NULL DEFAULT 'active',
			reason TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("privacy: create privacy_retention_records table: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_retention_records_tenant
			ON privacy_retention_records(tenant_id)
	`); err != nil {
		return fmt.Errorf("privacy: create retention records tenant index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS privacy_processor_contracts (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT '',
			vendor_name TEXT NOT NULL,
			vendor_contact TEXT NOT NULL DEFAULT '',
			contract_reference TEXT NOT NULL DEFAULT '',
			processing_scope TEXT NOT NULL DEFAULT '',
			data_categories JSONB NOT NULL DEFAULT '[]',
			security_review_status TEXT NOT NULL DEFAULT 'pending_review',
			security_review_date TIMESTAMPTZ,
			reviewer_id TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'pending_review',
			expires_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("privacy: create privacy_processor_contracts table: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_processor_contracts_tenant
			ON privacy_processor_contracts(tenant_id)
	`); err != nil {
		return fmt.Errorf("privacy: create processor contracts tenant index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS privacy_security_log_settings (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			region TEXT NOT NULL DEFAULT 'IN',
			retention_years INTEGER NOT NULL DEFAULT 5,
			incident_report_process TEXT NOT NULL DEFAULT '',
			active BOOLEAN NOT NULL DEFAULT true,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("privacy: create privacy_security_log_settings table: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS privacy_identity_alternatives (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			actor_id TEXT NOT NULL,
			identity_type TEXT NOT NULL,
			identity_value TEXT NOT NULL DEFAULT '',
			masked_value TEXT NOT NULL DEFAULT '',
			verification_hash TEXT NOT NULL DEFAULT '',
			verified BOOLEAN NOT NULL DEFAULT false,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("privacy: create privacy_identity_alternatives table: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_identity_alternatives_tenant
			ON privacy_identity_alternatives(tenant_id)
	`); err != nil {
		return fmt.Errorf("privacy: create identity alternatives tenant index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_identity_alternatives_actor
			ON privacy_identity_alternatives(actor_id)
	`); err != nil {
		return fmt.Errorf("privacy: create identity alternatives actor index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS privacy_aadhaar_preferences (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			actor_id TEXT NOT NULL,
			aadhaar_provided BOOLEAN NOT NULL DEFAULT false,
			aadhaar_masked TEXT NOT NULL DEFAULT '',
			verification_result BOOLEAN NOT NULL DEFAULT false,
			alternate_id_type TEXT NOT NULL DEFAULT '',
			alternate_id_value TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("privacy: create privacy_aadhaar_preferences table: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_aadhaar_preferences_tenant
			ON privacy_aadhaar_preferences(tenant_id)
	`); err != nil {
		return fmt.Errorf("privacy: create aadhaar preferences tenant index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS privacy_evaluation_exports (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			actor_id TEXT NOT NULL DEFAULT '',
			dataset_name TEXT NOT NULL DEFAULT '',
			dataset_scope TEXT NOT NULL DEFAULT '',
			is_deidentified BOOLEAN NOT NULL DEFAULT false,
			deidentification_method TEXT NOT NULL DEFAULT '',
			approved_by TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'created',
			denial_reason TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("privacy: create privacy_evaluation_exports table: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_eval_exports_tenant
			ON privacy_evaluation_exports(tenant_id)
	`); err != nil {
		return fmt.Errorf("privacy: create evaluation exports tenant index: %w", err)
	}

	return nil
}
