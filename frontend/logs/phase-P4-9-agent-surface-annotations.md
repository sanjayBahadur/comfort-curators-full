# Phase P4.9 — data-agent annotation pass across drivable routes

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/gpt-5.6-luna
- **Status:** complete

## What I built

Added route-level `useAgentSurface` registrations to the four routes that mount Superhost. IDs are stable, descriptive, and unique per route. Native controls receive the hook ref directly. The shared custom `Select` is represented by its route-local label surface, with `open_panel` invoking the Select trigger by its explicit DOM ID.

No new intent was added. No payment UI was registered, and the existing `PaymentBoundary` scope in `stay.tsx` was left unchanged.

## Files added or changed

- **Added** `logs/phase-P4-9-agent-surface-annotations.md`
- **Modified** `app/src/routes/dashboard.tsx`
- **Modified** `app/src/routes/ops-tickets.tsx`
- **Modified** `app/src/routes/ops-ticket-detail.tsx`
- **Modified** `app/src/routes/stay.tsx`

## Elements registered, by route

### `/dashboard`

- `dashboard-package-link` — first property’s package link; `focus`, `click`, `scroll_to`.
- `dashboard-retry` — owner dashboard retry button in the error state; `focus`, `click`.

### `/ops/tickets`

- `ops-tickets-new-ticket` — NEW TICKET header link; `focus`, `click`, `scroll_to`.
- `ops-tickets-property-filter` — property filter surface; `scroll_to`, `open_panel`.
- `ops-tickets-type-filter` — ticket type filter surface; `scroll_to`, `open_panel`.
- `ops-tickets-status-filter` — ticket status filter surface; `scroll_to`, `open_panel`.

### `/ops/tickets/:ticketId`

- `ops-ticket-detail-new-ticket` — NEW TICKET header link; `focus`, `click`, `scroll_to`.
- `ops-ticket-detail-prepare` — prepare-for-dispatch button when the ticket is preparable; `focus`, `click`.
- `ops-ticket-detail-find-candidates` — find/refresh ranked candidates button; `focus`, `click`.

### `/stay`

- `stay-property-selector` — current-property Select surface; `scroll_to`, `open_panel`.
- `stay-help-ticket-type` — request type native select; `focus`, `click`, `scroll_to`.
- `stay-help-reason` — help reason textarea; `focus`, `set`, `scroll_to`.
- `stay-help-window-start` — requested-window start input; `focus`, `set`.
- `stay-help-window-end` — requested-window end input; `focus`, `set`.
- `stay-help-submit` — send-help-request button; `focus`, `click`.

## Decisions I made

- Registered the dashboard package link as the primary owner action rather than annotating static metrics or every property card.
- Kept ticket filters to `scroll_to` and `open_panel`; the custom Select does not accept a ref or expose a native value setter, so `set` would not be truthful.
- Added requested-window inputs on `/stay` because filling a help request plausibly includes its time window.
- The current ticket-detail implementation has no reply/note text field or status select. Its real operational controls are preparation, candidate evaluation, and assignment; dynamic assignment buttons were not individually registered because hook calls cannot be made inside the candidate map.
- Route cleanup remains provided by the existing `useAgentSurface` effect cleanup. IDs are route-specific, and React Router mounts only one route at a time in `main.tsx`.

## What did NOT work

- The first build/lint attempt could not start because this checkout had no `node_modules`. `npm ci` installed the locked dependencies; the required commands then passed.
- There is no browser executable in the sandbox. Vite served `200` SPA responses for `/dashboard`, `/ops/tickets`, `/ops/tickets/example`, and `/stay`, but I could not run a browser navigation to inspect live DOM registration or click interactions.
- There is no live backend. Therefore dashboard and queue/detail data-backed success states, populated property selectors, populated ticket filters, candidate lists, and Superhost thread connection could not be reached. The source-reachable loading/error/empty branches were reviewed; no claim of live interaction verification is made.

## Deviations from the plan

- The requested detail-route reply/note form does not exist in the current route, so no nonexistent control was added.
- Custom Selects use route-local wrapper registrations rather than changes to the shared Select component, which was outside this annotation-only block.

## Open questions

- If the ticket-detail route later gains a reply/note form or a user-selectable status control, it should receive its own native registrations in that route.
- If model-driven testing requires `ui.set_value` on custom Selects, the shared Select component will need an explicit ref/value adapter; adding another intent is not warranted.
