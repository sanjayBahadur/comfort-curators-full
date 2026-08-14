import { useMemo } from "react";
import { Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { getTicketQueue, isPropertyReady } from "../lib/api/ops";
import { OpsHeader, OpsSkeleton, StaffGate, StatusLabel } from "./ops-shared";
import { propertyAddress, propertyName } from "./ops-format";
import "./ops.css";
import "./ops-rosters.css";

const CLOSED_TICKET_STATES = new Set(["closed", "cancelled", "rejected"]);

export default function OpsProperties() {
  const queueQuery = useQuery({ queryKey: ["ops", "ticket-queue"], queryFn: getTicketQueue });
  const properties = queueQuery.data?.properties ?? [];
  const openTicketsByProperty = useMemo(() => {
    const counts = new Map<string, number>();
    for (const ticket of queueQuery.data?.tickets ?? []) {
      if (!CLOSED_TICKET_STATES.has(ticket.data.status)) {
        counts.set(ticket.data.property_id, (counts.get(ticket.data.property_id) ?? 0) + 1);
      }
    }
    return counts;
  }, [queueQuery.data?.tickets]);

  return (
    <StaffGate>
      <main className="ops-shell registration-frame">
        <OpsHeader section="02 / PROPERTIES" />
        <section className="ops-title-row">
          <div><p>THE PLACES WE KEEP.</p><h1>Properties</h1></div>
          <strong>{queueQuery.data ? `${properties.length} PROPERT${properties.length === 1 ? "Y" : "IES"}` : "—"}</strong>
        </section>

        {queueQuery.isLoading ? <OpsSkeleton rows={6} /> : queueQuery.isError ? (
          <section className="ops-empty"><strong>PROPERTIES UNAVAILABLE.</strong><p>The backend message is shown in the toast. Retry when the service is ready.</p><button type="button" onClick={() => void queueQuery.refetch()}>RETRY</button></section>
        ) : properties.length === 0 ? (
          <section className="ops-empty"><strong>NO PROPERTIES.</strong><p>Properties will appear here after the first property is onboarded.</p></section>
        ) : (
          <div className="ops-table-wrap">
            <table className="ops-table">
              <thead><tr><th>NAME</th><th>ADDRESS</th><th>STATE</th><th>READINESS</th><th>ACTIVE PACKAGE</th><th>OPEN TICKETS</th><th><span className="sr-only">Open</span></th></tr></thead>
              <tbody>
                {properties.map((property) => (
                  <tr key={property.id}>
                    <td data-label="NAME"><strong>{propertyName(property)}</strong><small>{property.id}</small></td>
                    <td data-label="ADDRESS">{propertyAddress(property)}</td>
                    <td data-label="STATE"><StatusLabel status={property.data.state} /></td>
                    <td data-label="READINESS">
                      {isPropertyReady(property.data.readiness) ? (
                        <StatusLabel status="ready" />
                      ) : (
                        <span className="ops-readiness-pending" title="Missing: owner contract, compliance, or mandatory fields">PENDING</span>
                      )}
                    </td>
                    <td data-label="ACTIVE PACKAGE">—</td>
                    <td data-label="OPEN TICKETS">{openTicketsByProperty.get(property.id) ?? 0}</td>
                    <td className="ops-row-action"><Link aria-label={`Open ${propertyName(property)}`} to={`/properties/${property.id}`}>→</Link></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </main>
    </StaffGate>
  );
}
