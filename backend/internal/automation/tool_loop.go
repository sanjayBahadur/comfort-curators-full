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

// Was 6. Confirmed live (Mahanagar Suite, "under 2000 INR, furniture and
// toiletries"): with only 6 iterations to spend, adding N catalog items
// costs N of them outright (one ui_click per item), leaving no room to
// also check shop-cart-running-total between adds the way the prompt
// asks -- a real 8-9 item budget build has no choice but to cram every
// add into as few turns as possible and check the total only once, at
// the very end, past the point where overshooting is still preventable.
// The run in question added 9 items blind, checked the total for the
// first and only time on iteration 9, then hit the cap and failed before
// that check's result ever reached a turn that could act on it -- and the
// cart it left behind was already over budget. Raised well past what a
// real multi-item build plus interleaved running-total checks needs.
const MaxToolLoopIterations = 30
