# Phase P0.7b — compliance handler HTTP-level authorization tests

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/deepseek-v4-pro
- **Status:** complete

## What I built

HTTP-level authorization tests for the three role-gated endpoints in
`internal/compliance/handler.go` (`handleCreateItem`, `handleRenewItem`,
`handleScanExpiry`), closing the gap identified by the independent P0.7
review: the earlier compliance integration tests only exercised the service
layer, never the handler-level `hasRole` gate.

Three test functions in a new `internal/compliance/handler_test.go`
(package `compliance`, since the handler and `hasRole` are unexported):

- `TestComplianceHandlerRoleDenied` — table-driven, 6 cases (3 endpoints ×
  2 roles), proves that `jarvis` and `superhost` both receive `403
  FORBIDDEN` at the handler level. Uses a `nil` service because the role
  check fires before any service call.
- `TestComplianceHandlerRoleAllowedCreateAndRenew` — proves that an
  `"owner"` role passes the gate on `POST /v1/compliance/items` and
  `POST /v1/compliance/items/{item_id}/renew` (the request subsequently
  fails on body validation, but demonstrably not at the role gate).
- `TestComplianceHandlerRoleAllowedScanExpiry` — same for
  `POST /v1/compliance/scan-expiry`. This endpoint has no body to parse,
  so a real `ComplianceService` with a connected pool is used; the test
  skips when PostgreSQL is unavailable.

## Files added or changed

- `internal/compliance/handler_test.go` — new file, 200 lines.
- `logs/phase-P0-7b-compliance-handler-tests.md` — this file.

## Decisions I made

- Followed the existing `iam.WithSubject(ctx, security.Subject{...})`
  pattern from `tests/onboarding/handler_test.go` rather than inventing a
  new injection mechanism.
- Used `ComplianceHandler` with a `nil` service for the denial tests and
  the create/renew allowed tests, since the handler returns before any
  service call when denied, and body parsing fails before the service call
  for the allowed cases.
- For `handleScanExpiry`'s allowed case, used a full `ComplianceService`
  backed by a real pool because the handler proceeds directly to
  `svc.ScanExpired()` after the role check with no intermediate body
  validation gate.
- Did not touch `handler.go`, `tests/compliance/compliance_test.go`,
  `internal/reservations/`, or `contracts/`.

## What did NOT work

- Nothing. `go build ./...`, `go vet ./...`, `go test -p 1
  ./internal/compliance/...` all pass. The scan-expiry non-denied test
  skips in environments without PostgreSQL (the denial and create/renew
  tests all pass without a database).

## Deviations from the plan

- None.

## Open questions

- None.
