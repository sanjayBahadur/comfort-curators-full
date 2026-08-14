import { useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it } from "vitest";
import { AgentSurfaceProvider, useAgentSurface } from "../components/agent-surface/context";
import type { AgentAction } from "../components/agent-surface/types";
import { ControlSessionProvider, useControlSession } from "../components/superhost/ControlSession";
import { useSuperhostUIActionDriver } from "../components/superhost/useSuperhostUIActionDriver";
import type { SuperhostStreamEvent } from "../lib/api/superhost-stream";

function event(id: string, name: string, tool: string, argumentsValue?: Record<string, unknown>): SuperhostStreamEvent {
  return {
    event_id: id,
    run_id: "run-cart-demo",
    event_name: name,
    event_data: name === "ToolCallProposed.v1"
      ? { tool_name: tool, arguments: argumentsValue }
      : { tool_name: tool, result_summary: `${tool} allowed` },
    occurred_at: "2026-08-13T12:00:00Z",
  };
}

const clickEvents = [
  event("coffee-proposed", "ToolCallProposed.v1", "ui_click", { surface_id: "shop-catalog-add-coffee" }),
  event("coffee-allowed", "PolicyAllowed.v1", "ui_click"),
  event("kit-proposed", "ToolCallProposed.v1", "ui_click", { surface_id: "shop-catalog-add-welcome" }),
  event("kit-allowed", "PolicyAllowed.v1", "ui_click"),
];

function AddSurface({ id, label, onAdd }: { id: string; label: string; onAdd: () => void }) {
  const surface = useAgentSurface(id, ["click"] as AgentAction[], label);
  return <button ref={surface.ref} data-agent-drag-target="cart-zone" onClick={onAdd}>{label}</button>;
}

function CartHarness({ onComplete }: { onComplete: (value: { coffee: number; welcome: number; didLines: number }) => void }) {
  const [coffee, setCoffee] = useState(0);
  const [welcome, setWelcome] = useState(0);
  const [events, setEvents] = useState<SuperhostStreamEvent[]>([]);
  const { grant, session } = useControlSession();
  const lines = useSuperhostUIActionDriver(events);

  useEffect(() => {
    grant();
  }, [grant]);

  useEffect(() => {
    if (session.state === "granted") setEvents(clickEvents);
  }, [session.state]);

  useEffect(() => {
    if (coffee === 1 && welcome === 1 && lines.filter((line) => line.text.startsWith("did: click:")).length === 2) {
      onComplete({ coffee, welcome, didLines: 2 });
    }
  }, [coffee, welcome, lines, onComplete]);

  return (
    <>
      <AddSurface id="shop-catalog-add-coffee" label="Add Filter Coffee 100g to the package" onAdd={() => setCoffee((value) => value + 1)} />
      <AddSurface id="shop-catalog-add-welcome" label="Add Welcome Kit — Premium to the package" onAdd={() => setWelcome((value) => value + 1)} />
      <div data-agent-drop-zone="cart-zone">Cart</div>
    </>
  );
}

describe("Superhost cart automation", () => {
  it("applies two real gated ui_click events to registered cart controls", async () => {
    const container = document.createElement("div");
    document.body.appendChild(container);
    const root = createRoot(container);

    const result = await new Promise<{ coffee: number; welcome: number; didLines: number }>((resolve, reject) => {
      // The test deliberately waits through two full visual ring + drag
      // animations. Leave headroom when all jsdom files contend in parallel.
      const timeout = window.setTimeout(() => reject(new Error("cart automation did not finish")), 15000);
      root.render(
        <AgentSurfaceProvider>
          <ControlSessionProvider>
            <CartHarness onComplete={(value) => {
              window.clearTimeout(timeout);
              resolve(value);
            }} />
          </ControlSessionProvider>
        </AgentSurfaceProvider>,
      );
    });

    expect(result).toEqual({ coffee: 1, welcome: 1, didLines: 2 });
    root.unmount();
    container.remove();
  }, 20000);
});
