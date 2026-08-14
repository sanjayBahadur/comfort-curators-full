# Phase P1.6 — select/dialog replacement

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/gpt-5.6-luna
- **Status:** complete

## What I built
Replaced all 15 native `<select>` controls with the shared `Select` primitive and replaced the package-shop quick-view `<dialog>` with the shared `Modal` primitive.

## Files added or changed
- `app/src/routes/onboarding.tsx`
- `app/src/routes/ops-tickets.tsx`
- `app/src/routes/ops-ticket-new.tsx`
- `app/src/routes/package-shop.tsx`
- `logs/phase-P1-6-select-dialog-replacement.md`

## Decisions I made
- Kept onboarding's nine named controls uncontrolled with `name` and `defaultValue`, so `Select` renders its hidden named input.
- Verified each onboarding submit handler reads `new FormData(event.currentTarget)` and reads the same names (`role`, `property_type`, `primary_goal`, `rental_strategy`, `overall_status`, `communication`, `language`, `access`, and `furnishing`); the hidden inputs therefore preserve the existing API payload path.
- Kept controlled values and callback behavior for the property picker, ticket filters, and new-ticket form.
- Preserved every option value and label, including required validation on the three required controlled controls.
- Let `Modal` own quick-view focus restoration, Escape handling, close control, and visibility from the existing `quickView` item state.

## What did NOT work
The first build and lint attempts were blocked because dependencies were not installed (`tsc` and `oxlint` were unavailable). Running `npm ci` from `app/` installed the lockfile dependencies; the requested checks then passed.

## Deviations from the plan
None. No shared primitive or `src/index.css` was changed.

## Open questions
None.
