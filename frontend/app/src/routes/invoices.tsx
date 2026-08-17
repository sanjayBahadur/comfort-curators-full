import { useEffect, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { useSearchParams } from "react-router-dom";
import { toast } from "sonner";
import Money from "../components/money";
import Select from "../components/ui/Select";
import { useAgentSurface } from "../components/agent-surface/context";
import type { AgentAction } from "../components/agent-surface/types";
import { setCurrentProperty, clearCurrentProperty } from "../lib/current-property";
import { getOpsProperties, type OpsPropertyData } from "../lib/api/ops";
import type { Envelope } from "../lib/api/client";
import { getPropertyContributionReport, type PropertyContributionReport } from "../lib/api/owner";
import { currentMonth, type ContributionData } from "../lib/api/dashboard";
import {
  createReportDraft,
  deliverReportDraft,
  getContributionReport,
  getOwnerExceptionsForProperty,
  getServiceLevelSummary,
  reviewReportDraft,
  type CommunicationDraftData,
  type ServiceLevelSummaryData,
} from "../lib/api/reports";
import { formatMoney } from "../lib/money";
import { propertyAddress, propertyName } from "./ops-format";
import { OwnerGate, OwnerRecordsHeader, OwnerRecordsSkeleton } from "./owner-records";
import "./owner-records.css";
import "./invoices.css";

function isReportEmpty(report: PropertyContributionReport) {
  return report.charge_count === 0 && report.credit_count === 0;
}

const REPORT_PERIOD_LABEL = new Intl.DateTimeFormat("en-IN", { month: "long", year: "numeric", timeZone: "UTC" });

type GeneratedReport = {
  period: { start: string; end: string };
  contribution: ContributionData;
  serviceLevel: ServiceLevelSummaryData;
  exceptionCount: number;
  exceptionLines: string[];
};

async function generateMonthlyReport(propertyId: string): Promise<GeneratedReport> {
  const period = currentMonth();
  const [contribution, serviceLevel, exceptions] = await Promise.all([
    getContributionReport(propertyId, period),
    getServiceLevelSummary(propertyId, period),
    getOwnerExceptionsForProperty(propertyId),
  ]);
  return {
    period,
    contribution: contribution.data,
    serviceLevel: serviceLevel.data,
    exceptionCount: exceptions.items.length,
    exceptionLines: exceptions.items.map((item) => `- ${item.data.label}: ${item.data.summary} (${item.data.status})`),
  };
}

function reportDraftText(property: Envelope<OpsPropertyData> | undefined, report: GeneratedReport): { subject: string; body: string } {
  const label = propertyName(property);
  const periodLabel = REPORT_PERIOD_LABEL.format(new Date(report.period.start));
  const c = report.contribution;
  const s = report.serviceLevel;
  const body = [
    `Monthly report — ${label}`,
    ...(property ? [propertyAddress(property)] : []),
    `Period: ${periodLabel}`,
    "",
    "FINANCIALS",
    `Revenue: ${formatMoney(c.revenue_minor_units, c.currency)}`,
    `Supply margin: ${formatMoney(c.supply_margin_minor_units, c.currency)}`,
    `Vendor cost: ${formatMoney(c.vendor_cost_minor_units, c.currency)}`,
    `Refunds: ${formatMoney(c.refund_minor_units, c.currency)}`,
    `Discounts: ${formatMoney(c.discount_minor_units, c.currency)}`,
    `Tax: ${formatMoney(c.tax_minor_units, c.currency)}`,
    `Net contribution: ${formatMoney(c.net_contribution_minor_units, c.currency)}`,
    "",
    "SERVICE LEVEL",
    `${s.closed_tickets} of ${s.total_tickets} tickets closed this period · ${s.open_incidents} open incident(s) · ${s.open_recoveries} open service recovery(ies).`,
    "",
    "EXCEPTIONS THIS PERIOD",
    report.exceptionCount === 0 ? "None on record." : report.exceptionLines.join("\n"),
    "",
    "Prepared by Superhost from live property records for manual filing. Reviewed and approved by the property owner before delivery.",
  ].join("\n");
  return { subject: `${label} — ${periodLabel} monthly report for filing`, body };
}

export default function Invoices() {
  const [searchParams, setSearchParams] = useSearchParams();
  const propertyQuery = useQuery({ queryKey: ["owner", "properties"], queryFn: getOpsProperties });
  const propertyId = searchParams.get("property") ?? "";
  const reportQuery = useQuery({
    queryKey: ["owner", "contribution", propertyId],
    queryFn: () => getPropertyContributionReport(propertyId),
    enabled: Boolean(propertyId),
  });
  const properties = propertyQuery.data?.items ?? [];
  const selectedProperty = properties.find((property) => property.id === propertyId);
  const report = reportQuery.data;

  // Without this, the global Superhost drawer's thread stays scoped to
  // whatever property was last set from another page (or none at all,
  // widening to portfolio scope) -- verified live: asked to file this
  // page's report, Superhost answered about a different property because
  // this page never told the shared current-property store which one is
  // actually on screen. See ops-calendar.tsx / package-shop.tsx for the
  // same pattern.
  useEffect(() => {
    if (selectedProperty) setCurrentProperty({ id: selectedProperty.id, label: propertyName(selectedProperty) });
    else clearCurrentProperty();
    return clearCurrentProperty;
  }, [selectedProperty]);

  const [generatedReport, setGeneratedReport] = useState<GeneratedReport | null>(null);
  const [draft, setDraft] = useState<CommunicationDraftData | null>(null);

  const generateSurface = useAgentSurface("owner-report-generate", ["click"] as AgentAction[], "Generate the monthly report from live records (step 1 of 3)");
  const draftSurface = useAgentSurface("owner-report-draft", ["click"] as AgentAction[], "Draft the report for accounting review (step 2 of 3, needs step 1 first)");
  const fileSurface = useAgentSurface("owner-report-file", ["click"] as AgentAction[], "Approve and file the draft with accounting (step 3 of 3, needs step 2 first)");

  const generateMutation = useMutation({
    mutationFn: () => generateMonthlyReport(propertyId),
    onSuccess: (result) => {
      setGeneratedReport(result);
      setDraft(null);
      toast.success("Monthly report generated from live records");
    },
    onError: (error: unknown) => toast.error(error instanceof Error ? error.message : "Could not generate the report"),
  });

  const draftMutation = useMutation({
    mutationFn: async () => {
      if (!generatedReport) throw new Error("Generate the report first");
      const { subject, body } = reportDraftText(selectedProperty, generatedReport);
      const created = await createReportDraft({ subject, body });
      return created.data;
    },
    onSuccess: (data) => {
      setDraft(data);
      toast.success("Drafted for accounting review");
    },
    onError: (error: unknown) => toast.error(error instanceof Error ? error.message : "Could not draft the report"),
  });

  const fileMutation = useMutation({
    mutationFn: async () => {
      if (!draft) throw new Error("Draft the report first");
      await reviewReportDraft(draft.id, "approved");
      await deliverReportDraft(draft.id);
    },
    onSuccess: () => {
      setDraft((current) => (current ? { ...current, status: "delivered" } : current));
      toast.success("Filed with accounting");
    },
    onError: (error: unknown) => toast.error(error instanceof Error ? error.message : "Could not file the report"),
  });

  function setProperty(value: string) {
    setGeneratedReport(null);
    setDraft(null);
    setSearchParams((current) => {
      const next = new URLSearchParams(current);
      if (value) next.set("property", value);
      else next.delete("property");
      return next;
    }, { replace: true });
  }

  return (
    <OwnerGate>
      <main className="owner-records invoice-page registration-frame">
        <OwnerRecordsHeader section="04 / INVOICES" />
        <section className="owner-records-title">
          <div>
            <p>CHARGES + CREDITS / ALL RECORDED ACTIVITY</p>
            <h1>Invoices</h1>
          </div>
          <strong>{propertyId ? "PROPERTY SCOPED" : "SELECT A PROPERTY"}</strong>
        </section>

        <section className="owner-records-selector" aria-label="Invoice property selector">
          <label>
            <span>PROPERTY</span>
            <Select
              value={propertyId}
              onChange={setProperty}
              options={[
                { value: "", label: "SELECT PROPERTY" },
                ...properties.map((property) => ({ value: property.id, label: propertyName(property) })),
              ]}
            />
          </label>
        </section>

        {propertyId && (
          <section className="owner-records-section invoice-report" aria-labelledby="monthly-report-title">
            <header><span>02</span><h2 id="monthly-report-title">Monthly report</h2><small>SUPERHOST-ASSEMBLED · FOR ACCOUNTING</small></header>
            <p className="owner-records-note">Assembles this property's real contribution, service-level, and exception figures into one filing-ready report, then routes it to accounting for manual filing.</p>
            <div className="invoice-report-actions">
              <button ref={generateSurface.ref} type="button" onClick={() => generateMutation.mutate()} disabled={generateMutation.isPending}>
                {generateMutation.isPending ? "GENERATING…" : "GENERATE MONTHLY REPORT"}
              </button>
              {/* All three steps stay mounted (disabled until ready) rather
                  than appearing only once their precondition is met. An
                  agent surface only exists for a tool-caller once it's in
                  the DOM at the moment a message is sent -- a button that
                  only mounts after step 1 completes is one Superhost can
                  never even know to plan for. Keeping it present, just
                  disabled, lets the model see the whole real sequence
                  upfront instead of discovering it only has a tool for
                  step 1 (confirmed live: asked to do all three, it saw
                  only the first button and stopped there to ask permission). */}
              <button ref={draftSurface.ref} type="button" onClick={() => draftMutation.mutate()} disabled={!generatedReport || draftMutation.isPending || Boolean(draft)}>
                {draft ? "DRAFTED" : draftMutation.isPending ? "DRAFTING…" : "DRAFT FOR ACCOUNTING"}
              </button>
              <button ref={fileSurface.ref} type="button" onClick={() => fileMutation.mutate()} disabled={!draft || draft.status === "delivered" || fileMutation.isPending}>
                {draft?.status === "delivered" ? "FILED" : fileMutation.isPending ? "FILING…" : "APPROVE & FILE"}
              </button>
            </div>

            {generatedReport && (
              <div className="owner-records-summary invoice-report-summary">
                <article>
                  <small>REVENUE</small>
                  <div>
                    <p>This period</p>
                    <Money className="owner-records-money" value={generatedReport.contribution.revenue_minor_units} currency={generatedReport.contribution.currency} />
                  </div>
                </article>
                <article>
                  <small>SUPPLY MARGIN</small>
                  <div>
                    <p>This period</p>
                    <Money className="owner-records-money" value={generatedReport.contribution.supply_margin_minor_units} currency={generatedReport.contribution.currency} />
                  </div>
                </article>
                <article data-net="true">
                  <small>NET CONTRIBUTION</small>
                  <div>
                    <p>This period</p>
                    <Money className="owner-records-money" value={generatedReport.contribution.net_contribution_minor_units} currency={generatedReport.contribution.currency} />
                  </div>
                </article>
              </div>
            )}
            {generatedReport && (
              <p className="owner-records-note">
                {generatedReport.serviceLevel.closed_tickets} of {generatedReport.serviceLevel.total_tickets} tickets closed this period · {generatedReport.serviceLevel.open_incidents} open incident(s) · {generatedReport.exceptionCount} owner exception(s) on record.
              </p>
            )}
            {draft && (
              <p className="owner-records-note" aria-live="polite">
                <strong>{draft.status === "delivered" ? "FILED WITH ACCOUNTING" : draft.status === "approved" ? "APPROVED · SENDING" : "DRAFTED · UNDER REVIEW"}</strong>
                <span> · {draft.subject}</span>
              </p>
            )}
          </section>
        )}

        {!propertyId ? (
          <section className="owner-records-empty"><strong>Select a property.</strong><p>The statement is scoped to one property at a time.</p></section>
        ) : propertyQuery.isLoading ? <OwnerRecordsSkeleton /> : propertyQuery.isError ? (
          <section className="owner-records-error" role="alert"><p>PROPERTIES UNAVAILABLE</p><h2>We could not read your properties.</h2><button type="button" onClick={() => void propertyQuery.refetch()}>TRY AGAIN →</button></section>
        ) : reportQuery.isLoading ? <OwnerRecordsSkeleton /> : reportQuery.isError ? (
          <section className="owner-records-error" role="alert"><p>STATEMENT UNAVAILABLE</p><h2>We could not read the statement.</h2><button type="button" onClick={() => void reportQuery.refetch()}>TRY AGAIN →</button></section>
        ) : !report || isReportEmpty(report) ? (
          <section className="owner-records-empty"><strong>No charges yet.</strong><p>The first statement appears after the first recorded charge or credit.</p></section>
        ) : (
          <>
            <section className="owner-records-section invoice-statement" aria-labelledby="invoice-summary-title">
              <header><span>01</span><h2 id="invoice-summary-title">Ledger statement</h2><small>{report.currency} · AGGREGATE RECORD</small></header>
              <div className="invoice-statement-topmatter">
                <div className="invoice-statement-property">
                  <small>PROPERTY OF RECORD</small>
                  <strong>{propertyName(selectedProperty)}</strong>
                  {selectedProperty && <address>{propertyAddress(selectedProperty)}</address>}
                </div>
                <div className="invoice-statement-title">
                  <span>STATEMENT</span>
                  <strong>{report.currency}</strong>
                </div>
              </div>
              <div className="invoice-statement-metadata" aria-label="Statement counts">
                <span><strong>{report.charge_count}</strong> CHARGE{report.charge_count === 1 ? "" : "S"}</span>
                <span><strong>{report.credit_count}</strong> CREDIT{report.credit_count === 1 ? "" : "S"}</span>
                <span><strong>{report.subledger_entry_count}</strong> SUBLEDGER {report.subledger_entry_count === 1 ? "ENTRY" : "ENTRIES"}</span>
              </div>
              <div className="owner-records-summary">
                <article>
                  <small>ALL RECORDED CHARGES</small>
                  <div>
                    <p>Total charges</p>
                    <Money className="owner-records-money" value={report.total_charges_minor_units} currency={report.currency} />
                  </div>
                </article>
                <article>
                  <small>ALL RECORDED CREDITS</small>
                  <div>
                    <p>Total credits</p>
                    <Money className="owner-records-money" value={report.total_credits_minor_units} currency={report.currency} />
                  </div>
                </article>
                <article data-net="true">
                  <small>CHARGES MINUS CREDITS</small>
                  <div>
                    <p>Net</p>
                    <Money className="owner-records-money" value={report.net_minor_units} currency={report.currency} />
                  </div>
                </article>
              </div>
              <div className="invoice-barcode" aria-hidden="true" />
            </section>
            <p className="owner-records-note invoice-note"><strong>AGGREGATE STATEMENT ONLY · PER-CHARGE DETAIL NOT YET ISSUED</strong><span>The contribution report returns aggregate totals. No reachable endpoint itemizes individual charges or credits.</span></p>
          </>
        )}
      </main>
    </OwnerGate>
  );
}
