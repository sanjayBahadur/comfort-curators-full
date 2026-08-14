package durability

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"comfort-curators-backend/internal/platform/config"
	"comfort-curators-backend/internal/platform/database"

	"github.com/jackc/pgx/v5"
)

type recordingSink struct {
	events []EventEnvelope
	err    error
}

func (s *recordingSink) Deliver(_ context.Context, event EventEnvelope) error {
	s.events = append(s.events, event)
	return s.err
}

func relayTestDB(t *testing.T) (*database.DB, bool) {
	t.Helper()
	cfg := config.LoadFromEnv()
	if cfg.DBUser == "" {
		cfg.DBUser = "ccuser"
	}
	if cfg.DBPass == "" {
		cfg.DBPass = "ccpass"
	}
	if cfg.DBName == "" {
		cfg.DBName = "comfort_curators"
	}
	db, err := database.Connect(context.Background(), cfg)
	if err != nil {
		return nil, false
	}
	if err := database.RunMigrations(context.Background(), db); err != nil {
		db.Close()
		t.Fatalf("run migrations: %v", err)
	}
	return db, true
}

func relayTestEvent(id, name string) EventEnvelope {
	return EventEnvelope{
		EventID:          id,
		EventName:        name,
		EventVersion:     "1",
		OccurredAt:       time.Now().UTC(),
		TenantID:         "00000000-0000-0000-0000-000000000001",
		ActorType:        "system",
		ActorID:          "00000000-0000-0000-0000-000000000002",
		CorrelationID:    "00000000-0000-0000-0000-000000000003",
		CausationID:      "00000000-0000-0000-0000-000000000004",
		AggregateType:    "test",
		AggregateID:      "00000000-0000-0000-0000-000000000005",
		AggregateVersion: 1,
		Payload:          json.RawMessage(`{"ok":true}`),
	}
}

func appendRelayTestEvent(t *testing.T, db *database.DB, event EventEnvelope) {
	t.Helper()
	store := NewOutboxStore(db.Pool)
	err := database.WithTx(context.Background(), db.Pool, func(ctx context.Context, tx pgx.Tx) error {
		return store.Append(ctx, tx, event)
	})
	if err != nil {
		t.Fatalf("append event: %v", err)
	}
}

func TestOutboxRelayDeliversOnce(t *testing.T) {
	db, ok := relayTestDB(t)
	if !ok {
		t.Skip("PostgreSQL not available")
	}
	defer db.Close()

	ctx := context.Background()
	event := relayTestEvent("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeee3", "test.relay_delivery")
	defer db.Pool.Exec(ctx, `DELETE FROM outbox_events WHERE event_id = $1`, event.EventID)
	appendRelayTestEvent(t, db, event)

	sink := &recordingSink{}
	store := NewOutboxStore(db.Pool)
	if err := store.RelayOnce(ctx, sink); err != nil {
		t.Fatalf("relay event: %v", err)
	}
	if len(sink.events) != 1 || sink.events[0].EventID != event.EventID {
		t.Fatalf("sink received %#v, want one event %s", sink.events, event.EventID)
	}
	if err := store.RelayOnce(ctx, sink); !errors.Is(err, ErrNoPendingEvents) {
		t.Fatalf("second relay error = %v, want ErrNoPendingEvents", err)
	}

	var status string
	var attempts int
	if err := db.Pool.QueryRow(ctx,
		`SELECT status, attempt FROM outbox_events WHERE event_id = $1`, event.EventID,
	).Scan(&status, &attempts); err != nil {
		t.Fatalf("read delivered event: %v", err)
	}
	if status != OutboxStatusDelivered || attempts != 1 {
		t.Fatalf("status=%s attempts=%d, want delivered/1", status, attempts)
	}
}

func TestOutboxRelayDeadLettersAfterMaxAttempts(t *testing.T) {
	db, ok := relayTestDB(t)
	if !ok {
		t.Skip("PostgreSQL not available")
	}
	defer db.Close()

	ctx := context.Background()
	event := relayTestEvent("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeee4", "test.relay_failure")
	defer db.Pool.Exec(ctx, `DELETE FROM outbox_events WHERE event_id = $1`, event.EventID)
	appendRelayTestEvent(t, db, event)

	sink := &recordingSink{err: errors.New("sink unavailable")}
	store := NewOutboxStore(db.Pool)
	for attempt := 1; attempt <= OutboxMaxAttempts; attempt++ {
		if err := store.RelayOnce(ctx, sink); err == nil {
			t.Fatal("expected sink failure")
		}
		if attempt < OutboxMaxAttempts {
			if _, err := db.Pool.Exec(ctx,
				`UPDATE outbox_events SET next_retry_at = NOW() WHERE event_id = $1`, event.EventID,
			); err != nil {
				t.Fatalf("make retry immediately available: %v", err)
			}
		}
	}

	var status string
	var attempts int
	var nextRetry *time.Time
	var lastError string
	if err := db.Pool.QueryRow(ctx,
		`SELECT status, attempt, next_retry_at, last_error FROM outbox_events WHERE event_id = $1`, event.EventID,
	).Scan(&status, &attempts, &nextRetry, &lastError); err != nil {
		t.Fatalf("read dead-letter event: %v", err)
	}
	if status != OutboxStatusFailed || attempts != OutboxMaxAttempts || nextRetry != nil || lastError == "" {
		t.Fatalf("status=%s attempts=%d next_retry=%v last_error=%q, want failed/%d/no retry/error", status, attempts, nextRetry, lastError, OutboxMaxAttempts)
	}
	if err := store.RelayOnce(ctx, sink); !errors.Is(err, ErrNoPendingEvents) {
		t.Fatalf("dead-letter relay error = %v, want ErrNoPendingEvents", err)
	}
	if len(sink.events) != OutboxMaxAttempts {
		t.Fatalf("sink calls=%d, want %d", len(sink.events), OutboxMaxAttempts)
	}
}
