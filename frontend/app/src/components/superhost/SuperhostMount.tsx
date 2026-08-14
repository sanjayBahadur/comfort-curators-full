import { useCallback, useEffect, useRef, useState } from "react";
import ConfirmBlock from "./ConfirmBlock";
import Terminal, { type TerminalLine } from "./Terminal";
import TaskChecklist from "./TaskChecklist";
import { useTerminalStreamView } from "./behavior";
import { buildSuperhostChecklist, type ChecklistTaskSeed } from "./task-checklist";
import { useSuperhostUIActionDriver } from "./useSuperhostUIActionDriver";
import { useControlSession } from "./ControlSession";
import { useSuperhostStream } from "../../lib/api/superhost-stream";
import { useAgentSurfaceContext } from "../agent-surface/context";
import { sendSuperhostMessage } from "../../lib/api/superhost";
import { api, type Envelope } from "../../lib/api/client";
import { getRole } from "../../lib/auth/session";
import { superhostScenariosFor } from "../../lib/superhost-demo-scenarios";
import "./SuperhostMount.css";

type ThreadData = {
  thread_id: string;
  run_id: string;
  created_at: string;
};

type SuperhostMountProps = {
  propertyId: string | null | undefined;
  routeKey: string;
  purpose: string;
  emptyMessage: string;
  // When true and propertyId is empty, connect a portfolio-scoped thread
  // (backend: ContextAssembler.AssemblePortfolio) instead of staying
  // idle -- Superhost can see and act on every property this account
  // manages in one conversation. Off by default: most embeddings
  // (stay.tsx, a specific property's own page) have a real reason to
  // want exactly one property in scope and shouldn't silently widen to
  // "everything" just because propertyId happens to be empty at some
  // point in their own loading state.
  allowPortfolio?: boolean;
};

type ThreadStatus =
  | { state: "idle" }
  | { state: "creating" }
  | { state: "ready"; threadId: string }
  | { state: "error"; message: string };

const tenantId = import.meta.env.VITE_DEMO_TENANT_ID as string | undefined;

export default function SuperhostMount({ propertyId, routeKey, purpose, emptyMessage, allowPortfolio = false }: SuperhostMountProps) {
  const { session, grant } = useControlSession();
  const controlGranted = session.state === "granted";
  const [thread, setThread] = useState<ThreadStatus>({ state: "idle" });
  const [streamGeneration, setStreamGeneration] = useState(0);
  const portfolioMode = allowPortfolio && !propertyId;
  const hasScope = Boolean(propertyId) || portfolioMode;
  const threadId = thread.state === "ready" ? thread.threadId : null;
  // Same value CreateThread is asked for below, computed here too so it's
  // available before that request resolves -- see useSuperhostStream's
  // cacheKey: it's what lets last-known content render immediately
  // instead of the terminal going blank on every open/reconnect.
  // Role-scoped: this app's demo auth is one browser session per role, and
  // without a role component here, switching roles in the same browser
  // would show one account's cached conversation to another's, briefly,
  // before the real stream corrected it.
  const idempotencyKey = `superhost-terminal:${routeKey}:${propertyId || "portfolio"}`;
  const cacheKey = `${getRole() ?? "unknown"}:${idempotencyKey}`;
  const stream = useSuperhostStream(threadId, Boolean(threadId), streamGeneration, cacheKey);
  const streamStateRef = useRef(stream.state);
  streamStateRef.current = stream.state;
  const view = useTerminalStreamView(stream.events, stream.state);
  const uiDriverLines = useSuperhostUIActionDriver(stream.events, cacheKey);
  const { registry } = useAgentSurfaceContext();

  const [message, setMessage] = useState("");
  const [sending, setSending] = useState(false);
  const [taskSeeds, setTaskSeeds] = useState<ChecklistTaskSeed[]>([]);

  useEffect(() => {
    setTaskSeeds([]);
    setMessage("");
    if (!propertyId && !portfolioMode) {
      setThread({ state: "idle" });
      return;
    }
    if (!tenantId) {
      setThread({ state: "error", message: "not connected: demo tenant is not configured" });
      return;
    }

    const controller = new AbortController();
    setThread({ state: "creating" });

    void api<Envelope<ThreadData>>("/v1/superhost/threads", {
      method: "POST",
      signal: controller.signal,
      body: JSON.stringify({
        idempotency_key: idempotencyKey,
        tenant_id: tenantId,
        property_id: propertyId || "",
        purpose,
      }),
    }).then((response) => {
      if (!controller.signal.aborted) setThread({ state: "ready", threadId: response.data.thread_id });
    }).catch((error: unknown) => {
      if (!controller.signal.aborted) {
        setThread({ state: "error", message: `thread unavailable: ${error instanceof Error ? error.message : String(error)}` });
      }
    });

    return () => controller.abort();
  }, [propertyId, purpose, routeKey, portfolioMode]);

  const handleSend = useCallback(async () => {
    const trimmed = message.trim();
    if (!trimmed) return;
    if (thread.state !== "ready" || !controlGranted) return;

    const content = trimmed;
    setMessage("");
    setSending(true);

    const uiSurfaces = Array.from(registry.values()).map((e) => ({
      id: e.id,
      label: e.label,
      actions: Array.from(e.actions),
    }));

    const echoLine: TerminalLine = {
      id: `operator-msg-${crypto.randomUUID()}`,
      kind: "operator",
      text: content,
    };
    const taskId = echoLine.id;
    setTaskSeeds((previous) => [...previous, { id: taskId, line: echoLine, runId: null }]);

    try {
      const response = await sendSuperhostMessage(thread.threadId, content, uiSurfaces);
      setTaskSeeds((previous) => previous.map((seed) => (
        seed.id === taskId ? { ...seed, runId: response.resource_id } : seed
      )));
      if (streamStateRef.current !== "open" && streamStateRef.current !== "connecting") {
        setStreamGeneration((current) => current + 1);
      }
    } catch (error) {
      const errorLine: TerminalLine = {
        id: `operator-err-${crypto.randomUUID()}`,
        kind: "denial",
        text: `send failed: ${error instanceof Error ? error.message : String(error)}`,
      };
      setTaskSeeds((previous) => previous.map((seed) => (
        seed.id === taskId
          ? { ...seed, localLines: [...(seed.localLines ?? []), errorLine] }
          : seed
      )));
    } finally {
      setSending(false);
    }
  }, [message, thread, registry, controlGranted]);

  const streamStatusLine = stream.state === "error"
    ? {
        id: "stream-error",
        kind: "denial" as const,
        text: `stream unavailable: ${stream.error?.message ?? "unknown stream error"}`,
      }
    : null;
  const baseLines = view.lines.length > 0
    ? streamStatusLine ? [...view.lines, streamStatusLine] : view.lines
    : [{
    id: "mount-status",
    kind: "system" as const,
    text: thread.state === "creating"
      ? "connecting: creating thread"
      : thread.state === "error"
        ? thread.message
        : stream.state === "error"
          ? `stream unavailable: ${stream.error?.message ?? "unknown stream error"}`
          : stream.state === "done" && thread.state === "ready"
            ? "stream ended: no activity received"
          : hasScope
            ? "connected: waiting for activity"
            : emptyMessage,
  }];
  const checklist = buildSuperhostChecklist(stream.events, taskSeeds, view.lines, uiDriverLines);
  const baseLineIds = new Set(view.lines.map((line) => line.id));
  const statusLines = baseLines.filter((line) => !baseLineIds.has(line.id));
  const lines = [...checklist.unassignedLines, ...statusLines];
  const approval = view.approvals.at(-1);

  const canSend = thread.state === "ready" && controlGranted && !sending;
  const demoScenarios = superhostScenariosFor(getRole(), window.location.pathname).slice(0, 3);

  return (
    <section
      className="superhost-mount"
      aria-labelledby={`${routeKey}-terminal-title`}
      // Interacting with Superhost's own surface (typing a follow-up,
      // clicking CONFIRM/NOT NOW, granting control) is not "the human
      // took back the wheel" -- see ControlSession.tsx's
      // data-control-exempt check. Without this, granting control and
      // then typing what you want Superhost to do revoked the grant on
      // the very first keystroke.
      data-control-exempt="true"
    >
      {/* A decorative "A machine surface for the work in front of you."
          headline used to sit here, four lines tall in the drawer's
          narrow width -- filler text eating the exact space that should
          have been showing real logs/tasks. This is now one compact
          status line; the id/aria-labelledby wiring stays on it so the
          section's accessible name doesn't change. */}
      <header className="superhost-mount-heading">
        <h2 id={`${routeKey}-terminal-title`} className="sr-only">Superhost live thread</h2>
        <span>
          {thread.state !== "ready" ? "NOT CONNECTED" : controlGranted ? `THREAD ${thread.threadId}` : "CONNECTED · CONTROL NOT HANDED OVER"}
        </span>
      </header>
      <Terminal
        lines={lines}
        activity={<TaskChecklist tasks={checklist.tasks} />}
        cursorVisible={thread.state === "creating" || view.cursorVisible}
      >
        {approval && <ConfirmBlock key={approval.requestId} approval={approval} />}
      </Terminal>
      {/* Hand over control first, deliberately, before the composer that
          lets you direct it -- see SuperhostMount.css. Handing over
          control used to only exist in the global drawer's own chrome,
          which meant every page-embedded mount (dashboard, stay,
          property detail, ops tickets...) had no way to grant control at
          all, so ui_* actions could never work there. Moving it in here
          makes every embedding capable, consistently. */}
      {thread.state === "ready" && !controlGranted ? (
        <div className="superhost-mount-grant">
          <button type="button" className="superhost-mount-grant-button" onClick={() => grant()}>
            [ HAND OVER CONTROL ]
          </button>
          <p>Superhost can only act on your screen once you hand over control.</p>
        </div>
      ) : (
        <>
          {demoScenarios.length > 0 && (
            <div className="superhost-mount-scenarios" aria-label="Reliable Superhost demo prompts">
              {demoScenarios.map((scenario) => (
                <button
                  key={scenario.id}
                  type="button"
                  onClick={() => setMessage(scenario.prompt)}
                  disabled={!canSend}
                >
                  {scenario.label}
                </button>
              ))}
            </div>
          )}
          <div className="superhost-mount-composer">
            <input
              type="text"
              className="superhost-mount-composer-input"
              value={message}
              onChange={(e) => setMessage(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") void handleSend();
              }}
              placeholder={thread.state === "ready" ? "send a message to the model..." : "waiting for thread..."}
              disabled={!canSend}
              aria-label="Message for Superhost"
            />
            <button
              type="button"
              className="superhost-mount-composer-send"
              onClick={() => void handleSend()}
              disabled={!canSend}
            >
              SEND
            </button>
          </div>
        </>
      )}
    </section>
  );
}
