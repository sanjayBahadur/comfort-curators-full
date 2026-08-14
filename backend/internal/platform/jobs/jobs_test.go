package jobs

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"testing"
	"time"

	"comfort-curators-backend/internal/platform/config"
	"comfort-curators-backend/internal/platform/database"
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

func testJobStore(t *testing.T) (*JobStore, *database.DB) {
	t.Helper()
	if !postgresAvailable() {
		t.Skip("PostgreSQL not available")
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
		t.Skipf("PostgreSQL not available: %v", err)
	}

	ctx := context.Background()
	if err := database.RunMigrations(ctx, db); err != nil {
		db.Close()
		t.Fatalf("run migrations: %v", err)
	}

	if _, err := db.Pool.Exec(ctx, `DELETE FROM jobs`); err != nil {
		db.Close()
		t.Fatalf("clean jobs table: %v", err)
	}

	t.Cleanup(func() {
		db.Pool.Exec(context.Background(), `DELETE FROM jobs`)
		db.Close()
	})

	return NewJobStore(db.Pool), db
}

func TestClaimSetsHeartbeatToNowNotLeaseExpiry(t *testing.T) {
	store, db := testJobStore(t)
	ctx := context.Background()

	payload := json.RawMessage(`{"task":"heartbeat-now"}`)
	res, err := store.Enqueue(ctx, EnqueueRequest{JobType: "test.heartbeat-now", Payload: payload})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	jobID := res.Job.ID
	workerID := "worker-heartbeat-now"

	leaseDuration := 30 * time.Second
	before := time.Now().UTC()
	claimed, err := store.Claim(ctx, workerID, leaseDuration)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	after := time.Now().UTC()

	if claimed.ID != jobID {
		t.Fatalf("claimed wrong job: %s", claimed.ID)
	}
	if claimed.LastHeartbeatAt == nil {
		t.Fatal("expected last_heartbeat_at to be set on claim")
	}
	if claimed.LeaseExpiresAt == nil {
		t.Fatal("expected lease_expires_at to be set on claim")
	}

	if !claimed.LastHeartbeatAt.Before(*claimed.LeaseExpiresAt) {
		t.Errorf("last_heartbeat_at %v must be strictly earlier than lease_expires_at %v",
			claimed.LastHeartbeatAt, claimed.LeaseExpiresAt)
	}
	if claimed.LastHeartbeatAt.Before(before.Add(-time.Second)) || claimed.LastHeartbeatAt.After(after.Add(time.Second)) {
		t.Errorf("last_heartbeat_at %v not close to now", claimed.LastHeartbeatAt)
	}
	wantExpiry := claimed.LastHeartbeatAt.Add(leaseDuration)
	if claimed.LeaseExpiresAt.Sub(wantExpiry) > time.Second {
		t.Errorf("lease_expires_at %v should be heartbeat + lease duration %v", claimed.LeaseExpiresAt, wantExpiry)
	}

	var storedHeartbeat, storedExpiry time.Time
	err = db.Pool.QueryRow(ctx,
		`SELECT last_heartbeat_at, lease_expires_at FROM jobs WHERE id = $1`, jobID,
	).Scan(&storedHeartbeat, &storedExpiry)
	if err != nil {
		t.Fatalf("query stored timestamps: %v", err)
	}
	if !storedHeartbeat.Before(storedExpiry) {
		t.Errorf("stored last_heartbeat_at %v must be earlier than stored lease_expires_at %v",
			storedHeartbeat, storedExpiry)
	}
	if storedHeartbeat.Before(before.Add(-time.Second)) || storedHeartbeat.After(after.Add(time.Second)) {
		t.Errorf("stored last_heartbeat_at %v not close to now", storedHeartbeat)
	}
}

func TestRecoverExpiredLeasesDeadLettersExhaustedJobs(t *testing.T) {
	store, db := testJobStore(t)
	ctx := context.Background()

	payload := json.RawMessage(`{"task":"lease-exhaust"}`)
	res, err := store.Enqueue(ctx, EnqueueRequest{
		JobType:     "test.lease-exhaust",
		Payload:     payload,
		MaxAttempts: 3,
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	jobID := res.Job.ID

	const leaseDuration = 300 * time.Millisecond
	workerID := "worker-lease-exhaust"

	for attempt := 1; attempt <= 3; attempt++ {
		claimed, err := store.Claim(ctx, workerID, leaseDuration)
		if err != nil {
			t.Fatalf("claim cycle %d: %v", attempt, err)
		}
		if claimed.ID != jobID {
			t.Fatalf("claim cycle %d: claimed wrong job %s", attempt, claimed.ID)
		}

		time.Sleep(2 * leaseDuration)

		recovered, err := store.RecoverExpiredLeases(ctx)
		if err != nil {
			t.Fatalf("recover expired leases cycle %d: %v", attempt, err)
		}
		if attempt < 3 {
			if recovered != 1 {
				t.Fatalf("cycle %d: expected 1 recovered lease, got %d", attempt, recovered)
			}
		} else {
			if recovered != 1 {
				t.Fatalf("cycle %d: expected exhausted job to be dead-lettered, got %d recovered", attempt, recovered)
			}
		}
	}

	var status string
	var errMsg *string
	err = db.Pool.QueryRow(ctx,
		`SELECT status, error_message FROM jobs WHERE id = $1`, jobID,
	).Scan(&status, &errMsg)
	if err != nil {
		t.Fatalf("query job: %v", err)
	}
	if status != StatusDead {
		t.Errorf("expected status=%s after exhausting max attempts, got %s", StatusDead, status)
	}
	if errMsg == nil || *errMsg == "" {
		t.Error("expected error_message to be set on dead-lettered job")
	}

	_, err = store.Claim(ctx, "worker-after-exhaust", 10*time.Second)
	if err != ErrNoWork {
		t.Errorf("expected ErrNoWork after exhausting max attempts, got: %v", err)
	}

	recovered, err := store.RecoverExpiredLeases(ctx)
	if err != nil {
		t.Fatalf("recover expired leases after dead-letter: %v", err)
	}
	if recovered != 0 {
		t.Errorf("expected no recoverable leases after dead-lettering, got %d", recovered)
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
		t.Error("exhausted job not visible in dead letter list")
	}
}
