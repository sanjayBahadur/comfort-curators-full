import { api, type Envelope } from "./client";

export const TICKET_TYPES = [
  "turnover",
  "pre_arrival_inspection",
  "restock",
  "incident",
  "routine_maintenance",
  "specialist_vendor_request",
  "property_onboarding",
  "document_review",
  "inventory_count",
] as const;

export const TICKET_STATUSES = [
  "draft",
  "proposed",
  "approved",
  "scheduled",
  "assigned",
  "in_progress",
  "evidence_submitted",
  "verified",
  "closed",
  "blocked",
  "cancelled",
  "rejected",
] as const;

export type TicketType = (typeof TICKET_TYPES)[number];
export type TicketStatus = (typeof TICKET_STATUSES)[number];

export type Address = {
  line1: string;
  line2?: string;
  city: string;
  state: string;
  postal_code: string;
  country: string;
};

export type Readiness = {
  owner_contract_accepted: boolean;
  compliance_complete: boolean;
  mandatory_fields_set: boolean;
};

export type OpsPropertyData = {
  id: string;
  service_address: Address;
  geolocation_zone: string;
  timezone: string;
  state: string;
  readiness: Readiness;
  access_method?: string;
  emergency_contacts?: Array<{ name: string; phone: string; role?: string }>;
};

/** Mirrors the backend's Readiness.Ready() — all three inputs satisfied. */
export const isPropertyReady = (readiness: Readiness) =>
  readiness.owner_contract_accepted && readiness.compliance_complete && readiness.mandatory_fields_set;

export type RequestedWindow = { start: string; end: string };

export type TicketData = {
  id: string;
  tenant_id: string;
  property_id: string;
  type: TicketType;
  status: TicketStatus;
  reason: string;
  requested_window?: RequestedWindow;
  created_by: string;
  assigned_to?: string;
  version: number;
  created_at: string;
  updated_at: string;
};

export type WorkerData = {
  id: string;
  legal_name: string;
  classification: "employee" | "vendor";
  specialist: boolean;
  service_zone: string;
  skills: string[];
  status: string;
  verified_identity: boolean;
};

export type StateEventData = {
  id: string;
  ticket_id: string;
  from_state: string;
  to_state: string;
  actor_id: string;
  reason: string;
  evidence?: string[];
  version: number;
  created_at: string;
};

export type ChecklistItemData = {
  id: string;
  ticket_id: string;
  template_item_index: number;
  label: string;
  status: "pending" | "in_progress" | "completed" | "not_applicable";
  evidence_required?: true;
  completed_by?: string;
  completed_at?: string;
  evidence_ids?: string[];
  notes?: string;
};

export type EvidenceData = {
  id: string;
  ticket_id: string;
  content_hash: string;
  size_bytes: number;
  status: "accepted" | "rejected";
  captured_by: string;
  captured_at: string;
  checklist_item_id?: string;
  object_id?: string;
  file_name?: string;
  content_type?: string;
};

export type AssignmentData = {
  id: string;
  ticket_id: string;
  worker_id: string;
  assigned_by: string;
  status: "offered" | "accepted" | "declined" | "completed";
  accept_until?: string;
  accepted_at?: string;
  version: number;
  created_at: string;
  updated_at: string;
};

export type ConstraintCheck = {
  constraint: string;
  hard: boolean;
  passed: boolean;
  detail?: string;
};

export type DispatchCandidate = {
  worker_id: string;
  eligible: boolean;
  score: number;
  checks: ConstraintCheck[];
};

export type Collection<T> = { items: Array<Envelope<T>> | null; next_cursor: string | null };
export type PropertyCollection = { items: Array<Envelope<OpsPropertyData>>; next_cursor: string | null };
export type WorkerCollection = { items: Array<Envelope<WorkerData>>; total: number };

export type CalendarFeedData = {
  id: string;
  tenant_id: string;
  property_id: string;
  source: string;
  url: string;
  status: string;
  property_timezone: string;
  stale_after_minutes: number;
  minimum_turnaround_minutes: number;
  last_polled_at?: string;
  last_success_at?: string;
  last_content_hash?: string;
  last_error?: string;
  version: number;
  created_at: string;
};

export type FeedHealthData = {
  feed: CalendarFeedData;
  fresh: boolean;
  stale: boolean;
  stale_since?: string;
  last_success_at?: string;
  last_error?: string;
  open_exceptions: number;
};

export type ReservationData = {
  id: string;
  tenant_id: string;
  property_id: string;
  feed_id: string;
  external_event_id: string;
  source: string;
  guest_summary: string;
  status: string;
  start_at: string;
  end_at: string;
  all_day: boolean;
  timezone?: string;
  sequence: number;
  version: number;
  created_at: string;
};

export type TurnoverProposalData = {
  id: string;
  tenant_id: string;
  property_id: string;
  reservation_id: string;
  kind: string;
  status: string;
  scheduled_at: string;
  checklist_hint?: string;
  version: number;
  created_at: string;
  updated_at: string;
};

export type PollFeedResult = {
  feed_id: string;
  unchanged: boolean;
  events_parsed: number;
  events_skipped: number;
  events_created: number;
  events_updated: number;
  events_cancelled: number;
  exceptions_created: number;
  stale_feed_resolved: boolean;
};

export type ProposalGenerationResult = {
  proposed: number;
  updated: number;
  cancelled: number;
  skipped: boolean;
  reason?: string;
};

export type TicketQueueRow = Envelope<TicketData> & {
  property: Envelope<OpsPropertyData>;
  assignments: Array<Envelope<AssignmentData>>;
};

export type TicketQueue = {
  properties: Array<Envelope<OpsPropertyData>>;
  workers: Array<Envelope<WorkerData>>;
  tickets: TicketQueueRow[];
};

export type TicketDetailBundle = {
  ticket: Envelope<TicketData>;
  properties: Array<Envelope<OpsPropertyData>>;
  workers: Array<Envelope<WorkerData>>;
  events: Array<Envelope<StateEventData>>;
  checklist: Array<Envelope<ChecklistItemData>>;
  evidence: Array<Envelope<EvidenceData>>;
  assignments: Array<Envelope<AssignmentData>>;
};

const items = <T,>(collection: Collection<T>) => collection.items ?? [];

export const getOpsProperties = () => api<PropertyCollection>("/v1/properties");
export const getWorkers = () => api<WorkerCollection>("/v1/workers");
export const getCalendarHealth = (propertyId: string) =>
  api<{ feeds: FeedHealthData[] }>(`/v1/properties/${encodeURIComponent(propertyId)}/calendar-health`);
export const getReservations = (propertyId: string) =>
  api<{ items: Array<Envelope<ReservationData>> }>(`/v1/properties/${encodeURIComponent(propertyId)}/reservations`);
export const getTurnoverProposals = (propertyId: string) =>
  api<{ items: Array<Envelope<TurnoverProposalData>> }>(`/v1/properties/${encodeURIComponent(propertyId)}/turnover-proposals`);
export const pollFeed = (feedId: string) =>
  api<{ status: string; result: PollFeedResult }>(`/v1/calendar-feeds/${encodeURIComponent(feedId)}/polls`, { method: "POST" });
export const generateTurnoverProposals = (propertyId: string) =>
  api<{ result: ProposalGenerationResult }>(`/v1/properties/${encodeURIComponent(propertyId)}/turnover-proposals/generate`, { method: "POST" });

export async function getTicketQueue(): Promise<TicketQueue> {
  const [propertyCollection, workerCollection] = await Promise.all([
    getOpsProperties(),
    getWorkers(),
  ]);
  const properties = propertyCollection.items;
  const ticketCollections = await Promise.all(
    properties.map((property) =>
      api<Collection<TicketData>>(`/v1/tickets?property_id=${encodeURIComponent(property.id)}&limit=200`),
    ),
  );
  const ticketResources = ticketCollections.flatMap(items);
  const assignmentCollections = await Promise.all(
    ticketResources.map((ticket) =>
      api<Collection<AssignmentData>>(`/v1/tickets/${ticket.id}/dispatch/assignments`),
    ),
  );
  const propertyById = new Map(properties.map((property) => [property.id, property]));

  return {
    properties,
    workers: workerCollection.items,
    tickets: ticketResources.flatMap((ticket, index) => {
      const property = propertyById.get(ticket.data.property_id);
      return property
        ? [{ ...ticket, property, assignments: items(assignmentCollections[index]) }]
        : [];
    }),
  };
}

export async function getTicketDetail(ticketId: string): Promise<TicketDetailBundle> {
  const [ticket, properties, workers, events, checklist, evidence, assignments] = await Promise.all([
    api<Envelope<TicketData>>(`/v1/tickets/${ticketId}`),
    getOpsProperties(),
    getWorkers(),
    api<Collection<StateEventData>>(`/v1/tickets/${ticketId}/state-events`),
    api<Collection<ChecklistItemData>>(`/v1/tickets/${ticketId}/checklist-items`),
    api<Collection<EvidenceData>>(`/v1/tickets/${ticketId}/evidence`),
    api<Collection<AssignmentData>>(`/v1/tickets/${ticketId}/dispatch/assignments`),
  ]);

  return {
    ticket,
    properties: properties.items,
    workers: workers.items,
    events: items(events),
    checklist: items(checklist),
    evidence: items(evidence),
    assignments: items(assignments),
  };
}

export type CreateTicketInput = {
  property_id: string;
  type: TicketType;
  reason: string;
  requested_window: RequestedWindow;
};

export const createTicket = (input: CreateTicketInput) =>
  api<Envelope<TicketData>>("/v1/tickets", {
    method: "POST",
    body: JSON.stringify(input),
  });

export const transitionTicket = (
  ticketId: string,
  toState: string,
  reason = "Prepared for dispatch in the operations desk",
) =>
  api<Envelope<TicketData>>(`/v1/tickets/${ticketId}/transitions`, {
    method: "POST",
    body: JSON.stringify({
      to_state: toState,
      reason,
      evidence_ids: [],
    }),
  });

export const getDispatchCandidates = (ticketId: string) =>
  api<{ data: { ticket_id: string; candidates: DispatchCandidate[] | null } }>(
    `/v1/tickets/${ticketId}/dispatch/candidates`,
    { method: "POST", body: JSON.stringify({}) },
  );

export const assignWorker = (ticketId: string, workerId: string) =>
  api<Envelope<{ assignment: AssignmentData; pay_treatment: { role: string; worker_id: string; compensation_band?: string } }>>(
    `/v1/tickets/${ticketId}/dispatch/assign`,
    { method: "POST", body: JSON.stringify({ worker_id: workerId }) },
  );

export type ChecklistSyncItem = Pick<ChecklistItemData, "template_item_index" | "label" | "status"> & {
  completed_by?: string;
  evidence_ids?: string[];
  evidence_required?: boolean;
  notes?: string;
};

export const syncChecklist = (ticketId: string, items: ChecklistSyncItem[]) =>
  api<Collection<ChecklistItemData>>(`/v1/tickets/${ticketId}/checklist-syncs`, {
    method: "POST",
    body: JSON.stringify({ items }),
  });

export const registerTicketEvidence = (ticketId: string, input: {
  checklist_item_id?: string;
  object_id?: string;
  content_hash: string;
  file_name?: string;
  content_type?: string;
  size_bytes: number;
}) => api<Envelope<EvidenceData>>(`/v1/tickets/${ticketId}/evidence`, {
  method: "POST",
  body: JSON.stringify(input),
});

export async function getCuratorJobs(): Promise<TicketQueue> {
  const queue = await getTicketQueue();
  return {
    ...queue,
    tickets: queue.tickets
      .filter((ticket) => ticket.assignments.some((assignment) => assignment.data.status !== "declined"))
      .filter((ticket) => !["closed", "cancelled", "rejected", "verified"].includes(ticket.data.status))
      .sort((left, right) => {
        const leftStart = left.data.requested_window?.start ?? left.data.created_at;
        const rightStart = right.data.requested_window?.start ?? right.data.created_at;
        return new Date(leftStart).getTime() - new Date(rightStart).getTime();
      }),
  };
}

export async function sha256Hex(value: string) {
  const bytes = new TextEncoder().encode(value);
  const digest = await crypto.subtle.digest("SHA-256", bytes);
  return Array.from(new Uint8Array(digest), (byte) => byte.toString(16).padStart(2, "0")).join("");
}
