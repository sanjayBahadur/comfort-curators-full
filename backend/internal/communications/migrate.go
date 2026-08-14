package communications

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func EnsureSchema(ctx context.Context, db *pgxpool.Pool) error {
	ddl := []string{
		`CREATE TABLE IF NOT EXISTS message_templates (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			template_key TEXT NOT NULL,
			audience TEXT NOT NULL,
			consent_class TEXT NOT NULL,
			channel TEXT NOT NULL DEFAULT 'push',
			severity TEXT NOT NULL DEFAULT 'normal',
			status TEXT NOT NULL DEFAULT 'active',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (tenant_id, template_key)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_message_templates_tenant
		 ON message_templates(tenant_id, audience, status)`,

		`CREATE TABLE IF NOT EXISTS message_template_versions (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			template_id TEXT NOT NULL REFERENCES message_templates(id),
			version INT NOT NULL,
			language TEXT NOT NULL,
			subject TEXT NOT NULL DEFAULT '',
			body TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (template_id, version, language)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_message_template_versions_template
		 ON message_template_versions(template_id, version)`,

		`CREATE TABLE IF NOT EXISTS communication_preferences (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			recipient_id TEXT NOT NULL,
			audience TEXT NOT NULL,
			consent_transactional BOOLEAN NOT NULL DEFAULT TRUE,
			consent_urgent BOOLEAN NOT NULL DEFAULT TRUE,
			consent_marketing BOOLEAN NOT NULL DEFAULT FALSE,
			consent_sponsored BOOLEAN NOT NULL DEFAULT FALSE,
			channel TEXT NOT NULL DEFAULT 'push',
			severity TEXT NOT NULL DEFAULT 'normal',
			quiet_hours_start_minute INT NOT NULL DEFAULT 0,
			quiet_hours_end_minute INT NOT NULL DEFAULT 0,
			escalation_contacts JSONB DEFAULT '[]',
			version INT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (tenant_id, recipient_id, audience)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_communication_preferences_tenant
		 ON communication_preferences(tenant_id, recipient_id, audience)`,

		`CREATE TABLE IF NOT EXISTS communication_drafts (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			audience TEXT NOT NULL,
			recipient_id TEXT NOT NULL,
			source TEXT NOT NULL,
			template_key TEXT DEFAULT NULL,
			consent_class TEXT NOT NULL,
			channel TEXT NOT NULL DEFAULT 'push',
			severity TEXT NOT NULL DEFAULT 'normal',
			subject TEXT NOT NULL DEFAULT '',
			body TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'draft',
			requires_review BOOLEAN NOT NULL DEFAULT FALSE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_communication_drafts_tenant
		 ON communication_drafts(tenant_id, recipient_id, status)`,

		`CREATE TABLE IF NOT EXISTS communication_reviews (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			draft_id TEXT NOT NULL REFERENCES communication_drafts(id),
			reviewer_id TEXT NOT NULL,
			decision TEXT NOT NULL,
			reason TEXT NOT NULL DEFAULT '',
			reviewed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_communication_reviews_draft
		 ON communication_reviews(tenant_id, draft_id, reviewed_at)`,

		`CREATE TABLE IF NOT EXISTS deliveries (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			draft_id TEXT DEFAULT NULL,
			recipient_id TEXT NOT NULL,
			audience TEXT NOT NULL,
			consent_class TEXT NOT NULL,
			channel TEXT NOT NULL DEFAULT 'push',
			status TEXT NOT NULL DEFAULT 'queued',
			error TEXT DEFAULT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			delivered_at TIMESTAMPTZ DEFAULT NULL,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_deliveries_tenant
		 ON deliveries(tenant_id, recipient_id, created_at)`,

		`CREATE TABLE IF NOT EXISTS conversation_links (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			property_id TEXT NOT NULL,
			audience TEXT NOT NULL,
			recipient_id TEXT NOT NULL,
			purpose TEXT NOT NULL DEFAULT 'stay',
			token_hash TEXT NOT NULL UNIQUE,
			token_tail TEXT NOT NULL,
			expires_at TIMESTAMPTZ NOT NULL,
			used_at TIMESTAMPTZ DEFAULT NULL,
			revoked_at TIMESTAMPTZ DEFAULT NULL,
			status TEXT NOT NULL DEFAULT 'active',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_conversation_links_tenant
		 ON conversation_links(tenant_id, property_id, audience)`,
	}

	for _, stmt := range ddl {
		if _, err := db.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("communications schema: %w", err)
		}
	}

	return nil
}
