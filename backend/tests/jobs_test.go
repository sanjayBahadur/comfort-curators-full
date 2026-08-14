package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"comfort-curators-backend/internal/platform/app"
	"comfort-curators-backend/internal/platform/config"
	"comfort-curators-backend/internal/platform/database"
	"comfort-curators-backend/internal/platform/jobs"
)

func jobsPostgresAvailable() bool {
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

func jobsDBConfig() (config.Config, *database.DB, bool) {
	if !jobsPostgresAvailable() {
		return config.Config{}, nil, false
	}

	host := os.Getenv("CC_DB_HOST")
	if host == "" {
		host = "localhost"
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

	cfg := config.Config{
		DBHost: host,
		DBPort: 5432,
		DBUser: user,
		DBPass: pass,
		DBName: name,
		DBSSL:  "disable",
	}

	db, err := database.Connect(context.Background(), cfg)
	if err != nil {
		return cfg, nil, false
	}

	if _, err := db.Pool.Exec(context.Background(), `SELECT 1`); err != nil {
		db.Close()
		return cfg, nil, false
	}

	return cfg, db, true
}

func ensureJobsTables(ctx context.Context, db *database.DB) error {
	if err := database.RunMigrations(ctx, db); err != nil {
		return err
	}
	return nil
}

func cleanupJobsTable(ctx context.Context, db *database.DB) {
	db.Pool.Exec(ctx, `DELETE FROM jobs`)
}

func TestJobsTwoWorkersCannotClaimOneLease(t *testing.T) {
	_, db, ok := jobsDBConfig()
	if !ok {
		t.Skip("PostgreSQL not available")
	}
	defer db.Close()

	ctx := context.Background()
	if err := ensureJobsTables(ctx, db); err != nil {
		t.Fatalf("ensure tables: %v", err)
	}
	defer cleanupJobsTable(ctx, db)

	store := jobs.NewJobStore(db.Pool)

	payload := json.RawMessage(`{"task":"concurrent-test"}`)
	req := jobs.EnqueueRequest{JobType: "test.concurrent", Payload: payload}
	res, err := store.Enqueue(ctx, req)
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	jobID := res.Job.ID

	var mu sync.Mutex
	var winners []string
	var wg sync.WaitGroup

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(workerNum int) {
			defer wg.Done()

			workerID := fmt.Sprintf("worker-%d-concurrent", workerNum)
			claimed, err := store.Claim(ctx, workerID, 10*time.Second)
			if err == nil {
				mu.Lock()
				winners = append(winners, claimed.ID)
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()

	if len(winners) != 1 {
		t.Errorf("expected exactly 1 worker to claim, got %d winners", len(winners))
	}
	for _, id := range winners {
		if id != jobID {
			t.Errorf("claimed unexpected job %s, expected %s", id, jobID)
		}
	}
}

func TestJobsExpiredLeaseIsRecovered(t *testing.T) {
	_, db, ok := jobsDBConfig()
	if !ok {
		t.Skip("PostgreSQL not available")
	}
	defer db.Close()

	ctx := context.Background()
	if err := ensureJobsTables(ctx, db); err != nil {
		t.Fatalf("ensure tables: %v", err)
	}
	defer cleanupJobsTable(ctx, db)

	store := jobs.NewJobStore(db.Pool)

	payload := json.RawMessage(`{"task":"expire-test"}`)
	req := jobs.EnqueueRequest{JobType: "test.expire", Payload: payload}
	res, err := store.Enqueue(ctx, req)
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	jobID := res.Job.ID

	// Claim with a very short lease
	firstWorker := "worker-alpha"
	claimed, err := store.Claim(ctx, firstWorker, 1*time.Second)
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if claimed.ID != jobID {
		t.Fatalf("claimed wrong job: %s", claimed.ID)
	}
	if *claimed.LeaseOwner != firstWorker {
		t.Errorf("lease owner: got %s, want %s", *claimed.LeaseOwner, firstWorker)
	}

	// Wait for lease to expire
	time.Sleep(1500 * time.Millisecond)

	// Second worker tries to claim - should recover the expired lease
	secondWorker := "worker-beta"
	recovered, err := store.Claim(ctx, secondWorker, 10*time.Second)
	if err != nil {
		t.Fatalf("recovery claim: %v", err)
	}
	if recovered.ID != jobID {
		t.Fatalf("recovered wrong job: %s, expected %s", recovered.ID, jobID)
	}
	if *recovered.LeaseOwner != secondWorker {
		t.Errorf("recovered lease owner: got %s, want %s", *recovered.LeaseOwner, secondWorker)
	}

	// First worker should no longer be the owner
	err = store.Heartbeat(ctx, jobID, firstWorker, 10*time.Second)
	if err != jobs.ErrNotOwner {
		t.Errorf("expected ErrNotOwner for first worker heartbeat, got: %v", err)
	}
}

func TestJobsDuplicateExecutionDoesNotDuplicateEffects(t *testing.T) {
	_, db, ok := jobsDBConfig()
	if !ok {
		t.Skip("PostgreSQL not available")
	}
	defer db.Close()

	ctx := context.Background()
	if err := ensureJobsTables(ctx, db); err != nil {
		t.Fatalf("ensure tables: %v", err)
	}
	defer cleanupJobsTable(ctx, db)

	store := jobs.NewJobStore(db.Pool)

	idemKey := fmt.Sprintf("idem-key-%d", time.Now().UnixNano())
	payload := json.RawMessage(`{"task":"dedup-test"}`)

	req1 := jobs.EnqueueRequest{
		JobType:        "test.dedup",
		Payload:        payload,
		IdempotencyKey: &idemKey,
	}
	res1, err := store.Enqueue(ctx, req1)
	if err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	if res1.Duplicate {
		t.Error("first enqueue should not be a duplicate")
	}

	// Same key, same payload - should be duplicate
	res2, err := store.Enqueue(ctx, req1)
	if err != nil {
		t.Fatalf("second enqueue: %v", err)
	}
	if !res2.Duplicate {
		t.Error("second enqueue with same key+payload should be a duplicate")
	}
	if res2.Job.ID != res1.Job.ID {
		t.Errorf("duplicate job ID mismatch: got %s, want %s", res2.Job.ID, res1.Job.ID)
	}

	// Verify only one row exists
	var count int
	err = db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM jobs WHERE idempotency_key = $1`, idemKey).Scan(&count)
	if err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 job row, got %d", count)
	}

	// Different payload with same key should conflict
	diffPayload := json.RawMessage(`{"task":"dedup-test-different"}`)
	req3 := jobs.EnqueueRequest{
		JobType:        "test.dedup",
		Payload:        diffPayload,
		IdempotencyKey: &idemKey,
	}
	_, err = store.Enqueue(ctx, req3)
	if err != jobs.ErrIdempotencyKeyConflict {
		t.Errorf("expected ErrIdempotencyKeyConflict, got: %v", err)
	}
}

func TestJobsTerminalFailureIsVisible(t *testing.T) {
	_, db, ok := jobsDBConfig()
	if !ok {
		t.Skip("PostgreSQL not available")
	}
	defer db.Close()

	ctx := context.Background()
	if err := ensureJobsTables(ctx, db); err != nil {
		t.Fatalf("ensure tables: %v", err)
	}
	defer cleanupJobsTable(ctx, db)

	store := jobs.NewJobStore(db.Pool)

	payload := json.RawMessage(`{"task":"terminal-fail-test"}`)
	req := jobs.EnqueueRequest{
		JobType:     "test.terminal",
		Payload:     payload,
		MaxAttempts: 3,
	}
	res, err := store.Enqueue(ctx, req)
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	jobID := res.Job.ID
	workerID := "worker-terminal"

	for i := 1; i <= 3; i++ {
		claimed, err := store.Claim(ctx, workerID, 10*time.Second)
		if err != nil {
			t.Fatalf("claim attempt %d: %v", i, err)
		}
		if claimed.ID != jobID {
			t.Fatalf("wrong job claimed on attempt %d", i)
		}

		failMsg := fmt.Sprintf("failure on attempt %d", i)
		err = store.Fail(ctx, jobID, workerID, failMsg)
		if i < 3 {
			if err != nil {
				t.Fatalf("fail attempt %d: %v", i, err)
			}

			// Wait for next_retry_at to be ready
			time.Sleep(100 * time.Millisecond)
		} else {
			// Final attempt should succeed in marking as dead
			if err != nil {
				t.Fatalf("fail attempt %d: %v", i, err)
			}
		}
	}

	// Verify job is in dead letter state
	var status string
	var errMsg string
	err = db.Pool.QueryRow(ctx,
		`SELECT status, error_message FROM jobs WHERE id = $1`, jobID,
	).Scan(&status, &errMsg)
	if err != nil {
		t.Fatalf("query job: %v", err)
	}
	if status != jobs.StatusDead {
		t.Errorf("expected status=%s, got %s", jobs.StatusDead, status)
	}
	if errMsg == "" {
		t.Error("expected error_message to be set on dead letter job")
	}

	deadJobs, err := store.GetDeadLetterJobs(ctx)
	if err != nil {
		t.Fatalf("get dead letter jobs: %v", err)
	}
	found := false
	for _, dj := range deadJobs {
		if dj.ID == jobID {
			found = true
			break
		}
	}
	if !found {
		t.Error("dead letter job not found in GetDeadLetterJobs result")
	}
}

func TestJobsRecoverExpiredLeasesBulk(t *testing.T) {
	_, db, ok := jobsDBConfig()
	if !ok {
		t.Skip("PostgreSQL not available")
	}
	defer db.Close()

	ctx := context.Background()
	if err := ensureJobsTables(ctx, db); err != nil {
		t.Fatalf("ensure tables: %v", err)
	}
	defer cleanupJobsTable(ctx, db)

	store := jobs.NewJobStore(db.Pool)

	// Enqueue two jobs
	for i := 0; i < 2; i++ {
		payload := json.RawMessage(fmt.Sprintf(`{"task":"bulk-expire-%d"}`, i))
		req := jobs.EnqueueRequest{JobType: "test.bulk", Payload: payload}
		_, err := store.Enqueue(ctx, req)
		if err != nil {
			t.Fatalf("enqueue job %d: %v", i, err)
		}
	}

	// Claim both with very short leases
	worker := "worker-bulk"
	for i := 0; i < 2; i++ {
		_, err := store.Claim(ctx, worker, 1*time.Second)
		if err != nil {
			t.Fatalf("claim %d: %v", i, err)
		}
	}

	// Wait for leases to expire
	time.Sleep(1500 * time.Millisecond)

	// Recover expired leases
	recovered, err := store.RecoverExpiredLeases(ctx)
	if err != nil {
		t.Fatalf("recover expired leases: %v", err)
	}
	if recovered != 2 {
		t.Errorf("expected 2 recovered leases, got %d", recovered)
	}

	// Verify jobs are claimable again after recovery
	_, err = store.Claim(ctx, "worker-recovery", 10*time.Second)
	if err != nil {
		t.Fatalf("claim after recovery: %v", err)
	}
	_, err = store.Claim(ctx, "worker-recovery-2", 10*time.Second)
	if err != nil {
		t.Fatalf("second claim after recovery: %v", err)
	}
}

func TestJobsCompleteAndHeartbeat(t *testing.T) {
	_, db, ok := jobsDBConfig()
	if !ok {
		t.Skip("PostgreSQL not available")
	}
	defer db.Close()

	ctx := context.Background()
	if err := ensureJobsTables(ctx, db); err != nil {
		t.Fatalf("ensure tables: %v", err)
	}
	defer cleanupJobsTable(ctx, db)

	store := jobs.NewJobStore(db.Pool)

	payload := json.RawMessage(`{"task":"complete-test"}`)
	req := jobs.EnqueueRequest{JobType: "test.complete", Payload: payload}
	res, err := store.Enqueue(ctx, req)
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	jobID := res.Job.ID
	workerID := "worker-complete"

	claimed, err := store.Claim(ctx, workerID, 2*time.Second)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed.ID != jobID {
		t.Fatalf("claimed wrong job")
	}

	// Heartbeat should extend lease
	err = store.Heartbeat(ctx, jobID, workerID, 10*time.Second)
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}

	// Transition to running
	err = store.StartRunning(ctx, jobID, workerID)
	if err != nil {
		t.Fatalf("start running: %v", err)
	}

	result := json.RawMessage(`{"outcome":"success","processed":true}`)
	err = store.Complete(ctx, jobID, workerID, result)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	var status string
	var storedResult json.RawMessage
	err = db.Pool.QueryRow(ctx,
		`SELECT status, result FROM jobs WHERE id = $1`, jobID,
	).Scan(&status, &storedResult)
	if err != nil {
		t.Fatalf("query job: %v", err)
	}
	if status != jobs.StatusCompleted {
		t.Errorf("expected status=%s, got %s", jobs.StatusCompleted, status)
	}
	if string(storedResult) != string(result) {
		t.Errorf("result mismatch: got %s, want %s", storedResult, result)
	}
}

func TestJobsEnqueueIdempotencyCompletedReturnsResult(t *testing.T) {
	_, db, ok := jobsDBConfig()
	if !ok {
		t.Skip("PostgreSQL not available")
	}
	defer db.Close()

	ctx := context.Background()
	if err := ensureJobsTables(ctx, db); err != nil {
		t.Fatalf("ensure tables: %v", err)
	}
	defer cleanupJobsTable(ctx, db)

	store := jobs.NewJobStore(db.Pool)

	idemKey := fmt.Sprintf("completed-key-%d", time.Now().UnixNano())
	payload := json.RawMessage(`{"task":"completed-idem-test"}`)

	req1 := jobs.EnqueueRequest{
		JobType:        "test.completed",
		Payload:        payload,
		IdempotencyKey: &idemKey,
	}
	res1, err := store.Enqueue(ctx, req1)
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	jobID := res1.Job.ID
	workerID := "worker-completed"

	claimed, err := store.Claim(ctx, workerID, 10*time.Second)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed.ID != jobID {
		t.Fatalf("wrong job claimed")
	}

	result := json.RawMessage(`{"outcome":"done","id":"12345"}`)
	err = store.Complete(ctx, jobID, workerID, result)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	// Re-enqueue with same idempotency key should return completed result
	res2, err := store.Enqueue(ctx, req1)
	if err != nil {
		t.Fatalf("re-enqueue after complete: %v", err)
	}
	if !res2.Duplicate {
		t.Error("re-enqueue of completed job should be marked duplicate")
	}
	if res2.Job.Status != jobs.StatusCompleted {
		t.Errorf("expected status=%s, got %s", jobs.StatusCompleted, res2.Job.Status)
	}
	if string(res2.Job.Result) != string(result) {
		t.Errorf("cached result mismatch: got %s, want %s", res2.Job.Result, result)
	}
}

func TestJobsRegistryDispatch(t *testing.T) {
	registry := jobs.NewRegistry()

	called := false
	expectedResult := json.RawMessage(`{"ok":true}`)
	registry.Register("test.handler", func(ctx context.Context, job *jobs.Job) (json.RawMessage, error) {
		called = true
		if job.Type != "test.handler" {
			t.Errorf("unexpected job type in handler: %s", job.Type)
		}
		return expectedResult, nil
	})

	job := &jobs.Job{Type: "test.handler", Status: jobs.StatusRunning}
	result, err := registry.Dispatch(context.Background(), job)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if !called {
		t.Error("handler was not called")
	}
	if string(result) != string(expectedResult) {
		t.Errorf("result mismatch: got %s, want %s", result, expectedResult)
	}
}

func TestJobsRegistryUnknownType(t *testing.T) {
	registry := jobs.NewRegistry()

	job := &jobs.Job{Type: "test.unknown", Status: jobs.StatusRunning}
	_, err := registry.Dispatch(context.Background(), job)
	if err == nil {
		t.Fatal("expected error for unknown job type")
	}
}

func TestJobsCancel(t *testing.T) {
	_, db, ok := jobsDBConfig()
	if !ok {
		t.Skip("PostgreSQL not available")
	}
	defer db.Close()

	ctx := context.Background()
	if err := ensureJobsTables(ctx, db); err != nil {
		t.Fatalf("ensure tables: %v", err)
	}
	defer cleanupJobsTable(ctx, db)

	store := jobs.NewJobStore(db.Pool)

	payload := json.RawMessage(`{"task":"cancel-test"}`)
	req := jobs.EnqueueRequest{JobType: "test.cancel", Payload: payload}
	res, err := store.Enqueue(ctx, req)
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	jobID := res.Job.ID

	err = store.Cancel(ctx, jobID)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}

	var status string
	var errMsg *string
	err = db.Pool.QueryRow(ctx,
		`SELECT status, error_message FROM jobs WHERE id = $1`, jobID,
	).Scan(&status, &errMsg)
	if err != nil {
		t.Fatalf("query cancelled job: %v", err)
	}
	if status != jobs.StatusDead {
		t.Errorf("expected status=%s, got %s", jobs.StatusDead, status)
	}
	if errMsg == nil || *errMsg != "cancelled" {
		t.Errorf("expected error_message='cancelled', got: %v", errMsg)
	}

	deadJobs, err := store.GetDeadLetterJobs(ctx)
	if err != nil {
		t.Fatalf("get dead letter jobs: %v", err)
	}
	found := false
	for _, dj := range deadJobs {
		if dj.ID == jobID {
			found = true
			break
		}
	}
	if !found {
		t.Error("cancelled job not visible in dead letter list")
	}
}

func TestJobsWorkerLoopEndToEnd(t *testing.T) {
	_, db, ok := jobsDBConfig()
	if !ok {
		t.Skip("PostgreSQL not available")
	}
	defer db.Close()

	ctx := context.Background()
	if err := ensureJobsTables(ctx, db); err != nil {
		t.Fatalf("ensure tables: %v", err)
	}
	defer cleanupJobsTable(ctx, db)

	store := jobs.NewJobStore(db.Pool)
	registry := jobs.NewRegistry()
	workerID := fmt.Sprintf("worker-e2e-%d", time.Now().UnixNano())

	processed := make(chan struct{})
	registry.Register("test.e2e", func(ctx context.Context, job *jobs.Job) (json.RawMessage, error) {
		close(processed)
		return json.RawMessage(`{"status":"done"}`), nil
	})

	payload := json.RawMessage(`{"task":"e2e-test"}`)
	req := jobs.EnqueueRequest{JobType: "test.e2e", Payload: payload}
	res, err := store.Enqueue(ctx, req)
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	jobID := res.Job.ID

	claimed, err := store.Claim(ctx, workerID, 10*time.Second)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed.ID != jobID {
		t.Fatalf("wrong job claimed: %s", claimed.ID)
	}

	err = store.StartRunning(ctx, jobID, workerID)
	if err != nil {
		t.Fatalf("start running: %v", err)
	}

	result, dispatchErr := registry.Dispatch(ctx, claimed)
	if dispatchErr != nil {
		t.Fatalf("dispatch: %v", dispatchErr)
	}

	select {
	case <-processed:
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not execute within timeout")
	}

	err = store.Complete(ctx, jobID, workerID, result)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	var status string
	err = db.Pool.QueryRow(ctx,
		`SELECT status FROM jobs WHERE id = $1`, jobID,
	).Scan(&status)
	if err != nil {
		t.Fatalf("query completed job: %v", err)
	}
	if status != jobs.StatusCompleted {
		t.Errorf("expected status=%s, got %s", jobs.StatusCompleted, status)
	}
}

func TestJobsDeadLetterEndpoint(t *testing.T) {
	t.Setenv("CC_DB_USER", "testuser")
	t.Setenv("CC_DB_PASS", "testpass")
	t.Setenv("CC_DB_NAME", "testdb")
	t.Setenv("CC_SKIP_DB", "true")
	t.Setenv("CC_HTTP_PORT", "18090")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		app.RunAPI(ctx)
	}()

	baseURL := "http://127.0.0.1:18090"
	if err := waitForServer(baseURL+"/health/live", 5*time.Second); err != nil {
		t.Fatalf("server did not become ready: %v", err)
	}

	resp, err := http.Get(baseURL + "/jobs/dead-letter")
	if err != nil {
		t.Fatalf("dead-letter request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 when DB is skipped (route not registered), got %d", resp.StatusCode)
	}
}

func TestJobsDeadLetterEndpointWithData(t *testing.T) {
	_, db, ok := jobsDBConfig()
	if !ok {
		t.Skip("PostgreSQL not available")
	}
	defer db.Close()

	ctx := context.Background()
	if err := ensureJobsTables(ctx, db); err != nil {
		t.Fatalf("ensure tables: %v", err)
	}
	defer cleanupJobsTable(ctx, db)

	store := jobs.NewJobStore(db.Pool)

	payload := json.RawMessage(`{"task":"dl-endpoint-test"}`)
	req := jobs.EnqueueRequest{
		JobType:     "test.dl-endpoint",
		Payload:     payload,
		MaxAttempts: 1,
	}
	res, err := store.Enqueue(ctx, req)
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	jobID := res.Job.ID
	workerID := "worker-dl-endpoint"

	claimed, err := store.Claim(ctx, workerID, 10*time.Second)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed.ID != jobID {
		t.Fatalf("wrong job claimed")
	}

	err = store.Fail(ctx, jobID, workerID, "test terminal failure")
	if err != nil {
		t.Fatalf("fail: %v", err)
	}

	deadJobs, err := store.GetDeadLetterJobs(ctx)
	if err != nil {
		t.Fatalf("get dead letter jobs: %v", err)
	}

	found := false
	for _, dj := range deadJobs {
		if dj.ID == jobID {
			found = true
			if dj.ErrorMessage == nil || *dj.ErrorMessage != "test terminal failure" {
				t.Errorf("expected error_message='test terminal failure', got: %v", dj.ErrorMessage)
			}
			break
		}
	}
	if !found {
		t.Error("dead letter job not found in GetDeadLetterJobs result")
	}

	if len(deadJobs) != 1 {
		t.Errorf("expected 1 dead letter job, got %d", len(deadJobs))
	}
}
