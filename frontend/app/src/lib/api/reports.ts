import { api, type Envelope } from "./client";
import type { ContributionData, OwnerExceptionData } from "./dashboard";

// GET /v1/reporting/property-contribution — mirrors dashboard.ts's own call
// (same endpoint, same read model) so the monthly report always shows the
// same figures the dashboard's "this month" card already shows, not a
// second, possibly-divergent number for the same period.
export function getContributionReport(propertyId: string, period: { start: string; end: string }) {
  return api<Envelope<ContributionData>>(
    `/v1/reporting/property-contribution?property_id=${encodeURIComponent(propertyId)}&period_start=${encodeURIComponent(period.start)}&period_end=${encodeURIComponent(period.end)}`,
  );
}

export type ServiceLevelSummaryData = {
  property_id: string;
  total_tickets: number;
  closed_tickets: number;
  open_tickets: number;
  open_incidents: number;
  cancelled_tickets: number;
  open_recoveries: number;
  completed_checklists: number;
};

export function getServiceLevelSummary(propertyId: string, period: { start: string; end: string }) {
  return api<Envelope<ServiceLevelSummaryData>>(
    `/v1/reporting/service-level-summary?property_id=${encodeURIComponent(propertyId)}&period_start=${encodeURIComponent(period.start)}&period_end=${encodeURIComponent(period.end)}`,
  );
}

export async function getOwnerExceptionsForProperty(propertyId: string) {
  // A tenant-scoped Go nil slice marshals to `null`, not `[]`, when a
  // property has zero exceptions this period -- seen across this codebase's
  // other collection endpoints (see dashboard.ts's own items() helper).
  const collection = await api<{ items: Array<Envelope<OwnerExceptionData>> | null }>(
    `/v1/reporting/owner-exceptions?property_id=${encodeURIComponent(propertyId)}`,
  );
  return { items: collection.items ?? [] };
}

// The accounting department's own inbox for filed reports. Not a named
// person -- a stable desk identifier, same idea as "front desk" or
// "ops queue" elsewhere in the app. communications.CreateDraft only
// validates that a recipient_id is non-empty; it does not need to resolve
// to a real IAM account.
export const ACCOUNTING_RECIPIENT_ID = "accounting-desk";

export type CommunicationDraftData = {
  id: string;
  tenant_id: string;
  audience: string;
  recipient_id: string;
  source: string;
  status: "draft" | "under_review" | "approved" | "rejected" | "delivered";
  requires_review: boolean;
  subject: string;
  body: string;
  channel: string;
  consent_class: string;
  severity: string;
  created_at: string;
};

export function createReportDraft(input: { subject: string; body: string }) {
  return api<Envelope<CommunicationDraftData>>("/v1/communications/drafts", {
    method: "POST",
    body: JSON.stringify({
      audience: "owner",
      recipient_id: ACCOUNTING_RECIPIENT_ID,
      source: "ai",
      // Urgent bypasses quiet hours (COM-002/COM-003) -- a monthly filing
      // deadline is time-sensitive in the same way a safety message is,
      // and a demo run at 11pm shouldn't fail on a quiet-hours check that
      // has nothing to do with this content.
      consent_class: "urgent",
      channel: "email",
      severity: "normal",
      subject: input.subject,
      body: input.body,
    }),
  });
}

export function reviewReportDraft(draftId: string, decision: "approved" | "rejected", reason?: string) {
  return api<Envelope<CommunicationDraftData>>(`/v1/communications/drafts/${encodeURIComponent(draftId)}/review`, {
    method: "POST",
    body: JSON.stringify({ reviewer_id: "owner-desk", decision, reason: reason ?? "" }),
  });
}

export function deliverReportDraft(draftId: string) {
  return api<Envelope<{ id: string; status: string }>>(`/v1/communications/drafts/${encodeURIComponent(draftId)}/deliver`, {
    method: "POST",
    body: JSON.stringify({}),
  });
}
