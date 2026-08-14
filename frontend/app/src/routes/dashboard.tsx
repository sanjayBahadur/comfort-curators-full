import { useEffect, useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, Navigate, useLocation } from "react-router-dom";
import Money from "../components/money";
import {
  getOwnerDashboard,
  type DashboardProperty,
  type OwnerExceptionData,
} from "../lib/api/dashboard";
import type { Envelope } from "../lib/api/client";
import type { TicketData } from "../lib/api/ops";
import { getRole, getToken } from "../lib/auth/session";
import { setCurrentProperty, clearCurrentProperty } from "../lib/current-property";
import { useAgentSurface } from "../components/agent-surface/context";
import type { AgentAction } from "../components/agent-surface/types";
import { getPropertyImage } from "../lib/property-images";
import OwnerPropertyOverview from "../components/owner/OwnerPropertyOverview";
import OwnerTaskTerminal from "../components/owner/OwnerTaskTerminal";
import "./dashboard.css";

const COMMITTED_STATUSES = new Set(["approved", "scheduled", "assigned", "in_progress"]);
const FINISHED_STATUSES = new Set(["verified", "closed"]);
const READINESS_LABELS = {
  owner_contract_accepted: "Owner contract",
  compliance_complete: "Compliance",
  mandatory_fields_set: "Operating details",
} as const;

const DASHBOARD_DATE = new Intl.DateTimeFormat("en-IN", {
  day: "2-digit",
  month: "short",
  hour: "2-digit",
  minute: "2-digit",
  hour12: false,
  timeZone: "Asia/Kolkata",
});

type AttentionItem = {
  id: string;
  label: string;
  summary: string;
  propertyName: string;
  status: string;
};

function OwnerGate({ children }: { children: React.ReactNode }) {
  if (!getToken() || getRole() !== "owner") return <Navigate to="/login" replace />;
  return children;
}

function propertyName(property: DashboardProperty) {
  return property.property.data.service_address.line2
    || property.property.data.service_address.line1
    || "Managed property";
}

function shortAddress(property: DashboardProperty) {
  const address = property.property.data.service_address;
  return `${address.line1}, ${address.city}`;
}

function readinessScore(property: DashboardProperty) {
  return Object.values(property.property.data.readiness).filter(Boolean).length;
}

function ticketWindow(ticket: Envelope<TicketData>) {
  const window = ticket.data.requested_window;
  return window ? `${DASHBOARD_DATE.format(new Date(window.start))} — ${DASHBOARD_DATE.format(new Date(window.end))}` : "Window pending";
}

function isContributionEmpty(property: DashboardProperty) {
  const contribution = property.contribution.data;
  return contribution.revenue_minor_units === 0
    && contribution.supply_margin_minor_units === 0
    && contribution.vendor_cost_minor_units === 0
    && contribution.refund_minor_units === 0
    && contribution.exception_cost_minor_units === 0
    && contribution.discount_minor_units === 0
    && contribution.tax_minor_units === 0
    && contribution.net_contribution_minor_units === 0;
}

function buildUpcoming(properties: DashboardProperty[]) {
  const now = Date.now();
  const sevenDays = now + 7 * 24 * 60 * 60_000;
  return properties
    .flatMap((property) => property.tickets.map((ticket) => ({ ticket, property })))
    .filter(({ ticket }) => COMMITTED_STATUSES.has(ticket.data.status))
    .filter(({ ticket }) => {
      const start = ticket.data.requested_window?.start;
      if (!start) return false;
      const time = new Date(start).getTime();
      return Number.isFinite(time) && time >= now && time < sevenDays;
    })
    .sort((left, right) =>
      (left.ticket.data.requested_window?.start ?? "").localeCompare(right.ticket.data.requested_window?.start ?? ""),
    );
}

function buildAttention(
  properties: DashboardProperty[],
  exceptions: Array<Envelope<OwnerExceptionData>>,
): AttentionItem[] {
  const propertyById = new Map(properties.map((property) => [property.property.id, property]));
  const reported = exceptions.map((exception) => ({
    id: exception.id,
    label: exception.data.label,
    summary: exception.data.summary,
    propertyName: propertyName(propertyById.get(exception.data.property_id)!),
    status: exception.data.status,
  }));
  const propertyConcerns = properties.flatMap((property) => {
    const concerns: AttentionItem[] = [];
    if (property.property.data.state !== "active") {
      concerns.push({
        id: `${property.property.id}-state`,
        label: "Property is not active",
        summary: `Current lifecycle state: ${property.property.data.state.replaceAll("_", " ")}.`,
        propertyName: propertyName(property),
        status: property.property.data.state,
      });
    }
    const missingReadiness = Object.entries(property.property.data.readiness)
      .filter(([, ready]) => !ready)
      .map(([key]) => READINESS_LABELS[key as keyof typeof READINESS_LABELS]);
    if (missingReadiness.length > 0) {
      concerns.push({
        id: `${property.property.id}-readiness`,
        label: "Readiness needs a decision",
        summary: `${missingReadiness.join(", ")} ${missingReadiness.length === 1 ? "is" : "are"} incomplete.`,
        propertyName: propertyName(property),
        status: "incomplete",
      });
    }
    if (!property.activePackage) {
      concerns.push({
        id: `${property.property.id}-package`,
        label: "No active package",
        summary: "Choose the operating inventory package for this property.",
        propertyName: propertyName(property),
        status: "decision required",
      });
    }
    for (const document of property.documents.filter((entry) => entry.data.status === "expired" || entry.data.status === "revoked")) {
      concerns.push({
        id: document.id,
        label: "Document needs attention",
        summary: document.data.title,
        propertyName: propertyName(property),
        status: document.data.status,
      });
    }
    return concerns;
  });
  return [...reported, ...propertyConcerns];
}

function DashboardSkeleton() {
  return (
    <div className="owner-skeleton" aria-label="Loading owner dashboard" aria-busy="true">
      <span /><span /><span /><span /><span /><span />
    </div>
  );
}

export default function Dashboard() {
  const location = useLocation();
  const isActive = (to: string) => location.pathname === to || location.pathname.startsWith(`${to}/`);
  const packageLinkSurface = useAgentSurface("dashboard-package-link", ["focus", "click", "scroll_to"] as AgentAction[], "Open the property package page");
  const retrySurface = useAgentSurface("dashboard-retry", ["focus", "click"] as AgentAction[], "Retry loading the owner dashboard");
  const dashboardQuery = useQuery({ queryKey: ["owner", "dashboard"], queryFn: getOwnerDashboard });
  const dashboard = dashboardQuery.data;
  const [selectedPropertyIndex, setSelectedPropertyIndex] = useState(0);
  const packageScrollRef = useRef<HTMLDivElement>(null);
  const upcoming = useMemo(() => buildUpcoming(dashboard?.properties ?? []), [dashboard?.properties]);
  const attention = useMemo(
    () => buildAttention(dashboard?.properties ?? [], dashboard?.exceptions ?? []),
    [dashboard?.exceptions, dashboard?.properties],
  );
  const firstPropertyId = dashboard?.properties[0]?.property.id;
  const selectedProperty = dashboard?.properties[selectedPropertyIndex] ?? dashboard?.properties[0] ?? null;

  useEffect(() => {
    if (selectedProperty) {
      setCurrentProperty({ id: selectedProperty.property.id, label: propertyName(selectedProperty) });
    } else {
      clearCurrentProperty();
    }
    return clearCurrentProperty;
  }, [selectedProperty]);

  useEffect(() => {
    if (dashboard && selectedPropertyIndex >= dashboard.properties.length) setSelectedPropertyIndex(0);
  }, [dashboard, selectedPropertyIndex]);

  useEffect(() => {
    const scroller = packageScrollRef.current;
    if (!scroller || !dashboard?.properties.length) return;
    scroller.scrollTo({ top: selectedPropertyIndex * scroller.clientHeight });
  }, [dashboard?.properties.length, selectedPropertyIndex]);

  const handlePackageScroll = () => {
    const scroller = packageScrollRef.current;
    if (!scroller || !dashboard?.properties.length || scroller.clientHeight === 0) return;
    const nextIndex = Math.max(0, Math.min(
      dashboard.properties.length - 1,
      Math.round(scroller.scrollTop / scroller.clientHeight),
    ));
    if (nextIndex !== selectedPropertyIndex) setSelectedPropertyIndex(nextIndex);
  };
  const completedTurnovers = dashboard?.properties.reduce(
    (count, property) => count + property.tickets.filter((ticket) =>
      ticket.data.type === "turnover" && FINISHED_STATUSES.has(ticket.data.status),
    ).length,
    0,
  ) ?? 0;
  const allDocuments = (dashboard?.properties ?? [])
    .flatMap((property) => property.documents.map((document) => ({ document, property })))
    .sort((left, right) => right.document.data.created_at.localeCompare(left.document.data.created_at));
  const recentDocuments = allDocuments.slice(0, 5);

  return (
    <OwnerGate>
      <main className="owner-dashboard registration-frame">
        <header className="owner-header">
          <Link className="owner-wordmark" to="/dashboard">COMFORT CURATORS / OWNER</Link>
          <span>01 / OWNER HOME</span>
          <nav aria-label="Owner navigation">
            <Link to="/dashboard" aria-current={isActive("/dashboard") ? "page" : undefined}>DASHBOARD</Link>
            <Link to="/onboarding" aria-current={isActive("/onboarding") ? "page" : undefined}>ONBOARD</Link>
            {firstPropertyId ? (
              <Link to={`/properties/${firstPropertyId}/package`} aria-current={isActive(`/properties/${firstPropertyId}/package`) ? "page" : undefined}>PACKAGE</Link>
            ) : (
              <span className="owner-nav-placeholder" aria-disabled="true">PACKAGE</span>
            )}
            <Link to="/invoices" aria-current={isActive("/invoices") ? "page" : undefined}>INVOICES</Link>
            <Link to="/documents" aria-current={isActive("/documents") ? "page" : undefined}>DOCUMENTS</Link>
            <Link to="/debug#seed-reset" aria-current={isActive("/debug") ? "page" : undefined}>SEED</Link>
            <Link to="/login" aria-current={isActive("/login") ? "page" : undefined}>ACCESS DESK</Link>
          </nav>
        </header>

        <section className="owner-intro" aria-labelledby="dashboard-title">
          <p>PORTFOLIO / LUCKNOW</p>
          <h1 id="dashboard-title">Is everything fine?</h1>
          <div>
            <strong>{dashboard ? `${dashboard.properties.length} MANAGED ${dashboard.properties.length === 1 ? "PROPERTY" : "PROPERTIES"}` : "QUIETLY CHECKING"}</strong>
            <span>Exceptions first. Routine work stays out of the way.</span>
          </div>
        </section>

        {dashboardQuery.isLoading ? <DashboardSkeleton /> : dashboardQuery.isError || !dashboard ? (
          <section className="owner-load-error" role="alert">
            <p>OWNER VIEW UNAVAILABLE</p>
            <h2>We could not assemble the portfolio.</h2>
            <button ref={retrySurface.ref} type="button" onClick={() => void dashboardQuery.refetch()}>TRY AGAIN →</button>
          </section>
        ) : dashboard.properties.length === 0 ? (
          <section className="owner-no-properties">
            <p>01 / PROPERTIES</p>
            <h2>No properties yet.</h2>
            <Link to="/onboarding">START ONBOARDING →</Link>
          </section>
        ) : (
          <>
            <OwnerTaskTerminal property={selectedProperty ?? dashboard.properties[0]} />
            <OwnerPropertyOverview properties={dashboard.properties} selectedIndex={selectedPropertyIndex} onSelect={setSelectedPropertyIndex} />
            <section className="owner-section owner-properties" aria-labelledby="properties-title">
              <header><span>02</span><h2 id="properties-title">Your properties</h2><small>STATE + READINESS</small></header>
              <div className="owner-property-grid">
                {dashboard.properties.map((property) => {
                  const next = upcoming.find((entry) => entry.property.property.id === property.property.id);
                  const image = getPropertyImage(property.property.data.service_address);
                  return (
                    <article className="owner-property" key={property.property.id}>
                      <div className="owner-property-heading">
                        {image && (
                          <img
                            className="owner-property-photo"
                            src={image.src}
                            alt={image.alt}
                            loading="lazy"
                          />
                        )}
                        <p>{shortAddress(property)}</p>
                        <h3>{propertyName(property)}</h3>
                        <span>{property.property.data.state.replaceAll("_", " ")}</span>
                      </div>
                      <div className="owner-readiness">
                        <strong>READINESS {readinessScore(property)} / 3</strong>
                        <ul>
                          {Object.entries(property.property.data.readiness).map(([key, ready]) => (
                            <li key={key} data-ready={ready}><span>{READINESS_LABELS[key as keyof typeof READINESS_LABELS]}</span><b>{ready ? "READY" : "INCOMPLETE"}</b></li>
                          ))}
                        </ul>
                      </div>
                      <div className="owner-next-work">
                        <span>NEXT OWNER-VISIBLE WORK</span>
                        {next ? <><strong>{next.ticket.data.type.replaceAll("_", " ")}</strong><time>{ticketWindow(next.ticket)}</time></> : <strong>Nothing scheduled this week.</strong>}
                      </div>
                      <div className="owner-property-links">
                        <Link to={`/properties/${property.property.id}`}>PROPERTY DETAILS →</Link>
                        <Link ref={property === dashboard.properties[0] ? packageLinkSurface.ref : undefined} to={`/properties/${property.property.id}/package`}>VIEW PACKAGE →</Link>
                      </div>
                    </article>
                  );
                })}
              </div>
            </section>

            <section className="owner-section owner-attention" data-empty={attention.length === 0} aria-labelledby="attention-title" aria-live="polite">
              <header><span>03</span><h2 id="attention-title">Needs your attention</h2><small>{attention.length === 0 ? "ALL QUIET" : `${attention.length} OPEN`}</small></header>
              {attention.length === 0 ? (
                <div className="owner-attention-empty"><i aria-hidden="true">OK</i><p>No exceptions. We’ll surface anything that needs your decision.</p></div>
              ) : (
                <ul className="owner-attention-list">
                  {attention.map((item) => <li key={item.id}><span>{item.propertyName}</span><strong>{item.label}</strong><p>{item.summary}</p><small>{item.status}</small></li>)}
                </ul>
              )}
            </section>

            <div className="owner-dashboard-grid">
              <section className="owner-section owner-upcoming" aria-labelledby="upcoming-title">
                <header><span>04</span><h2 id="upcoming-title">Next seven days</h2><small>{upcoming.length} COMMITTED</small></header>
                {upcoming.length === 0 ? <p className="owner-quiet-empty">Nothing scheduled this week.</p> : (
                  <ol>
                    {upcoming.map(({ ticket, property }, index) => (
                      <li key={ticket.id}><b>{String(index + 1).padStart(2, "0")}</b><div><strong>{ticket.data.type.replaceAll("_", " ")}</strong><span>{propertyName(property)}</span></div><time>{ticketWindow(ticket)}</time><small>{ticket.data.status}</small></li>
                    ))}
                  </ol>
                )}
              </section>

              <section className="owner-section owner-packages" aria-labelledby="packages-title">
                <header><span>05</span><h2 id="packages-title">Your package</h2><small>PACKAGE BILL</small></header>
                <div className="owner-package-switcher" aria-label="Package bill selector">
                  <span>BILL {String(selectedPropertyIndex + 1).padStart(2, "0")} / {String(dashboard.properties.length).padStart(2, "0")}</span>
                  <strong>{propertyName(selectedProperty ?? dashboard.properties[0])}</strong>
                  <div>
                    <button type="button" disabled={dashboard.properties.length < 2} aria-label="Previous package bill" onClick={() => setSelectedPropertyIndex((selectedPropertyIndex - 1 + dashboard.properties.length) % dashboard.properties.length)}>←</button>
                    <button type="button" disabled={dashboard.properties.length < 2} aria-label="Next package bill" onClick={() => setSelectedPropertyIndex((selectedPropertyIndex + 1) % dashboard.properties.length)}>→</button>
                  </div>
                </div>
                <div className="owner-package-scroll" ref={packageScrollRef} onScroll={handlePackageScroll}>
                  {dashboard.properties.map((property, propertyIndex) => (
                    <article className="owner-package-receipt" data-empty={!property.activePackage} data-property-index={propertyIndex} key={property.property.id}>
                      <div className="owner-package-receipt-brand">
                        <span>COMFORT CURATORS</span>
                        <b aria-hidden="true">CC</b>
                      </div>
                      <div className="owner-package-receipt-meta">
                        <span>PROPERTY OF RECORD</span>
                        <small>{property.activePackage ? `ACTIVE / V${property.activePackage.data.version_number}` : "PACKAGE / NOT SET"}</small>
                      </div>
                      <h3>{propertyName(property)}</h3>
                      <address>{shortAddress(property)}</address>
                      {property.activePackage ? (
                        <>
                          <dl className="owner-package-lines">
                            <div><dt>Recurring supply plan</dt><dd><Money value={property.activePackage.data.monthly_cost_minor_units} currency={property.activePackage.data.currency} /></dd></div>
                            <div><dt>Expected consumption</dt><dd>{property.activePackage.data.monthly_consumption_units} units / month</dd></div>
                            {property.activePackage.data.setup_cost_minor_units > 0 && (
                              <div><dt>One-time setup</dt><dd><Money value={property.activePackage.data.setup_cost_minor_units} currency={property.activePackage.data.currency} /></dd></div>
                            )}
                          </dl>
                          <div className="owner-package-total">
                            <span>Monthly total</span>
                            <Money className="owner-package-cost" value={property.activePackage.data.monthly_cost_minor_units} currency={property.activePackage.data.currency} />
                          </div>
                          <div className="owner-package-barcode" aria-hidden="true" />
                          <footer>
                            <small>SERVER PRICED · RECURRING MONTHLY</small>
                            <Link to={`/properties/${property.property.id}/package`}>REVIEW BILL →</Link>
                          </footer>
                        </>
                      ) : (
                        <div className="owner-package-empty">
                          <strong>No package on file.</strong>
                          <p>Curate the property’s recurring supplies and review the monthly bill before activation.</p>
                          <Link to={`/properties/${property.property.id}/package`}>BUILD PACKAGE →</Link>
                        </div>
                      )}
                    </article>
                  ))}
                </div>
              </section>

              <section className="owner-section owner-contribution" aria-labelledby="contribution-title">
                <header><span>06</span><h2 id="contribution-title">This month</h2><small>RECORDED ACTIVITY</small></header>
                {dashboard.properties.every(isContributionEmpty) ? (
                  <p className="owner-quiet-empty">Your first statement appears after the first completed service.</p>
                ) : (
                  <ul>
                    {dashboard.properties.map((property) => <li key={property.property.id}><span>{propertyName(property)}</span><Money value={property.contribution.data.net_contribution_minor_units} currency={property.contribution.data.currency} /><small>NET CONTRIBUTION</small></li>)}
                  </ul>
                )}
              </section>

              <section className="owner-section owner-documents" aria-labelledby="documents-title">
                <header><span>07</span><h2 id="documents-title">Recent documents</h2><small>{allDocuments.length} ON FILE</small></header>
                {allDocuments.length === 0 ? (
                  <p className="owner-quiet-empty">No owner documents are on file yet.</p>
                ) : (
                  <ul>{recentDocuments.map(({ document, property }) => <li key={document.id}><span>{propertyName(property)}</span><strong>{document.data.title}</strong><small>{document.data.status} · ADDED {DASHBOARD_DATE.format(new Date(document.data.created_at))}</small></li>)}</ul>
                )}
                <Link className="owner-documents-all" to="/documents">VIEW ALL →</Link>
              </section>
            </div>

            <section className="owner-standards" aria-labelledby="standards-title">
              <header><span>08 / OPERATING STANDARDS</span><h2 id="standards-title">We operate your property to Superhost standards.</h2><p>The platform designation remains the platform’s decision. Here is the line between our work and yours.</p></header>
              <div className="owner-control-columns">
                <section>
                  <p>WE CONTROL</p>
                  <ul>
                    <li><strong>Response timeliness</strong><span>Measurement appears when owner communication events are available.</span></li>
                    <li><strong>Turnovers inside their window</strong><span>{completedTurnovers > 0 ? `${completedTurnovers} completed turnover ${completedTurnovers === 1 ? "record" : "records"} available.` : "Measurement starts after the first completed turnover."}</span></li>
                    <li><strong>Incident resolution</strong><span>{dashboard.exceptions.some((exception) => exception.data.source === "incident") ? "An owner-visible incident is open above." : "No owner-visible incident is open."}</span></li>
                  </ul>
                </section>
                <section>
                  <p>YOU CONTROL</p>
                  <ul>
                    <li><strong>Nightly pricing</strong><span>Your booking-platform price and discount decisions.</span></li>
                    <li><strong>Calendar availability</strong><span>Which dates you make available for guests.</span></li>
                    <li><strong>Cancellations</strong><span>Owner-side cancellation policy and decisions.</span></li>
                  </ul>
                </section>
              </div>
            </section>
          </>
        )}
      </main>
    </OwnerGate>
  );
}
