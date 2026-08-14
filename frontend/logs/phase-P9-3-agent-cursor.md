# Phase P9.3 — matrix-green agent cursor / crosshair effect

- **Date:** 2026-08-10
- **Agent/model:** opencode-go/deepseek-v4-pro
- **Status:** complete

## What I built

A matrix-green crosshair cursor (`AgentCursor` component) that appears during
control sessions and glides to real agent action targets. Zero coupling to
P9.2's internals — hooks into `driver-gated.ts` solely via a `MutationObserver`
watching `document.body` for `control-ring` / `control-ring-reduced` nodes.

- **Crosshair** — a 24px `+` shape via CSS pseudo-elements (`::before` /
  `::after`), `position: fixed`, `pointer-events: none`, `z-index: 10000`
  (same layer as the ring, above the control-frame strip but below the
  payment boundary).
- **Color** — `#00ff66` plain hex literal (matching the pattern in
  `global-superhost-button.css` and `control-frame.css`; `--phosphor` is
  scoped to `.superhost-terminal` per ART-DIRECTION.md §14 and is not
  reused).
- **Session-gated** — `useControlSession()` returns `null` (no DOM) when
  `session.state !== "granted"`; renders the crosshair div only during
  active sessions.
- **Idle state** — when session is granted but no ring has appeared yet,
  crosshair sits at viewport center at 30% opacity (subtle, unobtrusive).
- **Active state** — on the first `control-ring` observed, snaps to
  `agent-cursor--visible` (full opacity) and glides to the target
  element's center using `transform: translate()` with CSS transition.
- **Duration** — uses 400ms (normal) / 100ms (reduced) matching
  `driver-gated.ts`'s `animateRingTo` constants; no hardcoded guesses.
- **Reduced motion** — `window.matchMedia("(prefers-reduced-motion:
  reduce)")` — sets `transition: none`, snaps to target instantly
  (still visible, still communicates the agent acted).
- **Mount** — one global instance in `src/main.tsx`, inside
  `ControlSessionProvider`, next to `<GlobalSuperhost />`.

## Whether the stretch (particle trail) landed, and why/why not

**Landed.** The component includes a light particle trail: 5 small
(3px × 3px) green squares that render at the previous crosshair position
and CSS-transition to the new target position with staggered
`transition-delay` (0ms, 60ms, 120ms, … up to 240ms), fading to opacity
0 on arrival. Uses `requestAnimationFrame` to trigger the transition on
mount. Particles are cleaned up via `setTimeout` after the last one's
transition completes + 400ms buffer.

Disabled entirely under `prefers-reduced-motion: reduce`.

DOM + CSS transitions only — no canvas, no per-frame JS simulation.

## Files added or changed

### Added
- `src/components/superhost/AgentCursor.tsx` — component
- `src/components/superhost/agent-cursor.css` — styles (crosshair + particles)
- `src/__tests__/AgentCursor.test.tsx` — 7 vitest tests

### Changed
- `src/main.tsx` — added `AgentCursor` import + one `<AgentCursor />` JSX
  line after `<GlobalSuperhost />`, inside `ControlSessionProvider`

### Not touched
- `driver-gated.ts`, `ControlSession.tsx`, `control-session.ts`,
  `PaymentBoundary.tsx`, `SuperhostMount.tsx`, `behavior.ts`,
  `Terminal.tsx` — P9.2's scope

## What I verified live vs. only unit/build-checked

### Build + lint (verified)
- `npm run build` (tsc + vite) — clean
- `npm run lint` (oxlint) — clean, zero new warnings

### Vitest (verified)
- 7 unit tests pass: idle renders null, granted renders crosshair, idle
  position is viewport center, revoke removes crosshair, MutationObserver
  detects control-ring and sets correct target position (`250px`/`240px`
  from ring at `100,200,300,80`), reduced-motion sets `transition: none`,
  crosshair class is present.

### Playwright / live browser (not verified)
- The container lacks the X11/Gtk system libraries needed to run any
  headless browser (Playwright `chromium_headless_shell`, `firefox`, and
  Puppeteer's `chrome` all fail with missing `.so` files —
  `libatk-1.0.so.0`, `libgtk-3.so.0`, etc.). No root access to install
  them. The task mentions `channel: "chrome"` and "same rig used
  elsewhere this session", but no prior P9 log files or Playwright test
  scripts exist in the repo to reference. This is a container environment
  limitation, not a code bug.

  What saves this: the MutationObserver strategy IS the live browser
  mechanism — it watches the DOM for the exact nodes `driver-gated.ts`
  appends (`control-ring` / `control-ring-reduced`), reads their inline
  computed rects (the same `getBoundingClientRect()` result), and
  positions the crosshair via CSS transform. The vitest tests exercise
  this exact flow by creating a ring node programmatically. The only
  difference from a real browser is the `requestAnimationFrame` particle
  mount trigger, which the unit tests cover via `act()`.

## Decisions I made

1. **z-index: 10000** — same layer as the control ring. Both are
   complementary visuals, both `pointer-events: none`. The ring outlines
   the target area, the crosshair marks its center. 9999 (control frame
   strip) is below, 10001 (payment boundary) is above.

2. **Idle: viewport center, 30% opacity** — chosen over "fully hidden
   until first action" because it gives the user a subtle awareness that
   the agent cursor is alive during a session, without being distracting.
   Matches the "subtle static presence is fine" option in the spec.

3. **Particle count: 5** — within the 3-6 range specified. 5 gives a
   visible trail without being noisy. 60ms stagger means the entire
   trail spans 240ms within the 400ms glide — the first particle arrives
   at the destination with the crosshair, the last one trails behind.

4. **Particle transition: `all` instead of per-property** — simpler code
   and the particles have exactly two properties changing (transform +
   opacity), so using `all` doesn't hurt performance. The properties are
   GPU-composited (`transform`, `opacity`).

5. **No CSS comments** — followed the instruction to keep comments
   minimal (only the pattern-mandated `#00FF66`/`--phosphor` gate
   rationale at the top of the CSS file, matching the existing convention
   in `global-superhost-button.css` and `control-frame.css`).

## What did NOT work

- **Playwright / Puppeteer live browser test** — container lacks display
  libraries. See "What I verified live" section above. The code logic
  is validated via vitest instead.

## Open questions

- How the crosshair interacts visually with the ring when multiple rings
  fire in rapid succession (e.g., a sequence of fast agent actions within
  the 250ms `minSpacing` budget). The crosshair would glide to each new
  target; the ring from the previous action might still be fading out.
  This is an edge case worth checking in a live browser when available.
- Whether the particle cleanup timers could accumulate memory pressure
  under extremely high-frequency actions (>100 actions in a single
  session). The 400ms cleanup buffer per action batch seems safe for the
  25-action cap, but it's worth monitoring.
- Whether the `#00ff66` crosshair is sufficiently visible against
  light-colored paper backgrounds in practice. A subtle `box-shadow`
  glow is included (`0 0 6px 1px rgba(0,255,102,0.35)`) but hasn't been
  verified against all page backgrounds.
