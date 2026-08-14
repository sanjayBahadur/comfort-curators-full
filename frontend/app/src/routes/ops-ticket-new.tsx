import { useEffect, useMemo, useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "react-router-dom";
import { toast } from "sonner";
import { createTicket, getOpsProperties, TICKET_TYPES, type TicketType } from "../lib/api/ops";
import { OpsHeader, OpsSkeleton, StaffGate } from "./ops-shared";
import { humanize, propertyName } from "./ops-format";
import Select from "../components/ui/Select";
import "./ops.css";

function localInputValue(date: Date) {
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60_000);
  return local.toISOString().slice(0, 16);
}

const startDefault = new Date(Date.now() + 86_400_000);
startDefault.setHours(10, 0, 0, 0);
const endDefault = new Date(startDefault.getTime() + 3 * 60 * 60_000);

export default function OpsTicketNew() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const propertiesQuery = useQuery({ queryKey: ["ops", "properties"], queryFn: getOpsProperties });
  const [propertyId, setPropertyId] = useState("");
  const [type, setType] = useState<TicketType>("turnover");
  const [start, setStart] = useState(() => localInputValue(startDefault));
  const [end, setEnd] = useState(() => localInputValue(endDefault));
  const [reason, setReason] = useState("");
  const properties = useMemo(() => propertiesQuery.data?.items ?? [], [propertiesQuery.data?.items]);

  useEffect(() => {
    if (!propertyId && properties[0]) setPropertyId(properties[0].id);
  }, [properties, propertyId]);

  const createMutation = useMutation({
    mutationFn: createTicket,
    onSuccess: async (ticket) => {
      await queryClient.invalidateQueries({ queryKey: ["ops", "ticket-queue"] });
      toast.success("Ticket created in draft");
      navigate(`/ops/tickets/${ticket.id}`);
    },
  });

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const startDate = new Date(start);
    const endDate = new Date(end);
    if (!propertyId) return toast.error("Choose a property");
    if (!reason.trim()) return toast.error("Reason is required");
    if (!Number.isFinite(startDate.getTime()) || !Number.isFinite(endDate.getTime()) || endDate <= startDate) {
      return toast.error("Requested window must end after it starts");
    }
    createMutation.mutate({
      property_id: propertyId,
      type,
      reason: reason.trim(),
      requested_window: { start: startDate.toISOString(), end: endDate.toISOString() },
    });
  }

  return (
    <StaffGate>
      <main className="ops-shell registration-frame">
        <OpsHeader section="02 / CREATE TICKET" />
        <section className="ops-title-row ops-title-row--form">
          <div><p>NEW WORK / REAL BACKEND RECORD</p><h1>Create ticket</h1></div>
          <Link to="/ops/tickets">← BACK TO QUEUE</Link>
        </section>

        {propertiesQuery.isLoading ? <OpsSkeleton rows={4} /> : propertiesQuery.isError ? (
          <section className="ops-empty"><strong>Properties unavailable.</strong><button type="button" onClick={() => void propertiesQuery.refetch()}>RETRY</button></section>
        ) : properties.length === 0 ? (
          <section className="ops-empty"><strong>No active operating context.</strong><p>Create and activate a property before opening work.</p></section>
        ) : (
          <form className="ops-form" onSubmit={submit}>
            <div className="ops-form-intro">
              <span>01</span>
              <div><strong>Describe the work.</strong><p>The ticket begins in DRAFT. Schedule it from the detail screen before assignment.</p></div>
            </div>
            <label className="ops-field ops-field--wide">
              <span>PROPERTY</span>
               <Select value={propertyId} onChange={setPropertyId} required options={properties.map((property) => ({ value: property.id, label: propertyName(property) }))} />
            </label>
            <label className="ops-field ops-field--wide">
              <span>TYPE</span>
               <Select value={type} onChange={(value) => setType(value as TicketType)} required options={TICKET_TYPES.map((entry) => ({ value: entry, label: humanize(entry).toUpperCase() }))} />
            </label>
            <label className="ops-field">
              <span>WINDOW START</span>
              <input type="datetime-local" value={start} onChange={(event) => setStart(event.currentTarget.value)} required />
            </label>
            <label className="ops-field">
              <span>WINDOW END</span>
              <input type="datetime-local" value={end} onChange={(event) => setEnd(event.currentTarget.value)} required />
            </label>
            <label className="ops-field ops-field--wide">
              <span>REASON</span>
              <textarea value={reason} onChange={(event) => setReason(event.currentTarget.value)} rows={5} maxLength={500} placeholder="What happened, what is needed, and what outcome is expected?" required />
              <small>{reason.length} / 500</small>
            </label>
            <footer className="ops-form-actions">
              <Link to="/ops/tickets">CANCEL</Link>
              <button type="submit" disabled={createMutation.isPending}>{createMutation.isPending ? "CREATING…" : "CREATE DRAFT →"}</button>
            </footer>
          </form>
        )}
      </main>
    </StaffGate>
  );
}
