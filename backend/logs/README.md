# logs/

This repo's native completion record is a `CC-Task:` commit trailer via
Gastown (see `AGENTS.md`, `docs/development/gastown_operating_model.md`).
This directory does **not** replace that — it exists only for the
Jarvis→Superhost rename work (`P0.1`–`P0.4`, later `P3.*`), which the human
chose to dispatch through `.sandcastle/` instead of the existing Gastown rig
(see `DECISIONS.md`). Convention borrowed from the frontend repo
(`ComfortCurators`, `POLICY.md`):

```markdown
# Phase <id> — <name>

- **Date:** YYYY-MM-DD
- **Agent/model:** e.g. opencode-go/deepseek-v4-pro
- **Status:** complete | partial | blocked

## What I built
## Files added or changed
## Decisions I made
## What did NOT work
## Deviations from the plan
```

One file per block: `phase-<id-with-dashes>-<slug>.md`, e.g.
`phase-P0-1-jarvis-rename.md`. `.sandcastle/blocks.mjs close <id>` looks for
a file starting `phase-<id-with-dashes>-` (case-insensitive) with
`**Status:** complete`.

`DECISIONS.md` — running one-line log of cross-block decisions, append only.
