# Phase 7 — Curator Job View

- **Date:** 2026-08-08
- **Agent/model:** GPT-5.6 Codex (orchestrator); Qwen 3.7 Plus (host seed/browser acceptance); DeepSeek V4 Pro (read-only final review)
- **Status:** complete

## What I built
The staff-gated `/jobs` route now presents assigned, non-terminal dispatch work as a mobile-first curator queue. Each job opens at `/jobs/:ticketId` with the real property, arrival window, access/work brief, assignment, checklist rows, and an explicit metadata-only evidence disclosure.

Curators can check the API checklist, submit immutable evidence metadata with a browser-generated SHA-256 digest for required items, sync checklist completion, and advance the ticket through the real `scheduled → assigned → in_progress → evidence_submitted` transitions. No file bytes or MinIO content are claimed. Tickets without a checklist remain visible but cannot be falsely completed.

The Phase 2 seed now attaches two checklist items to each seeded ticket when none exist, preserving existing rows on rerun and tolerating already-progressed tickets.

## Files added or changed
`app/src/lib/api/ops.ts` — curator job filtering, checklist sync, evidence metadata, and SHA-256 helpers.

`app/src/routes/curator-jobs.tsx` — mobile-first assigned-job list grouped by arrival day.

`app/src/routes/curator-job-detail.tsx` — work brief, checklist, evidence disclosure, and real completion mutation.

`app/src/routes/curator.css` — sharp editorial mobile/desktop job surfaces and 390px containment rules.

`app/src/main.tsx` — registers `/jobs` and `/jobs/:ticketId`.

`app/src/routes/ops-shared.tsx` — links the staff operations header to the curator view.

`app/scripts/seed.ts` — idempotently attaches checklist rows and safely handles already-progressed tickets.

`INTEGRATION.md` — records checklist-sync and evidence metadata contracts.

`logs/DECISIONS.md` — records the assignment/filtering, evidence, and no-checklist decisions.

## Decisions I made
Curator access uses the real `staff` role because the backend has no separate curator role; the UI labels the lane clearly without implying security separation.

The queue is derived from each accessible property's ticket collection plus its assignment subresource. Non-declined assignments are shown; terminal tickets are omitted. This respects the backend's mandatory `property_id` ticket query behavior.

Evidence is metadata-only. The client sends a deterministic SHA-256 digest and a `stub://` object reference, never a fake upload URL or file content.

Completion requires every checklist item to be checked and required evidence to be accepted before the backend transition to `evidence_submitted` is requested.

## What did NOT work
The first live browser pass found horizontal overflow at 390px and selected an older acceptance ticket with no checklist. The mobile grid was corrected with min-content-safe columns and wrapping. The seed was extended so the Phase 2 demo tickets have real checklist rows; the UI still retains an honest no-checklist state for unrelated historical tickets.

The first browser harness could not complete the no-checklist ticket; it was not treated as a product success. The corrected harness selected the seeded checklist-bearing ticket and completed it successfully.

## Deviations from the plan
The backend exposes one shared staff role rather than a curator role, so the curator route uses staff gating. File upload remains intentionally stubbed, as required by the phase.

## New API knowledge
`POST /v1/tickets/{id}/checklist-syncs` accepts checklist item rows keyed by `template_item_index` and returns the checklist collection. `POST /v1/tickets/{id}/evidence` accepts a SHA-256 content hash plus optional checklist/object/file metadata; required evidence is enforced when moving to `evidence_submitted`. These contracts are now in `INTEGRATION.md`.

## How to verify (human runs these)
1. Run `cd ~/comfort-curators-frontend/app && npm run lint && npm run build`. Expected: both commands exit zero.
2. Run `npx tsx scripts/seed.ts` twice with the backend and Vite app running. Expected: checklist rows are created once, then reported as reused; no duplicate tickets or checklist rows appear.
3. Open `http://127.0.0.1:3000/login`, choose STAFF, then open `/jobs`. At 390px, expected: assigned job cards render with no horizontal scroll.
4. Open a seeded Phase 2 turnover job. Expected: property, arrival window, access brief, two checklist items, and metadata-only evidence disclosure are visible.
5. Check every item and press `COMPLETE JOB →`. Expected: evidence metadata is recorded and status becomes `evidence_submitted`; reload preserves that status.
6. Open a job with no checklist, if present. Expected: it says no checklist is attached and disables completion rather than inventing work.

## Open questions for the human

## What's next
Phase 7 is complete. Stop at this boundary for manual approval. Phase 8 should begin only after approval.
