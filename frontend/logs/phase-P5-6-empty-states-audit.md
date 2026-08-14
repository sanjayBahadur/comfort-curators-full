# Phase P5.6 — empty states and skeletons audit, all P5 surfaces

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/deepseek-v4-flash
- **Status:** complete

## What I built

Audited all seven P5 surfaces against SCREENS.md's three-states standard
(loading skeleton, real empty copy, populated) plus error/retry and
prerequisite ("select a property") handling, and fixed the genuinely missing
or inconsistent pieces. This was a consistency pass over surfaces that already
mostly had states, not a rewrite.

## Files added or changed

- `app/src/routes/stay.tsx` — added a `StaySkeleton` component (card + catalog
  variants), swapped the two text-only loading messages ("Checking the
  reservation record…", "Loading the catalog…") for real skeletons, added
  working retry buttons to the reservation and catalog error states, and added
  an error+retry notice for a failed properties list.
- `app/src/routes/stay.css` — added `.stay-skeleton` (card/catalog), the
  inline retry buttons for `.stay-state--error` and `.stay-error`, the pulse
  keyframe, and a `prefers-reduced-motion` guard.
- `app/src/routes/ops-calendar.tsx` — added a `PROPERTY LIST UNAVAILABLE.`
  error+retry block for the properties query that feeds the page's selector
  (previously the selector could fail with no affordance and the page just
  stayed on the "SELECT A PROPERTY." prompts).

## Decisions I made

Page-by-page audit result:

- **ops-calendar.tsx** — the three content sections (feed health, reservations,
  turnover proposals) were already correct: each has `OpsSkeleton`, specific
  empty copy, and an error state with a working `RETRY` that says "The backend
  message is shown in the toast." The only gap was the *properties* query: it
  feeds the selector, and if it failed the page silently stayed on the
  "SELECT A PROPERTY." prompts with no retry. Added a compact error+retry block
  under the selector using the page's own `ops-empty` pattern. No data-fetching
  logic touched.
- **ops-properties.tsx** — already correct: `OpsSkeleton rows={6}`, real empty
  copy ("NO PROPERTIES." / "Properties will appear here after the first
  property is onboarded."), and a working error+retry state. No change.
- **ops-workers.tsx** — already correct: `OpsSkeleton rows={6}`, "NO WORKERS."
  empty copy, error+retry, and the roster drawer has its own designed
  "No current assignments in the loaded ticket queue." state. No change.
- **property-detail.tsx** — already correct: `PropertySkeleton` at page level,
  `property-quiet-empty` for compliance holds / lifecycle history / emergency
  contacts (consistent register: serif headline + muted one-line rationale),
  and a `property-detail-error` block with a working `TRY AGAIN` that refetches.
  No change.
- **invoices.tsx** — already correct: `OwnerRecordsSkeleton`, "No charges yet."
  empty copy matching SCREENS.md, `owner-records-error` + `TRY AGAIN` for both
  the properties and the report query, and a "Select a property." prerequisite
  state. No change.
- **documents.tsx** — already correct: `OwnerRecordsSkeleton` for both the page
  and the "On file" section, "No documents on file." empty copy, error+retry
  for both queries, and the "Select a property." prerequisite state. No change.
- **stay.tsx** — the only page that genuinely fell short of the standard.
  Its two query-backed sections used text-only loading messages instead of a
  skeleton, and its error states had no retry affordance. Fixed both, and
  added a properties-list error notice for parity with the other
  selector-driven pages.

Cross-page "select a property" pattern (item 5): ops-calendar, invoices,
documents, and stay each gate content behind a property selector. Their copy is
consistent *within* each family and I deliberately did not unify across
families — the ops surface uses the all-caps `SELECT A PROPERTY.` Archivo Black
voice (ops-calendar, matching its other `ops-empty` blocks), while the
owner/guest surfaces use sentence-case serif "Select a property." (invoices,
documents, stay). Both read as neutral prompts (never as errors), so this is a
design-system distinction, not an inconsistency. Unifying it would have been a
wording nit against two established design registers.

## What did NOT work

- First build attempt failed with `sh: 1: tsc: not found` because `npm ci` had
  not been run in this sandbox yet. `npm ci` installed the lockfile, after
  which `npm run build` and `npm run lint` both passed.

## Deviations from the plan

- None material. The audit found five of the seven pages already consistent,
  so the actual code change was concentrated on stay.tsx (skeleton + retry)
  and a small error-handling addition to ops-calendar.tsx. No data-fetching
  logic, API calls, or route structure was changed.

## Open questions

- `stay.tsx`'s "House guide" section derives from the already-loaded properties
  list rather than its own query, so it has no skeleton of its own. If a
  `propertyId` is present before the properties list resolves (URL state), it
  briefly shows "Select a property to see recorded access and emergency
  details." — an edge case left as-is since the selector cannot be populated
  before the list resolves.
- The ops surface's three content sections mention "The backend message is
  shown in the toast." in their error states while the owner surfaces do not
  reference the toast at all. I treated this as each family's established
  register and did not unify; flagging it in case a future pass wants one
  voice everywhere.
