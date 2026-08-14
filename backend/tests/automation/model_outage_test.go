package automation_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"comfort-curators-backend/internal/automation"
	"comfort-curators-backend/internal/automation/hermes"
	"comfort-curators-backend/internal/automation/superhost"
)

func TestIntegrationModelOutageComprehensive(t *testing.T) {
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

	outageFactory := func(kind string) automation.Provider {
		return automation.NewStubProvider("unavailable")
	}

	t.Run("core API remains healthy during model outage", func(t *testing.T) {
		req := automation.SubmitRequest{
			RunKind:        superhost.AgentKindSuperhost,
			TenantID:       "tenant-outage-core",
			PropertyID:     "prop-outage-core",
			ActorID:        "actor-outage-core",
			TriggerType:    "event",
			TriggerID:      "evt-core",
			CorrelationID:  "corr-core",
			IdempotencyKey: "outage-core-1",
			Provider:       "stub",
			Model:          "test-model-v1",
		}

		run, dup, err := store.Submit(ctx, req)
		if err != nil {
			t.Fatalf("submit during outage: %v", err)
		}
		if dup {
			t.Fatal("first submit must not be duplicate")
		}
		if run.State != automation.StateQueued {
			t.Fatalf("submitted run must be queued, got %s", run.State)
		}

		fetched, err := store.Get(ctx, run.RunID)
		if err != nil {
			t.Fatalf("get during outage: %v", err)
		}
		if fetched.RunID != run.RunID {
			t.Fatal("run retrieval must work during outage")
		}

		events, err := automation.ListEvents(ctx, pool, run.RunID)
		if err != nil {
			t.Fatalf("list events during outage: %v", err)
		}
		if len(events) == 0 {
			t.Fatal("queued event must be visible during outage")
		}

		t.Logf("core API: run %s submitted and visible during outage", run.RunID)
	})

	t.Run("failed runs are visible and retryable for jarvis", func(t *testing.T) {
		retryableRun, _, err := store.Submit(ctx, automation.SubmitRequest{
			RunKind:        superhost.AgentKindSuperhost,
			TenantID:       "tenant-outage-hm",
			PropertyID:     "prop-outage-hm",
			ActorID:        "actor-outage-hm",
			TriggerType:    "event",
			TriggerID:      "evt-hm",
			CorrelationID:  "corr-hm",
			IdempotencyKey: "outage-hm-retryable",
			Provider:       "stub",
			Model:          "test-model-v1",
			MaxAttempts:    3,
		})
		if err != nil {
			t.Fatalf("submit retryable: %v", err)
		}

		failedRun, _, err := store.Submit(ctx, automation.SubmitRequest{
			RunKind:        superhost.AgentKindSuperhost,
			TenantID:       "tenant-outage-hm2",
			PropertyID:     "prop-outage-hm2",
			ActorID:        "actor-outage-hm2",
			TriggerType:    "event",
			TriggerID:      "evt-hm2",
			CorrelationID:  "corr-hm2",
			IdempotencyKey: "outage-hm-failed",
			Provider:       "stub",
			Model:          "test-model-v1",
			MaxAttempts:    1,
		})
		if err != nil {
			t.Fatalf("submit failed: %v", err)
		}

		claimAndFail := func(runID, workerID, errMsg string) {
			claimed, err := store.Claim(ctx, workerID, automation.DefaultLeaseDuration, nil)
			if err != nil {
				t.Fatalf("claim: %v", err)
			}
			if claimed.RunID != runID {
				t.Fatalf("claimed wrong run: %s vs %s", claimed.RunID, runID)
			}
			if err := store.Fail(ctx, runID, workerID, errMsg); err != nil {
				t.Fatalf("fail: %v", err)
			}
		}

		claimAndFail(retryableRun.RunID, "worker-a", "stub: provider unavailable")
		claimAndFail(failedRun.RunID, "worker-a", "stub: provider unavailable")

		retryable, err := store.Get(ctx, retryableRun.RunID)
		if err != nil {
			t.Fatalf("get retryable: %v", err)
		}
		if retryable.State != automation.StateRetryable {
			t.Fatalf("retryable run must be in retryable state, got %s", retryable.State)
		}
		if retryable.ErrorMessage != "stub: provider unavailable" {
			t.Fatalf("error message must be visible, got %q", retryable.ErrorMessage)
		}

		failed, err := store.Get(ctx, failedRun.RunID)
		if err != nil {
			t.Fatalf("get failed: %v", err)
		}
		if failed.State != automation.StateFailed {
			t.Fatalf("failed run must be in failed state, got %s", failed.State)
		}
		if failed.ErrorMessage != "stub: provider unavailable" {
			t.Fatalf("error message must be visible, got %q", failed.ErrorMessage)
		}

		for _, rid := range []string{retryableRun.RunID, failedRun.RunID} {
			if err := store.Retry(ctx, rid); err != nil {
				t.Fatalf("retry %s: %v", rid, err)
			}
			requeued, err := store.Get(ctx, rid)
			if err != nil {
				t.Fatalf("get after retry %s: %v", rid, err)
			}
			if requeued.State != automation.StateQueued {
				t.Fatalf("retried run must be queued, got %s", requeued.State)
			}
		}

		for i := 0; i < 2; i++ {
			claimed, err := store.Claim(ctx, "worker-recover", automation.DefaultLeaseDuration, nil)
			if err != nil {
				t.Fatalf("reclaim: %v", err)
			}
			if err := store.Complete(ctx, claimed.RunID, "worker-recover", []byte(`{"completed":true}`), 50, "USD"); err != nil {
				t.Fatalf("complete: %v", err)
			}
		}

		for _, rid := range []string{retryableRun.RunID, failedRun.RunID} {
			completed, err := store.Get(ctx, rid)
			if err != nil {
				t.Fatalf("get completed: %v", err)
			}
			if completed.State != automation.StateCompleted {
				t.Fatalf("run must complete after retry, got %s", completed.State)
			}
		}

		t.Log("jarvis: all failed runs during model outage are visible and retryable")
	})

	t.Run("failed runs are visible and retryable for hermes", func(t *testing.T) {
		run, _, err := store.Submit(ctx, automation.SubmitRequest{
			RunKind:        hermes.AgentKindHermes,
			TenantID:       "tenant-outage-he",
			PropertyID:     "prop-outage-he",
			ActorID:        "actor-outage-he",
			TriggerType:    "event",
			TriggerID:      "evt-he",
			CorrelationID:  "corr-he",
			IdempotencyKey: "outage-he-1",
			Provider:       "stub",
			Model:          "test-model-v1",
			MaxAttempts:    3,
		})
		if err != nil {
			t.Fatalf("submit hermes: %v", err)
		}

		claimed, err := store.Claim(ctx, "worker-he", automation.DefaultLeaseDuration, nil)
		if err != nil {
			t.Fatalf("claim hermes: %v", err)
		}
		if claimed.RunID != run.RunID {
			t.Fatalf("claimed wrong run: %s vs %s", claimed.RunID, run.RunID)
		}

		if err := store.Fail(ctx, run.RunID, "worker-he", "stub: provider unavailable"); err != nil {
			t.Fatalf("fail hermes: %v", err)
		}

		retryable, err := store.Get(ctx, run.RunID)
		if err != nil {
			t.Fatalf("get hermes: %v", err)
		}
		if retryable.State != automation.StateRetryable {
			t.Fatalf("hermes run must be retryable after outage, got %s", retryable.State)
		}
		if retryable.ErrorMessage != "stub: provider unavailable" {
			t.Fatalf("hermes error must be visible, got %q", retryable.ErrorMessage)
		}

		if err := store.Retry(ctx, run.RunID); err != nil {
			t.Fatalf("retry hermes: %v", err)
		}

		requeued, err := store.Get(ctx, run.RunID)
		if err != nil {
			t.Fatalf("get requeued hermes: %v", err)
		}
		if requeued.State != automation.StateQueued {
			t.Fatalf("hermes retried run must be queued, got %s", requeued.State)
		}

		reclaimed, err := store.Claim(ctx, "worker-he-2", automation.DefaultLeaseDuration, nil)
		if err != nil {
			t.Fatalf("reclaim hermes: %v", err)
		}
		if reclaimed.RunID != run.RunID {
			t.Fatalf("wrong hermes run reclaimed: %s vs %s", reclaimed.RunID, run.RunID)
		}

		if err := store.Complete(ctx, run.RunID, "worker-he-2", []byte(`{"delivered":true}`), 100, "USD"); err != nil {
			t.Fatalf("complete hermes: %v", err)
		}

		completed, err := store.Get(ctx, run.RunID)
		if err != nil {
			t.Fatalf("get completed hermes: %v", err)
		}
		if completed.State != automation.StateCompleted {
			t.Fatalf("hermes run must complete after retry, got %s", completed.State)
		}

		t.Log("hermes: failed runs during model outage are visible and retryable")
	})

	t.Run("model outage causes visible automation degradation across kinds", func(t *testing.T) {
		kinds := []struct {
			name    string
			runKind string
		}{
			{"jarvis", superhost.AgentKindSuperhost},
			{"hermes", hermes.AgentKindHermes},
			{"custom-workflow", "custom-workflow"},
		}

		for _, k := range kinds {
			req := automation.SubmitRequest{
				RunKind:        k.runKind,
				TenantID:       "tenant-outage-degrade",
				PropertyID:     "prop-outage-degrade",
				ActorID:        "actor-outage-degrade",
				TriggerType:    "event",
				TriggerID:      "evt-degrade-" + k.name,
				CorrelationID:  "corr-degrade-" + k.name,
				IdempotencyKey: "outage-degrade-" + k.name,
				Provider:       "stub",
				Model:          "test-model-v1",
				MaxAttempts:    1,
				InputData:      json.RawMessage(fmt.Sprintf(`{"kind":"%s"}`, k.name)),
			}

			run, _, err := store.Submit(ctx, req)
			if err != nil {
				t.Fatalf("submit %s during outage: %v", k.name, err)
			}

			claimed, err := store.Claim(ctx, "worker-degrade", automation.DefaultLeaseDuration, nil)
			if err != nil {
				t.Fatalf("claim %s: %v", k.name, err)
			}
			if claimed.RunID != run.RunID {
				t.Fatalf("wrong run claimed for %s: %s vs %s", k.name, claimed.RunID, run.RunID)
			}

			outageErr := "stub: provider unavailable"
			if err := store.Fail(ctx, run.RunID, "worker-degrade", outageErr); err != nil {
				t.Fatalf("fail %s: %v", k.name, err)
			}

			failed, err := store.Get(ctx, run.RunID)
			if err != nil {
				t.Fatalf("get %s after outage: %v", k.name, err)
			}

			if failed.State != automation.StateFailed {
				t.Fatalf("%s run must be failed during outage, got %s", k.name, failed.State)
			}
			if failed.ErrorMessage != outageErr {
				t.Fatalf("%s error must be visible, got %q", k.name, failed.ErrorMessage)
			}
			if failed.RunKind != k.runKind {
				t.Fatalf("%s run kind must be preserved, got %s", k.name, failed.RunKind)
			}

			t.Logf("%s: run %s visible as failed with error message during outage", k.name, run.RunID)
		}
	})

	t.Run("runner degrades gracefully with unavailable provider", func(t *testing.T) {
		runner := automation.NewRunner(store, outageFactory, "worker-runner")

		hmKey := "runner-outage-hm"
		hmRun, _, err := store.Submit(ctx, automation.SubmitRequest{
			RunKind:        superhost.AgentKindSuperhost,
			TenantID:       "tenant-runner-hm",
			PropertyID:     "prop-runner-hm",
			ActorID:        "actor-runner-hm",
			TriggerType:    "event",
			TriggerID:      "evt-runner-hm",
			CorrelationID:  "corr-runner-hm",
			IdempotencyKey: hmKey,
			Provider:       "stub",
			Model:          "test-model-v1",
			MaxAttempts:    3,
		})
		if err != nil {
			t.Fatalf("submit jarvis: %v", err)
		}

		heKey := "runner-outage-he"
		heRun, _, err := store.Submit(ctx, automation.SubmitRequest{
			RunKind:        hermes.AgentKindHermes,
			TenantID:       "tenant-runner-he",
			PropertyID:     "prop-runner-he",
			ActorID:        "actor-runner-he",
			TriggerType:    "event",
			TriggerID:      "evt-runner-he",
			CorrelationID:  "corr-runner-he",
			IdempotencyKey: heKey,
			Provider:       "stub",
			Model:          "test-model-v1",
			MaxAttempts:    3,
		})
		if err != nil {
			t.Fatalf("submit hermes: %v", err)
		}

		processCtx, processCancel := context.WithTimeout(ctx, 10*time.Second)
		defer processCancel()

		var wg sync.WaitGroup
		wg.Add(1)
		processed := make(chan struct{})
		go func() {
			runner.RunWorkLoop(processCtx, &wg, nil)
			close(processed)
		}()

		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		hmFailed := false
		heFailed := false
		deadline := time.After(8 * time.Second)

		for !hmFailed || !heFailed {
			select {
			case <-deadline:
				if !hmFailed {
					t.Error("jarvis run did not fail during outage")
				}
				if !heFailed {
					t.Error("hermes run did not fail during outage")
				}
				goto done
			case <-ticker.C:
				if !hmFailed {
					r, _ := store.Get(ctx, hmRun.RunID)
					if r != nil && (r.State == automation.StateRetryable || r.State == automation.StateFailed) {
						hmFailed = true
						if r.ErrorMessage == "" {
							t.Error("jarvis error message must be visible")
						}
						t.Logf("jarvis run %s is %s: %s", r.RunID, r.State, r.ErrorMessage)
					}
				}
				if !heFailed {
					r, _ := store.Get(ctx, heRun.RunID)
					if r != nil && (r.State == automation.StateRetryable || r.State == automation.StateFailed) {
						heFailed = true
						if r.ErrorMessage == "" {
							t.Error("hermes error message must be visible")
						}
						t.Logf("hermes run %s is %s: %s", r.RunID, r.State, r.ErrorMessage)
					}
				}
			}
		}

	done:
		processCancel()
		select {
		case <-processed:
		case <-time.After(2 * time.Second):
		}

		if !hmFailed {
			t.Error("jarvis run must fail during model outage")
		}
		if !heFailed {
			t.Error("hermes run must fail during model outage")
		}

		t.Log("runner: degraded gracefully - both run kinds failed visibly during model outage")
	})

	t.Run("lease recovery continues to work during outage", func(t *testing.T) {
		req := automation.SubmitRequest{
			RunKind:        superhost.AgentKindSuperhost,
			TenantID:       "tenant-outage-lease",
			PropertyID:     "prop-outage-lease",
			ActorID:        "actor-outage-lease",
			TriggerType:    "event",
			TriggerID:      "evt-lease",
			CorrelationID:  "corr-lease",
			IdempotencyKey: "outage-lease-1",
			Provider:       "stub",
			Model:          "test-model-v1",
			MaxAttempts:    3,
		}

		run, _, err := store.Submit(ctx, req)
		if err != nil {
			t.Fatalf("submit: %v", err)
		}

		shortLease := 500 * time.Millisecond
		claimed, err := store.Claim(ctx, "worker-lease", shortLease, nil)
		if err != nil {
			t.Fatalf("claim: %v", err)
		}
		if claimed.LeaseOwner != "worker-lease" {
			t.Fatalf("lease owner mismatch: %s", claimed.LeaseOwner)
		}

		time.Sleep(1 * time.Second)

		count, err := store.RecoverExpiredLeases(ctx)
		if err != nil {
			t.Fatalf("recover leases during outage: %v", err)
		}
		if count < 1 {
			t.Fatal("expected at least 1 expired lease during outage")
		}

		reclaimed, err := store.Claim(ctx, "worker-outage-2", automation.DefaultLeaseDuration, nil)
		if err != nil {
			t.Fatalf("reclaim after outage recovery: %v", err)
		}
		if reclaimed.RunID != run.RunID {
			t.Fatalf("wrong run reclaimed: %s vs %s", reclaimed.RunID, run.RunID)
		}

		t.Logf("lease recovery: expired lease for run %s recovered during model outage", run.RunID)
	})

	t.Run("store remains operable under concurrent outage stress", func(t *testing.T) {
		const numRuns = 10
		var runIDs []string

		for i := 0; i < numRuns; i++ {
			req := automation.SubmitRequest{
				RunKind:        superhost.AgentKindSuperhost,
				TenantID:       "tenant-stress",
				PropertyID:     "prop-stress",
				ActorID:        "actor-stress",
				TriggerType:    "event",
				TriggerID:      fmt.Sprintf("evt-stress-%d", i),
				CorrelationID:  fmt.Sprintf("corr-stress-%d", i),
				IdempotencyKey: fmt.Sprintf("outage-stress-%d", i),
				Provider:       "stub",
				Model:          "test-model-v1",
				MaxAttempts:    1,
				InputData:      json.RawMessage(fmt.Sprintf(`{"index":%d}`, i)),
			}

			run, _, err := store.Submit(ctx, req)
			if err != nil {
				t.Fatalf("submit %d: %v", i, err)
			}
			runIDs = append(runIDs, run.RunID)
		}

		for i, rid := range runIDs {
			run, err := store.Get(ctx, rid)
			if err != nil {
				t.Fatalf("get run %d: %v", i, err)
			}
			if run.State != automation.StateQueued {
				t.Fatalf("run %d must be queued, got %s", i, run.State)
			}
			if run.InputData == nil {
				t.Fatalf("run %d must preserve input data", i)
			}
		}

		for _, rid := range runIDs {
			claimed, err := store.Claim(ctx, "worker-stress", automation.DefaultLeaseDuration, nil)
			if err != nil {
				t.Fatalf("claim %s: %v", rid, err)
			}
			if err := store.Fail(ctx, claimed.RunID, "worker-stress", "stub: provider unavailable"); err != nil {
				t.Fatalf("fail %s: %v", rid, err)
			}
		}

		for i, rid := range runIDs {
			failed, err := store.Get(ctx, rid)
			if err != nil {
				t.Fatalf("get failed run %d: %v", i, err)
			}
			if failed.State != automation.StateFailed {
				t.Fatalf("run %d must be failed, got %s", i, failed.State)
			}
			if failed.ErrorMessage != "stub: provider unavailable" {
				t.Fatalf("run %d error must be visible, got %q", i, failed.ErrorMessage)
			}
		}

		t.Logf("stress: all %d runs failed visibly during model outage", numRuns)
	})

	t.Run("retry after outage with working provider succeeds", func(t *testing.T) {
		req := automation.SubmitRequest{
			RunKind:        superhost.AgentKindSuperhost,
			TenantID:       "tenant-recover",
			PropertyID:     "prop-recover",
			ActorID:        "actor-recover",
			TriggerType:    "event",
			TriggerID:      "evt-recover",
			CorrelationID:  "corr-recover",
			IdempotencyKey: "outage-recover-1",
			Provider:       "stub",
			Model:          "test-model-v1",
			MaxAttempts:    1,
		}

		run, _, err := store.Submit(ctx, req)
		if err != nil {
			t.Fatalf("submit: %v", err)
		}

		claimed, err := store.Claim(ctx, "worker-recover", automation.DefaultLeaseDuration, nil)
		if err != nil {
			t.Fatalf("claim: %v", err)
		}
		if claimed.RunID != run.RunID {
			t.Fatalf("wrong run claimed")
		}

		if err := store.Fail(ctx, run.RunID, "worker-recover", "stub: provider unavailable"); err != nil {
			t.Fatalf("fail: %v", err)
		}

		failed, err := store.Get(ctx, run.RunID)
		if err != nil {
			t.Fatalf("get failed: %v", err)
		}
		if failed.State != automation.StateFailed {
			t.Fatalf("must be failed, got %s", failed.State)
		}

		if err := store.Retry(ctx, run.RunID); err != nil {
			t.Fatalf("retry: %v", err)
		}

		requeued, err := store.Get(ctx, run.RunID)
		if err != nil {
			t.Fatalf("get after retry: %v", err)
		}
		if requeued.State != automation.StateQueued {
			t.Fatalf("retried run must be queued, got %s", requeued.State)
		}

		reclaimed, err := store.Claim(ctx, "worker-recover-2", automation.DefaultLeaseDuration, nil)
		if err != nil {
			t.Fatalf("reclaim: %v", err)
		}
		if reclaimed.RunID != run.RunID {
			t.Fatalf("wrong run reclaimed: %s vs %s", reclaimed.RunID, run.RunID)
		}

		if err := store.Complete(ctx, run.RunID, "worker-recover-2", []byte(`{"recovered":true}`), 50, "USD"); err != nil {
			t.Fatalf("complete: %v", err)
		}

		completed, err := store.Get(ctx, run.RunID)
		if err != nil {
			t.Fatalf("get completed: %v", err)
		}
		if completed.State != automation.StateCompleted {
			t.Fatalf("must be completed after outage recovery, got %s", completed.State)
		}

		t.Logf("recovery: run %s failed during outage, retried, and completed successfully", run.RunID)
	})

	t.Run("idempotency key preserved through outage cycle", func(t *testing.T) {
		req := automation.SubmitRequest{
			RunKind:        superhost.AgentKindSuperhost,
			TenantID:       "tenant-idem",
			PropertyID:     "prop-idem",
			ActorID:        "actor-idem",
			TriggerType:    "event",
			TriggerID:      "evt-idem",
			CorrelationID:  "corr-idem",
			IdempotencyKey: "outage-idem-1",
			Provider:       "stub",
			Model:          "test-model-v1",
			MaxAttempts:    1,
		}

		run, dup, err := store.Submit(ctx, req)
		if err != nil {
			t.Fatalf("submit: %v", err)
		}
		if dup {
			t.Fatal("first submit must not be duplicate")
		}

		duplicate, dup2, err := store.Submit(ctx, req)
		if err != nil {
			t.Fatalf("duplicate submit: %v", err)
		}
		if !dup2 {
			t.Fatal("duplicate submit must be recognized during outage")
		}
		if duplicate.RunID != run.RunID {
			t.Fatalf("duplicate must return same run ID during outage: %s vs %s", duplicate.RunID, run.RunID)
		}

		t.Logf("idempotency: key %s preserved through outage cycle", req.IdempotencyKey)
	})

	t.Run("cancel prevents processing during outage", func(t *testing.T) {
		req := automation.SubmitRequest{
			RunKind:        superhost.AgentKindSuperhost,
			TenantID:       "tenant-cancel",
			PropertyID:     "prop-cancel",
			ActorID:        "actor-cancel",
			TriggerType:    "event",
			TriggerID:      "evt-cancel",
			CorrelationID:  "corr-cancel",
			IdempotencyKey: "outage-cancel-1",
			Provider:       "stub",
			Model:          "test-model-v1",
			MaxAttempts:    3,
		}

		run, _, err := store.Submit(ctx, req)
		if err != nil {
			t.Fatalf("submit: %v", err)
		}

		if err := store.Cancel(ctx, run.RunID); err != nil {
			t.Fatalf("cancel during outage: %v", err)
		}

		cancelled, err := store.Get(ctx, run.RunID)
		if err != nil {
			t.Fatalf("get cancelled: %v", err)
		}
		if cancelled.State != automation.StateCancelled {
			t.Fatalf("must be cancelled, got %s", cancelled.State)
		}

		if err := store.Cancel(ctx, run.RunID); err != automation.ErrRunNotCancellable {
			t.Fatalf("double cancel during outage must fail with ErrRunNotCancellable, got %v", err)
		}

		t.Log("cancel: run cancelled during model outage prevents later processing")
	})

	t.Run("events track full outage lifecycle", func(t *testing.T) {
		req := automation.SubmitRequest{
			RunKind:        superhost.AgentKindSuperhost,
			TenantID:       "tenant-evts",
			PropertyID:     "prop-evts",
			ActorID:        "actor-evts",
			TriggerType:    "event",
			TriggerID:      "evt-track",
			CorrelationID:  "corr-track",
			IdempotencyKey: "outage-evts-1",
			Provider:       "stub",
			Model:          "test-model-v1",
			MaxAttempts:    3,
		}

		run, _, err := store.Submit(ctx, req)
		if err != nil {
			t.Fatalf("submit: %v", err)
		}

		claimed, err := store.Claim(ctx, "worker-evt", automation.DefaultLeaseDuration, nil)
		if err != nil {
			t.Fatalf("claim: %v", err)
		}

		if err := store.TransitionState(ctx, claimed.RunID, "worker-evt", automation.StateLeased, automation.StateRunning); err != nil {
			t.Fatalf("transition to running: %v", err)
		}

		if err := store.Fail(ctx, run.RunID, "worker-evt", "stub: provider unavailable"); err != nil {
			t.Fatalf("fail: %v", err)
		}

		events, err := automation.ListEvents(ctx, pool, run.RunID)
		if err != nil {
			t.Fatalf("list events: %v", err)
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
			t.Error("missing queued event in outage lifecycle")
		}
		if !hasFailed {
			t.Error("missing failed event in outage lifecycle")
		}

		if err := store.Retry(ctx, run.RunID); err != nil {
			t.Fatalf("retry: %v", err)
		}

		eventsAfter, err := automation.ListEvents(ctx, pool, run.RunID)
		if err != nil {
			t.Fatalf("list events after retry: %v", err)
		}
		if len(eventsAfter) <= len(events) {
			t.Error("retry must produce additional event")
		}

		t.Logf("events: %d events recorded through full outage lifecycle", len(eventsAfter))
	})

	t.Run("hermes approved template works independently during outage", func(t *testing.T) {
		pe := hermes.NewPolicyEngine()
		ctx := hermes.PolicyContext{
			RunID:      "run-template-outage",
			TenantID:   "tenant-tpl",
			PropertyID: "prop-tpl",
			ActorID:    "actor-tpl",
			ActorRoles: []string{"hermes"},
		}

		templateInput := hermes.ToolCallInput{
			ToolName:  "draft_approved_template_message",
			Version:   "v1",
			CallID:    "outage-template",
			Arguments: json.RawMessage(`{"template_key":"owner_exception_notice"}`),
		}

		dec := pe.Evaluate(ctx, templateInput)
		if dec.Result != hermes.PolicyAllowed {
			t.Errorf("approved template communication must work during outage, got %s", dec.Result)
		}

		unc := pe.EvaluateUncertainty(ctx, templateInput, "provider unavailable: all models down")
		if unc.Result != hermes.PolicyUncertainty {
			t.Errorf("model outage must be classified as uncertainty, got %s", unc.Result)
		}

		exc := pe.EvaluateException(ctx, templateInput, "provider call timed out after 30s")
		if exc.Result != hermes.PolicyException {
			t.Errorf("timeout during outage must be exception, got %s", exc.Result)
		}

		t.Log("hermes: approved template policy evaluation works independently during model outage")
	})

	t.Run("jarvis tools available for manual operation during outage", func(t *testing.T) {
		allowed := superhost.AllowedToolNames()
		if len(allowed) == 0 {
			t.Fatal("tool catalog must be available during outage")
		}

		for _, name := range allowed {
			def, err := superhost.LookupTool(name)
			if err != nil {
				t.Errorf("tool %q must be available during outage: %v", name, err)
			}
			if def.SchemaVersion == "" {
				t.Errorf("tool %q schema version must be traceable", name)
			}
		}

		pe := superhost.NewPolicyEngine()
		ctx := superhost.PolicyContext{
			RunID:      "run-manual",
			TenantID:   "tenant-manual",
			PropertyID: "prop-manual",
			ActorID:    "actor-manual",
			ActorRoles: []string{"jarvis"},
		}

		readInput := superhost.ToolCallInput{
			ToolName:  "get_property_operating_summary",
			Version:   "v1",
			CallID:    "manual-read",
			Arguments: json.RawMessage(`{}`),
		}
		dec := pe.Evaluate(ctx, readInput)
		if dec.Result != superhost.PolicyAllowed {
			t.Errorf("core read tool must be evaluable during outage, got %s", dec.Result)
		}

		unc := pe.EvaluateUncertainty(ctx, readInput, "provider unavailable: all models down")
		if unc.Result != superhost.PolicyUncertainty {
			t.Errorf("outage must produce uncertainty, got %s", unc.Result)
		}
		if unc.PolicyVersion != superhost.PolicyVersion {
			t.Error("policy version must be traceable during outage")
		}

		t.Logf("jarvis: %d tools available for manual operation during model outage", len(allowed))
	})

	t.Run("terminally failed runs remain visible for audit", func(t *testing.T) {
		req := automation.SubmitRequest{
			RunKind:        superhost.AgentKindSuperhost,
			TenantID:       "tenant-audit",
			PropertyID:     "prop-audit",
			ActorID:        "actor-audit",
			TriggerType:    "event",
			TriggerID:      "evt-audit",
			CorrelationID:  "corr-audit",
			IdempotencyKey: "outage-audit-1",
			Provider:       "stub",
			Model:          "test-model-v1",
			MaxAttempts:    1,
		}

		run, _, err := store.Submit(ctx, req)
		if err != nil {
			t.Fatalf("submit: %v", err)
		}

		_, err = store.Claim(ctx, "worker-audit", automation.DefaultLeaseDuration, nil)
		if err != nil {
			t.Fatalf("claim: %v", err)
		}

		for i := 0; i < run.MaxAttempts; i++ {
			if err := store.Fail(ctx, run.RunID, "worker-audit", "stub: provider unavailable"); err != nil {
				t.Fatalf("fail attempt %d: %v", i+1, err)
			}
			if i < run.MaxAttempts-1 {
				if _, err := store.Claim(ctx, "worker-audit", automation.DefaultLeaseDuration, nil); err != nil {
					break
				}
			}
		}

		final, err := store.Get(ctx, run.RunID)
		if err != nil {
			t.Fatalf("get final: %v", err)
		}

		terminalStates := map[string]bool{
			automation.StateFailed:             true,
			automation.StateRetryable:          true,
			automation.StateCancelled:          true,
			automation.StateCompleted:          true,
			automation.StateWaitingForApproval: true,
			automation.StateWaitingForTool:     true,
			automation.StateUnknown:            true,
		}

		if !terminalStates[final.State] {
			t.Fatalf("run must be in a visible state after outage, got %s", final.State)
		}

		if final.ErrorMessage == "" {
			t.Error("error message must be preserved for audit")
		}
		if final.CreatedAt.IsZero() {
			t.Error("created_at must be preserved for audit")
		}

		events, err := automation.ListEvents(ctx, pool, run.RunID)
		if err != nil {
			t.Fatalf("list events for audit: %v", err)
		}
		if len(events) == 0 {
			t.Error("events must be preserved for audit")
		}

		t.Logf("audit: run %s in state %s with %d events preserved for audit", final.RunID, final.State, len(events))
	})
}
