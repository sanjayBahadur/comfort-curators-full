import type { SuperhostStreamEvent } from "../../lib/api/superhost-stream";
import type { TerminalLine } from "./Terminal";

export type ChecklistTaskSeed = {
  id: string;
  line: TerminalLine;
  runId: string | null;
  localLines?: TerminalLine[];
};

export type ChecklistTaskState =
  | "not_started"
  | "running"
  | "done"
  | "waiting"
  | "denied"
  | "blocked"
  | "unknown";

export type ChecklistTask = {
  id: string;
  title: string;
  runId: string | null;
  state: ChecklistTaskState;
  steps: TerminalLine[];
};

export type SuperhostChecklist = {
  tasks: ChecklistTask[];
  unassignedLines: TerminalLine[];
};

const TERMINAL_EVENTS = new Set([
  "AgentRunCompleted.v1",
  "AgentRunFailed.v1",
  "AgentRunCancelled.v1",
]);

const MEANINGFUL_EVENTS = new Set([
  "ToolCallProposed.v1",
  "PolicyAllowed.v1",
  "PolicyDenied.v1",
  "ApprovalRequired.v1",
]);

const ACTIVE_EVENTS = new Set([
  "ToolCallProposed.v1",
  "PolicyAllowed.v1",
  "AgentRunFallback.v1",
  "AgentRunToken.v1",
]);

function eventData(event: SuperhostStreamEvent): Record<string, unknown> {
  return event.event_data && typeof event.event_data === "object"
    ? event.event_data as Record<string, unknown>
    : {};
}

function stringValue(data: Record<string, unknown>, key: string): string {
  return typeof data[key] === "string" ? data[key] : "";
}

function approvalLine(event: SuperhostStreamEvent): TerminalLine {
  const data = eventData(event);
  const summary = stringValue(data, "summary");
  const toolName = stringValue(data, "tool_name");
  return {
    id: event.event_id,
    kind: "system",
    text: `approval required${toolName ? ` · ${toolName}` : ""}${summary ? `: ${summary}` : ""}`,
  };
}

function eventsThroughTerminal(events: SuperhostStreamEvent[]): SuperhostStreamEvent[] {
  const terminalIndex = events.findIndex((event) => TERMINAL_EVENTS.has(event.event_name));
  return terminalIndex === -1 ? events : events.slice(0, terminalIndex + 1);
}

export function deriveChecklistTaskState(events: SuperhostStreamEvent[]): ChecklistTaskState {
  if (events.length === 0 || events.every((event) => event.event_name === "AgentRunQueued.v1")) {
    return "not_started";
  }

  const terminal = events.findLast((event) => TERMINAL_EVENTS.has(event.event_name));
  if (terminal?.event_name === "AgentRunFailed.v1" || terminal?.event_name === "AgentRunCancelled.v1") {
    return "blocked";
  }

  const lastMeaningfulEvent = events.findLast((event) => MEANINGFUL_EVENTS.has(event.event_name));
  if (lastMeaningfulEvent?.event_name === "ApprovalRequired.v1") return "waiting";
  if (lastMeaningfulEvent?.event_name === "PolicyDenied.v1") return "denied";
  if (terminal?.event_name === "AgentRunCompleted.v1") return "done";

  if (!events.some((event) => ACTIVE_EVENTS.has(event.event_name))) return "unknown";

  return "running";
}

/**
 * Projects stream facts into checklist tasks. A submitted operator line is
 * associated only with the run ID returned by that submission; events are
 * never assigned by timing guesses. Events outside those runs stay visible as
 * unassigned terminal activity.
 */
export function buildSuperhostChecklist(
  events: SuperhostStreamEvent[],
  seeds: ChecklistTaskSeed[],
  streamLines: TerminalLine[],
  uiDriverLines: TerminalLine[],
): SuperhostChecklist {
  const streamLineById = new Map(streamLines.map((line) => [line.id, line]));
  const uiLineByEventId = new Map<string, TerminalLine>();

  for (const event of events) {
    const line = uiDriverLines.find((candidate) => candidate.id === `ui-action-${event.event_id}`);
    if (line) uiLineByEventId.set(event.event_id, line);
  }

  const assignedEventIds = new Set<string>();
  const tasks = seeds.map((seed): ChecklistTask => {
    const taskEvents = seed.runId
      ? eventsThroughTerminal(events.filter((event) => event.run_id === seed.runId))
      : [];
    const steps: TerminalLine[] = [];

    for (const event of taskEvents) {
      assignedEventIds.add(event.event_id);
      const mappedLine = streamLineById.get(event.event_id);
      if (mappedLine) steps.push(mappedLine);
      else if (event.event_name === "ApprovalRequired.v1") steps.push(approvalLine(event));

      const uiLine = uiLineByEventId.get(event.event_id);
      if (uiLine) steps.push(uiLine);
    }

    steps.push(...(seed.localLines ?? []));

    return {
      id: seed.id,
      title: seed.line.text,
      runId: seed.runId,
      state: deriveChecklistTaskState(taskEvents),
      steps,
    };
  });

  const unassignedStreamLines = streamLines.filter((line) => !assignedEventIds.has(line.id));
  const assignedUILineIds = new Set(tasks.flatMap((task) => task.steps.map((line) => line.id)));
  const unassignedUILines = uiDriverLines.filter((line) => !assignedUILineIds.has(line.id));

  return {
    tasks,
    unassignedLines: [...unassignedStreamLines, ...unassignedUILines],
  };
}
