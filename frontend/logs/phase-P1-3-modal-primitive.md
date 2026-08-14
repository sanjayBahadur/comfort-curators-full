# Phase P1.3 — Modal primitive

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/gpt-5.6-luna
- **Status:** complete

## What I built

Added a reusable controlled `Modal` primitive with a caller-supplied mono index label, title, arbitrary children, close button, Escape dismissal, focus trapping, and focus restoration to the triggering element. It is rendered through a `document.body` portal so it has a reliable stacking context without using the native `<dialog>` top layer. The package-shop call site remains unchanged for P1.6.

## Files added or changed

- `app/src/components/ui/Modal.tsx`
- `app/src/components/ui/Modal.css`
- `logs/phase-P1-3-modal-primitive.md`

## Decisions I made

- Used a fixed portal layer with no scrim so the page remains visible behind the popup.
- Used a `1.5deg` rotation, within the `+-3deg` readable-content limit, rather than the decorative `-12deg` to `-15deg` band.
- Used the specified `--ease-overshoot` curve for entry (`500ms`) and a clip-path/slide exit with `--ease-wipe` (`350ms`), never a plain fade.
- The dialog has `role="dialog"`, `aria-modal="true"`, and a generated title ID wired through `aria-labelledby`.
- Focus trapping is handled by the dialog keydown handler: Tab and Shift+Tab wrap through current focusable descendants, while Escape calls `onClose`. The opening active element is retained in a ref and focused when closing, including unmount cleanup for conditional callers.
- The primitive documents the caller rule that at most one popup should be shown per session; session counting is intentionally not part of this component.
- Reduced motion removes translation and rotation and keeps a short opacity transition.

## What did NOT work

- The first build and lint attempts could not start because `app/node_modules` was absent (`tsc: not found`, `oxlint: not found`). After `npm install`, both commands completed successfully.

## Deviations from the plan

- No deviations from the requested primitive-only scope. `src/routes/package-shop.tsx` was read for integration shape but not edited.

## Open questions

- There is no test harness in this repository. Accessibility behavior was verified by tracing the portal, focus refs, keydown handler, and cleanup/effect paths in `Modal.tsx`; P1.6 should verify the real quick-view trigger integration manually.
