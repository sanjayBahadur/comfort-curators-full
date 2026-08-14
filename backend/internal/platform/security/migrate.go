package security

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func EnsureSchema(ctx context.Context, db *pgxpool.Pool) error {
	for attempt := 0; attempt < 5; attempt++ {
		err := ensureSchema(ctx, db)
		if err == nil {
			return nil
		}
		if !strings.Contains(err.Error(), "pg_type_typname_nsp_index") {
			return err
		}
		time.Sleep(200 * time.Millisecond)
	}
	return ensureSchema(ctx, db)
}

func ensureSchema(ctx context.Context, db *pgxpool.Pool) error {
	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS encryption_keys (
			id TEXT PRIMARY KEY,
			algorithm TEXT NOT NULL DEFAULT 'aes256-gcm',
			active BOOLEAN NOT NULL DEFAULT true,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			rotated_at TIMESTAMPTZ
		)
	`); err != nil {
		return fmt.Errorf("security: create encryption_keys table: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS session_revocations (
			session_id TEXT PRIMARY KEY,
			reason TEXT NOT NULL DEFAULT '',
			revoked_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			revoked_by TEXT NOT NULL DEFAULT ''
		)
	`); err != nil {
		return fmt.Errorf("security: create session_revocations table: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS privileged_access_log (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			actor_id TEXT NOT NULL,
			tenant_id TEXT,
			action TEXT NOT NULL,
			resource_type TEXT NOT NULL DEFAULT '',
			resource_id TEXT NOT NULL DEFAULT '',
			mfa_used BOOLEAN NOT NULL DEFAULT false,
			success BOOLEAN NOT NULL DEFAULT false,
			details JSONB,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("security: create privileged_access_log table: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_privileged_access_actor
			ON privileged_access_log(actor_id)
	`); err != nil {
		return fmt.Errorf("security: create privileged_access_log actor index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_privileged_access_tenant
			ON privileged_access_log(tenant_id)
	`); err != nil {
		return fmt.Errorf("security: create privileged_access_log tenant index: %w", err)
	}

	return nil
}
