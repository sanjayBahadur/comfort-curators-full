# Phase P9.35 — shop agent-surface registration + dashboard pending-purchase visibility

- **Date:** 2026-08-13
- **Agent/model:** opencode-go/gpt-5.6-luna
- **Status:** partial

## What I built (both parts)

Added agent surfaces for the package shop's catalog add controls and cart remove/quantity controls. Added a localStorage-backed pending-purchase external store, saved the current cart by property, cleared it after successful package activation, and rendered pending purchase details and subtotal in the owner task terminal.

## Files added or changed

- `app/src/lib/pending-purchase.ts` — new localStorage-backed store with same-tab notifications and native cross-tab `storage` event handling.
- `app/src/routes/package-shop.tsx` — three agent surface registrations, cart persistence, and activation cleanup.
- `app/src/components/owner/OwnerTaskTerminal.tsx` — pending purchase subscription and checklist group.

No CSS change was needed: the existing shared checklist and marker rules cover the new `data-kind` value.

## Exact surface ids/actions registered, and why each is safe (outside PaymentBoundary)

- `shop-catalog-add-${item.id}` — `click`; attached to each catalog ADD button.
- `shop-cart-remove-${line.item.id}` — `click`; attached to each cart remove button.
- `shop-cart-qty-${line.item.id}` — `focus`, `set`; attached to each cart quantity input.

All three surfaces are rendered before the existing `PaymentBoundary`. Nothing in the costs, rules, or ACTIVATE section was registered, and `useAgentSurface` also structurally refuses registration inside a payment boundary.

## The pending-purchase store's shape and how it's cleared

The store exposes `PendingPurchaseLine` with name, SKU, quantity, unit price minor units, and currency. Records use `cc_pending_purchase_${propertyId}`, maintain cached snapshots for `useSyncExternalStore`, notify local listeners on writes, and invalidate snapshots for cross-tab storage events. The shop writes on cart changes and activation success calls `clearPendingPurchase(propertyId)`.

## What I verified live vs. build/lint-only

- Build: passed with `npm run build`.
- Lint: passed with `npm run lint`; only pre-existing warnings in unrelated files were reported.
- Tests: passed with `npx vitest run` — 5 files, 27 tests.
- Diff whitespace: passed with `git diff --check`.
- Surface search: confirmed the three package-shop registrations and no registration in the PaymentBoundary section.
- Live browser flow: not verified. `http://localhost:3000` was unavailable, and no running app/test data was available to exercise the requested Chrome navigation flow.

## Decisions I made

Used the existing `useAgentSurface` hook and checklist visual structure without changing payment-boundary behavior. Used `formatMoney` for the computed minor-unit subtotal and labeled the group explicitly as pending/not yet activated.

## What did NOT work

The initial build/lint attempt could not run because dependencies were not installed; `npm ci` resolved that. There is no `npm test` script. The live verification could not start from an already-running app because port 3000 was unavailable.

## Open questions

The requested live add, dashboard visibility, and activation-clear flow still needs verification against the project's real running backend/data environment.
