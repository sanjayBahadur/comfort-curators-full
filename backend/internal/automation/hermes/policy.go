package hermes

import (
	"encoding/json"
	"fmt"
	"time"
)

type PolicyResult string

const (
	PolicyAllowed          PolicyResult = "allowed"
	PolicyDenied           PolicyResult = "denied"
	PolicyApprovalRequired PolicyResult = "approval_required"
	PolicyUncertainty      PolicyResult = "uncertainty"
	PolicyException        PolicyResult = "exception"
)

const PolicyVersion = "hermes-policy-v1.0"

type PolicyDecision struct {
	DecisionID     string       `json:"decision_id"`
	RunID          string       `json:"run_id"`
	ToolName       string       `json:"tool_name"`
	ToolVersion    string       `json:"tool_version"`
	Result         PolicyResult `json:"result"`
	Reason         string       `json:"reason,omitempty"`
	InputClass     string       `json:"input_class"`
	OutputClass    string       `json:"output_class,omitempty"`
	ActorID        string       `json:"actor_id"`
	ActorRoles     []string     `json:"actor_roles,omitempty"`
	TenantID       string       `json:"tenant_id"`
	PropertyID     string       `json:"property_id"`
	IdempotencyKey string       `json:"idempotency_key"`
	PolicyVersion  string       `json:"policy_version"`
	DecidedAt      time.Time    `json:"decided_at"`
}

// PolicyContext carries the durable run context. Tool arguments may never
// widen it; tenant and property always derive from the run context.
type PolicyContext struct {
	RunID      string   `json:"run_id"`
	TenantID   string   `json:"tenant_id"`
	PropertyID string   `json:"property_id"`
	ActorID    string   `json:"actor_id"`
	ActorRoles []string `json:"actor_roles"`
}

type ToolCallInput struct {
	ToolName       string          `json:"tool_name"`
	Version        string          `json:"version"`
	Arguments      json.RawMessage `json:"arguments"`
	CallID         string          `json:"call_id"`
	IdempotencyKey string          `json:"idempotency_key,omitempty"`
}

// HermesPolicyEngine applies the narrow communication authority boundary. It
// can never decide liability, spend money or mutate operational truth; every
// high-risk proposal (free-form draft, delivery submission) requires approval.
type HermesPolicyEngine struct{}

func NewPolicyEngine() *HermesPolicyEngine {
	return &HermesPolicyEngine{}
}

func (pe *HermesPolicyEngine) Evaluate(ctx PolicyContext, input ToolCallInput) PolicyDecision {
	now := time.Now().UTC()

	decision := PolicyDecision{
		DecisionID:     fmt.Sprintf("pd-%d", now.UnixNano()),
		RunID:          ctx.RunID,
		ToolName:       input.ToolName,
		ToolVersion:    input.Version,
		ActorID:        ctx.ActorID,
		ActorRoles:     ctx.ActorRoles,
		TenantID:       ctx.TenantID,
		PropertyID:     ctx.PropertyID,
		IdempotencyKey: input.IdempotencyKey,
		PolicyVersion:  PolicyVersion,
		DecidedAt:      now,
	}

	if input.ToolName == "" {
		decision.Result = PolicyDenied
		decision.Reason = "empty tool name: model text alone cannot select an action"
		return decision
	}

	// Fails closed first: any attempt to name a liability, payment, legal or
	// mutation tool is denied before the allowlist is consulted.
	if IsToolProhibited(input.ToolName) {
		decision.Result = PolicyDenied
		decision.Reason = fmt.Sprintf("%v: %s", ErrToolProhibited, input.ToolName)
		return decision
	}

	def, err := LookupTool(input.ToolName)
	if err != nil {
		decision.Result = PolicyDenied
		decision.Reason = err.Error()
		return decision
	}

	if err := ValidateToolVersion(input.ToolName, input.Version); err != nil {
		decision.Result = PolicyDenied
		decision.Reason = err.Error()
		return decision
	}

	decision.InputClass = string(def.Kind)

	if def.Kind == ToolKindRestrict {
		decision.Result = PolicyDenied
		decision.Reason = "tool exercises prohibited authority"
		return decision
	}

	if err := input.ValidateScope(ctx.TenantID, ctx.PropertyID); err != nil {
		decision.Result = PolicyDenied
		decision.Reason = err.Error()
		return decision
	}

	if def.Kind != ToolKindRead && def.Kind != ToolKindPropose && def.Kind != ToolKindRequest {
		decision.Result = PolicyDenied
		decision.Reason = ErrDirectMutation.Error()
		return decision
	}

	if def.RequiresApproval {
		decision.Result = PolicyApprovalRequired
		decision.Reason = fmt.Sprintf("tool %s requires %s approval before execution", input.ToolName, def.ApprovalKind)
		decision.OutputClass = string(def.Kind)
		return decision
	}

	decision.Result = PolicyAllowed
	decision.OutputClass = string(def.Kind)
	return decision
}

// EvaluateUncertainty records a model/provider outage without executing any
// action. Manual or deterministic fallback remains available.
func (pe *HermesPolicyEngine) EvaluateUncertainty(ctx PolicyContext, input ToolCallInput, reason string) PolicyDecision {
	now := time.Now().UTC()
	return PolicyDecision{
		DecisionID:     fmt.Sprintf("pd-%d", now.UnixNano()),
		RunID:          ctx.RunID,
		ToolName:       input.ToolName,
		ToolVersion:    input.Version,
		Result:         PolicyUncertainty,
		Reason:         reason,
		InputClass:     input.ToolName,
		ActorID:        ctx.ActorID,
		TenantID:       ctx.TenantID,
		PropertyID:     ctx.PropertyID,
		IdempotencyKey: input.IdempotencyKey,
		PolicyVersion:  PolicyVersion,
		DecidedAt:      now,
	}
}

func (pe *HermesPolicyEngine) EvaluateException(ctx PolicyContext, input ToolCallInput, reason string) PolicyDecision {
	now := time.Now().UTC()
	return PolicyDecision{
		DecisionID:     fmt.Sprintf("pd-%d", now.UnixNano()),
		RunID:          ctx.RunID,
		ToolName:       input.ToolName,
		ToolVersion:    input.Version,
		Result:         PolicyException,
		Reason:         reason,
		InputClass:     input.ToolName,
		ActorID:        ctx.ActorID,
		TenantID:       ctx.TenantID,
		PropertyID:     ctx.PropertyID,
		IdempotencyKey: input.IdempotencyKey,
		PolicyVersion:  PolicyVersion,
		DecidedAt:      now,
	}
}

func (in ToolCallInput) ValidateScope(tenantID, propertyID string) error {
	var args map[string]any
	if err := json.Unmarshal(in.Arguments, &args); err != nil {
		return fmt.Errorf("hermes: invalid tool arguments: %w", err)
	}

	if t, ok := args["tenant_id"]; ok {
		if ts, ok := t.(string); ok && ts != "" && ts != tenantID {
			return fmt.Errorf("%w: tenant_id in arguments (%s) must match run context (%s)", ErrToolScopeMismatch, ts, tenantID)
		}
	}
	if p, ok := args["property_id"]; ok {
		if ps, ok := p.(string); ok && ps != "" && ps != propertyID {
			return fmt.Errorf("%w: property_id in arguments (%s) must match run context (%s)", ErrToolScopeMismatch, ps, propertyID)
		}
	}

	return nil
}
