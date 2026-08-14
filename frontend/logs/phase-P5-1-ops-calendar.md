# Phase P5.1 — /ops/calendar (DEF-08)

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/gpt-5.6-luna
- **Status:** complete

## What I built

Built the property-scoped `/ops/calendar` operations page with URL-persisted property selection, independent loading/error/empty/populated states for feed health, reservations, and turnover proposals, feed polling, and turnover proposal generation. Real poll and generation result counts are shown inline, and generation invalidates the selected property's proposals query.

## Files added or changed

- `app/src/lib/api/ops.ts`
- `app/src/routes/ops-calendar.tsx`
- `app/src/routes/ops-calendar.css`

## Decisions I made

- Used `calendar-health`'s nested `feed.id` for polling, so no extra calendar-feed list request was needed.
- Used `04 / CALENDAR` after the existing ticket queue, create, and detail sections.
- Reused `Select`, `OpsSkeleton`, `StatusLabel`, `formatWindow`, and `formatMoment` from the established ops patterns.

## What did NOT work

Nothing encountered during implementation.

## Deviations from the plan

No separate `GET /v1/properties/{id}/calendar-feeds` client was added because the required poll identifier is already present in every `calendar-health` feed entry.

## Open questions

None.

## Route patch for the orchestrator

Import: `import OpsCalendar from "./routes/ops-calendar";`

`<Route path="/ops/calendar" element={<RequireRole allow={["staff"]}><OpsCalendar /></RequireRole>} />`
