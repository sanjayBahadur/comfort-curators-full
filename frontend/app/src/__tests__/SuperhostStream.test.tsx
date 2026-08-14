import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  useSuperhostStream,
  type SuperhostStreamEvent,
} from "../lib/api/superhost-stream";

function event(eventId: string, runId: string): SuperhostStreamEvent {
  return {
    event_id: eventId,
    run_id: runId,
    event_name: "AgentRunCompleted.v1",
    occurred_at: "2026-08-10T12:00:00Z",
  };
}

function streamResponse(events: SuperhostStreamEvent[]): Response {
  const encoder = new TextEncoder();
  const body = events
    .map((streamEvent) => `data: ${JSON.stringify(streamEvent)}\n\n`)
    .join("") + "data: [DONE]\n\n";

  return new Response(new ReadableStream({
    start(controller) {
      controller.enqueue(encoder.encode(body));
      controller.close();
    },
  }), {
    status: 200,
    headers: { "Content-Type": "text/event-stream" },
  });
}

function Harness({ threadId, generation }: { threadId: string; generation: number }) {
  const stream = useSuperhostStream(threadId, true, generation);
  return (
    <div>
      <span data-testid="state">{stream.state}</span>
      <span data-testid="events">{stream.events.map((item) => item.event_id).join(",")}</span>
    </div>
  );
}

describe("useSuperhostStream", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("reconnects only when requested and merges replayed same-thread events by event_id", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(streamResponse([event("event-one", "run-one")]))
      .mockResolvedValueOnce(streamResponse([
        event("event-one", "run-one"),
        event("event-two", "run-two"),
      ]));
    vi.stubGlobal("fetch", fetchMock);

    const rendered = render(<Harness threadId="thread-one" generation={0} />);
    await waitFor(() => expect(screen.getByTestId("state").textContent).toBe("done"));
    expect(screen.getByTestId("events").textContent).toBe("event-one");
    expect(fetchMock).toHaveBeenCalledTimes(1);

    rendered.rerender(<Harness threadId="thread-one" generation={0} />);
    expect(fetchMock).toHaveBeenCalledTimes(1);

    rendered.rerender(<Harness threadId="thread-one" generation={1} />);
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(screen.getByTestId("state").textContent).toBe("done"));
    expect(screen.getByTestId("events").textContent).toBe("event-one,event-two");
  });

  it("clears prior history when connecting to a different thread", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(streamResponse([event("event-one", "run-one")]))
      .mockResolvedValueOnce(streamResponse([event("event-three", "run-three")]));
    vi.stubGlobal("fetch", fetchMock);

    const rendered = render(<Harness threadId="thread-one" generation={0} />);
    await waitFor(() => expect(screen.getByTestId("events").textContent).toBe("event-one"));

    rendered.rerender(<Harness threadId="thread-two" generation={0} />);
    await waitFor(() => expect(screen.getByTestId("events").textContent).toBe("event-three"));
  });
});
