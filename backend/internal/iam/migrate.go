package iam

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func EnsureSchema(ctx context.Context, db *pgxpool.Pool) error {
	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS users (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			tenant_id TEXT NOT NULL,
			contact TEXT NOT NULL,
			role TEXT NOT NULL,
			state TEXT NOT NULL DEFAULT 'active',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("iam: create users table: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS authentication_methods (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id UUID NOT NULL,
			method TEXT NOT NULL DEFAULT 'otp',
			secret_hash TEXT NOT NULL,
			expires_at TIMESTAMPTZ NOT NULL,
			consumed BOOLEAN NOT NULL DEFAULT false,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("iam: create authentication_methods table: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			user_id UUID NOT NULL,
			tenant_id TEXT NOT NULL,
			actor_id TEXT NOT NULL,
			roles JSONB NOT NULL DEFAULT '[]',
			expires_at TIMESTAMPTZ NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("iam: create sessions table: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS mfa_methods (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id UUID NOT NULL,
			method TEXT NOT NULL DEFAULT 'totp',
			secret TEXT NOT NULL,
			verified BOOLEAN NOT NULL DEFAULT false,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("iam: create mfa_methods table: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_users_tenant_contact
			ON users(tenant_id, contact)
	`); err != nil {
		return fmt.Errorf("iam: create users tenant_contact index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_auth_methods_user_expires
			ON authentication_methods(user_id, expires_at)
	`); err != nil {
		return fmt.Errorf("iam: create auth_methods index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_sessions_expires
			ON sessions(expires_at)
	`); err != nil {
		return fmt.Errorf("iam: create sessions index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_mfa_methods_user
			ON mfa_methods(user_id)
	`); err != nil {
		return fmt.Errorf("iam: create mfa_methods index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS tenants (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			name TEXT NOT NULL,
			state TEXT NOT NULL DEFAULT 'active',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("iam: create tenants table: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS memberships (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			tenant_id UUID NOT NULL,
			user_id UUID NOT NULL,
			role TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE(tenant_id, user_id)
		)
	`); err != nil {
		return fmt.Errorf("iam: create memberships table: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS support_access_grants (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			tenant_id TEXT NOT NULL,
			granted_by_user_id TEXT NOT NULL,
			granted_to_user_id TEXT NOT NULL,
			reason TEXT NOT NULL,
			scope TEXT NOT NULL DEFAULT 'tenant',
			expires_at TIMESTAMPTZ NOT NULL,
			active BOOLEAN NOT NULL DEFAULT true,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("iam: create support_access_grants table: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_memberships_tenant
			ON memberships(tenant_id)
	`); err != nil {
		return fmt.Errorf("iam: create memberships tenant index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_memberships_user
			ON memberships(user_id)
	`); err != nil {
		return fmt.Errorf("iam: create memberships user index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_support_access_grants_tenant
			ON support_access_grants(tenant_id, granted_to_user_id)
	`); err != nil {
		return fmt.Errorf("iam: create support_access_grants index: %w", err)
	}

	return nil
}
