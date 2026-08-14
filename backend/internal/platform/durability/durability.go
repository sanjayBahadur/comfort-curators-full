package durability

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"comfort-curators-backend/internal/platform/database"
	"comfort-curators-backend/internal/platform/logging"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrKeyConflict     = errors.New("durability: idempotency key exists with a different request body")
	ErrVersionConflict = errors.New("durability: optimistic version conflict")
	ErrNoPendingEvents = errors.New("durability: no pending events")
)

func hashBytes(b []byte) string {
	h := sha256.New()
	h.Write(b)
	return fmt.Sprintf("%x", h.Sum(nil))
}

type ProcessResult struct {
	ResultRef json.RawMessage
	Replay    bool
}

type IdempotencyStore struct {
	pool *pgxpool.Pool
}

func NewIdempotencyStore(pool *pgxpool.Pool) *IdempotencyStore {
	return &IdempotencyStore{pool: pool}
}

func (s *IdempotencyStore) Process(
	ctx context.Context,
	key, operationClass string,
	body []byte,
	fn func(ctx context.Context, tx pgx.Tx) (json.RawMessage, error),
) (ProcessResult, error) {
	requestHash := hashBytes(body)

	var resultRef json.RawMessage
	var replay bool

	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		var existingHash string
		var existingResult string
		err := tx.QueryRow(ctx,
			`SELECT request_hash, result_ref FROM idempotency_records
			 WHERE idempotency_key = $1
			 FOR UPDATE`,
			key,
		).Scan(&existingHash, &existingResult)

		if err == nil {
			if existingHash == requestHash {
				resultRef = json.RawMessage(existingResult)
				replay = true
				return nil
			}
			return ErrKeyConflict
		}

		if err != pgx.ErrNoRows {
			return fmt.Errorf("durability: check existing key: %w", err)
		}

		resultRef, err = fn(ctx, tx)
		if err != nil {
			return err
		}

		_, err = tx.Exec(ctx,
			`INSERT INTO idempotency_records (idempotency_key, operation_class, request_hash, result_ref)
			 VALUES ($1, $2, $3, $4)`,
			key, operationClass, requestHash, string(resultRef),
		)
		if err != nil {
			return fmt.Errorf("durability: insert idempotency record: %w", err)
		}

		return nil
	})

	if err != nil {
		return ProcessResult{}, err
	}

	return ProcessResult{ResultRef: resultRef, Replay: replay}, nil
}

type EventEnvelope struct {
	EventID          string          `json:"event_id"`
	EventName        string          `json:"event_name"`
	EventVersion     string          `json:"event_version"`
	OccurredAt       time.Time       `json:"occurred_at"`
	TenantID         string          `json:"tenant_id"`
	ActorType        string          `json:"actor_type"`
	ActorID          string          `json:"actor_id"`
	CorrelationID    string          `json:"correlation_id"`
	CausationID      string          `json:"causation_id"`
	AggregateType    string          `json:"aggregate_type"`
	AggregateID      string          `json:"aggregate_id"`
	AggregateVersion int             `json:"aggregate_version"`
	Payload          json.RawMessage `json:"payload"`
	PropertyID       *string         `json:"property_id,omitempty"`
	IdempotencyKey   *string         `json:"idempotency_key,omitempty"`
	TraceID          *string         `json:"trace_id,omitempty"`
}

type OutboxStore struct {
	pool *pgxpool.Pool
}

const (
	OutboxStatusPending   = "pending"
	OutboxStatusDelivered = "delivered"
	OutboxStatusFailed    = "failed"
	OutboxMaxAttempts     = 5
)

type EventSink interface {
	Deliver(ctx context.Context, event EventEnvelope) error
}

type LogEventSink struct{}

func NewLogEventSink() *LogEventSink {
	return &LogEventSink{}
}

func (s *LogEventSink) Deliver(ctx context.Context, event EventEnvelope) error {
	logging.Info(ctx, "outbox event delivered",
		"event_id", event.EventID,
		"event_name", event.EventName,
		"event_version", event.EventVersion,
		"tenant_id", event.TenantID,
		"aggregate_type", event.AggregateType,
		"aggregate_id", event.AggregateID,
		"payload", event.Payload,
	)
	return nil
}

func NewOutboxStore(pool *pgxpool.Pool) *OutboxStore {
	return &OutboxStore{pool: pool}
}

func (s *OutboxStore) Append(ctx context.Context, tx pgx.Tx, event EventEnvelope) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO outbox_events (
			event_id, event_name, event_version, occurred_at,
			tenant_id, actor_type, actor_id,
			correlation_id, causation_id,
			aggregate_type, aggregate_id, aggregate_version,
			payload, property_id, idempotency_key, trace_id
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7,
			$8, $9,
			$10, $11, $12,
			$13, $14, $15, $16
		)`,
		event.EventID, event.EventName, event.EventVersion, event.OccurredAt,
		event.TenantID, event.ActorType, event.ActorID,
		event.CorrelationID, event.CausationID,
		event.AggregateType, event.AggregateID, event.AggregateVersion,
		event.Payload, event.PropertyID, event.IdempotencyKey, event.TraceID,
	)
	if err != nil {
		return fmt.Errorf("durability: append outbox event: %w", err)
	}
	return nil
}

func (s *OutboxStore) AppendEvents(ctx context.Context, tx pgx.Tx, events ...EventEnvelope) error {
	for _, event := range events {
		if err := s.Append(ctx, tx, event); err != nil {
			return err
		}
	}
	return nil
}

// RelayOnce keeps the row lock until the sink result has been recorded. A
// failed sink call is durable and retryable; after the attempt ceiling the
// event remains in the failed state.
func (s *OutboxStore) RelayOnce(ctx context.Context, sink EventSink) error {
	var deliveryErr error
	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		var event EventEnvelope
		var maxAttempts int
		err := tx.QueryRow(ctx,
			`SELECT event_id::text, event_name, event_version, occurred_at,
			        tenant_id::text, actor_type, actor_id::text,
			        correlation_id::text, causation_id::text,
			        aggregate_type, aggregate_id::text, aggregate_version,
			        payload, property_id::text, idempotency_key, trace_id, max_attempts
			 FROM outbox_events
			 WHERE (status = $1 OR (status = $2 AND next_retry_at <= NOW()))
			 ORDER BY occurred_at ASC, created_at ASC
			 LIMIT 1
			 FOR UPDATE SKIP LOCKED`,
			OutboxStatusPending, OutboxStatusFailed,
		).Scan(
			&event.EventID, &event.EventName, &event.EventVersion, &event.OccurredAt,
			&event.TenantID, &event.ActorType, &event.ActorID,
			&event.CorrelationID, &event.CausationID,
			&event.AggregateType, &event.AggregateID, &event.AggregateVersion,
			&event.Payload, &event.PropertyID, &event.IdempotencyKey, &event.TraceID, &maxAttempts,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNoPendingEvents
		}
		if err != nil {
			return fmt.Errorf("durability: claim outbox event: %w", err)
		}

		var attempt int
		if err := tx.QueryRow(ctx,
			`UPDATE outbox_events
			 SET attempt = attempt + 1, last_error = NULL
			 WHERE event_id = $1
			 RETURNING attempt`, event.EventID,
		).Scan(&attempt); err != nil {
			return fmt.Errorf("durability: increment outbox attempt: %w", err)
		}

		if err := sink.Deliver(ctx, event); err != nil {
			now := time.Now().UTC()
			var nextRetry any
			if maxAttempts <= 0 {
				maxAttempts = OutboxMaxAttempts
			}
			if attempt < maxAttempts {
				delay := time.Duration(1<<attempt) * time.Second
				if delay > 30*time.Minute {
					delay = 30 * time.Minute
				}
				nextRetry = now.Add(delay)
			}
			if _, updateErr := tx.Exec(ctx,
				`UPDATE outbox_events
				 SET status = $2, last_error = $3, next_retry_at = $4
				 WHERE event_id = $1`, event.EventID, OutboxStatusFailed, err.Error(), nextRetry,
			); updateErr != nil {
				return fmt.Errorf("durability: mark outbox event failed: %w", updateErr)
			}
			deliveryErr = fmt.Errorf("durability: deliver outbox event: %w", err)
			return nil
		}

		if _, err := tx.Exec(ctx,
			`UPDATE outbox_events
			 SET status = $2, delivered_at = NOW(), next_retry_at = NULL, last_error = NULL
			 WHERE event_id = $1`, event.EventID, OutboxStatusDelivered,
		); err != nil {
			return fmt.Errorf("durability: mark outbox event delivered: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return deliveryErr
}

func CheckVersion(expected, actual int) error {
	if expected != actual {
		return fmt.Errorf("%w: expected version %d, got %d", ErrVersionConflict, expected, actual)
	}
	return nil
}
