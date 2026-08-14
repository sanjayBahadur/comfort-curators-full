# Phase P9.2 — ui_* tool-call to gated-driver wiring

- **Date:** 2026-08-10
- **Agent/model:** opencode-go/deepseek-v4-pro
- **Status:** complete

## What I built

1. **`sendSuperhostMessage` API function** (`src/lib/api/superhost.ts`) — POSTs to `/v1/superhost/threads/:id/messages` with `idempotency_key`, `content`, and `ui_surfaces`. Returns `{request_id, status, resource_id}` (202 Accepted).

2. **`useSuperhostUIActionDriver` hook** (`src/components/superhost/useSuperhostUIActionDriver.ts`) — Processes the SSE event stream, pairs `ToolCallProposed.v1` with its corresponding outcome event (`PolicyAllowed.v1`/`PolicyDenied.v1`/`ApprovalRequired.v1`), builds `AgentIntent` from the paired arguments, and calls the gated driver. Returns `TerminalLine[]` for outcome reporting.

3. **Message composer in `SuperhostMount.tsx`** — A text input + SEND button below the terminal. Disabled when `thread.state !== "ready"`. On submit: snapshots the registry via `useAgentSurfaceContext()`, POSTs the message with current surfaces, clears input, echoes the message as an `operator`-kind `TerminalLine` (`"$ "` prefix via existing `Terminal.tsx` mechanism), and surfaces network errors as `denial`-kind lines.

4. **Line merging** — Operator lines and UI action outcome lines are appended to the base `view.lines` in `SuperhostMount.tsx`, preserving the existing typewriter/cursor logic in `useTerminalStreamView`.

## The pairing mechanism

The SSE stream guarantees that a `ToolCallProposed.v1` is immediately followed by exactly one outcome event for the same call, completing before the next tool call's proposal starts (no interleaving).

The hook walks `events` in order. On `ToolCallProposed.v1` for a `ui_*` tool, it pushes the `{tool_name, arguments}` tuple into a FIFO queue keyed by `tool_name`. On any outcome event (`PolicyAllowed.v1`, `PolicyDenied.v1`, `ApprovalRequired.v1`) for a `ui_*` tool, it shifts the oldest entry from that tool's queue. Only `PolicyAllowed.v1` triggers a gated driver call; the other outcomes are consumed silently (their results are already rendered by `eventToTerminalLine` in `behavior.ts`).

**Dedup guard:** A `Set<string>` ref of already-seen `event_id`s prevents reprocessing on every re-render (`events` is a full array, not a delta feed).

**Sequential processing:** Gated calls are chained via a `Promise<void>` ref (`.then()` chain), ensuring they execute one at a time — not concurrently — respecting the gated driver's 250ms action-spacing and 25-action cap.

**Defensive handling:** If `surface_id` is missing or not a string, a `denial` line is produced and the action is skipped.

**Non-`ui_*` tools:** Events for `propose_*`, `request_*`, `get_*`, etc. are completely ignored by this hook. They flow through the existing `behavior.ts` → `useTerminalStreamView` → `ConfirmBlock` pipeline unchanged.

## Files added or changed

- **Added** `app/src/components/superhost/useSuperhostUIActionDriver.ts`
- **Added** `app/src/__tests__/SuperhostUIActionDriver.test.tsx`
- **Added** `app/src/__tests__/setup.ts` (vitest setup — `window.matchMedia` mock for jsdom)
- **Added** `app/playwright.config.ts`
- **Added** `app/e2e/superhost-composer.spec.ts` (Playwright e2e, not runnable in this sandbox — no browser)
- **Modified** `app/src/lib/api/superhost.ts` (added `UISurfaceInput`, `SendMessageResponse`, `sendSuperhostMessage`)
- **Modified** `app/src/components/superhost/SuperhostMount.tsx` (composer, line merging, driver hook)
- **Modified** `app/src/components/superhost/SuperhostMount.css` (composer styles)
- **Modified** `app/vitest.config.ts` (setup file, include/exclude patterns)

## Tests added, and what could/couldn't be verified live

**Vitest (10 tests, all passing):**
- `ui_click` propose+allowed pair → gated driver called with correct intent and registry
- Successful ui_click produces an `agent`-kind line with `"did: <action>: <label>"` text
- Non-`ui_*` tool (e.g. `propose_check_in`) → gated driver never called, no lines produced
- Gated driver refusal → `denial`-kind line with `"blocked: <reason> — <detail>"`
- Missing `surface_id` in arguments → `denial`-kind line with `"missing surface_id"`
- `ui_set_value` passes the `value` argument through to the intent
- `ui_focus`, `ui_scroll_to`, `ui_open_panel` all produce correct intent types
- Non-`ui_*` `PolicyAllowed.v1` (e.g. `get_weather`) is silently ignored, no driver call

**Playwright (written, not runnable):**
- Two e2e tests in `e2e/superhost-composer.spec.ts`: one for operator line echo after send, one for disabled composer when thread isn't ready. Both mock the API responses via `page.route()`.
- **Cannot verify live:** No Chrome binary in this sandbox (`which google-chrome` returned nothing). No live backend, so the full SSE stream → `ui_*` event → gated driver → DOM action pipeline cannot be tested end-to-end. The unit test covers the pairing logic and gated driver call; the Playwright test covers the composer UI.

## Decisions I made

1. **Hooked into `useSuperhostUIActionDriver` in `SuperhostMount.tsx`** rather than a separate wrapper. The hook is called alongside `useTerminalStreamView`, both consuming the same `stream.events` array. The outcome lines are appended after `view.lines` — this is safe because operator/ui-action lines don't participate in the typewriter animation.

2. **Used a promise-chain (`chainRef`) for sequential processing** rather than a busy-flag/skip approach. This ensures that even if the effect fires multiple times (e.g., rapid event arrival), gated calls remain strictly sequential.

3. **Created a separate file (`useSuperhostUIActionDriver.ts`)** rather than extending `behavior.ts`. The pairing logic (FIFO queue, promise chain, dedup set) is non-trivial and better isolated.

4. **Added `vi.mock` at the top of the test file** with a dynamic import in `beforeAll`. This is necessary because `createGatedDriver` calls `window.matchMedia` at the module level (via `ring.css` → `driver-gated.ts` → `animateRingTo`), and `vi.mock` must be hoisted before the module loads.

5. **Echoed operator messages as `operator`-kind lines** using the existing `Terminal.tsx` `"$ "` prefix. The `"operator"` `TerminalLineKind` was already defined but unused — this is the first code path that actually produces one.

## What did NOT work

- The initial test approach using `vi.doMock` inside the `renderAndCapture` helper failed because `driver-gated.ts` was already imported at the top of the test file (via `useSuperhostUIActionDriver`). Switching to hoisted `vi.mock` + dynamic import solved this.
- `window.matchMedia` is not available in jsdom, causing `driver.ts` module-level code to throw. Fixed by adding a `vitest.config.ts` setup file (`src/__tests__/setup.ts`) that stubs `window.matchMedia`.
- The vitest config was picking up `node_modules` test files when no `include` pattern was specified. Fixed by explicitly setting `include: ["src/**/*.test.{ts,tsx}"]` and `exclude` patterns.
- No Playwright browser binary in this sandbox — e2e tests are written but cannot be executed here.

## Open questions

- When a live backend + browser are available, the Playwright e2e tests should be run to confirm the full loop: open drawer → grant session → type message → POST → operator line → SSE events → ui_* actions → gated driver → DOM changes.
- The `"$ "` operator prefix in `Terminal.tsx` was previously unused. If P9.3's visual redesign changes the prefix styling, this code path should still work since it only sets `kind: "operator"`.
