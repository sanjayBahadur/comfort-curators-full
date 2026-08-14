package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"comfort-curators-backend/internal/platform/database"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AgentRunStore struct {
	pool *pgxpool.Pool
}

func NewAgentRunStore(pool *pgxpool.Pool) *AgentRunStore {
	return &AgentRunStore{pool: pool}
}

func (s *AgentRunStore) Pool() *pgxpool.Pool {
	return s.pool
}

func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

func (s *AgentRunStore) Submit(ctx context.Context, req SubmitRequest) (*AgentRun, bool, error) {
	maxAttempts := req.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = DefaultMaxAttempts
	}

	if req.IdempotencyKey != "" {
		existing, err := s.GetByIdempotencyKey(ctx, req.RunKind, req.IdempotencyKey)
		if err != nil && !errors.Is(err, ErrRunNotFound) {
			return nil, false, fmt.Errorf("automation: check idempotency: %w", err)
		}
		// GetByIdempotencyKey already excludes cancelled/failed runs (those are
		// eligible for a fresh attempt), so any match found here -- active or
		// completed -- is the one prior call this key is allowed to produce.
		// Falling through to INSERT for a terminal-but-completed match hit the
		// idempotency key's unique constraint instead of returning it.
		if existing != nil {
			return existing, true, nil
		}
	}

	now := time.Now().UTC()
	var run AgentRun
	var errMsg, idemKey *string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO agent_runs (
			run_kind, tenant_id, property_id, actor_id, trigger_type,
			trigger_id, correlation_id, idempotency_key, state,
			provider, model, prompt_template_version,
			input_schema_version, output_schema_version, input_data,
			max_attempts, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		RETURNING run_id, run_kind, tenant_id, property_id, actor_id, trigger_type,
			trigger_id, correlation_id, idempotency_key, state, state_version,
			attempt, max_attempts, provider, model, prompt_template_version,
			input_schema_version, output_schema_version, input_data, output_data,
			messages_json, error_message, usage_minor_units, usage_currency, usage_input_tokens,
			usage_output_tokens, usage_total_tokens, usage_known, created_at, updated_at`,
		req.RunKind, req.TenantID, req.PropertyID, req.ActorID, req.TriggerType,
		req.TriggerID, req.CorrelationID, nullString(req.IdempotencyKey), StateQueued,
		req.Provider, req.Model, req.PromptTemplateVersion,
		req.InputSchemaVersion, req.OutputSchemaVersion, req.InputData,
		maxAttempts, now, now,
	).Scan(
		&run.RunID, &run.RunKind, &run.TenantID, &run.PropertyID, &run.ActorID,
		&run.TriggerType, &run.TriggerID, &run.CorrelationID, &idemKey,
		&run.State, &run.StateVersion, &run.Attempt, &run.MaxAttempts,
		&run.Provider, &run.Model, &run.PromptTemplateVersion,
		&run.InputSchemaVersion, &run.OutputSchemaVersion, &run.InputData,
		&run.OutputData, &run.MessagesJSON, &errMsg, &run.UsageMinorUnits, &run.UsageCurrency,
		&run.UsageInputTokens, &run.UsageOutputTokens, &run.UsageTotalTokens,
		&run.UsageKnown, &run.CreatedAt, &run.UpdatedAt,
	)
	if err != nil {
		// The idempotency check above (GetByIdempotencyKey then INSERT) is a
		// classic check-then-act race: two callers with the same key can both
		// pass the check before either commits, and one loses the INSERT to
		// idx_agent_runs_idempotency. That is exactly the case an idempotency
		// key exists to make safe -- a caller retrying (or, as found live,
		// React re-mounting an effect and firing the request twice) should
		// get the winning run back, not a raw constraint-violation error
		// surfaced all the way to the terminal.
		if req.IdempotencyKey != "" && isUniqueViolation(err) {
			existing, getErr := s.GetByIdempotencyKey(ctx, req.RunKind, req.IdempotencyKey)
			if getErr == nil && existing != nil {
				return existing, true, nil
			}
		}
		return nil, false, fmt.Errorf("automation: submit: %w", err)
	}
	if errMsg != nil {
		run.ErrorMessage = *errMsg
	}
	if idemKey != nil {
		run.IdempotencyKey = *idemKey
	}

	if err := RecordEvent(ctx, s.pool, run.RunID, EventRunQueued, nil); err != nil {
		return nil, false, err
	}

	return &run, false, nil
}

func (s *AgentRunStore) Claim(ctx context.Context, workerID string, leaseDuration time.Duration, allowedKinds []string) (*AgentRun, error) {
	if leaseDuration <= 0 {
		leaseDuration = DefaultLeaseDuration
	}

	var run AgentRun
	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		now := time.Now().UTC()

		// state = $1 (a text equality) and lease_expires_at < $2 (a
		// timestamp comparison) must be separate parameters: they used to
		// share a single $1 bound to `now` (a time.Time), so Postgres
		// inferred $1's type from whichever occurrence it resolved first
		// and then rejected the other with "operator does not exist:
		// timestamp with time zone < text" (or the reverse) every time
		// this ran -- no worker could ever claim a queued run.
		kindFilter := ""
		args := []any{StateQueued, now}
		if len(allowedKinds) > 0 {
			kindFilter = "AND run_kind = ANY($3)"
			args = append(args, allowedKinds)
		}

		query := fmt.Sprintf(
			`SELECT run_id, run_kind, tenant_id, property_id, actor_id, trigger_type,
				trigger_id, correlation_id, idempotency_key, state, state_version,
				attempt, max_attempts, provider, model, prompt_template_version,
				input_schema_version, output_schema_version, input_data, output_data,
				messages_json, error_message, usage_minor_units, usage_currency, usage_input_tokens,
				usage_output_tokens, usage_total_tokens, usage_known, created_at
			 FROM agent_runs
			 WHERE (state = $1
			    OR (state IN ('leased', 'running', 'waiting_for_tool', 'waiting_for_approval')
			        AND lease_expires_at < $2))
			 %s
			 ORDER BY created_at ASC
			 LIMIT 1
			 FOR UPDATE SKIP LOCKED`, kindFilter,
		)

		row := tx.QueryRow(ctx, query, args...)

		var oldState string
		var created time.Time
		var errMsg, idemKey *string
		err := row.Scan(
			&run.RunID, &run.RunKind, &run.TenantID, &run.PropertyID, &run.ActorID,
			&run.TriggerType, &run.TriggerID, &run.CorrelationID, &idemKey,
			&oldState, &run.StateVersion, &run.Attempt, &run.MaxAttempts,
			&run.Provider, &run.Model, &run.PromptTemplateVersion,
			&run.InputSchemaVersion, &run.OutputSchemaVersion, &run.InputData,
			&run.OutputData, &run.MessagesJSON, &errMsg, &run.UsageMinorUnits, &run.UsageCurrency,
			&run.UsageInputTokens, &run.UsageOutputTokens, &run.UsageTotalTokens,
			&run.UsageKnown, &created,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrRunNotFound
		}
		if err != nil {
			return fmt.Errorf("automation: select for claim: %w", err)
		}
		if errMsg != nil {
			run.ErrorMessage = *errMsg
		}
		if idemKey != nil {
			run.IdempotencyKey = *idemKey
		}

		leaseExpires := now.Add(leaseDuration)
		newAttempt := run.Attempt + 1

		_, err = tx.Exec(ctx,
			`UPDATE agent_runs
			 SET state = $2, state_version = state_version + 1,
			     lease_owner = $3, lease_expires_at = $4,
			     heartbeat_at = $4, attempt = $5, updated_at = $4,
			     error_message = NULL
			 WHERE run_id = $1`,
			run.RunID, StateLeased, workerID, leaseExpires, newAttempt,
		)
		if err != nil {
			return fmt.Errorf("automation: update claim: %w", err)
		}

		eventData, _ := json.Marshal(map[string]any{
			"lease_owner":      workerID,
			"lease_expires_at": leaseExpires.Format(time.RFC3339),
			"attempt":          newAttempt,
			"previous_state":   oldState,
		})

		_, err = tx.Exec(ctx,
			`INSERT INTO agent_run_events (run_id, event_name, event_data, occurred_at)
			 VALUES ($1, $2, $3, $4)`,
			run.RunID, EventLeaseClaimed, json.RawMessage(eventData), now,
		)
		if err != nil {
			return fmt.Errorf("automation: record claim event: %w", err)
		}

		run.State = StateLeased
		run.LeaseOwner = workerID
		run.LeaseExpiresAt = &leaseExpires
		run.HeartbeatAt = &leaseExpires
		run.Attempt = newAttempt
		run.CreatedAt = created
		run.UpdatedAt = leaseExpires

		return nil
	})

	if err != nil {
		if errors.Is(err, ErrRunNotFound) {
			return nil, ErrRunNotFound
		}
		return nil, err
	}
	return &run, nil
}

func (s *AgentRunStore) Heartbeat(ctx context.Context, runID, workerID string, extension time.Duration) error {
	if extension <= 0 {
		extension = DefaultLeaseDuration
	}

	now := time.Now().UTC()
	newExpiry := now.Add(extension)

	tag, err := s.pool.Exec(ctx,
		`UPDATE agent_runs
		 SET heartbeat_at = $3, lease_expires_at = $4, updated_at = $3
		 WHERE run_id = $1
		   AND lease_owner = $2
		   AND state IN ('leased', 'running', 'waiting_for_tool', 'waiting_for_approval')`,
		runID, workerID, now, newExpiry,
	)
	if err != nil {
		return fmt.Errorf("automation: heartbeat: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrLeaseExpired
	}
	return nil
}

func (s *AgentRunStore) TransitionState(ctx context.Context, runID, workerID, fromState, toState string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE agent_runs
		 SET state = $3, state_version = state_version + 1, updated_at = NOW()
		 WHERE run_id = $1 AND lease_owner = $2 AND state = $4`,
		runID, workerID, toState, fromState,
	)
	if err != nil {
		return fmt.Errorf("automation: transition to %s: %w", toState, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrLeaseExpired
	}
	return nil
}

func (s *AgentRunStore) Complete(ctx context.Context, runID, workerID string, output json.RawMessage, usageMinor int64, usageCurr string) error {
	return s.CompleteWithUsage(ctx, runID, workerID, output, ProviderUsage{
		UsageMinor:    usageMinor,
		UsageCurrency: usageCurr,
		UsageKnown:    true,
	})
}

func (s *AgentRunStore) CompleteWithUsage(ctx context.Context, runID, workerID string, output json.RawMessage, usage ProviderUsage) error {
	now := time.Now().UTC()

	tag, err := s.pool.Exec(ctx,
		`UPDATE agent_runs
		 SET state = $3, state_version = state_version + 1,
		     output_data = $4, updated_at = $5,
		     usage_minor_units = $6, usage_currency = $7,
		     usage_input_tokens = $8, usage_output_tokens = $9,
		     usage_total_tokens = $10, usage_known = $11,
		     lease_owner = NULL, lease_expires_at = NULL,
		     heartbeat_at = NULL
		 WHERE run_id = $1
		   AND lease_owner = $2
		   AND state IN ('leased', 'running', 'waiting_for_tool', 'waiting_for_approval')`,
		runID, workerID, StateCompleted, output, now,
		usage.UsageMinor, usage.UsageCurrency,
		usage.InputTokens, usage.OutputTokens, usage.TotalTokens, usage.UsageKnown,
	)
	if err != nil {
		return fmt.Errorf("automation: complete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrLeaseExpired
	}

	// output is included here as a plain string, not left for a second
	// GET /v1/agent-runs/{id} round trip: the SSE stream is the frontend's
	// one continuous source of truth (see superhost-stream.ts), and
	// before this fix AgentRunCompleted.v1 carried only usage stats --
	// the model's actual final answer was written to the run's own
	// output_data column and nowhere else, so a run that finished with a
	// plain conversational response (no trailing tool call) rendered as
	// the literal string "run completed" in the terminal and nothing
	// else. Every prior verification this session that "saw" a real
	// answer in the terminal was actually seeing a tool-call summary
	// line (ApprovalRequired's canned text, PolicyAllowed's
	// result_summary) -- a pure narrative answer with no tool call had
	// never actually been checked end to end in the browser itself until
	// this line was missing.
	var outputText string
	_ = json.Unmarshal(output, &outputText)
	eventData, _ := json.Marshal(map[string]any{
		"output":              outputText,
		"usage_minor_units":   usage.UsageMinor,
		"usage_currency":      usage.UsageCurrency,
		"usage_input_tokens":  usage.InputTokens,
		"usage_output_tokens": usage.OutputTokens,
		"usage_total_tokens":  usage.TotalTokens,
		"usage_known":         usage.UsageKnown,
	})
	if err := RecordEvent(ctx, s.pool, runID, EventRunCompleted, json.RawMessage(eventData)); err != nil {
		return err
	}

	return nil
}

func (s *AgentRunStore) Fail(ctx context.Context, runID, workerID string, errMsg string) error {
	now := time.Now().UTC()

	tag, err := s.pool.Exec(ctx,
		`UPDATE agent_runs SET
		     state = CASE
		         WHEN attempt >= max_attempts THEN $4
		         ELSE $3
		     END,
		     state_version = state_version + 1,
		     error_message = $5,
		     lease_owner = NULL,
		     lease_expires_at = NULL,
		     heartbeat_at = NULL,
		     updated_at = $2
		 WHERE run_id = $1
		   AND lease_owner = $6
		   AND state IN ('leased', 'running', 'waiting_for_tool', 'waiting_for_approval')`,
		runID, now,
		StateRetryable,
		StateFailed,
		errMsg,
		workerID,
	)
	if err != nil {
		return fmt.Errorf("automation: fail: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrLeaseExpired
	}

	eventData, _ := json.Marshal(map[string]any{
		"error_message": errMsg,
	})
	if err := RecordEvent(ctx, s.pool, runID, EventRunFailed, json.RawMessage(eventData)); err != nil {
		return err
	}

	return nil
}

func (s *AgentRunStore) Retry(ctx context.Context, runID string) error {
	now := time.Now().UTC()

	tag, err := s.pool.Exec(ctx,
		`UPDATE agent_runs
		 SET state = $2, state_version = state_version + 1,
		     attempt = 0, error_message = NULL,
		     lease_owner = NULL, lease_expires_at = NULL,
		     heartbeat_at = NULL, updated_at = $3
		 WHERE run_id = $1
		   AND state IN ('retryable', 'failed')`,
		runID, StateQueued, now,
	)
	if err != nil {
		return fmt.Errorf("automation: retry: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrRunNotRetryable
	}

	eventData, _ := json.Marshal(map[string]any{
		"retried_at": now.Format(time.RFC3339),
	})
	if err := RecordEvent(ctx, s.pool, runID, EventRunQueued, json.RawMessage(eventData)); err != nil {
		return err
	}

	return nil
}

func (s *AgentRunStore) Cancel(ctx context.Context, runID string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE agent_runs
		 SET state = $2, state_version = state_version + 1,
		     error_message = COALESCE(error_message, 'cancelled'),
		     lease_owner = NULL, lease_expires_at = NULL,
		     heartbeat_at = NULL, updated_at = NOW()
		 WHERE run_id = $1
		   AND state IN ('queued', 'leased', 'running', 'waiting_for_tool',
		                 'waiting_for_approval', 'retryable', 'unknown')`,
		runID, StateCancelled,
	)
	if err != nil {
		return fmt.Errorf("automation: cancel: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrRunNotCancellable
	}

	eventData, _ := json.Marshal(map[string]any{
		"reason": "cancelled",
	})
	if err := RecordEvent(ctx, s.pool, runID, EventRunCancelled, json.RawMessage(eventData)); err != nil {
		return err
	}

	return nil
}

func (s *AgentRunStore) RecoverExpiredLeases(ctx context.Context) (int64, error) {
	now := time.Now().UTC()

	// Deliberately excludes 'waiting_for_approval': that state means a run
	// is correctly, intentionally idle -- paused on a human decision, not
	// abandoned by a dead worker. TransitionState (see handleToolCalls)
	// moves a run into this state without touching lease_owner/
	// lease_expires_at, which still holds the short processing-lease
	// deadline from when the run was actively running. Including this
	// state here meant any approval a human hadn't decided within that
	// short window got silently "recovered" -- requeued and resumed via
	// resumeRun, which unconditionally injects a synthetic "Approved by
	// human reviewer." tool result. That's a real bug we found live: a
	// restock proposal auto-completed itself, telling the operator it had
	// been "approved by a human reviewer" when no human had approved
	// anything, and the real approval click that came seconds later 500'd
	// with "invalid state transition" because the run was already
	// terminal. A human approval decision (DecideRun) is the only thing
	// that should ever move a run out of waiting_for_approval.
	tag, err := s.pool.Exec(ctx,
		`UPDATE agent_runs
		 SET state = $1, state_version = state_version + 1,
		     lease_owner = NULL, lease_expires_at = NULL,
		     heartbeat_at = NULL, updated_at = $2
		 WHERE state IN ('leased', 'running', 'waiting_for_tool')
		   AND lease_expires_at < $2`,
		StateQueued, now,
	)
	if err != nil {
		return 0, fmt.Errorf("automation: recover expired leases: %w", err)
	}

	count := tag.RowsAffected()

	if count > 0 {
		rows, err := s.pool.Query(ctx,
			`SELECT run_id FROM agent_runs
			 WHERE state = $1 AND updated_at = $2`,
			StateQueued, now,
		)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var rid string
				if err := rows.Scan(&rid); err == nil {
					eventData, _ := json.Marshal(map[string]any{"recovered_at": now.Format(time.RFC3339)})
					_ = RecordEvent(ctx, s.pool, rid, EventLeaseExpired, json.RawMessage(eventData))
				}
			}
		}
	}

	return count, nil
}

// UpdateStreamingText overwrites the run's live in-progress narrative text.
// Best-effort and unconditional on lease ownership -- this is display-only
// state a human is watching in real time, not a business record, so a lost
// race with a lease change is fine to just overwrite rather than reject.
func (s *AgentRunStore) UpdateStreamingText(ctx context.Context, runID, text string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE agent_runs SET streaming_text = $2 WHERE run_id = $1`,
		runID, text,
	)
	if err != nil {
		return fmt.Errorf("automation: update streaming text: %w", err)
	}
	return nil
}

// GetStreamingText reads the run's current live in-progress narrative text,
// for the SSE poll loop to compare against what it last sent. A cheap,
// single-column read -- deliberately not routed through Get, which selects
// the full run row this call doesn't need.
func (s *AgentRunStore) GetStreamingText(ctx context.Context, runID string) (string, error) {
	var text string
	err := s.pool.QueryRow(ctx,
		`SELECT streaming_text FROM agent_runs WHERE run_id = $1`,
		runID,
	).Scan(&text)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrRunNotFound
	}
	if err != nil {
		return "", fmt.Errorf("automation: get streaming text: %w", err)
	}
	return text, nil
}

func (s *AgentRunStore) SaveMessages(ctx context.Context, runID, workerID string, messages json.RawMessage) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE agent_runs
		 SET messages_json = $3, updated_at = NOW()
		 WHERE run_id = $1 AND lease_owner = $2`,
		runID, workerID, messages,
	)
	if err != nil {
		return fmt.Errorf("automation: save messages: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrLeaseExpired
	}
	return nil
}

func (s *AgentRunStore) DecideRun(ctx context.Context, runID, toState, errMsg string) error {
	now := time.Now().UTC()
	errMsgPtr := nullString(errMsg)

	tag, err := s.pool.Exec(ctx,
		`UPDATE agent_runs
		 SET state = $2, state_version = state_version + 1,
		     lease_owner = NULL, lease_expires_at = NULL,
		     heartbeat_at = NULL, error_message = COALESCE($3, error_message),
		     updated_at = $4
		 WHERE run_id = $1 AND state = 'waiting_for_approval'`,
		runID, toState, errMsgPtr, now,
	)
	if err != nil {
		return fmt.Errorf("automation: decide run: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrInvalidTransition
	}
	return nil
}

func (s *AgentRunStore) Get(ctx context.Context, runID string) (*AgentRun, error) {
	var run AgentRun
	var leaseExpires, heartbeat *time.Time
	var errMsg, idemKey, leaseOwner *string
	err := s.pool.QueryRow(ctx,
		`SELECT run_id, run_kind, tenant_id, property_id, actor_id, trigger_type,
			trigger_id, correlation_id, idempotency_key, state, state_version,
			attempt, max_attempts, lease_owner, lease_expires_at,
			heartbeat_at, provider, model, prompt_template_version,
			input_schema_version, output_schema_version, input_data, output_data,
			messages_json, error_message, usage_minor_units, usage_currency, usage_input_tokens,
			usage_output_tokens, usage_total_tokens, usage_known, created_at, updated_at
		 FROM agent_runs WHERE run_id = $1`,
		runID,
	).Scan(
		&run.RunID, &run.RunKind, &run.TenantID, &run.PropertyID, &run.ActorID,
		&run.TriggerType, &run.TriggerID, &run.CorrelationID, &idemKey,
		&run.State, &run.StateVersion, &run.Attempt, &run.MaxAttempts,
		&leaseOwner, &leaseExpires, &heartbeat,
		&run.Provider, &run.Model, &run.PromptTemplateVersion,
		&run.InputSchemaVersion, &run.OutputSchemaVersion, &run.InputData,
		&run.OutputData, &run.MessagesJSON, &errMsg, &run.UsageMinorUnits, &run.UsageCurrency,
		&run.UsageInputTokens, &run.UsageOutputTokens, &run.UsageTotalTokens,
		&run.UsageKnown, &run.CreatedAt, &run.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRunNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("automation: get: %w", err)
	}
	if errMsg != nil {
		run.ErrorMessage = *errMsg
	}
	if idemKey != nil {
		run.IdempotencyKey = *idemKey
	}
	if leaseOwner != nil {
		run.LeaseOwner = *leaseOwner
	}

	run.LeaseExpiresAt = leaseExpires
	run.HeartbeatAt = heartbeat
	return &run, nil
}

func (s *AgentRunStore) GetRunIDByThreadID(ctx context.Context, threadID string) (string, error) {
	var runID string
	err := s.pool.QueryRow(ctx,
		`SELECT run_id FROM superhost_threads WHERE thread_id = $1`,
		threadID,
	).Scan(&runID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrRunNotFound
	}
	if err != nil {
		return "", fmt.Errorf("automation: lookup thread: %w", err)
	}
	return runID, nil
}

func (s *AgentRunStore) GetByIdempotencyKey(ctx context.Context, runKind, idempotencyKey string) (*AgentRun, error) {
	var run AgentRun
	var leaseExpires, heartbeat *time.Time
	var errMsg, idemKey, leaseOwner *string
	err := s.pool.QueryRow(ctx,
		`SELECT run_id, run_kind, tenant_id, property_id, actor_id, trigger_type,
			trigger_id, correlation_id, idempotency_key, state, state_version,
			attempt, max_attempts, lease_owner, lease_expires_at,
			heartbeat_at, provider, model, prompt_template_version,
			input_schema_version, output_schema_version, input_data, output_data,
			messages_json, error_message, usage_minor_units, usage_currency, usage_input_tokens,
			usage_output_tokens, usage_total_tokens, usage_known, created_at, updated_at
		 FROM agent_runs
		 WHERE run_kind = $1 AND idempotency_key = $2
		   AND state NOT IN ('cancelled', 'failed')
		 ORDER BY created_at DESC LIMIT 1`,
		runKind, idempotencyKey,
	).Scan(
		&run.RunID, &run.RunKind, &run.TenantID, &run.PropertyID, &run.ActorID,
		&run.TriggerType, &run.TriggerID, &run.CorrelationID, &idemKey,
		&run.State, &run.StateVersion, &run.Attempt, &run.MaxAttempts,
		&leaseOwner, &leaseExpires, &heartbeat,
		&run.Provider, &run.Model, &run.PromptTemplateVersion,
		&run.InputSchemaVersion, &run.OutputSchemaVersion, &run.InputData,
		&run.OutputData, &run.MessagesJSON, &errMsg, &run.UsageMinorUnits, &run.UsageCurrency,
		&run.UsageInputTokens, &run.UsageOutputTokens, &run.UsageTotalTokens,
		&run.UsageKnown, &run.CreatedAt, &run.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRunNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("automation: get by idempotency: %w", err)
	}
	if errMsg != nil {
		run.ErrorMessage = *errMsg
	}
	if idemKey != nil {
		run.IdempotencyKey = *idemKey
	}
	if leaseOwner != nil {
		run.LeaseOwner = *leaseOwner
	}

	run.LeaseExpiresAt = leaseExpires
	run.HeartbeatAt = heartbeat
	return &run, nil
}
