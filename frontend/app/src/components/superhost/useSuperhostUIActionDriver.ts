import { useEffect, useRef, useState } from "react";
import { useAgentSurfaceContext } from "../agent-surface/context";
import { createGatedDriver } from "./driver-gated";
import { useControlSession } from "./ControlSession";
import type { SuperhostStreamEvent } from "../../lib/api/superhost-stream";
import type { AgentIntent } from "../agent-surface/types";
import type { TerminalLine } from "./Terminal";

type QueuedArgs = { tool_name: string; arguments: Record<string, unknown> };

function toIntent(toolName: string, args: { surface_id: string; value?: string }): AgentIntent | null {
  switch (toolName) {
    case "ui_focus": return { type: "ui.focus", id: args.surface_id };
    case "ui_set_value": return { type: "ui.set_value", id: args.surface_id, value: args.value ?? "" };
    case "ui_click": return { type: "ui.click", id: args.surface_id };
    case "ui_scroll_to": return { type: "ui.scroll_to", id: args.surface_id };
    case "ui_open_panel": return { type: "ui.open_panel", id: args.surface_id };
    default: return null;
  }
}

const UI_TOOLS = new Set(["ui_focus", "ui_set_value", "ui_click", "ui_scroll_to", "ui_open_panel"]);

function restoredHistoryLine(events: SuperhostStreamEvent[], driverKey: string): TerminalLine[] {
  const hasHistoricalUIActions = events.some((event) =>
    event.event_name === "ToolCallProposed.v1" || event.event_name === "PolicyAllowed.v1");
  return hasHistoricalUIActions
    ? [{
        id: `ui-history-${driverKey}`,
        kind: "system",
        text: "history restored: prior browser actions were not replayed",
      }]
    : [];
}

export function useSuperhostUIActionDriver(events: SuperhostStreamEvent[], driverKey = "default"): TerminalLine[] {
  const { registry } = useAgentSurfaceContext();
  const ctx = useControlSession();
  const ctxRef = useRef(ctx);
  ctxRef.current = ctx;
  const gatedRunRef = useRef(createGatedDriver(() => ctxRef.current));
  // `events` begins with cached/thread history. Treat that initial snapshot as
  // read-only terminal history; only events appended after this driver is live
  // may touch the browser. Previously every drawer remount replayed all old
  // PolicyAllowed events, producing repeated dashboard clicks and a wall of
  // misleading `not_granted` failures.
  //
  // That id-set baseline only protects events already present in `events` at
  // the moment this baseline is taken. It does NOT protect a thread that
  // wasn't cached in this browser yet: readCache() returns [] on a genuine
  // cache miss, so the baseline is empty, and the thread's real server-side
  // backlog (which can be a long, heavily-used thread's entire history)
  // then streams in afterward as if brand new -- every one of those old
  // PolicyAllowed UI-tool events gets *actually re-executed*, for real,
  // against a session that (correctly) has no control granted yet on a
  // fresh page load. Confirmed live: opening Superhost fresh on a
  // long-lived thread showed a wall of `not_granted` failures as the very
  // first thing, before the person had done anything at all.
  //
  // The wall-clock cutoff below closes that gap: anything that happened
  // before this driver mounted is history, full stop, regardless of
  // whether it happened to already be cached or is only now streaming in
  // for the first time. Comparing occurred_at (server time) against
  // Date.now() (client time) assumes clocks that are close enough --
  // true for this stack (client and server both run on localhost).
  const mountCutoffRef = useRef(Date.now());
  const driverKeyRef = useRef(driverKey);
  const processedRef = useRef<Set<string>>(new Set(events.map((event) => event.event_id)));
  const queueRef = useRef<Map<string, QueuedArgs[]>>(new Map());
  const chainRef = useRef<Promise<void>>(Promise.resolve());
  const [actionLines, setActionLines] = useState<TerminalLine[]>(() => restoredHistoryLine(events, driverKey));
  const mountedRef = useRef(true);

  if (driverKeyRef.current !== driverKey) {
    driverKeyRef.current = driverKey;
    mountCutoffRef.current = Date.now();
    processedRef.current = new Set(events.map((event) => event.event_id));
    queueRef.current.clear();
    chainRef.current = Promise.resolve();
  }

  useEffect(() => {
    mountedRef.current = true;
    return () => { mountedRef.current = false; };
  }, []);

  useEffect(() => {
    setActionLines(restoredHistoryLine(events, driverKey));
    // The current event snapshot was synchronously baselined above when the
    // key changed, so this only resets the visible per-thread driver log.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [driverKey]);

  useEffect(() => {
    const queue = queueRef.current;
    const processed = processedRef.current;
    const gatedRun = gatedRunRef.current;
    const cutoff = mountCutoffRef.current;
    const newActions: Array<{ eventId: string; toolName: string; intent: AgentIntent }> = [];

    for (const event of events) {
      if (processed.has(event.event_id)) continue;
      // Backlog arriving late (see mountCutoffRef above): mark it seen so
      // it's never reconsidered, but never let it queue or execute a real
      // browser action -- the ToolCallProposed/PolicyAllowed pairing logic
      // below already tolerates an orphaned entry on either side of a
      // skipped pair (an empty queue.get() on the paired event just
      // continues harmlessly), so skipping both consistently here is safe.
      if (Date.parse(event.occurred_at) <= cutoff) {
        processed.add(event.event_id);
        continue;
      }
      processed.add(event.event_id);

      const edata = event.event_data && typeof event.event_data === "object"
        ? event.event_data as Record<string, unknown>
        : {};

      if (event.event_name === "ToolCallProposed.v1") {
        const toolName = typeof edata["tool_name"] === "string" ? edata["tool_name"] : "";
        if (!UI_TOOLS.has(toolName)) continue;
        const args = edata["arguments"] && typeof edata["arguments"] === "object"
          ? edata["arguments"] as Record<string, unknown>
          : {};
        const entry = queue.get(toolName) ?? [];
        entry.push({ tool_name: toolName, arguments: args });
        queue.set(toolName, entry);
      } else if (
        event.event_name === "PolicyAllowed.v1" ||
        event.event_name === "PolicyDenied.v1" ||
        event.event_name === "ApprovalRequired.v1"
      ) {
        const toolName = typeof edata["tool_name"] === "string" ? edata["tool_name"] : "";
        if (!UI_TOOLS.has(toolName)) continue;

        const entry = queue.get(toolName);
        if (!entry || entry.length === 0) continue;
        const paired = entry.shift()!;
        if (entry.length === 0) queue.delete(toolName);

        if (event.event_name === "PolicyAllowed.v1") {
          const args = paired.arguments;
          const surfaceId = typeof args["surface_id"] === "string" ? args["surface_id"] : "";
          if (!surfaceId) {
            const line: TerminalLine = {
              id: `ui-action-${event.event_id}`,
              kind: "denial",
              text: `blocked: missing surface_id in ${toolName} arguments`,
            };
            chainRef.current = chainRef.current.then(async () => {
              if (mountedRef.current) {
                setActionLines((prev) => [...prev, line]);
              }
            });
            continue;
          }
          const value = typeof args["value"] === "string" ? args["value"] : undefined;
          const intent = toIntent(toolName, { surface_id: surfaceId, value });
          if (intent) {
            newActions.push({ eventId: event.event_id, toolName, intent });
          }
        }
      }
    }

    if (newActions.length === 0) return;

    chainRef.current = chainRef.current.then(async () => {
      const outcomeLines: TerminalLine[] = [];

      for (const action of newActions) {
        if (!mountedRef.current) break;

        const entry = registry.get(action.intent.id);
        const label = entry?.label ?? action.intent.id;

        const result = await gatedRun(registry, action.intent);

        if (result.ok) {
          const desc = `${action.toolName.replace(/^ui_/, "")}: ${label}`;
          outcomeLines.push({
            id: `ui-action-${action.eventId}`,
            kind: "agent",
            text: `did: ${desc}`,
          });
        } else {
          outcomeLines.push({
            id: `ui-action-${action.eventId}`,
            kind: "denial",
            text: `blocked: ${result.reason} — ${result.detail}`,
          });
        }
      }

      if (mountedRef.current) {
        setActionLines((prev) => [...prev, ...outcomeLines]);
      }
    });
  }, [events, registry]);

  return actionLines;
}
