# Phase P1.1 — terminal exception doc (D1)

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/deepseek-v4-flash
- **Status:** complete

## What I built
Added `## 14. The Superhost terminal — a governed exception` to
`ART-DIRECTION.md`, the spec section that establishes decision D1 so `P4`
has a rule to implement against. Documentation only — no component code.

The section, in the doc's own voice, covers:
- What the terminal is — the one machine surface on a paper page, and why it
  is not "dark mode" (reconciling with §3's "No dark mode" line: one scoped
  component carrying a terminal metaphor, not a theme toggle).
- The exact CSS shape already decided in `IMPLEMENTATION-SPEC.md §3.6`.
- The scoping rule stated as a hard constraint in §3's style: green is scoped
  to `.superhost-terminal` and nowhere else, no `:root` tokens, no other
  component may reference `--phosphor` / `--phosphor-dim`, enforced by grep
  at P4's gate.
- Where it mounts (`/dashboard`, `/ops/tickets`, `/ops/tickets/:id`,
  `/stay`) — "a block on the page, not a floating chat widget."
- The one place red and green coexist — policy denials render in `--red`
  inside the terminal — stated explicitly as the one documented exception to
  §12's anti-slop `--red` count, so a later reviewer does not flag it.

## Files added or changed
- `ART-DIRECTION.md` — new `## 14.` section appended after `## 13.`; no other
  section touched. `grep -c '^## '` went 13 → 14 (+1).
- `logs/phase-P1-1-terminal-exception-doc.md` — this log.

## Decisions I made
- Matched the document's existing structure for component blocks: prose
  explaining the *why* (why it's not dark mode, why scoping matters), then the
  CSS block, then the rule called out in bold, mirroring §3's "No gradients.
  No shadows..." line and §4/§9's format.
- Preserved spec wording ("a block on the page, not a floating chat widget",
  "the machine's own refusal") to keep the doc traceable to
  `IMPLEMENTATION-SPEC.md §3.6`.
- Read the `a-philosophy-of-software-design.nano.md` rules book as directed;
  applied it as keeping one owner (the component) for the volatile exception
  rather than spreading green across the design system.

## What did NOT work
- Nothing. The rules-book pack and spec references were all present.

## Deviations from the plan
- None. `src/index.css` untouched (that is `P1.7`); no `.tsx`/`.css` component
  written (that is `P4`); no existing section content modified.

## Open questions
- None for P1.1. The actual Terminal component, `--phosphor` tokens, and the
  grep gate verification land in `P4` per plan.
