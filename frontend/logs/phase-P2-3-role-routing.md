# Phase P2.3 — role routing

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/gpt-5.6-luna
- **Status:** complete

## What I built

- Added the role routing source of truth: `homeFor`, `navFor`, and `allows`.
- Added the exported `RequireRole` component. Unauthenticated users go to `/login`; signed-in users with a disallowed role go to their own `homeFor(role)` destination.
- Declared the intended owner, staff, and guest navigation structures, including routes that will be added by later P5 blocks.

## Files added or changed

- Added `app/src/lib/auth/roles.ts`.
- Added `app/src/components/RequireRole.tsx`.
- Added this phase log at `logs/phase-P2-3-role-routing.md`.
- Did not change `app/src/routes/entry-route.tsx` or `app/src/main.tsx`; router assembly and the final entry flow belong to P2.4.

## Decisions I made

- Kept role definitions in the existing session module and imported its `Role` type without changing session behavior.
- Used `/properties/:propertyId/package` as the intended Package navigation target and matched dynamic property paths in `allows`.
- Kept `/stay` as `homeFor("guest")` even though that route does not exist yet; P5.5 owns the guest portal.
- Kept the guard in `src/components/` because it is a reusable route-element boundary while the role policy remains in `src/lib/auth/roles.ts`.

## What did NOT work

- The first build and lint attempts were blocked because dependencies were not installed (`tsc` and `oxlint` were unavailable). After `npm ci`, both commands passed.

## Deviations from the plan

- None. `entry-route.tsx` was intentionally left for P2.4 as directed.

## Open questions

- None for this block.
