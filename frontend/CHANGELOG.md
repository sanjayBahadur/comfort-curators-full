# CHANGELOG

A narrative, per-phase build log for future developers. Each entry summarizes
what the phase built and why, then lists concrete issues found and fixed.
Detailed records live in `logs/` next to each phase log.

## P0.5 — Jarvis → Superhost rename (frontend slice)

This block renamed the frontend's Jarvis activity surface to Superhost. It was
a straightforward rename: the panel component and its CSS, the trigger exports
and route-local state, the CSS selectors, query keys, labels, accessible
names, and the read-only API client were all updated so `app/src/` contains no
`jarvis` references. Files were renamed to `superhost-panel.tsx` /
`superhost-panel.css` and `lib/api/superhost.ts`, `dashboard.tsx` and
`ops-shared.tsx` were updated, the `AgentRun.run_kind` type kept its `string`
fallback with `superhost` as the named value, and the five living architecture
documents in scope (`ARCHITECTURE.md`, `INTEGRATION.md`, `PHASES.md`,
`SCREENS.md`, `PRD.md`) were updated; `ORCHESTRATION.md`,
`IMPLEMENTATION-SPEC.md`, and `KICKOFF-PROMPT.md` were left untouched as
requested. Because this frontend has no run-creation POST, there was no
`/v1/jarvis/runs` caller to change. For context: the backend equivalent of this
rename turned out to touch a real IAM-validated session role, not just a name —
the detail lives in the backend repo's own changelog, not this one.

### Issues found and fixed

- **First combined patch rejected because the CSS file is one rule per line.** The rename's
  first combined patch was rejected by review because the CSS is formatted as one rule per line;
  the changes were reapplied successfully in smaller patches.

Detailed record: `logs/phase-P0-5-frontend-rename.md`.

## Wave 1 — Design primitives and the /expansion pitch page

Wave 1 built five custom UI primitives in `app/src/components/ui/`, wired them
in so every native `<select>` and `<dialog>` in the app was replaced, and then
built the `/expansion` pitch page on top of the now-frozen design tokens.

The wave opened with `P1.1`, which added the Superhost terminal exception to
`ART-DIRECTION.md` (`## 14`, decision D1) — documentation only, establishing
the scoping rule (green lives only inside `.superhost-terminal`, no `:root`
tokens, `--phosphor` referenced nowhere else) so `P4` has a rule to implement
against. `P1.2` built the `Select` primitive: a listbox-pattern component with
controlled and uncontrolled value support, a hidden named input so form
submission is preserved, roving tabindex, Home/End/type-ahead navigation, and
outside-pointer close. `P1.3` built the `Modal` primitive: a `document.body`
portal with a fixed layer and no scrim, focus trapping with restoration to the
trigger, Escape dismissal, a 1.5deg entry within the readable-content limit,
and an overshoot/wipe entrance that never fades — deliberately replacing the
native `<dialog>` top layer. `P1.4` built `CookieSlip`, a bottom-left torn
paper consent slip with a red header accent, halftone corner, tape strip,
equal-prominence consent controls, and a `PRIVACY · 01` footer index, persisting
both choices to `localStorage`. `P1.5` added `Tooltip` (show/hide driven purely
by CSS `:hover` / `:focus-within`, with `aria-describedby` injected into the
trigger) and `Popover` (controlled, `role="dialog"`, Escape and outside
pointerdown close with focus restored, deliberately without a modal-style focus
trap). All five primitives use only existing paper/ink/rule/radius tokens —
`--radius: 0`, 1px `--rule` borders, no shadows. `P1.6` then replaced all 15
native `<select>` controls across onboarding, ticket filtering, and the new
ticket form with the shared `Select` (keeping onboarding's uncontrolled named
inputs so the `FormData` payload path is unchanged, and controlled value/callback
behavior on the filter and ticket forms), and replaced the package-shop
quick-view `<dialog>` with the shared `Modal`. Finally, `P1.7` audited
`src/index.css` against the shared art-direction tokens, tightened the primitive
CSS, added a live primitives section (`09 / PRIMITIVES`) to `/debug`, and froze
`index.css` — any later block that thinks it needs to touch it must stop and
escalate per `ORCHESTRATION.md` §3.

The `/expansion` pitch page grew in four blocks. `P6.1` built the skeleton: an
introductory thesis followed by three full-height stages — Comfort Curators,
Superhost OS, and Curators Crew — each with its exact thesis and revenue lines
and a single 1px rule separating sections, styled only with existing tokens and
route-scoped CSS. `P6.2` added the authority-model section with a semantic,
CSS-built diagram of the real Superhost tool-call flow — claim, context
assembly, model call, policy evaluation, the denial/approval/allowed branches,
and completion with usage and audit events — with `--red` reserved for the
policy-denied branch. `P6.3` added a unit-economics section with two plain SVG
charts (an illustrative revenue-mix composition and an indicative
contribution-per-relationship bar chart) carrying explicit illustrative
disclosure, using JetBrains Mono labels, flat fills, and hard 1px axes. `P6.4`
tightened the two regulatory framings flagged as pitch risk in
`IMPLEMENTATION-SPEC.md` §3.8: the Curators Crew revenue line now reads
"equipment financing via a licensed NBFC partner" with a one-sentence note
making clear the credit never sits on our books, and the authority-model copy
now states unambiguously that the machine assembles documents/filings/drafts
while a licensed professional files, represents, and signs — the
"machine prepares / licensed human decides / every decision audited" moat
framing left intact.

### Issues found and fixed

- **Undefined CSS tokens were silently falling back to nothing on pre-existing pages.** The
  `P1.7` audit found undefined legacy token references (`font-meta`, `font-shout`, `ink-60`,
  `ink-40`, `ease-expo-out`) resolving to nothing across pre-existing pages; each was replaced
  with the corresponding existing art-direction token.
- **A box-shadow slipped past P1.4's own review and was caught in the follow-up audit.** The
  cookie slip built in `P1.4` carried a shadow that the `P1.7` audit caught and removed, along
  with the cursor's hardcoded white replaced with `var(--paper)` and the tape's duplicate color
  literal replaced with `var(--ink)` plus opacity.
- **`.sandcastle/` tooling path bug (app/-vs-repo-root confusion) hit during dispatch.** During
  `P6.1` dispatch the referenced `.sandcastle/rules/clean-architecture/clean-architecture.mini.md`
  was not present — the scaffold lives at `app/.sandcastle/`, but the dispatched agent works from
  the repo root, so the relative path missed. This is a process/tooling issue, not app code, but
  worth knowing before extending the dispatch tooling.
- **Repeated `node_modules` absence blocked build and lint across the wave.** `tsc`/`oxlint`
  were repeatedly unavailable (`app/node_modules` absent) through `P1.3`–`P1.7` and `P6.3`/`P6.4`;
  each was fixed with `npm install` or `npm ci` from `app/`, which restored the locked
  dependencies with no manifest changes.

Detailed record: `logs/phase-P1-1-terminal-exception-doc.md` through
`logs/phase-P1-7-index-css-freeze.md`, and `logs/phase-P6-1-expansion-skeleton.md`
through `logs/phase-P6-4-regulatory-framing-copy.md`; cross-block context in
`logs/DECISIONS.md`.
