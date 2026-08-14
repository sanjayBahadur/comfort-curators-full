import { useState, useSyncExternalStore } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import type { DashboardProperty } from "../../lib/api/dashboard";
import { transitionTicket, type TicketData } from "../../lib/api/ops";
import type { Envelope } from "../../lib/api/client";
import { clearPendingPurchase, getPendingPurchase, subscribePendingPurchase } from "../../lib/pending-purchase";
import { activatePackage } from "../../lib/api/shop";
import { PaymentBoundary, PaymentBoundaryButton } from "../superhost/PaymentBoundary";
import { formatMoney } from "../../lib/money";
import "./OwnerTaskTerminal.css";

const ACTIVE_STATES = new Set(["approved", "scheduled", "assigned", "in_progress", "evidence_submitted"]);
const APPROVAL_STATES = new Set(["draft", "proposed"]);

const TASK_TIME = new Intl.DateTimeFormat("en-IN", {
  day: "2-digit",
  month: "short",
  hour: "2-digit",
  minute: "2-digit",
  hour12: false,
  timeZone: "Asia/Kolkata",
});

function propertyName(property: DashboardProperty) {
  return property.property.data.service_address.line2 || property.property.data.service_address.line1 || "Managed property";
}

function taskWindow(ticket: Envelope<TicketData>) {
  const start = ticket.data.requested_window?.start;
  return start ? TASK_TIME.format(new Date(start)) : "WINDOW PENDING";
}

function taskName(ticket: Envelope<TicketData>) {
  return ticket.data.type.replaceAll("_", " ");
}

export default function OwnerTaskTerminal({ property }: { property: DashboardProperty }) {
  const queryClient = useQueryClient();
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const [approvalResult, setApprovalResult] = useState("");
  const pendingPurchase = useSyncExternalStore(
    subscribePendingPurchase,
    () => getPendingPurchase(property.property.id),
    () => [],
  );
  const pendingPurchaseSubtotal = pendingPurchase.reduce((total, line) => total + line.unitPriceMinorUnits * line.quantity, 0);
  const pendingPurchaseCurrency = pendingPurchase[0]?.currency ?? "INR";
  const active = property.tickets
    .filter((ticket) => ACTIVE_STATES.has(ticket.data.status))
    .sort((left, right) => taskWindow(left).localeCompare(taskWindow(right)));
  const approvals = property.tickets
    .filter((ticket) => APPROVAL_STATES.has(ticket.data.status))
    .sort((left, right) => right.data.updated_at.localeCompare(left.data.updated_at));
  const approveTicket = useMutation({
    mutationFn: (ticketId: string) => transitionTicket(
      ticketId,
      "approved",
      "Approved by the owner from the dashboard task terminal",
    ),
    onMutate: () => setApprovalResult(""),
    onSuccess: async (ticket) => {
      setApprovalResult(`${taskName(ticket)} approved and recorded.`);
      setExpandedId(null);
      await queryClient.invalidateQueries({ queryKey: ["owner", "dashboard"] });
    },
  });
  const pendingDraftPackageId = pendingPurchase.find((line) => line.draftPackageId)?.draftPackageId;
  const approveCart = useMutation({
    mutationFn: async () => {
      if (!pendingDraftPackageId) throw new Error("The cart draft is still settling. Open it to review before approval.");
      return activatePackage(property.property.id, pendingDraftPackageId);
    },
    onMutate: () => setApprovalResult(""),
    onSuccess: async () => {
      clearPendingPurchase(property.property.id);
      setApprovalResult("Cart approved. The package is active and the review task is cleared.");
      setExpandedId(null);
      await queryClient.invalidateQueries({ queryKey: ["owner", "dashboard"] });
    },
  });

  const toggle = (ticketId: string) => {
    setApprovalResult("");
    setExpandedId((current) => current === ticketId ? null : ticketId);
  };

  return (
    <section className="owner-task-terminal" aria-labelledby="owner-task-terminal-title">
      <header>
        <div><span>SUPERHOST /</span><strong id="owner-task-terminal-title">TASKS</strong></div>
        <small>{propertyName(property)}</small>
      </header>
      <div className="owner-task-terminal-summary">
        <span>· {propertyName(property)}</span>
        <span>› {active.length} active · {approvals.length + (pendingPurchase.length > 0 ? 1 : 0)} awaiting approval</span>
      </div>
      <ol className="owner-task-terminal-checklist" aria-label="Property task checklist">
        {pendingPurchase.length > 0 && (
          <li data-kind="pending-purchase">
            <b aria-hidden="true">[$]</b>
            <div>
              <button className="owner-task-terminal-toggle" type="button" aria-expanded={expandedId === "pending-purchase"} onClick={() => toggle("pending-purchase")}>
                <strong>UNAPPROVED CART · REVIEW REQUIRED</strong>
                <span>{pendingPurchase.length} line{pendingPurchase.length === 1 ? "" : "s"} · {expandedId === "pending-purchase" ? "close" : "review"}</span>
              </button>
              {expandedId === "pending-purchase" && (
                <div className="owner-task-terminal-detail">
                  {pendingPurchase.map((line) => (
                    <p key={line.sku}>› {line.name} × {line.quantity}</p>
                  ))}
                  <p>› SUBTOTAL {formatMoney(pendingPurchaseSubtotal, pendingPurchaseCurrency)} · DRAFT / NOT YET ACTIVATED</p>
                  <div className="owner-task-terminal-actions">
                    <PaymentBoundary>
                      <PaymentBoundaryButton type="button" disabled={approveCart.isPending || !pendingDraftPackageId} onClick={() => approveCart.mutate()}>
                        {approveCart.isPending ? "[ ACTIVATING… ]" : pendingDraftPackageId ? "[ APPROVE ENTIRE CART ]" : "[ CART STILL SETTLING ]"}
                      </PaymentBoundaryButton>
                    </PaymentBoundary>
                    <Link to={`/properties/${property.property.id}/package`}>[ REVIEW CART ]</Link>
                    <button type="button" disabled={approveCart.isPending} onClick={() => {
                      clearPendingPurchase(property.property.id);
                      setExpandedId(null);
                      setApprovalResult("Cart discarded. The review task is cleared.");
                    }}>[ DISCARD CART ]</button>
                  </div>
                </div>
              )}
            </div>
          </li>
        )}
        {active.map((ticket) => (
          <li key={ticket.id} data-kind="active">
            <b aria-hidden="true">[✓]</b>
            <div>
              <button className="owner-task-terminal-toggle" type="button" aria-expanded={expandedId === ticket.id} onClick={() => toggle(ticket.id)}>
                <strong>{taskName(ticket)}</strong>
                <span>{ticket.data.status.replaceAll("_", " ")} · {taskWindow(ticket)} · {expandedId === ticket.id ? "close" : "inspect"}</span>
              </button>
              {expandedId === ticket.id && (
                <div className="owner-task-terminal-detail">
                  <p>› {ticket.data.reason || "No additional task context was recorded."}</p>
                </div>
              )}
            </div>
          </li>
        ))}
        {approvals.map((ticket) => (
          <li key={ticket.id} data-kind="approval">
            <b aria-hidden="true">[?]</b>
            <div>
              <button className="owner-task-terminal-toggle" type="button" aria-expanded={expandedId === ticket.id} onClick={() => toggle(ticket.id)}>
                <strong>{taskName(ticket)}</strong>
                <span>{ticket.data.status === "proposed" ? "awaiting owner review" : "draft awaiting proposal"} · {taskWindow(ticket)} · {expandedId === ticket.id ? "close" : "review"}</span>
              </button>
              {expandedId === ticket.id && (
                <div className="owner-task-terminal-detail">
                  <p>› {ticket.data.reason || "No additional proposal context was recorded."}</p>
                  {ticket.data.status === "proposed" ? (
                    <div className="owner-task-terminal-actions">
                      <button type="button" disabled={approveTicket.isPending} onClick={() => approveTicket.mutate(ticket.id)}>
                        {approveTicket.isPending && approveTicket.variables === ticket.id ? "[ RECORDING… ]" : "[ APPROVE ]"}
                      </button>
                      <button type="button" disabled={approveTicket.isPending} onClick={() => setExpandedId(null)}>[ NOT NOW ]</button>
                    </div>
                  ) : <p className="owner-task-terminal-note">· DRAFT MUST BE PROPOSED BEFORE OWNER APPROVAL</p>}
                </div>
              )}
            </div>
          </li>
        ))}
        {pendingPurchase.length === 0 && active.length === 0 && approvals.length === 0 && <li className="owner-task-terminal-empty"><b aria-hidden="true">[ ]</b><span>NO ACTIVE OR APPROVAL-PENDING TASKS</span></li>}
      </ol>
      {approvalResult && <p className="owner-task-terminal-result" role="status">✓ {approvalResult}</p>}
      {approveTicket.isError && <p className="owner-task-terminal-error" role="alert">! Approval was not recorded. Please try again.</p>}
      {approveCart.isError && <p className="owner-task-terminal-error" role="alert">! {approveCart.error instanceof Error ? approveCart.error.message : "Cart approval failed. Please review the cart."}</p>}
    </section>
  );
}
