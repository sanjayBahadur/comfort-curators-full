# Phase P3.2 — SSE streaming (DEF-04)

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/deepseek-v4-pro
- **Status:** complete

## What I built

SSE streaming handler for agent run events, delivering real-time event pushes to
connected clients. The handler:

1. Accepts `GET /v1/superhost/threads/{thread_id}/stream` with thread_id treated
   as run_id (thread-to-run mapping is P3.4's responsibility).
2. Authenticates and scopes to the tenant (same `subjectFromRequest` pattern as
   all other automation handlers).
3. Sets SSE headers (`Content-Type: text/event-stream`, `Cache-Control: no-cache`,
   `Connection: keep-alive`) and verifies `http.Flusher` support (returns 503 if
   unavailable).
4. Sends all existing events immediately via `ListEvents`, then polls at 500ms
   intervals via `ListEventsAfter` for new events.
5. Uses `(occurred_at, event_id)` composite cursor — not `occurred_at` alone —
   to avoid duplicate delivery or skipped events when two events share a timestamp.
6. Terminates with `data: [DONE]\n\n` when the run reaches a terminal state
   (`IsTerminal`) AND all events up to that point have been delivered.
7. Respects `r.Context().Done()` — stops polling and returns on client disconnect.

## Files added or changed

- `internal/automation/stream.go` — new file: SSE streaming HTTP handler
  (`handleStream` on `AgentRunHandler`)
- `internal/automation/events.go` — added `ListEventsAfter` cursor-based query,
  fixed `ListEvents` ORDER BY to include `event_id` for deterministic ordering
- `internal/automation/handler.go` — registered the stream route in `RegisterRoutes`
- `internal/automation/store.go` — added `Pool()` accessor for tests
- `internal/automation/stream_test.go` — new file: comprehensive SSE tests

## Decisions I made

- **Polling at 500ms** rather than Postgres LISTEN/NOTIFY. No NOTIFY wiring
  exists in this codebase; polling is the right scope for this block and is
  straightforward, correct, and bounded (one query per run per cycle).
- **Route registered in handler.go's `RegisterRoutes`** alongside all other
  automation routes, not in a separate file — keeps route registration co-located
  and discoverable. The handler implementation lives in `stream.go` to keep
  the file focused on SSE concerns.
- **thread_id = run_id** for now. P3.4 will create threads and the thread→run
  mapping; when that lands, the stream handler's lookup should use the mapping
  table. Noted as an open question below.
- **`ListEvents` ORDER BY fixed** from `ORDER BY occurred_at ASC` to
  `ORDER BY occurred_at ASC, event_id ASC` — the original ordering was
  non-deterministic for same-timestamp events, which the new cursor design
  depends on.

## What did NOT work

N/A — the implementation is straightforward: read events, poll, stream SSE.

## Deviations from the plan

None.

## Open questions

- **thread→run mapping** (for P3.4): Currently `thread_id` is used directly as
  `run_id`. When P3.4 creates the thread table with a `thread → run_id` mapping,
  this handler's lookup should be updated to resolve `thread_id` → `run_id`
  through that mapping instead. The handler already validates tenant ownership
  on the run (not on a thread resource), so the auth boundary is correct once
  the mapping exists and the run's tenant is verified.
