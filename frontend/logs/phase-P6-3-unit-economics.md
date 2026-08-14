# Phase P6.3 — unit-economics charts

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/gpt-5.6-luna
- **Status:** complete

## What I built

Added a unit-economics section after the existing authority-model section on `/expansion` with two plain SVG charts:

- An illustrative revenue-mix composition chart across Comfort Curators, Superhost OS, and Curators Crew, using the revenue lines established by P6.1.
- An indicative contribution-per-active-relationship bar chart to show the thesis of increasing surface area across stages.

Both charts carry explicit illustrative/indicative disclosure. Numeric labels and axes use JetBrains Mono, chart fills are flat, axes use hard 1px rules, and only the subscription series uses `--red`.

## Files added or changed

- `app/src/routes/expansion.tsx`
- `app/src/routes/expansion.css`
- `logs/phase-P6-3-unit-economics.md`

## Decisions I made

- Used plain SVG/CSS instead of adding a charting dependency for two static charts.
- Kept the charts as a single section rather than turning the pitch page into a reporting dashboard.
- Used hypothetical percentage and INR values only as visual illustrations, with disclosure in the section heading, chart captions, and accessible labels.
- Kept the existing stage and authority-model content unchanged in substance and did not touch `src/index.css`.

## What did NOT work

- The first build and lint attempts were blocked because `node_modules` was absent (`tsc` and `oxlint` were unavailable). `npm ci` installed the existing lockfile dependencies; no package manifest changes were made. The subsequent checks passed.

## Deviations from the plan

- None.

## Open questions

- The values are deliberately illustrative placeholders and should be replaced or removed when audited operating data exists.
