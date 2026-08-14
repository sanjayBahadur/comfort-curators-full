# Phase P1.4 — CookieSlip

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/gpt-5.6-luna
- **Status:** complete

## What I built

Added a standalone `CookieSlip` consent component. It renders as a bottom-left torn paper slip with a red header accent, halftone corner, tape strip, house-voice copy, equal-prominence consent controls, and the `PRIVACY · 01` footer index. Both choices persist in `localStorage`, and the slip tears away with a clip-path, rotation, and opacity animation after either choice.

## Files added or changed

- `app/src/components/ui/CookieSlip.tsx`
- `app/src/components/ui/CookieSlip.css`
- `logs/phase-P1-4-cookie-slip.md`

## Decisions I made

- Used `localStorage` with the `cc_cookie_consent` key because consent must survive visits. The existing app uses direct browser storage access and has no storage helper to extend.
- Used `cubic-bezier(.34, 1.56, .64, 1)` with `500ms` for entry, following `INTERACTION.md §3` rather than ART-DIRECTION.md §9's corrected stale example. Exit uses the existing wipe curve for `420ms` and animates clip-path, rotation, and opacity together.
- Treated the angled red header type as a decorative accent band, using `1.5deg`; body copy and controls remain unrotated and readable.
- Verified equal visual weight in CSS: both buttons use the same `min-height`, padding, `font-size`, `font-weight`, letter spacing, line height, and `1px` border width. Only the required solid-versus-outline fill differs.
- Used a 1px visible focus outline with a 3px offset for both keyboard-operable buttons.
- Reduced motion removes translation and rotation while retaining a short opacity and clip-path entrance/exit.
- Kept the component unmounted when consent is already stored and did not wire it into `main.tsx` or a route.

## What did NOT work

- The first build and lint attempts could not start because dependencies were not installed (`tsc: not found`, `oxlint: not found`). After `npm install`, both checks passed. npm reported three existing audit findings during installation; no audit fix was applied.

## Deviations from the plan

- No deviations from the requested component-only scope. `src/index.css`, `main.tsx`, and routes were not changed.

## Open questions

- There is no test harness for this component in the repository. The component was verified through TypeScript/build, oxlint, diff checks, and source inspection; the mounting block should verify the real consent flow visually when it integrates the component.
