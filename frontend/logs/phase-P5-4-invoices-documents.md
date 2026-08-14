# Phase P5.4 — /invoices, /documents

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/deepseek-v4-flash
- **Status:** complete

## What I built

Two owner-facing routes, both property-scoped through the URL
(`?property=` search param, persisted with `replace: true`) using the same
`getOpsProperties()` + `Select` selector pattern established by
`ops-calendar.tsx` and `ops-properties.tsx`.

**`/invoices`** — reads `GET /v1/reports/property-contribution` (all-time,
`?property_id=` scoped). Renders a monthly-summary card with total charges,
total credits, and net — every amount through `<Money>` — plus real
`charge_count`, `credit_count`, and `subledger_entry_count`. Empty state is
exactly `"No charges yet."` when `charge_count === 0 && credit_count === 0`
(a `null` report is treated as empty too). Does **not** build an itemized
charges/credits table, because the endpoint returns only aggregate totals; an
honest note on the page says there is no reachable line-item endpoint.
Loading skeleton, API error + retry state, and a "select a property" prompt.

**`/documents`** — reads `GET /v1/properties/{id}/documents`. Table columns:
name (title + id), type, status, uploaded date. `created_at` is the real field
used for the uploaded-date column (there is no separate `uploaded_at`). A
stubbed upload form (title, document type, optional expiry) POSTs metadata only
to `/v1/documents` — no file picker, no MinIO. On success it invalidates the
property documents query and the owner dashboard documents panel, resets the
form, and toasts "Document recorded (metadata only)". The form is labelled
METADATA ONLY. Loading/error/empty/populated states for the list.

Both pages share a new `owner-records.tsx` / `owner-records.css` (owner gate,
header with DASHBOARD / ONBOARD / INVOICES / DOCUMENTS / ACCESS DESK nav,
skeleton, summary cards, table, form, empty/error states).

## Files added or changed

- Added `app/src/lib/api/owner.ts` — `PropertyContributionReport`,
  `OwnerDocumentData` types, `getPropertyContributionReport`,
  `getPropertyDocuments`, `createDocument`.
- Added `app/src/routes/owner-records.tsx` — shared `OwnerGate`,
  `OwnerRecordsHeader`, `OwnerRecordsSkeleton`.
- Added `app/src/routes/owner-records.css` — shared styling for both pages.
- Added `app/src/routes/invoices.tsx`.
- Added `app/src/routes/documents.tsx`.
- Added `logs/phase-P5-4-invoices-documents.md`.

`src/index.css`, `src/main.tsx`, and `src/lib/auth/roles.ts` were not touched
(`allows`/`navFor` already anticipate both paths for `owner`).

## Decisions I made

- **No itemized list.** The block's live verification of the backend handler
  is authoritative: `GET /v1/reports/property-contribution` returns aggregate
  totals only. I built the page around `total_charges_minor_units`,
  `total_credits_minor_units`, `net_minor_units`, `currency`, and the real
  counts, and surfaced the missing itemization endpoint as an open question
  below plus a short honest note in the UI.
- **Response-shape tolerance for the report.** `INTEGRATION.md §10` records the
  live service returning a one-item collection; the P5.4 task re-verified the
  handler and saw a flat report object. `getPropertyContributionReport` accepts
  both (flat object or `{ items: [...] }`, envelope or unwrapped) so the page
  keeps working across that transition. `null`/empty reads as "No charges yet."
- **Stubbed upload body.** Backend source was not mounted in this sandbox, so
  `handleCreateDocument`'s exact required body could not be read. I POST only
  fields that provably exist on the `Document` model: `property_id`, `title`,
  `document_type`, and optional `expires_at` (RFC3339 via
  `new Date(date + "T00:00:00").toISOString()`). `document_type` is a free-form
  text field rather than an invented enum, because no verified list exists.
- **`created_at` as the uploaded date.** There is no `uploaded_at` field; the
  list uses `created_at`. Column header is `UPLOADED` per `SCREENS.md`.
  (`INTEGRATION.md` says the dashboard should describe `created_at` as "Added" —
  that guidance is for the dashboard's documents panel; this is the dedicated
  `/documents` screen whose spec column is "uploaded date". The stubbed nature
  of upload is stated explicitly on the page.)
- **Section numbers.** Owner pages use header spans 01 (dashboard), 02
  (onboarding), 03 (property detail); these two continue as 04 / INVOICES and
  05 / DOCUMENTS.
- **Shared CSS.** The two new routes share `owner-records.css` to avoid
  duplicating the owner header, selector, table, form, and empty/error styles
  twice; the existing owner pages were left untouched (behavior-preserving
  change only).

## What did NOT work

- `app/node_modules` was absent, so `npm ci` was required before `tsc -b` and
  `oxlint` could run. After install, both passed on the first attempt.
- The backend checkout referenced by the task (`internal/billing/service.go`,
  `internal/documents/handler.go`, `internal/documents/models.go`) is not
  mounted in this sandbox, so I could not read `handleCreateDocument`'s exact
  body fields or re-confirm the report route's envelope myself. I relied on the
  block's verified response shapes and `INTEGRATION.md`.

## Deviations from the plan

- None functionally. `main.tsx` was left untouched per instructions; the two
  route patches are below for the orchestrator.
- `document_type` is a free-form text input, not a fixed select, because the
  valid backend document-type values are not verifiable from this sandbox.

## Open questions

- **Itemization endpoint missing.** The backend has no reachable
  charges/credits *listing* endpoint (only `POST /v1/billing/charges` and
  `POST /v1/billing/credits`), so `/invoices` shows aggregate totals only. If a
  line-item endpoint lands later, the summary card can gain a real detail table.
- **`POST /v1/documents` exact body.** Confirm `handleCreateDocument`'s
  required fields and whether `document_type` is enum-validated. The form
  currently sends `property_id`, `title`, `document_type`, optional
  `expires_at` and free-text type.
- **Report response envelope.** Flat object vs `{ items: [...] }` — the client
  tolerates both; the live response should be locked down once the backend is
  reachable.

## Route patches for the orchestrator

Imports for `main.tsx`:

```tsx
import Invoices from "./routes/invoices";
import Documents from "./routes/documents";
```

```tsx
<Route path="/invoices" element={<RequireRole allow={["owner"]}><Invoices /></RequireRole>} />
<Route path="/documents" element={<RequireRole allow={["owner"]}><Documents /></RequireRole>} />
```
