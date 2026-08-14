# Phase P3.3 addendum — orchestrator verification + fixes

- **Date:** 2026-08-09
- **Agent/model:** claude-sonnet-5 (orchestrator, direct edits)
- **Status:** complete

## What happened

P3.3 (opencode-go/deepseek-v4-pro) implemented the runner tool-loop and
wrote six new tests (`internal/automation/runner_test.go`). Its own
sandbox had no reachable Postgres, so its "all tests pass" claim was a
`t.Skip()` false positive — the dispatch process never actually
exercised the new tests against a real database. Direct host-side
verification against a real, correctly-configured throwaway Postgres
found the true picture: **3 real, distinct bugs**, all now fixed and
verified.

## Bugs found and fixed (by the orchestrator, directly)

### 1. Infrastructure errors masquerading as policy denials

`internal/platform/app/tool_executor.go`: `superhostToolExecutor.Evaluate`
caught errors from `RecordToolCall`, `RecordPolicyDecision`, and
`RecordApprovalRequest` (all real DB writes) and folded them into
`ToolLoopOutcome{Type: ToolLoopDenied, DenialReason: fmt.Sprintf("record ...: %v", err)}`
with a **nil** Go error. A database outage during any of these three
writes would therefore be reported to the model — and to the terminal,
later — as "policy denied", not as the infrastructure failure it
actually was. Fixed by returning a real `error` from all three paths
(`return automation.ToolLoopOutcome{}, fmt.Errorf(...)`), which the
runner's existing `if evalErr != nil { return fmt.Errorf(...) }` path
already correctly turns into a failed run.

### 2. Loop-control bug: approval-required pause didn't actually stop the loop

`internal/automation/runner.go`'s `processRun` checked
`handleToolCalls`'s return value only for `err != nil`. When an
approval-required outcome successfully transitioned the run to
`waiting_for_approval` and returned `nil`, the surrounding `for`
loop had no signal telling it to stop — it fell through and called the
provider again on the next iteration, re-proposing the *same* tool
call. That second call's `TransitionState(running -> waiting_for_approval)`
then failed, because the row was no longer in `running` (the first,
correct transition had already moved it to `waiting_for_approval`) —
surfacing as the confusing `automation: lease expired or not owner`
error, which then failed the run entirely. Confirmed directly: the
`agent_run_events` table showed `ToolCallProposed.v1` and
`ApprovalRequired.v1` each recorded **twice** for a single test run
before the fix.

Fixed by changing `handleToolCalls`'s signature to
`(paused bool, err error)`. The approval-required branch returns
`(true, nil)`; every other successful branch returns `(false, nil)`
(loop continues); every error path returns `(false, err)`. Both call
sites in `processRun` now check `paused` explicitly and `return`
immediately when true, instead of only checking `err`.

### 3. Test-design bug: pre-claiming a run before starting the runner

The original `submitAndClaim` test helper called `store.Claim(...)`
and manually `TransitionState(leased -> running)` on a run *before*
starting the runner goroutine. But `RunWorkLoop` drives its own
`Claim()` internally, which only matches rows in the `queued` state —
a run pre-claimed outside the runner is invisible to it. Every test
using this helper would hang forever (runner polling `ErrRunNotFound`
against a row already stuck in `running`), only surfacing as a test
failure via `waitForState`'s timeout, not as any signal pointing at
the real cause. Fixed by replacing it with `submitQueuedRun`, which
just submits and leaves the run `queued`, letting `RunWorkLoop` claim,
run, and complete/fail it itself — matching how production actually
drives a run end to end.

## Also fixed: two test-correctness issues found while verifying #3

- **`t.Fatalf` inside the polling helper skipped cleanup.** The
  `waitForState` helper's `t.Fatalf` on timeout calls
  `runtime.Goexit()`, which skips any code after it in the *calling*
  test function — including the `cancel()`/`wg.Wait()` that should
  stop the runner's background goroutine. An orphaned goroutine would
  then spin against a pool that the test's `t.Cleanup` closes moments
  later, flooding the log with `context canceled`/`closed pool` errors
  that bled into the next test's output and made real failures harder
  to read. Fixed by using `defer wg.Wait()` / `defer cancel()`
  (in that order, so `cancel()` fires first on unwind) immediately
  after starting the goroutine in every test, so cleanup runs
  regardless of how the test function exits.
- **Wrong terminal state expected after the iteration cap.**
  `store.Fail()` sets `retryable`, not `failed`, while
  `attempt < max_attempts` (see `store.go`, and the pre-existing
  `TestFailRetries`/`TestRetryFailedRun` tests exercising the same
  behavior directly) — a fresh run's first attempt always lands in
  `retryable`. `TestRunnerSixIterationHardCap` asserted `StateFailed`;
  fixed to assert `StateRetryable`, matching real system behavior
  rather than an invented expectation.
- **Stub provider shape mismatch.** Three tests
  (`TestRunnerWithToolExecutorAllowedCompletes`,
  `TestRunnerToolCallProposedEvent`, `TestRunnerUsageAccumulation`)
  used `alwaysToolCallsProvider()`, a stub that *never* stops proposing
  tool calls — correct for the iteration-cap test, wrong for tests that
  want to observe a normal completion, since with that provider they'd
  always race the 6-iteration cap instead. Added `sequencedProvider` /
  `toolCallThenCompleteProvider()`: proposes a tool call once, then
  returns a final text answer on the next turn, so these three tests
  can observe genuine multi-turn completion.

## Also fixed: fragile system-prompt loading

`loadSuperhostSystemPrompt()` originally tried `os.ReadFile` against a
relative path (`internal/automation/superhost/prompt/v1.md`), which
only resolves correctly when the process's working directory happens
to be the repo root — not guaranteed in a container. Since P3.6 (which
landed before this fix) already provides
`superhost/prompt.V1()` via `go:embed`, switched to that: the prompt
text is compiled into the binary and no longer depends on the runtime
working directory at all. Removed the placeholder-fallback path (no
longer reachable — `prompt.V1()` always returns the real text) and the
now-unused `path/filepath` import.

## Verification

Direct host-side verification, `-p 1`, against a correctly-configured
throwaway Postgres (this package's tests read `CC_DB_HOST`/`CC_DB_PORT`/
`CC_DB_USER`/`CC_DB_PASS`/`CC_DB_NAME`, **not** `CC_DATABASE_URL` — note
for future verification of this package specifically):

- `go build ./...`, `go vet ./...`: clean.
- `go test -p 1 ./internal/automation/... ./internal/platform/app/...`:
  all pass except the already-confirmed, pre-existing, unrelated
  `internal/automation/superhost` FK-constraint seed-data failures
  (`reservations_feed_id_fkey`) — the same signature documented
  repeatedly earlier this session, not caused by this or any prior
  block.
- All 6 of P3.3's own tests pass for real (not skipped):
  `TestRunnerNoToolCallsCompletes`,
  `TestRunnerWithToolExecutorAllowedCompletes`,
  `TestRunnerApprovalRequiredPausesRun`,
  `TestRunnerSixIterationHardCap`, `TestRunnerToolCallProposedEvent`,
  `TestRunnerUsageAccumulation`.

## Files changed (this addendum, on top of P3.3's own commit)

- `internal/platform/app/tool_executor.go` — 3 error-return fixes.
- `internal/automation/runner.go` — `handleToolCalls` signature change
  to `(bool, error)`, both call sites updated.
- `internal/automation/runner_test.go` — `submitAndClaim` replaced with
  `submitQueuedRun`; `defer`-based goroutine cleanup; corrected
  expected terminal state for the cap test; added `sequencedProvider`
  and switched three tests to it.
- `internal/platform/app/app.go` — `loadSuperhostSystemPrompt` now uses
  `superhost/prompt.V1()` instead of relative-path file reads; import
  cleanup (`path/filepath` removed, `strings` and
  `automation/superhost/prompt` added).

## Open questions carried forward (from P3.3's own log, still open)

- Resume-from-`waiting_for_approval` path (a human approving the
  paused run) is not built — `validTransitions` already allows
  `waiting_for_approval -> running`, but nothing drives that
  transition yet. Likely P3.4 or a follow-up block.
- Per-tool JSON argument schemas still don't exist (`{"type":"object"}`
  generic parameters, carried from P3.1).
