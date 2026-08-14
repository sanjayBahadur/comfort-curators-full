# Phase P3.0 — superhost-stream.yaml contract freeze

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/deepseek-v4-pro
- **Status:** complete
- **Human sign-off:** recorded in logs/DECISIONS.md (P3.0 line)

## What I built

Created `contracts/api/superhost-stream.yaml` — the frozen SSE event contract
for `GET /v1/superhost/threads/{id}/stream` plus companion thread endpoints.
The contract matches:

- The existing `AgentRunEvent` struct (`internal/automation/models.go:152`)
  and `agent_run_events` table (`internal/automation/schema.go:53-59`).
- The existing event name convention from `internal/automation/models.go:160-169`.
- The YAML style and conventions of `contracts/api/openapi.yaml` and
  `contracts/agents/state_machines.yaml`.

Four new versioned event names for the tool-call flow, each with a distinct
`event_data` shape:
1. `ToolCallProposed.v1` — model proposal before policy evaluation
2. `PolicyDenied.v1` — policy returned denied
3. `ApprovalRequired.v1` — human approval required, run paused
4. `PolicyAllowed.v1` — read tool allowed and executed

Existing run-lifecycle events reused as-is:
`AgentRunQueued.v1`, `AgentRunCompleted.v1`, `AgentRunFailed.v1`,
`AgentRunCancelled.v1`.

Also frozen: request/response shapes for `POST /v1/superhost/threads` (create)
and `POST /v1/superhost/threads/{id}/messages` (send message).

## Files added or changed

- `contracts/api/superhost-stream.yaml` (new)
- `logs/phase-P3-0-stream-contract.md` (this file)

No other files under `contracts/` or anywhere else were modified.

## Decisions I made

- **Standalone file, not merged into openapi.yaml:** SSE streams don't fit
  OpenAPI's request/response model cleanly. This matches how
  `contracts/agents/state_machines.yaml` is a standalone file for
  event-shaped documentation rather than being shoehorned into OpenAPI.

- **Envelope matches AgentRunEvent exactly:** `event_id`, `run_id`,
  `event_name`, `event_data`, `occurred_at` — same fields as the struct
  and the table, no additions or renames.

- **event_data for PolicyAllowed.v1 carries a `result_summary` string,**
  not the full raw result. The task said "a summary of the result fed
  back to the model (not the full raw result necessarily — your judgment
  on how much to include, but it should be enough for the terminal to
  show something meaningful, not just 'ok')." I chose a string summary
  rather than an object because the terminal needs displayable text,
  and the model doesn't need the raw JSON back either.

- **Thread creation returns a run_id in the 201 response** so the caller
  can immediately open a stream. This keeps the UX simple: one POST to
  create, one GET to stream.

- **send_message is fire-and-forget** (202 with a run resource_id) —
  the response doesn't block on agent execution; events flow over SSE.

## What did NOT work

Nothing — this is a standalone contract file; no implementation, no
compilation, no tests to fail.

## Deviations from the plan

None. The approved shape was implemented exactly as described: five new
event shapes (four new tool-flow ones plus the envelope), reuse of
existing completion events, and two thread endpoint request/response
shapes all documented in the same file.

## Open questions

None — the contract is frozen as a seam for P3.1–P3.4 and P4.
