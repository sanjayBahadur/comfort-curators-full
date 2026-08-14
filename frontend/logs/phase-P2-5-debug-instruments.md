# Phase P2.5 — dev instruments moved to /debug

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/deepseek-v4-flash
- **Status:** complete

## What I built

- Added section `10 / INSTRUMENTS` to `src/routes/debug.tsx`, after the
  `09 / PRIMITIVES` section P1.7 owns (that section is untouched), and
  renumbered the closing footer from `10 / END OF PROOF` to
  `11 / END OF PROOF` so the page numbering stays sequential.
- The section restores the three Phase-1 acceptance instruments P2.2 cut
  from `/login` (DEF-11 move, do not delete), adapted to the debug page's
  existing `control-grid` / `control-panel` / `light-field` / `dark-field` /
  `api-console` / `section-index` patterns:

  1. **Money-formatting proof** (light panel, `instrument-money`): renders
     `<Money value={45000} currency="INR" className="money-proof" />` with a
     magenta `PROOF ONLY` stamp, the `OPERATING SAMPLE / INR` label, and the
     "Rendered from 45000 minor units — no float math" note. Reuses the
     existing `money-proof` class from `index.css`; the stamp was rebuilt as
     `instrument-stamp` (same torn-paper look, page-fitting text instead of
     `NOT A REAL LOGIN`).
  2. **Session console** (dark panel): shows `ACTIVE ROLE`, `TOKEN CHECK`
     (`sessionNote`), and `AUTHENTICATED PROPERTIES` (`propertyCount` fetched
     via `api("/v1/properties")`), plus the role-scoped links to the owner
     dashboard, operations desk, and inventory shop, a `MINT OWNER SESSION`
     control (mirrors the page's own auto sign-in as owner), and
     `CLEAR SESSION` (disabled when no role is active).
  3. **Forced API error proof** (full-width `api-console` block): a
     `FORCE A REAL ERROR →` button calling
     `api("/v1/properties/not-a-real-property")` after ensuring a token
     exists. The shared client surfaces the backend's real message through
     the global toast; the same caught `ApiError` (status / code / message /
     request_id) is also rendered inline in `--danger` red.

## Files added or changed

- `app/src/routes/debug.tsx` — new `10 / INSTRUMENTS` section + footer
  renumber; imports for `Money`, `Link`, `getRole`/`signOut`/`Role`,
  `useEffect`, and `./debug.css`.
- `app/src/routes/debug.css` — new route-scoped CSS for the instrument
  panel/stamp/session-list/error styles (the page previously had no route
  CSS; this follows the login.css / ops.css / onboarding.css convention).
- `logs/phase-P2-5-debug-instruments.md` — this log.

## Decisions I made

- Read `logs/phase-P2-2-login-rework.md` in full first and adapted the
  verbatim JSX it captured rather than copying it, per the block brief.
- Reused existing global classes where they already fit (`control-grid`,
  `control-panel`, `light-field`, `dark-field`, `specimen-label`,
  `button-row`, `button button-outline/solid`, `api-console`, `api-error`,
  `money-proof`) and only added route-scoped `debug.css` for the bits with
  no existing home (stamp, session list, error block spacing). This kept
  `src/index.css` untouched.
- The stamp text changed from `NOT A REAL LOGIN` to `PROOF ONLY` because
  the instrument no longer lives on the login page; the "this is a proof,
  not production UI" intent is preserved.
- `forceApiError` mints an owner session first when no token exists,
  matching the debug page's established auto sign-in pattern (the original
  `"Choose a demo role first"` guard does not translate to `/debug`).
- The session console refreshes itself whenever the page's existing
  `/v1/properties` query succeeds, so it reflects the owner session the page
  mints on load instead of showing a stale "NO ROLE SELECTED".
- Renumbered the footer to `11 / END OF PROOF` so section numbering stays
  sequential after inserting `10 / INSTRUMENTS`.
- Kept P2.2's empty-catch convention (comment noting the shared client
  already surfaces the backend message) so no double toasts are raised.

## What did NOT work

- First build run failed with `tsc: not found` because `node_modules` was
  not installed; ran `npm ci` and rebuilt successfully.
- Initial build after wiring surfaced `error TS2554: Expected 1 arguments,
  but got 0` on `refreshSession()` from the mount effect; fixed by passing
  the explicit `"Session restored from this tab"` note.

## Deviations from the plan

- None of substance. The section landed as `10 / INSTRUMENTS` directly
  after `09 / PRIMITIVES` (footer renumbered to 11), which was the
  allowed "your call" for numbering.

## Open questions

- None.
