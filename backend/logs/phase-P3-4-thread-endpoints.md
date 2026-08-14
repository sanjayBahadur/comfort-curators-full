# Phase P3.4 — thread endpoints (create, message, stream)

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/deepseek-v4-pro
- **Status:** complete

## What I built

Three endpoints on top of the existing `AgentRunStore`, per the frozen contract
`contracts/api/superhost-stream.yaml`:

1. **`POST /v1/superhost/threads`** — `create_thread`: creates a thread (thin
   wrapper over an `agent_runs` row), validates tenant/property via the existing
   `ContextAssembler`, and returns `201` with `{thread_id, run_id, created_at}`.
   Idempotent on repeated `idempotency_key` (returns the same thread, matching
   `AgentRunStore.Submit`'s semantics — not a `409` on identical retry).

2. **`POST /v1/superhost/threads/{thread_id}/messages`** — `send_message`:
   resolves the thread, assembles a fresh context, submits a new agent run for
   each message (with property context + user message as `input_data`), returns
   `202` with `{request_id, status: "accepted", resource_id}`. Validates
   content length (1–4000 characters), idempotent via the run's key.

3. **`GET /v1/superhost/threads/{thread_id}/stream`** — already registered by
   P3.2 in `AgentRunHandler`. Updated to resolve `thread_id → run_id` via
   `superhost_threads` with a backward-compatible fallback: if the thread table
   doesn't exist or the thread isn't found, `thread_id` is treated directly as
   `run_id` (preserving P3.2's existing behavior and tests).

### Pre-work checklist
- **P3.2 (SSE streaming)**: landed (commit `e672a3b`)
- **P3.3 (runner tool-loop)**: landed (commit `2bf2d6d`)

Both were in place before this work started.

## Files added or changed

| File | Change |
|------|--------|
| `internal/automation/schema.go` | Added `superhost_threads` table DDL + idempotency index |
| `internal/automation/store.go` | Added `GetRunIDByThreadID` method on `AgentRunStore` |
| `internal/automation/stream.go` | Wired `GetRunIDByThreadID` into `handleStream` with backward-compat fallback |
| `internal/automation/superhost/thread_store.go` | **New.** `ThreadStore` with `CreateThread`, `GetThread`, `GetThreadByIdempotencyKey` |
| `internal/automation/superhost/handler.go` | Added `ThreadStore` field, `NewHandlerWithThreads` constructor, `handleCreateThread`, `handleSendMessage`, registered two new routes |
| `internal/automation/superhost/handler_test.go` | Unchanged |
| `internal/automation/superhost/context_test.go` | Added `superhost_threads` to truncation list |
| `internal/automation/superhost/thread_handler_test.go` | **New.** 16 tests covering create, message, stream resolution, auth, idempotency, validation |

## Decisions I made

1. **Thread is a thin wrapper table.** A thread is a separate row in
   `superhost_threads` that maps `thread_id → run_id`. The contract response
   shows `thread_id` and `run_id` as distinct fields, and the `idempotency_key`
   on the thread endpoint is not the same as a run-level idempotency key (it
   needs its own uniqueness scope per tenant). Adding a dedicated table is
   cleaner than aliasing and reusing `run_id` because: (a) the contract
   explicitly returns both IDs, (b) future phases may attach thread-level
   metadata (purpose, title, status), and (c) thread-level resolution for the
   stream endpoint is unambiguous.

2. **Idempotency semantics match `AgentRunStore.Submit`.** The `409 Duplicate
   idempotency_key` error in the contract is for cases where the key is reused
   with *different* parameters. An identical retry returns `201` + the existing
   thread (same as `Submit` returning the existing run with `duplicate=true`).
   This is implemented via `ThreadStore.CreateThread`'s upfront lookup +
   `Submit`'s idempotency.

3. **Thread ID = run ID for simplicity.** The `thread_id` is set to the
   Postgres-generated `run.RunID` (a UUID). This avoids an extra ID generation
   step and makes the mapping natural — a thread created with a given
   idempotency key always produces the same thread_id and run_id.

4. **send_message creates a new run per message.** `AgentRunStore` doesn't
   support appending a turn to an existing run. The honest minimal
   implementation: each `send_message` assembles fresh context, packs the user
   message + property context into `input_data`, and submits a new agent run.
   The run's idempotency key (the message's key) ensures idempotent retries.
   This is documented below as an open question.

5. **Stream endpoint: backward-compatible resolution.** The stream handler now
   tries `GetRunIDByThreadID` first. If that fails (thread table hasn't been
   migrated yet or thread doesn't exist), it falls back to treating `thread_id`
   directly as `run_id`. This preserves P3.2's existing tests and any callers
   that pass a raw run_id in the path.

6. **Not built: approval-decision endpoints.** Deciding a pending
   `ApprovalRequest` (approve/reject) is explicitly out of scope per the task
   instructions.

7. **`ErrCrossPropertyDenied` maps to `422`** on the thread endpoint, not
   `403`. The contract says "Invalid property or tenant reference" returns
   `422`. The `ContextAssembler` returns `ErrCrossPropertyDenied` for both
   nonexistent properties and cross-tenant access — both cases are "invalid
   reference" from the caller's perspective, so `422 Unprocessable Entity` is
   the correct mapping. (The existing run endpoint uses `403` but runs are
   submitted by an explicit property_id in the body — the thread endpoint
   contract specifies `422`.)

## What did NOT work

- **Multi-turn conversation isn't wired end to end.** The `send_message`
  endpoint creates a new agent run for each message rather than appending to an
  existing run's conversation history. `AgentRunStore` has no
  "append-turn-to-existing-run" primitive (runs are single-attempt, linear
  state machines). Building a full turn-history mechanism that feeds prior
  messages into the model is a genuinely missing primitive and is flagged as an
  open question below.

## Deviations from the plan

- **`ErrCrossPropertyDenied` → `422` instead of `403` on the thread endpoint.**
  The plan said "match the run endpoint" but the frozen contract specifies
  `422` for "Invalid property or tenant reference". Both nonexistent properties
  and cross-tenant attempts are "invalid references" — the caller has provided
  a property/tenant pair the system cannot process, not an authorized-but-not-
  allowed access. The existing run endpoint's `403` is a different contract
  (the run submit uses the body's `property_id` directly, not a path parameter).

## Open questions

1. **Multi-turn conversation append is not wired.** To support genuine
   conversational threads, the agent run model needs an "append turn" primitive
   — either a conversation-history table, a run-level `input_data` append
   mechanism, or both. Until that exists, each `send_message` creates an
   independent agent run. This is a known gap in the agent-run infrastructure,
   not specific to the thread endpoint.
