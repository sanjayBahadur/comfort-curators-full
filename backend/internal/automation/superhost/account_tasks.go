package superhost

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	AccountTaskStatusOpen    = "open"
	AccountTaskStatusDone    = "done"
	AccountTaskStatusBlocked = "blocked"
)

// AccountTask is one entry in Superhost's own per-(tenant, actor) task
// ledger -- see schema.go's superhost_account_tasks DDL for why this
// exists and what it deliberately is not (a record of real business
// state; tickets/reservations/holds remain the source of truth for that).
type AccountTask struct {
	ID            string    `json:"id"`
	TenantID      string    `json:"tenant_id"`
	ActorID       string    `json:"actor_id"`
	PropertyID    string    `json:"property_id,omitempty"`
	Description   string    `json:"description"`
	Status        string    `json:"status"`
	ResolvedNote  string    `json:"resolved_note,omitempty"`
	OriginRunID   string    `json:"origin_run_id,omitempty"`
	ResolvedRunID string    `json:"resolved_run_id,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type AccountTaskStore struct {
	pool *pgxpool.Pool
}

func NewAccountTaskStore(pool *pgxpool.Pool) *AccountTaskStore {
	return &AccountTaskStore{pool: pool}
}

// Log records a new open task for this (tenant, actor). propertyID and
// originRunID may be empty.
func (s *AccountTaskStore) Log(ctx context.Context, tenantID, actorID, propertyID, description, originRunID string) (*AccountTask, error) {
	if description == "" {
		return nil, fmt.Errorf("superhost: account task description is required")
	}
	now := time.Now().UTC()
	var t AccountTask
	err := s.pool.QueryRow(ctx,
		`INSERT INTO superhost_account_tasks
			(tenant_id, actor_id, property_id, description, status, origin_run_id, created_at, updated_at)
		 VALUES ($1, $2, NULLIF($3, ''), $4, $5, NULLIF($6, ''), $7, $7)
		 RETURNING id, tenant_id, actor_id, COALESCE(property_id, ''), description, status,
			COALESCE(resolved_note, ''), COALESCE(origin_run_id, ''), COALESCE(resolved_run_id, ''),
			created_at, updated_at`,
		tenantID, actorID, propertyID, description, AccountTaskStatusOpen, originRunID, now,
	).Scan(&t.ID, &t.TenantID, &t.ActorID, &t.PropertyID, &t.Description, &t.Status,
		&t.ResolvedNote, &t.OriginRunID, &t.ResolvedRunID, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("superhost: log account task: %w", err)
	}
	return &t, nil
}

// Resolve marks a task done or blocked, scoped to the same (tenant, actor)
// that created it -- one account's Superhost thread cannot resolve
// another account's notes.
func (s *AccountTaskStore) Resolve(ctx context.Context, tenantID, actorID, taskID, status, note, resolvedRunID string) (*AccountTask, error) {
	if status != AccountTaskStatusDone && status != AccountTaskStatusBlocked {
		return nil, fmt.Errorf("superhost: invalid resolution status %q", status)
	}
	now := time.Now().UTC()
	var t AccountTask
	err := s.pool.QueryRow(ctx,
		`UPDATE superhost_account_tasks
		 SET status = $1, resolved_note = NULLIF($2, ''), resolved_run_id = NULLIF($3, ''), updated_at = $4
		 WHERE id = $5::uuid AND tenant_id = $6 AND actor_id = $7
		 RETURNING id, tenant_id, actor_id, COALESCE(property_id, ''), description, status,
			COALESCE(resolved_note, ''), COALESCE(origin_run_id, ''), COALESCE(resolved_run_id, ''),
			created_at, updated_at`,
		status, note, resolvedRunID, now, taskID, tenantID, actorID,
	).Scan(&t.ID, &t.TenantID, &t.ActorID, &t.PropertyID, &t.Description, &t.Status,
		&t.ResolvedNote, &t.OriginRunID, &t.ResolvedRunID, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("superhost: account task %s not found for this account", taskID)
		}
		return nil, fmt.Errorf("superhost: resolve account task: %w", err)
	}
	return &t, nil
}

// ListForAccount returns this account's open tasks first (oldest first, so
// the longest-standing item surfaces), then its most recently resolved
// tasks up to recentResolvedLimit -- enough for Superhost to say "last
// time we spoke about X, here's where that landed" without pulling the
// account's entire history into every context assembly.
func (s *AccountTaskStore) ListForAccount(ctx context.Context, tenantID, actorID string, recentResolvedLimit int) ([]AccountTask, error) {
	rows, err := s.pool.Query(ctx,
		`(SELECT id, tenant_id, actor_id, COALESCE(property_id, ''), description, status,
			COALESCE(resolved_note, ''), COALESCE(origin_run_id, ''), COALESCE(resolved_run_id, ''),
			created_at, updated_at
		  FROM superhost_account_tasks
		  WHERE tenant_id = $1 AND actor_id = $2 AND status = 'open'
		  ORDER BY created_at ASC)
		 UNION ALL
		 (SELECT id, tenant_id, actor_id, COALESCE(property_id, ''), description, status,
			COALESCE(resolved_note, ''), COALESCE(origin_run_id, ''), COALESCE(resolved_run_id, ''),
			created_at, updated_at
		  FROM superhost_account_tasks
		  WHERE tenant_id = $1 AND actor_id = $2 AND status != 'open'
		  ORDER BY updated_at DESC
		  LIMIT $3)`,
		tenantID, actorID, recentResolvedLimit,
	)
	if err != nil {
		return nil, fmt.Errorf("superhost: list account tasks: %w", err)
	}
	defer rows.Close()

	var tasks []AccountTask
	for rows.Next() {
		var t AccountTask
		if err := rows.Scan(&t.ID, &t.TenantID, &t.ActorID, &t.PropertyID, &t.Description, &t.Status,
			&t.ResolvedNote, &t.OriginRunID, &t.ResolvedRunID, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("superhost: scan account task: %w", err)
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}
