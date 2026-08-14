# Phase P9.19 — Staff calendar grid

## Outcome

Rebuilt `/ops/calendar` so reservations are presented first as a real month calendar instead of a flat table. The existing property-scoped reservation, feed-health, and turnover-proposal APIs are unchanged.

## Implementation

- Added a six-week, seven-column month grid with Sunday-through-Saturday headings.
- Added previous month, today, and next month navigation plus a red today registration mark.
- Converted the existing reservation `start_at` and `end_at` values into property/reservation-timezone date ranges.
- Rendered each reservation as an ink bar across its check-in through check-out dates, splitting cleanly at week boundaries and stacking overlapping stays into separate lanes.
- Added selectable reservation bars with an accessible detail strip containing the existing guest window, status, and feed source information.
- Kept an empty month visible when a selected property's feed has no reservations so the primary surface still reads as a calendar.
- Moved Feed Health and Turnover Proposals below the calendar and suppressed their large pre-selection empty panels.
- Added a horizontally scrollable 840px calendar canvas for tablet-sized screens and a responsive detail/toolbar layout.
- Added a `prefers-reduced-motion` rule; no month-navigation motion was introduced.

## Visual constraints

- Warm paper backgrounds, true ink rules, mono date/metadata labels, and a single red today marker follow `ART-DIRECTION.md`.
- No gradients, shadows, rounded corners, pastel status chips, rotation, or torn/grain treatments were introduced.

## Verification

- `npm run build` — passed (Vite emitted its existing bundle-size advisory only).
- `npm run lint` — passed; seven existing warnings remain in unrelated Superhost/agent-surface files, with no calendar warnings.
- `git diff --check` — passed.
- Required forbidden-style grep — no matches in the introduced calendar changes.
- Read back `CalendarGrid.tsx`, `CalendarGrid.css`, `ops-calendar.tsx`, and `ops-calendar.css` in full.

Browser verification was intentionally left to the orchestrator, per the phase instructions.
