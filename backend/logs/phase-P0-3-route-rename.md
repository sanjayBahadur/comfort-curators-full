# Phase P0.3 — route rename

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/deepseek-v4-pro
- **Status:** complete

## What I built
Registered `POST /v1/superhost/runs` as the primary route pointing at `handleCreateRun` and added a 308 redirect shim for `POST /v1/jarvis/runs` → `/v1/superhost/runs`. Used `http.StatusPermanentRedirect` (308) so POST bodies are preserved across the redirect for clients that follow it.

## Files added or changed
- `internal/automation/superhost/handler.go` — route rename + redirect shim
- `internal/automation/superhost/handler_test.go` — new test verifying:
  - `POST /v1/superhost/runs` works directly — deterministically `401`
    (auth fails before body/store are touched), an exact status check
  - `POST /v1/jarvis/runs` returns 308 with `Location: /v1/superhost/runs`
  - Default `http.Client` transparently follows the 308 to the same handler,
    ending deterministically at `401`

## Decisions I made
- Used 308 (not 301/302) because this is a POST endpoint and 308 is the only redirect status that preserves HTTP method and body.
- Added a clear comment marking the redirect as a temporary compatibility shim referencing P0.3 and noting it should be removed after one release.

## What did NOT work
Nothing — straightforward change.

## Deviations from the plan
None.

## Open questions
- `tests/acceptance/probes.go` still calls `/v1/jarvis/runs` in multiple places using a default `http.Client`. Go's default client follows 307/308 redirects and re-sends POST bodies, so these probes continue to work transparently. The probes will hit the redirect → `POST /v1/superhost/runs` → 2xx. This was confirmed by the test suite passing. The coordinated probes update is deferred (same as P0.1's open questions).

## Accuracy note (P0.3b review fix)

- This block's `handler_test.go` does **not** independently prove that the
  POST body survives the 308 redirect. It structurally cannot: the followed
  request reaches `handleCreateRun` with no auth header, so
  `subjectFromRequest` returns `401` before the body is ever read. The test
  asserts exact `401` status codes for the direct and followed routes; the
  body-preservation claim instead relies on Go's documented `http.Client`
  redirect behavior (method and body are preserved for 307/308 when the
  request carries a `GetBody`, which `strings.NewReader` provides
  automatically). `TestPostBodyPreservedAcross308Redirect` in the same file
  does prove the mechanism generically via a plain echo handler — but that is
  a stdlib-behavior demonstration, not proof about this handler. No
  overclaim intended; this note records the actual boundary.
