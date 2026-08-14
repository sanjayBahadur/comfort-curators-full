package automation

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func EnsureSchema(ctx context.Context, db *pgxpool.Pool) error {
	ddl := []string{
		`CREATE TABLE IF NOT EXISTS agent_runs (
			run_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			run_kind TEXT NOT NULL,
			tenant_id TEXT NOT NULL,
			property_id TEXT NOT NULL,
			actor_id TEXT NOT NULL,
			trigger_type TEXT NOT NULL,
			trigger_id TEXT NOT NULL,
			correlation_id TEXT NOT NULL,
			idempotency_key TEXT,
			state TEXT NOT NULL DEFAULT 'queued'
				CHECK (state IN ('queued', 'leased', 'running', 'waiting_for_tool',
				                 'waiting_for_approval', 'retryable', 'unknown',
				                 'completed', 'failed', 'cancelled')),
			state_version INTEGER NOT NULL DEFAULT 1,
			attempt INTEGER NOT NULL DEFAULT 0,
			max_attempts INTEGER NOT NULL DEFAULT 3,
			lease_owner TEXT,
			lease_expires_at TIMESTAMPTZ,
			heartbeat_at TIMESTAMPTZ,
			provider TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '',
			prompt_template_version TEXT NOT NULL DEFAULT '',
			input_schema_version TEXT NOT NULL DEFAULT '',
			output_schema_version TEXT NOT NULL DEFAULT '',
			input_data JSONB,
			output_data JSONB,
			error_message TEXT,
			usage_minor_units BIGINT NOT NULL DEFAULT 0,
			usage_currency TEXT NOT NULL DEFAULT 'USD',
			usage_input_tokens BIGINT NOT NULL DEFAULT 0,
			usage_output_tokens BIGINT NOT NULL DEFAULT 0,
			usage_total_tokens BIGINT NOT NULL DEFAULT 0,
			usage_known BOOLEAN NOT NULL DEFAULT false,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`ALTER TABLE agent_runs ADD COLUMN IF NOT EXISTS usage_input_tokens BIGINT NOT NULL DEFAULT 0`,
		`ALTER TABLE agent_runs ADD COLUMN IF NOT EXISTS usage_output_tokens BIGINT NOT NULL DEFAULT 0`,
		`ALTER TABLE agent_runs ADD COLUMN IF NOT EXISTS usage_total_tokens BIGINT NOT NULL DEFAULT 0`,
		`ALTER TABLE agent_runs ADD COLUMN IF NOT EXISTS usage_known BOOLEAN NOT NULL DEFAULT false`,
		`ALTER TABLE agent_runs ADD COLUMN IF NOT EXISTS messages_json JSONB`,
		// streaming_text is a live, ephemeral view of the model's in-progress
		// narrative text for the run's current provider call -- overwritten
		// on every delta while a call is in flight, read by the SSE stream
		// handler's poll loop (see stream.go), and never itself the record
		// of truth: output_data/AgentRunCompleted.v1 remain that. It exists
		// purely so a human watching the terminal sees the model actually
		// composing its answer, not a blank wait followed by a full reveal.
		`ALTER TABLE agent_runs ADD COLUMN IF NOT EXISTS streaming_text TEXT NOT NULL DEFAULT ''`,
		`UPDATE agent_runs SET run_kind = 'superhost' WHERE run_kind = 'jarvis'`,
		`CREATE TABLE IF NOT EXISTS agent_run_events (
			event_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			run_id UUID NOT NULL REFERENCES agent_runs(run_id),
			event_name TEXT NOT NULL,
			event_data JSONB,
			occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_agent_runs_state_claim
			ON agent_runs (state, created_at ASC)
			WHERE state = 'queued'`,
		`CREATE INDEX IF NOT EXISTS idx_agent_runs_lease_expires
			ON agent_runs (lease_expires_at)
			WHERE state IN ('leased', 'running', 'waiting_for_tool', 'waiting_for_approval')
				AND lease_expires_at IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_agent_runs_kind_state
			ON agent_runs (run_kind, state)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_runs_idempotency
			ON agent_runs (run_kind, idempotency_key)
			WHERE idempotency_key IS NOT NULL AND state NOT IN ('cancelled', 'failed')`,
		`CREATE INDEX IF NOT EXISTS idx_agent_run_events_run
			ON agent_run_events (run_id, occurred_at)`,
		`CREATE TABLE IF NOT EXISTS superhost_threads (
			thread_id TEXT PRIMARY KEY,
			run_id TEXT NOT NULL,
			tenant_id TEXT NOT NULL,
			property_id TEXT NOT NULL,
			purpose TEXT NOT NULL,
			idempotency_key TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`ALTER TABLE superhost_threads ADD COLUMN IF NOT EXISTS actor_id TEXT NOT NULL DEFAULT ''`,
		// Dropped and recreated scoped by actor_id too, not just
		// (tenant_id, idempotency_key): the frontend's idempotency key is
		// only "routeKey:propertyId" (see SuperhostMount.tsx), with no
		// actor component. Under the old index, two different accounts
		// opening Superhost on the same property with the same routeKey
		// -- e.g. two guests, or a guest and staff both scoped to one
		// property -- would resolve to and share the exact same thread,
		// leaking one account's conversation history into another's. Real
		// actor identity is now always the authenticated subject (see
		// handler.go), so scoping the lookup by it is both correct and
		// safe.
		`DROP INDEX IF EXISTS idx_superhost_threads_idempotency`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_superhost_threads_idempotency_v2
			ON superhost_threads (tenant_id, actor_id, idempotency_key)
			WHERE idempotency_key != ''`,
	}

	for _, stmt := range ddl {
		if _, err := db.Exec(ctx, stmt); err != nil {
			return err
		}
	}

	return nil
}
