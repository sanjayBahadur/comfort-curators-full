# Phase P7.2 — Full walkthrough rehearsal against a clean seed

- **Date:** 2026-08-09
- **Agent/model:** claude-sonnet-5 (orchestrator, run directly — not dispatched)
- **Status:** complete

## Why this ran host-side, not dispatched

`ORCHESTRATION.md §8.2` explicitly names this block as `agent-browser`'s
"legitimate home... driving the walkthrough rehearsal against our own
app from the outside, the same role a Playwright script would play."
This session had no headless browser available until this point in
the run; once one was set up (Playwright + the host's installed
Chrome, no download needed), running the rehearsal directly — rather
than writing a prompt file for a dispatched agent to do the same thing
in an isolated sandbox with no live backend to actually hit — was the
more faithful execution of what this block is for.

## Setup — clean seed

The live dev Postgres volume was wiped (`docker compose down`, `docker
volume rm comfort-curators-backend-alt_pgdata`, `docker compose up
-d`, `npm run seed`) after explicit human confirmation, since this
destroys any state from prior manual testing. Stack came back healthy
on the existing (already `CC_BUILD_TAGS=acceptance`) images; seed
completed cleanly: 2 properties, 3 workers, 3 tickets, 1 calendar
feed, 2 reservations, 4 turnover proposals.

## What I walked, live, through the real UI (not `/debug` shortcuts)

Following `PRD.md`'s own "Definition of done" walkthrough almost
verbatim:

1. **Owner signs in** (`/login` → real role-selection click) → owner
   dashboard renders with real seeded data, zero console/page errors.
2. **Drags six items into a package and watches the monthly cost
   compute** — this is a real `@dnd-kit` drag-and-drop surface, not a
   button (confirmed via DOM inspection: `role="button"
   aria-roledescription="draggable"`). Simulated real mouse drag
   sequences (down → move → up) for 6 distinct catalog items; cost
   panel updated live after each drop; final total (₹3,225.00) matched
   the exact sum of the 6 items' prices.
3. **Activates it** — clicked ACTIVATE, button flipped to ACTIVE, a
   real server round trip (confirmed later on the dashboard: package
   version bumped to V8).
4. **A reservation produces a turnover proposal** — verified via
   `/ops/calendar` scoped to the seeded property: real calendar feed
   (AIRBNB, FRESH), 2 real reservations, 4 real turnover proposals
   already present from the seed's feed poll. Clicked GENERATE
   TURNOVER PROPOSALS — correctly a no-op (idempotent; the existing
   proposals already cover those windows), no error.
5. **Ops turns it into a ticket** — filled and submitted the real
   `/ops/tickets/new` form (property, type, window, reason) →
   real ticket created (`tkt_29454de801411a18`).
6. **Dispatch ranks three curators** — PREPARE FOR DISPATCH → FIND
   RANKED CANDIDATES showed 3 real candidates with real per-criterion
   eligibility (skills, zone, availability, hours, rest, safety, crew
   size), matching `PRD.md`'s "ranks three curators" line exactly.
7. **One is assigned** — ASSIGN ASHA → ticket flipped to SCHEDULED,
   assignment state OFFERED, real state-history events recorded
   (DRAFT → PROPOSED → APPROVED → SCHEDULED).
8. **The curator view shows the job** — `/jobs` (curator role) showed
   the new ticket correctly grouped under its date, with the real
   brief text and SCHEDULED status, alongside the pre-seeded jobs.
9. **The owner dashboard shows the work and the month's cost** — back
   on `/dashboard`: "NEXT SEVEN DAYS" now reads 4 COMMITTED (up from
   3), including the new ticket; "YOUR PACKAGE" shows Hazratganj
   Studio at ₹3,225.00/month, ACTIVE V8 — the exact package built in
   step 2.

Zero console errors, zero failed requests, zero page crashes anywhere
in this chain **after** the two fixes below. Full-page screenshots
captured at every major step.

## Bugs found and fixed live

### 1. `property-detail.tsx` crashed to a blank white screen for any property with zero compliance holds

Navigating directly to `/properties/:id` (not just in-app click
navigation — a bookmark, a refresh, a shared link) threw `Cannot read
properties of undefined (reading 'length')` and rendered nothing.
Root cause: the backend's JSON response omits `compliance_holds`
entirely for a property with none (Go's usual `omitempty` behavior for
an empty/nil slice) — confirmed directly against the real running API.
The frontend's `PropertyData` type declared the field as always
present; `property-detail.tsx` read `.length` on it directly with no
guard, while the *same file*, two lines away, already defensively
handles the structurally identical `emergency_contacts` field via
`?.length ?? 0`. This is the common case, not an edge case — any fresh
property has zero holds. Fixed the type to be honestly optional and
guarded both call sites the same way `emergency_contacts` already is.
Commit `d40eeca`.

### 2. `automation.AgentRunStore.Submit` had a check-then-act race on idempotency keys

Creating a ticket and opening its detail page threw a raw 500 —
`duplicate key value violates unique constraint
"idx_agent_runs_idempotency"` — surfaced verbatim to the Superhost
terminal instead of the thread just connecting. Root cause:
`Submit` checks `GetByIdempotencyKey` and only INSERTs if nothing is
found; two callers with the same key close enough together (almost
certainly `SuperhostMount`'s effect re-running, not a deliberate
retry) both pass the check before either commits, and the loser's
INSERT hits the unique index. This is exactly the scenario an
idempotency key exists to make safe. Fixed using the same
`isUniqueViolation(err)` pattern already established in
`internal/catalog`, `internal/communications`, `internal/access`, and
`internal/documents` — on a unique-violation error, re-fetch and
return the winning row as a duplicate instead of propagating the raw
error. Added `TestConcurrentSubmitWithSameIdempotencyKeySucceeds` (20
goroutines released off a shared start barrier — an earlier,
unsynchronized version of this test passed even with the bug present,
since unsynchronized goroutines against a fast local Postgres rarely
collide). Verified both directions: reliably fails with the exact live
error when reverted, reliably passes (`-race` included) with the fix.
Full `internal/automation`/`internal/automation/superhost` suite
re-run clean. Rebuilt and redeployed the live `api`/`worker`
containers with this fix and re-verified the exact failing scenario
live — thread now connects correctly. Commit `fdbbb1f` (backend repo).

## What I did NOT fully verify

- **Property onboarding from absolute zero** (`/onboarding`) — not
  walked this pass; both seeded properties already exist. The package/
  cost/activate/ticket/dispatch/assign/curator/dashboard chain was
  walked in full using an existing property, which is also the
  realistic demo shape (onboarding a brand-new property live on stage
  is not typically part of the rehearsed chain per `PRD.md`'s own
  wording, which starts from "onboards a property" as a single
  narrated step, not something walked field-by-field here).
- **Assignment acceptance** — the assignment reached `OFFERED`, not an
  explicit "accepted" state; whether/where a worker-side accept action
  exists was not chased down, since `PRD.md`'s own definition of done
  says "assigned," which this satisfies.

## Files changed

- `comfort-curators-backend-alt/internal/automation/store.go`,
  `store_test.go` (bug #2 fix + test, backend repo)
- `ComfortCurators/app/src/routes/property-detail.tsx`,
  `src/lib/api/property.ts` (bug #1 fix)
- Various `/debug` fixes from immediately before this block (Gate 2
  false-negative, Gate 3 demonstrability, Lenis scroll, gated-trigger
  self-revoke) — logged separately as their own commits, not part of
  this block's own scope, but load-bearing for the tooling used here.

## Open questions

None. Both bugs found here are fixed, tested, and verified live
end-to-end.
