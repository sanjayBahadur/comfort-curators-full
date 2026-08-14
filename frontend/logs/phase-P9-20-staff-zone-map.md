# Phase P9.20 — Staff zone map

## What shipped

Added a curator-facing location schematic at `/jobs/map`. It groups properties and open tickets into ruled columns using each property's real `geolocation_zone` value. Properties with blank or missing zone text, and their tickets, are retained in a clearly labeled `UNZONED` column.

Property markers expose the recorded service address and zone, then link to a staff-safe property record. Ticket markers expose the ticket type, status, destination property, service address, and zone, then link to the curator ticket record. The existing curator job detail now also reads the real `geolocation_zone` field for its travel-zone label instead of treating the city as a zone.

The diagram uses the product's paper, ink, hairline-rule, mono-label, and sharp-corner visual language. No gradients, shadows, rounded corners, geographic tiles, or third-party map styling were added.

## Accuracy boundary

This is a schematic zone diagram built only from real structured `service_address` and plain-text `geolocation_zone` data returned by the property API.

It is **not a real GPS map**. It does not claim geographic position, coordinates, street geometry, route order, travel time, or distance. No latitude/longitude values were fabricated, no geocoder is called, and no Google Maps, Mapbox, Leaflet, or other map SDK was introduced.

## Verification

- `npm run build` — passed.
- `npm run lint` — passed with only the repository's pre-existing warnings in unrelated Superhost files.
- Read back `curator-zone-map.tsx`, `curator-property-detail.tsx`, and `curator.css` in full.
- `git diff --check` — passed.
- Grepped added diff lines for `border-radius`, `box-shadow`, `linear-gradient`, and `radial-gradient` — no matches.
- Per the phase constraint, no browser was launched in this sandbox. Browser verification remains for the orchestrator.
