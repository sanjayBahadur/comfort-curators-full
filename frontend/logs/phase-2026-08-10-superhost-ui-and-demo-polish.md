# Session 2026-08-10 — Superhost UI overhaul, expansion rewrite, demo polish

- **Date:** 2026-08-10
- **Agent/model:** Claude Sonnet 5, orchestrator session (mix of direct work and
  Codex/opencode dispatches — dispatches are individually logged and referenced
  below; this log is the connective narrative across the whole day)
- **Status:** complete
- **Branch:** `v1.1`
- **Commits:** `6ac4306a` → `2ca09a03` (`git log --oneline 6ac4306a~1..2ca09a03`)

Like the backend's equivalent log for today, this exists because the existing
docs (`README.md`, `DEMO-RUNBOOK.md`) describe an earlier build-pack process and
seed numbers that predate this session by a wide margin. Read this for what
actually changed today; treat `logs/phase-P9-*.md` (the per-dispatch logs
referenced below) as the detailed record for each individually-dispatched piece.

## What changed, in the order it happened

### 1. Superhost UX overhaul: control-first, real terminal, real map (`6ac4306a`)

Direct response to live user feedback that Superhost "doesn't feel smart and
fully autonomous at all." Several structural fixes landed together:

- **Terminal scroll and complexity.** The terminal was unscrollable and showed
  every raw tool-call/policy event unconditionally, reading as "garbage,
  too heavy for a normal user." Bounded the terminal's height with real
  vertical scroll, and added `TaskChecklist.tsx` — groups the raw event stream
  into per-task entries, collapsing intermediate trace lines behind a
  `<details>` while always showing the one line that actually matters (the
  final answer, denial reason, or approval summary).
- **Grant-control-first flow.** Handing over control used to live only in the
  global drawer's chrome — page-embedded terminals had no way to grant control
  at all, so `ui_*` actions could never work there. Moved the
  `[ HAND OVER CONTROL ]` button into `SuperhostMount` itself, shown before the
  composer (deliberately — you grant control, *then* you get the ability to
  direct it), so every embedding is capable, not just the drawer.
- **`PaymentBoundary` scope fix ("the annoying lock at the store").** It fired
  on mere component existence in the mounted tree rather than actual
  scroll-reach into a payment area. Fixed via an `IntersectionObserver`-based
  sentinel instead of firing on mount.
- **Portfolio-capable control.** `useControlSession` extended so a granted
  control session isn't locked to one property — the direct answer to "handing
  over control should not just limit itself to property rather a user."

### 2. Calendar compactness + real map provider (dispatched, `27ad434d`/`P9.22`, `deec1d61`/`P9.23`)

- Calendar grid made more compact; past reservations rendered greyed/dimmed,
  the active one kept full contrast — direct fix for "calendar ui needs to be
  slightly more compact and the bar that shows the past reservation should be
  greyed out."
- Replaced the placeholder zone visualization with a real map: **Leaflet +
  CARTO Positron tiles** (`https://{s}.basemaps.cartocdn.com/light_all/...`,
  no API key required), chosen specifically for matching the site's
  ink-on-paper monochrome aesthetic. Staff's own ticket locations plot on it
  with a fake-but-plausible current-location marker and a live-traffic-style
  ETA, per the "just enough for demo" scope given.
- Both independently verified (build+lint clean, no banned CSS patterns, live
  Chrome check) before merging — see `logs/phase-P9-22-design-fixes.md` and
  `logs/phase-P9-23-real-staff-map.md` for the per-dispatch detail.

### 3. Real per-account context, streaming answer rendering, caching (`0377785b`, `26acf6d6`)

- `useSuperhostStream` gained a `cacheKey`-scoped localStorage cache so
  reopening Superhost shows the last-known real conversation immediately
  instead of a blank loading state on every open.
- Fixed the frontend half of the backend's `AgentRunCompleted.v1` missing-text
  bug (see the backend session log, item 5): `behavior.ts`'s
  `eventToTerminalLine` now reads the event's real `output` field, with the
  literal fallback string "run completed" only shown if that field is
  genuinely absent.

### 4. Global drawer defaults to portfolio scope (`c04a20e0`)

The global Superhost drawer now connects a portfolio-scoped thread by default
instead of requiring a property pick first — direct frontend counterpart to
the backend's portfolio-thread work the same day.

### 5. Expansion page rewrite (dispatched, `fb1b053f`/P9.24, merged `0eb18e2b`)

Direct response to "the expansion page is dull and repetative... fresh and
creative... superhost os will be the deep tech, curators crew will be the hr,
comfort curators will be property and real estate." Dispatched via Codex,
independently verified (grep-gated for banned CSS, clean build/lint, live
Chrome walkthrough of all 7 slides) before merging. Full detail:
`logs/phase-P9-24-expansion-flywheel.md`.

### 6. Compact terminal heading (`6d03666a`)

The terminal used to open with a four-line decorative "A machine surface for
the work in front of you." heading, eating real vertical space from the log
content itself in the drawer's narrow width. Direct fix for "we dont need
filler text... rather usable space for logs" — replaced with a single compact
status line (`NOT CONNECTED` / `CONNECTED · CONTROL NOT HANDED OVER` /
`THREAD <id>`), with the old heading text preserved only as a visually-hidden
`sr-only` accessible name so nothing regresses for screen readers.

### 7. Real live token streaming, frontend half (`57b0ca3f`)

Frontend counterpart to the backend's `9072177` (same day) — see that log for
the backend architecture. Three pieces here:

- `superhost-stream.ts`'s SSE merge used to silently ignore any event whose
  `event_id` it had already seen. The new synthetic `AgentRunToken.v1` frame
  deliberately reuses one stable `event_id` ("token-<run_id>") across a whole
  streaming turn so each delta *replaces* the last one instead of appending a
  new line per tick — this needed the merge changed from ignore-on-match to
  replace-on-match. A no-op for every other (persisted, immutable) event type.
- `behavior.ts` renders `AgentRunToken.v1` as-is, with no `useTypewriter`
  fakery layered on top — it's already real, live text. Once a run's real
  terminal event lands, its token line is filtered out of the render (the
  terminal event already carries the identical final text).
- `task-checklist.ts` counts `AgentRunToken.v1` as "running" activity, so a
  pure-narrative turn with no tool call still shows as in-progress while
  streaming, not "unknown."

Live-verified across owner/staff/guest sessions: real, visible character-by-
character growth as DeepSeek generates the answer.

### 8. Merge assembly + P9.24 (`0eb18e2b`)

Final merge of the dispatched expansion rewrite, independently re-verified
post-merge (build, lint, live browser walkthrough of all 7 slides) before
committing.

### 9. Fix the phantom horizontal scroll (`f8ac9a7b`)

Direct fix for "please fix the terminal vertical scroll, we need to remove the
need for the horizontal scroll." Root cause, found in both `Terminal.css` and
`TaskChecklist.css`: a line's `min-width: max-content` fought directly with
the `white-space: pre-wrap` meant to wrap it, and the ancestor `<ol>`s were
`display: grid` with no `grid-template-columns`, so the implicit auto column
grew to fit the unwrapped width instead of wrapping it. Fixed at the source:
removed the `min-width: max-content` rules, added `min-width: 0` +
`overflow-wrap: break-word` to the actual text-bearing spans (now given
explicit classes — `.superhost-terminal-text`, `.superhost-task-wrap` — for
this), and bounded the grid columns with `grid-template-columns: minmax(0,
1fr)`. Verified with a full DOM-tree walk on a real long response: zero
elements overflowing horizontally.

### 10. Real photography on the expansion page (`0ea681b9`)

Direct fix for "the plot map looks terrible, lets use the images from
~/Documents/assets... idc about the copy right its for the investors." The
small abstract "ONE SYSTEM / NODE IN FOCUS" triangle diagram (three circles
labeled CC/OS/HR) repeated in each stage slide's sidebar was replaced with
real documentary photography — one image per stage, picked for fit: a carved
sandstone facade for Comfort Curators, a mural of nine painted windows (a
literal metaphor for an agent watching into every property) for Superhost,
and a hand-painted auto-rickshaw with pedestrians for Curators Crew. Source
photos converted to a high-contrast grayscale duotone (ImageMagick) so they
read as documentary evidence inside the page's ink-on-paper system rather
than fighting it as raw color photos. New assets under
`app/src/assets/expansion/`.

### 11. The logo, for real (`f56489b1`)

Direct fix for "logo looks bad, lets just literally use our heading font and
use CC as our logo." The only actual graphic logo anywhere in the app was
`public/favicon.svg` — a black square with "CC" drawn as custom bezier paths
approximating letterforms, not real type (everywhere else, "brand" text is
plain text already set in the real fonts). Replaced the hand-drawn paths with
real "CC" text set in Instrument Serif, via an inline `@font-face` with the
actual woff2 embedded as base64 — fully self-contained, no external font
request to fail silently.

### 12. Stay page: two real layout bugs, found by looking (`2ca09a03`)

Found via an actual visual sweep across every role's main screens (not a
code-only read):

- The guest portal's own "ACCESS DESK" back-to-login link sat in the exact
  same top-left corner as `GlobalBackButton` (a later, fixed-position,
  always-present addition covering every route) — the two rendered as
  clipped, colliding text. Removed the redundant local link and gave
  `.stay-shell` enough top padding (28px → 72px) to clear the fixed button,
  since this page has no separate top nav bar of its own the way
  dashboard/expansion do.
- "The store" section spanned only 8 of 12 grid columns; `.stay-grid`'s
  background is deliberately `var(--ink)` (the 1px gap between cells is how
  this page draws hairline dividers), but the *entirely unfilled* remaining 4
  columns exposed that same ink background as a large solid black void, not a
  hairline. Widened the section to span 12 — its own catalog grid reflows to
  whatever width it's given, so this is a straightforward improvement.

## Decisions made this session

- **Leaflet + CARTO Positron over a paid map provider** — no API key, and the
  Positron tile style is close enough to the site's own monochrome palette to
  not need heavy CSS filtering.
- **`AgentRunToken.v1` merge is replace-on-matching-id, not append** — the one
  deliberate exception to the otherwise-append-only SSE event log, scoped
  narrowly (see item 7).
- **Grayscale duotone treatment for the expansion photos**, not raw color —
  the page's design system (`ART-DIRECTION.md`) is strict monochrome with a
  single red accent; introducing full-color street photography untreated
  would have clashed rather than elevated the page.
- **Removed `stay.tsx`'s own `ACCESS DESK` link rather than repositioning
  it** — `GlobalBackButton` already does the identical job (navigate back to
  `/login`); keeping both was the duplication that caused the collision in
  the first place.

## What did NOT work / had to be revisited

- The first pass at the streaming terminal's typewriter bypass only checked
  whether the *current* line was still the live token line (`id` starting
  with `token-`) — this correctly stopped animating the in-progress line, but
  once the real `AgentRunCompleted.v1` event replaced it, that *new* line
  still went through the full fake typewriter reveal a second time, producing
  a brief truncated-looking flash right as a response finished (caught by
  watching a real response settle, not by code review). Fixed by also
  checking whether the line's `run_id` ever had a token event at all.

## Open questions / known gaps for whoever picks this up next

- **`README.md`/`DEMO-RUNBOOK.md` are stale relative to this session** — they
  describe an earlier phase-by-phase build-pack process and seed numbers
  (`2 properties, 3 workers, 3 tickets...`) that no longer match the seed's
  real output (62 catalog items, 9 tickets, 17 reservations, as of this
  session's `npm run seed`). Not rewritten in full here, to keep this change
  reviewable — see the backend session log's equivalent note.
- **Only a partial visual sweep was done.** Login, all three dashboards,
  property detail, ops tickets, and the guest stay page were checked live in
  a real browser and two bugs were found and fixed. Onboarding, invoices, the
  curator-facing routes, and the calendar itself were not covered this pass.
- **UI-driven ticket creation via Superhost isn't fully wired** — see the
  backend session log; this is a shared gap, not frontend-only.
