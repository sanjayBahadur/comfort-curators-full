# Phase P3.2 addendum — orchestrator verification + fixes

- **Date:** 2026-08-09
- **Agent/model:** claude-sonnet-5 (orchestrator, direct edits)
- **Status:** complete

## What happened

P3.2 (opencode-go/deepseek-v4-pro) self-committed and self-merged before
this review started — its own build/vet/test run in the dispatch sandbox
passed. Direct host-side verification against a real Postgres (this
package's tests read `CC_DB_HOST`/`CC_DB_PORT`/`CC_DB_USER`/`CC_DB_PASS`/
`CC_DB_NAME`, not `CC_DATABASE_URL`) found one real race condition (fixed
proactively, before it could be observed as a test failure) and two real
bugs the sandbox's own test run apparently didn't hit (environment
differences are plausible, but not confirmed — not worth chasing further
now that both are fixed and verified).

## Issues found and fixed

### 1. Terminal-state race could drop the final event (found by inspection, fixed proactively)

The original handler checked `IsTerminal(run.State)` using a `run` value
read *before* fetching that cycle's events. `CompleteWithUsage`/`Fail`/
`Cancel` each `UPDATE agent_runs SET state = ...` and then `INSERT` the
run's terminal event (`AgentRunCompleted.v1` etc.) as two separate
statements, not one transaction. A poll that read `run.State` as already
terminal in the gap between those two statements would emit `[DONE]`
having never delivered that terminal event — an SSE terminal that's
supposed to render every event verbatim would silently miss the one
event that says the run finished. This is the kind of glitch that's
rare in a unit test (single-digit-millisecond gap) but real on a live
demo backed by network-latency database calls.

Fixed by restructuring the handler around one helper
(`streamDeliver`) that always fetches-and-delivers events *before*
checking terminal state, and by doing one extra `streamDeliver` call
before actually emitting `[DONE]` — closing the remaining sliver of the
same race. This also let the initial connect and every poll tick share
one code path instead of duplicating the deliver-then-check logic (the
initial call used to go through a separate, non-cursor `ListEvents`
call; it now uses the same cursor-based path with a zero cursor, which
matches every existing event).

### 2. Zero-value cursor crashed the first call (introduced, then fixed, in the same pass)

Unifying the initial load into the cursor-based path (fix #1) meant the
very first call now passes Go's zero-value `string` (`""`) as the
`event_id` cursor into `ListEventsAfter`'s `event_id > $3` comparison —
`event_id` is a `UUID` column, and Postgres rejects `""` with "invalid
input syntax for type uuid" rather than treating it as "less than
everything". Fixed by initializing the cursor to the nil UUID
(`00000000-...-000000000000`), which sorts below every real
`gen_random_uuid()` value and correctly matches "everything so far".

### 3. `TestStreamRunNotFound` used a non-UUID path parameter

Not a bug in `stream.go` itself, but a real, adjacent gap it surfaced:
`store.Get` only classifies a genuine "no rows" `pgx.ErrNoRows` result as
`ErrRunNotFound`. A syntactically invalid UUID (e.g. the test's original
`"nonexistent-run-id"`) fails to *cast* against the `run_id UUID` column
before Postgres even gets to "no rows", producing a different error class
that both `handleStream` and the pre-existing `handleGet`
(`/v1/agent-runs/{run_id}`) fall through to a generic 500 for — this
predates P3.2 and is not scoped to this handler. It's also tangled up
with P3.4's still-open thread_id→run_id mapping (right now `thread_id`
*is* `run_id`, so any non-UUID-shaped `thread_id` hits this). Fixed the
test to use a syntactically valid but nonexistent UUID (what this block
actually guarantees a 404 for), and left a comment pointing at the
adjacent gap for whoever builds P3.4's real thread-ID scheme or fixes
`store.Get`'s error classification.

While in `stream.go`, also switched `err == ErrRunNotFound` to
`errors.Is(err, ErrRunNotFound)` for consistency with `handleGet`'s
existing pattern (doesn't change current behavior — `Get` never wraps
this specific sentinel — but matches the codebase's own convention and
is safer if that ever changes).

## Verification

`go build ./...`, `go vet ./...`: clean. `go test -p 1
./internal/automation/... ./internal/platform/app/...` against a real
throwaway Postgres: all pass, including all 7 of P3.2's own stream
tests, except the already-confirmed, pre-existing, unrelated
`internal/automation/superhost` FK-constraint seed-data failures.

## Files changed (this addendum, on top of P3.2's own commit)

- `internal/automation/stream.go` — race fix (`streamDeliver` helper,
  deliver-before-terminal-check, extra catch-up read before `[DONE]`),
  zero-UUID cursor fix, `errors.Is` for the not-found check.
- `internal/automation/stream_test.go` — `TestStreamRunNotFound` uses a
  valid-but-nonexistent UUID instead of a non-UUID string.

## Open questions carried forward (from P3.2's own log, still open)

- `thread_id` is used directly as `run_id`; P3.4's real thread table
  needs to resolve the mapping (and, per the finding above, should also
  make "no such thread" cleanly 404 regardless of the ID's shape).
- The `(occurred_at, event_id)` tiebreak cursor: `event_id` is a random
  `gen_random_uuid()`, not a monotonically increasing value, so if two
  events for the same run ever share the *exact* same `occurred_at`
  (microsecond resolution — narrow but not impossible under load), the
  tiebreak ordering is arbitrary and, in the specific unlucky case where
  a poll cycle's cursor lands between two same-timestamp events in the
  "wrong" random order, one of them could be skipped on the next poll.
  Not fixed here: closing this properly needs a real monotonic sequence
  column (e.g. `BIGINT GENERATED ALWAYS AS IDENTITY`) added to
  `agent_run_events` via a migration, which is a bigger change than this
  review's scope. The non-streaming `GET /v1/agent-runs/{run_id}/events`
  endpoint is unaffected (it has no cursor, always returns the full,
  correctly-ordered list) — only a live SSE stream could theoretically
  miss one event's live delivery. Flagging for a future hardening pass.
