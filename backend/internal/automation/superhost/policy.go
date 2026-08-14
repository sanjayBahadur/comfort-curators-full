package superhost

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var (
	ErrPolicyDenied           = errors.New("superhost: policy denied")
	ErrPolicyUncertainty      = errors.New("superhost: uncertain result, review required")
	ErrPolicyTimeout          = errors.New("superhost: provider timeout, cannot determine result")
	ErrPolicyInvalidInput     = errors.New("superhost: invalid tool input")
	ErrPolicyIdempotency      = errors.New("superhost: idempotency check failed")
	ErrPolicyApprovalRequired = errors.New("superhost: approval required before execution")
	ErrPolicySelfApproval     = errors.New("superhost: maker-checker separation required, requester cannot approve")
)

type PolicyResult string

const (
	PolicyAllowed          PolicyResult = "allowed"
	PolicyDenied           PolicyResult = "denied"
	PolicyApprovalRequired PolicyResult = "approval_required"
	PolicyUncertainty      PolicyResult = "uncertainty"
	PolicyException        PolicyResult = "exception"
)

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

const PolicyVersion = "superhost-policy-v1.0"

type PolicyEngine struct{}

func NewPolicyEngine() *PolicyEngine {
	return &PolicyEngine{}
}

func (pe *PolicyEngine) Evaluate(ctx PolicyContext, input ToolCallInput) PolicyDecision {
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
		decision.Reason = "empty tool name"
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

	if !audienceAllowed(def, ctx.ActorRoles) {
		decision.Result = PolicyDenied
		decision.Reason = fmt.Sprintf("tool %s is not available to this account's role", input.ToolName)
		return decision
	}

	if err := input.ValidateScope(ctx.TenantID, ctx.PropertyID); err != nil {
		decision.Result = PolicyDenied
		decision.Reason = err.Error()
		return decision
	}

	if isDirectMutation(def) {
		decision.Result = PolicyDenied
		decision.Reason = ErrToolDirectMutation.Error()
		return decision
	}

	if def.RequiresApproval {
		decision.Result = PolicyApprovalRequired
		decision.Reason = fmt.Sprintf("tool %s requires %s approval", input.ToolName, def.ApprovalKind)
		decision.OutputClass = string(def.Kind)
		return decision
	}

	decision.Result = PolicyAllowed
	decision.OutputClass = string(def.Kind)
	return decision
}

// roleAudience maps a real IAM role (see internal/iam/models.go's
// RoleOwner/RoleGuest/RoleStaff) to the ToolAudience it's entitled to.
var roleAudience = map[string]ToolAudience{
	"owner": ToolAudienceOwner,
	"staff": ToolAudienceOperations,
	"guest": ToolAudienceGuest,
}

// systemActorRoles are subject-type strings that represent a system-
// triggered run (a cron job, an internal escalation) rather than a real
// human account with one of the three IAM roles -- see
// internal/iam/models.go's recognized subject types. These aren't "an
// account with a role" in the sense audienceAllowed is scoping for, so
// they're exempt from role-based tool gating entirely.
var systemActorRoles = map[string]bool{
	"jarvis":    true,
	"superhost": true,
}

// audienceAllowed decides whether a tool can be used by an actor holding
// actorRoles. ToolAudienceInternal/ToolAudienceUI are role-agnostic --
// every account can use those regardless of role, since they're either
// pure reads or browser UI actions, not role-specific business authority.
// Everything else requires the actor to hold a role that maps to one of
// the tool's listed audiences; an actor with no resolved human role (a
// lookup failure -- see tool_executor.go) fails closed rather than
// defaulting to "allowed."
func audienceAllowed(def ToolDefinition, actorRoles []string) bool {
	for _, aud := range def.Audiences {
		if aud == ToolAudienceInternal || aud == ToolAudienceUI {
			return true
		}
	}
	for _, role := range actorRoles {
		if systemActorRoles[role] {
			return true
		}
		want, ok := roleAudience[role]
		if !ok {
			continue
		}
		for _, aud := range def.Audiences {
			if aud == want {
				return true
			}
		}
	}
	return false
}

func isDirectMutation(def ToolDefinition) bool {
	return def.Kind != ToolKindRead && def.Kind != ToolKindPropose && def.Kind != ToolKindRequest && def.Kind != ToolKindUIAction
}

func (pe *PolicyEngine) EvaluateUncertainty(ctx PolicyContext, input ToolCallInput, reason string) PolicyDecision {
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

func (pe *PolicyEngine) EvaluateException(ctx PolicyContext, input ToolCallInput, reason string) PolicyDecision {
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

type PolicyContext struct {
	RunID      string   `json:"run_id"`
	TenantID   string   `json:"tenant_id"`
	PropertyID string   `json:"property_id"`
	ActorID    string   `json:"actor_id"`
	ActorRoles []string `json:"actor_roles"`
}

type ToolCallBatchInput struct {
	Calls []ToolCallInput `json:"calls"`
}

func (pe *PolicyEngine) EvaluateBatch(ctx PolicyContext, batch ToolCallBatchInput) ([]PolicyDecision, error) {
	if len(batch.Calls) == 0 {
		return nil, fmt.Errorf("superhost: empty tool call batch")
	}

	decisions := make([]PolicyDecision, 0, len(batch.Calls))
	seen := make(map[string]bool)

	for _, call := range batch.Calls {
		if call.CallID == "" {
			return nil, fmt.Errorf("superhost: tool call missing call_id")
		}
		if seen[call.CallID] {
			return nil, fmt.Errorf("superhost: duplicate call_id in batch: %s", call.CallID)
		}
		seen[call.CallID] = true

		dec := pe.Evaluate(ctx, call)
		decisions = append(decisions, dec)
	}

	return decisions, nil
}

type ApprovalRequest struct {
	RequestID      string          `json:"request_id"`
	RunID          string          `json:"run_id"`
	DecisionID     string          `json:"decision_id"`
	ToolName       string          `json:"tool_name"`
	ToolVersion    string          `json:"tool_version"`
	ApprovalKind   string          `json:"approval_kind"`
	RequesterID    string          `json:"requester_id"`
	RequesterRoles []string        `json:"requester_roles,omitempty"`
	TenantID       string          `json:"tenant_id"`
	PropertyID     string          `json:"property_id"`
	State          string          `json:"state"`
	ProposedData   json.RawMessage `json:"proposed_data,omitempty"`
	ActorID        string          `json:"actor_id"`
	ActorRole      string          `json:"actor_role,omitempty"`
	Evidence       string          `json:"evidence,omitempty"`
	Reason         string          `json:"reason,omitempty"`
	PolicyVersion  string          `json:"policy_version"`
	RequestedAt    time.Time       `json:"requested_at"`
	DecidedAt      *time.Time      `json:"decided_at,omitempty"`
}

const (
	ApprovalStatePending   = "pending"
	ApprovalStateApproved  = "approved"
	ApprovalStateRejected  = "rejected"
	ApprovalStateExpired   = "expired"
	ApprovalStateCancelled = "cancelled"
)

var ErrApprovalInvalidState = errors.New("superhost: invalid approval state transition")
var ErrApprovalNotPending = errors.New("superhost: approval is not in pending state")

var validApprovalTransitions = map[string][]string{
	ApprovalStatePending: {ApprovalStateApproved, ApprovalStateRejected, ApprovalStateExpired, ApprovalStateCancelled},
}

func NewApprovalRequest(requestID, runID, decisionID, toolName, toolVersion, approvalKind, requesterID, tenantID, propertyID string, requesterRoles []string, proposedData json.RawMessage) *ApprovalRequest {
	return &ApprovalRequest{
		RequestID:      requestID,
		RunID:          runID,
		DecisionID:     decisionID,
		ToolName:       toolName,
		ToolVersion:    toolVersion,
		ApprovalKind:   approvalKind,
		RequesterID:    requesterID,
		RequesterRoles: requesterRoles,
		TenantID:       tenantID,
		PropertyID:     propertyID,
		State:          ApprovalStatePending,
		ProposedData:   proposedData,
		PolicyVersion:  PolicyVersion,
		RequestedAt:    time.Now().UTC(),
	}
}

func (ar *ApprovalRequest) Decide(approverID, approverRole, toState, evidence, reason string) error {
	if ar.State != ApprovalStatePending {
		return fmt.Errorf("%w: current state is %s", ErrApprovalNotPending, ar.State)
	}

	allowed, ok := validApprovalTransitions[ar.State]
	if !ok {
		return ErrApprovalInvalidState
	}

	valid := false
	for _, s := range allowed {
		if s == toState {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("%w: %s -> %s", ErrApprovalInvalidState, ar.State, toState)
	}

	if approverID == ar.RequesterID && toState == ApprovalStateApproved {
		return ErrPolicySelfApproval
	}

	now := time.Now().UTC()
	ar.State = toState
	ar.ActorID = approverID
	ar.ActorRole = approverRole
	ar.Evidence = evidence
	ar.Reason = reason
	ar.DecidedAt = &now

	return nil
}

func (ar *ApprovalRequest) IsTerminal() bool {
	switch ar.State {
	case ApprovalStateApproved, ApprovalStateRejected, ApprovalStateExpired, ApprovalStateCancelled:
		return true
	default:
		return false
	}
}
