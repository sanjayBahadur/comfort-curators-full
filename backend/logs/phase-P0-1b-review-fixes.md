# Phase P0.1b — review-driven fixes

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/deepseek-v4-flash
- **Status:** complete

## What I built

Two review-driven consistency fixes in `internal/automation/superhost/`:

- **Fix 1 (wire-value regression):** restored the `context_source` response
  wire value in `handler.go` from `"superhost-context-assembler"` back to
  `"jarvis-context-assembler"`, matching what
  `tests/acceptance/probes.go` still hard-requires. P0.1's own brief said to
  preserve wire-format values; `probes.go` was left untouched as its update
  is explicitly deferred until after P0.2/P0.3 land.
- **Fix 2 (two sources of truth):** `tools.go` now declares
  `const AgentKindSuperhost = "superhost"` (was `"jarvis"`), and `handler.go`'s
  two `req.RunKind = "superhost"` literals now reference the constant
  (`req.RunKind = AgentKindSuperhost`) instead of duplicating the value, so the
  value cannot drift apart again.

## Files added or changed

- `internal/automation/superhost/handler.go` — context_source reverted to
  `"jarvis-context-assembler"`; both RunKind assignments now use
  `AgentKindSuperhost`.
- `internal/automation/superhost/tools.go` — `AgentKindSuperhost` value changed
  from `"jarvis"` to `"superhost"`.
- `logs/phase-P0-1b-review-fixes.md` — this file.

## Decisions I made

- Kept the constant name `AgentKindSuperhost` (no rename) and only changed its
  value, so the 12 call sites in `tests/automation/model_outage_test.go`
  (which reference the symbol, not the string) keep working unchanged.
- Did not touch `tests/acceptance/probes.go` per the task's explicit deferral.
- Did not touch `contracts/`, README prose, `docs/development/ROUTE_INVENTORY.md`,
  or the migration backfill in `internal/automation/schema.go`.

## What did NOT work

- Nothing. `go build ./...`, `go vet ./...`, and the unit test suite all pass.

## Deviations from the plan

- None. Note: the task said the 12 `AgentKindSuperhost` call sites are in
  `internal/automation/model_outage_test.go`, but the file actually lives at
  `tests/automation/model_outage_test.go`; I verified that package's tests
  pass there.

## Open questions

- None.
