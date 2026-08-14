# Phase P3.1 — http_provider.go (DEF-02, DEF-03)

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/deepseek-v4-pro
- **Status:** complete

## What I built

### DEF-02 — Authentication

- Added `APIKey string` field to `HTTPProvider` struct.
- `Call()` reads `CC_MODEL_API_KEY` from the environment as a fallback when
  the constructor field is empty.
- Sets `Authorization: Bearer <key>` on every outgoing request.
- The key is never logged and never appears in error bodies. Error paths only
  include the server's response body, never our request headers or body.

### DEF-03 — System prompt, tools, tool_calls

- Added `System string` field to `ProviderRequest`. When set, `Call()` prepends
  a `system`-role message before the user message.
- Added `Tools []ChatToolDef` to the HTTP chat request. Tools are built from
  `superhost.AllowedToolNames()` and `superhost.LookupTool()` in a
  `superhostTools()` helper in `app.go` (the composition root, which imports
  both `automation` and `superhost`), breaking what would otherwise be a
  circular import between `automation` and `superhost`.
- Each tool uses its real `Name` and `Description` from `ToolDefinition`,
  with a generic `{"type":"object"}` parameters schema (accepting any JSON
  object as arguments, consistent with `ToolCallInput.Arguments`).
- Modeled `tool_calls` in `chatChoiceMessage` with the OpenAI-compatible shape:
  `id`, `type`, `function` (`name`, `arguments`).
- Extracted `tool_calls` from the model response into
  `ProviderResponse.ToolCalls` as `json.RawMessage`.
- Default model `gpt-5.6-luna` and base URL `https://api.openai.com` (decision
  D5) — only used when the caller doesn't specify a model/URL.

## Files added or changed

- `internal/automation/http_provider.go` — DEF-02 (auth header, API key) and
  DEF-03 (system prompt, tools array, tool_calls modeling, default model/URL)
- `internal/automation/provider.go` — added `System string` to `ProviderRequest`
- `internal/automation/usage_test.go` — updated `NewHTTPProvider` call sites
  for new signature
- `internal/platform/app/app.go` — updated `NewHTTPProvider` call, added
  `superhostTools()` helper, added `"log"` import

## Decisions I made

1. **Tools injected via constructor, not imported**: `http_provider.go` does
   not import `superhost` directly because `superhost/handler.go` imports
   `automation`, creating a cycle. Instead, `ChatToolDef`/`ChatToolFunction`
   are exported types, and the composition root (`app.go`) builds the tool
   list and passes it to the provider constructor.

2. **Generic `{"type":"object"}` parameters**: No per-tool JSON schemas exist
   in the codebase. `ToolDefinition` has `Name`, `SchemaVersion` (a version
   string, not a schema), `Kind`, `Description` — no `Parameters` field.
   Using `{"type":"object"}` is the minimal correct thing that accepts the
   `json.RawMessage` arguments `ToolCallInput` already handles downstream.

3. **API key precedence**: Constructor value takes priority; environment
   variable (`CC_MODEL_API_KEY`) is the fallback. This allows tests to pass
   an empty string and still work.

## What did NOT work

- The initial implementation imported `superhost` directly from
  `http_provider.go`, which created a circular import through
  `superhost/handler.go` → `automation`. Fixed by moving tool construction to
  the composition root.

## Deviations from the plan

None.

## Open questions

- Per-tool argument JSON schemas would make tool selection more reliable and
  do not exist yet. Flagged in a `log.Printf` at the composition root.
