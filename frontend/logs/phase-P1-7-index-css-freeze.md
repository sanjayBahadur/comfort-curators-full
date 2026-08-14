# Phase P1.7 — index.css final pass + freeze

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/gpt-5.6-luna
- **Status:** complete

## What I built

- Audited `src/index.css` against the shared art-direction tokens and replaced undefined legacy references with the existing tokens: `font-meta`, `font-shout`, `ink-60`, `ink-40`, and `ease-expo-out`.
- Replaced the cursor's hardcoded white with `var(--paper)` and removed the cookie slip shadow. The cookie slip tape now uses `var(--ink)` with opacity rather than a duplicate color literal.
- Tightened the primitive CSS to use `var(--rule)` and `var(--radius)` consistently.
- Added section `09 / PRIMITIVES` to `/debug`, with live Select, Modal, Tooltip, Popover, and CookieSlip controls. The footer is now `10 / END OF PROOF`.

## Files added or changed

- `app/src/index.css`
- `app/src/components/ui/CookieSlip.css`
- `app/src/components/ui/Modal.css`
- `app/src/routes/debug.tsx`
- `logs/phase-P1-7-index-css-freeze.md`

## Decisions I made

- Appended the primitives section rather than inserting it into the existing proof sequence, preserving all existing section content and making it the new highest numbered section.
- The consent reset removes `cc_cookie_consent` and increments a React `key`, remounting `CookieSlip` so its session-persisted choice can be exercised without manual storage editing.
- Manual verification: choose each Select option and confirm the live value changes; open and close the Modal with its close control and Escape; hover/focus the Tooltip trigger; open the Popover and dismiss it with Escape or outside click; accept either CookieSlip choice, then use `RESET CONSENT + SHOW` to display it again.
- The only remaining `grep -n "border-radius\|box-shadow" src/index.css` hit is `border-radius: var(--radius)`, where `--radius` is `0px` by design. There are no shadow hits.

`src/index.css` is now frozen. Any later block that thinks it needs to touch it must stop and escalate instead, per `ORCHESTRATION.md` §3.

## What did NOT work

- The first build and lint attempts were blocked because `node_modules` was absent. `npm ci` restored the locked dependencies; the required checks then passed.

## Deviations from the plan

- No route-specific CSS was added because the existing debug stylesheet and shared control classes were sufficient for the primitive demonstration.

## Open questions

- None.
