import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import TaskChecklist from "../components/superhost/TaskChecklist";
import {
  buildSuperhostChecklist,
  deriveChecklistTaskState,
  type ChecklistTaskSeed,
} from "../components/superhost/task-checklist";
import type { TerminalLine } from "../components/superhost/Terminal";
import type { SuperhostStreamEvent } from "../lib/api/superhost-stream";

function event(
  eventId: string,
  runId: string,
  eventName: string,
  eventData: Record<string, unknown> = {},
): SuperhostStreamEvent {
  return {
    event_id: eventId,
    run_id: runId,
    event_name: eventName,
    event_data: eventData,
    occurred_at: "2026-08-09T12:00:00Z",
  };
}

function line(id: string, text: string, kind: TerminalLine["kind"] = "agent"): TerminalLine {
  return { id, text, kind };
}

const seeds: ChecklistTaskSeed[] = [
  { id: "task-one", line: line("operator-one", "Inspect the dashboard", "operator"), runId: "run-one" },
  { id: "task-two", line: line("operator-two", "Open the ticket", "operator"), runId: "run-two" },
];

describe("Superhost task checklist projection", () => {
  it("groups steps by the submission's real run ID and keeps unrelated activity visible", () => {
    const events = [
      event("initial", "initial-run", "AgentRunQueued.v1"),
      event("one-proposal", "run-one", "ToolCallProposed.v1"),
      event("two-proposal", "run-two", "ToolCallProposed.v1"),
      event("one-complete", "run-one", "AgentRunCompleted.v1"),
      event("one-after-terminal", "run-one", "PolicyAllowed.v1"),
    ];
    const streamLines = [
      line("initial", "run queued", "system"),
      line("one-proposal", "proposed ui_click"),
      line("two-proposal", "proposed ui_open_panel"),
      line("one-complete", "run completed", "system"),
      line("one-after-terminal", "late result"),
    ];

    const result = buildSuperhostChecklist(events, seeds, streamLines, []);

    expect(result.tasks[0].steps.map((step) => step.id)).toEqual(["one-proposal", "one-complete"]);
    expect(result.tasks[0].state).toBe("done");
    expect(result.tasks[1].steps.map((step) => step.id)).toEqual(["two-proposal"]);
    expect(result.tasks[1].state).toBe("running");
    expect(result.unassignedLines.map((step) => step.id)).toEqual(["initial", "one-after-terminal"]);
  });

  it("renders approvals and real ui driver outcomes as nested steps", () => {
    const events = [
      event("proposal", "run-one", "ToolCallProposed.v1", { tool_name: "ui_click" }),
      event("allowed", "run-one", "PolicyAllowed.v1", { tool_name: "ui_click" }),
      event("approval", "run-one", "ApprovalRequired.v1", {
        tool_name: "create_ticket",
        summary: "Create a maintenance ticket",
      }),
    ];
    const result = buildSuperhostChecklist(
      events,
      seeds.slice(0, 1),
      [line("proposal", "proposed ui_click"), line("allowed", "ui click queued")],
      [line("ui-action-allowed", "did: click: Open ticket")],
    );

    expect(result.tasks[0].state).toBe("waiting");
    expect(result.tasks[0].steps.map((step) => step.text)).toEqual([
      "proposed ui_click",
      "ui click queued",
      "did: click: Open ticket",
      "approval required · create_ticket: Create a maintenance ticket",
    ]);
  });

  it("distinguishes queued, running, waiting, denied, failed, clean completion, and ambiguity", () => {
    expect(deriveChecklistTaskState([])).toBe("not_started");
    expect(deriveChecklistTaskState([event("queued", "run", "AgentRunQueued.v1")])).toBe("not_started");
    expect(deriveChecklistTaskState([event("proposal", "run", "ToolCallProposed.v1")])).toBe("running");
    expect(deriveChecklistTaskState([event("approval", "run", "ApprovalRequired.v1")])).toBe("waiting");
    expect(deriveChecklistTaskState([
      event("denied", "run", "PolicyDenied.v1"),
      event("complete", "run", "AgentRunCompleted.v1"),
    ])).toBe("denied");
    expect(deriveChecklistTaskState([
      event("denied", "run", "PolicyDenied.v1"),
      event("next-proposal", "run", "ToolCallProposed.v1"),
    ])).toBe("running");
    expect(deriveChecklistTaskState([event("failed", "run", "AgentRunFailed.v1")])).toBe("blocked");
    expect(deriveChecklistTaskState([
      event("allowed", "run", "PolicyAllowed.v1"),
      event("complete", "run", "AgentRunCompleted.v1"),
    ])).toBe("done");
    expect(deriveChecklistTaskState([event("future", "run", "UnknownFutureEvent.v1")])).toBe("unknown");
  });

  it("renders task markers, labels, and indented denial semantics", () => {
    render(<TaskChecklist tasks={[
      {
        id: "denied-task",
        title: "Pay the vendor",
        runId: "run-denied",
        state: "denied",
        steps: [line("denial", "Payment requires owner approval", "denial")],
      },
    ]} />);

    const checklist = screen.getByRole("list", { name: "Superhost task checklist" });
    expect(within(checklist).getByText("[!]")).toBeTruthy();
    expect(within(checklist).getByText("DENIED")).toBeTruthy();
    expect(within(checklist).getByRole("alert").textContent).toContain("Payment requires owner approval");
  });
});
