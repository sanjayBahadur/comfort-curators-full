# Phase P2.4 — router assembly

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/gpt-5.6-luna
- **Status:** complete

## What I built

- Composed `/` as the single entry point: an authenticated role is sent to `homeFor(role)`, while an unauthenticated session receives the existing `Intro` flow.
- Wrapped every protected route in `RequireRole` with the role allow-list specified by `roles.ts` and the route contract.
- Added the public `/expansion` route.
- Added the existing guest home placeholder at `/stay`, guarded for the `guest` role, so every `homeFor` destination resolves to a route.

## Files added or changed

- `app/src/main.tsx`
- `app/src/routes/entry-route.tsx`
- `logs/phase-P2-4-router-assembly.md`

## Decisions I made

- Kept `EntryRoute` as the composition point for `/` rather than duplicating auth logic in `main.tsx`. It now handles the authenticated `homeFor` redirect and renders `Intro` only when no token exists; a token without a role still returns to `/login`.
- Used `RequireRole` unchanged so missing sessions redirect to `/login` and authenticated disallowed roles redirect to their own `homeFor(role)`. Therefore an owner visiting `/ops/tickets` goes to `/dashboard`, not `/login`.
- Used `allow={["owner"]}` for the property package route because `roles.ts`'s `isPropertyPath` rule makes that route owner-reachable.
- Did not render `navFor(role)` in this block. There is no shared shell: owner, curator, and operations routes render separate headers, and `OpsHeader` is staff-specific. Nav integration belongs in a later surface/layout block.

## What did NOT work

- Nothing known at implementation time.

## Deviations from the plan

- None.

## Open questions

- Role-scoped navigation still needs a shared navigation/layout decision before it can consume `navFor(role)` consistently across the independently rendered surfaces.
