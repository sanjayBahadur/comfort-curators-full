import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import {
  type ControlSession,
  createIdleSession,
  grantSession,
  canAct as checkCanActFn,
  recordAction as recordActionFn,
  remainingTime,
  isAgentInFlight,
} from "./control-session";

export type RevokeReason =
  | "esc"
  | "click_strip"
  | "user_interaction"
  | "ttl_expired"
  | "action_capped"
  | "external"
  | "manual";

type ControlSessionContextValue = {
  session: ControlSession;
  grant: () => void;
  revoke: (reason: RevokeReason) => void;
  canAct: true | "not_granted" | "expired" | "action_capped" | "too_fast";
  recordAction: () => void;
  remainingMs: number;
  revokeReason: RevokeReason | null;
  timeDisplay: string;
};

const ControlSessionContext = createContext<ControlSessionContextValue | null>(null);

export function useControlSession(): ControlSessionContextValue {
  const ctx = useContext(ControlSessionContext);
  if (!ctx) throw new Error("useControlSession must be used within ControlSessionProvider");
  return ctx;
}

function formatTime(ms: number): string {
  const totalSeconds = Math.ceil(ms / 1000);
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`;
}

export function ControlSessionProvider({ children }: { children: ReactNode }) {
  const [session, setSession] = useState<ControlSession>(createIdleSession);
  const [timeDisplay, setTimeDisplay] = useState("00:00");
  const [revokeReason, setRevokeReason] = useState<RevokeReason | null>(null);
  const sessionRef = useRef<ControlSession>(session);
  const revokeRef = useRef<(reason: RevokeReason) => void>(() => {});

  sessionRef.current = session;

  const revoke = useCallback((reason: RevokeReason) => {
    const idle = createIdleSession();
    sessionRef.current = idle;
    setRevokeReason(reason);
    setSession(idle);
  }, []);

  revokeRef.current = revoke;

  const grant = useCallback(() => {
    const granted = grantSession(Date.now());
    sessionRef.current = granted;
    setRevokeReason(null);
    setSession(granted);
  }, []);

  const doRecordAction = useCallback(() => {
    setSession((current) => {
      if (current.state !== "granted") return current;
      const now = Date.now();
      const next = recordActionFn(current, now);
      if (next.actionCount >= next.maxActions) {
        const idle = createIdleSession();
        sessionRef.current = idle;
        setRevokeReason("action_capped");
        return idle;
      }
      sessionRef.current = next;
      return next;
    });
  }, []);

  useEffect(() => {
    if (session.state !== "granted") {
      setTimeDisplay("00:00");
      return;
    }

    const update = () => {
      const latest = sessionRef.current;
      if (latest.state !== "granted") return;
      const remaining = remainingTime(latest, Date.now());
      setTimeDisplay(formatTime(remaining));
      if (remaining <= 0) {
        revokeRef.current("ttl_expired");
      }
    };
    update();
    const interval = window.setInterval(update, 200);
    return () => window.clearInterval(interval);
  }, [session.state, session.startedAt]);

  useEffect(() => {
    function handleKeydown(e: KeyboardEvent) {
      if (e.key === "Escape" && sessionRef.current.state === "granted") {
        e.preventDefault();
        revokeRef.current("esc");
      }
    }

    function handleUserEvent(e: Event) {
      if (sessionRef.current.state !== "granted") return;
      if (!e.isTrusted) return;
      if (isAgentInFlight()) return;

      const ignoredTags = new Set(["SCRIPT", "STYLE", "LINK", "META"]);
      const target = e.target as HTMLElement | null;
      if (target && ignoredTags.has(target.tagName)) return;

      if (e.type === "click" || e.type === "input" || e.type === "keydown") {
        let el: HTMLElement | null = target;
        while (el) {
          // data-control-revoke: the strip -- clicking it deliberately
          // revokes via its own labeled reason (click_strip), so the
          // generic path must not also fire.
          // data-agent-trigger: debug/test-harness buttons that simulate
          // an agent-driven action by calling the gated driver directly
          // (there is no real model-to-intent pipeline wired yet -- see
          // P4.7's phase log). Clicking one of these is the human
          // directing a simulated test case, not a real interaction with
          // page content, and must not itself count as "the human took
          // the wheel back" -- if it did, no gated action could ever be
          // manually demonstrated, since the trigger click would revoke
          // the session before the driver's own grant-check ever ran.
          // data-control-exempt: the Superhost composer itself (see
          // SuperhostMount.tsx). Handing over control is meant to be
          // followed by typing a request in the same composer -- without
          // this, the very first keystroke after granting silently
          // revoked the grant before the model's response even came
          // back, so "hand over control, then say what you want" could
          // never work. This check used to run for clicks only, which is
          // why it didn't already cover the composer's "input"/keydown
          // events; extended to all three so typing is exempt too, not
          // just clicking into the field.
          if (
            el.hasAttribute("data-control-revoke") ||
            el.hasAttribute("data-agent-trigger") ||
            el.hasAttribute("data-control-exempt")
          ) {
            return;
          }
          el = el.parentElement;
        }
        revokeRef.current("user_interaction");
      }
    }

    document.addEventListener("keydown", handleKeydown, { capture: true });
    document.addEventListener("click", handleUserEvent, { capture: true });
    document.addEventListener("input", handleUserEvent, { capture: true });

    return () => {
      document.removeEventListener("keydown", handleKeydown, { capture: true });
      document.removeEventListener("click", handleUserEvent, { capture: true });
      document.removeEventListener("input", handleUserEvent, { capture: true });
    };
  }, []);

  const now = Date.now();
  const value: ControlSessionContextValue = {
    session,
    grant,
    revoke,
    canAct: checkCanActFn(session, now),
    recordAction: doRecordAction,
    remainingMs: session.state === "granted" ? remainingTime(session, now) : 0,
    revokeReason,
    timeDisplay,
  };

  return (
    <ControlSessionContext.Provider value={value}>
      {children}
    </ControlSessionContext.Provider>
  );
}
