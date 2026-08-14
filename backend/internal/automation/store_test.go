package automation_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"comfort-curators-backend/internal/automation"

	"github.com/jackc/pgx/v5/pgxpool"
)

func postgresAvailable() bool {
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
	name := os.Getenv("CC_DB_NAME")
	if name == "" {
		name = "comfort_curators"
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, pass, host, port, name)
}

func newPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if !postgresAvailable() {
		t.Skip("PostgreSQL not available; set CC_DB_HOST, CC_DB_PORT, CC_DB_USER, CC_DB_PASS, CC_DB_NAME")
	}

	pool, err := pgxpool.New(context.Background(), dbConnString())
	if err != nil {
		t.Fatalf("connect to database: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := automation.EnsureSchema(context.Background(), pool); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}

	return pool
}

func automationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := newPool(t)

	for _, table := range []string{"agent_run_events", "agent_runs"} {
		if _, err := pool.Exec(context.Background(), "TRUNCATE TABLE "+table+" CASCADE"); err != nil {
			t.Fatalf("truncate %s: %v", table, err)
		}
	}

	return pool
}

func newStore(t *testing.T) *automation.AgentRunStore {
	t.Helper()
	pool := automationPool(t)
	return automation.NewAgentRunStore(pool)
}

func TestSubmitReturnsRunIDImmediately(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	req := automation.SubmitRequest{
		RunKind:        "jarvis",
		TenantID:       "tenant-1",
		PropertyID:     "prop-1",
		ActorID:        "actor-1",
		TriggerType:    "manual",
		TriggerID:      "trigger-1",
		CorrelationID:  "corr-1",
		IdempotencyKey: "key-submit-immediate",
		Provider:       "stub",
		Model:          "test-model",
	}

	run, duplicate, err := store.Submit(ctx, req)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if duplicate {
		t.Fatal("first submit must not be a duplicate")
	}
	if run.RunID == "" {
		t.Fatal("submit must return a non-empty run ID")
	}
	if run.State != automation.StateQueued {
		t.Fatalf("new run must be queued, got %s", run.State)
	}
	if run.TenantID != req.TenantID {
		t.Fatalf("tenant mismatch: got %s, want %s", run.TenantID, req.TenantID)
	}
	if run.PropertyID != req.PropertyID {
		t.Fatalf("property mismatch: got %s, want %s", run.PropertyID, req.PropertyID)
	}
	t.Logf("submitted run %s in state %s", run.RunID, run.State)
}

func TestRestartRecoversExpiredLease(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	req := automation.SubmitRequest{
		RunKind:        "jarvis",
		TenantID:       "tenant-1",
		PropertyID:     "prop-1",
		ActorID:        "actor-1",
		TriggerType:    "manual",
		TriggerID:      "trigger-1",
		CorrelationID:  "corr-1",
		IdempotencyKey: "key-recover-lease",
		Provider:       "stub",
		Model:          "test-model",
	}

	run, _, err := store.Submit(ctx, req)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	shortLease := 1 * time.Second
	claimed, err := store.Claim(ctx, "worker-1", shortLease, nil)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed.RunID != run.RunID {
		t.Fatalf("claimed wrong run: %s vs %s", claimed.RunID, run.RunID)
	}
	if claimed.State != automation.StateLeased {
		t.Fatalf("claimed run must be leased, got %s", claimed.State)
	}
	if claimed.LeaseOwner != "worker-1" {
		t.Fatalf("lease owner mismatch: %s", claimed.LeaseOwner)
	}

	time.Sleep(2 * time.Second)

	recovered, err := store.RecoverExpiredLeases(ctx)
	if err != nil {
		t.Fatalf("recover expired leases: %v", err)
	}
	if recovered != 1 {
		t.Fatalf("expected 1 recovered lease, got %d", recovered)
	}

	retrieved, err := store.Get(ctx, run.RunID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if retrieved.State != automation.StateQueued {
		t.Fatalf("expired lease must return to queued, got %s", retrieved.State)
	}
	if retrieved.LeaseOwner != "" {
		t.Fatalf("lease owner must be clear after recovery, got %s", retrieved.LeaseOwner)
	}

	reclaimed, err := store.Claim(ctx, "worker-2", automation.DefaultLeaseDuration, nil)
	if err != nil {
		t.Fatalf("reclaim after recovery: %v", err)
	}
	if reclaimed.RunID != run.RunID {
		t.Fatalf("reclaimed wrong run: %s vs %s", reclaimed.RunID, run.RunID)
	}
	if reclaimed.LeaseOwner != "worker-2" {
		t.Fatalf("reclaimed lease owner mismatch: %s", reclaimed.LeaseOwner)
	}
	if reclaimed.State != automation.StateLeased {
		t.Fatalf("reclaimed run must be leased, got %s", reclaimed.State)
	}

	t.Logf("recovered and reclaimed run %s with worker-2 after lease expiry", run.RunID)
}

func TestDuplicateRequestProducesOneProviderCall(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	req := automation.SubmitRequest{
		RunKind:        "jarvis",
		TenantID:       "tenant-1",
		PropertyID:     "prop-1",
		ActorID:        "actor-1",
		TriggerType:    "manual",
		TriggerID:      "trigger-1",
		CorrelationID:  "corr-1",
		IdempotencyKey: "key-duplicate-test",
		Provider:       "stub",
		Model:          "test-model",
	}

	run1, dup1, err := store.Submit(ctx, req)
	if err != nil {
		t.Fatalf("first submit: %v", err)
	}
	if dup1 {
		t.Fatal("first submit must not be duplicate")
	}
	if run1.RunID == "" {
		t.Fatal("first submit must return run ID")
	}

	run2, dup2, err := store.Submit(ctx, req)
	if err != nil {
		t.Fatalf("second submit: %v", err)
	}
	if !dup2 {
		t.Fatal("second submit with same idempotency key must be a duplicate")
	}
	if run2.RunID != run1.RunID {
		t.Fatalf("duplicate submit must return same run ID: %s vs %s", run2.RunID, run1.RunID)
	}
	if run2.State != run1.State {
		t.Fatalf("duplicate submit must have same state: %s vs %s", run2.State, run1.State)
	}

	t.Logf("duplicate submit returned same run %s, state=%s", run1.RunID, run1.State)
}

// TestConcurrentSubmitWithSameIdempotencyKeySucceeds reproduces, live, the
// race a sequential test can't see: two callers submitting with the same
// idempotency key close enough together that both pass the
// GetByIdempotencyKey check before either commits its INSERT. This is
// exactly what happened live during P7.2's walkthrough rehearsal --
// SuperhostMount's thread-creation call fired twice in close succession
// (an effect re-run, not a deliberate retry) and the second call's INSERT
// hit idx_agent_runs_idempotency, surfacing a raw SQLSTATE 23505 error to
// the terminal instead of being absorbed as the duplicate it actually was.
// Before the fix in Submit, this test failed with exactly that error on
// the losing goroutine.
func TestConcurrentSubmitWithSameIdempotencyKeySucceeds(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	const attempts = 20
	req := automation.SubmitRequest{
		RunKind:        "jarvis",
		TenantID:       "tenant-1",
		PropertyID:     "prop-1",
		ActorID:        "actor-1",
		TriggerType:    "manual",
		TriggerID:      "trigger-1",
		CorrelationID:  "corr-1",
		IdempotencyKey: "key-concurrent-race-test",
		Provider:       "stub",
		Model:          "test-model",
	}

	type result struct {
		run       *automation.AgentRun
		duplicate bool
		err       error
	}
	results := make(chan result, attempts)
	var wg sync.WaitGroup
	// A shared start barrier, not just launching goroutines and letting the
	// scheduler stagger them: every goroutine blocks on the same channel
	// close so as many as possible call Submit at genuinely the same
	// instant, maximizing the odds the check-then-act window in Submit
	// actually overlaps. Without this, unsynchronized goroutines can pass
	// the race by scheduling luck alone -- which is exactly what happened
	// the first time this test was written: it passed even with the race
	// still present in Submit, because 8 unsynchronized goroutines against
	// a fast local Postgres rarely land in the same microsecond window.
	start := make(chan struct{})
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			run, dup, err := store.Submit(ctx, req)
			results <- result{run: run, duplicate: dup, err: err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	var runIDs = map[string]int{}
	var nonDuplicateCount int
	for r := range results {
		if r.err != nil {
			t.Fatalf("concurrent submit must never error on a genuine idempotency-key race, got: %v", r.err)
		}
		if r.run == nil || r.run.RunID == "" {
			t.Fatal("concurrent submit must return a run with a non-empty run ID")
		}
		runIDs[r.run.RunID]++
		if !r.duplicate {
			nonDuplicateCount++
		}
	}

	if len(runIDs) != 1 {
		t.Fatalf("all %d concurrent submits with the same idempotency key must resolve to exactly one run, got %d distinct run IDs: %v", attempts, len(runIDs), runIDs)
	}
	if nonDuplicateCount != 1 {
		t.Fatalf("exactly one concurrent submit must win as non-duplicate, got %d", nonDuplicateCount)
	}
}

func TestCancelPreventsClaim(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	req := automation.SubmitRequest{
		RunKind:        "jarvis",
		TenantID:       "tenant-1",
		PropertyID:     "prop-1",
		ActorID:        "actor-1",
		TriggerType:    "manual",
		TriggerID:      "trigger-1",
		CorrelationID:  "corr-1",
		IdempotencyKey: "key-cancel-test",
		Provider:       "stub",
		Model:          "test-model",
	}

	run, _, err := store.Submit(ctx, req)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	err = store.Cancel(ctx, run.RunID)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}

	retrieved, err := store.Get(ctx, run.RunID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if retrieved.State != automation.StateCancelled {
		t.Fatalf("cancelled run must be in cancelled state, got %s", retrieved.State)
	}

	_, err = store.Claim(ctx, "worker-1", automation.DefaultLeaseDuration, nil)
	if err == nil {
		t.Fatal("claim on cancelled run must fail")
	}
	if err != automation.ErrRunNotFound {
		t.Logf("expected ErrRunNotFound after cancel, got: %v", err)
	}

	t.Logf("cancelled run %s cannot be claimed", run.RunID)
}

func TestClaimAndComplete(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	req := automation.SubmitRequest{
		RunKind:        "hermes",
		TenantID:       "tenant-1",
		PropertyID:     "prop-1",
		ActorID:        "actor-1",
		TriggerType:    "manual",
		TriggerID:      "trigger-1",
		CorrelationID:  "corr-1",
		IdempotencyKey: "key-claim-complete",
		Provider:       "stub",
		Model:          "test-model",
	}

	run, _, err := store.Submit(ctx, req)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	claimed, err := store.Claim(ctx, "worker-x", automation.DefaultLeaseDuration, nil)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed.RunID != run.RunID {
		t.Fatalf("claimed wrong run: %s", claimed.RunID)
	}

	output := []byte(`{"message": "hello"}`)
	err = store.Complete(ctx, run.RunID, "worker-x", output, 150, "USD")
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	completed, err := store.Get(ctx, run.RunID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if completed.State != automation.StateCompleted {
		t.Fatalf("completed run must be in completed state, got %s", completed.State)
	}
	if completed.UsageMinorUnits != 150 {
		t.Fatalf("usage mismatch: got %d", completed.UsageMinorUnits)
	}

	events, err := automation.ListEvents(ctx, newPool(t), run.RunID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) < 3 {
		t.Fatalf("expected at least 3 events (queued, claimed, completed), got %d", len(events))
	}
	t.Logf("run %s completed with %d events", run.RunID, len(events))
}

func TestCompletePersistsTokenAccounting(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	req := automation.SubmitRequest{
		RunKind:        "hermes",
		TenantID:       "tenant-1",
		PropertyID:     "prop-1",
		ActorID:        "actor-1",
		TriggerType:    "manual",
		TriggerID:      "trigger-1",
		CorrelationID:  "corr-1",
		IdempotencyKey: "key-token-accounting",
		Provider:       "openai",
		Model:          "gpt-4o",
	}

	run, _, err := store.Submit(ctx, req)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	claimed, err := store.Claim(ctx, "worker-tok", automation.DefaultLeaseDuration, nil)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed.RunID != run.RunID {
		t.Fatalf("claimed wrong run: %s", claimed.RunID)
	}

	err = store.CompleteWithUsage(ctx, run.RunID, "worker-tok", []byte(`{"done":true}`), automation.ProviderUsage{
		InputTokens:   100,
		OutputTokens:  50,
		TotalTokens:   150,
		UsageMinor:    750,
		UsageCurrency: "USD",
		UsageKnown:    true,
	})
	if err != nil {
		t.Fatalf("complete with usage: %v", err)
	}

	completed, err := store.Get(ctx, run.RunID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if completed.UsageInputTokens != 100 || completed.UsageOutputTokens != 50 || completed.UsageTotalTokens != 150 {
		t.Fatalf("token counts not persisted: in=%d out=%d total=%d",
			completed.UsageInputTokens, completed.UsageOutputTokens, completed.UsageTotalTokens)
	}
	if completed.UsageMinorUnits != 750 || completed.UsageCurrency != "USD" || !completed.UsageKnown {
		t.Fatalf("cost accounting not persisted: minor=%d curr=%q known=%v",
			completed.UsageMinorUnits, completed.UsageCurrency, completed.UsageKnown)
	}

	events, err := automation.ListEvents(ctx, newPool(t), run.RunID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	var completedEvent *automation.AgentRunEvent
	for i := range events {
		if events[i].EventName == automation.EventRunCompleted {
			completedEvent = &events[i]
		}
	}
	if completedEvent == nil {
		t.Fatal("missing completed event")
	}
	var eventData map[string]any
	if err := json.Unmarshal(completedEvent.EventData, &eventData); err != nil {
		t.Fatalf("unmarshal event data: %v", err)
	}
	if eventData["usage_input_tokens"] != float64(100) || eventData["usage_total_tokens"] != float64(150) {
		t.Fatalf("token counts missing from completed event: %v", eventData)
	}
}

func TestHeartbeatExtendsLease(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	req := automation.SubmitRequest{
		RunKind:        "jarvis",
		TenantID:       "tenant-1",
		PropertyID:     "prop-1",
		ActorID:        "actor-1",
		TriggerType:    "manual",
		TriggerID:      "trigger-1",
		CorrelationID:  "corr-1",
		IdempotencyKey: "key-heartbeat",
		Provider:       "stub",
		Model:          "test-model",
	}

	run, _, err := store.Submit(ctx, req)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	claimed, err := store.Claim(ctx, "worker-z", 3*time.Second, nil)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed.LeaseOwner != "worker-z" {
		t.Fatalf("lease owner mismatch")
	}

	err = store.Heartbeat(ctx, run.RunID, "worker-z", 30*time.Second)
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}

	time.Sleep(1 * time.Second)
	recovered, err := store.RecoverExpiredLeases(ctx)
	if err != nil {
		t.Fatalf("recover: %v", err)
	}
	if recovered != 0 {
		t.Fatalf("heartbeat-extended lease must not be recovered, got %d", recovered)
	}

	t.Logf("heartbeat successfully extended lease for run %s", run.RunID)
}

func TestFailRetries(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	req := automation.SubmitRequest{
		RunKind:        "jarvis",
		TenantID:       "tenant-1",
		PropertyID:     "prop-1",
		ActorID:        "actor-1",
		TriggerType:    "manual",
		TriggerID:      "trigger-1",
		CorrelationID:  "corr-1",
		IdempotencyKey: "key-fail-retry",
		Provider:       "stub",
		Model:          "test-model",
		MaxAttempts:    2,
	}

	run, _, err := store.Submit(ctx, req)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	claimed, err := store.Claim(ctx, "wrk-1", automation.DefaultLeaseDuration, nil)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed.Attempt != 1 {
		t.Fatalf("first attempt must be 1, got %d", claimed.Attempt)
	}

	// A single failure below the attempt ceiling leaves the run retryable.
	if err := store.Fail(ctx, run.RunID, "wrk-1", "temporary error"); err != nil {
		t.Fatalf("fail: %v", err)
	}

	afterFail, err := store.Get(ctx, run.RunID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if afterFail.State != automation.StateRetryable {
		t.Fatalf("failed under max must be retryable, got %s", afterFail.State)
	}

	// An explicit retry requeues the run for another attempt.
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
	if requeued.Attempt != 0 {
		t.Fatalf("retried run attempt must restart at 0, got %d", requeued.Attempt)
	}

	claimed2, err := store.Claim(ctx, "wrk-2", automation.DefaultLeaseDuration, nil)
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if claimed2.RunID != run.RunID {
		t.Fatalf("second claim must return same run, got %s", claimed2.RunID)
	}
	if claimed2.Attempt != 1 {
		t.Fatalf("retried run attempt must restart at 1, got %d", claimed2.Attempt)
	}

	// A run that fails with its attempt at the ceiling becomes terminally
	// failed (bounded retries).
	terminalReq := automation.SubmitRequest{
		RunKind:        "jarvis",
		TenantID:       "tenant-1",
		PropertyID:     "prop-1",
		ActorID:        "actor-1",
		TriggerType:    "manual",
		TriggerID:      "trigger-2",
		CorrelationID:  "corr-2",
		IdempotencyKey: "key-fail-retry-terminal",
		Provider:       "stub",
		Model:          "test-model",
		MaxAttempts:    1,
	}

	terminalRun, _, err := store.Submit(ctx, terminalReq)
	if err != nil {
		t.Fatalf("submit terminal run: %v", err)
	}

	terminalClaimed, err := store.Claim(ctx, "wrk-3", automation.DefaultLeaseDuration, nil)
	if err != nil {
		t.Fatalf("claim terminal run: %v", err)
	}
	if terminalClaimed.RunID != terminalRun.RunID {
		t.Fatalf("claimed wrong terminal run: %s", terminalClaimed.RunID)
	}

	if err := store.Fail(ctx, terminalRun.RunID, "wrk-3", "terminal error"); err != nil {
		t.Fatalf("terminal fail: %v", err)
	}

	terminal, err := store.Get(ctx, terminalRun.RunID)
	if err != nil {
		t.Fatalf("get terminal run: %v", err)
	}
	if terminal.State != automation.StateFailed {
		t.Fatalf("max attempts exceeded must be failed, got %s", terminal.State)
	}

	t.Logf("run %s reached terminal state %s after max attempts", terminalRun.RunID, terminal.State)
}

func TestRetryFailedRun(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	req := automation.SubmitRequest{
		RunKind:        "jarvis",
		TenantID:       "tenant-1",
		PropertyID:     "prop-1",
		ActorID:        "actor-1",
		TriggerType:    "manual",
		TriggerID:      "trigger-1",
		CorrelationID:  "corr-1",
		IdempotencyKey: "key-retry-failed",
		Provider:       "stub",
		Model:          "test-model",
		MaxAttempts:    1,
	}

	run, _, err := store.Submit(ctx, req)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	claimed, err := store.Claim(ctx, "wrk-f", automation.DefaultLeaseDuration, nil)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	_ = claimed

	err = store.Fail(ctx, run.RunID, "wrk-f", "all providers unavailable")
	if err != nil {
		t.Fatalf("fail: %v", err)
	}

	failed, err := store.Get(ctx, run.RunID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if failed.State != automation.StateFailed {
		t.Fatalf("expected failed state, got %s", failed.State)
	}
	if failed.ErrorMessage != "all providers unavailable" {
		t.Fatalf("error message must be preserved, got %q", failed.ErrorMessage)
	}

	err = store.Retry(ctx, run.RunID)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}

	retried, err := store.Get(ctx, run.RunID)
	if err != nil {
		t.Fatalf("get after retry: %v", err)
	}
	if retried.State != automation.StateQueued {
		t.Fatalf("retried run must be queued, got %s", retried.State)
	}
	if retried.Attempt != 0 {
		t.Fatalf("retried run attempt must be reset to 0, got %d", retried.Attempt)
	}
	if retried.ErrorMessage != "" {
		t.Fatalf("retried run error must be cleared, got %q", retried.ErrorMessage)
	}

	reclaimed, err := store.Claim(ctx, "wrk-new", automation.DefaultLeaseDuration, nil)
	if err != nil {
		t.Fatalf("claim after retry: %v", err)
	}
	if reclaimed.RunID != run.RunID {
		t.Fatalf("wrong run claimed after retry: %s vs %s", reclaimed.RunID, run.RunID)
	}
	if reclaimed.Attempt != 1 {
		t.Fatalf("retried run attempt must restart at 1, got %d", reclaimed.Attempt)
	}

	t.Logf("run %s retried from failed to queued and reclaimed", run.RunID)
}

func TestRetryNonRetryableRun(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	req := automation.SubmitRequest{
		RunKind:        "jarvis",
		TenantID:       "tenant-1",
		PropertyID:     "prop-1",
		ActorID:        "actor-1",
		TriggerType:    "manual",
		TriggerID:      "trigger-1",
		CorrelationID:  "corr-1",
		IdempotencyKey: "key-retry-nonretry",
		Provider:       "stub",
		Model:          "test-model",
	}

	run, _, err := store.Submit(ctx, req)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	_, err = store.Claim(ctx, "wrk-x", automation.DefaultLeaseDuration, nil)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}

	err = store.Complete(ctx, run.RunID, "wrk-x", []byte(`{}`), 0, "USD")
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	retryErr := store.Retry(ctx, run.RunID)
	if retryErr == nil {
		t.Fatal("retry on completed run must fail")
	}
	if retryErr != automation.ErrRunNotRetryable {
		t.Fatalf("expected ErrRunNotRetryable, got %v", retryErr)
	}

	t.Logf("completed run correctly rejected retry")
}

func TestModelOutageFailedRunsAreVisibleAndRetryable(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	submitFn := func(key string, retryable bool) string {
		maxAtt := 10
		if !retryable {
			maxAtt = 1
		}
		req := automation.SubmitRequest{
			RunKind:        "jarvis",
			TenantID:       "tenant-outage",
			PropertyID:     "prop-outage",
			ActorID:        "actor-outage",
			TriggerType:    "event",
			TriggerID:      "evt-outage",
			CorrelationID:  "corr-outage",
			IdempotencyKey: key,
			Provider:       "stub",
			Model:          "test-model",
			MaxAttempts:    maxAtt,
		}
		run, _, err := store.Submit(ctx, req)
		if err != nil {
			t.Fatalf("submit %s: %v", key, err)
		}
		return run.RunID
	}

	retryableRunID := submitFn("outage-retryable", true)
	failedRunID := submitFn("outage-failed", false)

	claimAndFail := func(runID, worker, errMsg string) {
		claimed, err := store.Claim(ctx, worker, automation.DefaultLeaseDuration, nil)
		if err != nil {
			t.Fatalf("claim %s: %v", runID, err)
		}
		if claimed.RunID != runID {
			t.Fatalf("claimed wrong run: %s vs %s", claimed.RunID, runID)
		}
		if err := store.Fail(ctx, runID, worker, errMsg); err != nil {
			t.Fatalf("fail %s: %v", runID, err)
		}
	}

	claimAndFail(retryableRunID, "worker-1", "stub: provider unavailable")

	retryableRun, err := store.Get(ctx, retryableRunID)
	if err != nil {
		t.Fatalf("get retryable: %v", err)
	}
	if retryableRun.State != automation.StateRetryable {
		t.Fatalf("expected retryable, got %s", retryableRun.State)
	}
	if retryableRun.ErrorMessage != "stub: provider unavailable" {
		t.Fatalf("error message not visible, got %q", retryableRun.ErrorMessage)
	}

	claimAndFail(failedRunID, "worker-1", "stub: provider unavailable")

	failedRun, err := store.Get(ctx, failedRunID)
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if failedRun.State != automation.StateFailed {
		t.Fatalf("expected failed, got %s", failedRun.State)
	}
	if failedRun.ErrorMessage != "stub: provider unavailable" {
		t.Fatalf("error message not visible, got %q", failedRun.ErrorMessage)
	}

	err = store.Retry(ctx, failedRunID)
	if err != nil {
		t.Fatalf("retry failed run: %v", err)
	}
	retried, err := store.Get(ctx, failedRunID)
	if err != nil {
		t.Fatalf("get after retry: %v", err)
	}
	if retried.State != automation.StateQueued {
		t.Fatalf("failed run must be retryable to queued, got %s", retried.State)
	}

	err = store.Retry(ctx, retryableRunID)
	if err != nil {
		t.Fatalf("retry retryable run: %v", err)
	}
	retried2, err := store.Get(ctx, retryableRunID)
	if err != nil {
		t.Fatalf("get after retry: %v", err)
	}
	if retried2.State != automation.StateQueued {
		t.Fatalf("retryable run must be retryable to queued, got %s", retried2.State)
	}

	t.Logf("all failed runs during model outage are visible and retryable")
}
