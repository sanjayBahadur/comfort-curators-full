package documents

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func EnsureSchema(ctx context.Context, db *pgxpool.Pool) error {
	ddl := []string{
		`CREATE TABLE IF NOT EXISTS documents (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			property_id TEXT NOT NULL,
			title TEXT NOT NULL,
			document_type TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'draft',
			expires_at TIMESTAMPTZ DEFAULT NULL,
			current_version INT NOT NULL DEFAULT 1,
			version INT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_documents_tenant
		 ON documents(tenant_id, property_id, document_type)`,
		`CREATE INDEX IF NOT EXISTS idx_documents_status
		 ON documents(tenant_id, status)`,

		`CREATE TABLE IF NOT EXISTS document_versions (
			id TEXT PRIMARY KEY,
			document_id TEXT NOT NULL,
			tenant_id TEXT NOT NULL,
			version_number INT NOT NULL,
			content_hash TEXT NOT NULL,
			object_key TEXT NOT NULL,
			filename TEXT NOT NULL DEFAULT '',
			content_type TEXT NOT NULL DEFAULT '',
			size_bytes BIGINT NOT NULL DEFAULT 0,
			uploaded_by TEXT NOT NULL DEFAULT '',
			metadata TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE(document_id, content_hash)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_document_versions_doc
		 ON document_versions(document_id, tenant_id, version_number)`,

		`CREATE TABLE IF NOT EXISTS document_extractions (
			id TEXT PRIMARY KEY,
			document_version_id TEXT NOT NULL,
			tenant_id TEXT NOT NULL,
			field_name TEXT NOT NULL,
			field_value TEXT NOT NULL DEFAULT '',
			field_category TEXT NOT NULL DEFAULT 'general',
			source_location TEXT NOT NULL DEFAULT '',
			confidence TEXT NOT NULL DEFAULT 'high',
			confidence_score DOUBLE PRECISION NOT NULL DEFAULT 0,
			extracted_by TEXT NOT NULL DEFAULT '',
			extracted_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_document_extractions_version
		 ON document_extractions(document_version_id, tenant_id)`,
		`CREATE INDEX IF NOT EXISTS idx_document_extractions_confidence
		 ON document_extractions(tenant_id, field_name, confidence)`,

		`CREATE TABLE IF NOT EXISTS document_reviews (
			id TEXT PRIMARY KEY,
			document_id TEXT NOT NULL,
			document_version_id TEXT NOT NULL,
			tenant_id TEXT NOT NULL,
			reviewer_id TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			decision TEXT NOT NULL DEFAULT '',
			comments TEXT NOT NULL DEFAULT '',
			reviewed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_document_reviews_doc
		 ON document_reviews(document_id, tenant_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_document_reviews_reviewer
		 ON document_reviews(reviewer_id, status)`,

		`CREATE TABLE IF NOT EXISTS submission_packets (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			property_id TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'draft',
			document_ids JSONB NOT NULL DEFAULT '[]',
			created_by TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			submitted_at TIMESTAMPTZ DEFAULT NULL,
			version INT NOT NULL DEFAULT 1,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_submission_packets_tenant
		 ON submission_packets(tenant_id, property_id, status)`,

		`CREATE TABLE IF NOT EXISTS submission_receipts (
			id TEXT PRIMARY KEY,
			packet_id TEXT NOT NULL,
			tenant_id TEXT NOT NULL,
			confirmed_by TEXT NOT NULL,
			receipt_hash TEXT NOT NULL,
			document_version_refs JSONB NOT NULL DEFAULT '[]',
			confirmed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_submission_receipts_packet
		 ON submission_receipts(packet_id, tenant_id)`,
	}

	for _, stmt := range ddl {
		if _, err := db.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("documents schema: %w", err)
		}
	}

	return nil
}
