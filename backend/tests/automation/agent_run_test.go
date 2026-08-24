package automation_test

import (
	"context"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"comfort-curators-backend/internal/automation"
	"comfort-curators-backend/internal/platform/testdb"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestIntegrationAgentRunLifecycle(t *testing.T) {
	if !isPostgresReady() {
		t.Skip("PostgreSQL not available")
	}

	pool, err := createTestPool()
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	defer pool.Close()

	if err := automation.EnsureSchema(context.Background(), pool); err != nil {
		t.Logf("ensure schema (may already exist): %v", err)
	}
	defer func() {
		for _, table := range []string{"agent_run_events", "agent_runs"} {
			if _, err := pool.Exec(context.Background(), "TRUNCATE TABLE "+table+" CASCADE"); err != nil {
				t.Logf("cleanup %s: %v", table, err)
			}
		}
	}()

	store := automation.NewAgentRunStore(pool)
	ctx := context.Background()

	t.Run("submit returns run ID immediately", func(t *testing.T) {
		req := automation.SubmitRequest{
			RunKind:        "jarvis",
			TenantID:       "tenant-int",
			PropertyID:     "prop-int",
			ActorID:        "actor-int",
			TriggerType:    "event",
			TriggerID:      "evt-1",
			CorrelationID:  "corr-int-1",
			IdempotencyKey: "int-key-1",
			Provider:       "stub",
			Model:          "test-model-v1",
		}

		run, dup, err := store.Submit(ctx, req)
		if err != nil {
			t.Fatalf("submit: %v", err)
		}
		if dup {
			t.Fatal("first submit must not be duplicate")
		}
		if run.RunID == "" {
			t.Fatal("run ID must not be empty")
		}
		if run.State != automation.StateQueued {
			t.Fatalf("expected queued, got %s", run.State)
		}
		t.Logf("submit: run_id=%s state=%s", run.RunID, run.State)
	})

	t.Run("duplicate request returns same run ID", func(t *testing.T) {
		req := automation.SubmitRequest{
			RunKind:        "jarvis",
			TenantID:       "tenant-int",
			PropertyID:     "prop-int",
			ActorID:        "actor-int",
			TriggerType:    "event",
			TriggerID:      "evt-1",
			CorrelationID:  "corr-int-1",
			IdempotencyKey: "int-key-1",
			Provider:       "stub",
			Model:          "test-model-v1",
		}

		run1, dup1, err := store.Submit(ctx, req)
		if err != nil {
			t.Fatalf("first submit: %v", err)
		}
		if dup1 {
			t.Fatal("first submit must not be duplicate")
		}

		run2, dup2, err := store.Submit(ctx, req)
		if err != nil {
			t.Fatalf("second submit: %v", err)
		}
		if !dup2 {
			t.Fatal("second submit must be duplicate")
		}
		if run2.RunID != run1.RunID {
			t.Fatalf("duplicate must return same ID: %s vs %s", run2.RunID, run1.RunID)
		}
		t.Logf("duplicate: both returned run_id=%s", run1.RunID)
	})

	t.Run("lease recovery enables restart", func(t *testing.T) {
		req := automation.SubmitRequest{
			RunKind:        "hermes",
			TenantID:       "tenant-int",
			PropertyID:     "prop-int",
			ActorID:        "actor-int",
			TriggerType:    "event",
			TriggerID:      "evt-2",
			CorrelationID:  "corr-int-2",
			IdempotencyKey: "int-key-leased",
			Provider:       "stub",
			Model:          "test-model-v1",
		}

		run, _, err := store.Submit(ctx, req)
		if err != nil {
			t.Fatalf("submit: %v", err)
		}

		shortLease := 500 * time.Millisecond
		claimed, err := store.Claim(ctx, "worker-int-1", shortLease, nil)
		if err != nil {
			t.Fatalf("claim: %v", err)
		}
		if claimed.LeaseOwner != "worker-int-1" {
			t.Fatalf("lease owner mismatch")
		}

		time.Sleep(1 * time.Second)

		count, err := store.RecoverExpiredLeases(ctx)
		if err != nil {
			t.Fatalf("recover: %v", err)
		}
		if count < 1 {
			t.Fatalf("expected at least 1 recovered lease, got %d", count)
		}

		reclaimed, err := store.Claim(ctx, "worker-int-2", automation.DefaultLeaseDuration, nil)
		if err != nil {
			t.Fatalf("second claim after recovery: %v", err)
		}
		if reclaimed.RunID != run.RunID {
			t.Fatalf("wrong run claimed: %s vs %s", reclaimed.RunID, run.RunID)
		}
		if reclaimed.LeaseOwner != "worker-int-2" {
			t.Fatalf("reclaimed lease owner mismatch: expected worker-int-2, got %s", reclaimed.LeaseOwner)
		}

		err = store.Complete(ctx, run.RunID, "worker-int-2", []byte(`{}`), 50, "USD")
		if err != nil {
			t.Fatalf("complete: %v", err)
		}

		final, err := store.Get(ctx, run.RunID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if final.State != automation.StateCompleted {
			t.Fatalf("expected completed, got %s", final.State)
		}
		t.Logf("lease recovery: run %s completed after restart", run.RunID)
	})

	t.Run("cancel prevents later processing", func(t *testing.T) {
		req := automation.SubmitRequest{
			RunKind:        "jarvis",
			TenantID:       "tenant-int",
			PropertyID:     "prop-int",
			ActorID:        "actor-int",
			TriggerType:    "event",
			TriggerID:      "evt-3",
			CorrelationID:  "corr-int-3",
			IdempotencyKey: "int-key-cancel",
			Provider:       "stub",
			Model:          "test-model-v1",
		}

		run, _, err := store.Submit(ctx, req)
		if err != nil {
			t.Fatalf("submit: %v", err)
		}

		err = store.Cancel(ctx, run.RunID)
		if err != nil {
			t.Fatalf("cancel: %v", err)
		}

		cancelled, err := store.Get(ctx, run.RunID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if cancelled.State != automation.StateCancelled {
			t.Fatalf("expected cancelled, got %s", cancelled.State)
		}

		err = store.Cancel(ctx, run.RunID)
		if err != automation.ErrRunNotCancellable {
			t.Fatalf("double cancel must fail with ErrRunNotCancellable, got %v", err)
		}

		t.Logf("cancel: run %s is cancelled", run.RunID)
	})

	t.Run("events are recorded", func(t *testing.T) {
		req := automation.SubmitRequest{
			RunKind:        "jarvis",
			TenantID:       "tenant-int",
			PropertyID:     "prop-int",
			ActorID:        "actor-int",
			TriggerType:    "event",
			TriggerID:      "evt-4",
			CorrelationID:  "corr-int-4",
			IdempotencyKey: "int-key-events",
			Provider:       "stub",
			Model:          "test-model-v1",
		}

		run, _, err := store.Submit(ctx, req)
		if err != nil {
			t.Fatalf("submit: %v", err)
		}

		claimed, err := store.Claim(ctx, "wrk-evt", 30*time.Second, nil)
		if err != nil {
			t.Fatalf("claim: %v", err)
		}
		_ = claimed

		err = store.Complete(ctx, run.RunID, "wrk-evt", []byte(`{"done":true}`), 200, "USD")
		if err != nil {
			t.Fatalf("complete: %v", err)
		}

		events, err := automation.ListEvents(ctx, pool, run.RunID)
		if err != nil {
			t.Fatalf("list events: %v", err)
		}
		if len(events) == 0 {
			t.Fatal("must have at least 1 event")
		}

		hasQueued := false
		hasCompleted := false
		for _, e := range events {
			if e.EventName == automation.EventRunQueued {
				hasQueued = true
			}
			if e.EventName == automation.EventRunCompleted {
				hasCompleted = true
			}
		}
		if !hasQueued {
			t.Error("missing queued event")
		}
		if !hasCompleted {
			t.Error("missing completed event")
		}
		t.Logf("events: %d events recorded for run %s", len(events), run.RunID)
	})

	t.Run("model outage: failed runs are visible and retryable", func(t *testing.T) {
		req := automation.SubmitRequest{
			RunKind:        "hermes",
			TenantID:       "tenant-outage",
			PropertyID:     "prop-outage",
			ActorID:        "actor-outage",
			TriggerType:    "event",
			TriggerID:      "evt-outage",
			CorrelationID:  "corr-outage",
			IdempotencyKey: "int-key-outage",
			Provider:       "stub",
			Model:          "test-model-v1",
			MaxAttempts:    1,
		}

		run, _, err := store.Submit(ctx, req)
		if err != nil {
			t.Fatalf("submit during outage: %v", err)
		}
		if run.State != automation.StateQueued {
			t.Fatalf("submitted run must be queued, got %s", run.State)
		}

		_, err = store.Claim(ctx, "worker-outage", automation.DefaultLeaseDuration, nil)
		if err != nil {
			t.Fatalf("claim during outage: %v", err)
		}

		err = store.Fail(ctx, run.RunID, "worker-outage", "stub: provider unavailable")
		if err != nil {
			t.Fatalf("mark as failed during outage: %v", err)
		}

		failed, err := store.Get(ctx, run.RunID)
		if err != nil {
			t.Fatalf("get failed run: %v", err)
		}
		if failed.State != automation.StateFailed {
			t.Fatalf("run must be failed during outage, got %s", failed.State)
		}
		if failed.ErrorMessage != "stub: provider unavailable" {
			t.Fatalf("error message must be visible, got %q", failed.ErrorMessage)
		}

		events, err := automation.ListEvents(ctx, pool, run.RunID)
		if err != nil {
			t.Fatalf("list events for failed run: %v", err)
		}
		hasQueued := false
		hasFailed := false
		for _, e := range events {
			if e.EventName == automation.EventRunQueued {
				hasQueued = true
			}
			if e.EventName == automation.EventRunFailed {
				hasFailed = true
			}
		}
		if !hasQueued {
			t.Error("missing queued event for failed run")
		}
		if !hasFailed {
			t.Error("missing failed event for failed run")
		}

		err = store.Retry(ctx, run.RunID)
		if err != nil {
			t.Fatalf("retry failed run: %v", err)
		}

		retried, err := store.Get(ctx, run.RunID)
		if err != nil {
			t.Fatalf("get after retry: %v", err)
		}
		if retried.State != automation.StateQueued {
			t.Fatalf("retried run must be queued, got %s", retried.State)
		}

		reclaimed, err := store.Claim(ctx, "worker-recover", automation.DefaultLeaseDuration, nil)
		if err != nil {
			t.Fatalf("claim after retry: %v", err)
		}
		if reclaimed.RunID != run.RunID {
			t.Fatalf("wrong run claimed after retry: %s vs %s", reclaimed.RunID, run.RunID)
		}

		err = store.Complete(ctx, run.RunID, "worker-recover", []byte(`{"completed": true}`), 200, "USD")
		if err != nil {
			t.Fatalf("complete after retry: %v", err)
		}

		completed, err := store.Get(ctx, run.RunID)
		if err != nil {
			t.Fatalf("get completed: %v", err)
		}
		if completed.State != automation.StateCompleted {
			t.Fatalf("run must complete after retry, got %s", completed.State)
		}

		t.Logf("model outage: run %s failed, retried, and completed", run.RunID)
	})
}

func isPostgresReady() bool {
	host := os.Getenv("CC_DB_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("CC_DB_PORT")
	if port == "" {
		port = "5432"
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func dbConnString() string {
	host := os.Getenv("CC_DB_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("CC_DB_PORT")
	if port == "" {
		port = "5432"
	}
	user := os.Getenv("CC_DB_USER")
	if user == "" {
		user = "ccuser"
	}
	pass := os.Getenv("CC_DB_PASS")
	if pass == "" {
		pass = "ccpass"
	}
	name := testdb.MustName()
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, pass, host, port, name)
}

func createTestPool() (*pgxpool.Pool, error) {
	return pgxpool.New(context.Background(), dbConnString())
}
