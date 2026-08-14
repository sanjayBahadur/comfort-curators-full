package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"comfort-curators-backend/internal/platform/database"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNoWork                 = errors.New("jobs: no work available")
	ErrIdempotencyKeyConflict = errors.New("jobs: idempotency key exists with different payload")
	ErrNotOwner               = errors.New("jobs: lease owner mismatch")
	ErrMaxAttemptsExceeded    = errors.New("jobs: max attempts exceeded, job moved to dead letter")
)

const (
	StatusPending   = "pending"
	StatusClaimed   = "claimed"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusDead      = "dead"

	DefaultLeaseDuration = 30 * time.Second
	DefaultMaxAttempts   = 5
	DefaultMaxRetry      = 30 * time.Minute
)

type Job struct {
	ID              string          `json:"id"`
	Type            string          `json:"job_type"`
	Status          string          `json:"status"`
	Payload         json.RawMessage `json:"payload"`
	IdempotencyKey  *string         `json:"idempotency_key,omitempty"`
	Attempt         int             `json:"attempt"`
	MaxAttempts     int             `json:"max_attempts"`
	LeaseOwner      *string         `json:"lease_owner,omitempty"`
	LeaseExpiresAt  *time.Time      `json:"lease_expires_at,omitempty"`
	LastHeartbeatAt *time.Time      `json:"last_heartbeat_at,omitempty"`
	NextRetryAt     *time.Time      `json:"next_retry_at,omitempty"`
	ErrorMessage    *string         `json:"error_message,omitempty"`
	Result          json.RawMessage `json:"result,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
	CompletedAt     *time.Time      `json:"completed_at,omitempty"`
}

type EnqueueRequest struct {
	JobType        string          `json:"job_type"`
	Payload        json.RawMessage `json:"payload"`
	IdempotencyKey *string         `json:"idempotency_key,omitempty"`
	MaxAttempts    int             `json:"max_attempts,omitempty"`
}

type EnqueueResult struct {
	Job       *Job `json:"job"`
	Duplicate bool `json:"duplicate"`
}

type JobStore struct {
	pool *pgxpool.Pool
}

func NewJobStore(pool *pgxpool.Pool) *JobStore {
	return &JobStore{pool: pool}
}

func (s *JobStore) Enqueue(ctx context.Context, req EnqueueRequest) (*EnqueueResult, error) {
	maxAttempts := req.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = DefaultMaxAttempts
	}

	if req.IdempotencyKey != nil && *req.IdempotencyKey != "" {
		var existing Job
		err := s.pool.QueryRow(ctx,
			`SELECT id, job_type, status, payload, attempt, max_attempts,
			        idempotency_key, result, error_message, created_at, updated_at, completed_at
			 FROM jobs WHERE idempotency_key = $1
			 ORDER BY created_at DESC LIMIT 1`,
			*req.IdempotencyKey,
		).Scan(
			&existing.ID, &existing.Type, &existing.Status,
			&existing.Payload, &existing.Attempt, &existing.MaxAttempts,
			&existing.IdempotencyKey, &existing.Result, &existing.ErrorMessage,
			&existing.CreatedAt, &existing.UpdatedAt, &existing.CompletedAt,
		)

		if err == nil {
			if existing.Status == StatusCompleted {
				return &EnqueueResult{Job: &existing, Duplicate: true}, nil
			}
			var existingPayload, newPayload any
			if json.Unmarshal(existing.Payload, &existingPayload) != nil ||
				json.Unmarshal(req.Payload, &newPayload) != nil ||
				!jsonPayloadsEqual(existing.Payload, req.Payload) {
				return nil, ErrIdempotencyKeyConflict
			}
			return &EnqueueResult{Job: &existing, Duplicate: true}, nil
		}

		if err != pgx.ErrNoRows {
			return nil, fmt.Errorf("jobs: check idempotency key: %w", err)
		}
	}

	var job Job
	err := s.pool.QueryRow(ctx,
		`INSERT INTO jobs (job_type, payload, idempotency_key, max_attempts)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, job_type, status, payload, attempt, max_attempts,
		           idempotency_key, created_at, updated_at`,
		req.JobType, req.Payload, req.IdempotencyKey, maxAttempts,
	).Scan(
		&job.ID, &job.Type, &job.Status, &job.Payload,
		&job.Attempt, &job.MaxAttempts, &job.IdempotencyKey,
		&job.CreatedAt, &job.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("jobs: enqueue: %w", err)
	}

	return &EnqueueResult{Job: &job, Duplicate: false}, nil
}

func (s *JobStore) Claim(ctx context.Context, workerID string, leaseDuration time.Duration) (*Job, error) {
	if leaseDuration <= 0 {
		leaseDuration = DefaultLeaseDuration
	}

	var job Job
	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		now := time.Now().UTC()

		row := tx.QueryRow(ctx,
			`SELECT id, job_type, status, payload, attempt, max_attempts,
			        idempotency_key, error_message, result, created_at
			 FROM jobs
			 WHERE status = 'pending'
			    OR (status = 'failed' AND next_retry_at <= $1)
			    OR (status IN ('claimed', 'running') AND lease_expires_at < $1)
			 ORDER BY created_at ASC
			 LIMIT 1
			 FOR UPDATE SKIP LOCKED`,
			now,
		)

		var oldStatus string
		var created time.Time
		err := row.Scan(
			&job.ID, &job.Type, &oldStatus, &job.Payload,
			&job.Attempt, &job.MaxAttempts, &job.IdempotencyKey,
			&job.ErrorMessage, &job.Result, &created,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNoWork
		}
		if err != nil {
			return fmt.Errorf("jobs: select for claim: %w", err)
		}

		leaseExpires := now.Add(leaseDuration)
		newAttempt := job.Attempt + 1

		_, err = tx.Exec(ctx,
			`UPDATE jobs
			 SET status = $2, lease_owner = $3, lease_expires_at = $4,
			     last_heartbeat_at = $5, attempt = $6, updated_at = $5,
			     error_message = NULL
			 WHERE id = $1`,
			job.ID, StatusClaimed, workerID, leaseExpires, now, newAttempt,
		)
		if err != nil {
			return fmt.Errorf("jobs: update claim: %w", err)
		}

		job.Status = StatusClaimed
		job.LeaseOwner = &workerID
		job.LeaseExpiresAt = &leaseExpires
		job.LastHeartbeatAt = &now
		job.Attempt = newAttempt
		job.CreatedAt = created
		job.UpdatedAt = now

		return nil
	})

	if err != nil {
		return nil, err
	}
	return &job, nil
}

func (s *JobStore) Heartbeat(ctx context.Context, jobID, workerID string, extension time.Duration) error {
	if extension <= 0 {
		extension = DefaultLeaseDuration
	}

	now := time.Now().UTC()
	newExpiry := now.Add(extension)

	tag, err := s.pool.Exec(ctx,
		`UPDATE jobs
		 SET last_heartbeat_at = $3, lease_expires_at = $4, updated_at = $3
		 WHERE id = $1 AND lease_owner = $2 AND status IN ('claimed', 'running')`,
		jobID, workerID, now, newExpiry,
	)
	if err != nil {
		return fmt.Errorf("jobs: heartbeat: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotOwner
	}
	return nil
}

func (s *JobStore) StartRunning(ctx context.Context, jobID, workerID string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE jobs
		 SET status = $3, updated_at = NOW()
		 WHERE id = $1 AND lease_owner = $2 AND status = 'claimed'`,
		jobID, workerID, StatusRunning,
	)
	if err != nil {
		return fmt.Errorf("jobs: start running: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotOwner
	}
	return nil
}

func (s *JobStore) Complete(ctx context.Context, jobID, workerID string, result json.RawMessage) error {
	now := time.Now().UTC()

	tag, err := s.pool.Exec(ctx,
		`UPDATE jobs
		 SET status = $3, result = $4, completed_at = $5, updated_at = $5,
		     lease_owner = NULL, lease_expires_at = NULL
		 WHERE id = $1 AND lease_owner = $2 AND status IN ('claimed', 'running')`,
		jobID, workerID, StatusCompleted, result, now,
	)
	if err != nil {
		return fmt.Errorf("jobs: complete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotOwner
	}
	return nil
}

func (s *JobStore) Fail(ctx context.Context, jobID, workerID string, errMsg string) error {
	now := time.Now().UTC()

	tag, err := s.pool.Exec(ctx,
		`UPDATE jobs SET
		     status = CASE
		         WHEN attempt >= max_attempts THEN $4
		         ELSE $3
		     END,
		     error_message = $5,
		     next_retry_at = CASE
		         WHEN attempt >= max_attempts THEN NULL
		         ELSE $2::timestamptz + make_interval(secs => LEAST(POWER(2, attempt)::bigint, 1800))
		     END,
		     lease_owner = NULL,
		     lease_expires_at = NULL,
		     updated_at = $2
		 WHERE id = $1 AND lease_owner = $6 AND status IN ('claimed', 'running')`,
		jobID, now,
		StatusFailed,
		StatusDead,
		errMsg,
		workerID,
	)
	if err != nil {
		return fmt.Errorf("jobs: fail: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotOwner
	}
	return nil
}

func (s *JobStore) RecoverExpiredLeases(ctx context.Context) (int64, error) {
	now := time.Now().UTC()

	tag, err := s.pool.Exec(ctx,
		`UPDATE jobs
		 SET status = CASE
		         WHEN attempt >= max_attempts THEN $3
		         ELSE $1
		     END,
		     lease_owner = NULL,
		     lease_expires_at = NULL,
		     last_heartbeat_at = NULL,
		     error_message = CASE
		         WHEN attempt >= max_attempts THEN $4
		         ELSE error_message
		     END,
		     updated_at = $2
		 WHERE status IN ('claimed', 'running')
		   AND lease_expires_at < $2`,
		StatusPending, now, StatusDead, "job exhausted max attempts through repeated lease expiry",
	)
	if err != nil {
		return 0, fmt.Errorf("jobs: recover expired leases: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (s *JobStore) GetDeadLetterJobs(ctx context.Context) ([]Job, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, job_type, status, payload, attempt, max_attempts,
		        idempotency_key, error_message, result, created_at, updated_at, completed_at
		 FROM jobs WHERE status = $1 ORDER BY updated_at DESC`,
		StatusDead,
	)
	if err != nil {
		return nil, fmt.Errorf("jobs: list dead letter: %w", err)
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		var j Job
		if err := rows.Scan(
			&j.ID, &j.Type, &j.Status, &j.Payload,
			&j.Attempt, &j.MaxAttempts, &j.IdempotencyKey,
			&j.ErrorMessage, &j.Result,
			&j.CreatedAt, &j.UpdatedAt, &j.CompletedAt,
		); err != nil {
			return nil, fmt.Errorf("jobs: scan dead letter: %w", err)
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

func (s *JobStore) Cancel(ctx context.Context, jobID string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE jobs
		 SET status = $2, error_message = $3, updated_at = NOW(),
		     lease_owner = NULL, lease_expires_at = NULL
		 WHERE id = $1 AND status IN ('pending', 'claimed')`,
		jobID, StatusDead, "cancelled",
	)
	if err != nil {
		return fmt.Errorf("jobs: cancel: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("jobs: cancel: job not found or not in cancellable state")
	}
	return nil
}

type HandlerFunc func(ctx context.Context, job *Job) (json.RawMessage, error)

type Registry struct {
	mu       sync.RWMutex
	handlers map[string]HandlerFunc
}

func NewRegistry() *Registry {
	return &Registry{handlers: make(map[string]HandlerFunc)}
}

func (r *Registry) Register(jobType string, handler HandlerFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[jobType] = handler
}

func (r *Registry) Dispatch(ctx context.Context, job *Job) (json.RawMessage, error) {
	r.mu.RLock()
	handler, ok := r.handlers[job.Type]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("jobs: no handler registered for job type %q", job.Type)
	}
	return handler(ctx, job)
}

func jsonPayloadsEqual(a, b json.RawMessage) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	var av, bv any
	if err := json.Unmarshal(a, &av); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &bv); err != nil {
		return false
	}
	aJSON, _ := json.Marshal(av)
	bJSON, _ := json.Marshal(bv)
	return string(aJSON) == string(bJSON)
}
