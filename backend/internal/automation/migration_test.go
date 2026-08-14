package automation_test

import (
	"context"
	"testing"

	"comfort-curators-backend/internal/automation"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMigrationBackfillJarvisToSuperhost(t *testing.T) {
	pool := newPool(t)

	for _, table := range []string{"agent_run_events", "agent_runs"} {
		if _, err := pool.Exec(context.Background(), "TRUNCATE TABLE "+table+" CASCADE"); err != nil {
			t.Fatalf("truncate %s: %v", table, err)
		}
	}

	seedPreMigrationRow(t, pool)
	verifyRowIsSuperhostAfterEnsureSchema(t, pool)
	verifyBackfillIsIdempotent(t, pool)
	verifySuperhostRowsAreUntouched(t, pool)
}

func seedPreMigrationRow(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	_, err := pool.Exec(ctx,
		`INSERT INTO agent_runs (
			run_kind, tenant_id, property_id, actor_id, trigger_type,
			trigger_id, correlation_id, idempotency_key, state
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		"jarvis", "tenant-mig", "prop-mig", "actor-mig",
		"manual", "migration-test", "corr-mig",
		"migration-key-j-to-s", "queued",
	)
	if err != nil {
		t.Fatalf("seed pre-migration row: %v", err)
	}

	var runKind string
	err = pool.QueryRow(ctx,
		`SELECT run_kind FROM agent_runs WHERE idempotency_key = $1`,
		"migration-key-j-to-s",
	).Scan(&runKind)
	if err != nil {
		t.Fatalf("read back seeded row: %v", err)
	}
	if runKind != "jarvis" {
		t.Fatalf("seeded row must have run_kind='jarvis' before migration, got %q", runKind)
	}
	t.Logf("seeded row with run_kind=jarvis")
}

func verifyRowIsSuperhostAfterEnsureSchema(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	if err := automation.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("EnsureSchema after seeding: %v", err)
	}

	var runKind string
	err := pool.QueryRow(ctx,
		`SELECT run_kind FROM agent_runs WHERE idempotency_key = $1`,
		"migration-key-j-to-s",
	).Scan(&runKind)
	if err != nil {
		t.Fatalf("read back after migration: %v", err)
	}
	if runKind != "superhost" {
		t.Fatalf("backfill failed: run_kind is %q, want 'superhost'", runKind)
	}
	t.Logf("backfill succeeded: run_kind is now %s", runKind)
}

func verifyBackfillIsIdempotent(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	if err := automation.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("second EnsureSchema: %v", err)
	}

	var runKind string
	err := pool.QueryRow(ctx,
		`SELECT run_kind FROM agent_runs WHERE idempotency_key = $1`,
		"migration-key-j-to-s",
	).Scan(&runKind)
	if err != nil {
		t.Fatalf("read back after second schema run: %v", err)
	}
	if runKind != "superhost" {
		t.Fatalf("idempotent re-run mutated run_kind to %q, want 'superhost'", runKind)
	}
	t.Logf("backfill is idempotent: second run preserved superhost")
}

func verifySuperhostRowsAreUntouched(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	store := automation.NewAgentRunStore(pool)
	req := automation.SubmitRequest{
		RunKind:        "superhost",
		TenantID:       "tenant-mig",
		PropertyID:     "prop-mig",
		ActorID:        "actor-mig",
		TriggerType:    "manual",
		TriggerID:      "migration-test-2",
		CorrelationID:  "corr-mig-2",
		IdempotencyKey: "migration-key-already-sh",
		Provider:       "stub",
		Model:          "test-model",
	}
	run, _, err := store.Submit(ctx, req)
	if err != nil {
		t.Fatalf("submit superhost run: %v", err)
	}

	if run.RunKind != "superhost" {
		t.Fatalf("submitted run must have run_kind='superhost', got %q", run.RunKind)
	}

	if err := automation.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("EnsureSchema with existing superhost row: %v", err)
	}

	var runKind string
	err = pool.QueryRow(ctx,
		`SELECT run_kind FROM agent_runs WHERE run_id = $1`,
		run.RunID,
	).Scan(&runKind)
	if err != nil {
		t.Fatalf("read back superhost row: %v", err)
	}
	if runKind != "superhost" {
		t.Fatalf("existing superhost row must not be touched: got %q, want 'superhost'", runKind)
	}
	t.Logf("existing superhost row preserved across backfill")
}
