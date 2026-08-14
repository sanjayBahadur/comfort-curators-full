import { describe, expect, it } from "vitest";
import { useEffect, useState, type ReactNode } from "react";
import { createRoot } from "react-dom/client";
import { AgentSurfaceProvider, useAgentSurfaceContext, useAgentSurface } from "../components/agent-surface/context";
import type { AgentAction } from "../components/agent-surface/types";
import {
  PaymentBoundary,
  PaymentBoundaryButton,
  PaymentBoundaryTriggerButton,
} from "../components/superhost/PaymentBoundary";
import { ControlSessionProvider, useControlSession } from "../components/superhost/ControlSession";

async function waitUntil(predicate: () => boolean, timeoutMs = 1500) {
  const deadline = Date.now() + timeoutMs;
  while (!predicate()) {
    if (Date.now() >= deadline) throw new Error("condition did not settle before timeout");
    await new Promise((resolve) => window.setTimeout(resolve, 20));
  }
}

function TestChild({ id }: { id: string }) {
  const { ref } = useAgentSurface(id, ["click"] as AgentAction[], `Test element ${id}`);
  return <button ref={ref} data-testid={id}>Pay {id}</button>;
}

type RegistrySnapshot = { size: number; ids: string[] };

describe("Gate 2 — PaymentBoundary structural unreachability", () => {
  function renderAndSnapshot(ui: ReactNode): Promise<RegistrySnapshot | null> {
    return new Promise((resolve) => {
      const container = document.createElement("div");
      document.body.appendChild(container);
      const root = createRoot(container);

      function Capture() {
        const { registry } = useAgentSurfaceContext();
        const [done, setDone] = useState(false);

        useEffect(() => {
          window.setTimeout(() => setDone(true), 0);
        }, []);

        if (!done) return null;

        resolve({ size: registry.size, ids: Array.from(registry.keys()) });
        return null;
      }

      root.render(
        <AgentSurfaceProvider>
          <ControlSessionProvider>
            {ui}
            <Capture />
          </ControlSessionProvider>
        </AgentSurfaceProvider>,
      );
    });
  }

  it("prevents elements inside PaymentBoundary from registering", async () => {
    const snapshot = await renderAndSnapshot(
      <PaymentBoundary>
        <TestChild id="pay-button" />
      </PaymentBoundary>,
    );

    expect(snapshot).not.toBeNull();
    expect(snapshot!.size).toBe(0);
    expect(snapshot!.ids).not.toContain("pay-button");
  });

  it("allows elements outside PaymentBoundary to register", async () => {
    const snapshot = await renderAndSnapshot(<TestChild id="safe-button" />);

    expect(snapshot).not.toBeNull();
    expect(snapshot!.size).toBe(1);
    expect(snapshot!.ids).toContain("safe-button");
  });

  it("does not set data-agent attributes on elements inside PaymentBoundary", async () => {
    return new Promise<void>((resolve) => {
      const container = document.createElement("div");
      document.body.appendChild(container);
      const root = createRoot(container);

      function Capture() {
        const [done, setDone] = useState(false);

        useEffect(() => {
          window.setTimeout(() => setDone(true), 0);
        }, []);

        if (!done) return null;

        const el = container.querySelector('[data-testid="stripped"]');
        expect(el).not.toBeNull();
        expect(el?.hasAttribute("data-agent")).toBe(false);
        expect(el?.hasAttribute("data-agent-actions")).toBe(false);
        expect(el?.hasAttribute("data-agent-label")).toBe(false);
        root.unmount();
        container.remove();
        resolve();
        return null;
      }

      root.render(
        <AgentSurfaceProvider>
          <ControlSessionProvider>
            <PaymentBoundary>
              <TestChild id="stripped" />
            </PaymentBoundary>
            <Capture />
          </ControlSessionProvider>
        </AgentSurfaceProvider>,
      );
    });
  });

  it("multiple children inside PaymentBoundary all fail to register", async () => {
    const snapshot = await renderAndSnapshot(
      <PaymentBoundary>
        <TestChild id="button-a" />
        <TestChild id="button-b" />
        <TestChild id="button-c" />
      </PaymentBoundary>,
    );

    expect(snapshot).not.toBeNull();
    expect(snapshot!.size).toBe(0);
  });

  it("hands control back on the first protected click and executes only after a second human click", async () => {
    const container = document.createElement("div");
    document.body.appendChild(container);
    const root = createRoot(container);

    function Harness() {
      const { grant } = useControlSession();
      const [executions, setExecutions] = useState(0);
      useEffect(() => grant(), [grant]);
      return (
        <>
          <PaymentBoundary>
            <PaymentBoundaryButton data-testid="protected" onClick={() => setExecutions((value) => value + 1)}>
              Activate
            </PaymentBoundaryButton>
          </PaymentBoundary>
          <output data-testid="executions">{executions}</output>
        </>
      );
    }

    root.render(
      <AgentSurfaceProvider>
        <ControlSessionProvider><Harness /></ControlSessionProvider>
      </AgentSurfaceProvider>,
    );

    await new Promise((resolve) => window.setTimeout(resolve, 30));
    const button = container.querySelector<HTMLButtonElement>('[data-testid="protected"]')!;
    button.click();
    await waitUntil(() => document.body.textContent?.includes("CONTROL REVOKED / PAYMENT BOUNDARY") === true);
    expect(container.querySelector('[data-testid="executions"]')?.textContent).toBe("0");
    expect(document.body.textContent).toContain("CONTROL REVOKED / PAYMENT BOUNDARY");

    button.click();
    await waitUntil(() => container.querySelector('[data-testid="executions"]')?.textContent === "1");
    expect(container.querySelector('[data-testid="executions"]')?.textContent).toBe("1");

    root.unmount();
    container.remove();
  });

  it("lets the model hit an external checkout tripwire without exposing the protected subtree", async () => {
    const container = document.createElement("div");
    document.body.appendChild(container);
    const root = createRoot(container);

    function Harness() {
      const { grant, session } = useControlSession();
      const [executions, setExecutions] = useState(0);
      useEffect(() => grant(), [grant]);
      return (
        <>
          <PaymentBoundary boundaryId="checkout-test">
            <div data-testid="protected-tree">
              <PaymentBoundaryButton onClick={() => setExecutions((value) => value + 1)}>
                Human payment
              </PaymentBoundaryButton>
            </div>
          </PaymentBoundary>
          {session.state === "granted" && (
            <PaymentBoundaryTriggerButton
              boundaryId="checkout-test"
              agentId="checkout-tripwire"
              agentLabel="Request owner review"
            >
              Review and activate
            </PaymentBoundaryTriggerButton>
          )}
          <output data-testid="executions">{executions}</output>
          <output data-testid="session">{session.state}</output>
        </>
      );
    }

    root.render(
      <AgentSurfaceProvider>
        <ControlSessionProvider><Harness /></ControlSessionProvider>
      </AgentSurfaceProvider>,
    );

    await new Promise((resolve) => window.setTimeout(resolve, 30));
    const protectedTree = container.querySelector('[data-testid="protected-tree"]')!;
    expect(protectedTree.querySelector("[data-agent]")).toBeNull();
    const tripwire = container.querySelector<HTMLButtonElement>('[data-agent="checkout-tripwire"]')!;
    expect(tripwire).not.toBeNull();

    tripwire.click();
    await new Promise((resolve) => window.setTimeout(resolve, 30));
    expect(container.querySelector('[data-testid="session"]')?.textContent).toBe("idle");
    expect(container.querySelector('[data-testid="executions"]')?.textContent).toBe("0");
    expect(document.body.textContent).toContain("CONTROL REVOKED / PAYMENT BOUNDARY");

    root.unmount();
    container.remove();
  });
});
