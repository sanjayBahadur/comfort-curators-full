# Phase P3.10 addendum — orchestrator verification + fix

- **Date:** 2026-08-09
- **Agent/model:** claude-sonnet-5 (orchestrator, direct edit)
- **Status:** complete

## What happened

P3.10 (opencode-go/deepseek-v4-pro) self-committed a genuinely
well-designed solution to the orchestrator-identified gap: no way to
decide a pending approval or resume the paused conversation. The
design — a single `messages_json` checkpoint column, requeue-through-
the-existing-claim-loop rather than a new resume mechanism, and a
`resumeRun` path in the runner that reuses `handleToolCalls` unchanged
— is sound and required no architectural correction, unlike some
earlier blocks in this wave.

Its own dispatch sandbox had no reachable Postgres (same recurring
limitation as every prior block), so none of its 5 new tests — the
single most important one being the full pause→approve→resume→complete
round trip — were ever actually run.

## Bug found and fixed

**Test-setup ordering bug**: `approveTestPool` (the new test file's own
pool helper) called `TRUNCATE TABLE ai_tool_calls, policy_decisions,
approval_requests` *before* `superhost.EnsureToolSchema`, which is what
creates those tables. On a fresh database none of the three tables
exist yet at truncate time, so every single test in the file failed
immediately with `relation "ai_tool_calls" does not exist` — before
ever reaching the actual approval-decision logic under test. Fixed by
reordering: `EnsureToolSchema` first, then truncate.

This is a test-infrastructure bug, not a production-code bug — the
actual `handler.go`/`runner.go`/`store.go` changes were correct as
written. Confirmed by re-running after the one-line reorder: all 5
tests pass for real, including the full round trip (claim → pause with
checkpoint persisted → HTTP decide → requeue → reclaim as attempt 2 →
resume with the approval result fed back → complete).

## Verification

`go build ./...`, `go vet ./...`: clean. `go test -p 1
./internal/automation/... ./internal/platform/app/...
./internal/procurement/...` against a real throwaway Postgres: all
pass, including `TestFullRoundTripPauseApproveResumeComplete` — the
test that actually proves the demo's signature moment (propose → pause
→ human confirms → agent continues) now genuinely works end to end,
not just in isolated pieces.

## Reviewed and accepted as-is (not bugs, documented for awareness)

- **`attempt` counter conflation.** `Claim()`'s existing logic
  increments `attempt` on every claim, including a post-approval
  requeue — so a run that pauses for approval multiple times in
  sequence consumes "attempts" toward `max_attempts` (default 3) even
  though nothing failed. With one or two approval round-trips this is
  harmless (well within the default cap); a run needing many sequential
  approvals could hit `max_attempts` prematurely on an unrelated later
  failure. Not fixed — the failure direction is the safe one (a run
  goes to `failed` slightly earlier than ideal, never silently retries
  past a real limit), and this is not a path the demo's scripted beats
  exercise. Flagging for awareness, not blocking.
- **`SaveMessages` then `TransitionState` non-atomicity.** If
  `SaveMessages` (persisting the checkpoint) succeeds but the
  subsequent `TransitionState(running -> waiting_for_approval)` fails
  (e.g. a concurrent lease expiry), the checkpoint is left persisted on
  a run that never actually reached `waiting_for_approval`. Extremely
  narrow window, same class of risk as similar two-step, non-
  transactional writes already present elsewhere in this file
  (`Claim`, `Complete`, `Fail` are also two-plus-statement, not single
  transactions) — consistent with the codebase's existing risk
  tolerance for this pattern, not a new regression.

## Files changed (this addendum)

- `internal/automation/approve_test.go` — reordered `EnsureToolSchema`
  before the truncate loop in `approveTestPool`.
