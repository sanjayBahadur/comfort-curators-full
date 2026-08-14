import { useMemo, useState } from "react";
import { Link, Navigate, useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { getRole, getToken } from "../lib/auth/session";
import { getTicketDetail, registerTicketEvidence, sha256Hex, syncChecklist, transitionTicket, type ChecklistSyncItem } from "../lib/api/ops";
import { formatWindow, humanize, propertyName } from "./ops-format";
import { OpsSkeleton, StatusLabel } from "./ops-shared";
import CuratorHeader from "./curator-header";
import "./curator.css";

export default function CuratorJobDetail() {
  const { ticketId = "" } = useParams();
  const client = useQueryClient();
  const [checked, setChecked] = useState<Record<string, boolean>>({});
  const query = useQuery({ queryKey: ["curator", "job", ticketId], queryFn: () => getTicketDetail(ticketId), enabled: Boolean(ticketId) });
  const detail = query.data;
  const ticket = detail?.ticket;
  const property = detail?.properties.find((entry) => entry.id === ticket?.data.property_id);
  const allChecked = useMemo(() => Boolean(detail?.checklist.length) && (detail?.checklist.every((item) => checked[item.id] ?? (item.data.status === "completed" || item.data.status === "not_applicable")) ?? false), [checked, detail?.checklist]);

  const complete = useMutation({
    mutationFn: async () => {
      if (!detail || !ticket) throw new Error("Job is not loaded");
      if (!allChecked) throw new Error("Complete every checklist item before submitting the job.");
      const evidenceIds = new Map<string, string[]>();
      for (const item of detail.checklist.filter((entry) => entry.data.evidence_required && !(entry.data.evidence_ids?.length))) {
        const evidence = await registerTicketEvidence(ticket.id, { checklist_item_id: item.id, object_id: `stub://${ticket.id}/${item.id}`, content_hash: await sha256Hex(`${ticket.id}:${item.id}:curator-evidence`), file_name: `${item.data.label.replaceAll(/\s+/g, "-").toLowerCase()}.metadata`, content_type: "application/x-comfort-curators-evidence-metadata", size_bytes: 0 });
        evidenceIds.set(item.id, [evidence.id]);
      }
      const items: ChecklistSyncItem[] = detail.checklist.map((entry) => ({ template_item_index: entry.data.template_item_index, label: entry.data.label, status: checked[entry.id] || entry.data.status === "completed" ? "completed" : entry.data.status, completed_by: checked[entry.id] ? "curator-demo" : entry.data.completed_by, evidence_required: entry.data.evidence_required, evidence_ids: evidenceIds.get(entry.id) ?? entry.data.evidence_ids, notes: entry.data.notes }));
      await syncChecklist(ticket.id, items);
      let status = ticket.data.status;
      if (status === "scheduled") { await transitionTicket(ticket.id, "assigned"); status = "assigned"; }
      if (status === "assigned") { await transitionTicket(ticket.id, "in_progress"); status = "in_progress"; }
      if (status === "in_progress") await transitionTicket(ticket.id, "evidence_submitted");
    },
    onSuccess: async () => { await client.invalidateQueries({ queryKey: ["curator", "job", ticketId] }); await client.invalidateQueries({ queryKey: ["curator", "jobs"] }); toast.success("Evidence recorded · job submitted"); },
  });

  if (!getToken() || getRole() !== "staff") return <Navigate to="/login" replace />;
  if (!ticketId) return <Navigate to="/jobs" replace />;
  if (query.isLoading) return <main className="curator-shell"><OpsSkeleton rows={6} /></main>;
  if (query.isError || !detail || !ticket) return <main className="curator-shell"><section className="curator-empty"><strong>JOB UNAVAILABLE.</strong><p>The backend message is shown in the toast.</p><Link to="/jobs">RETURN TO JOBS</Link></section></main>;

  const access = property?.data.access_method || "Check the property access record before arrival.";
  const done = ticket.data.status === "evidence_submitted" || ticket.data.status === "verified" || ticket.data.status === "closed";
  return <main className="curator-shell">
     <CuratorHeader section="07 / JOB BRIEF" />
    <section className="curator-detail-hero"><p className="curator-kicker"><Link to="/jobs">← TODAY'S JOBS</Link> · {ticket.id}</p><h1>{humanize(ticket.data.type)}</h1><StatusLabel status={ticket.data.status} /></section>
    <section className="curator-detail-meta"><div><span>PROPERTY</span><strong>{propertyName(property)}</strong></div><div><span>ARRIVAL WINDOW</span><strong>{formatWindow(ticket.data.requested_window)}</strong></div><div><span>ASSIGNMENT</span><strong>{detail.assignments.find((assignment) => assignment.data.status !== "declined")?.data.status || "assigned"}</strong></div><div><span>TRAVEL ZONE</span><strong>{property?.data.geolocation_zone?.trim() || "UNZONED"}</strong></div></section>
    <div className="curator-detail-body">
      <section className="curator-panel"><header><span>01</span><strong>ACCESS + WORK BRIEF</strong></header><div className="curator-access"><strong>{propertyName(property)}</strong><p>{access}</p><p>{ticket.data.reason}</p>{property?.data.emergency_contacts?.[0] && <p>Emergency contact: {property.data.emergency_contacts[0].name} · {property.data.emergency_contacts[0].phone}</p>}</div></section>
      <section className="curator-panel"><header><span>02</span><strong>CHECKLIST</strong><small>{detail.checklist.length} ITEMS</small></header>{detail.checklist.length === 0 ? <div className="curator-evidence-note">No checklist is attached. Operations must add one before this job can be completed.</div> : <ul className="curator-checklist">{detail.checklist.map((item) => <li key={item.id}><input type="checkbox" id={item.id} checked={checked[item.id] ?? (item.data.status === "completed" || item.data.status === "not_applicable")} disabled={done || complete.isPending} onChange={(event) => setChecked((current) => ({ ...current, [item.id]: event.target.checked }))} /><label htmlFor={item.id}>{item.data.label}</label>{item.data.evidence_required && <small>{item.data.evidence_ids?.length ? "EVIDENCE ON FILE" : "EVIDENCE REQUIRED"}</small>}</li>)}</ul>}</section>
      <section className="curator-panel"><header><span>03</span><strong>REQUIRED EVIDENCE</strong></header><div className="curator-evidence-note">File upload is not connected in this demo. Completing the job records immutable evidence metadata and a SHA-256 digest against each required checklist item.</div></section>
      {complete.isError && <div className="curator-error" role="alert">{complete.error instanceof Error ? complete.error.message : "The job could not be completed."}</div>}
      <div className="curator-actions"><Link to="/jobs">BACK TO JOBS</Link><button type="button" disabled={done || complete.isPending || !allChecked || detail.checklist.length === 0} onClick={() => complete.mutate()}>{done ? "JOB SUBMITTED" : complete.isPending ? "RECORDING EVIDENCE…" : allChecked ? "COMPLETE JOB →" : "FINISH THE CHECKLIST FIRST"}</button></div>
    </div>
  </main>;
}
