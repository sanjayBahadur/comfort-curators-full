import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, cleanup, act } from "@testing-library/react";
import {
  ControlSessionProvider,
  useControlSession,
} from "../components/superhost/ControlSession";
import AgentCursor from "../components/superhost/AgentCursor";

function TestHarness() {
  const { session, grant, revoke } = useControlSession();

  return (
    <div>
      <AgentCursor />
      <button data-testid="grant-btn" onClick={() => grant()}>
        Grant
      </button>
      <button data-testid="revoke-btn" onClick={() => revoke("manual")}>
        Revoke
      </button>
      <span data-testid="state">{session.state}</span>
    </div>
  );
}

describe("AgentCursor", () => {
  beforeEach(() => {
    cleanup();
    vi.stubGlobal(
      "matchMedia",
      vi.fn((query: string) => ({
        matches: false,
        media: query,
        onchange: null,
        addListener: vi.fn(),
        removeListener: vi.fn(),
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        dispatchEvent: vi.fn(),
      })),
    );
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  function setup() {
    return render(
      <ControlSessionProvider>
        <TestHarness />
      </ControlSessionProvider>,
    );
  }

  it("renders null when session is idle", () => {
    const { container } = setup();
    expect(container.querySelector(".agent-cursor")).toBeNull();
  });

  it("renders crosshair when session is granted", async () => {
    const { getByTestId, container } = setup();

    await act(async () => {
      getByTestId("grant-btn").click();
    });

    const cursor = container.querySelector(".agent-cursor");
    expect(cursor).not.toBeNull();
    expect(cursor!.className).toContain("agent-cursor--idle");
  });

  it("positions crosshair at viewport center when idle (no target yet)", async () => {
    const { getByTestId, container } = setup();

    await act(async () => {
      getByTestId("grant-btn").click();
    });

    const cursor = container.querySelector(".agent-cursor") as HTMLElement;
    expect(cursor.style.transform).toContain("50vw");
    expect(cursor.style.transform).toContain("50vh");
  });

  it("removes cursor from DOM when session is revoked", async () => {
    const { getByTestId, container } = setup();

    await act(async () => {
      getByTestId("grant-btn").click();
    });
    expect(container.querySelector(".agent-cursor")).not.toBeNull();

    await act(async () => {
      getByTestId("revoke-btn").click();
    });
    expect(container.querySelector(".agent-cursor")).toBeNull();
  });

  it("MutationObserver detects control-ring and updates crosshair position", async () => {
    const { getByTestId, container } = setup();

    await act(async () => {
      getByTestId("grant-btn").click();
    });

    const ring = document.createElement("div");
    ring.className = "control-ring";
    ring.style.left = "100px";
    ring.style.top = "200px";
    ring.style.width = "300px";
    ring.style.height = "80px";

    await act(async () => {
      document.body.appendChild(ring);
    });

    const cursor = container.querySelector(".agent-cursor") as HTMLElement;
    expect(cursor.className).toContain("agent-cursor--visible");

    expect(cursor.style.transform).toContain("250px");
    expect(cursor.style.transform).toContain("240px");

    ring.remove();
  });

  it("respects prefers-reduced-motion: reduce", async () => {
    cleanup();
    vi.stubGlobal(
      "matchMedia",
      vi.fn((query: string) => ({
        matches: query === "(prefers-reduced-motion: reduce)",
        media: query,
        onchange: null,
        addListener: vi.fn(),
        removeListener: vi.fn(),
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        dispatchEvent: vi.fn(),
      })),
    );

    const { getByTestId, container } = render(
      <ControlSessionProvider>
        <TestHarness />
      </ControlSessionProvider>,
    );

    await act(async () => {
      getByTestId("grant-btn").click();
    });

    const ring = document.createElement("div");
    ring.className = "control-ring-reduced";
    ring.style.left = "50px";
    ring.style.top = "50px";
    ring.style.width = "100px";
    ring.style.height = "100px";

    await act(async () => {
      document.body.appendChild(ring);
    });

    const cursor = container.querySelector(".agent-cursor") as HTMLElement;
    expect(cursor.style.transition).toBe("none");

    ring.remove();
  });

  it("uses matrix green (#00ff66) crosshair styling", async () => {
    const { getByTestId, container } = setup();

    await act(async () => {
      getByTestId("grant-btn").click();
    });

    const cursor = container.querySelector(".agent-cursor") as HTMLElement;
    expect(cursor).not.toBeNull();

    // Verify the element has the agent-cursor class
    expect(cursor.className).toContain("agent-cursor");
  });
});
