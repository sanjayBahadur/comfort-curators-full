# Phase P9.1b — thread run pointer + rendered UI surfaces section

- **Date:** 2026-08-10
- **Agent/model:** opencode-go/deepseek-v4-pro
- **Status:** complete

## What I built

Two fixes for gaps the orchestrator found verifying P9.1:

1. **ThreadStore.UpdateThreadRun** — handleSendMessage now advances
   `superhost_threads.run_id` to the newest run on every message, so the
   SSE stream endpoint picks up events from later runs instead of being
   pinned to the thread's first run forever.

2. **renderUISurfaces helper** — folds a readable "Available UI surfaces"
   prose section into the user-turn content that the model sees, matching
   the label the system prompt (`prompt/v1.md`) tells the model to look
   for.

## Files added or changed

- `internal/automation/superhost/thread_store.go` — added `UpdateThreadRun`
  (UPDATE superhost_threads SET run_id = $1 WHERE thread_id = $2) between
  `GetThread` and `GetThreadByIdempotencyKey`.
- `internal/automation/superhost/handler.go` — `handleSendMessage` now
  calls `h.threadStore.UpdateThreadRun` after `h.store.Submit` succeeds
  (500 on failure); content-payload construction now appends the
  `renderUISurfaces` block to the user-facing content while keeping
  `ui_surfaces` as a raw structured key.
- `internal/automation/superhost/handler.go` — added `renderUISurfaces`
  function (plain Go string-builder, no templating layer).
- `internal/automation/superhost/handler_test.go` — `TestRenderUISurfacesEmpty`
  and `TestRenderUISurfacesNonEmpty`.
- `internal/automation/superhost/thread_handler_test.go` —
  `TestUpdateThreadRunAdvancesRunID` (creates a thread, sends two messages,
  verifies `GetRunIDByThreadID` returns the newest run's ID each time).

## Tests added

- `TestRenderUISurfacesEmpty` — nil/empty slice produces the "none registered"
  line.
- `TestRenderUISurfacesNonEmpty` — surfaces list produces heading + one
  formatted line per surface with id, label, and actions.
- `TestUpdateThreadRunAdvancesRunID` — after a create + two messages, the
  resolved run ID moves from initial -> first message run -> second message
  run (integration test, skips without PostgreSQL).

## Decisions I made

- The heading text "Available UI surfaces:" in the rendered output matches
  the prompt's "Available UI surfaces" reference; no prompt change was
  needed (the colon is punctuation, the identifying text is identical).
- `UpdateThreadRun` does not check rows-affected — it's called after both
  the run exists and the thread lookup (for auth / property scope) already
  succeeded, so a mismatch row count at that point is a genuine bug, not a
  recoverable runtime condition.
- Grep of every `.RunID` reference in the superhost package confirmed
  nothing else assumes `superhost_threads.run_id` stays pinned.

## What did NOT work

N/A — both gaps were straightforward, well-described, and the codebase
needed no restructuring to accommodate the fixes.

## Open questions

None.
