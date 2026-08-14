# Phase P0.3b — review-driven fixes

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/deepseek-v4-flash
- **Status:** complete

## What I built

Test-rigor and log-accuracy fixes driven by an independent review of the
`P0.3` commit (`568ed14`), per `internal/automation/superhost/handler_test.go`:

- **Exact status assertions.** Both the direct-route request
  (`POST /v1/superhost/runs`) and the followed-redirect request
  (`POST /v1/jarvis/runs` via default `http.Client`) now assert
  `resp.StatusCode != http.StatusUnauthorized` instead of loose
  exclusions (`not 404` / `not 308`). I confirmed in `handler.go` that
  `handleCreateRun` calls `subjectFromRequest(r)` first, which fails with no
  auth header and returns `401` before the body or store are touched, so
  `401` is deterministic for both. The exact check is strictly stronger: it
  still catches a missing route (`404`) and a shim that did not redirect
  (would stay `308`), and now also any other unexpected status.
- **No body-preservation overclaim.** The followed-redirect path cannot prove
  body arrival because auth fails before the body is read. The log no longer
  claims this test "re-sends the body"; it now records that body preservation
  across the 308 relies on Go's documented `http.Client` redirect behavior
  (`GetBody` provided by `strings.NewReader`), which this block does not
  independently re-prove.
- **Generic mechanism proof (optional, taken).** Added a separate, small test
  `TestPostBodyPreservedAcross308Redirect` that redirects to a plain
  `httptest` echo handler and asserts the exact body arrives at the target —
  proving the mechanism generically rather than through `handleCreateRun`'s
  auth gate. No auth-token setup was needed.

## Files added or changed

- `internal/automation/superhost/handler_test.go` — both assertions tightened
  to exact `401`; added `TestPostBodyPreservedAcross308Redirect` proving the
  body-preservation mechanism via an echo handler.
- `logs/phase-P0-3-route-rename.md` — corrected the "Files added or changed"
  bullet and added an "Accuracy note (P0.3b review fix)" section replacing
  the body-preservation overclaim.
- `logs/phase-P0-3b-review-fixes.md` — this file.

## Decisions I made

- Did not touch `handler.go`'s route/redirect logic — the reviewer confirmed
  it is correct; this block is test rigor and log accuracy only.
- Kept the redirect-shim assertions (`308` + `Location`) exactly as they were;
  those were already precise and are the independent proof that the shim
  exists and points at the primary route.
- Kept the followed-redirect assertion at `401` (same handler, no auth) rather
  than a status-insensitive "not 308", per the task.

## What did NOT work

- Nothing. `go build ./...`, `go vet ./...`, and the unit test suite all pass.

## Deviations from the plan

- None.

## Open questions

- None.
