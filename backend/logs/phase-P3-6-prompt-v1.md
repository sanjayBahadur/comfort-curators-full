# Phase P3.6 — prompt/v1.md (scoping, plain-language voice)

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/gpt-5.6-luna
- **Status:** complete

## What I built

Added the governed Superhost v1 system prompt and a small Go loader that embeds
it with `go:embed`. The prompt states that the agent proposes and never
executes, lists the exact registered tools and descriptions, limits success
claims to returned results or completed approvals, and uses the frozen plain
approval wording.

Added a test that checks the loader is non-empty and contains every name from
`AllowedToolNames()`.

## Files added or changed

- `internal/automation/superhost/prompt/v1.md`
- `internal/automation/superhost/prompt/prompt.go`
- `internal/automation/superhost/prompt/prompt_test.go`
- `logs/phase-P3-6-prompt-v1.md`

## Decisions I made

- Kept the prompt as a versioned static artifact. Dynamic generation would
  make the governed text harder to review and would not remove the need for a
  drift test.
- Cross-checked all 12 names and descriptions against `AllowedToolNames()` and
  `LookupTool()` in `internal/automation/superhost/tools.go`.
- Included the four stream outcomes relevant to model language:
  `ToolCallProposed.v1`, `PolicyDenied.v1`, `ApprovalRequired.v1`, and
  `PolicyAllowed.v1`.

## What did NOT work

Nothing.

## Deviations from the plan

None.

## Open questions

None.
