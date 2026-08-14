# Phase P9.16 — calendar dummy data (two distinct feeds)

- **Date:** 2026-08-10
- **Agent/model:** gpt-5.6-sol (codex, medium effort)
- **Status:** complete

## What I built

- Expanded Gomti Riverside's existing public iCalendar fixture from 2 to 9 non-overlapping reservations, retaining the two existing UIDs and dates so the next poll updates the feed without replacing those records.
- Added a separate Hazratganj Studio fixture with 8 reservations, its own product identifier, different dates, and a distinct UID namespace.
- Kept every `SUMMARY` as `Reserved`. The backend parser copies `SUMMARY` into `guest_summary` and uses normalized summary text during duplicate detection, so it is operational data rather than purely decorative. A generic value keeps the public fixtures useful without adding PII-shaped demo content.
- Added a property-specific `calendarFeedUrl` to each existing property seed, changed `seedReservationChain` to accept that URL, and invoked the chain for both properties using the same indexed-loop convention already used by package and document seeding.
- Extended preflight to validate both static fixture paths and extended final verification to fetch and require reservations for each property, not only the first.

## The two ICS fixtures — date ranges and event counts for each

- `public/demo.ics` (Gomti Riverside 2BHK): 9 events from `2026-07-18T11:00:00Z` through `2026-09-13T05:00:00Z`. Stay lengths vary from 2 to 5 nights. Four stays end before the real demo date of 2026-08-10, one starts that day, and four are upcoming.
- `public/demo-hazratganj.ics` (Hazratganj Studio): 8 events from `2026-07-20T11:00:00Z` through `2026-09-10T05:00:00Z`. Stay lengths vary from 2 to 5 nights. Three stays end before 2026-08-10, one crosses that date, and four are upcoming.
- A local fixture validator confirmed 17 unique UIDs across two property-specific namespaces, valid `YYYYMMDDTHHMMSSZ` fields, `DTSTART < DTEND`, generic summaries, and no within-feed overlaps.

## Files added or changed

- `app/public/demo.ics`
- `app/public/demo-hazratganj.ics`
- `app/scripts/seed.ts`
- `logs/phase-P9-16-calendar-data.md`

## Whether you ran the seed live or only type-checked it, and why

The seed was not completed live. Initial and final checks of `http://127.0.0.1:8080/health/ready` and `http://localhost:3000/demo.ics` refused connections. During one transient interval, a backend and stale Vite server appeared long enough for `npx tsx scripts/seed.ts` to pass backend readiness and `/demo.ics`, but the new `/demo-hazratganj.ics` correctly failed the expanded preflight. Both services disappeared again immediately afterward. I did not bypass that fail-fast check or claim a double-run/GET result against mismatched fixture content.

Verification completed locally:

- `npm run build` — passed.
- `npm run lint` — passed with seven pre-existing warnings and no errors.
- `npx tsc --noEmit` — passed.
- `git diff --check` — passed.
- Static fixture validation — passed for event counts, distinct UIDs, timestamp format, ordering, summaries, and non-overlap.

The first build/lint attempt exposed an incomplete `node_modules` tree: declared runtime packages and `oxlint` were missing. `npm install` restored lockfile dependencies without changing tracked manifests; the three verification commands then passed. npm reported the existing Node 20 engine warning for `@testing-library/jest-dom` and three dependency audit findings.

## Decisions I made

- Preserved `CC_DEMO_ICAL_URL` for Gomti and added `CC_DEMO_HAZRATGANJ_ICAL_URL` for the second property, with Docker-host defaults pointing to their respective public files.
- Kept feed registration keyed by the exact per-property URL: `feeds.items.find((item) => item.data.url === feedUrl)`. On a rerun, each property reuses its feed. The backend content hash makes an identical second poll return unchanged; its `(feed_id, external_event_id)` uniqueness/upsert path also protects reservation identity. Updating Gomti's fixture therefore reuses its original two event UIDs and adds the seven new ones once, while Hazratganj creates its independent eight once.
- Used sequential per-property ingestion. This matches existing seed loops, produces readable property-scoped progress, and avoids obscuring which property failed.
- Kept iCalendar ingestion as the only reservation creation path. Backend route and reservation source review found no direct reservation-create endpoint.

## What did NOT work

- Live seed completion, a second live run, and live `GET /v1/properties/{id}/reservations` confirmation were unavailable because no matching backend plus Vite instance remained reachable.
- The transient Vite instance served only the old fixture set. The new two-file preflight rejected it as intended instead of seeding partial calendar data.
- No `AGENTS.md` exists in this worktree or its parent repository search, so it could not be read. Both files in `app/.sandcastle/rules/clean-architecture/` were read in full.

## Open questions

- No implementation question remains. When the normal backend and this worktree's Vite server are running together, the operational follow-up is to run the seed twice and confirm 9 Gomti reservations and 8 Hazratganj reservations through each property's reservation GET endpoint.
