package operations

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func ensureDispatchSchema(ctx context.Context, db *pgxpool.Pool) error {
	ddl := []string{
		`CREATE TABLE IF NOT EXISTS ticket_assignments (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			ticket_id TEXT NOT NULL REFERENCES tickets(id),
			worker_id TEXT NOT NULL,
			assigned_by TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'offered',
			accept_until TIMESTAMPTZ DEFAULT NULL,
			accepted_at TIMESTAMPTZ DEFAULT NULL,
			version INT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,

		`CREATE INDEX IF NOT EXISTS idx_ticket_assignments_ticket
		 ON ticket_assignments(tenant_id, ticket_id)`,

		`CREATE INDEX IF NOT EXISTS idx_ticket_assignments_worker
		 ON ticket_assignments(tenant_id, worker_id)`,

		`CREATE INDEX IF NOT EXISTS idx_ticket_assignments_status
		 ON ticket_assignments(tenant_id, status)`,

		`CREATE TABLE IF NOT EXISTS dispatch_overrides (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			ticket_id TEXT NOT NULL REFERENCES tickets(id),
			worker_id TEXT NOT NULL,
			overridden_by TEXT NOT NULL,
			reason TEXT NOT NULL,
			overridden_constraint TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,

		`CREATE INDEX IF NOT EXISTS idx_dispatch_overrides_ticket
		 ON dispatch_overrides(tenant_id, ticket_id)`,

		`CREATE INDEX IF NOT EXISTS idx_dispatch_overrides_worker
		 ON dispatch_overrides(tenant_id, worker_id)`,

		`CREATE TABLE IF NOT EXISTS route_plans (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			worker_id TEXT NOT NULL,
			planned_date DATE NOT NULL,
			total_travel_minutes INT NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,

		`CREATE INDEX IF NOT EXISTS idx_route_plans_worker
		 ON route_plans(tenant_id, worker_id, planned_date)`,

		`CREATE TABLE IF NOT EXISTS route_stops (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			route_plan_id TEXT NOT NULL REFERENCES route_plans(id),
			ticket_id TEXT NOT NULL REFERENCES tickets(id),
			property_id TEXT NOT NULL,
			sequence INT NOT NULL DEFAULT 0,
			estimated_arrival TIMESTAMPTZ DEFAULT NULL,
			estimated_departure TIMESTAMPTZ DEFAULT NULL,
			travel_from_previous_minutes INT NOT NULL DEFAULT 0
		)`,

		`CREATE INDEX IF NOT EXISTS idx_route_stops_plan
		 ON route_stops(route_plan_id, sequence)`,
	}

	for _, stmt := range ddl {
		if _, err := db.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("dispatch schema: %w", err)
		}
	}

	return nil
}
