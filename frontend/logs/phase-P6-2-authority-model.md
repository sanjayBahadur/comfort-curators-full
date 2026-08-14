# Phase P6.2 — authority-model section + diagram

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/gpt-5.6-luna
- **Status:** complete

## What I built

Added the authority-model section after the three expansion stages. It explains the propose/approve thesis and includes a semantic, CSS-built diagram of the Superhost tool-call flow: claim, context assembly, model call, policy evaluation, denial/approval/allowed branches, and completion with usage and audit events.

## Files added or changed

- `src/routes/expansion.tsx`
- `src/routes/expansion.css`
- `logs/phase-P6-2-authority-model.md`

## Decisions I made

- Kept the diagram in `expansion.css` because it is a page-specific layout and does not warrant a new stylesheet.
- Used hard 1px rules and rectangular boxes, with a responsive single-column branch layout on small screens.
- Reserved `--red` for the policy-denied branch.
- Left `src/index.css`, the existing stage sections, and all P1-scoped files untouched.

## What did NOT work

Nothing identified during implementation.

## Deviations from the plan

None.

## Open questions

None.
