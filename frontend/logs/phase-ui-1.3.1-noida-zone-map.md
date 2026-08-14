# Phase UI 1.3.1 — Noida zone map interaction

- **Date:** 2026-08-13
- **Agent/model:** GPT-5
- **Status:** complete

## What I built
Restaged the frontend-only field map in Noida, converted property and ticket pins into Lucide-inspired monochrome map badges, made selection updates preserve the map viewport, and added an explicit control that switches the mouse wheel between normal page scrolling and map zooming.

## Files added or changed
`app/src/lib/demo-property-locations.ts` — replaces the old Lucknow demo coordinate anchors with Noida Sector 18 and Sector 137 anchors without modifying stored property addresses.

`app/src/routes/curator-zone-map.tsx` — adds icon-backed marker markup, a persistent Leaflet instance, Noida field-copy, and a map-zoom/page-scroll mode control.

`app/src/routes/curator.css` — styles the circular markers, simulated curator marker, and interaction-mode control.

`logs/phase-ui-1.3.1-noida-zone-map.md` — records the map decisions and verification steps.

## Decisions I made
The backend's seeded property addresses remain unchanged and continue to appear in property records. Only the existing frontend demo-coordinate layer is staged in Noida, because it was already explicitly simulated rather than geocoded.

Map zoom begins locked so ordinary wheel and trackpad gestures scroll the page. The user must deliberately select `ENABLE MAP ZOOM` before wheel, touch, double-click, or box zoom is captured by Leaflet. The same button then becomes `SCROLL PAGE`, providing a clear exit.

The zoom mode control sits in the map's top-right corner and uses the prior Superhost neon (`#00ff66`) with a persistent neon glow while active. Nearby-work navigation uses the Debug page's success green (`--ok`, `#1a6b3c`) across its control, plotted lines, dotted segments, destination points, and estimate accents. The built-in Leaflet plus/minus control is removed to keep one clear interaction model.

The global Superhost trigger, scrim, and drawer use the app's 9996–9997 overlay tier, above Leaflet's 1000-level controls and panes, so an open drawer always covers the map and its controls.

Marker glyphs follow Lucide's lightweight outline language: home for properties, wrench for tickets, and navigation for the simulated curator. Property pins retain black identifiers, ticket pins retain the red exception accent, and selected markers use a high-contrast outline. The icon geometry is locally bundled so the map has no icon-CDN runtime dependency.

The Leaflet map is created once. Marker and route layers update independently, so selecting an item no longer recreates the map, refits its bounds, or discards the user's pan and zoom position.

Open tickets are indexed by distance from the simulated curator rather than API order. The top-left `NAVIGATE NEARBY WORKS` control plots up to five closest routes and opens a scrollable panel contained entirely inside the map. Cards reuse the existing property photography and expose distance, demo ETA, and traffic estimates; selecting one updates the detail route without moving the map.

Selecting a nearby-work card promotes its route to a darker solid green with greater weight and a larger destination point. The other proximity routes remain lighter and dotted, preserving the full comparison while making the active destination unmistakable.

## What did NOT work
none

## Deviations from the plan
none

## New API knowledge
none

## How to verify (human runs these)
Run `cd ~/open-code-projects/ComfortCurators/app && npm run build`. Expected: the production build completes successfully.

Open `/jobs/map`. Expected: the copy identifies Noida, property and ticket markers are circular and photo-backed, and the simulated curator marker is also circular.

Scroll over the map before enabling zoom. Expected: the page scrolls and the map scale stays fixed. Click `ENABLE MAP ZOOM`, then scroll over the map. Expected: the map zooms. Click `SCROLL PAGE`. Expected: wheel gestures move the page again.

## Open questions for the human

## What's next
Visually inspect marker density and control placement at the target browser size before committing this phase.
