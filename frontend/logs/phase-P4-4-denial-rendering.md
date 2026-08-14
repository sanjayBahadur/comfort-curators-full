# Phase P4.4 — denial rendering in --red inside the terminal

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/deepseek-v4-flash
- **Status:** complete

## What I built

This was a verification-and-targeted-fix block, not greenfield rendering. The
denial line already existed (`.superhost-terminal-line-denial` in `Terminal.css`
and the `PolicyDenied.v1` mapping in `behavior.ts`); I verified each of the five
checks and made two small fixes.

Per-check verdicts, stated plainly:

1. **Reason text shown verbatim — already correct, no change.**
   `behavior.ts`'s `PolicyDenied.v1` case passes `value(data, "reason")` through
   unmodified (`text: value(data, "reason")`); `Terminal.tsx` renders `line.text`
   directly. No prefix/wrap/truncation is applied to the reason itself — the
   only prefix is the `!` line glyph, which is terminal chrome, rendered in a
   separate `aria-hidden` span, not part of the reason text. Contract's "shown
   verbatim, short human-readable explanation" holds.

2. **Exact `--red` value — already correct, no change.**
   `.superhost-terminal-line-denial { color: var(--red); }` resolves to the root
   token `--red: #ff0000` (`src/index.css`). No hardcoded hex, no
   terminal-flavored shade. It is the same red used everywhere else in the app,
   as `ART-DIRECTION.md §14` requires.

3. **Nothing else in the terminal turns red accidentally — verified + one
   clarifying comment.**
   Grep over `Terminal.css`, `ConfirmBlock.css`, and `behavior.ts` finds exactly
   two `--red` usages in the terminal scope: the denial line rule, and
   `.superhost-confirm-error`. The latter is a failed decision-post (a network
   hiccup / transient 500 on the `/decide` POST), not a policy denial — a
   different class of thing. Judgment call: I kept it on `--red` (it is the
   same "the machine refused/failed" signal family, and it already carries
   `role="alert"`), and added a comment in `ConfirmBlock.css` documenting that
   it is distinct from the denial line so a future anti-slop reviewer does not
   flag it as a second denial treatment. No general terminal chrome uses red.

4. **Screen-reader / keyboard treatment — fixed one gap.**
   The `aria-label="Superhost activity"` on the line list gives the region a
   name but is not a live region, and a red denial is otherwise purely visual.
   To give a non-visual "this was denied" signal, denial text now renders with
   `role="alert"` (`Terminal.tsx`), matching how `ConfirmBlock`'s error state
   already announces itself. Because `role="alert"` is an assertive live region,
   I also made `useTerminalStreamView` skip the typewriter for denial lines
   (`behavior.ts`) — otherwise the per-tick text mutation would re-announce
   partial denial text dozens of times. A denial is a refusal, not a narrative
   line; it renders whole. The `!` prefix stays `aria-hidden`, so the alert
   announces only the verbatim reason.

5. **`/debug` demos — verified correct.**
   Both demos still render the denial correctly. Section 11's static demo passes
   a denial `TerminalLine` ("policy denied: payment needs human approval.")
   straight to `Terminal`, producing the `!`-prefixed red line with the new
   `role="alert"` span. `SuperhostBehaviorDemo`'s `demo-denial` event
   (`PolicyDenied.v1`, reason "Payment requires an owner approval.") flows
   through `eventToTerminalLine` → denial kind → same rendering. No changes
   needed to the demo data.

## Files added or changed

- `app/src/components/superhost/Terminal.tsx` — denial text span gets
  `role="alert"`.
- `app/src/components/superhost/behavior.ts` — denial lines skip the typewriter.
- `app/src/components/superhost/ConfirmBlock.css` — clarifying comment on
  `.superhost-confirm-error` (no rule change).
- `logs/phase-P4-4-denial-rendering.md`

`src/index.css`, `src/main.tsx`, and `Terminal.tsx`'s line-rendering structure
were untouched, per the block constraints.

## Decisions I made

- Kept `ConfirmBlock`'s error state on `--red` with a documenting comment
  rather than changing it to a different hue — it is a decision-flow error, not
  terminal chrome, and shares the "refusal/error" semantics with the denial.
- Chose `role="alert"` over a broad `aria-live` on the whole line list. A
  container-wide live region would re-announce every typewriter tick for every
  line kind; scoping the alert to denials (with the typewriter disabled for
  them) gives the one assertive signal the check was about without noise.
- Put `role="alert"` on the text span, not the `<li>`, to preserve the list's
  `listitem` semantics.

## What did NOT work

- `npm run build` initially failed with `tsc: not found` because dependencies
  were absent; `npm install` resolved it, and build + lint then passed.
- No live backend thread was available in this sandbox, so a real
  `PolicyDenied.v1` from the wire could not be exercised end to end; the mock
  `/debug` event path covers the mapping and rendering.

## Deviations from the plan

- None material. Checks 1, 2, and 5 were already correct and required no code
  changes; the work was confirming them and making the two targeted a11y fixes
  in checks 3 (comment only) and 4.

## Open questions

- The whole line list has no live region, so non-denial lines (proposals,
  completions) are not announced to screen-reader users as they stream in.
  That is a general terminal concern beyond this block's denial scope; worth
  revisiting when the terminal is mounted on the four P4.5 surfaces.
- The `superhost-stream.yaml` contract file is not present in this repo (it
  lives in the backend); the verbatim-reason requirement was verified against
  the mapping code and the contract quote in the block brief rather than the
  YAML itself.
