# P9.23 — Real staff map

## Outcome

Replaced the `/jobs/map` zone schematic with a Leaflet map using the required
CARTO Positron tiles and `© OpenStreetMap contributors © CARTO` attribution.
The page continues to use the existing ticket-queue query, open-ticket
filtering, property grouping, marker selection, detail panel, and record links.

The map now includes:

- ink-black property markers and red open-ticket markers;
- a distinct curator crosshair marker;
- a dashed route line to the selected property/ticket destination;
- distance and ETA/traffic details that update with marker selection;
- app-styled zoom controls, attribution, labels, and mobile sizing.

## Demo-data disclosure

All location and traffic information on this screen is simulated and
demo-only. The property coordinates are frontend-only points around
Hazratganj and the Gomti Riverside area of Lucknow; they are not backend
property coordinates. The curator position is a fake nearby point, not real
GPS. The route is a straight Leaflet polyline, not a road-following route. The
ETA and traffic label come from deterministic client-side arithmetic, not a
real routing service or live traffic feed.

## Dependencies

- `leaflet` 1.9.4
- `@types/leaflet` 1.9.22

## Verification

- `npm install leaflet` — passed
- `npm install --save-dev @types/leaflet` — passed
- `npm run build` — passed
- `npm run lint` — passed (existing repository warnings remain; no warning is
  introduced by this phase)
- New CSS checked for `border-radius`, `box-shadow`, `linear-gradient`, and
  `radial-gradient` — none introduced
- Browser verification intentionally left to the orchestrator per phase brief
