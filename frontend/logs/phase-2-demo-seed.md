# Phase 2 — Demo seed

- **Date:** 2026-08-08
- **Agent/model:** GPT-5.6 Codex (orchestrator); DeepSeek V4 Flash, DeepSeek V4 Pro, Qwen 3.7 Plus (read-only specialists)
- **Status:** complete

## What I built
`app/scripts/seed.ts` now builds a coherent Lucknow demo scenario through the real halted backend. It preflights the backend and Vite-hosted iCalendar file before writing, mints a staff session, check-then-creates 15 INR catalog items, address-matches and activates two properties through the full lifecycle and readiness gate, creates and activates a priced package for each property, creates three workers with dispatch-compatible skills and availability, creates/schedules/dispatches three ticket types, reuses or polls the demo calendar feed, and verifies reservations plus persisted turnover proposals. It prints created/reused counters and final seeded-versus-total counts.

After the human-approved demo-volume reset, the corrected script completed twice consecutively against the clean live stack. The first run created exactly 15 catalog items, 2 active properties, 2 active packages, 3 workers, 3 tickets, 3 dispatch assignments, 1 calendar feed, 2 reservations, and 4 proposals. The second run created nothing, reused every managed resource, and reported the same exact totals.

## Files added or changed
`app/scripts/seed.ts` — implements the preflighted, check-then-create, partial-run-recoverable Phase 2 scenario and final assertions.

`app/package.json` — adds `npm run seed` with `tsx --no-cache`; disabling the cache avoids a sandbox-only IPC socket denial while preserving normal `tsx` execution.

`app/tsconfig.node.json` — type-checks the seed script as part of the normal production build.

`INTEGRATION.md` — records the newly resolved readiness, transition, dispatch, availability, ticket-list, feed-list, poll, proposal, and effective idempotency contracts.

`logs/DECISIONS.md` — records cross-phase idempotency, dispatch-worker, and proposal-verification decisions.

`logs/phase-2-demo-seed.md` — preserves this implementation and live-test history.

## Decisions I made
The script keeps the documented property `idempotency_key` for forward compatibility but does not trust it: the halted handler reads the field and then discards it. Properties are reused by normalized `line1` plus postal code. Every other resource has a stable domain key: SKU, worker legal name, package status/content, ticket reason, assignment presence, or feed URL.

Both active properties receive an active package rather than leaving the second property operationally hollow. The property API has no name field, so the requested human-readable property names are retained in `service_address.line2` while the real street remains `line1`.

The documented worker skills are retained and supplemented with backend aliases: `turnover`, `general`, `restock`, and `inventory`. Each worker also receives an availability window. The real dispatch evaluator otherwise rejects the plausible Phase 2 workers, so tickets would have no candidates.

Calendar-chain success is based on persisted reservations/proposals and the poll result. The poll itself creates turnover and inspection proposals; the explicit generate call that follows is therefore expected to return zero new proposals on a healthy idempotent run.

OpenCode routing was task-specific. DeepSeek Flash extracted contracts, DeepSeek Pro reviewed idempotency/runtime risks, and Qwen 3.7 Plus performed the acceptance review. Two broad early Pro/Qwen explorations were terminated when they continued exploring without returning bounded findings; later file-scoped reviews completed. Source inspection and live API behavior remained authoritative over advisory model output.

## What did NOT work
Plain `tsx` initially attempted to open an IPC cache socket that the execution sandbox forbids (`listen EPERM /tmp/tsx-1000/*.pipe`). `tsx --no-cache` removes that sandbox-only IPC requirement and ran normally.

The first live pass reached ticket dispatch and revealed that an empty `GET /v1/tickets/{id}/dispatch/assignments` response serializes `items` as `null`, not `[]`. The script now normalizes that response.

The next partial-recovery pass revealed that `GET /v1/tickets` always applies `property_id` as a SQL predicate; omitting the supposedly optional filter returns zero items. Because the first implementation used that empty collection for its existence check, it created one second turnover ticket before failing final verification. Ticket discovery and verification now query each property and combine the collections. Two corrected runs added no further duplicates.

The pre-reset live tenant contained one catalog item, one unrelated property, and the duplicate turnover development artifact. Per the working policy, the orchestrator did not delete backend records directly. The human approved and performed a demo-volume reset; the two clean runs that followed produced exact seeded and total counts.

Direct `curl` from the restricted tool namespace cannot reach loopback, even while the `tsx --no-cache` process can execute the live acceptance chain. The script's authenticated final reads are the captured live evidence.

## Deviations from the plan
The plan says “package version” in the singular; this seed creates and activates one package per property so both active properties have coherent commercial data. It also creates worker availability windows and backend skill aliases, which the plan omitted but real dispatch requires.

The acceptance text expects an unfiltered `/v1/tickets` read to show three and a post-poll generate call to report `proposed > 0`. The halted backend cannot meet those two literal checks: ticket list requires `property_id`, and feed polling already creates proposals. The equivalent honest checks are combined property-scoped ticket reads and either the poll's `proposals_proposed` or the persisted proposal collection.

## New API knowledge
Property readiness is a three-boolean PUT. Property transitions require a non-empty `If-Match`, target state, and reason. Property `idempotency_key` is not wired to persistence in this build.

Tickets must be advanced to `scheduled` before dispatch. Candidate evaluation requires dispatch-specific skill aliases and at least one availability window. Assignment takes `worker_id`; empty assignment collections use `items: null`; repeated assignment POSTs are not idempotent.

Unfiltered ticket listing returns no rows because the store always compares `property_id`; callers must query per property. Calendar feeds can be listed and must be reused by URL because no uniqueness constraint exists. A successful poll returns ingestion counts and synchronizes reservations and proposals in one transaction. These details were added to `INTEGRATION.md`.

## How to verify (human runs these)
1. Run `cd ~/comfort-curators-frontend/app && npm run lint && npm run build` — expected: both exit zero, including TypeScript checking of `scripts/seed.ts`.
2. Keep the acceptance backend and `npm run dev` running, then run `npx tsx --no-cache scripts/seed.ts` — expected: it reaches `✓ Phase 2 seed complete` and prints exact totals of catalog 15, properties 2 active, workers 3, tickets 3, reservations 2, and proposals 4.
3. Run `npx tsx --no-cache scripts/seed.ts` a second time — expected: it completes again; `created` is empty and all managed resource counts appear under `reused`, with totals unchanged.
4. With a staff token, query `GET /v1/catalog/items`, `GET /v1/properties`, and `GET /v1/workers` — expected: all 15 seed SKUs, the two named address records in `active`, and Asha/Ravi/Meena.
5. Query `GET /v1/tickets?property_id={gomti_id}` and the same for `{hazratganj_id}` — on a clean database, expected combined unique Phase 2 reasons: 3. Do not use unfiltered `/v1/tickets`; this halted build returns an empty collection for an empty property filter.
6. Query `GET /v1/properties/{gomti_id}/reservations` and `GET /v1/properties/{gomti_id}/turnover-proposals` — expected: 2 reservations and 4 proposals. A following generate POST may truthfully report zero newly proposed because those four already exist.

## Open questions for the human
None.

## What's next
Stop for manual Phase 2 acceptance. After approval, begin Phase 3 from `SHOP.md`; do not start it before the human confirms this seed and current demo data.
