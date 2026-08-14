# Phase P5.5 — /stay guest portal + store UI (D3)

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/gpt-5.6-luna
- **Status:** complete

## What I built

- Replaced the phase-0 portal implementation with a single-page guest portal containing current stay, house guide, request help, and store sections.
- Added property selection using `getOpsProperties()` because guest-session-to-property resolution does not exist yet.
- Added real reservation loading and an honest active/no-active/error state.
- Surfaced recorded `access_method` and `emergency_contacts`, plus clearly generic emergency reference copy.
- Added guest-scoped help types (`incident`, `restock`, `routine_maintenance`) using the existing `createTicket()` API.
- Added the real store catalog, provider filters, search, cart, quote, and direct human-confirmed order flow against the P3.9 endpoints.
- Provider names are plain mono text only: `INSTAMART`, `ZEPTO`, and `BLINKIT`.
- Cart and quote confirmation use the existing off-centre `Modal` primitive with the established quick-view label convention.

## Files added or changed

- `app/src/routes/stay.tsx`
- `app/src/routes/stay.css`
- `app/src/lib/api/store.ts`

`main.tsx` was intentionally not changed; the route patch is handed off below.

## Decisions I made

- The property selector is required before reservation, catalog, quote, order, or ticket actions can run.
- The order call is only reachable from the guest's `CONFIRM ORDER` button after a live quote; it is not connected to Superhost.
- The order payload uses `VITE_DEMO_TENANT_ID`, matching the existing demo session configuration. The confirm action is disabled when that value is absent.
- Guest help exposes only incident, restock, and routine maintenance because the remaining operations ticket types are internal workflow categories rather than guest-facing requests.

## What did NOT work

- The first build/lint attempt could not start because dependencies were not installed. `npm ci` installed the lockfile dependencies; the requested build and lint then passed.

## Deviations from the plan

- No route import change was made directly because the block instructions reserve that patch for the orchestrator. The exact replacement is recorded below.

## Open questions

- The backend needs a real guest-session/reservation link so the guest's current property can be derived automatically instead of selected.
- The order endpoint requires `tenant_id`; this implementation uses the existing demo tenant environment value until a session-derived tenant identity is exposed.

## Route patch for the orchestrator

In `app/src/main.tsx`:

```diff
-import Home from "./routes/home";
+import Stay from "./routes/stay";
@@
-<Route path="/stay" element={<RequireRole allow={["guest"]}><Home /></RequireRole>} />
+<Route path="/stay" element={<RequireRole allow={["guest"]}><Stay /></RequireRole>} />
```
