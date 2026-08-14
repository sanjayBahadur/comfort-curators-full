# UI 1.3.1 — Branch documentation and release notes

- **Branch:** `ui-1.3.1`
- **Date:** 2026-08-13
- **Status:** complete
- **Purpose:** further UI polish after the `v1.3` visual release

## Summary

This branch improves the application navigation, curator field experience, operations ticket search, portfolio calendar, guest property selection, owner dashboard, and owner onboarding. The work preserves the existing backend contracts and the site's editorial black-and-white visual system while using Superhost green only for connected, active, or confirmed states.

No backend files were modified. No live Airbnb integration, geocoding service, generative queue search, automated pricing feed, or GPS tracking capability was added or implied.

## Commit chronology

| Commit | Change |
| --- | --- |
| `6b29f1e5` | Unified application navbar spacing across operations, owner, and curator routes. |
| `dd21a995` | Restaged the curator zone map in Noida and improved map interaction and nearby-work navigation. |
| `1ebc8608` | Added the natural-language ticket queue finder. |
| `cbc6b9f1` | Added the portfolio calendar overview and reservation-detail treatment. |
| `2addf729` | Added the owner property command dashboard and safe `/stay` default property selection. |
| Current branch closeout | Adds the property-handover onboarding dossier, document drop zone, and owner map/navbar stacking correction. |

## Route-by-route changes

### Shared application navigation

Affected routes include `/ops/*`, `/dashboard`, `/onboarding`, `/properties/*`, `/invoices`, `/documents`, and `/jobs/*`.

- Separated numbered page context from navigation entries so labels no longer collide with the first nav item.
- Standardized back-button and wordmark clearance across curator pages.
- Removed the unwanted left partition while preserving the existing active black navigation state.
- Kept `/expansion` outside this application-navbar change.
- Added scroll-linked job-card entrances on the curator jobs page with reduced-motion fallback.

### `/jobs/map`

- Moved the frontend-only demonstration coordinates to Noida without changing stored backend addresses.
- Replaced generic circular pins with locally bundled home, wrench, and navigation glyphs.
- Preserved pan and zoom when selecting map items by keeping one Leaflet instance alive.
- Made page scrolling the safe default; map zoom requires an explicit mode toggle.
- Added `NAVIGATE NEARBY WORKS`, plotting up to five distance-ranked demo routes and contained estimate cards.
- Uses Debug-theme green for routes and controls; the selected route becomes darker and solid while alternatives remain lighter and dotted.
- Kept the global Superhost drawer above Leaflet's stacking range.

### `/ops/tickets`

- Replaced the aggressive full inline terminal with a compact Superhost-styled queue finder.
- Supports plain-language combinations of loaded property names, ticket types, and existing workflow statuses.
- Updates the real URL filters, dropdowns, and queue table.
- Clearly labels the feature as a deterministic local interpreter rather than a generative model.
- Uses `Use human language to search` as the calm user-facing prompt.

### `/ops/calendar`

- Defaults to `ALL PROPERTIES` and keeps the calendar visible without a selected property or reservations.
- Aggregates existing property-scoped reservations, health records, and proposals into a portfolio view.
- Places a compact Superhost occupancy overview beside the property selector on desktop.
- Uses neutral grey reservation hatching, visible date labels, a green active tag, and an accessible reservation modal.
- Retains explicit single-property scope for turnover proposal writes.

### `/stay`

- Selects the first real account property after a successful property query.
- Repairs an invalid selection to the first valid property.
- Leaves accounts with no properties safely unselected and avoids invalid dependent requests.

### `/dashboard`

- Added a selected-property carousel and synchronized current-property context.
- Added record-derived ticket, package, document, contribution, reservation, and readiness metrics.
- Added live CARTO/OpenStreetMap tiles with an honestly labeled approximate address marker.
- Added a compact property reservation calendar and essential property links.
- Replaced the generic Superhost mount with one continuous task checklist.
- Active items expand to show recorded context.
- Proposed tickets expose a real `proposed → approved` transition with an owner-dashboard audit reason.
- Draft tickets remain non-approvable until proposed.
- Raised the sticky owner navbar above Leaflet's 1000-level controls and isolated the map panel stacking context.

### `/onboarding`

- Replaced the conventional intake entry with a five-chapter editorial dossier:
  1. Listing
  2. Details
  3. Authority
  4. Documents
  5. Handover
- Uses a full-bleed existing property photograph, ruled architectural specifications, restrained registration-red marks, and green completed-chapter indicators.
- Starts with an explicitly dummy public Airbnb URL; the URL is not fetched in this build.
- Presents a small typographic demo market outlook instead of an embedded terminal.
- Makes permission boundaries explicit: public listing details and selected references may cross the threshold; credentials, messages, payments, account control, and publishing authority do not.
- Adds a large document-blue/manila archival drop zone with real whole-zone browse/drop behavior, drag-active feedback, supported formats, and selected-file count.
- Records only selected filenames as evidence references; file contents are not uploaded.
- Creates server state only at the final handover action, using the existing property and onboarding-case APIs.
- Seeds property basics and goals while leaving authority, ownership, safety, inspection, package, and contract checks incomplete.
- Direct visits now show the dossier instead of silently auto-resuming; saved cases remain accessible in its first chapter.

## Design system impact

- Instrument Serif remains the voice for human and editorial declarations.
- Archivo Black is reserved for decisive operational headings and metrics.
- JetBrains Mono carries provenance, evidence, workflow state, and navigation context.
- Warm paper, ink, neutral rules, restrained red, and Superhost green remain the primary palette.
- New document-blue and manila tones are limited to the file handover zone, where they provide a familiar paper/archive cue.
- Responsive behavior was added or refined at the relevant desktop, tablet, and mobile breakpoints.
- Motion-sensitive interactions provide reduced-motion fallbacks.

## Data and capability boundaries

- No backend changes were made.
- Owner dashboard metrics are computed from existing API records.
- Owner approval uses the existing ticket transition endpoint and valid workflow order.
- Map coordinates and nearby travel estimates are demonstrative, not geocoded or GPS-tracked.
- Queue search is a local term interpreter, not AI inference.
- Calendar portfolio mode aggregates existing per-property endpoints in the client.
- Airbnb lookup, listing photograph, nearby profiles, and pricing outlook are explicitly demonstrative.
- Onboarding file selection stores filename references only; it does not upload bytes.
- Autonomy preferences remain recorded preferences and do not enforce behavior.

## Validation completed

- `cd ~/open-code-projects/ComfortCurators/app && npm run build`
  - Passed after the final onboarding and map-stacking changes.
- `cd ~/open-code-projects/ComfortCurators/app && npx vitest run`
  - 5 test files passed; 27 tests passed.
- `cd ~/open-code-projects/ComfortCurators/app && npm run lint`
  - Completed with eight pre-existing warnings and no new lint errors.
- `git diff --check`
  - Passed with no whitespace errors.

## Phase documentation index

- `logs/phase-ui-1.3.1-ops-navbar-spacing.md`
- `logs/phase-ui-1.3.1-noida-zone-map.md`
- `logs/phase-ui-1.3.1-ticket-queue-finder.md`
- `logs/phase-ui-1.3.1-calendar-portfolio-overview.md`
- `logs/phase-ui-1.3.1-stay-default-property.md`
- `logs/phase-ui-1.3.1-owner-dashboard-command-view.md`
- `logs/phase-ui-1.3.1-owner-map-navbar-stack.md`
- `logs/phase-ui-1.3.1-property-handover-dossier.md`

## Recommended final smoke test

1. Open `/dashboard`; switch properties, inspect the map/calendar, expand a task, and confirm the navbar stays above Leaflet while scrolling.
2. Open `/onboarding`; progress through all five dossier chapters, drag documents onto the zone, and verify the final handover opens the existing server-backed wizard.
3. Open `/ops/tickets`; try `assigned turnovers at Gomti Riverside` and then `clear filters`.
4. Open `/ops/calendar` in portfolio and single-property scopes; open a reservation modal.
5. Open `/jobs/map`; enable and disable map zoom, navigate nearby work, and select a route without losing the viewport.
6. Open `/stay`; confirm the first available property is selected and empty accounts remain safe.
