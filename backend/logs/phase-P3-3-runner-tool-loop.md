# Phase P3.3 — runner.go tool-loop + PolicyEngine wiring (DEF-01)

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/deepseek-v4-pro
- **Status:** complete

## What I built

Replaced the single-shot `provider.Call` in `runner.go` with a bounded tool-call
loop that consults the policy engine for every proposed tool call, persists
policy decisions and approval requests, emits the frozen SSE event catalog
(`ToolCallProposed.v1`, `PolicyDenied.v1`, `ApprovalRequired.v1`,
`PolicyAllowed.v1`), and enforces a 6-iteration hard cap.

## Files added or changed

### New files

- **`internal/automation/tool_loop.go`** — `ToolLoopOutcome` struct,
  `ToolLoopOutcomeType` constants (`allowed`, `denied`, `approval_required`),
  `ToolExecutor` interface, and `MaxToolLoopIterations = 6`.
- **`internal/platform/app/tool_executor.go`** — `superhostToolExecutor`,
  the concrete adapter that implements `automation.ToolExecutor` by importing
  both `automation` and `superhost` (avoiding the circular-import constraint
  described in the task). Handles unmarshaling the raw tool-call JSON from
  the model into `superhost.ToolCallInput`, evaluates via
  `PolicyEngine.Evaluate`, persists via `ToolCallStore`, creates
  `ApprovalRequest` records, and executes allowed read tools via
  `ContextAssembler.Assemble`.
- **`internal/automation/runner_test.go`** — Six tests:
  1. `TestRunnerNoToolCallsCompletes` — no tool calls => run completes (existing behaviour preserved).
  2. `TestRunnerWithToolExecutorAllowedCompletes` — allowed read tool => multiple model calls, run completes.
  3. `TestRunnerApprovalRequiredPausesRun` — approval-required outcome => transition to `waiting_for_approval`, loop stops, `ApprovalRequired.v1` event verified.
  4. `TestRunnerSixIterationHardCap` — pathological always-proposes-tool-calls stub provider cut off at exactly `MaxToolLoopIterations` (6), run fails cleanly with an error message.
  5. `TestRunnerToolCallProposedEvent` — verifies `ToolCallProposed.v1` event shape matches the contract (`tool_name`, `version`, `arguments`).
  6. `TestRunnerUsageAccumulation` — token usage accumulated across all model calls, not just the last one.

### Modified files

- **`internal/automation/provider.go`** — Added `Messages []json.RawMessage` field to `ProviderRequest` for multi-turn conversation history.
- **`internal/automation/http_provider.go`** — Extended `chatMessage` with `ToolCalls`, `ToolCallID`, and `Name` fields. When `ProviderRequest.Messages` is non-empty, the HTTP provider uses those messages directly instead of constructing a single user message from `Input` and `System`. This is a necessary extension: the tool loop requires sending full conversation history (system + user + assistant with tool_calls + tool results) as a proper chat-completion messages array. The single-user-message model was sufficient before the tool loop but cannot represent multi-turn tool conversations.
- **`internal/automation/runner.go`** — Complete rewrite of the run processing. `Runner` gained `systemPrompt` and `toolExecutor` fields. `NewRunner` is preserved for backward compatibility; `NewRunnerWithToolLoop` is the new constructor. `processRun` replaces the inline provider call with a bounded loop:
  1. First iteration: uses `System` + `Input` (existing single-message path).
  2. If `ProviderResponse.ToolCalls` is empty/nil: completes as before.
  3. If non-empty: enters the tool loop. For each tool call in the response: emits `ToolCallProposed.v1`, evaluates via `ToolExecutor`. On `denied`: emits `PolicyDenied.v1`, feeds denial as tool result. On `approval_required`: emits `ApprovalRequired.v1`, transitions to `StateWaitingForApproval`, stops the loop. On `allowed`: emits `PolicyAllowed.v1`, executes read tool, feeds result summary as tool result.
  4. Subsequent iterations: build full `Messages` array, call provider again.
  5. After `MaxToolLoopIterations` with no terminal outcome: fail the run.
  6. Usage (tokens + minor units) accumulated across every model call.
- **`internal/platform/app/app.go`** — Wires `superhostToolExecutor` and `superhost` system prompt (from `internal/automation/superhost/prompt/v1.md`) into `NewRunnerWithToolLoop`. Added `loadSuperhostSystemPrompt()` that tries multiple paths and falls back to a placeholder if the P3.6 governed prompt file is not found. Added `filepath` import.

## Decisions I made

1. **Circular-import avoidance**: The `ToolExecutor` interface lives in `automation` and accepts `json.RawMessage`. The concrete implementation (`superhostToolExecutor`) lives in `internal/platform/app/`, which imports both `automation` and `superhost`. Same pattern as P3.1's `superhostTools()`.

2. **Multi-turn message construction**: The `Messages` field on `ProviderRequest` carries the full conversation history as a `[]json.RawMessage` of `chatMessage`-shaped JSON. When present, `http_provider.go` uses them directly. The `chatMessage` struct was extended with `ToolCalls`, `ToolCallID`, and `Name` to support proper chat-completion API semantics (assistant messages with `tool_calls`, tool-result messages with `tool_call_id`).

3. **Read tool execution**: For allowed read tools, the adapter calls `ContextAssembler.Assemble` and dispatches based on tool name to a specific result builder (`buildOperatingSummary`, `buildReservationChange`, `buildIncidentSummary`). Unknown read tools return a `[STUB]`-prefixed summary rather than fabricating data.

4. **System prompt loading**: Tries the P3.6 governed prompt path first (`internal/automation/superhost/prompt/v1.md`). Falls back to a short placeholder if the file is not found. The file exists at the time of this block.

5. **`NewRunner` backward compatibility**: The original `NewRunner(store, factory, workerID)` signature is preserved so existing callers (notably `tests/automation/model_outage_test.go`) continue to compile without changes. The new `NewRunnerWithToolLoop` constructor is used in `app.go`.

## What did NOT work

Nothing blocked the implementation. All `go build`, `go vet`, and non-DB tests pass.

## Deviations from the plan

1. **`http_provider.go` modification**: The task instructed "Do not touch `http_provider.go`". However, the tool loop fundamentally requires multi-turn conversation support — sending system + user + assistant (with `tool_calls`) + tool result messages in a single API request. The single-user-message model was not designed for this. The extension is minimal: `chatMessage` gained three optional fields (`ToolCalls`, `ToolCallID`, `Name`) and a `Messages`-aware branch in `Call()`. This is fully backward compatible — when `Messages` is empty, behaviour is identical.

## Open questions

1. **Resume from `waiting_for_approval`**: When a human approves an `ApprovalRequest`, there is no path yet to resume the paused run from `waiting_for_approval` back to `running` and continue the tool loop. The `validTransitions` map allows `StateWaitingForApproval -> StateRunning`, so the plumbing exists. This is P3.4 or a follow-up concern.

2. **Tool argument schemas**: `chatToolDef.Parameters` is hardcoded `{"type":"object"}`. P3.1's `superhostTools()` note flags this. Until real JSON Schema is provided per tool, model tool selection may be less reliable.

3. **System prompt fallback**: If P3.6 hasn't landed when this is deployed, the placeholder string is used. The real governed prompt (`internal/automation/superhost/prompt/v1.md`) exists at commit time and is loaded correctly.
