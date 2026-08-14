# Phase 4 — Operations Tickets and Dispatch

- **Date:** 2026-08-08
- **Agent/model:** GPT-5.6 Codex (orchestrator); DeepSeek V4 Flash (API audit and host acceptance); DeepSeek V4 Pro (read-only final review); Qwen 3.7 Plus (bounded UX review attempt)
- **Status:** complete

## What I built
The staff operations desk now exists at `/ops/tickets`, `/ops/tickets/new`, and `/ops/tickets/:ticketId`. Staff sessions enter the ticket queue directly; other roles remain outside the operations routes.

The queue requests tickets for each accessible property and merges them into a dense ruled table with property, type, requested window, status, assignee, and age. Property, type, and status filters are immediate URL state. At 820px and below, each row becomes a labelled record; a real 390px touch viewport has no horizontal overflow.

The creation screen posts a real ticket with property, type, future requested window, and reason, then opens its detail. Detail loads the work brief, checklist, evidence, chronological state history, and assignment collection. A fresh draft is explicitly moved through proposed, approved, and scheduled before dispatch, leaving three real state events. Scheduling always refreshes server state even if a multi-step transition stops partway through.

Dispatch requests the real backend candidate evaluation, orders eligible workers first and score descending, and exposes the backend constraint reasons instead of inventing suitability copy. Assignment posts one real offer, refreshes the detail and queue, renders the assignee from the assignment subresource, and prevents duplicate offers while an assignment exists.

The deterministic live-browser acceptance passed 13/13 checks against the reset acceptance backend: cross-property queue, create/reuse acceptance ticket, chronological history, real ranked candidates and checks, persisted assignment, queue reflection, URL filters, zero radius/shadow, zero operations grain, clean runtime, and exact 390px responsive containment. Desktop and mobile screenshots were inspected at original resolution.

## Files added or changed
`app/src/lib/api/ops.ts` — typed ticket, worker, state-event, checklist, evidence, candidate, and assignment contracts plus queue/detail/create/transition/dispatch operations.

`app/src/routes/entry-route.tsx` — sends an existing staff session at `/` to the operations queue and all other sessions to login.

`app/src/routes/ops-format.ts` — shared property, worker, time-window, moment, status, and age presentation helpers.

`app/src/routes/ops-shared.tsx` — staff route gate, operations header, status marker, and loading skeleton.

`app/src/routes/ops-tickets.tsx` — merged cross-property queue with URL-backed filters and assignment display.

`app/src/routes/ops-ticket-new.tsx` — validated real ticket creation form and detail redirect.

`app/src/routes/ops-ticket-detail.tsx` — brief/checklist/evidence/history surface, explicit scheduling chain, candidate constraints, ranking, and assignment.

`app/src/routes/ops.css` — low-collage ruled desktop and mobile operations system, including the 390px stacked-table correction.

`app/src/main.tsx` — registers the operations routes and role-aware root entry.

`app/src/routes/login.tsx` — adds the staff operations entry action.

`INTEGRATION.md` — records candidate ordering/check shape and the assignment subresource's authoritative role.

`logs/DECISIONS.md` — records the cross-property queue, ranking, assignment, and explicit state-chain decisions.

## Decisions I made
The queue requests tickets once per accessible property and merges them because the halted backend's nominally optional `property_id` filter is effectively mandatory. Assignment collections are fetched with each ticket so assignee names reflect the real offer resource rather than the ticket's unchanged `assigned_to` field.

Candidate order is eligible first, then score descending, then worker name for deterministic ties. The backend returns candidate checks and scores but does not return the array in ranking order. Check detail is preferred verbatim; restrained human-readable fallback copy is used only when detail is absent.

The UI provides one “prepare for dispatch” action but records every required transition separately. It does not claim the process is automatic. Query refresh runs after success or failure so a partial transition chain never leaves stale status on screen.

Operations suppresses the global grain and avoids collage devices over tables. This follows the screen-specific art direction while retaining the project's type, hard rules, warm paper, and red interaction language.

## What did NOT work
The broad Qwen 3.7 Plus UX run did not complete within a useful bounded window and was terminated. Its output was not used. DeepSeek V4 Flash completed the narrower live-contract audit, and DeepSeek V4 Pro completed the bounded final correctness review.

The first browser harness waited for a full document navigation after React Router ticket creation, so it timed out despite a successful SPA route change. The harness was changed to observe rendered route state and reuse the acceptance ticket by its exact reason.

The first automated candidate `ElementHandle.click()` did not emit an assignment request even though the control was enabled. A DOM click through the same rendered button exercised the React handler and produced the real assignment offer; the product code did not require an assignment fix.

The first 390px check measured 449px of document overflow. The absolute row-action cell inherited the data-cell's 112px grid track, making its link extend past the 54px action rail. Making that one mobile cell a block restored exact 390px containment.

Codex's restricted Chromium process could not launch because crashpad socket setup was denied. The same deterministic harness ran through the authorized OpenCode host environment, which also has the live backend access needed for acceptance.

The Pro reviewer found that a partially successful scheduling chain would persist backend progress while the UI refreshed only on total success. Moving invalidation to `onSettled` fixed that stale-state edge case.

## Deviations from the plan
None. The operations surface implements only the Phase 4 ticket queue, creation, detail/history, candidate evaluation, and assignment scope.

## New API knowledge
The candidates endpoint returns its array in database order rather than score order. Each constraint check contains `constraint`, `hard`, `passed`, and optional `detail` fields. A successful assignment creates an offer in the assignment subresource but does not change ticket status, `assigned_to`, or state history. An empty assignment collection can return `items: null`. These findings are now in `INTEGRATION.md`.

## How to verify (human runs these)
1. Run `cd ~/comfort-curators-frontend/app && npm run lint && npm run build`. Expected: both commands exit zero.
2. Run `npm run dev`, open `http://127.0.0.1:3000/login`, and choose STAFF. Expected: `OPEN OPERATIONS DESK` appears; opening it shows tickets from both Gomti Riverside 2BHK and Hazratganj Studio.
3. Change property, type, and status filters. Expected: rows update immediately, the URL query updates without a ticket refetch, and reload restores the selected filters.
4. Open `NEW TICKET`, choose a property and `TURNOVER`, enter a future start/end and a reason, then create it. Expected: one HTTP 201 ticket POST and a draft detail screen.
5. On the new detail, click `PREPARE FOR DISPATCH`. Expected: status becomes scheduled and state history shows draft → proposed, proposed → approved, and approved → scheduled in order.
6. Click `FIND RANKED CANDIDATES`. Expected: eligible workers precede ineligible workers; each shows its score, skills, and real backend constraint checks/reasons.
7. Assign an eligible worker. Expected: one assignment offer is created, `CURRENT ASSIGNMENT` shows the worker and offered state, the queue shows the same assignee, and all other assignment buttons are disabled.
8. Set the browser viewport to 390px. Expected: table headings are replaced by per-row labels, the action arrow stays in its 54px rail, and there is no horizontal scroll.

## Open questions for the human

## What's next
Phase 4 is complete. Stop at this boundary for manual approval. After approval, begin Phase 5 from `PHASES.md`; do not extend the operations desk speculatively before that phase is restated.
