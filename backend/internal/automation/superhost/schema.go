package superhost

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func EnsureToolSchema(ctx context.Context, db *pgxpool.Pool) error {
	ddl := []string{
		`CREATE TABLE IF NOT EXISTS ai_tool_calls (
			call_id TEXT PRIMARY KEY,
			run_id TEXT NOT NULL,
			tool_name TEXT NOT NULL,
			tool_version TEXT NOT NULL DEFAULT 'v1',
			tool_kind TEXT NOT NULL,
			state TEXT NOT NULL DEFAULT 'proposed'
				CHECK (state IN ('proposed', 'policy_checking', 'approval_required',
				                 'executing', 'succeeded', 'denied', 'retryable',
				                 'failed', 'cancelled')),
			input_data JSONB,
			output_data JSONB,
			idempotency_key TEXT NOT NULL DEFAULT '',
			tenant_id TEXT NOT NULL,
			property_id TEXT NOT NULL,
			actor_id TEXT NOT NULL,
			input_class TEXT NOT NULL DEFAULT '',
			output_class TEXT NOT NULL DEFAULT '',
			policy_result TEXT NOT NULL DEFAULT '',
			error_message TEXT,
			attempt INTEGER NOT NULL DEFAULT 0,
			max_attempts INTEGER NOT NULL DEFAULT 3,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS policy_decisions (
			decision_id TEXT PRIMARY KEY,
			run_id TEXT NOT NULL,
			call_id TEXT,
			tool_name TEXT NOT NULL,
			tool_version TEXT NOT NULL DEFAULT 'v1',
			result TEXT NOT NULL
				CHECK (result IN ('allowed', 'denied', 'approval_required',
				                  'uncertainty', 'exception')),
			reason TEXT,
			input_class TEXT NOT NULL DEFAULT '',
			output_class TEXT NOT NULL DEFAULT '',
			actor_id TEXT NOT NULL,
			actor_roles JSONB,
			tenant_id TEXT NOT NULL,
			property_id TEXT NOT NULL,
			idempotency_key TEXT NOT NULL DEFAULT '',
			policy_version TEXT NOT NULL DEFAULT '',
			decided_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS approval_requests (
			request_id TEXT PRIMARY KEY,
			run_id TEXT NOT NULL,
			decision_id TEXT,
			tool_name TEXT NOT NULL,
			tool_version TEXT NOT NULL DEFAULT 'v1',
			approval_kind TEXT NOT NULL DEFAULT '',
			requester_id TEXT NOT NULL,
			requester_roles JSONB,
			tenant_id TEXT NOT NULL,
			property_id TEXT NOT NULL,
			state TEXT NOT NULL DEFAULT 'pending'
				CHECK (state IN ('pending', 'approved', 'rejected', 'expired', 'cancelled')),
			proposed_data JSONB,
			actor_id TEXT,
			actor_role TEXT,
			evidence TEXT,
			reason TEXT,
			policy_version TEXT NOT NULL DEFAULT '',
			requested_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			decided_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		// Superhost's own per-account task ledger. Not a record of real
		// business state -- tickets/reservations/holds elsewhere remain
		// the source of truth for that -- this is Superhost's own working
		// memory about a specific (tenant, actor) pair: things it noted
		// it should follow up on, and what it's already resolved. It
		// exists because each Superhost run today starts with no memory
		// of prior runs, let alone prior threads; this gives an account
		// continuity across sessions without pretending every note here
		// is a verified operational fact. IDs match this codebase's
		// convention elsewhere in this file (TEXT, not UUID) since
		// property_id and some actor/tenant ids in this system are not
		// always strict UUIDs.
		`CREATE TABLE IF NOT EXISTS superhost_account_tasks (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			tenant_id TEXT NOT NULL,
			actor_id TEXT NOT NULL,
			property_id TEXT,
			description TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'open'
				CHECK (status IN ('open', 'done', 'blocked')),
			resolved_note TEXT,
			origin_run_id TEXT,
			resolved_run_id TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_superhost_account_tasks_actor
			ON superhost_account_tasks (tenant_id, actor_id, status, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_ai_tool_calls_run
			ON ai_tool_calls (run_id)`,
		`CREATE INDEX IF NOT EXISTS idx_ai_tool_calls_idempotency
			ON ai_tool_calls (tool_name, idempotency_key, state)
			WHERE idempotency_key != '' AND state NOT IN ('cancelled', 'failed')`,
		`CREATE INDEX IF NOT EXISTS idx_policy_decisions_run
			ON policy_decisions (run_id)`,
		`CREATE INDEX IF NOT EXISTS idx_approval_requests_run
			ON approval_requests (run_id)`,
	}

	for _, stmt := range ddl {
		if _, err := db.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("superhost: ensure tool schema: %w", err)
		}
	}

	return nil
}

type ToolCallRecord struct {
	CallID         string          `json:"call_id"`
	RunID          string          `json:"run_id"`
	ToolName       string          `json:"tool_name"`
	ToolVersion    string          `json:"tool_version"`
	ToolKind       string          `json:"tool_kind"`
	State          string          `json:"state"`
	InputData      json.RawMessage `json:"input_data,omitempty"`
	OutputData     json.RawMessage `json:"output_data,omitempty"`
	IdempotencyKey string          `json:"idempotency_key"`
	TenantID       string          `json:"tenant_id"`
	PropertyID     string          `json:"property_id"`
	ActorID        string          `json:"actor_id"`
	InputClass     string          `json:"input_class"`
	OutputClass    string          `json:"output_class"`
	PolicyResult   string          `json:"policy_result"`
	ErrorMessage   *string         `json:"error_message,omitempty"`
	Attempt        int             `json:"attempt"`
	MaxAttempts    int             `json:"max_attempts"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

const (
	ToolCallStateProposed       = "proposed"
	ToolCallStatePolicyChecking = "policy_checking"
	ToolCallStateApprovalReq    = "approval_required"
	ToolCallStateExecuting      = "executing"
	ToolCallStateSucceeded      = "succeeded"
	ToolCallStateDenied         = "denied"
	ToolCallStateRetryable      = "retryable"
	ToolCallStateFailed         = "failed"
	ToolCallStateCancelled      = "cancelled"
)

var toolCallTerminalStates = map[string]bool{
	ToolCallStateSucceeded: true,
	ToolCallStateDenied:    true,
	ToolCallStateFailed:    true,
	ToolCallStateCancelled: true,
}

var validToolCallTransitions = map[string][]string{
	ToolCallStateProposed:       {ToolCallStatePolicyChecking, ToolCallStateCancelled},
	ToolCallStatePolicyChecking: {ToolCallStateApprovalReq, ToolCallStateExecuting, ToolCallStateDenied, ToolCallStateFailed},
	ToolCallStateApprovalReq:    {ToolCallStateExecuting, ToolCallStateDenied, ToolCallStateCancelled},
	ToolCallStateExecuting:      {ToolCallStateSucceeded, ToolCallStateRetryable, ToolCallStateFailed, ToolCallStateCancelled},
	ToolCallStateRetryable:      {ToolCallStatePolicyChecking, ToolCallStateFailed, ToolCallStateCancelled},
}

func IsToolCallTerminal(state string) bool {
	return toolCallTerminalStates[state]
}

func ValidateToolCallTransition(from, to string) error {
	if toolCallTerminalStates[from] {
		return fmt.Errorf("superhost: tool call terminal state %s cannot transition", from)
	}
	allowed, ok := validToolCallTransitions[from]
	if !ok {
		return fmt.Errorf("superhost: unknown tool call state %s", from)
	}
	for _, a := range allowed {
		if a == to {
			return nil
		}
	}
	return fmt.Errorf("superhost: invalid tool call transition %s -> %s", from, to)
}

type ToolCallStore struct {
	pool *pgxpool.Pool
}

func NewToolCallStore(pool *pgxpool.Pool) *ToolCallStore {
	return &ToolCallStore{pool: pool}
}

func (s *ToolCallStore) RecordToolCall(ctx context.Context, tc ToolCallRecord) error {
	now := time.Now().UTC()
	if tc.CreatedAt.IsZero() {
		tc.CreatedAt = now
	}
	tc.UpdatedAt = now

	_, err := s.pool.Exec(ctx,
		`INSERT INTO ai_tool_calls (
			call_id, run_id, tool_name, tool_version, tool_kind, state,
			input_data, idempotency_key, tenant_id, property_id, actor_id,
			input_class, output_class, policy_result, attempt, max_attempts,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		ON CONFLICT (call_id) DO UPDATE SET
			state = EXCLUDED.state,
			output_data = EXCLUDED.output_data,
			policy_result = EXCLUDED.policy_result,
			output_class = EXCLUDED.output_class,
			error_message = EXCLUDED.error_message,
			attempt = ai_tool_calls.attempt + 1,
			updated_at = EXCLUDED.updated_at`,
		tc.CallID, tc.RunID, tc.ToolName, tc.ToolVersion, tc.ToolKind, tc.State,
		tc.InputData, tc.IdempotencyKey, tc.TenantID, tc.PropertyID, tc.ActorID,
		tc.InputClass, tc.OutputClass, tc.PolicyResult,
		tc.Attempt, tc.MaxAttempts,
		tc.CreatedAt, tc.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("superhost: record tool call: %w", err)
	}
	return nil
}

func (s *ToolCallStore) GetToolCall(ctx context.Context, callID string) (*ToolCallRecord, error) {
	var tc ToolCallRecord
	var errMsg *string
	err := s.pool.QueryRow(ctx,
		`SELECT call_id, run_id, tool_name, tool_version, tool_kind, state,
			input_data, output_data, idempotency_key, tenant_id, property_id,
			actor_id, input_class, output_class, policy_result,
			error_message, attempt, max_attempts, created_at, updated_at
		 FROM ai_tool_calls WHERE call_id = $1`,
		callID,
	).Scan(
		&tc.CallID, &tc.RunID, &tc.ToolName, &tc.ToolVersion, &tc.ToolKind, &tc.State,
		&tc.InputData, &tc.OutputData, &tc.IdempotencyKey, &tc.TenantID, &tc.PropertyID,
		&tc.ActorID, &tc.InputClass, &tc.OutputClass, &tc.PolicyResult,
		&errMsg, &tc.Attempt, &tc.MaxAttempts, &tc.CreatedAt, &tc.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("superhost: get tool call: %w", err)
	}
	tc.ErrorMessage = errMsg
	return &tc, nil
}

func (s *ToolCallStore) RecordPolicyDecision(ctx context.Context, pd PolicyDecision) error {
	rolesJSON, _ := json.Marshal(pd.ActorRoles)
	_, err := s.pool.Exec(ctx,
		`INSERT INTO policy_decisions (
			decision_id, run_id, tool_name, tool_version, result,
			reason, input_class, output_class, actor_id, actor_roles,
			tenant_id, property_id, idempotency_key, policy_version, decided_at, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		ON CONFLICT (decision_id) DO NOTHING`,
		pd.DecisionID, pd.RunID, pd.ToolName, pd.ToolVersion,
		string(pd.Result), pd.Reason, pd.InputClass, pd.OutputClass,
		pd.ActorID, rolesJSON, pd.TenantID, pd.PropertyID,
		pd.IdempotencyKey, pd.PolicyVersion, pd.DecidedAt, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("superhost: record policy decision: %w", err)
	}
	return nil
}

func (s *ToolCallStore) RecordApprovalRequest(ctx context.Context, ar ApprovalRequest) error {
	rolesJSON, _ := json.Marshal(ar.RequesterRoles)
	_, err := s.pool.Exec(ctx,
		`INSERT INTO approval_requests (
			request_id, run_id, decision_id, tool_name, tool_version,
			approval_kind, requester_id, requester_roles, tenant_id, property_id,
			state, proposed_data, actor_id, actor_role, evidence, reason,
			policy_version, requested_at, decided_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $20)
		ON CONFLICT (request_id) DO UPDATE SET
			state = EXCLUDED.state,
			actor_id = EXCLUDED.actor_id,
			actor_role = EXCLUDED.actor_role,
			evidence = EXCLUDED.evidence,
			reason = EXCLUDED.reason,
			decided_at = EXCLUDED.decided_at,
			updated_at = EXCLUDED.updated_at`,
		ar.RequestID, ar.RunID, ar.DecisionID, ar.ToolName, ar.ToolVersion,
		ar.ApprovalKind, ar.RequesterID, rolesJSON, ar.TenantID, ar.PropertyID,
		ar.State, ar.ProposedData, ar.ActorID, ar.ActorRole, ar.Evidence, ar.Reason,
		ar.PolicyVersion, ar.RequestedAt, ar.DecidedAt, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("superhost: record approval request: %w", err)
	}
	return nil
}

func (s *ToolCallStore) GetApprovalRequest(ctx context.Context, requestID string) (*ApprovalRequest, error) {
	var ar ApprovalRequest
	var requesterRolesRaw []byte
	var decidedAt *time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT request_id, run_id, decision_id, tool_name, tool_version,
			approval_kind, requester_id, requester_roles, tenant_id, property_id,
			state, proposed_data, actor_id, actor_role, evidence, reason,
			policy_version, requested_at, decided_at
		 FROM approval_requests WHERE request_id = $1`,
		requestID,
	).Scan(
		&ar.RequestID, &ar.RunID, &ar.DecisionID, &ar.ToolName, &ar.ToolVersion,
		&ar.ApprovalKind, &ar.RequesterID, &requesterRolesRaw, &ar.TenantID, &ar.PropertyID,
		&ar.State, &ar.ProposedData, &ar.ActorID, &ar.ActorRole, &ar.Evidence, &ar.Reason,
		&ar.PolicyVersion, &ar.RequestedAt, &decidedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("superhost: get approval request: %w", err)
	}
	if len(requesterRolesRaw) > 0 {
		json.Unmarshal(requesterRolesRaw, &ar.RequesterRoles)
	}
	ar.DecidedAt = decidedAt
	return &ar, nil
}

func (s *ToolCallStore) SaveApprovalDecision(ctx context.Context, ar ApprovalRequest) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE approval_requests
		 SET state = $2, actor_id = $3, actor_role = $4,
		     evidence = $5, reason = $6, decided_at = $7,
		     updated_at = NOW()
		 WHERE request_id = $1`,
		ar.RequestID, ar.State, ar.ActorID, ar.ActorRole,
		ar.Evidence, ar.Reason, ar.DecidedAt,
	)
	if err != nil {
		return fmt.Errorf("superhost: save approval decision: %w", err)
	}
	return nil
}
