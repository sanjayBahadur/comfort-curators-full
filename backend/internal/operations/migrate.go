package operations

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func EnsureDispatchSchema(ctx context.Context, db *pgxpool.Pool) error {
	return ensureDispatchSchema(ctx, db)
}

func EnsureSchema(ctx context.Context, db *pgxpool.Pool) error {
	ddl := []string{
		`CREATE TABLE IF NOT EXISTS tickets (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			property_id TEXT NOT NULL,
			type TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'draft',
			reason TEXT NOT NULL DEFAULT '',
			requested_window JSONB DEFAULT '{}',
			checklist_version_id TEXT DEFAULT NULL,
			created_by TEXT NOT NULL DEFAULT '',
			assigned_to TEXT DEFAULT NULL,
			verified_by TEXT DEFAULT NULL,
			verifier_note TEXT DEFAULT NULL,
			blocker JSONB DEFAULT NULL,
			follow_up_ticket_id TEXT DEFAULT NULL,
			reopen_reason TEXT DEFAULT NULL,
			notification_intent TEXT DEFAULT NULL,
			version INT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,

		`CREATE INDEX IF NOT EXISTS idx_tickets_tenant_property
		 ON tickets(tenant_id, property_id)`,

		`CREATE INDEX IF NOT EXISTS idx_tickets_status
		 ON tickets(tenant_id, status)`,

		`CREATE INDEX IF NOT EXISTS idx_tickets_type
		 ON tickets(tenant_id, type)`,

		`CREATE TABLE IF NOT EXISTS ticket_state_events (
			id TEXT PRIMARY KEY,
			ticket_id TEXT NOT NULL REFERENCES tickets(id),
			tenant_id TEXT NOT NULL,
			from_state TEXT NOT NULL,
			to_state TEXT NOT NULL,
			actor_id TEXT NOT NULL,
			reason TEXT NOT NULL DEFAULT '',
			evidence JSONB DEFAULT '[]',
			version INT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,

		`CREATE INDEX IF NOT EXISTS idx_ticket_state_events_ticket
		 ON ticket_state_events(tenant_id, ticket_id, created_at)`,

		`CREATE TABLE IF NOT EXISTS checklist_templates (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			ticket_type TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,

		`CREATE TABLE IF NOT EXISTS checklist_template_versions (
			id TEXT PRIMARY KEY,
			template_id TEXT NOT NULL REFERENCES checklist_templates(id),
			version INT NOT NULL,
			items JSONB NOT NULL DEFAULT '[]',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,

		`CREATE INDEX IF NOT EXISTS idx_checklist_template_versions
		 ON checklist_template_versions(template_id, version)`,

		`CREATE TABLE IF NOT EXISTS ticket_checklist_items (
			id TEXT PRIMARY KEY,
			ticket_id TEXT NOT NULL REFERENCES tickets(id),
			tenant_id TEXT NOT NULL,
			template_item_index INT NOT NULL DEFAULT 0,
			label TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'pending',
			completed_by TEXT DEFAULT NULL,
			completed_at TIMESTAMPTZ DEFAULT NULL,
			evidence_ids JSONB DEFAULT '[]',
			notes TEXT DEFAULT NULL,
			version INT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,

		`CREATE INDEX IF NOT EXISTS idx_ticket_checklist_items_ticket
		 ON ticket_checklist_items(tenant_id, ticket_id, template_item_index)`,

		`CREATE INDEX IF NOT EXISTS idx_ticket_checklist_items_status
		 ON ticket_checklist_items(tenant_id, status)`,

		`ALTER TABLE tickets ADD COLUMN IF NOT EXISTS severity TEXT DEFAULT NULL`,

		`ALTER TABLE ticket_checklist_items ADD COLUMN IF NOT EXISTS evidence_required BOOLEAN NOT NULL DEFAULT FALSE`,

		`CREATE TABLE IF NOT EXISTS ticket_evidence (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			ticket_id TEXT NOT NULL REFERENCES tickets(id),
			checklist_item_id TEXT DEFAULT NULL,
			object_id TEXT DEFAULT NULL,
			content_hash TEXT NOT NULL,
			file_name TEXT DEFAULT NULL,
			content_type TEXT DEFAULT NULL,
			size_bytes BIGINT NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'accepted',
			captured_by TEXT NOT NULL DEFAULT '',
			captured_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			version INT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,

		`CREATE INDEX IF NOT EXISTS idx_ticket_evidence_ticket
		 ON ticket_evidence(tenant_id, ticket_id, captured_at)`,

		`CREATE INDEX IF NOT EXISTS idx_ticket_evidence_hash
		 ON ticket_evidence(tenant_id, ticket_id, content_hash)`,

		`CREATE TABLE IF NOT EXISTS incident_alerts (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			property_id TEXT NOT NULL,
			ticket_id TEXT NOT NULL REFERENCES tickets(id),
			severity TEXT NOT NULL DEFAULT '',
			target TEXT NOT NULL,
			policy TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'queued',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,

		`CREATE INDEX IF NOT EXISTS idx_incident_alerts_queue
		 ON incident_alerts(tenant_id, property_id, status, created_at)`,

		`CREATE INDEX IF NOT EXISTS idx_incident_alerts_ticket
		 ON incident_alerts(tenant_id, ticket_id)`,

		`CREATE TABLE IF NOT EXISTS service_recoveries (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			property_id TEXT NOT NULL,
			incident_ticket_id TEXT NOT NULL REFERENCES tickets(id),
			follow_up_ticket_id TEXT DEFAULT NULL,
			severity TEXT NOT NULL DEFAULT 'low',
			original_reason TEXT NOT NULL DEFAULT '',
			original_evidence_hashes JSONB DEFAULT '[]',
			responsibility TEXT NOT NULL DEFAULT '',
			rework_cost_minor BIGINT NOT NULL DEFAULT 0,
			currency TEXT DEFAULT NULL,
			status TEXT NOT NULL DEFAULT 'open',
			created_by TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,

		`CREATE INDEX IF NOT EXISTS idx_service_recoveries_incident
		 ON service_recoveries(tenant_id, incident_ticket_id)`,

		`CREATE TABLE IF NOT EXISTS checklist_sync_records (
			id TEXT PRIMARY KEY,
			sync_key TEXT NOT NULL,
			tenant_id TEXT NOT NULL,
			ticket_id TEXT NOT NULL REFERENCES tickets(id),
			payload_hash TEXT NOT NULL,
			result TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,

		`CREATE UNIQUE INDEX IF NOT EXISTS idx_checklist_sync_key
		 ON checklist_sync_records(sync_key)`,

		`CREATE INDEX IF NOT EXISTS idx_checklist_sync_records_ticket
		 ON checklist_sync_records(tenant_id, ticket_id)`,

		`CREATE TABLE IF NOT EXISTS checklist_sync_conflicts (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			ticket_id TEXT NOT NULL REFERENCES tickets(id),
			checklist_item_id TEXT DEFAULT NULL,
			template_item_index INT NOT NULL DEFAULT 0,
			server_label TEXT NOT NULL DEFAULT '',
			server_status TEXT NOT NULL DEFAULT '',
			server_version INT NOT NULL DEFAULT 0,
			client_label TEXT NOT NULL DEFAULT '',
			client_status TEXT NOT NULL DEFAULT '',
			client_version INT NOT NULL DEFAULT 0,
			resolved BOOLEAN NOT NULL DEFAULT FALSE,
			resolution TEXT DEFAULT NULL,
			resolved_by TEXT DEFAULT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			resolved_at TIMESTAMPTZ DEFAULT NULL
		)`,

		`CREATE INDEX IF NOT EXISTS idx_checklist_sync_conflicts_ticket
		 ON checklist_sync_conflicts(tenant_id, ticket_id)`,

		`CREATE INDEX IF NOT EXISTS idx_checklist_sync_conflicts_open
		 ON checklist_sync_conflicts(tenant_id, ticket_id, resolved)`,

		`CREATE TABLE IF NOT EXISTS queued_offline_evidence (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			ticket_id TEXT NOT NULL REFERENCES tickets(id),
			checklist_item_id TEXT DEFAULT NULL,
			content_hash TEXT NOT NULL,
			file_name TEXT DEFAULT NULL,
			content_type TEXT DEFAULT NULL,
			size_bytes BIGINT NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'queued',
			captured_by TEXT NOT NULL DEFAULT '',
			captured_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,

		`CREATE INDEX IF NOT EXISTS idx_queued_offline_evidence_ticket
		 ON queued_offline_evidence(tenant_id, ticket_id, status)`,

		`CREATE INDEX IF NOT EXISTS idx_queued_offline_evidence_hash
		 ON queued_offline_evidence(tenant_id, content_hash)`,
	}

	for _, stmt := range ddl {
		if _, err := db.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("operations schema: %w", err)
		}
	}

	if err := ensureDispatchSchema(ctx, db); err != nil {
		return fmt.Errorf("dispatch schema: %w", err)
	}

	return nil
}
