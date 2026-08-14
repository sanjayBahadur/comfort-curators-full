# Phase P3.10 — approval decision endpoint + tool-loop resume (orchestrator-identified gap)

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/deepseek-v4-pro
- **Status:** complete

## What I built

Three pieces to close the pause→decide→resume gap:

1. **Persisted conversation state on pause.** Added `messages_json JSONB` column to `agent_runs` with a checkpoint struct (`runCheckpoint`) containing the full message array, iteration count, and the tool_call_id/tool_name that triggered the approval pause. When `handleToolCalls` hits `ToolLoopApprovalRequired`, it serializes and saves the checkpoint via `SaveMessages` before transitioning to `waiting_for_approval`.

2. **Approval-decision HTTP endpoint.** `POST /v1/superhost/approvals/{request_id}/decide` accepts `{"decision": "approved"|"rejected", "evidence"?, "reason"?}`. Enforces tenant scoping (403 for wrong-tenant, not 404), delegates self-approval check to `ApprovalRequest.Decide` (maps `ErrPolicySelfApproval` to 403), persists the decision via `SaveApprovalDecision`, then transitions the run: `waiting_for_approval -> queued` (approved, lease cleared) or `-> failed` (rejected, with clear error message).

3. **Resume path in `processRun`.** When a claimed run has non-empty `MessagesJSON`, `resumeRun` unmarshals the checkpoint, appends a `tool`-role message carrying the approval decision, sets the iteration offset from the checkpoint, and continues the loop. The 6-iteration hard cap spans the full run (pause + resume combined). Only approved runs reach this path; rejected runs go to `failed` and are never reclaimed.

## Files added or changed

- `internal/automation/schema.go` — `messages_json JSONB` column migration
- `internal/automation/models.go` — `MessagesJSON` field on `AgentRun`, `StateQueued` in `validTransitions[StateWaitingForApproval]`
- `internal/automation/models_test.go` — new transition test case
- `internal/automation/store.go` — `SaveMessages`, `DecideRun` methods; updated `messages_json` in all SELECT/RETURNING queries (`Submit`, `Claim`, `Get`, `GetByIdempotencyKey`)
- `internal/automation/runner.go` — `runCheckpoint` struct; persist checkpoint in `handleToolCalls` on pause; `resumeRun` method; `processRun` dispatches to `resumeRun` when `MessagesJSON` present
- `internal/automation/superhost/schema.go` — `GetApprovalRequest`, `SaveApprovalDecision` on `ToolCallStore`
- `internal/automation/superhost/handler.go` — `NewHandlerWithApprovals` constructor, `handleDecideApproval` handler, route registered
- `internal/platform/app/app.go` — wire `NewHandlerWithApprovals` with `ToolCallStore`
- `internal/automation/approve_test.go` — 5 tests (self-approval 403, rejection → failed, approval requeues, full pause→approve→resume→complete round trip, wrong-tenant 403)

## Decisions I made

- **Single JSON blob for checkpoint:** messages + iteration + last_tool_call_id/tool_name in one `messages_json` column, rather than three separate columns. Keeps the schema change minimal (one column) and the intent clear.
- **Approval tool result is a flat string:** `"Approved by human reviewer."` — no lookup of the approval request details in the runner. Only approved runs reach `resumeRun`, so the semantics are unambiguous. The runner does not gain a dependency on `ToolCallStore`.
- **Rejection goes to `failed`** (not `cancelled`). `failed` captures the clear error message (`"policy denied: human rejected <tool_name>"`) and is the natural terminal state for a policy-gated refusal. The task left this to judgment; `failed` with reason is more diagnostic than `cancelled`.
- **Decide endpoint path uses `/v1/superhost/approvals/{request_id}/decide`** — colocated with the other superhost routes, follows the `POST .../decide` action-resource pattern consistent with the codebase convention.

## What did NOT work

Nothing. Build, vet, and unit tests pass cleanly. Integration tests are written and compile; they skip when PostgreSQL is unavailable (expected in this provider session).

## Deviations from the plan

None.

## Open questions

None.
