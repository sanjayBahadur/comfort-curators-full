import { api, type Envelope } from "./client";
import type { PackageVersion } from "./shop";

export type OwnerPropertyData = {
  owner_authority_id: string;
  service_address: {
    line1: string;
    line2?: string;
    city: string;
    state: string;
    postal_code: string;
    country: string;
  };
  timezone: string;
  maximum_occupancy: number;
  state: string;
  readiness: {
    owner_contract_accepted: boolean;
    compliance_complete: boolean;
    mandatory_fields_set: boolean;
  };
};

export type OnboardingContact = {
  name: string;
  role?: string;
  phone: string;
  email?: string;
};

export type OnboardingEvidence = {
  id: string;
  kind: "legal" | "safety" | "document" | string;
  content_hash: string;
  object_ref: string;
  captured_at: string;
};

export type OnboardingInspection = {
  id: string;
  inspected_by: string;
  evidence_ref: string;
  findings: string;
  overall_status: string;
  performed_at: string;
};

export type PortfolioSection = {
  property_name: string;
  property_type: string;
  purchase_year: number;
  managed_units: number;
  primary_residence: boolean;
};

export type GoalsSection = {
  primary_goal: string;
  secondary_goals?: string[];
  rental_strategy: string;
  occupancy_target?: number;
};

export type ServicePreferencesSection = {
  furnishing_preference: string;
  communication_channel: string;
  service_language: string;
  guest_access_policy: string;
  approval_threshold_minor_units: number;
  currency: string;
  automation_limits?: string[];
};

export type BudgetsSection = {
  monthly_budget_minor_units: number;
  setup_budget_minor_units: number;
  renovation_budget_minor_units: number;
  currency: string;
  overspend_policy: string;
};

export type OnboardingCaseData = {
  tenant_id: string;
  property_id: string;
  owner_authority_id: string;
  status: "in_progress" | "ready" | "activated";
  portfolio?: PortfolioSection | null;
  goals?: GoalsSection | null;
  service_preferences?: ServicePreferencesSection | null;
  budgets?: BudgetsSection | null;
  contacts: OnboardingContact[] | null;
  photographs: Array<{ object_ref: string; caption?: string; captured_at: string }> | null;
  amenities: Array<{ name: string; quantity: number }> | null;
  safety?: {
    smoke_detectors_installed: boolean;
    fire_extinguisher_present: boolean;
    gas_leak_check_done: boolean;
    electrical_safety_certified: boolean;
    notes?: string;
  } | null;
  furnishing?: { furnishing_level: string; inventory_count: number; notes?: string } | null;
  remediation?: {
    open_items?: Array<{ description: string; resolved: boolean }>;
    completed_items?: Array<{ description: string; resolved: boolean }>;
  } | null;
  fit_score_inputs?: {
    property_score: number;
    market_score: number;
    operations_score: number;
    renovation_score: number;
    occupancy_score: number;
  } | null;
  evidence: OnboardingEvidence[] | null;
  inspections: OnboardingInspection[] | null;
  activation_holds: Array<{ code: string; message: string }> | null;
  created_at: string;
  updated_at: string;
};

export type OnboardingProgress = {
  progress: Array<{ key: string; complete: boolean }>;
};

export type AgreementData = {
  property_id: string;
  status: "draft" | "accepted";
  current_version: number;
  versions: Array<{
    id: string;
    version_number: number;
    content_hash: string;
    terms: Record<string, unknown>;
    created_at: string;
  }>;
  acceptance?: {
    accepted_by: string;
    accepted_at: string;
    content_hash: string;
    version_number: number;
  };
};

type Collection<T> = { items: Array<Envelope<T>> | null; next_cursor?: string | null };
type PackageCollection = { items: Array<Envelope<PackageVersion>> | null; total?: number };

export type OnboardingBootstrap = {
  properties: Array<Envelope<OwnerPropertyData>>;
  cases: Array<Envelope<OnboardingCaseData>>;
};

export type OnboardingWorkspace = {
  onboardingCase: Envelope<OnboardingCaseData>;
  progress: OnboardingProgress;
  packages: Array<Envelope<PackageVersion>>;
  agreements: Array<Envelope<AgreementData>>;
};

export type NewOwnerPropertyInput = {
  owner_authority_id: string;
  service_address: OwnerPropertyData["service_address"];
  timezone: string;
  access_method: string;
  status: "lead";
  maximum_occupancy: number;
};

const items = <T,>(collection: Collection<T>) => collection.items ?? [];

export async function getOnboardingBootstrap(): Promise<OnboardingBootstrap> {
  const [propertyCollection, caseCollection] = await Promise.all([
    api<Collection<OwnerPropertyData>>("/v1/properties?limit=200"),
    api<Collection<OnboardingCaseData>>("/v1/onboarding/cases"),
  ]);
  const properties = items(propertyCollection);
  const visiblePropertyIds = new Set(properties.map((property) => property.id));
  return {
    properties,
    cases: items(caseCollection).filter((entry) => visiblePropertyIds.has(entry.data.property_id)),
  };
}

export async function startOnboardingCase(property: Envelope<OwnerPropertyData>) {
  return api<Envelope<OnboardingCaseData>>("/v1/owners/onboarding-cases", {
    method: "POST",
    body: JSON.stringify({
      property_id: property.id,
      owner_authority_id: property.data.owner_authority_id,
    }),
  });
}

export function createOwnerProperty(input: NewOwnerPropertyInput) {
  return api<Envelope<OwnerPropertyData>>("/v1/properties", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export async function getOnboardingWorkspace(caseId: string): Promise<OnboardingWorkspace> {
  const encodedCaseId = encodeURIComponent(caseId);
  const onboardingCase = await api<Envelope<OnboardingCaseData>>(`/v1/onboarding/cases/${encodedCaseId}`);
  const propertyId = encodeURIComponent(onboardingCase.data.property_id);
  const [progress, packageCollection, agreementCollection] = await Promise.all([
    api<OnboardingProgress>(`/v1/onboarding/cases/${encodedCaseId}/progress`),
    api<PackageCollection>(`/v1/properties/${propertyId}/packages`),
    api<Collection<AgreementData>>("/v1/contracts/agreements"),
  ]);
  return {
    onboardingCase,
    progress,
    packages: packageCollection.items ?? [],
    agreements: items(agreementCollection).filter((agreement) => agreement.data.property_id === onboardingCase.data.property_id),
  };
}

export function saveOnboardingSection<T>(caseId: string, section: string, payload: T) {
  return api<Envelope<OnboardingCaseData>>(
    `/v1/onboarding/cases/${encodeURIComponent(caseId)}/sections/${encodeURIComponent(section)}`,
    { method: "PUT", body: JSON.stringify({ payload }) },
  );
}

export function saveOnboardingContacts(caseId: string, contacts: OnboardingContact[]) {
  return api<Envelope<OnboardingCaseData>>(`/v1/onboarding/cases/${encodeURIComponent(caseId)}/contacts`, {
    method: "PUT",
    body: JSON.stringify({ contacts }),
  });
}

export function recordOnboardingEvidence(
  caseId: string,
  input: { kind: "legal" | "safety" | "document"; content_hash: string; object_ref: string },
) {
  return api<Envelope<OnboardingCaseData>>(`/v1/onboarding/cases/${encodeURIComponent(caseId)}/evidence`, {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function recordOnboardingInspection(
  caseId: string,
  input: {
    property_id: string;
    inspected_by: string;
    evidence_hash: string;
    evidence_ref: string;
    findings: string;
    overall_status: string;
  },
) {
  return api<Envelope<OnboardingInspection>>(`/v1/onboarding/cases/${encodeURIComponent(caseId)}/inspections`, {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function acceptAgreement(agreementId: string) {
  return api<Envelope<AgreementData>>(`/v1/contracts/agreements/${encodeURIComponent(agreementId)}/accept`, {
    method: "POST",
  });
}
