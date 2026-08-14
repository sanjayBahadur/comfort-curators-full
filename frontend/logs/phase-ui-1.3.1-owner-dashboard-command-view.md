# Phase UI 1.3.1 — Owner dashboard command view

- **Date:** 2026-08-13
- **Agent/model:** GPT-5
- **Status:** complete

## What I built
Added a selected-property command view to the owner dashboard. Owners can move between managed homes with carousel controls; the selected home scopes a read-only Superhost task ledger, real ticket/package/document/contribution metrics, reservations, an address-context map, and a compact current-month booking calendar. Existing portfolio evidence sections remain below the new overview.

## Files added or changed
`app/src/components/owner/OwnerPropertyOverview.tsx` — implements property selection controls, verified-data metrics, property-scoped reservation loading, address map, booking calendar, and essential links.

`app/src/components/owner/OwnerPropertyOverview.css` — provides the responsive ruled-grid layout, map treatment, metric typography, and compact calendar.

`app/src/components/owner/OwnerTaskTerminal.tsx` — replaces the dashboard Superhost mount with a property-scoped task ledger. Checklist lines expand in place; active work reveals its recorded context, proposed work exposes a real owner approval action, and drafts explain that they must first be proposed.

`app/src/components/owner/OwnerTaskTerminal.css` — applies the black/phosphor terminal language without a composer, handover, or send field. Approval remains a quiet inline terminal command rather than a separate panel.

The terminal deliberately uses the original console's larger reading scale and sparse line rhythm. It is one continuous checklist, not a partitioned dashboard: `[✓]` identifies active workflow tasks and `[?]` identifies approval-pending records. Each collapsed item shows only task name, workflow status, and timing; selecting it reveals context and any valid action without adding columns or boxed panels.

`app/src/lib/api/ops.ts` — allows transition callers to provide an honest workflow-specific reason while preserving the existing operations-desk default.

`app/src/routes/dashboard.tsx` — owns selected-property state, scopes Superhost/current-property context, and mounts the command view.

`logs/phase-ui-1.3.1-owner-dashboard-command-view.md` — records data sources and honesty boundaries.

## Decisions I made
Metrics use only existing records: active and upcoming reservations, ticket workflow states, readiness flags, contribution reporting, packages, and documents. The map uses live CARTO/OpenStreetMap tiles but the pin is explicitly labeled an approximate address position because the owner dashboard API does not provide GPS coordinates. No live tracking claim is made.

The calendar shows the current property month and treats non-cancelled reservation date ranges as reserved. The first property is selected by default; carousel changes also update the shared current-property context and task ledger.

The ledger maps recorded `approved`, `scheduled`, `assigned`, `in_progress`, and `evidence_submitted` tickets to active tasks. Recorded `draft` and `proposed` tickets appear in the review checklist. Only a recorded `proposed` ticket can be approved; approval calls the existing ticket-transition API with an owner-dashboard audit reason and then refreshes dashboard data. Drafts remain read-only because skipping directly from draft would violate the recorded workflow.

## What did NOT work
none

## Deviations from the plan
none

## New API knowledge
none

## How to verify (human runs these)
Run `cd ~/open-code-projects/ComfortCurators/app && npm run build`. Expected: TypeScript and the production bundle complete successfully.

Open `/dashboard` as an owner with multiple properties. Expected: the first property is selected; carousel arrows swap the property title, map, metrics, calendar, links, and Superhost context.

Select an active task. Expected: its recorded reason expands below the checklist line. Select a proposed task and choose `[ APPROVE ]`. Expected: the approval records, a confirmation appears, and refreshed dashboard data moves the task into the active list. Draft tasks explain why they cannot yet be approved.

Inspect the map caption. Expected: it reads `LIVE MAP TILES / APPROXIMATE ADDRESS POSITION`, accurately separating live tiles from a non-GPS pin.

Resize below 1100px and 620px. Expected: metrics reflow to three and then two columns; map and calendar stack without horizontal page overflow.

## Open questions for the human

## What's next
Visually inspect the new command view at the target desktop width before committing. The earlier uncommitted `/stay` default-property phase remains separate.
