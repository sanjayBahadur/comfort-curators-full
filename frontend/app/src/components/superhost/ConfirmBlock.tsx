import { useEffect, useState } from "react";
import { api } from "../../lib/api/client";
import type { MappedSuperhostEvent } from "./behavior";
import "./ConfirmBlock.css";

type Approval = Extract<MappedSuperhostEvent, { type: "approval_required" }>;
type Decision = "approved" | "rejected";
type DecisionResponse = { request_id: string; decision: Decision };
type SubmissionState =
  | { status: "idle" }
  | { status: "pending"; decision: Decision }
  | { status: "success"; response: DecisionResponse }
  | { status: "error"; message: string };

type ConfirmBlockProps = {
  approval: Approval;
};

// Fired whenever an approval request is genuinely waiting on a person and
// again when it stops -- GlobalSuperhostButton listens for this to glow
// orange, so "Superhost needs you in the chat" is visible even with the
// drawer minimized (which it now is by default the instant a message is
// sent -- see GlobalSuperhost.tsx's own minimize-on-send). detail:true
// on mount (a fresh approval, keyed by requestId, always mounts a new
// instance of this component -- see SuperhostMount's `key={approval.
// requestId}`), detail:false the moment it's actually decided or this
// instance goes away.
export const SUPERHOST_NEEDS_INPUT_EVENT = "cc:superhost-needs-input";

function announceNeedsInput(pending: boolean) {
  window.dispatchEvent(new CustomEvent<boolean>(SUPERHOST_NEEDS_INPUT_EVENT, { detail: pending }));
}

export default function ConfirmBlock({ approval }: ConfirmBlockProps) {
  const [submission, setSubmission] = useState<SubmissionState>({ status: "idle" });

  useEffect(() => {
    announceNeedsInput(true);
    return () => announceNeedsInput(false);
    // Mount/unmount only -- a fresh approval is a fresh component instance
    // (keyed by requestId), so this never needs to re-fire mid-life.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (submission.status === "success") announceNeedsInput(false);
  }, [submission.status]);

  async function decide(decision: Decision) {
    setSubmission({ status: "pending", decision });
    try {
      const response = await api<DecisionResponse>(
        `/v1/superhost/approvals/${encodeURIComponent(approval.requestId)}/decide`,
        {
          method: "POST",
          body: JSON.stringify({ decision }),
        },
      );
      setSubmission({ status: "success", response });
    } catch (error) {
      setSubmission({
        status: "error",
        message: error instanceof Error ? error.message : String(error),
      });
    }
  }

  if (submission.status === "success") {
    return (
      <div className="superhost-confirm-block superhost-confirm-block-result" role="status">
        <span className="superhost-terminal-prefix" aria-hidden="true">&gt; </span>
        <span>
          decision recorded: {submission.response.decision} · request_id: {submission.response.request_id}
        </span>
      </div>
    );
  }

  const pending = submission.status === "pending";
  const errorMessage = submission.status === "error" ? submission.message : null;
  // Only an in-flight request disables the buttons. A failed decision (a
  // network hiccup, a transient 500) must not lock the human out
  // permanently with no path forward but a page reload -- they still
  // haven't actually confirmed or denied anything, so retrying is exactly
  // the right next action. Only a genuine success (the decision really is
  // recorded server-side) should make the buttons permanently inert.
  const decisionSubmitted = pending;

  return (
    <div className="superhost-confirm-block" aria-label="Approval request">
      <div className="superhost-confirm-summary superhost-terminal-line superhost-terminal-line-agent">
        <span className="superhost-terminal-prefix" aria-hidden="true">&gt; </span>
        <span>{approval.summary}</span>
      </div>
      {pending && (
        <p className="superhost-confirm-status" role="status">
          recording decision: {submission.decision}
        </p>
      )}
      {errorMessage && (
        <p className="superhost-confirm-error" role="alert">
          decision failed: {errorMessage}
        </p>
      )}
      <div className="superhost-confirm-actions">
        <button type="button" onClick={() => void decide("approved")} disabled={decisionSubmitted}>
          CONFIRM
        </button>
        <button type="button" onClick={() => void decide("rejected")} disabled={decisionSubmitted}>
          NOT NOW
        </button>
        <span className="superhost-confirm-label">
          APPROVAL · {approval.toolName} · REQUEST {approval.requestId}
        </span>
      </div>
    </div>
  );
}
