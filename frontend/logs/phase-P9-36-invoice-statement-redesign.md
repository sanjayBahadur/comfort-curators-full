# Phase P9.36 — invoices restyled as a real billing statement

- **Date:** 2026-08-13
- **Agent/model:** opencode-go/gpt-5.6-luna
- **Status:** partial

## What I built

Restyled `/invoices` as a scoped ledger statement while preserving the existing owner-records gate, selector, loading, property error, report error, empty-report, and aggregate total states. The populated report now has property top matter, service address, a STATEMENT label, currency, dense real count metadata, the existing three aggregate money totals, a procedural barcode footer, and dry aggregate-only fine print.

## Files added or changed

- Changed `app/src/routes/invoices.tsx`.
- Added `app/src/routes/invoices.css`, imported only by `invoices.tsx`.
- Added this phase log.
- Did not modify `owner-records.tsx`, `owner-records.css`, `documents.tsx`, `src/index.css`, backend code, or `src/components/superhost/`.

## How the barcode strip is drawn (confirm procedural, not an asset)

`app/src/routes/invoices.css` uses one `repeating-linear-gradient` with alternating `var(--ink)` and `var(--paper)` stops. No SVG or external asset was added.

## Confirmed no fabricated data — which real fields ended up on the page

The page uses `propertyName` and `propertyAddress` from the already-fetched property record, plus `currency`, `total_charges_minor_units`, `total_credits_minor_units`, `net_minor_units`, `charge_count`, `credit_count`, and `subledger_entry_count` from `PropertyContributionReport`. No invoice number, date, or line items were invented.

## What I verified live vs. build/lint-only, incl. the documents.tsx regression check

- `npm run build`: passed.
- `npm run lint`: passed with existing warnings in unrelated files.
- Stylesheet check: no `border-radius` or `box-shadow`; exactly one gradient declaration and it is the barcode `repeating-linear-gradient`.
- Vite served `/invoices` at `http://localhost:3000`.
- Live Chrome screenshots and populated `/invoices` verification were not possible: the environment has no `chrome`, `chromium`, or `chromium-browser` executable, and `npx playwright install chrome` could not install without root authentication.
- `/documents` was not screenshot-verified because of the missing browser. The shared primitives and `documents.tsx` were left untouched; the build passed.

## Decisions I made

- Kept the existing totals as the statement's three money fields rather than inventing per-charge rows.
- Used the selected property's existing display name and formatted service address as top matter.
- Used the report currency and all three report counts as label metadata.
- Scoped all new visual rules under `.invoice-page` so the documents surface keeps its existing stylesheet behavior.

## What did NOT work

The requested real Chrome Playwright verification could not run because the browser binary is unavailable and cannot be installed in this environment without authentication.

## Open questions

- The backend still exposes aggregate contribution totals only; a future per-charge detail endpoint would be required before this statement can honestly show individual line items.
- `AGENTS.md` was not present in the workspace, so there was no file available to read.
