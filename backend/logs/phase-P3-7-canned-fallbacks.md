# Phase P3.7 — canned fallbacks + OFFLINE FALLBACK marker

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/gpt-5.6-luna
- **Status:** complete

## What I built

- Added a fresh 20-second context deadline around every provider call, including tool-loop iterations.
- Added three small canned responses keyed by turnover-ticket, property-status, and generic intents.
- Persisted structured fallback output with `is_fallback: true`, `fallback_marker: "OFFLINE FALLBACK"`, and the selected intent.
- Recorded `AgentRunFallback.v1` with the marker, intent, reason, and honest unknown usage accounting before completing the run.
- Added runner coverage for a provider that waits through the deadline and for an unmatched intent.

## Files added or changed

- `internal/automation/fallback.go`
- `internal/automation/models.go`
- `internal/automation/runner.go`
- `internal/automation/runner_test.go`
- `logs/phase-P3-7-canned-fallbacks.md`

## Decisions I made

- Kept fallback selection in the automation package because it owns run input interpretation and output persistence.
- Used `InputData` intent/task/action fields, with small text heuristics, rather than inventing a new caller-facing intent enum.
- Applied fallback wrapping at the provider-call boundary so future tool-loop iterations receive the same resilience behavior.
- Fallback usage is zero with `usage_known: false`; no model cost or tokens are fabricated.

## What did NOT work

- No failures. The local automation integration tests skipped database-backed cases because PostgreSQL was unavailable in the provider session.

## Deviations from the plan

- None.

## Open questions

- The later terminal/frontend block should map the structured marker to its own scripted beat number, such as `OFFLINE FALLBACK · 03`.
