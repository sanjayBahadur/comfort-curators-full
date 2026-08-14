# Phase P0.5 — frontend rename

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/gpt-5.6-luna
- **Status:** complete

## What I built

Renamed the frontend Jarvis activity surface to Superhost, including its panel and trigger exports, route-local state, CSS selectors, query keys, and read-only API client. Updated the `AgentRun.run_kind` type to retain the `string` fallback while using `superhost` as the named value. Updated the five living architecture documents in scope.

## Files added or changed

- Added `app/src/routes/superhost-panel.tsx` and `app/src/routes/superhost-panel.css`.
- Added `app/src/lib/api/superhost.ts`.
- Updated `app/src/routes/dashboard.tsx` and `app/src/routes/ops-shared.tsx`.
- Updated `ARCHITECTURE.md`, `INTEGRATION.md`, `PHASES.md`, `SCREENS.md`, and `PRD.md`.
- Removed the old `jarvis-panel.tsx`, `jarvis-panel.css`, and `lib/api/jarvis.ts` paths through the file renames.

## Decisions I made

- Renamed all panel CSS classes, the loading animation name, query keys, labels, accessible names, and visible activity copy so `app/src/` contains no `jarvis` references.
- Updated the scoped documentation references to `Superhost`; left `ORCHESTRATION.md`, `IMPLEMENTATION-SPEC.md`, and `KICKOFF-PROMPT.md` untouched as requested.
- No run-creation POST exists in this frontend, so there was no `/v1/jarvis/runs` caller to change.

## What did NOT work

- Nothing required for the block failed. The first combined patch was rejected because the CSS file is formatted as one rule per line; the changes were reapplied successfully in smaller patches.

## Deviations from the plan

- None. `src/index.css` and backend files were not touched.

## Open questions

- None for this block.
