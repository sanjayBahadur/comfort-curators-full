package audit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrAuditAppendOnly = errors.New("audit events are immutable and cannot be updated or deleted")
)

type EventType string

const (
	EventTypeAccess   EventType = "access"
	EventTypeMutation EventType = "mutation"
	EventTypeAuth     EventType = "auth"
	EventTypeAdmin    EventType = "admin"
	EventTypeSystem   EventType = "system"
	EventTypeSecurity EventType = "security"
)

type AuditEvent struct {
	ID            string          `json:"id"`
	EventType     EventType       `json:"event_type"`
	TenantID      string          `json:"tenant_id,omitempty"`
	ActorID       string          `json:"actor_id"`
	Action        string          `json:"action"`
	ResourceType  string          `json:"resource_type"`
	ResourceID    string          `json:"resource_id"`
	PreviousState json.RawMessage `json:"previous_state,omitempty"`
	NewState      json.RawMessage `json:"new_state,omitempty"`
	Metadata      json.RawMessage `json:"metadata,omitempty"`
	CorrelationID string          `json:"correlation_id,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}

type AuditQuery struct {
	TenantID  string
	ActorID   string
	EventType EventType
	From      time.Time
	To        time.Time
	Limit     int
}

func newID(prefix string) string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(b[:])
}

type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type AuditStore struct {
	pool *pgxpool.Pool
}

func NewAuditStore(pool *pgxpool.Pool) *AuditStore {
	return &AuditStore{pool: pool}
}

func (s *AuditStore) Append(ctx context.Context, event AuditEvent) error {
	if event.ID == "" {
		event.ID = newID("aud")
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO audit_events (
			id, event_type, tenant_id, actor_id, action,
			resource_type, resource_id, previous_state, new_state,
			metadata, correlation_id
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9,
			$10, $11
		)
	`,
		event.ID, event.EventType, nullString(event.TenantID), event.ActorID, event.Action,
		event.ResourceType, event.ResourceID, event.PreviousState, event.NewState,
		event.Metadata, nullString(event.CorrelationID),
	)
	return err
}

func (s *AuditStore) AppendTx(ctx context.Context, q querier, event AuditEvent) error {
	if event.ID == "" {
		event.ID = newID("aud")
	}
	_, err := q.Exec(ctx, `
		INSERT INTO audit_events (
			id, event_type, tenant_id, actor_id, action,
			resource_type, resource_id, previous_state, new_state,
			metadata, correlation_id
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9,
			$10, $11
		)
	`,
		event.ID, event.EventType, nullString(event.TenantID), event.ActorID, event.Action,
		event.ResourceType, event.ResourceID, event.PreviousState, event.NewState,
		event.Metadata, nullString(event.CorrelationID),
	)
	return err
}

func (s *AuditStore) Query(ctx context.Context, filter AuditQuery) ([]AuditEvent, error) {
	query := `
		SELECT id, event_type, tenant_id, actor_id, action,
			resource_type, resource_id, previous_state, new_state,
			metadata, correlation_id, created_at
		FROM audit_events
		WHERE 1=1
	`
	var args []any
	argIdx := 1

	if filter.TenantID != "" {
		query += " AND tenant_id = $" + itoa(argIdx)
		args = append(args, filter.TenantID)
		argIdx++
	}
	if filter.ActorID != "" {
		query += " AND actor_id = $" + itoa(argIdx)
		args = append(args, filter.ActorID)
		argIdx++
	}
	if filter.EventType != "" {
		query += " AND event_type = $" + itoa(argIdx)
		args = append(args, filter.EventType)
		argIdx++
	}
	if !filter.From.IsZero() {
		query += " AND created_at >= $" + itoa(argIdx)
		args = append(args, filter.From)
		argIdx++
	}
	if !filter.To.IsZero() {
		query += " AND created_at <= $" + itoa(argIdx)
		args = append(args, filter.To)
		argIdx++
	}

	query += " ORDER BY created_at DESC"

	if filter.Limit > 0 {
		query += " LIMIT $" + itoa(argIdx)
		args = append(args, filter.Limit)
		argIdx++
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []AuditEvent
	for rows.Next() {
		var e AuditEvent
		var tenantID, correlationID *string
		if err := rows.Scan(
			&e.ID, &e.EventType, &tenantID, &e.ActorID, &e.Action,
			&e.ResourceType, &e.ResourceID, &e.PreviousState, &e.NewState,
			&e.Metadata, &correlationID, &e.CreatedAt,
		); err != nil {
			return nil, err
		}
		if tenantID != nil {
			e.TenantID = *tenantID
		}
		if correlationID != nil {
			e.CorrelationID = *correlationID
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func itoa(n int) string {
	if n <= 0 {
		return "0"
	}
	buf := make([]byte, 0, 12)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	return string(buf)
}
