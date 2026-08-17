import {
  type ButtonHTMLAttributes,
  createContext,
  useContext,
  useEffect,
  useCallback,
  useState,
  type ReactNode,
} from "react";
import { useControlSession } from "./ControlSession";
import { useAgentSurface, useAgentSurfaceContext } from "../agent-surface/context";
import Terminal, { type TerminalLine } from "./Terminal";
import { SUPERHOST_NEEDS_INPUT_EVENT } from "./ConfirmBlock";
import "./payment-boundary.css";

type PaymentBoundaryContextValue = {
  isInside: boolean;
  guardAction: () => boolean;
};

export const PaymentBoundaryContext = createContext<PaymentBoundaryContextValue>({
  isInside: false,
  guardAction: () => true,
});

export function usePaymentBoundary(): PaymentBoundaryContextValue {
  return useContext(PaymentBoundaryContext);
}

type PaymentBoundaryProps = {
  children: ReactNode;
  boundaryId?: string;
};

// The exact two-line message + footer from IMPLEMENTATION-SPEC.md §3.9.3,
// verbatim. Do not reword.
const GATE3_LINES: TerminalLine[] = [
  { id: "g3-1", kind: "denial", text: "i can't take this step. paying is yours." },
  { id: "g3-2", kind: "denial", text: "i've handed control back." },
  { id: "g3-footer", kind: "denial", text: "CONTROL REVOKED / PAYMENT BOUNDARY" },
];

// Gate 3 — session invalidation on a real protected-action attempt, not merely
// because a price or activation control is visible. Gate 2 still keeps every
// descendant out of AgentSurface, so the model cannot name or target it.
//
// This renders its own red frame + terminal notice directly (not by
// reaching into ControlFrame's DOM), because calling revoke() flips the
// control session to "idle" on the very next render, and ControlFrame
// only renders its border/strip while "granted" -- any attempt to style
// those elements after revoke() has already fired finds nothing in the
// DOM. Owning the visual as real component state sidesteps that race
// entirely and also means real routes (stay.tsx, package-shop.tsx), not
// just the /debug demo, actually show the revocation -- not a value
// manufactured client-side that isn't really seen by the visual layer.
const PAYMENT_BOUNDARY_ATTEMPT = "cc:payment-boundary-attempt";

export function PaymentBoundary({ children, boundaryId }: PaymentBoundaryProps) {
  const { session, revoke } = useControlSession();
  const { clearAll, restoreAll } = useAgentSurfaceContext();
  const [triggered, setTriggered] = useState(false);

  // Reaching a protected action is different from merely seeing its price or
  // button. The old IntersectionObserver revoked control whenever the bottom
  // of this subtree happened to be visible; on a desktop package layout that
  // can be true the instant control is granted, blocking every catalog action.
  // The guarded button below now invokes this only on an actual human attempt.
  // The first attempt hands control back and does not execute. A deliberate
  // second click, after handover, performs the human-owned action.
  const guardAction = useCallback(() => {
    if (session.state !== "granted") {
      setTriggered(false);
      return true;
    }
    clearAll();
    revoke("external");
    setTriggered(true);
    return false;
  }, [session.state, clearAll, revoke]);

  useEffect(() => {
    if (session.state === "granted") {
      restoreAll();
      setTriggered(false);
    }
  }, [session.state, restoreAll]);

  // The handoff itself is the clearest "come look" moment there is --
  // control was just taken away specifically because the next step is
  // the person's alone. Reuses the same signal ConfirmBlock uses (see
  // GlobalSuperhostButton), so the one launcher glows orange for either
  // reason: a real approval sitting in the chat, or a checkout tripwire
  // that just handed back control.
  useEffect(() => {
    window.dispatchEvent(new CustomEvent<boolean>(SUPERHOST_NEEDS_INPUT_EVENT, { detail: triggered }));
  }, [triggered]);

  useEffect(() => {
    if (!boundaryId) return undefined;
    const handleAttempt = (event: Event) => {
      if ((event as CustomEvent<string>).detail === boundaryId) guardAction();
    };
    window.addEventListener(PAYMENT_BOUNDARY_ATTEMPT, handleAttempt);
    return () => window.removeEventListener(PAYMENT_BOUNDARY_ATTEMPT, handleAttempt);
  }, [boundaryId, guardAction]);

  return (
    <PaymentBoundaryContext.Provider value={{ isInside: true, guardAction }}>
      {triggered && (
        <>
          <div className="payment-boundary-frame" aria-hidden="true" />
          <div className="payment-boundary-notice" role="alert">
            {/* The notice used to only clear on the next handover, with no
                way to dismiss it in place -- on a bottom-right layout it sat
                directly over the real, human-owned activate control it was
                supposed to hand back to, so there was no way to actually
                reach that button afterward. Dismissing here only hides this
                notice; it does not re-grant control, so the boundary itself
                is unaffected. */}
            <button
              type="button"
              className="payment-boundary-dismiss"
              aria-label="Dismiss payment boundary notice"
              data-control-exempt="true"
              onClick={() => setTriggered(false)}
            >
              ×
            </button>
            <Terminal lines={GATE3_LINES} />
          </div>
        </>
      )}
      {children}
    </PaymentBoundaryContext.Provider>
  );
}

export function PaymentBoundaryButton({ onClick, ...props }: ButtonHTMLAttributes<HTMLButtonElement>) {
  const { guardAction } = usePaymentBoundary();

  return (
    <button
      {...props}
      // Without this exemption the document-level "human took the wheel"
      // listener revokes first and the boundary cannot render its explicit red
      // handover. This button owns that one transition instead.
      data-control-exempt="true"
      onClick={(event) => {
        if (!guardAction()) {
          event.preventDefault();
          event.stopPropagation();
          return;
        }
        onClick?.(event);
      }}
    />
  );
}

type PaymentBoundaryTriggerButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  boundaryId: string;
  agentId: string;
  agentLabel: string;
};

// Model-visible checkout tripwire. This is deliberately outside the real
// PaymentBoundary subtree: clicking it can only request a handoff, never run
// the protected action. Once control is revoked, the route replaces it with
// the human-owned PaymentBoundaryButton.
export function PaymentBoundaryTriggerButton({
  boundaryId,
  agentId,
  agentLabel,
  onClick,
  ...props
}: PaymentBoundaryTriggerButtonProps) {
  const { ref } = useAgentSurface(agentId, ["click"], agentLabel);

  return (
    <button
      {...props}
      ref={ref}
      onClick={(event) => {
        window.dispatchEvent(new CustomEvent(PAYMENT_BOUNDARY_ATTEMPT, { detail: boundaryId }));
        onClick?.(event);
      }}
    />
  );
}
