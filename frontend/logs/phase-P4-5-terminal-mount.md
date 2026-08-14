# Phase P4.5 — mount terminal on /dashboard, /ops/tickets, /ops/tickets/:id, /stay

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/gpt-5.6-luna
- **Status:** complete

## What I built

Added a shared `SuperhostMount` wrapper that creates an idempotent, property-scoped thread, starts the authenticated SSE stream from the returned `thread_id`, maps live events into the existing terminal surface, and renders pending approval requests with `ConfirmBlock`.

The wrapper reports missing property context, missing demo tenant configuration, thread creation failures, ended streams, and stream errors without presenting them as connected states.

## Files added or changed

- Added `app/src/components/superhost/SuperhostMount.tsx`.
- Added `app/src/components/superhost/SuperhostMount.css`.
- Mounted the wrapper in `app/src/routes/dashboard.tsx`, `ops-tickets.tsx`, `ops-ticket-detail.tsx`, and `stay.tsx`.

## Decisions I made

- Reused `VITE_DEMO_TENANT_ID`, matching the existing stay order flow.
- Used deterministic keys in the form `superhost-terminal:<route>:<property_id>` so remounts retry idempotently while route/property scopes remain distinct.
- Used the existing queue property filter, stay property selector, and ticket-detail property.
- Scoped dashboard terminal creation only when the dashboard has exactly one property; a multi-property dashboard stays visibly unconnected rather than inventing an all-properties scope.
- Kept the terminal as a normal page block, not a floating widget.

## What did NOT work

- The first build and lint attempt could not run because dependencies were not installed. `npm ci` restored the lockfile dependencies; both checks then passed.

## Deviations from the plan

- The backend contract file path was not reachable from this sandbox, so the frozen contract excerpt supplied in the task was used.
- No live backend walkthrough was possible in this environment; the implementation was verified by TypeScript build and oxlint.

## Open questions

- A live acceptance run should verify the backend's idempotent `POST /v1/superhost/threads` retry behavior and the exact event payloads for approval/error cases.
