import { useQuery } from "@tanstack/react-query";
import { useSearchParams } from "react-router-dom";
import Money from "../components/money";
import Select from "../components/ui/Select";
import { getOpsProperties } from "../lib/api/ops";
import { getPropertyContributionReport, type PropertyContributionReport } from "../lib/api/owner";
import { propertyAddress, propertyName } from "./ops-format";
import { OwnerGate, OwnerRecordsHeader, OwnerRecordsSkeleton } from "./owner-records";
import "./owner-records.css";
import "./invoices.css";

function isReportEmpty(report: PropertyContributionReport) {
  return report.charge_count === 0 && report.credit_count === 0;
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

  function setProperty(value: string) {
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
