package automation

import (
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrInvalidTransition = errors.New("automation: invalid state transition")
	ErrRunNotFound       = errors.New("automation: run not found")
	ErrRunNotCancellable = errors.New("automation: run not in cancellable state")
	ErrRunNotRetryable   = errors.New("automation: run not in retryable state")
	ErrLeaseExpired      = errors.New("automation: lease expired or not owner")
	ErrDuplicateRun      = errors.New("automation: duplicate run with different payload")
)

const (
	StateQueued             = "queued"
	StateLeased             = "leased"
	StateRunning            = "running"
	StateWaitingForTool     = "waiting_for_tool"
	StateWaitingForApproval = "waiting_for_approval"
	StateRetryable          = "retryable"
	StateUnknown            = "unknown"
	StateCompleted          = "completed"
	StateFailed             = "failed"
	StateCancelled          = "cancelled"

	DefaultLeaseDuration = 30 * time.Second
	DefaultMaxAttempts   = 3
	DefaultMaxRetry      = 5 * time.Minute
)

var terminalStates = map[string]bool{
	StateCompleted: true,
	StateCancelled: true,
}

var cancellableStates = map[string]bool{
	StateQueued:             true,
	StateLeased:             true,
	StateRunning:            true,
	StateWaitingForTool:     true,
	StateWaitingForApproval: true,
	StateRetryable:          true,
	StateUnknown:            true,
}

var validTransitions = map[string][]string{
	StateQueued:             {StateLeased, StateCancelled},
	StateLeased:             {StateRunning, StateQueued, StateCancelled, StateFailed},
	StateRunning:            {StateWaitingForTool, StateWaitingForApproval, StateUnknown, StateRetryable, StateCompleted, StateCancelled, StateFailed},
	StateWaitingForTool:     {StateRunning, StateWaitingForApproval, StateRetryable, StateCancelled, StateFailed},
	StateWaitingForApproval: {StateRunning, StateCompleted, StateCancelled, StateFailed, StateQueued},
	StateRetryable:          {StateQueued, StateCancelled, StateFailed},
	StateUnknown:            {StateRetryable, StateQueued, StateCancelled, StateFailed},
	StateFailed:             {StateQueued},
}

func IsTerminal(state string) bool {
	return terminalStates[state]
}

func IsCancellable(state string) bool {
	return cancellableStates[state]
}

func ValidateTransition(from, to string) error {
	if terminalStates[from] {
		return ErrInvalidTransition
	}
	allowed, ok := validTransitions[from]
	if !ok {
		return ErrInvalidTransition
	}
	for _, a := range allowed {
		if a == to {
			return nil
		}
	}
	return ErrInvalidTransition
}

type AgentRun struct {
	RunID                 string          `json:"run_id"`
	RunKind               string          `json:"run_kind"`
	TenantID              string          `json:"tenant_id"`
	PropertyID            string          `json:"property_id"`
	ActorID               string          `json:"actor_id"`
	TriggerType           string          `json:"trigger_type"`
	TriggerID             string          `json:"trigger_id"`
	CorrelationID         string          `json:"correlation_id"`
	IdempotencyKey        string          `json:"idempotency_key,omitempty"`
	State                 string          `json:"state"`
	StateVersion          int             `json:"state_version"`
	Attempt               int             `json:"attempt"`
	MaxAttempts           int             `json:"max_attempts"`
	LeaseOwner            string          `json:"lease_owner,omitempty"`
	LeaseExpiresAt        *time.Time      `json:"lease_expires_at,omitempty"`
	HeartbeatAt           *time.Time      `json:"heartbeat_at,omitempty"`
	Provider              string          `json:"provider"`
	Model                 string          `json:"model"`
	PromptTemplateVersion string          `json:"prompt_template_version"`
	InputSchemaVersion    string          `json:"input_schema_version"`
	OutputSchemaVersion   string          `json:"output_schema_version"`
	InputData             json.RawMessage `json:"input_data,omitempty"`
	OutputData            json.RawMessage `json:"output_data,omitempty"`
	MessagesJSON          json.RawMessage `json:"messages_json,omitempty"`
	ErrorMessage          string          `json:"error_message,omitempty"`
	UsageMinorUnits       int64           `json:"usage_minor_units"`
	UsageCurrency         string          `json:"usage_currency"`
	UsageInputTokens      int64           `json:"usage_input_tokens"`
	UsageOutputTokens     int64           `json:"usage_output_tokens"`
	UsageTotalTokens      int64           `json:"usage_total_tokens"`
	UsageKnown            bool            `json:"usage_known"`
	CreatedAt             time.Time       `json:"created_at"`
	UpdatedAt             time.Time       `json:"updated_at"`
}

// ProviderUsage carries the usage accounting recorded when an agent run
// completes: the model provider's reported token counts and the monetary cost
// derived from the explicit pricing table. UsageKnown is false when the
// (provider, model) pair has no published price or no tokens were reported;
// the cost is then 0 rather than a fabricated figure.
type ProviderUsage struct {
	InputTokens   int64
	OutputTokens  int64
	TotalTokens   int64
	UsageMinor    int64
	UsageCurrency string
	UsageKnown    bool
}

type SubmitRequest struct {
	RunKind               string          `json:"run_kind"`
	TenantID              string          `json:"tenant_id"`
	PropertyID            string          `json:"property_id"`
	ActorID               string          `json:"actor_id"`
	TriggerType           string          `json:"trigger_type"`
	TriggerID             string          `json:"trigger_id"`
	CorrelationID         string          `json:"correlation_id"`
	IdempotencyKey        string          `json:"idempotency_key"`
	Provider              string          `json:"provider"`
	Model                 string          `json:"model"`
	PromptTemplateVersion string          `json:"prompt_template_version,omitempty"`
	InputSchemaVersion    string          `json:"input_schema_version,omitempty"`
	OutputSchemaVersion   string          `json:"output_schema_version,omitempty"`
	InputData             json.RawMessage `json:"input_data,omitempty"`
	MaxAttempts           int             `json:"max_attempts,omitempty"`
}

type AgentRunEvent struct {
	EventID    string          `json:"event_id"`
	RunID      string          `json:"run_id"`
	EventName  string          `json:"event_name"`
	EventData  json.RawMessage `json:"event_data,omitempty"`
	OccurredAt time.Time       `json:"occurred_at"`
}

const (
	EventRunQueued         = "AgentRunQueued.v1"
	EventRunStateChanged   = "AgentRunStateChanged.v1"
	EventRunCompleted      = "AgentRunCompleted.v1"
	EventRunFailed         = "AgentRunFailed.v1"
	EventRunCancelled      = "AgentRunCancelled.v1"
	EventRunFallback       = "AgentRunFallback.v1"
	EventToolProposed      = "AgentToolProposed.v1"
	EventToolPolicyDecided = "AgentToolPolicyDecided.v1"
	EventLeaseClaimed      = "AgentRunLeaseClaimed.v1"
	EventLeaseExpired      = "AgentRunLeaseExpired.v1"
)
