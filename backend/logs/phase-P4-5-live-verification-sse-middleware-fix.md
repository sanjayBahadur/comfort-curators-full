# Phase — live-verification fix: SSE streaming was broken in the real running server

- **Date:** 2026-08-09
- **Agent/model:** claude-sonnet-5 (orchestrator, direct fix, found during live
  verification of the frontend's `P4.5` terminal-mount block)
- **Status:** complete

## What happened

While verifying `P4.5` (mount the Superhost terminal on real routes), I
decided to go beyond the usual unit-test verification and actually rebuild
the backend containers and curl-test the real, live contract the frontend
component depends on: session create → thread create → SSE stream. The
first two worked. The stream request returned:

```json
{"code":"STREAM_UNAVAILABLE","message":"streaming not supported"}
```

**This means `P3.2`'s SSE streaming feature — merged, reviewed, and unit-
tested clean weeks-of-session-time ago — has never actually worked against
the real running server, for the entire duration this push has been
building on top of it (`P4.2`'s SSE client, `P4.3`'s `ConfirmBlock`,
`P4.5`'s terminal mount all silently depend on infrastructure that was
broken the whole time).**

## Root cause

`internal/platform/http/middleware.go`'s `statusWriter` wraps every
request's `http.ResponseWriter` to capture the status code for logging/
metrics/tracing:

```go
type statusWriter struct {
    http.ResponseWriter
    status int
}
```

Embedding `http.ResponseWriter` (an interface) promotes only *that*
interface's methods (`Header`, `Write`, `WriteHeader`). `http.Flusher`
(`Flush()`) is a separate interface — Go does not promote it through this
kind of embedding, even though the real, concrete `http.ResponseWriter`
Go's HTTP server hands to the handler chain genuinely does implement it.

Three middleware layers each wrap every request in a fresh `*statusWriter`
before it ever reaches a route handler: `ObservabilityTracing`,
`ObservabilityMetrics`, `RequestLogging` (all three wired in
`internal/platform/app/app.go`'s `RunAPI`). By the time `stream.go`'s
`handleStream` does `flusher, ok := w.(http.Flusher)`, the answer is always
`false` — not because the server can't flush, but because three layers of
this specific wrapper stripped the *ability to say so*.

**Why no test ever caught this**: every existing test (`stream_test.go` and
everything else in the backend) calls handlers directly —
`mux.ServeHTTP(httptest.NewRecorder(), req)` — which never passes through
`RunAPI`'s middleware chain at all. `httptest.NewRecorder()` itself
correctly implements `http.Flusher`, so every unit test's `w.(http.Flusher)`
check has always passed, testing a code path the real server never
actually takes. This is a real gap in this codebase's testing strategy
(no test exercises the full middleware-wrapped handler chain) — worth a
future block, not fixed here, but flagging it: **any future streaming/
long-lived-connection feature is at risk of the exact same invisible
failure mode unless a test is added that goes through the real handler
chain, not just the raw mux.**

## Fix

Added a `Flush()` method to `*statusWriter` that forwards to the wrapped
`ResponseWriter` if it implements `http.Flusher`:

```go
func (sw *statusWriter) Flush() {
    if f, ok := sw.ResponseWriter.(http.Flusher); ok {
        f.Flush()
    }
}
```

This is the standard fix for this well-known class of Go bug. It correctly
chains through arbitrarily many layers of `*statusWriter` wrapping, since
each layer now has its own real `Flush()` method that forwards to whatever
it wraps (whether that's the raw server `ResponseWriter` or another
`*statusWriter`).

## Verification

1. `go build ./...`, `go vet ./...`: clean.
2. `go test -p 1 ./internal/platform/http/... ./internal/automation/...`
   against a real throwaway Postgres: all pass (no regressions; the fix is
   additive).
3. **Live, end-to-end, against the real rebuilt Docker containers** (not a
   unit test):
   - `POST /auth/session/create` → real session token.
   - `npm run seed` (frontend repo) → real property/worker/ticket data for
     the demo tenant (was previously empty on this fresh container start).
   - `POST /v1/superhost/threads` → real thread created.
   - `GET /v1/superhost/threads/{id}/stream` → **before the fix**:
     `STREAM_UNAVAILABLE`. **After the fix**: a real SSE stream, correct
     `data: {...}\n\n` framing, real `AgentRunQueued.v1` /
     `AgentRunLeaseClaimed.v1` / `AgentRunCompleted.v1` events in order,
     terminated with `data: [DONE]`.

This is the first fully live (not mocked, not unit-tested-only) end-to-end
verification of the Superhost streaming path this entire session, and it
directly validates `P3.2`, `P3.4`'s thread endpoints, and the wire contract
`P4.2`'s frontend SSE client was built against.

## Files changed

- `internal/platform/http/middleware.go` — added `(*statusWriter).Flush()`.

## Why this matters for the demo

Before this fix, the single most important interactive feature of the
whole push — the live Superhost terminal — would have appeared to connect
(the thread-creation call succeeds) and then silently never show a single
event, indefinitely, on stage, in front of investors. There would have
been no error toast, no visible failure — `P4.5`'s `SuperhostMount`
component correctly shows an honest "not connected" state on a *failed*
stream request, but a request that returns 200 and then just... never
flushes anything would have looked like a frozen, "still connecting"
terminal forever. This is exactly the silent-failure class of bug this
session's whole verification discipline exists to catch before it reaches
a stage.
