# Phase DOC-1 — backend CHANGELOG entry for the rename

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/deepseek-v4-flash
- **Status:** complete

## What I built

Extended `docs/development/CHANGELOG.md` with one new phase entry, `## Phase
P0 -- Jarvis to Superhost rename`, written in the file's own established
voice: a narrative prose paragraph summarizing what the rename touched,
followed by an "### Issues found and fixed" section with one concrete bullet
per real review-caught issue, then a closing pointer line.

Covered in the narrative: the Go package rename (`jarvis` → `superhost`),
the `run_kind` data migration with idempotent backfill, the API route rename
with the 308 redirect shim, the `contracts/` update, and the IAM role
dual-accept work (the security-relevant finding that `RoleJarvis = "jarvis"`
is a real persisted session role, not just a name, so `"superhost"` was
added alongside rather than replacing it).

Covered in the issues section (the four the independent reviews actually
caught, written concretely): the `context_source` wire-value regression
(P0.1b), the `AgentKindSuperhost` value mismatch / two sources of truth
(P0.1b), the loose exclusion-based assertions in the route-redirect test
(P0.3b), and the compliance handler's missing HTTP-level authorization test
coverage (P0.7b).

The `RoleJarvis`/role-string duplication across `iam`, `compliance` and
`reservations` is noted plainly (grep-able) in the narrative: both `"jarvis"`
and `"superhost"` are now recognized (dual-accept), and the old value was
deliberately never removed for backward compatibility with sessions/tokens
that may still carry it.

## Files added or changed

- `docs/development/CHANGELOG.md` — appended the `## Phase P0` entry.
- `logs/phase-DOC-1-changelog.md` — this file.

## Decisions I made

- Matched the existing heading style exactly: `## Phase N -- Title` (double
  hyphen), consistent with the Phase 1–7 headings.
- Used one phase entry rather than one per P0 sub-block, since the P0 logs
  are one cohesive initiative.
- Four issue bullets only, per the task's explicit list of what the reviews
  caught; did not add a fifth for the P0.4 operation-count fix since the
  task scoped the issues section to the four named items.
- Closing pointer line references `logs/phase-P0-*.md` and
  `logs/DECISIONS.md` rather than `reports/build/phase-N.md`, because no
  mechanical phase report exists for P0 (that directory covers Phase 1–7);
  the pointer stays honest about where the real record lives.
- Did not touch `contracts/` (AGENTS.md restriction) or anything other than
  the CHANGELOG.

## What did NOT work

- Nothing. Documentation-only change; no build/test implications.

## Deviations from the plan

- None.

## Open questions

- None.
