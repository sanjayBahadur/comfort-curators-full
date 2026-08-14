import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { getTicketQueue, type WorkerData } from "../lib/api/ops";
import { OpsHeader, OpsSkeleton, StaffGate, StatusLabel } from "./ops-shared";
import { humanize, propertyName } from "./ops-format";
import "./ops.css";
import "./ops-rosters.css";

export default function OpsWorkers() {
  const queueQuery = useQuery({ queryKey: ["ops", "ticket-queue"], queryFn: getTicketQueue });
  const [selectedWorker, setSelectedWorker] = useState<WorkerData | null>(null);
  const workers = queueQuery.data?.workers ?? [];
  const assignments = selectedWorker
    ? (queueQuery.data?.tickets ?? []).flatMap((ticket) => ticket.assignments.filter((assignment) => assignment.data.worker_id === selectedWorker.id).map((assignment) => ({ assignment, ticket })))
    : [];

  function openWorker(worker: WorkerData) { setSelectedWorker(worker); }

  return (
    <StaffGate>
      <main className="ops-shell registration-frame">
        <OpsHeader section="03 / WORKERS" />
        <section className="ops-title-row">
          <div><p>THE PEOPLE WHO MAKE THE WORK REAL.</p><h1>Workers</h1></div>
          <strong>{queueQuery.data ? `${workers.length} WORKER${workers.length === 1 ? "" : "S"}` : "—"}</strong>
        </section>

        {queueQuery.isLoading ? <OpsSkeleton rows={6} /> : queueQuery.isError ? (
          <section className="ops-empty"><strong>WORKERS UNAVAILABLE.</strong><p>The backend message is shown in the toast. Retry when the service is ready.</p><button type="button" onClick={() => void queueQuery.refetch()}>RETRY</button></section>
        ) : workers.length === 0 ? (
          <section className="ops-empty"><strong>NO WORKERS.</strong><p>Verified workers will appear here when the roster is ready.</p></section>
        ) : (
          <div className="ops-table-wrap">
            <table className="ops-table">
              <thead><tr><th>NAME</th><th>ZONE</th><th>SKILLS</th><th>CLASSIFICATION</th><th>AVAILABILITY</th><th>IDENTITY</th><th><span className="sr-only">Open</span></th></tr></thead>
              <tbody>
                {workers.map((worker) => (
                  <tr key={worker.id} className="ops-clickable-row" tabIndex={0} role="button" onClick={() => openWorker(worker.data)} onKeyDown={(event) => { if (event.key === "Enter" || event.key === " ") { event.preventDefault(); openWorker(worker.data); } }}>
                    <td data-label="NAME"><strong>{worker.data.legal_name}</strong><small>{worker.id}</small></td>
                    <td data-label="ZONE">{worker.data.service_zone}</td>
                    <td data-label="SKILLS">{worker.data.skills.length ? worker.data.skills.join(", ") : "—"}</td>
                    <td data-label="CLASSIFICATION">{humanize(worker.data.classification)}</td>
                    <td data-label="AVAILABILITY">—</td>
                    <td data-label="IDENTITY"><StatusLabel status={worker.data.verified_identity ? "verified" : "unverified"} /></td>
                    <td className="ops-row-action"><button type="button" aria-label={`Open details for ${worker.data.legal_name}`} onClick={(event) => { event.stopPropagation(); openWorker(worker.data); }}>→</button></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </main>
      {selectedWorker && (
        <aside className="ops-roster-drawer" role="dialog" aria-modal="true" aria-labelledby="worker-drawer-title">
          <header><div><span>WORKER DETAIL</span><h2 id="worker-drawer-title">{selectedWorker.legal_name}</h2></div><button type="button" aria-label="Close worker details" onClick={() => setSelectedWorker(null)}>×</button></header>
          <section className="ops-roster-facts"><div><span>ZONE</span><strong>{selectedWorker.service_zone}</strong></div><div><span>CLASSIFICATION</span><strong>{humanize(selectedWorker.classification)}</strong></div><div><span>IDENTITY</span><strong>{selectedWorker.verified_identity ? "VERIFIED" : "NOT VERIFIED"}</strong></div><div><span>SKILLS</span><strong>{selectedWorker.skills.length ? selectedWorker.skills.join(", ") : "—"}</strong></div></section>
          <section className="ops-roster-panel"><header><span>01</span><strong>Availability windows</strong></header><p className="ops-roster-note">— Availability windows are not provided by the current worker API.</p></section>
          <section className="ops-roster-panel"><header><span>02</span><strong>Current assignments</strong></header>{assignments.length === 0 ? <p className="ops-roster-note">No current assignments in the loaded ticket queue.</p> : <ul className="ops-roster-list">{assignments.map(({ assignment, ticket }) => <li key={assignment.id}><strong>{propertyName(ticket.property)}</strong><span>{humanize(ticket.data.type)} · <StatusLabel status={ticket.data.status} /></span></li>)}</ul>}<p className="ops-roster-note">Assignment history is not provided by the current API.</p></section>
        </aside>
      )}
    </StaffGate>
  );
}
