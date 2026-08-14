package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"comfort-curators-backend/internal/platform/config"
	"comfort-curators-backend/internal/platform/database"
	"comfort-curators-backend/internal/platform/durability"

	"github.com/jackc/pgx/v5"
)

func durabilityPostgresAvailable() bool {
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

func durabilityDBConfig() (config.Config, *database.DB, bool) {
	if !durabilityPostgresAvailable() {
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

	_, err = db.Pool.Exec(context.Background(), `SELECT 1`)
	if err != nil {
		db.Close()
		return cfg, nil, false
	}

	return cfg, db, true
}

func ensureDurabilityTables(ctx context.Context, db *database.DB) error {
	if err := database.RunMigrations(ctx, db); err != nil {
		return err
	}
	return nil
}

func TestCCFND001IdempotencyReplay(t *testing.T) {
	_, db, ok := durabilityDBConfig()
	if !ok {
		t.Skip("PostgreSQL not available")
	}
	defer db.Close()

	ctx := context.Background()

	if err := ensureDurabilityTables(ctx, db); err != nil {
		t.Fatalf("ensure tables: %v", err)
	}

	_, err := db.Pool.Exec(ctx,
		`CREATE TABLE IF NOT EXISTS test_domain (id UUID PRIMARY KEY DEFAULT gen_random_uuid(), data TEXT NOT NULL)`)
	if err != nil {
		t.Fatalf("create test domain table: %v", err)
	}
	defer func() {
		db.Pool.Exec(ctx, `DROP TABLE IF EXISTS test_domain`)
		db.Pool.Exec(ctx, `DELETE FROM idempotency_records`)
	}()

	idemStore := durability.NewIdempotencyStore(db.Pool)

	key := fmt.Sprintf("test-idem-key-%d", time.Now().UnixNano())
	operationClass := "test.create_resource"

	body1 := json.RawMessage(`{"data": "hello-world"}`)

	var createdID string

	result, err := idemStore.Process(ctx, key, operationClass, body1, func(ctx context.Context, tx pgx.Tx) (json.RawMessage, error) {
		var id string
		err := tx.QueryRow(ctx,
			`INSERT INTO test_domain (data) VALUES ($1) RETURNING id::text`,
			"hello-world",
		).Scan(&id)
		if err != nil {
			return nil, fmt.Errorf("insert domain: %w", err)
		}
		createdID = id
		return json.RawMessage(fmt.Sprintf(`{"id":"%s"}`, id)), nil
	})
	if err != nil {
		t.Fatalf("first process: %v", err)
	}
	if result.Replay {
		t.Error("first call should not be a replay")
	}
	if createdID == "" {
		t.Error("expected domain row to be created on first call")
	}

	var firstResult map[string]any
	if err := json.Unmarshal(result.ResultRef, &firstResult); err != nil {
		t.Fatalf("unmarshal first result: %v", err)
	}
	if firstResult["id"] != createdID {
		t.Errorf("result id mismatch: got %v, want %v", firstResult["id"], createdID)
	}

	result2, err := idemStore.Process(ctx, key, operationClass, body1, func(ctx context.Context, tx pgx.Tx) (json.RawMessage, error) {
		return nil, fmt.Errorf("handler should not be called on replay")
	})
	if err != nil {
		t.Fatalf("second process (same key+body): %v", err)
	}
	if !result2.Replay {
		t.Error("second call with same key+body should be a replay")
	}
	if string(result2.ResultRef) != string(result.ResultRef) {
		t.Errorf("replay result mismatch: got %s, want %s", result2.ResultRef, result.ResultRef)
	}

	var domainCount int
	err = db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM test_domain`).Scan(&domainCount)
	if err != nil {
		t.Fatalf("count domain rows: %v", err)
	}
	if domainCount != 1 {
		t.Errorf("expected one domain effect, got %d", domainCount)
	}

	body2 := json.RawMessage(`{"data": "different-body"}`)
	_, err = idemStore.Process(ctx, key, operationClass, body2, func(ctx context.Context, tx pgx.Tx) (json.RawMessage, error) {
		return nil, fmt.Errorf("handler should not be called on conflict")
	})
	if err == nil {
		t.Fatal("expected conflict error for same key with different body")
	}
	if err != durability.ErrKeyConflict {
		t.Errorf("expected ErrKeyConflict, got: %v", err)
	}

	domainCount = 0
	err = db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM test_domain`).Scan(&domainCount)
	if err != nil {
		t.Fatalf("count domain rows after conflict: %v", err)
	}
	if domainCount != 1 {
		t.Errorf("expected one domain effect after conflict, got %d", domainCount)
	}
}

func TestCCFND001OutboxCommitAtomicity(t *testing.T) {
	_, db, ok := durabilityDBConfig()
	if !ok {
		t.Skip("PostgreSQL not available")
	}
	defer db.Close()

	ctx := context.Background()

	if err := ensureDurabilityTables(ctx, db); err != nil {
		t.Fatalf("ensure tables: %v", err)
	}

	_, err := db.Pool.Exec(ctx,
		`CREATE TABLE IF NOT EXISTS test_domain_atomic (id UUID PRIMARY KEY DEFAULT gen_random_uuid(), correlation_id UUID NOT NULL, data TEXT NOT NULL)`)
	if err != nil {
		t.Fatalf("create test domain table: %v", err)
	}
	defer func() {
		db.Pool.Exec(ctx, `DROP TABLE IF EXISTS test_domain_atomic`)
		db.Pool.Exec(ctx, `DELETE FROM outbox_events`)
	}()

	outboxStore := durability.NewOutboxStore(db.Pool)

	err = database.WithTx(ctx, db.Pool, func(ctx context.Context, tx pgx.Tx) error {
		correlationID := "00000000-0000-0000-0000-000000000001"
		_, err := tx.Exec(ctx,
			`INSERT INTO test_domain_atomic (correlation_id, data) VALUES ($1, $2)`,
			correlationID, "force-failure-test",
		)
		if err != nil {
			return err
		}

		evt := durability.EventEnvelope{
			EventID:          "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeee1",
			EventName:        "test.failed_transaction",
			EventVersion:     "1",
			OccurredAt:       time.Now().UTC(),
			TenantID:         "00000000-0000-0000-0000-0000000000aa",
			ActorType:        "test",
			ActorID:          "00000000-0000-0000-0000-000000000099",
			CorrelationID:    correlationID,
			CausationID:      "00000000-0000-0000-0000-0000000000cc",
			AggregateType:    "test",
			AggregateID:      "00000000-0000-0000-0000-0000000000aa",
			AggregateVersion: 1,
			Payload:          json.RawMessage(`{"attempt":"failed"}`),
		}
		if err := outboxStore.Append(ctx, tx, evt); err != nil {
			return err
		}

		return fmt.Errorf("forced transaction failure")
	})
	if err == nil {
		t.Fatal("expected forced failure error, got nil")
	}
	t.Logf("forced transaction failure: %v", err)

	var domainCount int
	err = db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM test_domain_atomic`).Scan(&domainCount)
	if err != nil {
		t.Fatalf("count domain rows after failure: %v", err)
	}
	if domainCount != 0 {
		t.Errorf("expected 0 domain rows after forced failure, got %d", domainCount)
	}

	var outboxCount int
	err = db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM outbox_events`).Scan(&outboxCount)
	if err != nil {
		t.Fatalf("count outbox rows after failure: %v", err)
	}
	if outboxCount != 0 {
		t.Errorf("expected 0 outbox rows after forced failure, got %d", outboxCount)
	}

	correlationID := "00000000-0000-0000-0000-000000000002"
	err = database.WithTx(ctx, db.Pool, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`INSERT INTO test_domain_atomic (correlation_id, data) VALUES ($1, $2)`,
			correlationID, "success-test",
		)
		if err != nil {
			return err
		}

		evt := durability.EventEnvelope{
			EventID:          "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeee2",
			EventName:        "test.successful_transaction",
			EventVersion:     "1",
			OccurredAt:       time.Now().UTC(),
			TenantID:         "00000000-0000-0000-0000-0000000000aa",
			ActorType:        "test",
			ActorID:          "00000000-0000-0000-0000-000000000099",
			CorrelationID:    correlationID,
			CausationID:      "00000000-0000-0000-0000-0000000000cc",
			AggregateType:    "test",
			AggregateID:      "00000000-0000-0000-0000-0000000000aa",
			AggregateVersion: 1,
			Payload:          json.RawMessage(`{"attempt":"success"}`),
		}
		return outboxStore.Append(ctx, tx, evt)
	})
	if err != nil {
		t.Fatalf("successful transaction: %v", err)
	}

	domainCount = 0
	err = db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM test_domain_atomic`).Scan(&domainCount)
	if err != nil {
		t.Fatalf("count domain rows after success: %v", err)
	}
	if domainCount != 1 {
		t.Errorf("expected 1 domain row after success, got %d", domainCount)
	}

	var domainCorrelation string
	err = db.Pool.QueryRow(ctx,
		`SELECT correlation_id::text FROM test_domain_atomic LIMIT 1`,
	).Scan(&domainCorrelation)
	if err != nil {
		t.Fatalf("read domain correlation: %v", err)
	}
	if domainCorrelation != correlationID {
		t.Errorf("domain correlation mismatch: got %s, want %s", domainCorrelation, correlationID)
	}

	err = db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM outbox_events`).Scan(&outboxCount)
	if err != nil {
		t.Fatalf("count outbox rows after success: %v", err)
	}
	if outboxCount != 1 {
		t.Errorf("expected 1 outbox row after success, got %d", outboxCount)
	}

	var outboxCorrelation string
	err = db.Pool.QueryRow(ctx,
		`SELECT correlation_id::text FROM outbox_events LIMIT 1`,
	).Scan(&outboxCorrelation)
	if err != nil {
		t.Fatalf("read outbox correlation: %v", err)
	}
	if outboxCorrelation != correlationID {
		t.Errorf("outbox correlation mismatch: got %s, want %s", outboxCorrelation, correlationID)
	}
}
