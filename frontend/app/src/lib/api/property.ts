import { api, type Envelope } from "./client";

export type PropertyAddress = {
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

export type EmergencyContact = {
  name: string;
  phone: string;
  role?: string;
};

// Matches internal/property/handler.go's holdMap exactly -- the backend's
// ComplianceHold has no code/message fields (an earlier guess, made without
// backend access); it's kind/severity/status/reason plus optional timestamps.
export type ComplianceHold = {
  id: string;
  property_id: string;
  kind: string;
  severity: string;
  status: string;
  reason: string;
  created_at: string;
  expires_at?: string;
  exception_by?: string;
  exception_at?: string;
  exception_expires_at?: string;
  resolved_at?: string;
};

export type PropertyData = {
  service_address: PropertyAddress;
  geolocation_zone: string;
  timezone: string;
  emergency_contacts?: EmergencyContact[];
  access_method?: string;
  maximum_occupancy: number;
  state: string;
  readiness: Readiness;
  compliance_holds?: ComplianceHold[];
};

export type PropertyTransition = {
  property_id: string;
  from_state: string;
  to_state: string;
  actor_id: string;
  reason: string;
  evidence_ids?: string[];
  created_at: string;
};

type Collection<T> = { items: Array<Envelope<T>> | null; next_cursor?: string | null };

export type PropertyDetail = {
  property: Envelope<PropertyData>;
  transitions: Array<Envelope<PropertyTransition>>;
};

const items = <T,>(collection: Collection<T>) => collection.items ?? [];

export async function getPropertyDetail(propertyId: string): Promise<PropertyDetail> {
  const encodedId = encodeURIComponent(propertyId);
  const [property, transitionCollection] = await Promise.all([
    api<Envelope<PropertyData>>(`/v1/properties/${encodedId}`),
    api<Collection<PropertyTransition>>(`/v1/properties/${encodedId}/transitions`),
  ]);
  return { property, transitions: items(transitionCollection) };
}
