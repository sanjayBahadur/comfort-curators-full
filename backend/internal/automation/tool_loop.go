package automation

import (
	"context"
	"encoding/json"
)

type ToolLoopOutcomeType string

const (
	ToolLoopAllowed          ToolLoopOutcomeType = "allowed"
	ToolLoopDenied           ToolLoopOutcomeType = "denied"
	ToolLoopApprovalRequired ToolLoopOutcomeType = "approval_required"
)

type ToolLoopOutcome struct {
	Type     ToolLoopOutcomeType `json:"type"`
	ToolName string              `json:"tool_name"`
	Version  string              `json:"version"`

	ResultSummary     string `json:"result_summary,omitempty"`
	DenialReason      string `json:"denial_reason,omitempty"`
	ApprovalRequestID string `json:"approval_request_id,omitempty"`
	ApprovalSummary   string `json:"approval_summary,omitempty"`
}

type ToolExecutor interface {
	Evaluate(ctx context.Context, run *AgentRun, toolCall json.RawMessage) (ToolLoopOutcome, error)
}

// ApprovedToolExecutor is an optional extension of ToolExecutor: an
// executor that can also carry out the real effect of a tool call once a
// human has actually approved it (see resumeRun). Not every executor
// implements this -- resumeRun type-asserts for it and falls back to a
// plain, honest acknowledgement (never a fabricated "it's done") when it
// doesn't, so existing test doubles that only implement Evaluate keep
// working unchanged.
type ApprovedToolExecutor interface {
	ExecuteApproved(ctx context.Context, run *AgentRun, toolName string, arguments json.RawMessage) (string, error)
}

const MaxToolLoopIterations = 6
