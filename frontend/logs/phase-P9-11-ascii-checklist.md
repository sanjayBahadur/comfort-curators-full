# Phase P9.11 — ASCII task checklist view

- **Date:** 2026-08-09
- **Agent/model:** gpt-5.6-sol (codex, medium effort)
- **Status:** complete

## What I built

- Added a pure event-to-checklist projection that turns each locally submitted operator message into one task and associates it with the `resource_id`/run ID returned by `sendSuperhostMessage`.
- Added the terminal checklist renderer with `[ ]`, `[~]`, `[x]`, `[?]`, and `[!]` markers; explicit state labels; and indented `|--` step lines.
- Nested mapped proposal, allowed, denial, queued/fallback/terminal, approval-required, and real `ui_*` driver outcome lines under their task.
- Kept initial-thread or otherwise unmatched stream activity visible as ordinary terminal prelude instead of assigning it to a task by timing guess.
- Added a structured `activity` slot to `Terminal` so the checklist can sit inside the existing terminal while its cursor and approval child slot continue to work unchanged.
- Updated the composer E2E expectation from a flat operator line to a newly appended checklist task.

## How task boundaries and status are derived from the real event stream

The composer first appends the operator task with no run ID, which truthfully renders as `NOT STARTED`. When the existing message endpoint accepts the message, its real `resource_id` is stored on that task as the run ID. The pure projection then includes only SSE events whose `SuperhostStreamEvent.run_id` matches that ID, in stream order, and stops at the first `AgentRunCompleted.v1`, `AgentRunFailed.v1`, or `AgentRunCancelled.v1`. It does not infer ownership from timestamps or adjacency. Events with no matching submitted task remain visible outside the checklist.

State is recalculated from those included events on every render:

- no events, or only `AgentRunQueued.v1` → `NOT STARTED` / `[ ]`
- proposal, allowed result, or fallback without a terminal result → `RUNNING` / `[~]`
- latest meaningful event is `ApprovalRequired.v1` → `WAITING FOR APPROVAL` / `[?]`
- latest meaningful event is `PolicyDenied.v1` → `DENIED` / `[!]`, including when a later run-completed event exists
- `AgentRunFailed.v1` or `AgentRunCancelled.v1` → `BLOCKED` / `[!]`
- clean `AgentRunCompleted.v1` → `DONE` / `[x]`
- an event sequence with no recognized status-bearing event → `STATUS UNKNOWN` / `[?]`

The latest stream line passed into the projection is still the line produced by `useTerminalStreamView`, so its typewriter-visible text, denial no-typewriter behavior, reduced-motion choice, and cursor calculation are preserved. Approval events also receive a nested checklist step while the unchanged `ConfirmBlock` remains interactive below the activity.

## Files added or changed

- Added `app/src/components/superhost/task-checklist.ts`
- Added `app/src/components/superhost/TaskChecklist.tsx`
- Added `app/src/components/superhost/TaskChecklist.css`
- Added `app/src/__tests__/SuperhostTaskChecklist.test.tsx`
- Modified `app/src/components/superhost/SuperhostMount.tsx`
- Modified `app/src/components/superhost/Terminal.tsx`
- Modified `app/e2e/superhost-composer.spec.ts`
- Added `logs/phase-P9-11-ascii-checklist.md`

The prohibited control-session, gated-driver, payment-boundary, UI-action-driver, and cursor files were read but not edited. `src/index.css` and `src/main.tsx` were not edited.

## What I verified live vs. build/lint-only

- `npx vitest run --config vitest.config.ts`: **passed**, 4 files and 25 tests. The new tests exercise exact run-ID grouping, first-terminal cutoff, unmatched prelude preservation, nested approval and `ui_*` outcome lines, every checklist state, ambiguous-event handling, markers, and denial alert semantics.
- `npm run build`: **passed**.
- `npm run lint`: **passed** with seven existing warnings in untouched files/tests and no new warning from this block.
- `git diff --check`: **passed**.
- Live browser/backend message verification was not claimed. Google Chrome is installed, but this checkout has no `VITE_DEMO_TENANT_ID`; `SuperhostMount` therefore intentionally stops at “demo tenant is not configured” before it can create a real thread or stream a run. Per the block convention, the component/projection test is the verification substitute.

## Decisions I made

- Used the message response's real run ID as the task/event join key. This is more stable than maintaining a second client-side lifecycle or guessing from event arrival time.
- Kept the grouping/status policy in a framework-free module and the React/CSS renderer as a separate outer adapter, following the available clean-architecture rules.
- Stopped task steps at the first terminal event even if malformed or late events with the same run ID arrive afterward; those late lines remain visible as unassigned activity rather than changing a finished task.
- Treated only the last meaningful proposal/policy/approval event as the current step outcome. A denial followed by a new proposal is running; a denial followed only by run completion remains denied. An approval remains waiting until later meaningful stream activity proves progress.
- Used `STATUS UNKNOWN` for unfamiliar event-only sequences instead of optimistically calling them running or done.
- Did not introduce terminal color tokens in the new stylesheet. It inherits the terminal color and uses opacity for dim branches/state labels; denial styling continues to use the established terminal denial class.

## What did NOT work

- The first component-test run used `jest-dom`-specific matchers, but this repository's Vitest type setup does not register those matchers. I replaced them with standard Vitest truthiness/text-content assertions; all tests and the build then passed.
- A real message/SSE walkthrough could not proceed without the required tenant configuration, so tool-call nesting was verified with the real event and `TerminalLine` shapes in a component test rather than claimed as live backend behavior.
- The requested `AGENTS.md` is absent from this worktree, and the requested Tier 1 rules directory contains only `clean-architecture.mini.md` and `clean-architecture.nano.md`. I read both available files in full and followed their boundary rules; there was no additional repository instruction file to read.

## Open questions

- None for this block. A later environment with a configured tenant and reachable backend should run the existing Chrome E2E flow to add live evidence, but no code or contract change is needed for that verification.
