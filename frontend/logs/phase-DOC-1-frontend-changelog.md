# Phase DOC-1 — frontend CHANGELOG

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/deepseek-v4-flash
- **Status:** complete

## What I built

Created `CHANGELOG.md` at the repo root — a narrative, per-phase build log for
future developers, matching the shape of the backend repo's
`docs/development/CHANGELOG.md` (prose paragraph, then an "Issues found and
fixed" section, ending with a pointer to the detailed record). Two entries:

1. **P0.5 — Jarvis → Superhost rename (frontend slice)**, synthesized from
   `logs/phase-P0-5-frontend-rename.md`, including the context note that the
   backend's equivalent rename touched a real IAM-validated session role (the
   detail lives in the backend repo's own changelog).
2. **Wave 1 — Design primitives and the /expansion pitch page**, synthesized
   from `logs/phase-P1-1-…md` through `logs/phase-P1-7-…md`, `logs/phase-P6-1-…md`
   through `logs/phase-P6-4-…md`, with `logs/DECISIONS.md` for cross-block
   context.

Documentation-only block. No application code touched; `src/index.css`
remained frozen and untouched.

## Files added or changed

- `CHANGELOG.md` (new, repo root)
- `logs/phase-DOC-1-frontend-changelog.md` (this log)

## Decisions I made

- Kept both entries to the requested shape: `## <Phase/Wave name>` heading, one
  narrative prose paragraph (not a bullet list), `### Issues found and fixed`
  with bold one-line summaries plus explanation, and a closing one-line pointer
  to the `logs/` phase files.
- For the undefined-CSS-token issue I listed the tokens exactly as the P1.7 log
  records them (`font-meta`, `font-shout`, `ink-60`, `ink-40`, `ease-expo-out`)
  rather than asserting a count, so the changelog stays faithful to the source
  log.
- Included the recurring `node_modules` absence as an "issue found and fixed"
  because it is genuinely recorded across `P1.3`–`P1.7` and `P6.3`/`P6.4`; it is
  the kind of recurring tooling friction a future developer would want flagged.
- Described the `.sandcastle/` miss as the app/-vs-repo-root working-directory
  confusion it was, and explicitly labelled it process/tooling rather than app
  code.

## What did NOT work

- The backend repo's `docs/development/CHANGELOG.md` was not readable from this
  checkout (sibling repo not present), so the format was matched from the task's
  description of its shape rather than read directly.

## Deviations from the plan

- None. `src/index.css` and all application code untouched; only the two
  required files were written.

## Open questions

- None.
