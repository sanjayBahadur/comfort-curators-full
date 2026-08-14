# Phase UI 1.3.1 — Calendar portfolio overview

- **Date:** 2026-08-13
- **Agent/model:** GPT-5
- **Status:** complete

## What I built
Made `/ops/calendar` useful without requiring a property selection. The default `ALL PROPERTIES` scope gathers the existing reservation, calendar-health, and turnover-proposal data for each account property, keeps the month grid visible even when there are no records, and adds a compact Superhost-themed occupancy overview beside the property selector on desktop. Selecting one property scopes the same calendar and summary to that property. Reservation windows use quiet grey hatching and open an accessible stay-detail modal without displacing the calendar.

## Files added or changed
`app/src/routes/ops-calendar.tsx` — adds safe portfolio aggregation, partial-failure reporting, always-visible calendar rendering, and computed occupancy/feed summary values.

`app/src/routes/ops-calendar.css` — adds the black and phosphor-green overview terminal beside the selector with responsive metric cells.

The calendar-only property selector uses a quiet light-grey surface and fills the full height of the overview band, avoiding an empty strip beneath the control.

`app/src/components/calendar/CalendarGrid.tsx` — replaces the expanding reservation detail rail with the existing accessible modal and resolves property context in portfolio mode.

`app/src/components/calendar/CalendarGrid.css` — gives reserved windows a quieter grey hatched treatment, protects date visibility, and styles the stay-detail information grid.

The calendar distinguishes time states without inventing reservation states: past stays are lightly hatched, upcoming and backend `active` stays use stronger neutral grey hatching, and a stay active at the current instant receives a dark border and green live dot. Today uses a restrained ink frame and neutral tint. Reservation labels are one consistently aligned strip with a green active tag.

`logs/phase-ui-1.3.1-calendar-portfolio-overview.md` — records the implementation and data-honesty decisions.

## Decisions I made
`ALL PROPERTIES` is the default because it gives operators an account-level view without silently choosing a property on their behalf. It uses only the existing property-scoped endpoints and combines their responses in the client.

`OCCUPIED NOW` means the percentage of scoped properties that have at least one active, non-cancelled reservation window at the current time. It is not a guest-capacity percentage or an occupancy forecast. If one property request fails while another succeeds, available records remain visible and the terminal reports how many property data sources are unavailable. If every scoped request fails, the existing error treatment is shown.

Generating turnover proposals remains property-only because the existing mutation requires one explicit property ID. Portfolio mode displays proposals but does not perform a multi-property write.

## What did NOT work
none

## Deviations from the plan
none

## New API knowledge
none

## How to verify (human runs these)
Run `cd ~/open-code-projects/ComfortCurators/app && npm run build`. Expected: TypeScript and the production bundle complete successfully.

Run `cd ~/open-code-projects/ComfortCurators/app && npx vitest run`. Expected: the configured test suite passes. There is no `npm test` script in this repository.

Open `/ops/calendar` without a `property` URL parameter. Expected: `ALL PROPERTIES` is selected; the overview reports portfolio totals; the month calendar cells are visible even with zero reservations; feeds and proposals show aggregated account data.

Choose a property. Expected: the URL gains the property parameter and every summary, calendar, feed, and proposal section reflects only that property. The proposal-generation button appears only in this explicit scope.

Test an account with no properties. Expected: the summary reports zero properties and the blank month grid remains usable for navigation without making invalid property requests.

## Open questions for the human

## What's next
Visually inspect the terminal metric density and calendar hierarchy at the target desktop and mobile widths before committing this phase.
