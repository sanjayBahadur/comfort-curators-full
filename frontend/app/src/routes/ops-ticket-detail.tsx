import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, Navigate, useParams } from "react-router-dom";
import { toast } from "sonner";
import {
  assignWorker,
  getDispatchCandidates,
  getTicketDetail,
  transitionTicket,
  type ConstraintCheck,
  type DispatchCandidate,
} from "../lib/api/ops";
import {
  OpsHeader,
  OpsSkeleton,
  StaffGate,
  StatusLabel,
} from "./ops-shared";
import {
  formatMoment,
  formatWindow,
  humanize,
  propertyName,
  workerName,
} from "./ops-format";
import SuperhostMount from "../components/superhost/SuperhostMount";
import { setCurrentProperty, clearCurrentProperty } from "../lib/current-property";
import { useAgentSurface } from "../components/agent-surface/context";
import type { AgentAction } from "../components/agent-surface/types";
import "./ops.css";

const PREPARE_STATES = ["draft", "proposed", "approved", "scheduled"];

const CHECK_LABELS: Record<string, string> = {
  age: "Minimum age",
  worker_status: "Active worker",
  skill: "Required skill",
  service_zone: "Service zone",
  availability: "Availability",
  working_hours: "Working hours",
  rest: "Rest period",
  travel: "Travel window",
  safety: "Safety requirement",
  access: "Property access",
  two_person: "Crew size",
};

const PASS_COPY: Record<string, string> = {
  age: "Meets minimum age",
  worker_status: "Currently active",
  skill: "Has a matching work skill",
  service_zone: "Covers the property zone",
  availability: "Availability is configured",
  working_hours: "Inside working-hours limit",
  rest: "Required rest is satisfied",
  travel: "Travel window is feasible",
  safety: "Safety requirement is satisfied",
  access: "Access requirement is satisfied",
  two_person: "Crew-size requirement is satisfied",
};

const FAIL_COPY: Record<string, string> = {
  age: "Does not meet minimum age",
  worker_status: "Not currently active",
  skill: "Missing a required work skill",
  service_zone: "Outside the property zone",
  availability: "No matching availability",
  working_hours: "Working-hours limit exceeded",
  rest: "Required rest is not satisfied",
  travel: "Travel window is not feasible",
  safety: "Safety requirement is not satisfied",
  access: "Access requirement is not satisfied",
  two_person: "A larger crew is required",
};

function checkCopy(check: ConstraintCheck) {
  return check.detail || (check.passed ? PASS_COPY[check.constraint] : FAIL_COPY[check.constraint]) || `${humanize(check.constraint)} ${check.passed ? "passed" : "failed"}`;
}

export default function OpsTicketDetail() {
  const { ticketId = "" } = useParams();
  const queryClient = useQueryClient();
  const [candidates, setCandidates] = useState<DispatchCandidate[] | null | undefined>(undefined);
  const newTicketSurface = useAgentSurface("ops-ticket-detail-new-ticket", ["focus", "click", "scroll_to"] as AgentAction[], "Open the new ticket form");
  const prepareSurface = useAgentSurface("ops-ticket-detail-prepare", ["focus", "click"] as AgentAction[], "Prepare the ticket for dispatch");
  const candidatesSurface = useAgentSurface("ops-ticket-detail-find-candidates", ["focus", "click"] as AgentAction[], "Find ranked dispatch candidates");
  const detailQuery = useQuery({
    queryKey: ["ops", "ticket", ticketId],
    queryFn: () => getTicketDetail(ticketId),
    enabled: Boolean(ticketId),
  });

  const prepareMutation = useMutation({
    mutationFn: async (status: string) => {
      const index = PREPARE_STATES.indexOf(status);
      if (index < 0 || index === PREPARE_STATES.length - 1) return;
      for (const nextState of PREPARE_STATES.slice(index + 1)) await transitionTicket(ticketId, nextState);
    },
    onSettled: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["ops", "ticket", ticketId] }),
        queryClient.invalidateQueries({ queryKey: ["ops", "ticket-queue"] }),
      ]);
    },
    onSuccess: () => {
      toast.success("Ticket prepared for dispatch");
    },
  });

  const candidateMutation = useMutation({
    mutationFn: () => getDispatchCandidates(ticketId),
    onSuccess: (response) => setCandidates(response.data.candidates),
  });

  const assignMutation = useMutation({
    mutationFn: (workerId: string) => assignWorker(ticketId, workerId),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["ops", "ticket", ticketId] }),
        queryClient.invalidateQueries({ queryKey: ["ops", "ticket-queue"] }),
      ]);
      toast.success("Assignment offer created");
    },
  });

  const rankedCandidates = useMemo(() => [...(candidates ?? [])].sort((left, right) => {
    if (left.eligible !== right.eligible) return Number(right.eligible) - Number(left.eligible);
    if (left.score !== right.score) return right.score - left.score;
    const leftName = workerName(left.worker_id, detailQuery.data?.workers ?? []);
    const rightName = workerName(right.worker_id, detailQuery.data?.workers ?? []);
    return leftName.localeCompare(rightName);
  }), [candidates, detailQuery.data?.workers]);

  const detail = detailQuery.data;
  const ticket = detail?.ticket;
  const property = detail?.properties.find((entry) => entry.id === ticket?.data.property_id);

  useEffect(() => {
    if (property) setCurrentProperty({ id: property.id, label: propertyName(property) });
    else clearCurrentProperty();
    return clearCurrentProperty;
  }, [property]);

  if (!ticketId) return <Navigate to="/ops/tickets" replace />;

  const assignment = detail?.assignments.find((entry) => entry.data.status !== "declined") ?? detail?.assignments.at(-1);
  const isAssignable = ticket?.data.status === "scheduled" || ticket?.data.status === "assigned";
  const canPrepare = ticket ? PREPARE_STATES.includes(ticket.data.status) && ticket.data.status !== "scheduled" : false;

  return (
    <StaffGate>
      <main className="ops-shell registration-frame">
        <OpsHeader section="03 / TICKET DETAIL" action={<Link ref={newTicketSurface.ref} to="/ops/tickets/new">NEW TICKET</Link>} />
        {detailQuery.isLoading ? <OpsSkeleton rows={7} /> : detailQuery.isError || !detail ? (
          <section className="ops-empty"><strong>Ticket unavailable.</strong><p>The backend message is shown in the toast.</p><Link to="/ops/tickets">RETURN TO QUEUE</Link></section>
        ) : (
          <>
            <section className="ops-detail-heading">
              <div>
                <Link to="/ops/tickets">← QUEUE</Link>
                <p>{ticket?.id}</p>
                <h1>{humanize(ticket?.data.type ?? "ticket")}</h1>
              </div>
              {ticket && <StatusLabel status={ticket.data.status} />}
            </section>

            <section className="ops-detail-meta">
              <div><span>PROPERTY</span><strong>{propertyName(property)}</strong></div>
              <div><span>REQUESTED WINDOW</span><strong>{formatWindow(ticket?.data.requested_window)}</strong></div>
              <div><span>ASSIGNMENT</span><strong>{workerName(assignment?.data.worker_id, detail.workers)}</strong>{assignment && <small>{humanize(assignment.data.status)}</small>}</div>
            </section>

            <SuperhostMount
              propertyId={property?.id}
              routeKey={`ops-ticket-${ticketId}`}
              purpose={`Ticket triage for ${ticketId}`}
              emptyMessage="not connected: waiting for the ticket property"
            />

            <div className="ops-detail-grid">
              <div className="ops-detail-main">
                <section className="ops-panel">
                  <header><span>01</span><strong>WORK BRIEF</strong></header>
                  <p className="ops-reason">{ticket?.data.reason}</p>
                </section>

                <section className="ops-panel">
                  <header><span>02</span><strong>CHECKLIST</strong><small>{detail.checklist.length} ITEMS</small></header>
                  {detail.checklist.length === 0 ? <p className="ops-quiet-empty">No checklist is attached to this ticket.</p> : (
                    <ul className="ops-checklist">
                      {detail.checklist.map((item) => <li key={item.id}><StatusLabel status={item.data.status} /><strong>{item.data.label}</strong>{item.data.evidence_required && <small>EVIDENCE REQUIRED</small>}</li>)}
                    </ul>
                  )}
                </section>

                <section className="ops-panel">
                  <header><span>03</span><strong>EVIDENCE</strong><small>{detail.evidence.length} RECORDS</small></header>
                  {detail.evidence.length === 0 ? <p className="ops-quiet-empty">No evidence has been submitted.</p> : (
                    <ul className="ops-evidence-list">
                      {detail.evidence.map((record) => <li key={record.id}><strong>{record.data.file_name || record.id}</strong><span>{record.data.content_type || "Recorded evidence"}</span><StatusLabel status={record.data.status} /></li>)}
                    </ul>
                  )}
                </section>

                <section className="ops-panel ops-history">
                  <header><span>04</span><strong>STATE HISTORY</strong><small>{detail.events.length} EVENTS</small></header>
                  {detail.events.length === 0 ? <p className="ops-quiet-empty">No state changes yet. Ticket creation itself does not emit a state event.</p> : (
                    <ol>
                      {detail.events.map((event) => <li key={event.id}><i aria-hidden="true" /><time>{formatMoment(event.data.created_at)}</time><strong>{humanize(event.data.from_state)} → {humanize(event.data.to_state)}</strong><p>{event.data.reason}</p><small>ACTOR {event.data.actor_id.slice(0, 8)}</small></li>)}
                    </ol>
                  )}
                </section>
              </div>

              <aside className="ops-dispatch" aria-labelledby="dispatch-title">
                <header><span>05 / DISPATCH</span><h2 id="dispatch-title">Rank the crew.</h2></header>
                {assignment && (
                  <section className="ops-current-assignment">
                    <span>CURRENT ASSIGNMENT</span>
                    <strong>{workerName(assignment.data.worker_id, detail.workers)}</strong>
                    <StatusLabel status={assignment.data.status} />
                    <small>Offer {assignment.id}</small>
                  </section>
                )}

                {canPrepare && (
                  <section className="ops-prepare">
                    <strong>NOT ASSIGNABLE YET</strong>
                    <p>Dispatch requires a scheduled ticket. This action records each real approval state in order.</p>
                    <button ref={prepareSurface.ref} type="button" disabled={prepareMutation.isPending} onClick={() => prepareMutation.mutate(ticket?.data.status ?? "draft")}>{prepareMutation.isPending ? "RECORDING STATES…" : "PREPARE FOR DISPATCH →"}</button>
                  </section>
                )}

                <button ref={candidatesSurface.ref} className="ops-candidate-trigger" type="button" disabled={candidateMutation.isPending} onClick={() => candidateMutation.mutate()}>{candidateMutation.isPending ? "EVALUATING…" : candidates === undefined ? "FIND RANKED CANDIDATES" : "REFRESH RANKING"}</button>
                <p className="ops-dispatch-note">Eligibility is evaluated by the backend from skills, zone, availability, hours, rest, safety, and crew size.</p>

                {candidates === null ? <p className="ops-candidate-empty">No eligible workers in this zone.</p> : rankedCandidates.length > 0 && (
                  <ol className="ops-candidates">
                    {rankedCandidates.map((candidate, index) => {
                      const worker = detail.workers.find((entry) => entry.id === candidate.worker_id);
                      const isCurrent = assignment?.data.worker_id === candidate.worker_id && assignment.data.status !== "declined";
                      return (
                        <li key={candidate.worker_id} data-eligible={candidate.eligible}>
                          <header><b>{String(index + 1).padStart(2, "0")}</b><div><strong>{worker?.data.legal_name ?? "Unknown worker"}</strong><span>{worker?.data.service_zone || "Zone not set"} · {candidate.score} CHECK POINTS</span></div><em>{candidate.eligible ? "ELIGIBLE" : "INELIGIBLE"}</em></header>
                          <p>{worker?.data.skills.join(" · ") || "No skills recorded"}</p>
                          <ul>{candidate.checks.map((check) => <li key={check.constraint} data-passed={check.passed}><i aria-hidden="true" /> <strong>{CHECK_LABELS[check.constraint] ?? humanize(check.constraint)}</strong><span>{checkCopy(check)}</span>{check.hard && <small>HARD</small>}</li>)}</ul>
                          <button type="button" disabled={!candidate.eligible || !isAssignable || Boolean(assignment) || assignMutation.isPending} onClick={() => assignMutation.mutate(candidate.worker_id)}>{isCurrent ? "CURRENT OFFER" : !isAssignable ? "SCHEDULE FIRST" : assignment ? "ASSIGNMENT EXISTS" : assignMutation.isPending ? "ASSIGNING…" : `ASSIGN ${worker?.data.legal_name?.split(" ")[0]?.toUpperCase() ?? "WORKER"} →`}</button>
                        </li>
                      );
                    })}
                  </ol>
                )}
              </aside>
            </div>
          </>
        )}
      </main>
    </StaffGate>
  );
}
