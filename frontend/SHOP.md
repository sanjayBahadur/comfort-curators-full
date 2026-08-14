# The Inventory Shop — interaction spec

`/properties/[id]/package`

The centrepiece. An owner browses our warehouse, filters it, drags what they want
into a cart, and watches the monthly cost of running their property assemble
itself. **This screen is the product.**

Read `ART-DIRECTION.md` first. This document is layout and behaviour only.

---

## 1. One window

Everything on one screen. No wizard, no step-through, no navigating away. Browse,
filter, drag, price — all visible simultaneously.

```
┌────────────────────────────────────────────────────────────────────────────┐
│ 01 / INVENTORY          GOMTI RIVERSIDE 2BHK              ⊹ registration ⊹ │
├──────────────┬──────────────────────────────────────────┬──────────────────┤
│              │                                          │                  │
│  FILTERS     │   ┌────────┐ ┌────────┐ ┌────────┐       │  02 / YOUR CART  │
│              │   │        │ │        │ │        │       │                  │
│  CATEGORY    │   │ [img]  │ │ [img]  │ │ [img]  │       │  ┌────────────┐  │
│  ▪ linen  12 │   │        │ │        │ │        │       │  │ BATH TOWEL │  │
│  ▫ bath    8 │   │ TOWEL  │ │ SHEET  │ │ SOAP   │       │  │ ×6   ₹450  │  │
│  ▫ kitchen 6 │   │ ₹450   │ │ ₹1,250 │ │ ₹95    │       │  └────────────┘  │
│  ▫ decor   9 │   └────────┘ └────────┘ └────────┘       │                  │
│              │                                          │  ┌────────────┐  │
│  PRICE       │   ┌────────┐ ┌────────┐ ┌────────┐       │  │ SOAP BAR   │  │
│  ├─────●───┤ │   │        │ │        │ │        │       │  │ ×12   ₹95  │  │
│              │   │        │ │        │ │        │       │  └────────────┘  │
│  LABEL       │   └────────┘ └────────┘ └────────┘       │                  │
│  ▪ curators' │                                          │  ╌╌╌╌╌╌╌╌╌╌╌╌╌╌  │
│  ▫ owner     │            ↓ scrolls ↓                   │  SETUP           │
│  ▫ alt       │                                          │  ₹2,700.00       │
│              │                                          │                  │
│  [ CLEAR ]   │                                          │  MONTHLY         │
│              │                                          │  ₹5,400.00       │
│              │                                          │                  │
│              │                                          │  [ ACTIVATE ]    │
└──────────────┴──────────────────────────────────────────┴──────────────────┘
   240px fixed          fluid, scrolls                        340px sticky
```

Separated by **1px black rules**, not gaps. Only the centre column scrolls.

Below 1100px: cart collapses to a sticky bottom bar showing item count and
monthly total; tapping expands it as a sheet. Filters become a top drawer.
**Drag still works on touch** — dnd-kit `TouchSensor` with a 200ms delay so
scrolling isn't hijacked.

## 2. Filters — instant, client-side

`GET /v1/catalog/items` returns everything. Filter in memory. **No refetch, no
spinner, no debounce.** Instant is the whole feel.

- **Category** — multi-select checkboxes, live counts in mono beside each
- **Price** — dual-handle range slider, values in mono
- **Label** — `curators_standard` / `owner_preferred` / `alternative`
- **Search** — matches name and SKU, `/` focuses it
- **Clear** — appears only when a filter is active

Filter state lives in the **URL query string**, so a filtered view is shareable
and survives reload.

Filter labels are mono uppercase, 11px. Checkboxes are 12px squares with a 1px
black rule — filled solid black when on. No custom checkbox library.

Result count above the grid: `24 ITEMS · 3 FILTERED` in mono.

## 3. The item card

```
┌──────────────────┐
│                  │  ← square image well, --paper-2
│      [img]       │    halftone overlay at 12% on hover
│                  │
├──────────────────┤  ← 1px rule
│ BATH TOWEL       │  Archivo 14px, uppercase
│ 500GSM · LINEN   │  mono 11px, --ink-60
│ ₹450.00          │  mono 13px, --ink
└──────────────────┘
```

- Whole card is the drag handle. `cursor: grab` → `grabbing`.
- Hover: `translateY(-2px)`, rule thickens to 2px, halftone fades in. **No shadow.**
- Already in cart: a small `--red` mono badge top-right showing quantity, and the
  card sits at 60% opacity. Still draggable to increment.
- No "Add to cart" button. **Dragging is the interaction.** A keyboard-accessible
  fallback lives in the card's context menu (§7).

Grid: `repeat(auto-fill, minmax(200px, 1fr))`, 1px black gaps achieved with a
background-black grid container.

## 4. Drag and drop

**dnd-kit.** `PointerSensor` (8px activation distance) + `TouchSensor` (200ms delay).

**Drag ghost** — the real card at 90% opacity, rotated `-2deg`, following the
cursor. Not an outline, not a shrunken clone. It should feel like you picked up
a physical card.

**Cart drop zone** — while dragging: 2px dashed black border, background shifts to
`--paper-2`, and a mono label appears: `DROP TO ADD`. No colour flood.

**On drop:**
1. Card animates into the cart list (~200ms, overshoot easing)
2. Cart row appears at quantity 1
3. Cost panel dims to 60% while the server recomputes
4. New totals cross-fade in over 150ms
5. **First item only:** a hand-drawn `--red` marker circle briefly appears around
   the monthly total, then fades. Once per session — the charm is in the restraint.

**Drag out of cart to remove** — dragging a cart row back over the grid removes
it, with a torn-paper exit animation.

## 5. Cart rows

```
┌────────────────────────────────┐
│ BATH TOWEL 500GSM          [×] │  Archivo 13px
│ ₹450 ea                        │  mono 11px --ink-60
│ QTY   [ − ] 6 [ + ]            │  mono; square 1px-rule buttons
│ MONTHLY USE  [ 12 ]            │  mono; inline number input
└────────────────────────────────┘
```

`quantity` and `expected_monthly_consumption` are both editable inline. Every
change re-POSTs the draft (debounced 400ms).

## 6. Cost — server-authoritative, always

**Never compute cost in the frontend.** On any change, POST the whole draft to
`POST /v1/properties/{id}/packages` and render the response:

```jsonc
{ "setup_cost_minor_units": 270000,      // ₹2,700.00
  "monthly_cost_minor_units": 540000,    // ₹5,400.00
  "monthly_consumption_units": 12,
  "currency": "INR" }
```

- Debounce 400ms so a drag doesn't fire ten round-trips
- In flight: cost panel at 60% opacity. **No spinner.** It should feel like it is
  settling, not loading.
- Values cross-fade — old out, new in, 150ms. **Never a count-up animation**; the
  number comes from the server and animating it would imply client-side maths.
- Setup and monthly are Instrument Serif at 32px, tabular numerals. The labels
  above them are mono 11px.

Below the totals, in italic Instrument Serif: *"recalculated by our warehouse"* —
small, `--ink-60`. It is true, and it tells the owner the number is real.

## 7. Policy drawer

Collapsed by default under the totals — `03 / RULES`.

- `substitution_policy` — three radio blocks: `OWNER APPROVAL` · `AUTOMATIC` ·
  `RESTRICTED`. Selected is solid black with white type.
- Approve price increases · Approve new SKUs — toggles
- Monthly budget limit — optional, mono input

## 8. Activate

Full-width black block, Archivo Black uppercase, white type.
Disabled (1px rule, `--ink-40` type) until the cart has at least one item — the
API rejects an empty package.

On click → `POST .../packages/{version_id}/activate`, then:
- Badge flips `DRAFT` → `ACTIVE`, the word set in `--red`
- A brief misregistration flicker on the badge (§ART-DIRECTION 7)
- Cart rows get a thin `--red` rule on the left
- **No confetti.** No success modal. A quiet, confident state change.

## 9. States

| State | Treatment |
|---|---|
| Loading | Card-shaped skeletons in `--paper-2`. No shimmer — a slow 2s opacity pulse. |
| Empty cart | Centered in the cart column: **"empty."** Archivo Black 24px, then *"for now."* in italic Instrument Serif, `--ink-40`. |
| No filter results | `"nothing matches."` + a `CLEAR FILTERS` link |
| Save error | Toast with the API's `message` verbatim, mono. Cart keeps its last good state — **never clear it on error.** |
| Offline | Mono strip at the top of the cart: `OFFLINE · CHANGES NOT SAVED` on `--red` |

## 10. Delight — budget: three

More than three and it becomes a toy.

1. **First drop** — the marker circle around the monthly total (§4)
2. **Empty cart** — the *"for now."* italic
3. **Activate** — the misregistration flicker

Everything else stays calm. The restraint is what makes these land.

## 11. Acceptance

```
1. Filter by category → grid updates instantly, no network request
2. Drag three items in → they appear in the cart with the pickup animation
3. Setup + monthly costs appear and change with quantity
4. Network tab: cost values come from an API response, never computed in JS
5. Change quantity → one debounced POST, not ten
6. Activate → badge flips DRAFT → ACTIVE
7. At 390px: cart is a bottom bar, drag still works by touch
8. Reload with filters set → filters restore from the URL
9. Costs render as ₹2,700.00, never 270000
10. No rounded corners. No shadows. Grain overlay present.
```
