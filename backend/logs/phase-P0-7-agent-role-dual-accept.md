# Phase P0.7 — agent role dual-accept (jarvis + superhost)

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/deepseek-v4-pro
- **Status:** complete

## What I built

Added `"superhost"` as a recognized IAM agent role alongside `"jarvis"` across the
IAM, compliance, and reservations packages. This is strictly additive — `"jarvis"`
remains accepted everywhere it was accepted before. Both role strings now trigger
the same denial checks in compliance and reservations.

## Files added or changed

- `internal/iam/models.go` — `ValidRole()` now accepts `"superhost"` in the case list alongside `"jarvis"`.
- `internal/compliance/models.go` — added `RoleSuperhost = "superhost"` constant; renamed `ErrJarvisDenied` → `ErrSuperhostDenied`, updated message to `"superhost cannot clear compliance holds"`.
- `internal/reservations/models.go` — added `RoleSuperhost = "superhost"` constant; renamed `ErrJarvisCannotMutate` → `ErrSuperhostCannotMutate`, updated message to `"superhost cannot mutate external calendars"`.
- `internal/compliance/handler.go` — made `hasRole` variadic (`targets ...string`); all 3 denial call sites now check both `RoleJarvis` and `RoleSuperhost`; updated inline error messages from `"jarvis cannot ..."` to `"superhost cannot ..."`.
- `internal/reservations/service.go` — made `hasRole` variadic (`targets ...string`); all 4 denial call sites now check both `RoleJarvis` and `RoleSuperhost`.
- `internal/reservations/handler.go` — updated 4 `errors.Is(err, ErrJarvisCannotMutate)` references to `ErrSuperhostCannotMutate`.
- `tests/reservations/service_test.go` — updated error assertions to `ErrSuperhostCannotMutate`; added Superhost CreateFeed and SetFeedStatus denial tests; added Superhost ResolveException denial test.
- `tests/reservations/reservation_test.go` — updated error assertion to `ErrSuperhostCannotMutate`; added Superhost ResolveConflict denial test.
- `tests/iam_test.go` — added `ValidRole("jarvis")` and `ValidRole("superhost")` assertions.

## Decisions I made

- **Variadic `hasRole`**. Both `internal/compliance/handler.go` and `internal/reservations/service.go` had identical `hasRole(roles, target)` signatures. I changed both to `hasRole(roles, targets ...string)` so call sites become `hasRole(roles, RoleJarvis, RoleSuperhost)` instead of duplicating `|| hasRole(roles, RoleSuperhost)` seven times. This is a small structural change to a package-private helper, not a new public API.
- **Kept jarvis tests intact**. Existing tests with `[]string{reservations.RoleJarvis}` are unchanged — they still prove backward compatibility. New test assertions with `[]string{reservations.RoleSuperhost}` prove the forward path.
- **`ErrSuperhostDenied` is defined but unused in service code**. The compliance denial checks are at the HTTP handler level (inline `writeError` calls), not the service level. `ErrSuperhostDenied` is defined in `models.go` for use by any future callers that need typed error matching, but it is not currently referenced anywhere in the service. The reservations equivalent `ErrSuperhostCannotMutate` is actively used in all 4 service methods and in 4 handler error-matching sites.

## What did NOT work

Nothing failed. Build, vet, and all 30 test packages pass.

## Deviations from the plan

None.

## Open questions

- **Compliance handler denial tests**: The existing `TestJarvisCannotClearHolds` in `tests/compliance/compliance_test.go` does not test the handler-level `hasRole` denial (it tests `ScanExpired` at the service level). Adding real HTTP handler-level denial tests for the compliance package would require a full HTTP server setup. The reservations denial tests cover the pattern at the service level (CreateFeed, SetFeedStatus, ResolveException, ResolveConflict all reject both roles and are verified by tests).
- **Acceptance probes**: `tests/acceptance/probes.go:1951` and `:1979` check for `"jarvis cannot mutate"` with a fallback to `"FORBIDDEN"`. Since the error message text changed to `"superhost cannot mutate external calendars"`, only the `"FORBIDDEN"` fallback will match. This is verified correct by code inspection — both probes use `&&` fallback to `"FORBIDDEN"` code.
- **`ErrSuperhostDenied` usage**: Defined but not referenced by any service or handler. It exists for typed error matching by any future callers. The compliance handler uses inline error strings for its 3 denial paths.
