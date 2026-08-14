# Phase 5 — Owner Dashboard

- **Date:** 2026-08-08
- **Agent/model:** GPT-5.6 Codex (orchestrator); DeepSeek V4 Flash (API audit and host acceptance); Qwen 3.8 Max (bounded UX audit attempt); DeepSeek V4 Pro (read-only final review)
- **Status:** complete

## What I built
The owner dashboard now exists at `/dashboard`. An authenticated owner session enters it from `/` or the access desk, while other roles remain outside the route.

The dashboard is a calm, exception-first medium collage. Two owner-visible property summaries show lifecycle state, the three real readiness checks, and the next committed work item in the coming seven days. The attention panel combines the backend's owner-visible reporting exceptions with explicit property readiness, lifecycle, package, and document concerns. When none exist it uses the required calm copy and renders no red attention treatment.

Each property's package panel selects only the explicit active version and displays the backend's exact monthly price. The current-month contribution panel uses the live period-bounded reporting endpoint rather than presenting the all-time report as monthly. Documents use their real titles, statuses, and added timestamps. The operating-standards section separates what Comfort Curators controls from what the owner controls without claiming a platform designation or inventing unsupported KPIs.

The live browser acceptance passed 12/12 assertions against the reset acceptance backend at 1440px and 390px: owner routing, two property summaries, active/readiness state, exact empty attention copy, zero red when quiet, exact active-package prices, honest operating boundary, committed-work filtering, first-statement copy, zero radius/shadow, restrained grain, responsive one-column layout, and a clean runtime. Desktop and mobile screenshots were inspected at original resolution.

## Files added or changed
`app/src/lib/api/dashboard.ts` — typed owner property, readiness, exception, document, contribution, package, and per-property work aggregation.

`app/src/routes/dashboard.tsx` — owner gate, property summaries, attention, next-seven-days, active packages, contribution, documents, and operating standards.

`app/src/routes/dashboard.css` — ruled editorial dashboard system with medium collage, quiet empty states, responsive reflow, and no soft UI treatment.

`app/src/routes/entry-route.tsx` — sends an existing owner session at `/` to the dashboard while retaining the staff operations destination.

`app/src/main.tsx` — registers `/dashboard`.

`app/src/routes/login.tsx` — adds the owner-dashboard entry action.

`INTEGRATION.md` — records the live owner-exception, period contribution, all-time contribution, package, document, and KPI limitations.

`logs/DECISIONS.md` — records the cross-phase owner exception, monthly reporting, active package, and standards decisions.

## Decisions I made
Owner attention starts with the verified reporting exception feed, then adds only explicit property concerns: a non-active lifecycle state, incomplete readiness, no active package, or an expired/revoked document. Every reported exception must still be marked owner-visible and belong to a property in the authenticated owner's property list.

Upcoming work requests tickets per visible property because the halted backend effectively requires `property_id`. It includes only approved, scheduled, assigned, or in-progress work whose requested window begins within seven days. Draft, proposed, rejected, cancelled, and closed workflow noise is excluded, along with worker details, raw identifiers, and internal reasons.

The current-month period uses UTC month boundaries because the reporting endpoint accepts RFC3339 bounds. Package pricing uses strict `status === "active"` selection and the server's `monthly_cost_minor_units` and `currency` without client-side arithmetic.

Operating standards state the responsibility boundary and report only evidence actually available. No percentage, response time, resolution duration, rework rate, or platform-designation claim is fabricated.

## What did NOT work
The broad Qwen 3.8 Max UX run did not complete a useful synthesis within its bounded window and was stopped. Its partial observations were treated only as advisory; the implementation was checked directly against the product documents and inspected at desktop and mobile sizes.

The first static build found one unused type left after the upcoming-work implementation was simplified. Removing it restored a clean TypeScript build.

Codex's restricted Chromium process cannot establish the required crashpad socket. The deterministic browser harness ran through the authorized OpenCode host environment, where DeepSeek V4 Flash exercised the live backend and captured both viewports.

The initial API specialist could not find an existing reusable bearer token and therefore source-verified protected contracts after confirming their unauthenticated 401 behavior. The orchestrator then created a temporary acceptance-only owner session and live-verified every Phase 5 read endpoint without mutating domain data. The large raw contract output was captured outside the repository.

## Deviations from the plan
None. The live reporting service exposed a more precise period-bounded contribution endpoint than the older integration table recorded; using and documenting that verified contract keeps the planned “This month” panel truthful.

## New API knowledge
The owner-exception feed can return `items: null` and already suppresses routine internal records. Period contribution is available under `/v1/reporting/property-contribution` with explicit RFC3339 boundaries, while `/v1/reports/property-contribution` is all-time and collection-shaped. Documents also return `items: null` when empty. Package versions require strict active-status selection. There is no aggregate operating-standards KPI endpoint.

## How to verify (human runs these)
1. Run `cd ~/comfort-curators-frontend/app && npm run lint && npm run build`. Expected: both commands exit zero.
2. Run `npm run dev`, open `http://127.0.0.1:3000/login`, and choose OWNER. Expected: `OPEN OWNER DASHBOARD` appears and opens `/dashboard`; reopening `/` with that session also lands on `/dashboard`.
3. Confirm both Lucknow properties appear. Expected: each shows `active`, readiness `3 / 3`, and all three readiness checks as ready.
4. Inspect `NEEDS YOUR ATTENTION`. Expected with the Phase 2 seed: `No exceptions. We’ll surface anything that needs your decision.` and no red warning treatment.
5. Compare `YOUR PACKAGE` with each property's active package response. Expected: Hazratganj Studio shows ₹2,670.00 per month for active V15 and Gomti Riverside 2BHK shows ₹32,350.00 per month for active V1 on the current seed.
6. Inspect `NEXT SEVEN DAYS`. Expected: only committed owner-visible work inside the period appears; there are no draft items, raw IDs, worker details, internal reasons, or state-history controls.
7. Inspect `THIS MONTH` and `DOCUMENTS`. Expected on the current seed: the first-statement and no-documents empty states appear without invented activity.
8. Set the viewport to 390px. Expected: property cards, dashboard panels, and the two operating-standard columns become a single readable column with no horizontal scroll.

## Open questions for the human

## What's next
Phase 5 is complete. Stop at this boundary for manual approval. After approval, begin Phase 6 from `PHASES.md`; do not extend the dashboard speculatively before that phase is restated.
