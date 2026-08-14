# Phase P7.1 — Merge assembly, resolve router patches

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/gpt-5.6-luna
- **Status:** complete

## What I found
- `src/main.tsx` has a consistent route table. Every route target exists, paths are unique, and all routed role groups use the expected `RequireRole` wrapper.
- Route modules that are not directly imported by `main.tsx` are reachable indirectly: `EntryRoute` uses `intro`, owner pages use `owner-records`, and dashboard/ops shared use `superhost-panel`.
- Provider nesting is intact and occurs once: `QueryClientProvider` -> `BrowserRouter` -> `AgentSurfaceProvider` -> `ControlSessionProvider` -> `Routes` plus `ControlFrame`.
- The Superhost and agent-surface implementation files and exports have live usage; no deletion was justified.
- `grep -rn phosphor app/src/` matches only `components/superhost/Terminal.css` and `components/superhost/ConfirmBlock.css`.
- A clean build and lint pass completed. Lint reports six existing Fast Refresh warnings in provider/context files but exits successfully.

## Files changed (if any)
- `logs/phase-P7-1-merge-assembly.md`

## Decisions I made
- No application source changes were needed. The route, provider, dead-code, and token-scope audits found no confirmed integration defect.
- Fast Refresh warnings were left unchanged because resolving them would alter provider/context export structure and is outside this integration-hygiene block.

## Open questions
- `npm ci` reports three dependency audit vulnerabilities and Vite reports a large-chunk warning; neither affects this block's build/lint gate and neither was changed.
