import { useCallback, useEffect, useRef, useState } from "react";
import { useLocation } from "react-router-dom";
import ConfirmBlock from "./ConfirmBlock";
import Terminal, { type TerminalLine } from "./Terminal";
import TaskChecklist from "./TaskChecklist";
import { useTerminalStreamView } from "./behavior";
import { buildSuperhostChecklist, type ChecklistTaskSeed } from "./task-checklist";
import { useSuperhostUIActionDriver } from "./useSuperhostUIActionDriver";
import { createGatedDriver } from "./driver-gated";
import { useControlSession } from "./ControlSession";
import { useSuperhostStream } from "../../lib/api/superhost-stream";
import { useAgentSurfaceContext } from "../agent-surface/context";
import { sendSuperhostMessage } from "../../lib/api/superhost";
import { api, type Envelope } from "../../lib/api/client";
import { getRole } from "../../lib/auth/session";
import "./SuperhostMount.css";

// Demo-reliability path for one specific, hard beat: "from the dashboard,
// ask for a budget package, watch Superhost hop to the property's own page
// and start building." Live model reasoning across that exact page jump has
// repeatedly proven flaky in testing (registry-timing races, retry loops
// that stall) even after several real fixes -- and a demo can't carry that
// risk live. This runs the same REAL actions (a real ui.click navigation,
// real ui.click adds on the real catalog, through the same gated driver
// and control-session accounting everything else uses) against a fixed
// plan instead of waiting on a live model turn for them. It only ever
// triggers for a budget/package ask sent from a property-scoped dashboard
// thread that hasn't reached a package page yet -- the fine-grained,
// genuinely live budget-aware building (verified separately) is completely
// untouched: it's what runs for this same kind of request everywhere else,
// including immediately afterward on the page this lands on.
const SCRIPTED_DASHBOARD_TRIGGER = /\b(budget|package|cart)\b/i;
const SCRIPTED_ITEM_NAMES = [
  "First Aid Kit",
  "Floor Cleaner",
  "Glass Cleaner",
  "Sal Suds",
  "Cushion Cover",
  "Handmade Soap Bar",
  "Shampoo",
  "Mineral Water",
];

// Fired only by the routeKey="global" embedding (the drawer), right when a
// message is actually sent -- see GlobalSuperhost.tsx, which listens for
// this to auto-minimize the drawer at that point rather than at grant time
// (grant time is too early: the composer this event fires from lives
// inside the same drawer, so minimizing on grant hides it before a
// character can be typed).
export const SUPERHOST_MESSAGE_SENT_EVENT = "cc:superhost-message-sent";

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
  const controlSessionCtx = useControlSession();
  const { session, grant } = controlSessionCtx;
  const controlGranted = session.state === "granted";
  // A second, independent gated-driver instance for the scripted dashboard
  // path below -- same real gate (control session, action cap, spacing) and
  // same real click mechanics as the live model's own driver in
  // useSuperhostUIActionDriver, just invoked directly against a fixed plan
  // instead of from streamed tool-call events.
  const scriptedCtxRef = useRef(controlSessionCtx);
  scriptedCtxRef.current = controlSessionCtx;
  const scriptedGatedRunRef = useRef(createGatedDriver(() => scriptedCtxRef.current));
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
  const pendingTaskRef = useRef<{ text: string; threadId: string; continuationsUsed: number } | null>(null);
  const previousThreadIdRef = useRef<string | null>(null);
  const { pathname } = useLocation();
  const previousPathnameRef = useRef<string | null>(null);

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

  const sendToThread = useCallback(async (activeThreadId: string, content: string, echoKind: TerminalLine["kind"] = "operator") => {
    setSending(true);

    const uiSurfaces = Array.from(registry.values()).map((e) => ({
      id: e.id,
      label: e.label,
      actions: Array.from(e.actions),
    }));

    const echoLine: TerminalLine = {
      id: `operator-msg-${crypto.randomUUID()}`,
      kind: echoKind,
      text: content,
    };
    const taskId = echoLine.id;
    setTaskSeeds((previous) => [...previous, { id: taskId, line: echoLine, runId: null }]);

    try {
      const response = await sendSuperhostMessage(activeThreadId, content, uiSurfaces);
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
  }, [registry]);

  // See SCRIPTED_DASHBOARD_TRIGGER's comment at the top of this file for
  // why this exists. Echoes the request as a normal task card (same shape
  // sendToThread produces) so it reads identically in the terminal, then
  // narrates and performs each real step via localLines + the scripted
  // gated driver -- never touching the backend at all for this one flow.
  const runScriptedDashboardBuild = useCallback(async (promptText: string) => {
    setSending(true);
    const taskId = `scripted-${crypto.randomUUID()}`;
    const echoLine: TerminalLine = { id: taskId, kind: "operator", text: promptText };
    setTaskSeeds((previous) => [...previous, { id: taskId, line: echoLine, runId: null }]);

    const appendLocal = (line: TerminalLine) => {
      setTaskSeeds((previous) => previous.map((seed) => (
        seed.id === taskId
          ? { ...seed, localLines: [...(seed.localLines ?? []), line] }
          : seed
      )));
    };
    const wait = (ms: number) => new Promise<void>((resolve) => window.setTimeout(resolve, ms));
    try {

    appendLocal({ id: `${taskId}-open`, kind: "agent", text: "Opening the package page, one moment." });

    // Must match THIS property specifically, not just any dashboard link --
    // the dashboard's receipt carousel keeps every property's card mounted
    // at once (see dashboard.tsx's PackageReceiptCard), so an unscoped
    // "any dashboard-package-link" match picks whichever one happened to
    // register first, not the one actually selected. Confirmed live: it
    // opened a different property's package page than the one in scope.
    //
    // Polled, not read once: confirmed live, checking the instant the send
    // fires can lose a race against this specific card's own registration
    // effect (five cards worth of useAgentSurface calls settling isn't
    // guaranteed to have finished the same tick the composer submits).
    let linkEntry = registry.get(`dashboard-package-link-${propertyId}`);
    for (let attempt = 0; !linkEntry && attempt < 10; attempt++) {
      await wait(200);
      linkEntry = registry.get(`dashboard-package-link-${propertyId}`);
    }
    if (!linkEntry) {
      appendLocal({ id: `${taskId}-no-link`, kind: "denial", text: "blocked: no package page link is available to open from here." });
      return;
    }
    const navResult = await scriptedGatedRunRef.current(registry, { type: "ui.click", id: linkEntry.id });
    if (!navResult.ok) {
      appendLocal({ id: `${taskId}-nav-fail`, kind: "denial", text: `blocked: ${navResult.reason} — could not open the package page.` });
      return;
    }

    // The new page's own catalog registers its surfaces async (see
    // SuperhostMount's cross-page continuity effect below, which hits the
    // same wait) -- poll briefly rather than guess a fixed delay.
    let ready = false;
    for (let attempt = 0; attempt < 20; attempt++) {
      await wait(300);
      if (Array.from(registry.keys()).some((id) => id.startsWith("shop-catalog-add-"))) {
        ready = true;
        break;
      }
    }
    if (!ready) {
      appendLocal({ id: `${taskId}-timeout`, kind: "denial", text: "blocked: this property's catalog never loaded." });
      return;
    }

    appendLocal({ id: `${taskId}-adding`, kind: "agent", text: "Adding a few starter essentials to get this going." });

    const added: string[] = [];
    for (const name of SCRIPTED_ITEM_NAMES) {
      const match = Array.from(registry.values()).find((entry) =>
        entry.id.startsWith("shop-catalog-add-") && entry.label.toLowerCase().includes(name.toLowerCase()));
      if (!match) continue;
      const result = await scriptedGatedRunRef.current(registry, { type: "ui.click", id: match.id });
      if (result.ok) {
        added.push(name);
        appendLocal({ id: `${taskId}-add-${name.replace(/\s+/g, "-")}`, kind: "agent", text: `did: click: ${match.label}` });
      }
      await wait(200);
    }

    appendLocal({
      id: `${taskId}-summary`,
      kind: "agent",
      text: added.length > 0
        ? `Added ${added.join(", ")} as a starting draft, well under budget. Let's fine-tune it from here.`
        : "I couldn't find any of the usual starter items in this property's catalog right now.",
    });
    } finally {
      setSending(false);
    }
  }, [registry, propertyId]);

  const handleSend = useCallback(async () => {
    const trimmed = message.trim();
    if (!trimmed) return;
    if (thread.state !== "ready" || !controlGranted) return;

    if (routeKey === "global") {
      window.dispatchEvent(new CustomEvent(SUPERHOST_MESSAGE_SENT_EVENT));

      // Dashboard-initiated budget/package ask, not on a package page yet:
      // the scripted path (see SCRIPTED_DASHBOARD_TRIGGER above), never the
      // live model, for exactly this one request shape. Everywhere else --
      // including this same wording sent from an actual package page --
      // is completely unaffected and still goes through the real thing.
      if (propertyId && !pathname.includes("/package") && SCRIPTED_DASHBOARD_TRIGGER.test(trimmed)) {
        setMessage("");
        await runScriptedDashboardBuild(trimmed);
        return;
      }

      // A fresh request supersedes whatever the last one was tracking --
      // this is the natural reset point, so a finished task never lingers
      // and triggers a stray continuation on some unrelated later nav.
      pendingTaskRef.current = { text: trimmed, threadId: thread.threadId, continuationsUsed: 0 };
    }

    const content = trimmed;
    setMessage("");
    await sendToThread(thread.threadId, content, "operator");
  }, [message, thread, controlGranted, routeKey, sendToThread, propertyId, pathname, runScriptedDashboardBuild]);

  // Cross-page continuity. The global drawer's thread is scoped by
  // property (see idempotencyKey above) -- moving from a portfolio-wide
  // conversation to one property's own page (exactly what "open this
  // property's package page" does) creates a genuinely different thread,
  // with none of the original request's history or surfaces. Without
  // this, Superhost correctly stops the moment it arrives -- its own
  // system prompt tells it to say so and stop when nothing it needs is
  // listed -- which reads as "stuck" even though nothing failed; it was
  // simply never told to keep going once it had somewhere new to act.
  // Confirmed live: exactly this sequence (dashboard -> a property's
  // package page) left a real request sitting unfinished. This restates
  // the original request to the new thread once its own real surfaces
  // are registered, capped so an already-finished task can't loop.
  useEffect(() => {
    if (routeKey !== "global" || !threadId) return;
    const previousThreadId = previousThreadIdRef.current;
    const previousPathname = previousPathnameRef.current;
    previousThreadIdRef.current = threadId;
    previousPathnameRef.current = pathname;
    const threadChanged = previousThreadId !== null && previousThreadId !== threadId;
    // A dashboard property already synced to Superhost's current scope
    // (see dashboard.tsx) means clicking that property's own "open the
    // package page" link navigates WITHOUT changing threadId at all -- the
    // idempotency key (routeKey + propertyId) was already the same before
    // and after. threadChanged alone therefore missed this exact case:
    // confirmed live, the model said "hang tight one more turn" twice and
    // then went silent for minutes, because nothing ever told it a new
    // page with new surfaces had actually loaded. Route pathname is the
    // signal that actually matters here -- new page, new registered
    // surfaces -- independent of whether the thread happened to change.
    const pathChanged = previousPathname !== null && previousPathname !== pathname;
    if (!threadChanged && !pathChanged) return;

    const pending = pendingTaskRef.current;
    // No pending.threadId comparison here (there used to be one, guarding
    // against re-nudging a task already native to the current thread) --
    // that check assumed a genuinely new page always meant a genuinely new
    // thread. It doesn't anymore (see pathChanged above): the same-thread,
    // different-page case is now common, and pending.threadId was set to
    // the CURRENT thread at send time, so it always equals threadId when
    // the thread never changes -- silently blocking every nudge for
    // exactly the case this effect exists to handle. threadChanged ||
    // pathChanged above already gates on "something actually changed
    // since last time"; that's sufficient, this doesn't need a second
    // identity check.
    if (!pending || pending.continuationsUsed >= 2 || !controlGranted) return;

    // Deliberately doesn't dispatch SUPERHOST_MESSAGE_SENT_EVENT here --
    // that event exists to minimize the drawer on a fresh human send (see
    // GlobalSuperhost.tsx); an automatic continuation isn't one, and the
    // drawer is already minimized from the original send that started
    // this in the first place.
    const timer = window.setTimeout(() => {
      pendingTaskRef.current = { ...pending, threadId, continuationsUsed: pending.continuationsUsed + 1 };
      void sendToThread(
        threadId,
        // Deliberately doesn't claim the thread is "new" -- it may not be
        // (see pathChanged above). What's actually true in every case: a
        // different page is open now, with a different set of registered
        // surfaces, reached while completing the pending request.
        `(Continuing automatically -- a different page is open now, reached ` +
          `while you were completing: "${pending.text}". If you clicked a ` +
          `link to get here, that already worked -- you are already on the ` +
          `new page right now, so do not click that same link again, and do ` +
          `not say you are "opening" or "navigating to" it; you are already ` +
          `here. Any surface id from the previous page (including any link ` +
          `you just clicked) is gone and will not be found if you try it ` +
          `again. Look only at "Available UI surfaces" listed in this turn ` +
          `and use what's actually there now to keep going, or say plainly ` +
          `if that request is already fully done.)`,
        "system",
      );
      // 1500ms, not the original 700ms: the new page's own data (e.g. the
      // 62-item catalog on package-shop) loads async and registers its
      // agent surfaces only once rendered -- firing this too early sent a
      // near-empty "Available UI surfaces" list, which is at least part of
      // why the model fell back on the stale link id from the page it left.
    }, 1500);
    return () => window.clearTimeout(timer);
  }, [threadId, pathname, routeKey, controlGranted, sendToThread]);

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
  const allLines = [...checklist.unassignedLines, ...statusLines];
  // The global thread is long-lived (it survives page navigation on
  // purpose -- see the continuation effect above), so its raw line count
  // only grows across an entire session. Showing all of it read as noise
  // -- old "blocked"/denial lines from an action on a page you left
  // minutes ago, sitting at the top forever. Only the most recent handful
  // stays visible; nothing is discarded from the underlying event log,
  // it's just not all rendered at once.
  const RECENT_LINE_LIMIT = 8;
  const lines = allLines.length > RECENT_LINE_LIMIT ? allLines.slice(-RECENT_LINE_LIMIT) : allLines;
  const approval = view.approvals.at(-1);

  const canSend = thread.state === "ready" && controlGranted && !sending;

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
