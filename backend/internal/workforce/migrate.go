package workforce

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func EnsureSchema(ctx context.Context, db *pgxpool.Pool) error {
	ddl := []string{
		`CREATE TABLE IF NOT EXISTS workers (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			legal_name TEXT NOT NULL,
			verified_identity BOOLEAN NOT NULL DEFAULT FALSE,
			date_of_birth TIMESTAMPTZ NOT NULL,
			age_eligible BOOLEAN NOT NULL DEFAULT FALSE,
			contact_method TEXT NOT NULL DEFAULT '',
			classification TEXT NOT NULL,
			specialist BOOLEAN NOT NULL DEFAULT FALSE,
			service_zone TEXT NOT NULL DEFAULT '',
			skills JSONB DEFAULT '[]',
			status TEXT NOT NULL DEFAULT 'active',
			version INT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_workers_tenant
		 ON workers(tenant_id, status)`,

		`CREATE TABLE IF NOT EXISTS worker_certifications (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			worker_id TEXT NOT NULL REFERENCES workers(id),
			work_type TEXT NOT NULL,
			issuer TEXT NOT NULL DEFAULT '',
			issued_at TIMESTAMPTZ NOT NULL,
			expires_at TIMESTAMPTZ NOT NULL,
			status TEXT NOT NULL DEFAULT 'valid',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_worker_certifications_worker
		 ON worker_certifications(tenant_id, worker_id, work_type)`,

		`CREATE TABLE IF NOT EXISTS worker_ratings (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			worker_id TEXT NOT NULL REFERENCES workers(id),
			score INT NOT NULL,
			source TEXT NOT NULL,
			comment TEXT DEFAULT NULL,
			recorded_by TEXT NOT NULL DEFAULT '',
			recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_worker_ratings_worker
		 ON worker_ratings(tenant_id, worker_id, recorded_at)`,

		`CREATE TABLE IF NOT EXISTS adverse_action_reviews (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			worker_id TEXT NOT NULL REFERENCES workers(id),
			action TEXT NOT NULL,
			evidence_refs JSONB DEFAULT '[]',
			reviewer_id TEXT NOT NULL,
			reason TEXT NOT NULL DEFAULT '',
			worker_version INT NOT NULL DEFAULT 1,
			decided_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_adverse_action_reviews_worker
		 ON adverse_action_reviews(tenant_id, worker_id, decided_at)`,

		`CREATE TABLE IF NOT EXISTS workforce_assignments (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			worker_id TEXT NOT NULL REFERENCES workers(id),
			work_type TEXT NOT NULL DEFAULT 'general',
			assigned_by TEXT NOT NULL DEFAULT '',
			assigned_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_workforce_assignments_worker
		 ON workforce_assignments(tenant_id, worker_id, assigned_at)`,

		`CREATE TABLE IF NOT EXISTS availability_windows (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			worker_id TEXT NOT NULL REFERENCES workers(id),
			day_of_week INT NOT NULL,
			start_minute INT NOT NULL,
			end_minute INT NOT NULL,
			effective_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_availability_windows_worker
		 ON availability_windows(tenant_id, worker_id, day_of_week)`,

		`CREATE TABLE IF NOT EXISTS time_entries (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			worker_id TEXT NOT NULL REFERENCES workers(id),
			ticket_id TEXT DEFAULT NULL,
			work_minutes INT NOT NULL DEFAULT 0,
			travel_minutes INT NOT NULL DEFAULT 0,
			overtime_flag BOOLEAN NOT NULL DEFAULT FALSE,
			recorded_by TEXT NOT NULL DEFAULT '',
			recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_time_entries_worker
		 ON time_entries(tenant_id, worker_id, recorded_at)`,

		`CREATE TABLE IF NOT EXISTS expenses (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			worker_id TEXT NOT NULL REFERENCES workers(id),
			ticket_id TEXT DEFAULT NULL,
			minor_units BIGINT NOT NULL DEFAULT 0,
			currency TEXT NOT NULL DEFAULT 'INR',
			category TEXT NOT NULL DEFAULT '',
			receipt_ref TEXT DEFAULT NULL,
			recorded_by TEXT NOT NULL DEFAULT '',
			recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_expenses_worker
		 ON expenses(tenant_id, worker_id, recorded_at)`,

		`CREATE TABLE IF NOT EXISTS grievances (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			worker_id TEXT NOT NULL REFERENCES workers(id),
			kind TEXT NOT NULL,
			reason TEXT NOT NULL DEFAULT '',
			evidence_refs JSONB DEFAULT '[]',
			status TEXT NOT NULL DEFAULT 'pending',
			submitted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			resolved_at TIMESTAMPTZ DEFAULT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_grievances_worker
		 ON grievances(tenant_id, worker_id, status)`,

		`CREATE TABLE IF NOT EXISTS sos_events (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			worker_id TEXT NOT NULL REFERENCES workers(id),
			ticket_id TEXT DEFAULT NULL,
			location TEXT DEFAULT '',
			triggered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			acknowledged_by TEXT DEFAULT NULL,
			acknowledged_at TIMESTAMPTZ DEFAULT NULL,
			resolution TEXT DEFAULT NULL,
			resolved_at TIMESTAMPTZ DEFAULT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sos_events_worker
		 ON sos_events(tenant_id, worker_id, triggered_at)`,

		`CREATE TABLE IF NOT EXISTS employment_terms (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			worker_id TEXT NOT NULL REFERENCES workers(id),
			role TEXT NOT NULL,
			compensation_band TEXT DEFAULT '',
			effective_date TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			end_date TIMESTAMPTZ DEFAULT NULL,
			agreement_ref TEXT DEFAULT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_employment_terms_worker
		 ON employment_terms(tenant_id, worker_id, effective_date)`,
	}

	for _, stmt := range ddl {
		if _, err := db.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("workforce schema: %w", err)
		}
	}

	return nil
}
