# Phase P5.3 — /properties/:propertyId

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/gpt-5.6-luna
- **Status:** complete

## What I built

Built the owner-facing property detail page with:

- Service address, lifecycle state, and a prominent link to the inventory package route.
- Readiness checklist sourced from the property response.
- High-visibility compliance hold banner and hold list when the API returns holds, plus a real clear state.
- Maximum occupancy, timezone, access method, and emergency contacts from the property response.
- Lifecycle transition history from `GET /v1/properties/{id}/transitions`, with an honest empty state.
- Loading skeleton, API error/retry state, missing-id redirect, and populated state.
- Responsive editorial styling matching the existing owner dashboard and onboarding surfaces.

## Files added or changed

- Added `app/src/lib/api/property.ts` for the property detail and transition API shapes/load.
- Added `app/src/routes/property-detail.tsx`.
- Added `app/src/routes/property-detail.css`.
- Added `logs/phase-P5-3-property-detail.md`.

## Decisions I made

- Kept the page read-only because the available detail and transition endpoints are reads; lifecycle changes remain an ops concern.
- Made compliance holds a full-width red alert because they are owner-relevant blockers and must not be buried in ordinary metadata.
- Used the existing ticket state-history visual language for property transitions, adapted to property lifecycle fields.
- Used the package route as the primary action because it is the existing child surface and the inventory shop is a P0 owner flow.
- Rendered optional access and emergency-contact data with explicit empty copy rather than inventing defaults.
- Compliance hold copy prefers returned `message`, `code`, and `status` values, with a neutral fallback for an otherwise-shaped returned hold object.

## What did NOT work

- The first build and lint attempts could not start because `app/node_modules` was absent (`tsc` and `oxlint` were unavailable). `npm ci` restored the pinned dependencies.

## Deviations from the plan

- The backend source checkout was not present in this workspace, so the property client used the verified response details in `INTEGRATION.md` and existing frontend API conventions. The implementation remains defensive for optional hold/contact fields.
- Per task instructions, `main.tsx` was not modified. The route registration is supplied below for the orchestrator.

## Open questions

- Confirm the exact backend `compliance_holds` object fields during integration testing if the live service adds a structured field beyond `code`, `message`, or `status`; the page currently preserves the safe generic fallback.

## Route patch for the orchestrator

Import:

```tsx
import PropertyDetail from "./routes/property-detail";
```

Route:

```tsx
<Route path="/properties/:propertyId" element={<RequireRole allow={["owner"]}><PropertyDetail /></RequireRole>} />
```
