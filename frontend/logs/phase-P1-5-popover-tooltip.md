# Phase P1.5 — Popover + Tooltip

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/deepseek-v4-flash
- **Status:** complete

## What I built

Two paper-block primitives in `src/components/ui/`, matched to the `Select`/`Modal`
family conventions (paper panel, `--radius: 0`, 1px `--rule` black border, no shadow,
red `:focus-visible` outline), built as primitives only — neither is wired into any
page, per the P1.2–P1.4 pattern.

- **`Tooltip`** — a stateless wrapper around a single trigger element. It clones the
  trigger to inject `aria-describedby` pointing at a generated `role="tooltip"` node,
  and shows the bubble on **hover** (`:hover`) **and keyboard focus**
  (`:focus-within`) via CSS — so it already does better than the native `title`
  it's meant to replace (which is mouse-only and inaccessible). No entrance drama: a
  short show-delay (160ms) followed by a 140ms fade/slide, using `--ease-expo-out`,
  never `--ease-overshoot`. `pointer-events: none` so the bubble never intercepts a
  click aimed at whatever sits below it.
- **`Popover`** — controlled (`open` / `onClose`, same contract as `Modal`), takes a
  `trigger` element and rich `children`. The trigger is cloned to receive
  `aria-haspopup="dialog"`, `aria-expanded`, and `aria-controls`; the panel is
  `role="dialog"` with a required `aria-label`. `Escape` and outside `pointerdown`
  both close it and restore focus to the trigger. On open, focus moves to the first
  focusable descendant (or the panel itself). Rendered inline (positioned absolutely
  relative to the trigger wrapper), matching how `Select` positions its listbox
  rather than `Modal`'s body-portal approach.

## Files added or changed

- `app/src/components/ui/Tooltip.tsx`
- `app/src/components/ui/Tooltip.css`
- `app/src/components/ui/Popover.tsx`
- `app/src/components/ui/Popover.css`
- `logs/phase-P1-5-popover-tooltip.md`

## Decisions I made

- **Tooltip show/hide is pure CSS, no React state.** Show/hide is driven entirely by
  `:hover` / `:focus-within` on the wrapper, with `visibility` used to remove the
  bubble from the accessibility tree when hidden. This is the genuinely simple
  implementation — no timers, no event listeners, no open/close state to leak. The
  only knob is an `id` override (for cases where the caller needs a stable,
  predictable id).
- **`aria-describedby` (and `aria-haspopup`/`expanded`/`controls` for Popover) are
  injected into the trigger via `cloneElement`** rather than a wrapper element,
  because the association must live on the actual focusable trigger to be announced.
- **Popover is controlled, not self-managing.** Same `open`/`onClose` contract as
  `Modal` keeps the family consistent and avoids hidden internal state plus a
  close-escape-hatch (e.g. a render-prop). The caller owns the trigger's `onClick`
  (one line to toggle) and content actions close via `onClose`.
- **No focus trap in Popover.** Chose explicitly: on open we move focus into the
  panel (first focusable), but we do not trap it — Tab past the last item exits to
  the page naturally, and because focus can leave, `Escape` is handled on a
  **document** listener so it still closes from anywhere. A full modal-style trap is
  overkill for a small, contextual, click-elsewhere-dismissible popup; this matches
  the a-philosophy-of-software-design rule against copying a heavier pattern that
  isn't earning its keep. `role="dialog"` (not `aria-modal`) is the correct, lighter
  role for that behavior.
- **Popover is positioned inline (absolute inside the trigger wrapper), not
  portaled.** Same approach as `Select`'s listbox; simpler than a portal + fixed
  measurement, with the known trade-off that a `overflow: hidden` ancestor would
  clip it — acceptable for this app and consistent with the existing family.
- **No exit animation on Popover.** Instant dismissal keeps a frequent,
  low-ceremony element snappy; the mounted-during-exit state machine Modal needs for
  its big moment is complexity not warranted here.
- **Tooltip entry is a plain fast fade/slide** (show delay 160ms to avoid hover
  flicker, 140ms fade, `--ease-expo-out`), deliberately without Modal/CookieSlip's
  overshoot. Reduced-motion strips both the delay and the transform.
- **Tooltip is placed below the trigger, centered** (native `title` placement),
  `width: max-content`, capped at `min(240px, 100vw - 32px)`. Popover is placed
  below the trigger, left-aligned; a caller can nudge alignment via the `className`
  prop (applied to the wrapper) with a descendant selector.
- **No new tokens.** Both components use only existing `:root` tokens
  (`--paper`, `--paper-2` not used, `--ink-body`, `--ink` via `--rule`, `--radius`,
  `--ease-expo-out`, `--red`, `--font-ui`).
- **`trigger`/`children` typed as `ReactElement<HTMLAttributes<HTMLElement>>`** so
  `cloneElement` typechecks against a fixed props surface; this also documents the
  contract that the trigger must be a plain, focusable HTML element.

## What did NOT work

- The first `tsc` run failed because `app/node_modules` was absent (`tsc: not
  found`); `npm install` fixed it, same as noted in P1.3.
- The initial `ReactElement` prop typing (unparameterized) failed under React 19's
  `@types/react`, where `ReactElement<P = unknown>` makes `cloneElement`'s props
  `Partial<unknown>` and rejects `aria-*` keys. Narrowing to
  `ReactElement<HTMLAttributes<HTMLElement>>` fixed it.

## Deviations from the plan

- No deviations from primitive-only scope. I did not touch `src/index.css` and did
  not wire either component into a page. `grep -rn "title=" src/` shows only a
  `title` prop on a local `StepHeading` helper (a custom prop, not the native
  `title` attribute), so there is no existing native-`title` call site to swap yet —
  consistent with wiring being out of scope for P1.5.

## Open questions

- No test harness exists. Behavior was verified by tracing the JSX/effects and the
  CSS state selectors (`:hover` / `:focus-within` for Tooltip; the two `useEffect`
  paths and the document listeners for Popover), plus a green `tsc`/`vite` build.
  Later blocks should manually confirm the tooltip shows on keyboard focus and that
  Popover restores focus after Escape/outside dismissal.
