# Phase P4.6 — AgentSurface registry + 5-intent driver (D6, §3.9.1)

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/deepseek-v4-pro
- **Status:** complete

## What I built

1. **AgentSurface types** (`components/agent-surface/types.ts`) — `AgentIntent` union (5 types: `ui.focus`, `ui.set_value`, `ui.click`, `ui.scroll_to`, `ui.open_panel`), `IntentResult` (discriminated union with `ok: true` vs `ok: false` + `reason` + `detail`), `AgentSurfaceEntry` (id, element ref, actions set, label, optional `onOpenPanel` callback), and the `AgentSurfaceRegistry` (a `Map<string, AgentSurfaceEntry>`).

2. **AgentSurface registry** (`components/agent-surface/context.tsx`) — React context-based store with:
   - `AgentSurfaceProvider` — wraps the entire app (placed inside `<BrowserRouter>` in `main.tsx`) so all routes share the same registry `Map`. The map persists across route changes; route-scoped cleanup happens because each `useAgentSurface` hook unregisters on unmount.
   - `useAgentSurfaceContext()` — accessor for the context value.
   - `useAgentSurface(id, actions, label, onOpenPanel?)` — hook that returns a callback `ref` to attach to DOM elements. On mount (ref callback fires with a node), it sets `data-agent`, `data-agent-actions`, and `data-agent-label` attributes on the element and registers it in the context store. On unmount (useEffect cleanup), it unregisters. Route-scoped: when a route component unmounts, all its `useAgentSurface` hooks are cleaned up.

3. **5-intent driver** (`components/agent-surface/driver.ts`) — `applyAgentIntent(registry, intent) -> IntentResult` that:
   - Looks up the ID in the registry → `unregistered_id` failure if not found.
   - Maps intent type to action key (`ui.set_value` → `"set"`, `ui.open_panel` → `"open_panel"`, etc.) and checks membership in the element's `actions` set → `action_not_allowed` failure if not allowed.
   - Checks element interactability (`isConnected`, `display:none`, `visibility:hidden`, `disabled` attribute) → `not_interactable` failure if blocked.
   - Performs the real DOM action:
     - `ui.focus` → `element.focus()`.
     - `ui.set_value` → uses `Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(element, value)` then dispatches native `input` and `change` events (bubbles: true) to trigger React's synthetic event system. Also handles `<select>` and `<textarea>`.
     - `ui.click` → `element.click()`.
     - `ui.scroll_to` → `element.scrollIntoView({ behavior: smooth | auto, block: 'nearest' })`, respects `prefers-reduced-motion`.
     - `ui.open_panel` → calls the entry's `onOpenPanel` callback if registered; returns `action_not_allowed` failure if the element doesn't have one (not a disclosure).
   - **Every failure returns a distinct, descriptive result. No silent no-ops.**

4. **Demonstration on `/debug`** (section 13) — registers 5 real elements:
   - `demo-input` (controlled React `<input>` with `focus` + `set` actions) — proves `ui.set_value` updates React state, not just DOM `.value`.
   - `demo-button` (button with click counter, `focus` + `click` actions).
   - `demo-select` (Select primitive with `focus` + `click` + `scroll_to` + `open_panel` actions; `onOpenPanel` clicks the internal trigger to open the dropdown).
   - `demo-modal-button` (button with `focus` + `click` + `open_panel` actions; `onOpenPanel` opens a Modal).
   - `demo-disabled` (disabled button with `focus` + `click` actions — exists solely to demonstrate the `not_interactable` failure mode).
   - Manual trigger panel with buttons for all 5 valid intents plus 3 failure cases (unregistered ID, disallowed action, non-interactable element). A result display shows the last intent's outcome.

## Files added or changed

- **Added** `app/src/components/agent-surface/types.ts`
- **Added** `app/src/components/agent-surface/context.tsx`
- **Added** `app/src/components/agent-surface/driver.ts`
- **Modified** `app/src/main.tsx` — wrapped routes in `<AgentSurfaceProvider>`
- **Modified** `app/src/routes/debug.tsx` — added imports, `AgentSurfaceDemo` component, section 13

## Decisions I made

1. **Context-based registry instead of module-level Map.** A module-level Map with subscriptions would need manual cleanup coordination with React's lifecycle. A context provider auto-cleans up when the provider unmounts, and each `useAgentSurface` hook auto-unregisters on component unmount. This gives route-scoped cleanup for free: when React unmounts a route's components, all that route's registrations vanish.

2. **Callback ref pattern** for `useAgentSurface`. The hook returns a `ref` callback (not a `RefObject`) because React's `ref` attribute on host elements accepts callback refs, and this avoids needing the caller to manually manage a `useRef` + `useEffect` for registration. The callback ref fires with the real DOM node after mount.

3. **`onOpenPanel` callback** for disclosure elements. Rather than trying to generalize "open a panel" for arbitrary DOM elements (which is impossible without knowing the component's internal API), the hook accepts an optional `onOpenPanel` callback. For Select, this clicks the internal trigger button. For Modal, this sets the `open` state. If not provided, the `open_panel` intent fails with `action_not_allowed`. This is the minimal correct design — it doesn't build a generic disclosure framework.

4. **Intent-to-action mapping.** The 5 intents map to attribute values: `ui.focus → "focus"`, `ui.set_value → "set"`, `ui.click → "click"`, `ui.scroll_to → "scroll_to"`, `ui.open_panel → "open_panel"`. These match the spec's `data-agent-actions` vocabulary naturally.

5. **Interactability checks.** I check `isConnected` (not in DOM), `getComputedStyle` for `display:none` and `visibility:hidden`, and the `disabled` property on form elements. I deliberately did not check `offsetParent === null` (which can be false for valid elements in certain layouts) or `getBoundingClientRect` width/height (which can be zero for valid elements like off-screen focus targets). The checks are strict but not over-broad.

## What did NOT work

Nothing failed. The build and lint both pass cleanly. The controlled input test uses React's synthetic event system correctly via the native value setter + dispatched `input`/`change` events — this is the standard pattern for programmatically updating React-controlled inputs.

## Deviations from the plan

None. The implementation follows §3.9.1 exactly: 5 intents, no generic escape hatch, no `eval`, no selector, no 6th intent. Failed intents produce distinguishable results, never silent no-ops.

## Open questions

None. The 5-intent vocabulary covered every interaction needed for the debug page demonstration (form input, button click, scroll, focus, disclosure open). None of the failure modes suggested a missing intent. If a later block (e.g., P4.9's annotation pass across drivable routes) finds that `ui.set_value` needs to work against custom controlled primitives beyond native inputs (e.g., the Select component's controlled value), that's solvable by adding a setter callback to the registration — the architecture supports extension without adding intents.
