# Phase P9.14 — demo data richness pass

- **Date:** 2026-08-09
- **Agent/model:** gpt-5.6-sol (codex, medium effort)
- **Status:** complete

## What was actually thin (your real survey findings, not assumptions)

I read the complete route table in `src/main.tsx`, traced each data-bearing route to its API client, read `scripts/seed.ts` in full, and checked the two-event `public/demo.ics` fixture. The pre-change seed created 62 catalog items, two active/ready properties, two active 15-line packages, three workers, three assigned tickets, three documents per property, one calendar feed on Gomti Riverside, and two reservations dated 10–13 and 15–18 August 2026.

- `/dashboard` had enough property, readiness, package, and aggregate document data, but only three homogeneous assigned tickets. Its attention list is not populated by ordinary tickets: it reads owner exceptions plus property/readiness/package/expired-document concerns. Its financial contribution and completed-turnover figures remained honestly empty.
- `/ops/tickets` was genuinely thin: three rows, three types, one status, and only two dates despite filters for nine types and twelve statuses.
- `/jobs` had three real dispatched jobs and was usable but small. Keeping those assigned jobs was important so enrichment would not empty the curator view.
- `/ops/calendar` was the largest unresolved gap: Gomti had one feed and two future reservations over an eight-day span; Hazratganj had no feed or reservations. There is no reservation-create endpoint. Reservations can only be ingested from iCalendar, whose actual event content is outside this block's `scripts/seed.ts`-only scope.
- `/documents` showed only three records for whichever property was selected. Six documents on the aggregate dashboard was acceptable, but the property-scoped screen still looked fixture-like.
- `/invoices` was empty. The reachable APIs are aggregate read endpoints; no verified charge, credit, or subledger mutation is available to this seed.
- `/properties/:id`, `/ops/properties`, `/ops/workers`, and `/properties/:id/package` were sufficiently credible for this pass: two distinct active properties with real lifecycle histories, three differentiated workers, active packages, and a rich catalog. Onboarding is an action flow rather than a list needing more fixtures.

## What I enriched, and through which real endpoints

- Expanded the ticket scenario from three to nine records, covering all nine supported ticket types, both properties, six meaningful workflow stages (`draft`, `proposed`, `approved`, `scheduled`, `assigned`, and `in_progress`), and requested windows from 28 July through 24 August 2026. Creation still uses `POST /v1/tickets`; status is advanced only through `POST /v1/tickets/{id}/transitions`; assigned work still uses the real candidate and dispatch endpoints; checklist metadata still uses `POST /v1/tickets/{id}/checklist-syncs`.
- Kept three jobs dispatched for the curator view and moved the existing restock job to `in_progress`, while leaving other new work at truthful pre-dispatch stages.
- Expanded documents from three to six per property (twelve total) with property-appropriate safety, warranty, compliance, NOC, and inventory records. Creation still uses `POST /v1/documents` after listing `GET /v1/properties/{id}/documents`.
- Generalized ticket advancement to stop at each seed record's intended stage. Completed seeded jobs at `evidence_submitted`, `verified`, or `closed` are preserved on later reruns rather than rewound.

## Files added or changed

- `app/scripts/seed.ts`
- `logs/phase-P9-14-demo-data-richness.md`

## Whether you ran the seed live or only type-checked it, and why

The seed was not run live. `http://127.0.0.1:8080/health/ready` and `http://localhost:3000/demo.ics` both refused connections, so neither the real backend nor the required Vite-served calendar fixture was reachable. I did not claim a double-run result without those services.

Verification completed locally:

- `npm run build` — passed after restoring the worktree's missing lockfile dependencies.
- `npm run lint` — passed with seven pre-existing warnings and no errors.
- `npx tsc --noEmit` — passed.
- `git diff --check` — passed.

Static idempotency review: documents list the property collection first and reuse exact property-qualified titles; tickets list both properties and reuse exact unique reasons; workflow advancement is a no-op at the desired or any later safe status; assignments are created only when the assignment collection is empty; checklists are synced only when no items exist. Existing catalog, property, package, worker, availability, feed, and iCalendar content-hash/external-UID reuse behavior was not changed. A second run therefore has no path that creates a duplicate in any resource touched here.

## Decisions I made

- Kept the two-property portfolio. The corrected actor sequence intentionally captures staff-visible properties, switches to the owner, asserts existing managed properties are owner-visible, and only then creates/address-matches. A third property would add little state variety because activation makes every seed property active, while increasing the chance of disturbing this subtle visibility/idempotency path and requiring parallel package/document arrays.
- Did not attach the same two-event iCalendar feed to Hazratganj. That would create backend records but would make both homes appear to have identical bookings and external event IDs, which is not high-quality demo data.
- Used only supported, low-risk ticket states. I did not force blocked, cancelled, rejected, evidence, verified, or closed records without satisfying their real workflow gates.
- Did not change frontend models, analytics fields, UI components, catalog description/image files, or property visibility/session logic.

## What did NOT work

- Live backend and Vite preflight checks failed because neither service was running in the sandbox, so live seeding and live double-run proof were unavailable.
- The first build/lint attempt found an incomplete `node_modules` (`oxlint` and declared runtime packages were absent). `npm install` restored dependencies without changing tracked manifests, after which the required commands passed.
- No `AGENTS.md` exists in this worktree or its parent repository search, so that requested file could not be read. Both files in the available Tier 1 clean-architecture rules pack were read in full.

## Open questions (anything that would need new backend surface, escalated rather than built)

- Calendar richness still needs an authorized source-data change: either permit richer, property-specific iCalendar fixtures outside `scripts/seed.ts`, or provide a verified reservation ingestion endpoint. The backend exposes no direct reservation-create route, so past/upcoming reservations across both properties cannot be truthfully added in this block alone.
- Invoice richness needs a verified write path for real charges, credits, or subledger entries. Current screens expose aggregate reporting only, and the API does not provide invoice line-item reads.
- Dashboard attention needs a verified owner-exception mutation or real incident/service-recovery workflow that produces owner-visible exceptions. Adding ordinary tickets does not feed that section.
- Completed-turnover analytics require the full real evidence/checklist verification workflow. This pass deliberately did not fabricate evidence just to increase a number.
- `/ops/properties` cannot show active-package details because that route does not fetch package records, and `/ops/workers` cannot show availability/history because its current API consumer does not provide them. More seed records cannot repair either presentation gap.
