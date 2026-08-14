# Phase P3.4 addendum — orchestrator verification + fixes

- **Date:** 2026-08-09
- **Agent/model:** claude-sonnet-5 (orchestrator, direct edits)
- **Status:** complete

## What happened

P3.4 (opencode-go/deepseek-v4-pro) self-committed before this review — a
thorough, well-reasoned implementation per its own log (thin
`superhost_threads` wrapper table, honest "no turn-append primitive"
open question, correct 422-vs-403 contract mapping with a documented
rationale for deviating from the plan). Its own dispatch sandbox had no
reachable Postgres, so none of its 16 new tests were ever actually
exercised. Direct host-side verification found three real, distinct
bugs — none in the handler/store logic itself, all in wiring and test
infrastructure — plus, as a side effect of fixing the last one, a
full resolution of the long-standing pre-existing FK-constraint gap
this session has repeatedly worked around rather than fixed.

## Bugs found and fixed

### 1. `app.go` was never updated to wire the thread store in (real, demo-breaking)

The composition root still called the old two-argument
`superhost.NewHandler(agentRunStore, superhostAssembler)`. The new
`NewHandlerWithThreads` constructor P3.4 added was never actually used
outside tests (which construct the handler directly). In the real
running server, `Handler.threadStore` is `nil`, and both new endpoints
(`POST /v1/superhost/threads`, `POST /v1/superhost/threads/{id}/messages`)
would unconditionally return `500 "thread store not configured"` —
this block's entire deliverable was unreachable outside its own test
file. Fixed by wiring `superhost.NewThreadStore(...)` and
`NewHandlerWithThreads(...)` into `app.go`, confirmed the pool it uses
(`superhostPool`) is the same pool `automation.EnsureSchema` (which
creates `superhost_threads`) runs against.

### 2. Every authenticated test in `thread_handler_test.go` was structurally incapable of working

All 16 new tests except the two `RequiresAuth` cases used
`httptest.NewServer(mux)` + a real `http.DefaultClient.Do(authRequest(...))`
round trip. This cannot work for two independent reasons: (a)
`httptest.NewRequest` sets `RequestURI`, which `http.Client.Do`
explicitly rejects ("Request.RequestURI can't be set in client
requests") — every test hit this immediately; (b) even past that,
`authRequest`'s `iam.WithSubject(ctx, ...)` injects the authenticated
Subject into the *client-side* request's Go context, which never
crosses an actual TCP connection to the server's own handler goroutine
— so even a corrected client-side request would have looked
unauthenticated to the server. Fixed by switching every test to drive
the mux in-process via `httptest.NewRecorder()` +
`mux.ServeHTTP(rec, req)`, matching the pattern already established
(and working) in `context_test.go` and P3.2's `stream_test.go`. Added
`doRequest` (plain) and `doStreamRequest` (goroutine + timeout, for the
streaming endpoint, mirroring `stream_test.go`'s own pattern) as shared
helpers.

### 3. The stream-resolution test expected `[DONE]` from a run nothing ever completes

`TestThreadToRunResolutionInStreamEndpoint` created a thread (which
leaves its underlying run `queued` — nothing in the test claims or
processes it) and then asserted the stream eventually sends `[DONE]`.
Since nothing ever drives the run to a terminal state, this would have
hung until timeout even after fix #2 made the request itself work.
Fixed by explicitly calling `store.Cancel(...)` on the run before
opening the stream, the same technique P3.2's own
`TestStreamEndsWithDONEOnTerminal` uses to force a terminal state
deterministically.

### 4. `thread_handler_test.go`'s own schema-setup helper was incomplete

`threadTestPool` only called `automation.EnsureSchema` and
`superhost.EnsureToolSchema` — missing `property.EnsureSchema` and the
other schemas the established `testPool` helper in `context_test.go`
sets up. `ContextAssembler.Assemble` (used by `CreateThread`) reads
from the `properties` table, so this failed immediately with "relation
properties does not exist" once fix #2 let the requests actually run.
Fixed by adding the same set of `EnsureSchema` calls and truncated
tables `testPool` already uses.

### 5. Bonus: the session's long-standing "pre-existing FK-constraint" gap is now actually fixed

Fixing #4 surfaced the well-documented `reservations_feed_id_fkey`
failure (repeatedly confirmed this session as pre-existing and
unrelated to every block touched so far) directly in this package's
own test run for the first time in a way that was worth fixing rather
than working around, since it was now blocking verification of P3.4's
own tests. Root cause: `context_test.go`'s `seedReservation` helper
hardcodes `feed_id = 'feed-1'` on the inserted reservation row without
ever inserting a corresponding `calendar_feeds` row first. Fixed by
inserting that feed row (idempotently, `ON CONFLICT (id) DO NOTHING`)
before the reservation insert. This is shared test infrastructure used
by many pre-existing tests in this package (`TestAssembleContext...`,
`TestCrossPropertyRequestIsDenied`, etc.) — **all of them now pass**,
for the first time this session. This was not itself P3.4's bug, but
fixing it here resolves a gap this session has hit and worked around
repeatedly across P0.7, P3.1, and P3.3's verification passes.

## Not fixed — documented as open questions

- **`ThreadStore.CreateThread`'s narrow concurrency race.** Two
  genuinely concurrent requests with the same idempotency key could
  both miss the upfront `GetThreadByIdempotencyKey` lookup, both call
  `Submit` (whose own unique index correctly lets only one actually
  insert a new `agent_runs` row), and then both attempt to `INSERT INTO
  superhost_threads` with the same `(tenant_id, idempotency_key)` —
  protected by a unique index, so the second INSERT fails, but the
  code doesn't catch that specific constraint violation and re-fetch
  the winning thread; it just bubbles up as a generic 500. Sequential
  idempotent retries (the realistic case — a client retrying after a
  timeout) work correctly; true concurrent duplicate submission does
  not get the graceful idempotent-return behavior. Narrow and low
  priority; flagging for a future hardening pass rather than fixing
  now.
- Everything already flagged in P3.4's own log (multi-turn conversation
  append has no real primitive yet; approval-decision endpoints are
  out of scope) still stands.

## Verification

`go build ./...`, `go vet ./...`: clean. `go test -p 1
./internal/automation/... ./internal/platform/app/...` against a real
throwaway Postgres: **every test passes**, including all 16 of P3.4's
own tests and every previously-failing pre-existing test in
`internal/automation/superhost`. This is the first fully clean run of
this test tree all session. (Scope note: this verifies the
automation/superhost/app subsystem this wave's work lives in, not the
full ~340-route backend — that's outside this review's scope.)

## Files changed (this addendum, on top of P3.4's own commit)

- `internal/platform/app/app.go` — wire `NewHandlerWithThreads` +
  `NewThreadStore` into the composition root.
- `internal/automation/superhost/thread_handler_test.go` — rewritten to
  drive requests in-process (`doRequest`/`doStreamRequest`) instead of
  through a structurally-broken real server+client round trip; added
  the missing schema setup; fixed the stream-resolution test to force
  a terminal state before asserting `[DONE]`.
- `internal/automation/superhost/context_test.go` — `seedReservation`
  now seeds the `calendar_feeds` row its hardcoded `feed_id` requires.
