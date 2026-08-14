# Phase P4.1 — components/superhost/Terminal.tsx + .css

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/gpt-5.6-luna
- **Status:** complete

## What I built
Added a presentational Superhost terminal shell that renders resolved agent, operator, denial, and system lines, with the required prefixes, scoped phosphor palette, red denial treatment, CSS-only block cursor, reduced-motion override, and children slot.

Added a `/debug` section with one example line of every kind, the cursor enabled, and a visible placeholder child slot for P4.3.

## Files added or changed
- `app/src/components/superhost/Terminal.tsx`
- `app/src/components/superhost/Terminal.css`
- `app/src/routes/debug.tsx`
- `app/src/routes/debug.css`

## Decisions I made
- Used an ordered list for stable line markup that P4.2 can drive without changing the shell.
- Used `! ` for denial lines and `· ` for system lines so both remain visually distinct while preserving the required `> ` and `$ ` prefixes for agent and operator lines.
- Kept the cursor as a conditional element with CSS animation only; reduced motion removes the animation.

## What did NOT work
- The first build and lint attempts could not start because `app/node_modules` was absent. `npm ci` restored the locked dependencies, after which both checks passed.
- The separately cited backend contract and refactoring rules file were not present at their supplied paths in this sandbox.

## Deviations from the plan
- None from the component contract. The existing debug footer was renumbered from section 11 to section 12 to avoid colliding with the new terminal section.

## Open questions
- None for P4.1.
