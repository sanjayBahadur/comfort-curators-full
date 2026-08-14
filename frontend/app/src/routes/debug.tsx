import { useQuery } from "@tanstack/react-query";
import { useCallback, useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import Money from "../components/money";
import { api, ApiError } from "../lib/api/client";
import { getRole, getToken, signIn, signOut, type Role } from "../lib/auth/session";
import { useAgentSurface, useAgentSurfaceContext } from "../components/agent-surface/context";
import { applyAgentIntent } from "../components/agent-surface/driver";
import type { AgentIntent, AgentAction, IntentResult } from "../components/agent-surface/types";
import { useControlSession } from "../components/superhost/ControlSession";
import { createGatedDriver, type GatedIntentResult } from "../components/superhost/driver-gated";
import { PaymentBoundary, PaymentBoundaryButton } from "../components/superhost/PaymentBoundary";
import CookieSlip from "../components/ui/CookieSlip";
import Modal from "../components/ui/Modal";
import Popover from "../components/ui/Popover";
import Select from "../components/ui/Select";
import Tooltip from "../components/ui/Tooltip";
import Terminal, { type TerminalLine } from "../components/superhost/Terminal";
import ConfirmBlock from "../components/superhost/ConfirmBlock";
import { useTerminalStreamView } from "../components/superhost/behavior";
import type { StreamConnectionState, SuperhostStreamEvent } from "../lib/api/superhost-stream";
import "./debug.css";

type PropertiesResponse = {
  items: Array<{ id: string }>;
  next_cursor: string | null;
};

const darkLink = { color: "var(--paper)", borderColor: "var(--paper)" };

const swatches = [
  { name: "paper", value: "#FAF9F7", className: "token-paper" },
  { name: "paper 02", value: "#F2F0EC", className: "token-paper-2" },
  { name: "paper 03", value: "#E8E5DF", className: "token-paper-3" },
  { name: "ink", value: "#000000", className: "token-ink" },
  { name: "red", value: "#FF0000", className: "token-red" },
  { name: "magenta", value: "#E52BBA", className: "token-magenta" },
  { name: "ok", value: "#1A6B3C", className: "token-ok" },
  { name: "warn", value: "#8A6A00", className: "token-warn" },
] as const;

const terminalDemoLines: TerminalLine[] = [
  { id: "agent", kind: "agent", text: "i found one turnover gap for gomti nagar." },
  { id: "operator", kind: "operator", text: "show the proposed next step." },
  { id: "denial", kind: "denial", text: "policy denied: payment needs human approval." },
  { id: "system", kind: "system", text: "RUN READY · WAITING FOR INPUT" },
];

const behaviorDemoEvents: SuperhostStreamEvent[] = [
  { event_id: "demo-proposal", run_id: "demo-run", event_name: "ToolCallProposed.v1", occurred_at: "2026-08-09T10:00:00Z", event_data: { tool_name: "create_maintenance_task", arguments: { room: "204", priority: "high" } } },
  { event_id: "demo-approval", run_id: "demo-run", event_name: "ApprovalRequired.v1", occurred_at: "2026-08-09T10:00:01Z", event_data: { tool_name: "create_maintenance_task", approval_request_id: "approval-demo", summary: "Create a high-priority task for room 204" } },
  { event_id: "demo-denial", run_id: "demo-run", event_name: "PolicyDenied.v1", occurred_at: "2026-08-09T10:00:02Z", event_data: { tool_name: "pay_vendor_invoice", reason: "Payment requires an owner approval." } },
  { event_id: "demo-fallback", run_id: "demo-run", event_name: "AgentRunFallback.v1", occurred_at: "2026-08-09T10:00:03Z", event_data: { is_fallback: true, fallback_marker: "OFFLINE FALLBACK", intent: "inspect property", reason: "provider unavailable", usage_known: false } },
  { event_id: "demo-complete", run_id: "demo-run", event_name: "AgentRunCompleted.v1", occurred_at: "2026-08-09T10:00:04Z", event_data: { usage_known: false } },
];

function SuperhostBehaviorDemo() {
  const [events, setEvents] = useState<SuperhostStreamEvent[]>([]);
  const [streamState, setStreamState] = useState<StreamConnectionState>("connecting");
  const view = useTerminalStreamView(events, streamState);

  useEffect(() => {
    setEvents([]);
    setStreamState("connecting");
    let index = 0;
    const timer = window.setInterval(() => {
      const event = behaviorDemoEvents[index];
      if (!event) {
        window.clearInterval(timer);
        setStreamState("done");
        return;
      }
      setEvents((current) => [...current, event]);
      setStreamState("open");
      index += 1;
    }, 700);
    return () => window.clearInterval(timer);
  }, []);

  const approval = view.approvals.at(-1);
  return (
    <section className="ruled-section terminal-demo-section" aria-labelledby="behavior-terminal-title">
      <div className="section-heading compact-heading">
        <p className="section-index">11A / STREAM BEHAVIOR</p>
        <h2 id="behavior-terminal-title">Events arrive. The surface keeps time.</h2>
      </div>
      <p className="specimen-label">MOCK SSE · {streamState.toUpperCase()} · REDUCED MOTION: {view.reducedMotion ? "ON" : "OFF"}</p>
      <Terminal lines={view.lines} cursorVisible={view.cursorVisible}>
        {approval && (
          <ConfirmBlock key={approval.requestId} approval={approval} />
        )}
      </Terminal>
    </section>
  );
}

function AgentSurfaceDemo() {
  const { registry } = useAgentSurfaceContext();
  const [lastResult, setLastResult] = useState<IntentResult | null>(null);
  const [inputValue, setInputValue] = useState("controlled initial value");
  const [clickCount, setClickCount] = useState(0);
  const [selectValue, setSelectValue] = useState("paper");
  const [modalOpen, setModalOpen] = useState(false);

  const darkLink = { color: "var(--paper)", borderColor: "var(--paper)" };

  const selectContainerRef = useRef<HTMLDivElement>(null);

  const demoInput = useAgentSurface("demo-input", ["focus", "set"] as AgentAction[], "Controlled demo input");
  const demoButton = useAgentSurface("demo-button", ["focus", "click"] as AgentAction[], "Demo button");
  const demoSelect = useAgentSurface("demo-select", ["focus", "click", "scroll_to", "open_panel"] as AgentAction[], "Demo select", () => {
    const trigger = selectContainerRef.current?.querySelector(".select-trigger") as HTMLButtonElement | null;
    trigger?.click();
  });
  const demoModalButton = useAgentSurface("demo-modal-button", ["focus", "click", "open_panel"] as AgentAction[], "Demo modal trigger", () => {
    setModalOpen(true);
  });
  const demoDisabled = useAgentSurface("demo-disabled", ["focus", "click"] as AgentAction[], "Disabled demo button");

  const run = useCallback((intent: AgentIntent) => {
    const result = applyAgentIntent(registry, intent);
    setLastResult(result);
  }, [registry]);

  function formatResult(r: IntentResult | null) {
    if (!r) return "NO INTENT TRIGGERED YET";
    if (r.ok) return `OK · ${r.intent.type} "${r.intent.id}"`;
    return `FAIL · ${r.reason} · ${r.detail}`;
  }

  return (
    <section className="ruled-section controls-section" aria-labelledby="agent-surface-title">
      <div className="section-heading compact-heading">
        <p className="section-index">13 / AGENT SURFACE</p>
        <h2 id="agent-surface-title">Five intents. Nothing else is implementable. (D6, §3.9.1)</h2>
      </div>

      <div className="control-grid">
        <div className="control-panel light-field" data-cursor-grow>
          <p className="specimen-label">Registered elements</p>

          <div style={{ display: "flex", flexDirection: "column", gap: "8px", marginTop: "8px" }}>
            <label style={{ fontSize: "12px", fontFamily: "var(--mono)", color: "var(--ink)" }}>
              CONTROLLED INPUT · value:{" "}
              <span style={{ color: "var(--magenta)" }}>{inputValue}</span>
            </label>
            <input
              ref={demoInput.ref}
              type="text"
              value={inputValue}
              onChange={(e) => setInputValue(e.target.value)}
              style={{
                border: "1px solid var(--paper-3)",
                padding: "4px 8px",
                fontFamily: "var(--mono)",
                fontSize: "12px",
                background: "var(--paper)",
              }}
            />

            <button
              ref={demoButton.ref}
              className="button button-solid"
              type="button"
              onClick={() => setClickCount((c) => c + 1)}
            >
              CLICK COUNT: {clickCount}
            </button>

            <div ref={(node) => { selectContainerRef.current = node; demoSelect.ref(node); }}>
              <Select
                aria-label="Agent surface demo select"
                value={selectValue}
                onChange={setSelectValue}
                options={[
                  { value: "paper", label: "PAPER" },
                  { value: "ink", label: "INK" },
                  { value: "red", label: "RED" },
                ]}
              />
            </div>

            <button
              ref={demoModalButton.ref}
              className="button button-outline"
              type="button"
              onClick={() => setModalOpen(true)}
            >
              OPEN MODAL (DISCLOSURE)
            </button>

            <button
              ref={demoDisabled.ref}
              className="button button-outline"
              type="button"
              disabled
            >
              DISABLED BUTTON
            </button>
          </div>
        </div>

        <div className="control-panel dark-field" data-lenis-prevent style={{ overflowY: "auto", maxHeight: "400px" }}>
          <p className="specimen-label specimen-label-inverse">Intent triggers</p>

          <div style={{ display: "flex", flexDirection: "column", gap: "4px", marginTop: "8px" }}>
            <p style={{ fontSize: "11px", fontFamily: "var(--mono)", color: "var(--paper-3)", marginBottom: "4px" }}>
              VALID INTENTS
            </p>
            <button className="button button-outline" style={darkLink} type="button" onClick={() => run({ type: "ui.focus", id: "demo-input" })}>
              FOCUS demo-input
            </button>
            <button className="button button-outline" style={darkLink} type="button" onClick={() => run({ type: "ui.set_value", id: "demo-input", value: "superhost wrote this" })}>
              SET_VALUE demo-input
            </button>
            <button className="button button-outline" style={darkLink} type="button" onClick={() => run({ type: "ui.click", id: "demo-button" })}>
              CLICK demo-button
            </button>
            <button className="button button-outline" style={darkLink} type="button" onClick={() => run({ type: "ui.scroll_to", id: "demo-select" })}>
              SCROLL_TO demo-select
            </button>
            <button className="button button-outline" style={darkLink} type="button" onClick={() => run({ type: "ui.open_panel", id: "demo-select" })}>
              OPEN_PANEL demo-select
            </button>
            <button className="button button-outline" style={darkLink} type="button" onClick={() => run({ type: "ui.open_panel", id: "demo-modal-button" })}>
              OPEN_PANEL demo-modal-button
            </button>

            <p style={{ fontSize: "11px", fontFamily: "var(--mono)", color: "var(--paper-3)", marginBottom: "4px", marginTop: "12px" }}>
              FAILURE CASES
            </p>
            <button className="button button-outline" style={{ color: "var(--red)", borderColor: "var(--red)" }} type="button" onClick={() => run({ type: "ui.focus", id: "unregistered-id" })}>
              UNREGISTERED ID
            </button>
            <button className="button button-outline" style={{ color: "var(--red)", borderColor: "var(--red)" }} type="button" onClick={() => run({ type: "ui.open_panel", id: "demo-input" })}>
              DISALLOWED ACTION (open_panel on input)
            </button>
            <button className="button button-outline" style={{ color: "var(--red)", borderColor: "var(--red)" }} type="button" onClick={() => run({ type: "ui.click", id: "demo-disabled" })}>
              NOT INTERACTABLE (disabled button)
            </button>
          </div>

          <div style={{
            marginTop: "12px",
            padding: "8px",
            border: lastResult?.ok ? "1px solid var(--ok)" : lastResult ? "1px solid var(--red)" : "1px solid var(--paper-3)",
            fontFamily: "var(--mono)",
            fontSize: "11px",
            color: lastResult?.ok ? "var(--ok)" : lastResult ? "var(--red)" : "var(--paper-3)",
            wordBreak: "break-all",
          }}>
            <p style={{ fontSize: "10px", color: "var(--paper-3)", marginBottom: "4px" }}>LAST RESULT</p>
            <pre style={{ whiteSpace: "pre-wrap", margin: 0, fontFamily: "inherit", fontSize: "inherit" }}>
              {formatResult(lastResult)}
            </pre>
          </div>
        </div>
      </div>

      <Modal open={modalOpen} onClose={() => setModalOpen(false)} title="Agent-opened modal" label="AGENT SURFACE DEMO">
        <p>This modal was opened by the agent via ui.open_panel.</p>
      </Modal>
    </section>
  );
}

function ControlSessionDemo() {
  const { registry } = useAgentSurfaceContext();
  const ctx = useControlSession();
  const [lastResult, setLastResult] = useState<GatedIntentResult | null>(null);
  const [inputValue, setInputValue] = useState("gated initial value");
  const [clickCount, setClickCount] = useState(0);

  const ctxRef = useRef(ctx);
  ctxRef.current = ctx;

  const gatedRunRef = useRef(createGatedDriver(() => ctxRef.current));

  const darkLink = { color: "var(--paper)", borderColor: "var(--paper)" };

  const csInput = useAgentSurface("cs-input", ["focus", "set"] as AgentAction[], "Gated control session input");
  const csButton = useAgentSurface("cs-button", ["focus", "click"] as AgentAction[], "Gated control session button");

  const gatedRun = useCallback(async (intent: AgentIntent) => {
    const result = await gatedRunRef.current(registry, intent);
    setLastResult(result);
  }, [registry]);

  function formatResult(r: GatedIntentResult | null) {
    if (!r) return "NO INTENT TRIGGERED YET";
    if (r.ok) return `OK · ${r.intent.type} "${r.intent.id}"`;
    return `FAIL · ${r.reason} · ${r.detail}`;
  }

  return (
    <section className="ruled-section controls-section" aria-labelledby="control-session-title">
      <div className="section-heading compact-heading">
        <p className="section-index">14 / CONTROL SESSION</p>
        <h2 id="control-session-title">Grant, frame, revoke + budgets. (§3.9.2)</h2>
      </div>

      <div className="control-grid">
        <div className="control-panel light-field" data-cursor-grow>
          <p className="specimen-label">Session status</p>

          <div style={{ display: "flex", flexDirection: "column", gap: "8px", marginTop: "8px" }}>
            <dl style={{ margin: 0, fontFamily: "var(--mono)", fontSize: "11px", lineHeight: 1.6 }}>
              <div style={{ display: "flex", gap: "24px" }}>
                <dt style={{ color: "var(--ink-60)" }}>STATE</dt>
                <dd style={{ color: ctx.session.state === "granted" ? "var(--ok)" : "var(--ink-40)", fontWeight: 700 }}>
                  {ctx.session.state.toUpperCase()}
                </dd>
              </div>
              <div style={{ display: "flex", gap: "24px" }}>
                <dt style={{ color: "var(--ink-60)" }}>ACTIONS</dt>
                <dd>{String(ctx.session.actionCount).padStart(2, "0")}/{ctx.session.maxActions}</dd>
              </div>
              <div style={{ display: "flex", gap: "24px" }}>
                <dt style={{ color: "var(--ink-60)" }}>REMAINING</dt>
                <dd>{ctx.timeDisplay}</dd>
              </div>
              <div style={{ display: "flex", gap: "24px" }}>
                <dt style={{ color: "var(--ink-60)" }}>CAN ACT</dt>
                <dd style={{ color: ctx.canAct === true ? "var(--ok)" : "var(--red)" }}>
                  {ctx.canAct === true ? "YES" : ctx.canAct.toUpperCase()}
                </dd>
              </div>
              {ctx.revokeReason && (
                <div style={{ display: "flex", gap: "24px" }}>
                  <dt style={{ color: "var(--ink-60)" }}>REVOKED</dt>
                  <dd style={{ color: "var(--red)" }}>{ctx.revokeReason.toUpperCase()}</dd>
                </div>
              )}
            </dl>

            <div className="button-row">
              <button
                className="button button-solid"
                type="button"
                onClick={() => ctx.grant()}
                disabled={ctx.session.state === "granted"}
              >
                GRANT CONTROL
              </button>
              <button
                className="button button-outline"
                type="button"
                onClick={() => ctx.revoke("manual")}
                disabled={ctx.session.state !== "granted"}
              >
                REVOKE (MANUAL)
              </button>
            </div>

            <label style={{ fontSize: "12px", fontFamily: "var(--mono)", color: "var(--ink)", marginTop: "8px" }}>
              GATED INPUT · value:{" "}
              <span style={{ color: "var(--magenta)" }}>{inputValue}</span>
            </label>
            <input
              ref={csInput.ref}
              type="text"
              value={inputValue}
              onChange={(e) => setInputValue(e.target.value)}
              style={{
                border: "1px solid var(--paper-3)",
                padding: "4px 8px",
                fontFamily: "var(--mono)",
                fontSize: "12px",
                background: "var(--paper)",
              }}
            />

            <button
              ref={csButton.ref}
              className="button button-solid"
              type="button"
              onClick={() => setClickCount((c) => c + 1)}
            >
              GATED CLICK COUNT: {clickCount}
            </button>
          </div>
        </div>

        <div className="control-panel dark-field" data-lenis-prevent style={{ overflowY: "auto", maxHeight: "400px" }}>
          <p className="specimen-label specimen-label-inverse">Gated intent triggers</p>

          <div style={{ display: "flex", flexDirection: "column", gap: "4px", marginTop: "8px" }}>
            <p style={{ fontSize: "11px", fontFamily: "var(--mono)", color: "var(--paper-3)", marginBottom: "4px" }}>
              GATED THROUGH CONTROL SESSION + RING
            </p>
            <button data-agent-trigger className="button button-outline" style={darkLink} type="button" onClick={() => { void gatedRun({ type: "ui.focus", id: "cs-input" }); }}>
              GATED FOCUS cs-input
            </button>
            <button data-agent-trigger className="button button-outline" style={darkLink} type="button" onClick={() => { void gatedRun({ type: "ui.set_value", id: "cs-input", value: "superhost gated write" }); }}>
              GATED SET_VALUE cs-input
            </button>
            <button data-agent-trigger className="button button-outline" style={darkLink} type="button" onClick={() => { void gatedRun({ type: "ui.click", id: "cs-button" }); }}>
              GATED CLICK cs-button
            </button>

            <p style={{ fontSize: "11px", fontFamily: "var(--mono)", color: "var(--paper-3)", marginBottom: "4px", marginTop: "12px" }}>
              GATED FAILURE CASES
            </p>
            <button data-agent-trigger className="button button-outline" style={{ color: "var(--red)", borderColor: "var(--red)" }} type="button" onClick={() => { void gatedRun({ type: "ui.focus", id: "unregistered-id" }); }}>
              GATED UNREGISTERED ID
            </button>
            <button data-agent-trigger className="button button-outline" style={{ color: "var(--red)", borderColor: "var(--red)" }} type="button" onClick={() => { void gatedRun({ type: "ui.click", id: "cs-input" }); }}>
              GATED DISALLOWED (click on input)
            </button>

            <p style={{ fontSize: "11px", fontFamily: "var(--paper-3)", color: "var(--paper-3)", marginBottom: "4px", marginTop: "12px" }}>
              BUDGET ENFORCEMENT
            </p>
            <p style={{ fontSize: "10px", fontFamily: "var(--mono)", color: "var(--paper-3)", margin: "0 0 4px 0" }}>
              Grant control, then try actions without grant:
            </p>
            <button data-agent-trigger className="button button-outline" style={darkLink} type="button" onClick={() => { void gatedRun({ type: "ui.focus", id: "cs-input" }); }}>
              TRY WITHOUT GRANT →
            </button>
          </div>

          <div style={{
            marginTop: "12px",
            padding: "8px",
            border: lastResult?.ok ? "1px solid var(--ok)" : lastResult ? "1px solid var(--red)" : "1px solid var(--paper-3)",
            fontFamily: "var(--mono)",
            fontSize: "11px",
            color: lastResult?.ok ? "var(--ok)" : lastResult ? "var(--red)" : "var(--paper-3)",
            wordBreak: "break-all",
          }}>
            <p style={{ fontSize: "10px", color: "var(--paper-3)", marginBottom: "4px" }}>GATED RESULT</p>
            <pre style={{ whiteSpace: "pre-wrap", margin: 0, fontFamily: "inherit", fontSize: "inherit" }}>
              {formatResult(lastResult)}
            </pre>
          </div>
        </div>
      </div>
    </section>
  );
}

// Gate 2's guard reads PaymentBoundaryContext via useContext, which resolves
// by where a component is *rendered*, not by where a ref ends up pointing.
// Calling useAgentSurface in a component that sits above <PaymentBoundary>
// and passing the ref down as a prop does NOT pick up isInside: true --
// these two must call the hook themselves, as actual descendants of
// <PaymentBoundary>, exactly like PaymentBoundary.gate2.test.tsx's
// TestChild does. (An earlier version of this demo got this wrong and
// reported a false "Gate 2 is broken" -- the real gate was never broken,
// only this page's own exercise of it.)
function PaymentBoundaryTestButton() {
  const { ref } = useAgentSurface("pb-button", ["click"] as AgentAction[], "Button inside PaymentBoundary — should NOT register");
  return (
    <button ref={ref} className="button button-outline" type="button" style={{ color: "var(--ink-40)", borderColor: "var(--ink-40)" }}>
      CONFIRM ORDER (BLOCKED)
    </button>
  );
}

function PaymentBoundaryTestInput() {
  const { ref } = useAgentSurface("pb-input", ["focus", "set"] as AgentAction[], "Input inside PaymentBoundary — should NOT register");
  return (
    <input ref={ref} type="text" defaultValue="payment amount" readOnly style={{ border: "1px solid var(--paper-3)", padding: "4px 8px", fontFamily: "var(--mono)", fontSize: "12px", background: "var(--paper)" }} />
  );
}

function PaymentBoundaryDemo() {
  const { registry } = useAgentSurfaceContext();
  const ctx = useControlSession();
  const [showBoundary, setShowBoundary] = useState(false);
  const [gate2Result, setGate2Result] = useState<"pending" | "pass" | "fail">("pending");
  const [gate2Detail, setGate2Detail] = useState("");
  const [gate3Result, setGate3Result] = useState<GatedIntentResult | null>(null);
  const checkedRef = useRef(false);

  const darkLink = { color: "var(--paper)", borderColor: "var(--paper)" };

  // Registered so the "agent drives into the boundary" simulation below can
  // target it via the real gated driver. A genuine human click still works
  // normally (onClick fires either way) -- registering it doesn't change
  // Gate 2 behavior, since this button itself isn't inside the boundary.
  const mountTriggerSurface = useAgentSurface("pb-mount-trigger", ["click"] as AgentAction[], "Enter the payment-adjacent section");

  const ctxRef = useRef(ctx);
  ctxRef.current = ctx;
  const gatedRunRef = useRef(createGatedDriver(() => ctxRef.current));

  function simulateAgentEnteringBoundary() {
    void gatedRunRef.current(registry, { type: "ui.click", id: "pb-mount-trigger" }).then((result) => {
      setGate3Result(result);
      if (result.ok) window.setTimeout(() => document.querySelector<HTMLButtonElement>("[data-debug-protected-action]")?.click(), 80);
    });
  }

  useEffect(() => {
    if (!showBoundary || checkedRef.current) return;
    const timer = window.setTimeout(() => {
      const hasButton = registry.has("pb-button");
      const hasInput = registry.has("pb-input");
      if (!hasButton && !hasInput) {
        setGate2Result("pass");
        setGate2Detail("Registry has 0 entries for PaymentBoundary children. Elements inside the boundary are structurally unreachable.");
      } else {
        setGate2Result("fail");
        const entries: string[] = [];
        if (hasButton) entries.push("pb-button");
        if (hasInput) entries.push("pb-input");
        setGate2Detail(`FAIL: registry contains ${entries.join(", ")} — Gate 2 is broken.`);
      }
      checkedRef.current = true;
    }, 50);
    return () => window.clearTimeout(timer);
  }, [showBoundary, registry]);

  function handleClose() {
    setShowBoundary(false);
    checkedRef.current = false;
    setGate2Result("pending");
    setGate2Detail("");
  }

  return (
    <section className="ruled-section controls-section" aria-labelledby="payment-boundary-title">
      <div className="section-heading compact-heading">
        <p className="section-index">15 / PAYMENT BOUNDARY (§3.9.3)</p>
        <h2 id="payment-boundary-title">Three independent gates. All three must hold.</h2>
      </div>

      <div className="control-grid">
        <div className="control-panel light-field" data-cursor-grow>
          <p className="specimen-label">Gate 2 — Structural unreachability</p>
          <p style={{ fontSize: "12px", margin: "4px 0 8px 0", color: "var(--ink-60)" }}>
            A PaymentBoundary-wrapped subtree strips data-agent attributes and never registers. Gate 2 isolates payment UI from the model.
          </p>
          <div className="button-row">
            <button ref={mountTriggerSurface.ref} className="button button-solid" type="button" onClick={() => { checkedRef.current = false; setGate2Result("pending"); setShowBoundary(true); }}>
              {showBoundary ? "BOUNDARY ACTIVE" : "MOUNT PAYMENT BOUNDARY"}
            </button>
            <button className="button button-outline" type="button" onClick={handleClose} disabled={!showBoundary}>
              DISMISS BOUNDARY
            </button>
          </div>
          <p style={{ fontSize: "10px", fontFamily: "var(--mono)", color: "var(--ink-40)", marginTop: "4px" }}>
            A direct human click here always revokes any live session first
            (that's item 2 of the gate, working correctly) — it will never
            show you Gate 3 firing. Use the simulation on the right instead.
          </p>
          <div style={{
            marginTop: "12px",
            padding: "8px",
            border: gate2Result === "pass" ? "1px solid var(--ok)" : gate2Result === "fail" ? "1px solid var(--red)" : "1px solid var(--paper-3)",
            fontFamily: "var(--mono)",
            fontSize: "11px",
            color: gate2Result === "pass" ? "var(--ok)" : gate2Result === "fail" ? "var(--red)" : "var(--paper)",
          }}>
            <pre style={{ whiteSpace: "pre-wrap", margin: 0, fontFamily: "inherit", fontSize: "inherit" }}>
              {gate2Result === "pending" ? "RUNNING..." : gate2Detail || "Awaiting mount."}
            </pre>
          </div>

          {showBoundary && (
            <div style={{ marginTop: "12px", padding: "8px", border: "2px solid var(--red)", fontSize: "12px", fontFamily: "var(--mono)" }}>
              <PaymentBoundary>
                <div style={{ padding: "12px", fontFamily: "var(--mono)", fontSize: "11px", color: "var(--red)" }}>
                  <p>PAYMENT BOUNDARY ACTIVE</p>
                  <p style={{ color: "var(--ink-60)" }}>Elements inside this boundary should NOT appear in the AgentSurface registry.</p>
                  <div className="button-row" style={{ marginTop: "8px" }}>
                    <PaymentBoundaryTestButton />
                    <PaymentBoundaryButton data-debug-protected-action className="button button-outline" type="button" style={{ color: "var(--red)", borderColor: "var(--red)" }}>
                      ATTEMPT PROTECTED ACTION
                    </PaymentBoundaryButton>
                  </div>
                  <div style={{ marginTop: "8px" }}>
                    <PaymentBoundaryTestInput />
                  </div>
                </div>
              </PaymentBoundary>
            </div>
          )}
        </div>

        <div className="control-panel dark-field" data-lenis-prevent style={{ overflowY: "auto", maxHeight: "500px" }}>
          <p className="specimen-label specimen-label-inverse">Gate 3 — Session invalidation</p>
          <p style={{ fontSize: "11px", fontFamily: "var(--mono)", color: "var(--paper-3)", margin: "4px 0 8px 0" }}>
            Grant control, then click SIMULATE AGENT ENTERING PAYMENT
            BOUNDARY below. This drives the MOUNT PAYMENT BOUNDARY button
            through the real gated driver — the same synthetic path a real
            model-driven action would use — so the click itself doesn't
            trip the "any human interaction revokes" rule the way clicking
            it directly would. PaymentBoundary then fires Gate 3 for real,
            with a session that was genuinely still live. Watch for the red
            page frame and the terminal notice it renders (fixed, bottom of
            viewport) — the exact mechanism stay.tsx and package-shop.tsx use.
          </p>

          <div style={{ display: "flex", flexDirection: "column", gap: "4px", marginBottom: "12px" }}>
            <dl style={{ margin: 0, fontFamily: "var(--mono)", fontSize: "11px", lineHeight: 1.6 }}>
              <div style={{ display: "flex", gap: "24px" }}>
                <dt style={{ color: "var(--paper-3)" }}>SESSION</dt>
                <dd style={{ color: ctx.session.state === "granted" ? "var(--ok)" : "var(--red)", fontWeight: 700 }}>
                  {ctx.session.state.toUpperCase()}
                </dd>
              </div>
              <div style={{ display: "flex", gap: "24px" }}>
                <dt style={{ color: "var(--paper-3)" }}>REGISTRY</dt>
                <dd>{registry.size} entries</dd>
              </div>
              {ctx.revokeReason && (
                <div style={{ display: "flex", gap: "24px" }}>
                  <dt style={{ color: "var(--paper-3)" }}>REVOKED</dt>
                  <dd style={{ color: "var(--red)" }}>{ctx.revokeReason.toUpperCase()}</dd>
                </div>
              )}
            </dl>
          </div>

          <div className="button-row">
            <button className="button button-outline" style={darkLink} type="button" onClick={() => ctx.grant()} disabled={ctx.session.state === "granted"}>
              GRANT CONTROL
            </button>
            <button data-agent-trigger className="button button-outline" style={{ color: "var(--red)", borderColor: "var(--red)" }} type="button" onClick={simulateAgentEnteringBoundary} disabled={ctx.session.state !== "granted"}>
              SIMULATE AGENT ENTERING PAYMENT BOUNDARY
            </button>
          </div>

          {gate3Result && (
            <div style={{
              marginTop: "12px",
              padding: "8px",
              border: gate3Result.ok ? "1px solid var(--ok)" : "1px solid var(--red)",
              fontFamily: "var(--mono)",
              fontSize: "11px",
              color: gate3Result.ok ? "var(--ok)" : "var(--red)",
            }}>
              <pre style={{ whiteSpace: "pre-wrap", margin: 0, fontFamily: "inherit", fontSize: "inherit" }}>
                {gate3Result.ok
                  ? `OK · ${gate3Result.intent.type} "${gate3Result.intent.id}"`
                  : `${gate3Result.reason} · ${gate3Result.detail}`}
              </pre>
            </div>
          )}

          <p style={{ fontSize: "10px", fontFamily: "var(--mono)", color: "var(--paper-3)", marginTop: "12px" }}>
            {ctx.session.state === "granted"
              ? "Session live — the simulation above will invalidate it for real."
              : "No live session — grant control first."}
          </p>
        </div>
      </div>
    </section>
  );
}

export default function Debug() {
  const [copied, setCopied] = useState(false);
  const [selectedPrimitive, setSelectedPrimitive] = useState("paper");
  const [modalOpen, setModalOpen] = useState(false);
  const [popoverOpen, setPopoverOpen] = useState(false);
  const [cookieReset, setCookieReset] = useState(0);
  const properties = useQuery({
    queryKey: ["debug", "properties"],
    retry: false,
    queryFn: async () => {
      if (!getToken()) await signIn("owner", "owner@demo.test");
      return api<unknown>("/v1/properties");
    },
  });

  const [activeRole, setActiveRole] = useState<Role | null>(() => getRole());
  const [sessionNote, setSessionNote] = useState(() =>
    getToken() ? "Session restored from this tab" : "No active demo session",
  );
  const [propertyCount, setPropertyCount] = useState<number | null>(null);
  const [shopPropertyId, setShopPropertyId] = useState<string | null>(null);
  const [forcedError, setForcedError] = useState<string | null>(null);

  useEffect(() => {
    if (properties.isSuccess) void refreshSession("Session restored from this tab");
  }, [properties.isSuccess]);

  async function refreshSession(note: string) {
    setActiveRole(getRole());
    setSessionNote(note);
    setPropertyCount(null);
    setShopPropertyId(null);
    if (!getToken()) return;

    try {
      const payload = await api<PropertiesResponse>("/v1/properties");
      setPropertyCount(payload.items.length);
      setShopPropertyId(payload.items[0]?.id ?? null);
    } catch {
      // The shared API client already surfaced the backend message.
    }
  }

  async function mintSession() {
    try {
      await signIn("owner", "owner@demo.test");
      await refreshSession("Session token minted");
    } catch {
      // The shared API client already surfaced the backend message.
    }
  }

  function clearSession() {
    signOut();
    setActiveRole(null);
    setSessionNote("No active demo session");
    setPropertyCount(null);
    setShopPropertyId(null);
  }

  async function forceApiError() {
    if (!getToken()) {
      try {
        await signIn("owner", "owner@demo.test");
      } catch {
        // The shared API client already surfaced the backend message.
        return;
      }
    }

    try {
      await api<unknown>("/v1/properties/not-a-real-property");
      setForcedError("Unexpected: the request succeeded.");
    } catch (error) {
      setForcedError(
        error instanceof ApiError
          ? `${error.status} ${error.code}\n${error.message}\nrequest_id: ${error.requestId}`
          : String(error),
      );
    }
  }

  return (
    <main className="style-sheet">
      <header className="sheet-hero ruled-section registration-frame">
        <div className="meta-row">
          <span>CC / SYSTEM 00.5</span>
          <span>LUCKNOW · INDIA</span>
          <span>BUILD 2026.08</span>
        </div>
        <div className="hero-copy">
          <p className="section-index">01 / FOUNDATION</p>
          <h1>
            Built to feel
            <br />
            <em>handled.</em>
          </h1>
          <p className="hero-deck">
            An immaculate operating grid with just enough ink on its hands.
          </p>
        </div>
        <div className="hero-note" aria-label="Design principle">
          The grid is immaculate.
          <br />
          The content violates it on purpose.
        </div>
      </header>

      <section className="ruled-section type-section" aria-labelledby="type-title">
        <div className="section-heading">
          <p className="section-index">02 / TYPE SYSTEM</p>
          <h2 id="type-title">Four voices. One register.</h2>
        </div>
        <div className="type-grid">
          <article className="type-specimen type-display">
            <p className="specimen-label">Instrument Serif · Display 96</p>
            <p className="display-xxl">Quietly<br /><em>in control.</em></p>
          </article>
          <article className="type-specimen type-editorial">
            <p className="specimen-label">Instrument Serif · Editorial 48</p>
            <p className="display-md">A property should feel ready before anyone asks.</p>
          </article>
          <article className="type-specimen type-ui">
            <p className="specimen-label">Archivo Variable · Interface 16</p>
            <p className="ui-copy">
              Owners see the exceptions. Curators see the work. The system keeps
              the routine noise to itself.
            </p>
          </article>
          <article className="type-specimen type-shout">
            <p className="specimen-label specimen-label-inverse">Archivo Black · Punk hit 40</p>
            <p className="shout-copy">READY<br />MEANS READY</p>
          </article>
          <article className="type-specimen type-mono">
            <p className="specimen-label">JetBrains Mono · Metadata 12</p>
            <p className="mono-copy">PROP_GOMTI_02 · INR 54,000.00 · ACTIVE · 08:45 IST</p>
          </article>
        </div>
      </section>

      <section className="ruled-section" aria-labelledby="tokens-title">
        <div className="section-heading compact-heading">
          <p className="section-index">03 / MATERIALS</p>
          <h2 id="tokens-title">Paper, ink, one scream.</h2>
        </div>
        <div className="token-grid">
          {swatches.map((swatch) => (
            <article className={`token-card ${swatch.className}`} key={swatch.name}>
              <span>{swatch.name}</span>
              <strong>{swatch.value}</strong>
            </article>
          ))}
        </div>
      </section>

      <section className="ruled-section grid-study" aria-labelledby="grid-title">
        <div className="section-heading compact-heading">
          <p className="section-index">04 / GRID</p>
          <h2 id="grid-title">Twelve columns. No alibis.</h2>
        </div>
        <div className="twelve-column-grid" aria-label="Twelve-column layout grid">
          {Array.from({ length: 12 }, (_, index) => (
            <span key={index}>{String(index + 1).padStart(2, "0")}</span>
          ))}
        </div>
      </section>

      <section className="ruled-section controls-section" aria-labelledby="controls-title">
        <div className="section-heading compact-heading">
          <p className="section-index">05 / CONTROLS</p>
          <h2 id="controls-title">Hard edges. Clear intent.</h2>
        </div>
        <div className="control-grid">
          <div className="control-panel light-field" data-cursor-grow>
            <p className="specimen-label">Cursor field / paper</p>
            <div className="button-row">
              <button className="button button-solid" type="button">Approve package</button>
              <button className="button button-outline" type="button">Review rules</button>
            </div>
          </div>
          <div className="control-panel dark-field" data-cursor-grow>
            <p className="specimen-label specimen-label-inverse">Cursor field / ink</p>
            <p className="dark-field-copy">Move across both fields.<br />The square changes sides.</p>
          </div>
        </div>
        <div className="metadata-strip">
          <span>STATUS · ACTIVE</span>
          <span>VERSION · 0001</span>
          <span>UPDATED · 08 AUG 2026</span>
          <span>REQUEST · REQ_41AC</span>
        </div>
      </section>

      <section className="ruled-section motion-section" aria-labelledby="motion-title">
        <div className="section-heading compact-heading">
          <p className="section-index">06 / MOTION</p>
          <h2 id="motion-title">Weight, then glide.</h2>
        </div>
        <div className="motion-grid">
          <article>
            <span className="motion-mark motion-mark-expo" />
            <p className="specimen-label">Expo out</p>
            <code>cubic-bezier(.16,1,.3,1)</code>
          </article>
          <article>
            <span className="motion-mark motion-mark-overshoot" />
            <p className="specimen-label">Overshoot</p>
            <code>cubic-bezier(.34,1.56,.64,1)</code>
          </article>
        </div>
      </section>

      <section className="ruled-section api-section" aria-labelledby="api-title">
        <div className="section-heading compact-heading">
          <p className="section-index">07 / LIVE SEAM</p>
          <h2 id="api-title">The proxy still tells the truth.</h2>
        </div>
        <div className="api-console">
          <p>GET /API/V1/PROPERTIES · SAME-ORIGIN · AUTHENTICATED</p>
          {properties.isPending && <pre>LOADING · 03</pre>}
          {properties.isError && (
            <pre className="api-error">
              {properties.error instanceof ApiError
                ? `${properties.error.status} ${properties.error.code}\n${properties.error.message}\nrequest_id: ${properties.error.requestId}`
                : String(properties.error)}
            </pre>
          )}
          {properties.isSuccess && <pre>{JSON.stringify(properties.data, null, 2)}</pre>}
        </div>
      </section>

      <section className="ruled-section seed-control" id="seed-reset" aria-labelledby="seed-title">
        <div className="section-heading compact-heading">
          <p className="section-index">08 / DEMO CONTROL</p>
          <h2 id="seed-title">Reseed without theatre.</h2>
        </div>
        <div className="seed-control-body">
          <p>The idempotent demo seed runs from the terminal against the halted acceptance backend. This control copies the command; it never pretends the browser can reset a database.</p>
          <code>cd ~/comfort-curators-frontend/app &amp;&amp; npx tsx scripts/seed.ts</code>
            <button type="button" onClick={() => { void navigator.clipboard?.writeText("cd ~/comfort-curators-frontend/app && npx tsx scripts/seed.ts"); setCopied(true); }}> {copied ? "COPIED" : "COPY RESEED COMMAND →"}</button>
        </div>
      </section>

      <section className="ruled-section controls-section" aria-labelledby="primitives-title">
        <div className="section-heading compact-heading">
          <p className="section-index">09 / PRIMITIVES</p>
          <h2 id="primitives-title">Five small proofs.</h2>
        </div>
        <div className="control-grid">
          <div className="control-panel light-field">
            <p className="specimen-label">Select / live value: {selectedPrimitive}</p>
            <div className="button-row">
              <Select
                aria-label="Choose a material"
                value={selectedPrimitive}
                onChange={setSelectedPrimitive}
                options={[
                  { value: "paper", label: "PAPER" },
                  { value: "paper-2", label: "PAPER 02" },
                  { value: "ink", label: "INK" },
                ]}
              />
            </div>
            <div className="button-row">
              <button className="button button-solid" type="button" onClick={() => setModalOpen(true)}>
                OPEN MODAL
              </button>
              <Tooltip label="The trigger is keyboard-focusable too.">
                <button className="button button-outline" type="button">HOVER OR FOCUS</button>
              </Tooltip>
            </div>
          </div>
          <div className="control-panel dark-field">
            <p className="specimen-label specimen-label-inverse">Popover / CookieSlip</p>
            <div className="button-row">
              <Popover
                open={popoverOpen}
                onClose={() => setPopoverOpen(false)}
                label="Debug popover"
                trigger={<button className="button button-outline" style={{ color: "var(--paper)", borderColor: "var(--paper)" }} type="button" onClick={() => setPopoverOpen((open) => !open)}>OPEN POPOVER</button>}
              >
                <p>Inline content, focus-managed and dismissible with Escape.</p>
              </Popover>
              <button
                className="button button-outline"
                style={{ color: "var(--paper)", borderColor: "var(--paper)" }}
                type="button"
                onClick={() => {
                  window.localStorage.removeItem("cc_cookie_consent");
                  setCookieReset((value) => value + 1);
                }}
              >
                RESET CONSENT + SHOW
              </button>
            </div>
            <p className="dark-field-copy">Every primitive is live.<br />Nothing is a screenshot.</p>
          </div>
        </div>
        <CookieSlip key={cookieReset} />
        <Modal open={modalOpen} onClose={() => setModalOpen(false)} title="A live modal" label="POPUP 09 / DEBUG">
          <p>Focus moves into this panel. Close it with the button or Escape.</p>
        </Modal>
      </section>

      <section className="ruled-section controls-section" aria-labelledby="instruments-title">
        <div className="section-heading compact-heading">
          <p className="section-index">10 / INSTRUMENTS</p>
          <h2 id="instruments-title">The proofs outlived the move.</h2>
        </div>

        <div className="control-grid">
          <div className="control-panel light-field instrument-panel instrument-money" aria-label="Money formatting proof">
            <span className="instrument-stamp">PROOF ONLY</span>
            <p className="specimen-label">OPERATING SAMPLE / INR</p>
            <Money value={45000} currency="INR" className="money-proof" />
            <p className="instrument-note">Rendered from 45000 minor units — no float math.</p>
          </div>

          <div className="control-panel dark-field instrument-panel" aria-label="Session console">
            <p className="specimen-label specimen-label-inverse">SESSION / THIS TAB ONLY</p>
            <dl className="instrument-session">
              <div>
                <dt>ACTIVE ROLE</dt>
                <dd data-session-role={activeRole ?? "none"}>
                  {activeRole ? `${activeRole.toUpperCase()} ACTIVE` : "NO ROLE SELECTED"}
                </dd>
              </div>
              <div>
                <dt>TOKEN CHECK</dt>
                <dd>{sessionNote}</dd>
              </div>
              <div>
                <dt>AUTHENTICATED PROPERTIES</dt>
                <dd>{propertyCount === null ? "—" : `${propertyCount} RETURNED`}</dd>
              </div>
            </dl>
            <div className="button-row">
              {activeRole === "owner" && (
                <Link className="button button-outline" style={darkLink} to="/dashboard">
                  OWNER DESK →
                </Link>
              )}
              {activeRole === "staff" && (
                <Link className="button button-outline" style={darkLink} to="/ops/tickets">
                  OPS DESK →
                </Link>
              )}
              {shopPropertyId && activeRole !== "guest" && (
                <Link
                  className="button button-outline"
                  style={darkLink}
                  to={`/properties/${shopPropertyId}/package`}
                >
                  INVENTORY SHOP →
                </Link>
              )}
              <button
                className="button button-outline"
                style={darkLink}
                type="button"
                onClick={() => void mintSession()}
              >
                MINT OWNER SESSION
              </button>
              <button
                className="button button-outline"
                style={darkLink}
                type="button"
                onClick={clearSession}
                disabled={!activeRole}
              >
                CLEAR SESSION
              </button>
            </div>
          </div>
        </div>

        <div className="api-console instrument-error" aria-label="Forced API error proof">
          <p>FORCED ERROR / GET /API/V1/PROPERTIES/NOT-A-REAL-PROPERTY</p>
          <button className="button button-solid" type="button" onClick={() => void forceApiError()}>
            FORCE A REAL ERROR →
          </button>
          {forcedError && <pre className="api-error">{forcedError}</pre>}
        </div>
      </section>

      <section className="ruled-section terminal-demo-section" aria-labelledby="terminal-title">
        <div className="section-heading compact-heading">
          <p className="section-index">11 / SUPERHOST TERMINAL</p>
          <h2 id="terminal-title">A machine surface on paper.</h2>
        </div>
        <Terminal lines={terminalDemoLines} cursorVisible>
          <div className="terminal-demo-slot">CONFIRM BLOCK SLOT · P4.3</div>
        </Terminal>
      </section>

      <SuperhostBehaviorDemo />

      <AgentSurfaceDemo />

      <ControlSessionDemo />

      <PaymentBoundaryDemo />

      <footer className="sheet-footer">
        <p className="section-index section-index-inverse">16 / END OF PROOF</p>
        <p>Nothing soft.<br /><em>Nothing accidental.</em></p>
        <span>SCROLL COMPLETE · SYSTEM READY</span>
      </footer>
    </main>
  );
}
