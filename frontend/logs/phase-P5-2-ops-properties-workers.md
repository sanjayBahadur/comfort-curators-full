# Phase P5.2 — /ops/properties, /ops/workers

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/gpt-5.6-luna
- **Status:** complete

## What I built

- Built `/ops/properties` with loading, error, empty, and populated states.
- Rendered property name, address, state, open ticket count, and explicit `—` values for readiness and active package because those fields are not available in the reachable frontend API contract.
- Built `/ops/workers` with loading, error, empty, and populated roster states.
- Added a worker detail drawer with real zone, skills, classification, identity verification, and current assignments found in the loaded ticket queue.
- Explicitly marked availability windows and assignment history as unavailable rather than fabricating them.

## Files added or changed

- `app/src/routes/ops-format.ts`
- `app/src/routes/ops-properties.tsx`
- `app/src/routes/ops-workers.tsx`
- `app/src/routes/ops-rosters.css`
- `logs/phase-P5-2-ops-properties-workers.md`

## Decisions I made

- Used `getTicketQueue()` for both pages. It already fetches the property and worker collections and includes each ticket's assignments, so it provides the real cross-resource data needed for open-ticket counts and the worker drawer without inventing a new endpoint.
- Counted open tickets as tickets whose status is not `closed`, `cancelled`, or `rejected`; all counted statuses are real ticket response fields.
- Left lifecycle transitions off the property list. The screen is a scannable list and the transition action belongs on `/properties/:id`, where the property context can be shown safely.
- Used a table for workers to match the established ticket queue pattern, with keyboard-accessible row activation and a detail drawer.

## What did NOT work

Nothing encountered during implementation.

## Deviations from the plan

- The host backend repository path supplied in the task was not mounted in this sandbox, so backend model and availability searches could not be performed. I kept the existing frontend types and rendered unavailable fields as `—`.
- No package/subscription endpoint was present in the reachable `src/lib/api` surface, so active package is `—`.

## Open questions

- Which backend property field supplies readiness, and should it be added to `OpsPropertyData`?
- Which endpoint and response shape supplies property package/subscription data?
- Does the workforce API expose availability windows, and is there a worker-scoped assignment history endpoint?

## Route patches for the orchestrator

Imports for `main.tsx`:

```tsx
import OpsProperties from "./routes/ops-properties";
import OpsWorkers from "./routes/ops-workers";
```

```tsx
<Route path="/ops/properties" element={<RequireRole allow={["staff"]}><OpsProperties /></RequireRole>} />
<Route path="/ops/workers" element={<RequireRole allow={["staff"]}><OpsWorkers /></RequireRole>} />
```
