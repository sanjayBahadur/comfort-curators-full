# Phase P9.8 — wider cart, quieter filters, cart-preset "loadouts"

- **Date:** 2026-08-10
- **Agent/model:** opencode-go/deepseek-v4-pro
- **Status:** complete

## What I built (all three parts)

### 1. Wider cart column
Changed the rightmost grid track from `340px` to `440px` in `.shop-shell`. Scaled
the cart-row thumbnail from 32×32 to 40×40 and the heading grid from
`32px 1fr auto` to `40px 1fr auto` with `gap: 10px` (was `8px`) to give line
items breathing room.

### 2. Quieter filters
Reduced the left column width from `240px` to `210px` and tightened the visual
weight without removing any controls:

- Column padding: `24px 20px 32px` → `20px 16px 24px`
- Filter-group legend: `font-size: 11px` → `10px`, added `color: var(--ink-60)`,
  margin `12px` → `10px`
- Filter-group labels: `font-size: 10px` → `9px`, `min-height: 30px` → `26px`,
  checkbox column `18px` → `16px`. Default color set to `var(--ink-60)`;
  checked labels become `var(--ink)` via `:has(input:checked)`. This makes
  unchecked items recede so the column reads as a quiet utility.
- Search label: added `color: var(--ink-60)`, reduced `margin-top` from `28px`
  to `20px`, input height `38px` → `34px`
- Clear button: `font-size: 11px` → `10px`, `margin-top: 26px` → `20px`,
  added `color: var(--ink-60)`

All checkboxes, range inputs, legend text, and clear/reset buttons remain
fully functional. This is visual-weight reduction only.

### 3. Cart-preset "loadouts" (client-side, localStorage)

A lightweight named-cart-preset system added to the cart column:

- **Save**: When cart is non-empty, a `+ SAVE` button appears in the PRESETS
  section header. Clicking it reveals a name input; pressing Enter or clicking
  SAVE persists the current `{sku, quantity, monthlyUse}` pairs to
  `localStorage` under key `cc_shop_presets_{propertyId}`. No backend call.
- **Load**: Each saved preset shows LOAD / DEL buttons. Clicking LOAD when the
  cart is empty immediately replaces cart state with live-catalog-resolved
  items (matching by SKU, so prices/availability reflect current catalog data,
  not stale saved data). If the cart is non-empty, a confirm prompt appears
  inline ("DISCARD CART? OK —") to prevent accidental discard. Clicking OK
  replaces the cart; clicking — cancels.
- **Delete**: The DEL button removes the preset from state and localStorage.
- **Scope**: Presets are scoped per `propertyId` — switching properties
  reloads that property's presets from localStorage.

## How presets are stored, keyed, and resolved back to live catalog data

- **Storage key**: `cc_shop_presets_${propertyId}` in `window.localStorage`
- **Data shape**: `Record<string, Array<{sku: string, quantity: number, monthlyUse: number}>>`
  — preset name → array of SKU/quantity/usage triples
- **Resolution on load**: `handleLoadPreset()` iterates the saved entries, finds
  the corresponding `CatalogResource` in `allItems` by exact SKU match, and
  constructs `CartLine[]` objects with the live item. SKUs not found in the
  live catalog are silently skipped (product may have been removed).
- **Duplicate SKU handling**: If a preset contains the same SKU twice (e.g. from
  two past saves), quantities are merged into a single cart line.
- **No backend calls**: `handleSavePreset`, `handleLoadPreset`, and
  `handleDeletePreset` only touch `localStorage` and React state. The existing
  draft-save debounce effect for `createPackageDraft` still fires from the
  normal `cart` → `signature` → effect flow; preset loading replaces `cart`
  state, which triggers the same debounced draft creation as manual cart
  changes would — there is no separate activation path.

## Files added or changed

- **`app/src/routes/package-shop.css`** (modified): grid width, filter
  quieting, cart-row thumbnail sizing, ~150 lines of new preset UI CSS
- **`app/src/routes/package-shop.tsx`** (modified): +78 lines — Preset types,
  localStorage helpers, 4 new state variables, 1 new useEffect for
  propertyId sync, 4 handler functions, preset save/load/delete/confirm UI
  in the cart column

No new files created. `scripts/seed.ts` and `src/lib/catalog-images.ts` were
not touched (P9.7 scope).

## What I verified live vs. build/lint-only

- **`npm run lint` (oxlint)**: passes — no new warnings
- **`npm run build` (tsc + vite)**: passes — clean build
- **Live Chrome/Playwright verification**: NOT performed. This sandbox is
  missing system libraries (`libnspr4.so`) and cannot launch a Chromium
  browser. A `curl` check confirms the dev server serves HTTP 200 at
  `http://localhost:3000`, but no in-browser verification was possible.

The following are confirmed by static analysis and build/lint only:
- Grid computes to `210px minmax(0, 1fr) 440px`
- Checkbox DOM and event handlers are unchanged; CSS selectors preserve
  all checked/unchecked states
- `handleSavePreset` writes only to `localStorage.setItem` — no network call
- `handleLoadPreset` reads from in-memory `allItems` and sets cart state
  via `setCart` — no network call
- The confirm prompt uses `setSwapConfirmPreset(name)` / `setSwapConfirmPreset(null)`
  — pure React state, no network call

## Decisions I made

- **Grid widths**: 210px / 440px chosen to keep checkboxes usable at 210px
  (16px checkbox + label text still fits) and give cart line items real space
  at 440px. 460px made the catalog grid too narrow on common 1920px screens.
- **Thumb size**: 40×40 keeps the thumbnail identifiable without dominating
  the cart row at the new column width.
- **Confirm pattern**: Since this codebase has no existing confirmation dialog
  pattern (cart item removal and filter clearing are all instant/no-confirm),
  I created a minimal inline confirm — the LOAD/DEL buttons are replaced with
  a `gridColumn: "2 / span 2"` flex row showing the warning text and
  OK / — buttons. This matches the existing idiom: 1px black rules, JetBrains
  Mono labels, no modal, no `window.confirm`.
- **`:has(input:checked)` for checked filter state**: Modern CSS only, but
  the app already uses `clip-path`, `feTurbulence`, and `@container` queries —
  no IE11 compatibility requirement.
- **Preset UI placement**: Between the drop-label area and the cart rows,
  with a `PRESETS` mono header matching the cart heading's design language.
  The "NONE SAVED" empty state follows the `var(--ink-60)` / 9px JetBrains Mono
  convention from the rest of the cart column.
- **No preset rename/edit**: Out of scope for this phase. Save creates, LOAD
  replaces, DEL deletes.

## What did NOT work

- Playwright verification: this sandbox cannot launch a browser (missing
  `libnspr4.so` and other system libraries). All verification is build/lint
  only.
- `.shop-filter-group > label:has(input:checked)` — this works in all modern
  browsers. Not tested in ancient engines, but the app requires `clip-path`
  and `feTurbulence` already.

## Open questions

- Should preset loading also restore the substitution policy / approval
  toggles? Currently it only restores cart line items.
- Should the confirm UI include a count of items in the current cart vs.
  in the preset being loaded? Currently it just says "DISCARD CART?".
