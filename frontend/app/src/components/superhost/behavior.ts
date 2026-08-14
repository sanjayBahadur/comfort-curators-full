import { useEffect, useState } from "react";
import type { TerminalLine } from "./Terminal";
import type { StreamConnectionState, SuperhostStreamEvent } from "../../lib/api/superhost-stream";

type EventData = Record<string, unknown>;

export type MappedSuperhostEvent =
  | { type: "line"; line: TerminalLine }
  | { type: "approval_required"; event: SuperhostStreamEvent; toolName: string; requestId: string; summary: string }
  | { type: "unknown"; event: SuperhostStreamEvent };

function dataOf(event: SuperhostStreamEvent): EventData {
  return event.event_data && typeof event.event_data === "object" ? event.event_data as EventData : {};
}

function value(data: EventData, key: string): string {
  return typeof data[key] === "string" ? data[key] as string : "";
}

export function eventToTerminalLine(event: SuperhostStreamEvent): MappedSuperhostEvent {
  const data = dataOf(event);
  const id = event.event_id;
  switch (event.event_name) {
    case "ToolCallProposed.v1":
      return { type: "line", line: { id, kind: "agent", text: `proposed ${value(data, "tool_name")}: ${JSON.stringify(data.arguments)}` } };
    case "PolicyDenied.v1":
      return { type: "line", line: { id, kind: "denial", text: value(data, "reason") } };
    case "ApprovalRequired.v1":
      return { type: "approval_required", event, toolName: value(data, "tool_name"), requestId: value(data, "approval_request_id"), summary: value(data, "summary") };
    case "PolicyAllowed.v1":
      return { type: "line", line: { id, kind: "agent", text: value(data, "result_summary") } };
    case "AgentRunFallback.v1":
      return { type: "line", line: { id, kind: "system", text: `OFFLINE FALLBACK · ${value(data, "reason")}` } };
    case "AgentRunToken.v1":
      // A live, in-progress view of the model's own narrative text -- see
      // stream.go's streamDeliverText. id is stable ("token-<run_id>", set
      // by the backend) and reused across the whole streaming turn, so
      // each new delta replaces this line in place rather than piling up
      // a fresh one per tick (see superhost-stream.ts's merge-by-id).
      // This is real text already, not a stand-in for it -- rendered
      // as-is below, with no typewriter fakery layered on top.
      return { type: "line", line: { id, kind: "agent", text: value(data, "text") } };
    case "AgentRunCompleted.v1": {
      // output is the model's real, final answer -- see
      // AgentRunStore.CompleteWithUsage (backend). A run that finishes
      // with a plain conversational response and no trailing tool call
      // has no other event carrying it: without this, the terminal
      // showed the literal string "run completed" and nothing else,
      // silently discarding the actual answer.
      const output = value(data, "output");
      return { type: "line", line: { id, kind: "agent", text: output || "run completed" } };
    }
    case "AgentRunFailed.v1":
      return { type: "line", line: { id, kind: "system", text: `run failed: ${value(data, "error_message")}` } };
    case "AgentRunCancelled.v1":
      return { type: "line", line: { id, kind: "system", text: `run cancelled${value(data, "reason") ? `: ${value(data, "reason")}` : ""}` } };
    case "AgentRunQueued.v1":
      return { type: "line", line: { id, kind: "system", text: "run queued" } };
    default:
      return { type: "unknown", event };
  }
}

export function useTypewriter(text: string, lineId: string | null, reducedMotion = false) {
  const [visibleText, setVisibleText] = useState(text);
  const [active, setActive] = useState(false);

  useEffect(() => {
    if (!lineId || reducedMotion) {
      setVisibleText(text);
      setActive(false);
      return;
    }

    setVisibleText("");
    setActive(true);
    const duration = Math.min(text.length * 12, 400);
    const interval = text.length ? Math.max(1, duration / text.length) : 0;
    let index = 0;
    const timer = window.setInterval(() => {
      index += 1;
      setVisibleText(text.slice(0, index));
      if (index >= text.length) {
        window.clearInterval(timer);
        setActive(false);
      }
    }, interval);
    return () => window.clearInterval(timer);
  }, [lineId, reducedMotion, text]);

  return { visibleText, active };
}

export function useTerminalStreamView(events: SuperhostStreamEvent[], state: StreamConnectionState) {
  const [reducedMotion, setReducedMotion] = useState(false);
  useEffect(() => {
    const query = window.matchMedia("(prefers-reduced-motion: reduce)");
    const update = () => setReducedMotion(query.matches);
    update();
    query.addEventListener("change", update);
    return () => query.removeEventListener("change", update);
  }, []);

  // Once a run has reached a real, persisted terminal event, its
  // AgentRunToken.v1 line (see eventToTerminalLine) is a stale snapshot of
  // text the terminal event already carries the final version of --
  // without this, both would render as two lines showing the same
  // paragraph back to back.
  const terminalRunIds = new Set(
    events
      .filter((event) => event.event_name === "AgentRunCompleted.v1" || event.event_name === "AgentRunFailed.v1" || event.event_name === "AgentRunCancelled.v1")
      .map((event) => event.run_id),
  );
  const liveEvents = events.filter((event) => !(event.event_name === "AgentRunToken.v1" && terminalRunIds.has(event.run_id)));

  // A run that streamed real text live (had at least one AgentRunToken.v1)
  // shouldn't have its *terminal* line re-animated with the typewriter once
  // AgentRunCompleted.v1 replaces it -- the human already watched that exact
  // text arrive for real; faking a second reveal over content they've
  // already read is a regression the typewriter's fabrication was never
  // meant to cover.
  const streamedRunIds = new Set(events.filter((event) => event.event_name === "AgentRunToken.v1").map((event) => event.run_id));
  const runIdByEventId = new Map(events.map((event) => [event.event_id, event.run_id]));

  const mapped = liveEvents.map(eventToTerminalLine);
  const lines = mapped.filter((item): item is { type: "line"; line: TerminalLine } => item.type === "line").map((item) => item.line);
  const lastLine = lines.at(-1);
  // isLiveLine: genuinely still streaming right now -- gates the blinking
  // cursor, so it must go false the instant the token line is superseded
  // by the real terminal line (see liveEvents' filter above), not stay on
  // just because this run streamed earlier.
  const isLiveLine = Boolean(lastLine && lastLine.id.startsWith("token-"));
  // wasStreamedLine: this exact line already arrived as real, live text at
  // some point (either it still is the live token line, or it's the
  // terminal line for a run that had one) -- gates only the typewriter
  // bypass. A denial is a refusal, not a narrative line: render it whole
  // so its role="alert" region is announced once instead of once per
  // typewriter tick. Re-animating a terminal line with the typewriter
  // after the human already watched that exact text arrive live would be
  // exactly the fabrication the typewriter's replay was never meant for.
  const wasStreamedLine = Boolean(
    lastLine && (lastLine.id.startsWith("token-") || streamedRunIds.has(runIdByEventId.get(lastLine.id) ?? "")),
  );
  const typewriterSource = lastLine && lastLine.kind !== "denial" && !wasStreamedLine ? lastLine : null;
  const typewriter = useTypewriter(typewriterSource?.text ?? "", typewriterSource?.id ?? null, reducedMotion);
  // typewriter.visibleText only tracks typewriterSource -- when the last
  // line is a denial, typewriterSource is null (skipped on purpose, see
  // above) and typewriter.visibleText is stuck at "", not the denial's
  // real text. Rendering it whole means using lastLine.text directly for
  // that case, not the (disconnected) typewriter output.
  const lastLineText = typewriterSource ? typewriter.visibleText : (lastLine?.text ?? "");
  const visibleLines = lastLine
    ? [...lines.slice(0, -1), { ...lastLine, text: lastLineText }]
    : lines;

  return {
    lines: visibleLines,
    approvals: mapped.filter((item): item is Extract<MappedSuperhostEvent, { type: "approval_required" }> => item.type === "approval_required"),
    cursorVisible: state === "connecting" || state === "open" || isLiveLine || ((state !== "done" && state !== "error") && typewriter.active),
    reducedMotion,
  };
}
