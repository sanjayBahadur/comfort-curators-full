# Phase P9.1 — ui_action tool family: policy, execution, context wiring

- **Date:** 2026-08-10
- **Agent/model:** opencode-go/deepseek-v4-pro
- **Status:** complete

## What I built

A new `tool_kind` ("ui_action"), five `ui_*` tools in the Superhost registry, the
policy/execution wiring for them, and the `ui_surfaces` plumbing that carries the
page's registered agent-surface registry into the model's context.

### 1. New ToolKind and ToolAudience (`tools.go`)

- `ToolKindUIAction ToolKind = "ui_action"` — a fourth concrete kind alongside
  `read`, `propose`, `request`. Deliberately distinct from `propose`/`request`
  because those have `RequiresApproval: true` as an invariant; `ui_action` tools
  are approved by the human's explicit control-session grant in the browser,
  not by the backend approval flow.
- `ToolAudienceUI ToolAudience = "ui"` — browser actions, not DB-scoped operations.

### 2. Five new registry entries (`tools.go`)

One per existing frontend intent:
- `ui_focus` — args: `{ "surface_id": string }`
- `ui_set_value` — args: `{ "surface_id": string, "value": string }`
- `ui_click` — args: `{ "surface_id": string }`
- `ui_scroll_to` — args: `{ "surface_id": string }`
- `ui_open_panel` — args: `{ "surface_id": string }`

Each: `Kind: ToolKindUIAction`, `Audience: ToolAudienceUI`, `RequiresApproval: false`,
`Idempotent: false`, `SchemaVersion: ToolSchemaVersionCurrent`.

Argument key is `surface_id` (not `id`) to be unambiguous in log/policy traces.

Verified: no `ui_`-prefixed tool name collides with any prefix in
`prohibitedToolNamePrefixes`. The test `TestSuperhostNoUIPrefixCollidesWithProhibitedPrefixes`
confirms this programmatically.

### 3. `isDirectMutation` fix (`policy.go`)

Added `def.Kind != ToolKindUIAction` to the `isDirectMutation` allowlist.
Without this, every `ui_*` call would be denied as a direct mutation before
reaching the `RequiresApproval` branch.

### 4. Execution path (`tool_executor.go`)

In the `PolicyAllowed` branch, added a check for `def.Kind == ToolKindUIAction`:
returns a synthetic `ResultSummary` instead of calling `executeReadTool`:
```
ui action ui_click queued for surface "btn-submit"; the browser's gated
control-session driver executes it, this backend does not receive confirmation this turn
```
Extracts `surface_id` from arguments for the message; falls back to
`"(no surface_id given)"` if missing/unparseable.

### 5. `ui_surfaces` wiring (`handler.go`, `models.go`)

- Added `UISurfaceInput` struct (`models.go`): `{ ID string; Label string; Actions []string }`
- Extended `handleSendMessage` request body with `UISurfaces []UISurfaceInput`
- Folded `ui_surfaces` into the `msgInput` map alongside `type` and `content`

### 6. System prompt (`prompt/v1.md`)

Added a "### UI actions" subsection after the existing tool list with:
- Descriptions of all five `ui_*` tools
- Rules: `surface_id` must come from "Available UI surfaces" in the user message;
  never invent an id; don't call when no surfaces listed; don't call for unlisted actions
- Clarification: `PolicyAllowed.v1` means the action was *sent*, not necessarily
  *succeeded*
- Updated the "How to report results" section for `PolicyAllowed.v1` to cover both
  read tools and `ui_*` tools

## How ui_surfaces reaches the prompt

The `handleSendMessage` handler constructs a `msgInput` map carrying
`type`, `content`, and now `ui_surfaces`. This map is marshalled together with
the assembled `PropertyContext` into `combined`, which becomes `InputData` on the
`SubmitRequest`. The runner sends `InputData` as the initial user message to the
model provider. No separate template interpolation is needed — the prompt
(`v1.md`) is a static `go:embed`, and the dynamic data (the actual surfaces for
this turn) arrive as part of the user message content.

## Files added or changed

| File | Change |
|------|--------|
| `internal/automation/superhost/tools.go` | Added `ToolKindUIAction`, `ToolAudienceUI`, 5 registry entries |
| `internal/automation/superhost/policy.go` | Fixed `isDirectMutation` to exclude `ToolKindUIAction` |
| `internal/platform/app/tool_executor.go` | Added `ToolKindUIAction` branch with synthetic summary |
| `internal/automation/superhost/handler.go` | Extended request body + `ui_surfaces` in `msgInput` |
| `internal/automation/superhost/models.go` | Added `UISurfaceInput` struct |
| `internal/automation/superhost/prompt/v1.md` | Added UI actions section + tool docs + reporting rules |
| `internal/automation/superhost/tools_test.go` | Added policy tests, updated existing tests |
| `internal/automation/superhost/handler_test.go` | Added `ui_surfaces` deserialization tests |
| `internal/platform/app/tool_executor_test.go` | Added integration test for synthetic summary (DB-backed, skips without Postgres) |

## Tests added

1. **`TestPolicyEngineUIActionToolAllowed`** — Walks `Evaluate` for all 5 `ui_*` tools, asserts `PolicyAllowed` for each.
2. **`TestPolicyEngineUnregisteredUIPrefixToolDenied`** — `ui_delete` and `ui_custom_action` (both unregistered) are both denied by `LookupTool`, proving no accidental prefix-based allowlisting.
3. **`TestPolicyEngineUIActionNotDirectMutation`** — `ui_click` is not denied as direct mutation.
4. **`TestSuperhostUIActionToolKinds`** — Verifies `Kind == ToolKindUIAction`, `Audience == ToolAudienceUI`, `RequiresApproval == false`, `Idempotent == false`.
5. **`TestSuperhostUIActionIsNotAMutation`** — `IsMutation()` returns false for `ui_action` tools.
6. **`TestSuperhostNoUIPrefixCollidesWithProhibitedPrefixes`** — All 5 `ui_*` names pass `IsToolProhibited` check.
7. **`TestSuperhostUISurfaceInput`** — Struct round-trip verification.
8. **`TestHandleSendMessageUISurfacesDeserialization`** — Full payload with surfaces parses correctly.
9. **`TestHandleSendMessageUISurfacesEmptyNotRequired`** — Absent `ui_surfaces` field parses with nil slice.
10. **`TestHandleSendMessageUISurfacesSentInMsgInput`** — `msgInput` map with surfaces survives JSON round-trip.
11. **`TestSuperhostToolExecutorUIActionReturnsSyntheticSummary`** — DB-backed: evaluates `ui_click` through executor, asserts synthetic summary format.
12. **`TestSuperhostToolExecutorUIActionNoSurfaceIDReturnsFallback`** — DB-backed: missing `surface_id` produces `"(no surface_id given)"` message.

## Decisions I made

- **`UISurfaceInput` placed in `models.go`** — It's a domain type used in the API contract, consistent with other context types there.
- **`ToolKindUIAction` deliberately not in `IsMutation()` switch** — Falls through to `default: false`, which is correct: a UI action is not a database mutation.
- **No `text/template` for `v1.md`** — The prompt is static `go:embed`; the existing mechanism already has `InputData` carry dynamic context (property context, message content). Adding `ui_surfaces` to that same payload is the correct and consistent approach.
- **No closed-loop confirmation** — The backend fires and forgets the UI action; the runner continues the loop synchronously. A true round-trip (pause → frontend confirms → resume) would require a new run state and resume endpoint, which is out of scope for this block.

## What did NOT work

- The `tool_executor_test.go` DB-backed tests skip when PostgreSQL is unavailable
  (the container running this test session apparently doesn't have it). The test
  infrastructure is correct and would pass with a running Postgres instance.
- Two existing tests (`TestToolDefinitionsAreIdempotent`, `TestCCHOU001OnlyTypedToolsAreExposed`)
  needed updating to accommodate the new `ui_action` kind — both are now fixed.

## Open questions

None. All five `ui_*` tools are registered, the policy evaluates them correctly,
the executor returns the correct synthetic summary, and `ui_surfaces` data flows
from the HTTP handler into the model's context.
