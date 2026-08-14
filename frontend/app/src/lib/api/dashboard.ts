import { api, type Envelope } from "./client";
import type { TicketData } from "./ops";
import type { PackageVersion } from "./shop";

export type Readiness = {
  owner_contract_accepted: boolean;
  compliance_complete: boolean;
  mandatory_fields_set: boolean;
};

export type DashboardPropertyData = {
  service_address: {
    line1: string;
    line2?: string;
    city: string;
    state: string;
    postal_code: string;
    country: string;
  };
  timezone: string;
  state: string;
  readiness: Readiness;
};

export type OwnerExceptionData = {
  source: "incident" | "service_recovery" | "financial" | string;
  source_id: string;
  property_id: string;
  label: string;
  summary: string;
  severity?: string;
  status: string;
  occurred_at: string;
  owner_visible: boolean;
};

export type ContributionData = {
  revenue_minor_units: number;
  supply_margin_minor_units: number;
  vendor_cost_minor_units: number;
  refund_minor_units: number;
  exception_cost_minor_units: number;
  discount_minor_units: number;
  tax_minor_units: number;
  net_contribution_minor_units: number;
  currency: string;
};

export type DocumentData = {
  id: string;
  property_id: string;
  title: string;
  document_type: string;
  status: "draft" | "active" | "expired" | "revoked" | "superseded" | string;
  expires_at?: string;
  current_version: number;
  created_at: string;
  updated_at: string;
};

type Collection<T> = { items: Array<Envelope<T>> | null; next_cursor?: string | null };

export type DashboardProperty = {
  property: Envelope<DashboardPropertyData>;
  packages: Array<Envelope<PackageVersion>>;
  activePackage?: Envelope<PackageVersion>;
  tickets: Array<Envelope<TicketData>>;
  documents: Array<Envelope<DocumentData>>;
  contribution: Envelope<ContributionData>;
};

export type OwnerDashboardData = {
  properties: DashboardProperty[];
  exceptions: Array<Envelope<OwnerExceptionData>>;
  period: { start: string; end: string };
};

const items = <T,>(collection: Collection<T>) => collection.items ?? [];

function currentMonth() {
  const now = new Date();
  const start = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), 1));
  const end = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth() + 1, 1));
  return { start: start.toISOString(), end: end.toISOString() };
}

async function getPropertyDashboard(
  property: Envelope<DashboardPropertyData>,
  period: { start: string; end: string },
): Promise<DashboardProperty> {
  const propertyId = encodeURIComponent(property.id);
  const [packageCollection, ticketCollection, documentCollection, contribution] = await Promise.all([
    api<Collection<PackageVersion>>(`/v1/properties/${propertyId}/packages`),
    api<Collection<TicketData>>(`/v1/tickets?property_id=${propertyId}&limit=200`),
    api<Collection<DocumentData>>(`/v1/properties/${propertyId}/documents?limit=200`),
    api<Envelope<ContributionData>>(
      `/v1/reporting/property-contribution?property_id=${propertyId}&period_start=${encodeURIComponent(period.start)}&period_end=${encodeURIComponent(period.end)}`,
    ),
  ]);
  const packages = items(packageCollection);
  const activePackage = packages
    .filter((version) => version.data.status === "active")
    .sort((left, right) => right.data.version_number - left.data.version_number)[0];

  return {
    property,
    packages,
    activePackage,
    tickets: items(ticketCollection),
    documents: items(documentCollection),
    contribution,
  };
}

export async function getOwnerDashboard(): Promise<OwnerDashboardData> {
  const period = currentMonth();
  const [propertyCollection, exceptionCollection] = await Promise.all([
    api<Collection<DashboardPropertyData>>("/v1/properties"),
    api<Collection<OwnerExceptionData>>("/v1/reporting/owner-exceptions"),
  ]);
  const properties = items(propertyCollection);
  const visibleIds = new Set(properties.map((property) => property.id));

  return {
    properties: await Promise.all(properties.map((property) => getPropertyDashboard(property, period))),
    exceptions: items(exceptionCollection).filter((exception) =>
      exception.data.owner_visible && visibleIds.has(exception.data.property_id),
    ),
    period,
  };
}
