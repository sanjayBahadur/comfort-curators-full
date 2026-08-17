import { type FormEvent, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, useSearchParams } from "react-router-dom";
import { getTicketQueue, TICKET_STATUSES, TICKET_TYPES } from "../lib/api/ops";
import {
  OpsHeader,
  OpsSkeleton,
  StaffGate,
  StatusLabel,
} from "./ops-shared";
import {
  ageLabel,
  formatWindow,
  humanize,
  propertyName,
  workerName,
} from "./ops-format";
import Select from "../components/ui/Select";
import { useAgentSurface } from "../components/agent-surface/context";
import type { AgentAction } from "../components/agent-surface/types";
import "./ops.css";

const TYPE_ALIASES: Partial<Record<(typeof TICKET_TYPES)[number], string[]>> = {
  turnover: ["turnover", "checkout", "check out", "cleaning"],
  pre_arrival_inspection: ["pre arrival", "pre-arrival", "inspection"],
  restock: ["restock", "restocking", "supplies", "inventory refill"],
  incident: ["incident", "issue", "emergency"],
  routine_maintenance: ["routine maintenance", "maintenance", "repair"],
  specialist_vendor_request: ["specialist", "vendor"],
  property_onboarding: ["property onboarding", "onboarding"],
  document_review: ["document review", "documents"],
  inventory_count: ["inventory count", "stock count"],
};

function normalizeFilterPrompt(value: string) {
  return value.toLocaleLowerCase("en-IN").replaceAll(/[^a-z0-9]+/g, " ").trim();
}

function startOfDay(date: Date) {
  const copy = new Date(date);
  copy.setHours(0, 0, 0, 0);
  return copy;
}

function addDays(date: Date, days: number) {
  const copy = new Date(date);
  copy.setDate(copy.getDate() + days);
  return copy;
}

// Real date-phrase parsing for the local NL filter, evaluated against the
// browser's current time -- "this week" (Monday through the following
// Monday), "today", "tomorrow" and "next N days" all become a real
// [after, before) window applied to a ticket's requested window, not just
// a phrase the interpreter silently drops. Kept separate from
// applyNaturalLanguageFilter so it stays a pure, testable parse.
function parseDateWindow(prompt: string): { after: Date; before: Date; label: string } | null {
  const now = new Date();
  if (/\btoday\b/.test(prompt)) {
    const start = startOfDay(now);
    return { after: start, before: addDays(start, 1), label: "today" };
  }
  if (/\btomorrow\b/.test(prompt)) {
    const start = addDays(startOfDay(now), 1);
    return { after: start, before: addDays(start, 1), label: "tomorrow" };
  }
  const nextDays = prompt.match(/\bnext (\d+) days?\b/);
  if (nextDays) {
    const days = Number(nextDays[1]);
    return { after: now, before: addDays(now, days), label: `next ${days} days` };
  }
  if (/\bnext week\b/.test(prompt)) {
    const mondayOffset = (now.getDay() + 6) % 7;
    const thisMonday = startOfDay(addDays(now, -mondayOffset));
    const nextMonday = addDays(thisMonday, 7);
    return { after: nextMonday, before: addDays(nextMonday, 7), label: "next week" };
  }
  if (/\bthis week\b/.test(prompt)) {
    const mondayOffset = (now.getDay() + 6) % 7;
    const monday = startOfDay(addDays(now, -mondayOffset));
    return { after: monday, before: addDays(monday, 7), label: "this week" };
  }
  return null;
}

export default function OpsTickets() {
  const [searchParams, setSearchParams] = useSearchParams();
  const queueQuery = useQuery({ queryKey: ["ops", "ticket-queue"], queryFn: getTicketQueue });
  const propertyFilter = searchParams.get("property") ?? "";
  const typeFilter = searchParams.get("type") ?? "";
  const statusFilter = searchParams.get("status") ?? "";
  const afterFilter = searchParams.get("after") ?? "";
  const beforeFilter = searchParams.get("before") ?? "";
  const [filterPrompt, setFilterPrompt] = useState("");
  const [filterReply, setFilterReply] = useState("Try: assigned turnovers at Gomti Riverside this week");
  const workers = queueQuery.data?.workers ?? [];
  const newTicketSurface = useAgentSurface("ops-tickets-new-ticket", ["focus", "click", "scroll_to"] as AgentAction[], "Open the new ticket form");
  const propertyFilterSurface = useAgentSurface("ops-tickets-property-filter", ["scroll_to", "open_panel"] as AgentAction[], "Property ticket filter", () => document.getElementById("ops-tickets-property-filter-trigger")?.click());
  const typeFilterSurface = useAgentSurface("ops-tickets-type-filter", ["scroll_to", "open_panel"] as AgentAction[], "Ticket type filter", () => document.getElementById("ops-tickets-type-filter-trigger")?.click());
  const statusFilterSurface = useAgentSurface("ops-tickets-status-filter", ["scroll_to", "open_panel"] as AgentAction[], "Ticket status filter", () => document.getElementById("ops-tickets-status-filter-trigger")?.click());
  // The reliable path for a spoken-language queue search: type the exact
  // request into this box and submit it, and it runs through the same
  // deterministic interpreter a human typing here gets -- not the model
  // guessing at three separate dropdown option labels blind.
  const nlFilterInputSurface = useAgentSurface("ops-tickets-nl-filter-input", ["focus", "set"] as AgentAction[], "Natural-language ticket queue search box -- type a property, ticket type, status, or time window (e.g. \"this week\") here");
  const nlFilterSubmitSurface = useAgentSurface("ops-tickets-nl-filter-submit", ["click"] as AgentAction[], "Submit the natural-language ticket queue search typed above");

  const filteredTickets = useMemo(() => {
    const tickets = queueQuery.data?.tickets ?? [];
    const after = afterFilter ? new Date(afterFilter) : null;
    const before = beforeFilter ? new Date(beforeFilter) : null;
    return tickets
      .filter((ticket) => !propertyFilter || ticket.data.property_id === propertyFilter)
      .filter((ticket) => !typeFilter || ticket.data.type === typeFilter)
      .filter((ticket) => !statusFilter || ticket.data.status === statusFilter)
      .filter((ticket) => {
        if (!after && !before) return true;
        const when = new Date(ticket.data.requested_window?.start ?? ticket.data.created_at);
        return (!after || when >= after) && (!before || when < before);
      })
      .sort((left, right) =>
        (left.data.requested_window?.start ?? left.data.created_at).localeCompare(
          right.data.requested_window?.start ?? right.data.created_at,
        ),
      );
  }, [afterFilter, beforeFilter, propertyFilter, queueQuery.data?.tickets, statusFilter, typeFilter]);

  function setFilter(name: string, value: string) {
    setSearchParams((current) => {
      const next = new URLSearchParams(current);
      if (value) next.set(name, value);
      else next.delete(name);
      return next;
    }, { replace: true });
  }

  function applyNaturalLanguageFilter(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const prompt = normalizeFilterPrompt(filterPrompt);
    if (!prompt) {
      setFilterReply("Enter a property, ticket type, or status to filter the queue.");
      return;
    }

    if (/\b(clear|reset|everything|all tickets)\b/.test(prompt)) {
      setSearchParams({}, { replace: true });
      setFilterReply("Queue restored · showing all tickets.");
      setFilterPrompt("");
      return;
    }

    // Whole-word match against the prompt's own tokens, not a raw substring
    // search -- "villa" is literally the first five letters of "village",
    // so a naive prompt.includes(token) check let "gomti village" resolve
    // to Indira Nagar Villa instead of Gomti Riverside. promptWords is an
    // exact-token set, so only a genuine whole word matches.
    const promptWords = new Set(prompt.split(" ").filter(Boolean));
    const properties = queueQuery.data?.properties ?? [];
    const property = properties.find((entry) => {
      const name = normalizeFilterPrompt(propertyName(entry));
      const address = normalizeFilterPrompt(entry.data.service_address.line1);
      const significantTokens = name.split(" ").filter((token) => token.length > 3);
      return prompt.includes(name) || prompt.includes(address) || significantTokens.some((token) => promptWords.has(token));
    });
    const type = TICKET_TYPES.find((entry) => (TYPE_ALIASES[entry] ?? [humanize(entry)]).some((alias) => prompt.includes(normalizeFilterPrompt(alias))));
    const status = [...TICKET_STATUSES]
      .sort((left, right) => humanize(right).length - humanize(left).length)
      .find((entry) => prompt.includes(normalizeFilterPrompt(humanize(entry))));
    const dateWindow = parseDateWindow(prompt);

    if (!property && !type && !status && !dateWindow) {
      setFilterReply("No supported filter found · try a property name, ticket type, status, or a time window like \"this week\".");
      return;
    }

    const next = new URLSearchParams();
    if (property) next.set("property", property.id);
    if (type) next.set("type", type);
    if (status) next.set("status", status);
    if (dateWindow) {
      next.set("after", dateWindow.after.toISOString());
      next.set("before", dateWindow.before.toISOString());
    }
    setSearchParams(next, { replace: true });

    const applied = [
      property ? propertyName(property) : null,
      type ? humanize(type) : null,
      status ? humanize(status) : null,
      dateWindow ? dateWindow.label : null,
    ].filter(Boolean);
    setFilterReply(`Applied · ${applied.join(" / ")} · queue filters synchronized`);
  }

  return (
    <StaffGate>
      <main className="ops-shell registration-frame">
        <OpsHeader section="01 / TICKET QUEUE" action={<Link ref={newTicketSurface.ref} to="/ops/tickets/new">NEW TICKET</Link>} />
        <section className="ops-title-row">
          <div>
            <p>WORK ARRIVED. WHO IS DOING IT?</p>
            <h1>Ticket queue</h1>
          </div>
          <strong>{queueQuery.data?.tickets.length ?? "—"} TOTAL</strong>
        </section>

        <section className="ops-filter-bar" aria-label="Ticket filters">
          <label ref={propertyFilterSurface.ref}>
            <span>PROPERTY</span>
             <Select id="ops-tickets-property-filter-trigger" value={propertyFilter} onChange={(value) => setFilter("property", value)} options={[{ value: "", label: "ALL PROPERTIES" }, ...(queueQuery.data?.properties.map((property) => ({ value: property.id, label: propertyName(property) })) ?? [])]} />
          </label>
          <label ref={typeFilterSurface.ref}>
            <span>TYPE</span>
             <Select id="ops-tickets-type-filter-trigger" value={typeFilter} onChange={(value) => setFilter("type", value)} options={[{ value: "", label: "ALL TYPES" }, ...TICKET_TYPES.map((type) => ({ value: type, label: humanize(type).toUpperCase() }))]} />
          </label>
          <label ref={statusFilterSurface.ref}>
            <span>STATUS</span>
             <Select id="ops-tickets-status-filter-trigger" value={statusFilter} onChange={(value) => setFilter("status", value)} options={[{ value: "", label: "ALL STATUSES" }, ...TICKET_STATUSES.map((status) => ({ value: status, label: humanize(status).toUpperCase() }))]} />
          </label>
          {(propertyFilter || typeFilter || statusFilter || afterFilter || beforeFilter) && (
            <button type="button" onClick={() => setSearchParams({}, { replace: true })}>CLEAR FILTERS ×</button>
          )}
        </section>

        <section className="ops-filter-terminal" aria-label="Natural language ticket filters">
          <header><span>SUPERHOST / QUEUE FINDER</span><small>LOCAL FILTER INTERPRETER</small></header>
          <form onSubmit={applyNaturalLanguageFilter}>
            <label htmlFor="ops-filter-prompt" className="sr-only">Describe the tickets to find</label>
            <span aria-hidden="true">›</span>
            <input ref={nlFilterInputSurface.ref} id="ops-filter-prompt" type="text" value={filterPrompt} onChange={(event) => setFilterPrompt(event.currentTarget.value)} placeholder="Use human language to search" autoComplete="off" />
            <button ref={nlFilterSubmitSurface.ref} type="submit">FILTER →</button>
          </form>
          <p aria-live="polite">· {filterReply}</p>
        </section>

        {queueQuery.isLoading ? <OpsSkeleton rows={6} /> : queueQuery.isError ? (
          <section className="ops-empty"><strong>Queue unavailable.</strong><p>The backend message is shown in the toast. Retry when the service is ready.</p><button type="button" onClick={() => void queueQuery.refetch()}>RETRY</button></section>
        ) : filteredTickets.length === 0 ? (
          <section className="ops-empty">
            <strong>{queueQuery.data?.tickets.length ? "No tickets match." : "No tickets."}</strong>
            <p>{queueQuery.data?.tickets.length ? "Clear the filters to return to the full operating queue." : "Create one, or generate turnover proposals from a reservation."}</p>
            <Link to="/ops/tickets/new">CREATE A TICKET →</Link>
          </section>
        ) : (
          <div className="ops-table-wrap">
            <table className="ops-table">
              <thead><tr><th>TYPE</th><th>PROPERTY</th><th>REQUESTED WINDOW</th><th>STATUS</th><th>ASSIGNEE</th><th>AGE</th><th><span className="sr-only">Open</span></th></tr></thead>
              <tbody>
                {filteredTickets.map((ticket) => {
                  const assignment = ticket.assignments.find((entry) => entry.data.status !== "declined") ?? ticket.assignments.at(-1);
                  return (
                    <tr key={ticket.id}>
                      <td data-label="TYPE"><strong>{humanize(ticket.data.type)}</strong><small>{ticket.id}</small></td>
                      <td data-label="PROPERTY">{propertyName(ticket.property)}</td>
                      <td data-label="WINDOW">{formatWindow(ticket.data.requested_window)}</td>
                      <td data-label="STATUS"><StatusLabel status={ticket.data.status} /></td>
                      <td data-label="ASSIGNEE">{workerName(assignment?.data.worker_id, workers)}</td>
                      <td data-label="AGE">{ageLabel(ticket.data.created_at)}</td>
                      <td className="ops-row-action"><Link aria-label={`Open ${humanize(ticket.data.type)} ticket`} to={`/ops/tickets/${ticket.id}`}>→</Link></td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </main>
    </StaffGate>
  );
}
