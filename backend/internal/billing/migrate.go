package billing

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func EnsureSchema(ctx context.Context, db *pgxpool.Pool) error {
	ddl := []string{
		`CREATE TABLE IF NOT EXISTS charges (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			property_id TEXT NOT NULL DEFAULT '',
			charge_type TEXT NOT NULL DEFAULT '',
			amount_minor_units BIGINT NOT NULL DEFAULT 0,
			currency TEXT NOT NULL DEFAULT 'INR',
			reason TEXT NOT NULL DEFAULT '',
			data JSONB NOT NULL DEFAULT '{}',
			contract_rule_id TEXT NOT NULL DEFAULT '',
			evidence_id TEXT NOT NULL DEFAULT '',
			ticket_id TEXT NOT NULL DEFAULT '',
			order_id TEXT NOT NULL DEFAULT '',
			approval_id TEXT NOT NULL DEFAULT '',
			idempotency_key TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'pending',
			version BIGINT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_charges_tenant ON charges(tenant_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_charges_property ON charges(tenant_id, property_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_charges_idempotency ON charges(tenant_id, idempotency_key)`,

		`CREATE TABLE IF NOT EXISTS invoices (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			property_id TEXT NOT NULL DEFAULT '',
			period_start TIMESTAMPTZ DEFAULT NULL,
			period_end TIMESTAMPTZ DEFAULT NULL,
			total_minor_units BIGINT NOT NULL DEFAULT 0,
			currency TEXT NOT NULL DEFAULT 'INR',
			status TEXT NOT NULL DEFAULT 'draft',
			idempotency_key TEXT NOT NULL DEFAULT '',
			version BIGINT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_invoices_tenant ON invoices(tenant_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_invoices_property ON invoices(tenant_id, property_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_invoices_idempotency ON invoices(tenant_id, idempotency_key)`,

		`CREATE TABLE IF NOT EXISTS invoice_lines (
			id TEXT PRIMARY KEY,
			invoice_id TEXT NOT NULL DEFAULT '',
			tenant_id TEXT NOT NULL,
			charge_type TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			amount_minor_units BIGINT NOT NULL DEFAULT 0,
			currency TEXT NOT NULL DEFAULT 'INR',
			contract_rule_id TEXT NOT NULL DEFAULT '',
			ticket_id TEXT NOT NULL DEFAULT '',
			order_id TEXT NOT NULL DEFAULT '',
			adjustment_id TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_invoice_lines_invoice ON invoice_lines(tenant_id, invoice_id)`,

		`CREATE TABLE IF NOT EXISTS credits (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			property_id TEXT NOT NULL DEFAULT '',
			credit_type TEXT NOT NULL DEFAULT '',
			amount_minor_units BIGINT NOT NULL DEFAULT 0,
			currency TEXT NOT NULL DEFAULT 'INR',
			reason TEXT NOT NULL DEFAULT '',
			original_entry_id TEXT NOT NULL DEFAULT '',
			original_entry_type TEXT NOT NULL DEFAULT '',
			data JSONB NOT NULL DEFAULT '{}',
			idempotency_key TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'issued',
			version BIGINT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_credits_tenant ON credits(tenant_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_credits_property ON credits(tenant_id, property_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_credits_idempotency ON credits(tenant_id, idempotency_key)`,

		`CREATE TABLE IF NOT EXISTS operational_subledger_entries (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			property_id TEXT NOT NULL DEFAULT '',
			entry_type TEXT NOT NULL DEFAULT '',
			amount_minor_units BIGINT NOT NULL DEFAULT 0,
			currency TEXT NOT NULL DEFAULT 'INR',
			reference_type TEXT NOT NULL DEFAULT '',
			reference_id TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_subledger_tenant ON operational_subledger_entries(tenant_id, property_id)`,
		`CREATE INDEX IF NOT EXISTS idx_subledger_reference ON operational_subledger_entries(tenant_id, reference_type, reference_id)`,

		`CREATE TABLE IF NOT EXISTS accounting_exports (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			period_start TIMESTAMPTZ DEFAULT NULL,
			period_end TIMESTAMPTZ DEFAULT NULL,
			format TEXT NOT NULL DEFAULT 'journal_csv',
			status TEXT NOT NULL DEFAULT 'requested',
			requested_by TEXT NOT NULL DEFAULT '',
			result_ref TEXT NOT NULL DEFAULT '',
			version BIGINT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_accounting_exports_tenant ON accounting_exports(tenant_id, status)`,

		`CREATE TABLE IF NOT EXISTS financial_approvals (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			request_id TEXT NOT NULL DEFAULT '',
			approver_id TEXT NOT NULL DEFAULT '',
			decision TEXT NOT NULL DEFAULT '',
			reason TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_financial_approvals_tenant ON financial_approvals(tenant_id, request_id)`,

		`CREATE TABLE IF NOT EXISTS maker_checker_requests (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			request_type TEXT NOT NULL DEFAULT '',
			property_id TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'draft',
			created_by TEXT NOT NULL DEFAULT '',
			submitted_by TEXT NOT NULL DEFAULT '',
			approved_by TEXT NOT NULL DEFAULT '',
			rejected_by TEXT NOT NULL DEFAULT '',
			payload JSONB NOT NULL DEFAULT '{}',
			idempotency_key TEXT NOT NULL DEFAULT '',
			requires_verification BOOLEAN NOT NULL DEFAULT FALSE,
			version BIGINT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_maker_checker_requests_tenant ON maker_checker_requests(tenant_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_maker_checker_requests_type ON maker_checker_requests(tenant_id, request_type)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_maker_checker_requests_idempotency ON maker_checker_requests(tenant_id, idempotency_key) WHERE idempotency_key != ''`,

		`CREATE TABLE IF NOT EXISTS bank_verifications (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			request_id TEXT NOT NULL DEFAULT '',
			verification_token TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'pending',
			verified_by TEXT NOT NULL DEFAULT '',
			verified_at TIMESTAMPTZ DEFAULT NULL,
			expires_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_bank_verifications_tenant ON bank_verifications(tenant_id, request_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_bank_verifications_request ON bank_verifications(tenant_id, request_id)`,

		`CREATE TABLE IF NOT EXISTS reconciliation_exceptions (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			property_id TEXT NOT NULL DEFAULT '',
			entry_id TEXT NOT NULL DEFAULT '',
			entry_type TEXT NOT NULL DEFAULT '',
			exception_type TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'open',
			recorded_by TEXT NOT NULL DEFAULT '',
			resolved_by TEXT NOT NULL DEFAULT '',
			resolved_at TIMESTAMPTZ DEFAULT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_reconciliation_exceptions_tenant ON reconciliation_exceptions(tenant_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_reconciliation_exceptions_property ON reconciliation_exceptions(tenant_id, property_id)`,
	}

	for _, stmt := range ddl {
		if _, err := db.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("billing schema: %w", err)
		}
	}

	return nil
}
