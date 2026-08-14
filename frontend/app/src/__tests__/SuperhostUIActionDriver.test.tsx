import { describe, expect, it, vi, afterEach, beforeAll } from "vitest";
import { useEffect, useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import { AgentSurfaceProvider, useAgentSurface } from "../components/agent-surface/context";
import type { AgentAction } from "../components/agent-surface/types";
import { ControlSessionProvider, useControlSession } from "../components/superhost/ControlSession";
import type { TerminalLine } from "../components/superhost/Terminal";
import type { GatedIntentResult } from "../components/superhost/driver-gated";
import type { AgentIntent, AgentSurfaceRegistry } from "../components/agent-surface/types";
import type { SuperhostStreamEvent } from "../lib/api/superhost-stream";

const gatedSpy = vi.fn<(_r: AgentSurfaceRegistry, _i: AgentIntent) => Promise<GatedIntentResult>>();

vi.mock("../components/superhost/driver-gated", () => ({
  createGatedDriver: vi.fn(() => gatedSpy),
}));

// Must import after the mock — dynamic import to avoid hoist ordering issues.
let useDriver: typeof import("../components/superhost/useSuperhostUIActionDriver").useSuperhostUIActionDriver;

beforeAll(async () => {
  const mod = await import("../components/superhost/useSuperhostUIActionDriver");
  useDriver = mod.useSuperhostUIActionDriver;
});

function makeEvent(
  event_id: string,
  event_name: string,
  event_data: Record<string, unknown>,
): SuperhostStreamEvent {
  return {
    event_id,
    run_id: "run-test",
    event_name,
    event_data,
    occurred_at: new Date().toISOString(),
  };
}

const UI_SURFACE_ID = "btn-test";
const UI_SURFACE_LABEL = "Test Button";

function TestSurface({ id }: { id: string }) {
  const { ref } = useAgentSurface(id, ["click"] as AgentAction[], UI_SURFACE_LABEL);
  return <button ref={ref} data-testid={id}>Click me</button>;
}

function makeEvents(
  toolName: string,
  extraArgs?: Record<string, unknown>,
): SuperhostStreamEvent[] {
  const proposeId = `ev-propose-${toolName}`;
  const allowedId = `ev-allowed-${toolName}`;
  const args = { surface_id: UI_SURFACE_ID, ...extraArgs };
  return [
    makeEvent(proposeId, "ToolCallProposed.v1", { tool_name: toolName, arguments: args }),
    makeEvent(allowedId, "PolicyAllowed.v1", {
      tool_name: toolName,
      result_summary: `ui action ${toolName} queued`,
    }),
  ];
}

type Captured = {
  lines: TerminalLine[];
  gatedCall: { intent: AgentIntent; registrySize: number } | null;
};

async function renderAndCapture(
  events: SuperhostStreamEvent[],
  gatedResult: GatedIntentResult = { ok: true, intent: { type: "ui.click", id: UI_SURFACE_ID } },
  startAsHistory = false,
): Promise<Captured | null> {
  gatedSpy.mockReset();
  gatedSpy.mockResolvedValue(gatedResult);

  return new Promise((resolve) => {
    const container = document.createElement("div");
    document.body.appendChild(container);
    const root = createRoot(container);

    function Capture() {
      const [done, setDone] = useState(false);
      const [liveEvents, setLiveEvents] = useState<SuperhostStreamEvent[]>(() => startAsHistory ? events : []);
      const lines = useDriver(liveEvents);
      const linesRef = useRef<TerminalLine[]>([]);
      linesRef.current = lines;
      const { grant } = useControlSession();

      useEffect(() => {
        grant();
        if (!startAsHistory) window.setTimeout(() => setLiveEvents(events), 0);
      }, [grant]);

      useEffect(() => {
        window.setTimeout(() => setDone(true), 100);
      }, []);

      useEffect(() => {
        if (!done) return;
        window.setTimeout(() => {
          if (linesRef.current.length > 0) {
            resolve({
              lines: linesRef.current,
              gatedCall: gatedSpy.mock.calls[0]
                ? { intent: gatedSpy.mock.calls[0][1], registrySize: gatedSpy.mock.calls[0][0].size }
                : null,
            });
          } else {
            resolve({ lines: linesRef.current, gatedCall: null });
          }
        }, 500);
      }, [done]);

      return null;
    }

    root.render(
      <AgentSurfaceProvider>
        <ControlSessionProvider>
          <TestSurface id={UI_SURFACE_ID} />
          <Capture />
        </ControlSessionProvider>
      </AgentSurfaceProvider>,
    );
  });
}

describe("useSuperhostUIActionDriver", () => {
  afterEach(() => {
    gatedSpy.mockReset();
  });

  it("calls gated driver for a ui_click propose+allowed pair", async () => {
    const events = makeEvents("ui_click");
    const captured = await renderAndCapture(events);

    expect(captured).not.toBeNull();
    expect(captured!.gatedCall).not.toBeNull();
    expect(captured!.gatedCall!.intent).toEqual({ type: "ui.click", id: UI_SURFACE_ID });
    expect(captured!.gatedCall!.registrySize).toBe(1);
  }, 10000);

  it("renders cached UI history without replaying its browser actions", async () => {
    const events = makeEvents("ui_click");
    const captured = await renderAndCapture(
      events,
      { ok: true, intent: { type: "ui.click", id: UI_SURFACE_ID } },
      true,
    );

    expect(captured).not.toBeNull();
    expect(captured!.gatedCall).toBeNull();
    expect(captured!.lines.some((line) => line.text.includes("prior browser actions were not replayed"))).toBe(true);
  }, 10000);

  it("produces an agent-line on successful ui_click", async () => {
    const events = makeEvents("ui_click");
    const captured = await renderAndCapture(events);

    expect(captured).not.toBeNull();
    const agentLines = captured!.lines.filter((l) => l.kind === "agent");
    expect(agentLines.length).toBeGreaterThanOrEqual(1);
    const didLine = agentLines.find((l) => l.text.includes("did:"));
    expect(didLine).toBeDefined();
    expect(didLine!.text).toContain("click:");
    expect(didLine!.text).toContain(UI_SURFACE_LABEL);
  }, 10000);

  it("never calls gated driver for non-ui_* tool events", async () => {
    const events = makeEvents("propose_check_in");
    const captured = await renderAndCapture(events);

    expect(captured).not.toBeNull();
    expect(captured!.gatedCall).toBeNull();
  }, 10000);

  it("produces a denial line when gated driver refuses the intent", async () => {
    const refusaLResult: GatedIntentResult = {
      ok: false,
      intent: { type: "ui.click", id: UI_SURFACE_ID },
      reason: "not_granted",
      detail: "control session check failed: not_granted",
    };

    const events = makeEvents("ui_click");
    const captured = await renderAndCapture(events, refusaLResult);

    expect(captured).not.toBeNull();
    const denialLines = captured!.lines.filter((l) => l.kind === "denial");
    expect(denialLines.length).toBeGreaterThanOrEqual(1);
    expect(denialLines[0].text).toContain("blocked:");
    expect(denialLines[0].text).toContain("not_granted");
  }, 10000);

  it("produces a denial line when surface_id is missing in arguments", async () => {
    const proposeId = "ev-propose-missing";
    const allowedId = "ev-allowed-missing";
    const events: SuperhostStreamEvent[] = [
      makeEvent(proposeId, "ToolCallProposed.v1", {
        tool_name: "ui_click",
        arguments: {},
      }),
      makeEvent(allowedId, "PolicyAllowed.v1", {
        tool_name: "ui_click",
        result_summary: "ui action ui_click queued",
      }),
    ];

    const captured = await renderAndCapture(events);

    expect(captured).not.toBeNull();
    const denialLines = captured!.lines.filter((l) => l.kind === "denial");
    expect(denialLines.length).toBeGreaterThanOrEqual(1);
    expect(denialLines[0].text).toContain("missing surface_id");
  }, 10000);

  it("handles ui_set_value with a value argument", async () => {
    const events = makeEvents("ui_set_value", { value: "hello world" });
    const captured = await renderAndCapture(events);

    expect(captured).not.toBeNull();
    expect(captured!.gatedCall).not.toBeNull();
    expect(captured!.gatedCall!.intent).toHaveProperty("value", "hello world");
  }, 10000);

  it("handles ui_focus", async () => {
    const events = makeEvents("ui_focus");
    const captured = await renderAndCapture(events);

    expect(captured).not.toBeNull();
    expect(captured!.gatedCall).not.toBeNull();
    expect(captured!.gatedCall!.intent.type).toBe("ui.focus");
  }, 10000);

  it("handles ui_scroll_to", async () => {
    const events = makeEvents("ui_scroll_to");
    const captured = await renderAndCapture(events);

    expect(captured).not.toBeNull();
    expect(captured!.gatedCall).not.toBeNull();
    expect(captured!.gatedCall!.intent.type).toBe("ui.scroll_to");
  }, 10000);

  it("handles ui_open_panel", async () => {
    const events = makeEvents("ui_open_panel");
    const captured = await renderAndCapture(events);

    expect(captured).not.toBeNull();
    expect(captured!.gatedCall).not.toBeNull();
    expect(captured!.gatedCall!.intent.type).toBe("ui.open_panel");
  }, 10000);

  it("passes events with non-ui_* PolicyAllowed through without driver interaction", async () => {
    const proposeId = "ev-propose-other";
    const allowedId = "ev-allowed-other";
    const events: SuperhostStreamEvent[] = [
      makeEvent(proposeId, "ToolCallProposed.v1", {
        tool_name: "get_weather",
        arguments: { city: "Lucknow" },
      }),
      makeEvent(allowedId, "PolicyAllowed.v1", {
        tool_name: "get_weather",
        result_summary: "weather fetched",
      }),
    ];

    const captured = await renderAndCapture(events);

    expect(captured).not.toBeNull();
    expect(captured!.gatedCall).toBeNull();
    expect(captured!.lines.length).toBe(0);
  }, 10000);
});
