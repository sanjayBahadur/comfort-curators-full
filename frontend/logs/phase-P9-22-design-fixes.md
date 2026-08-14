# Phase P9.22 — accepted design-review fixes

Date: 2026-08-10

## 1. Global header tracks

- Rebuilt the duplicated Owner, Property Detail, Onboarding, Owner Records, Ops, Package, and Expansion headers around explicit edge tracks: `136px` for the fixed Back control and `148px` for the fixed Superhost control. Brand, page locator, primary navigation, and page utility content now occupy separate interior grid tracks.
- Added intermediate responsive header layouts that move primary navigation into a deliberate ruled row before it can collide or wrap. Small-screen layouts reserve a top control row before rendering route-specific header content.
- Moved Ops `NEW TICKET` and `ACCESS DESK` into a persistent `WORKSPACE ACTIONS` sub-navigation band. The primary Ops route navigation no longer uses flex wrapping.
- Made Property Detail a real `PROPERTY` navigation destination and changed Package active matching to the exact `/properties/:id/package` route. `/properties/:id` no longer marks Package active.
- Kept Expansion’s `ESC / EXIT` and Package’s registration utility inside dedicated tracks immediately before the untouched Superhost slot.

## 2. User-facing copy

- Rewrote Staff login copy around triage, curator coordination, and scheduling.
- Rewrote Guest login copy around arrival details, house guidance, and stay essentials.
- Removed `Demo richness` and `Phase 2 demo` prefixes from seeded operational ticket reasons, including the balcony-latch incident shown on the Owner dashboard.
- Added a transition-specific description map for every property lifecycle state. Seeded lifecycle events now describe qualification, onboarding, remediation, readiness, activation, pause/suspension, and closeout distinctly while continuing to use backend-generated event timestamps.

## 3. Package Builder action

- Added a bottom-right `ADD +` control to every catalog product cell. Existing quantities are shown inline as `ADD + · N`.
- Added a single-item, 1.1-second `JUST ADDED` state using the red action accent. The persistent quantity badge was changed from red to ink so the transient newest addition is the only red product marker.
- Kept product quick view keyboard accessible and added a strong red focus outline to the add control.
- Replaced the two visibly stacked range sliders with one composed range control: a shared ruled track, a selected interval, two square handles, and explicit `MIN` / `MAX` price text.

## 4. Empty-state and dependency treatment

- Calendar now shows one unified property-scope explanation directly below the selector. Reservations, Feed Health, and Turnover Proposals each use a compact one-line `AWAITING PROPERTY SCOPE` row until a property is chosen.
- Documents now keeps the instruction in two places only: the selector and one concise empty-state sentence. The unselected hero prompt was removed, and the selector spans the full ruled band.
- Dashboard now renders its monthly activity and document rail in independently sized columns. Documents are sorted newest-first, capped at five recent entries, and followed by `VIEW ALL →` to `/documents`.

## Verification

- `npm run build` — passed.
- `npm run lint` — passed with exit code 0. The existing Fast Refresh/exhaustive-deps warnings are confined to pre-existing Superhost, agent-surface, and test files outside this change.
- `git diff --check` — passed.
- Added-line audit for all four prohibited corner, shadow, and gradient declarations — no matches.
- Protected-file diff audit — no changes to `SuperhostMount.tsx`, `Terminal.tsx`, `GlobalSuperhostDrawer.tsx`, `ControlSession.tsx`, or `GlobalSuperhostButton.tsx`.
- Exposed-copy grep — no remaining matches for `Demo richness`, `Phase 2 demo scenario activation`, or the removed Staff/Guest architecture language.
- Browser verification was intentionally not attempted in this sandbox, per the phase instructions.
