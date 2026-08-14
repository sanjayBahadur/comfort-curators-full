import { api, type Envelope } from "./client";

// GET /v1/reports/property-contribution — all recorded activity for one
// property. The backend handler returns aggregate totals only; there is no
// reachable endpoint that itemizes individual charges or credits.
export type PropertyContributionReport = {
  property_id: string;
  total_charges_minor_units: number;
  total_credits_minor_units: number;
  net_minor_units: number;
  currency: string;
  charge_count: number;
  credit_count: number;
  subledger_entry_count: number;
};

// Mirrors internal/documents/models.go's Document — the wire shape for
// GET /v1/properties/{id}/documents and POST /v1/documents.
export type OwnerDocumentData = {
  id: string;
  tenant_id: string;
  property_id: string;
  title: string;
  document_type: string;
  status: string;
  expires_at?: string;
  current_version: number;
  version: number;
  created_at: string;
  updated_at: string;
};

type DocumentCollection = {
  items: Array<Envelope<OwnerDocumentData>> | null;
  next_cursor: string | null;
};

type ContributionWire =
  | PropertyContributionReport
  | { items: Array<Envelope<PropertyContributionReport> | PropertyContributionReport> | null };

function isContributionReport(value: unknown): value is PropertyContributionReport {
  return typeof value === "object" && value !== null && "total_charges_minor_units" in value;
}

// The report is scoped by query param, not path. INTEGRATION.md §10 records
// that the live service returns a one-item collection; the P5.4 block
// re-verified the handler and saw a flat report object. Accept both so the
// page keeps working across that transition.
export async function getPropertyContributionReport(propertyId: string): Promise<PropertyContributionReport | null> {
  const wire = await api<ContributionWire>(
    `/v1/reports/property-contribution?property_id=${encodeURIComponent(propertyId)}`,
  );
  if (isContributionReport(wire)) return wire;
  const first = wire.items?.[0];
  if (!first) return null;
  return isContributionReport(first) ? first : first.data;
}

export function getPropertyDocuments(propertyId: string) {
  return api<DocumentCollection>(`/v1/properties/${encodeURIComponent(propertyId)}/documents`);
}

// Upload is stubbed by spec — POST metadata only, no file body and no MinIO.
// handleCreateDocument (internal/documents/handler.go) only reads title,
// document_type, and property_id from the body -- expires_at isn't accepted
// at creation (verified against the live handler), so it's deliberately not
// a field here rather than a value the backend would silently drop.
export type CreateDocumentInput = {
  property_id: string;
  title: string;
  document_type: string;
};

export function createDocument(input: CreateDocumentInput) {
  return api<Envelope<OwnerDocumentData>>("/v1/documents", {
    method: "POST",
    body: JSON.stringify(input),
  });
}
