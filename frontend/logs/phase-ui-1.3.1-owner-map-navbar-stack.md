# Phase UI 1.3.1 — Owner map navbar stack

- **Date:** 2026-08-13
- **Agent/model:** GPT-5
- **Status:** complete

## What I built
Corrected the owner dashboard stacking order so the sticky navigation always renders above Leaflet map tiles, markers, zoom controls, and attribution.

## Files added or changed
`app/src/routes/dashboard.css` — raises the owner navbar above Leaflet's documented internal stacking range.

`app/src/components/owner/OwnerPropertyOverview.css` — isolates the map panel's stacking context so map internals remain contained within the panel.

`logs/phase-ui-1.3.1-owner-map-navbar-stack.md` — records this focused UI correction.

## Decisions I made
The navbar uses z-index 2000 because Leaflet controls use 1000 and map panes extend through 700. The map panel is also isolated for a structural guarantee rather than relying only on a larger navbar number.

## What did NOT work
none

## Deviations from the plan
none

## New API knowledge
none

## How to verify (human runs these)
Open `/dashboard`, scroll until the property map passes beneath the sticky navbar, and interact with map controls. Expected: no map tile, marker, control, or attribution appears above or blocks the navbar.

Run `cd ~/open-code-projects/ComfortCurators/app && npm run build`. Expected: the production build completes successfully.

## Open questions for the human

## What's next
Continue visual review of the uncommitted onboarding dossier after confirming this stacking fix.
