import { useEffect } from "react";
import { Link, Navigate, useLocation, useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { getPropertyDetail, type ComplianceHold, type PropertyData, type PropertyTransition } from "../lib/api/property";
import { setCurrentProperty, clearCurrentProperty } from "../lib/current-property";
import { getPropertyImage } from "../lib/property-images";
import "./property-detail.css";

const READINESS_LABELS: Record<keyof PropertyData["readiness"], string> = {
  owner_contract_accepted: "Owner contract",
  compliance_complete: "Compliance",
  mandatory_fields_set: "Operating details",
};

const DETAIL_DATE = new Intl.DateTimeFormat("en-IN", {
  day: "2-digit",
  month: "short",
  year: "numeric",
  hour: "2-digit",
  minute: "2-digit",
  hour12: false,
});

function humanize(value: string) {
  return value.replaceAll("_", " ");
}

function addressLine(property: PropertyData) {
  const address = property.service_address;
  return [address.line1, address.line2, address.city, address.state, address.postal_code, address.country]
    .filter(Boolean)
    .join(", ");
}

function dateLabel(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : DETAIL_DATE.format(date);
}

function transitionLabel(transition: PropertyTransition) {
  return `${humanize(transition.from_state)} → ${humanize(transition.to_state)}`;
}

function holdCopy(hold: ComplianceHold) {
  return hold.reason || "Compliance hold recorded by the property service.";
}

function PropertySkeleton() {
  return <div className="property-detail-skeleton" aria-label="Loading property" aria-busy="true"><span /><span /><span /><span /></div>;
}

function Readiness({ readiness }: { readiness: PropertyData["readiness"] }) {
  const readyCount = Object.values(readiness).filter(Boolean).length;
  return (
    <section className="property-panel property-readiness" aria-labelledby="readiness-title">
      <header><span>02</span><h2 id="readiness-title">Readiness</h2><small>{readyCount} / {Object.keys(readiness).length} RECORDED</small></header>
      <ul>
        {(Object.entries(readiness) as Array<[keyof PropertyData["readiness"], boolean]>).map(([key, ready]) => (
          <li key={key} data-ready={ready}><i aria-hidden="true" /><span>{READINESS_LABELS[key] ?? humanize(key)}</span><strong>{ready ? "READY" : "INCOMPLETE"}</strong></li>
        ))}
      </ul>
    </section>
  );
}

function ComplianceHolds({ holds }: { holds: ComplianceHold[] }) {
  return (
    <section className="property-panel property-holds" data-empty={holds.length === 0} aria-labelledby="holds-title">
      <header><span>03</span><h2 id="holds-title">Compliance holds</h2><small>{holds.length === 0 ? "ALL CLEAR" : `${holds.length} ACTIVE`}</small></header>
      {holds.length === 0 ? <div className="property-quiet-empty"><strong>No compliance holds recorded.</strong><span>The property service has not returned a blocking hold.</span></div> : (
        <ul>{holds.map((hold) => <li key={hold.id}><strong>{humanize(hold.kind)} · {humanize(hold.severity)}</strong><p>{holdCopy(hold)}</p><small>{humanize(hold.status)}{hold.expires_at ? ` · expires ${dateLabel(hold.expires_at)}` : ""}</small></li>)}</ul>
      )}
    </section>
  );
}

function Lifecycle({ transitions }: { transitions: Array<{ id: string; data: PropertyTransition }> }) {
  return (
    <section className="property-panel property-history" aria-labelledby="history-title">
      <header><span>05</span><h2 id="history-title">Lifecycle history</h2><small>{transitions.length} EVENTS</small></header>
      {transitions.length === 0 ? <div className="property-quiet-empty"><strong>No transitions recorded yet.</strong><span>The current state is the first lifecycle record available.</span></div> : (
        <ol>{transitions.map((entry) => <li key={entry.id}><i aria-hidden="true" /><time>{dateLabel(entry.data.created_at)}</time><strong>{transitionLabel(entry.data)}</strong><p>{entry.data.reason || "No reason recorded."}</p><small>ACTOR {entry.data.actor_id.slice(0, 8)}</small></li>)}</ol>
      )}
    </section>
  );
}

export default function PropertyDetail() {
  const { propertyId = "" } = useParams();
  const location = useLocation();
  const isActive = (to: string) => location.pathname === to || location.pathname.startsWith(`${to}/`);
  const detailQuery = useQuery({
    queryKey: ["owner", "property", propertyId],
    queryFn: () => getPropertyDetail(propertyId),
    enabled: Boolean(propertyId),
  });

  const detail = detailQuery.data;
  const property = detail?.property.data;
  const title = property?.service_address.line2 || property?.service_address.line1 || "Managed property";
  const image = property ? getPropertyImage(property.service_address) : undefined;

  useEffect(() => {
    if (propertyId && property) setCurrentProperty({ id: propertyId, label: title });
    else clearCurrentProperty();
    return clearCurrentProperty;
  }, [propertyId, property, title]);

  if (!propertyId) return <Navigate to="/dashboard" replace />;

  return (
    <main className="property-detail registration-frame">
      <header className="property-detail-header">
        <Link className="property-detail-wordmark" to="/dashboard">COMFORT CURATORS / OWNER</Link>
        <span>03 / PROPERTY DETAIL</span>
        <nav aria-label="Owner navigation"><Link to="/dashboard" aria-current={isActive("/dashboard") ? "page" : undefined}>DASHBOARD</Link><Link to="/onboarding" aria-current={isActive("/onboarding") ? "page" : undefined}>ONBOARD</Link><Link to={`/properties/${propertyId}`} aria-current={location.pathname === `/properties/${propertyId}` ? "page" : undefined}>PROPERTY</Link><Link to={`/properties/${propertyId}/package`} aria-current={location.pathname === `/properties/${propertyId}/package` ? "page" : undefined}>PACKAGE</Link><Link to="/login" aria-current={isActive("/login") ? "page" : undefined}>ACCESS DESK</Link></nav>
      </header>

      {detailQuery.isLoading ? <PropertySkeleton /> : detailQuery.isError || !detail || !property ? (
        <section className="property-detail-error" role="alert"><p>PROPERTY UNAVAILABLE</p><h1>We could not read this property.</h1><button type="button" onClick={() => void detailQuery.refetch()}>TRY AGAIN →</button><Link to="/dashboard">RETURN TO DASHBOARD</Link></section>
      ) : (
        <>
          <section className="property-hero" aria-labelledby="property-title">
            <div>
              {image && (
                <img
                  className="property-hero-photo"
                  src={image.src}
                  alt={image.alt}
                />
              )}
              <Link to="/dashboard">← DASHBOARD</Link><p>OWNER PROPERTY / {detail.property.id}</p><h1 id="property-title">{title}</h1><strong>{addressLine(property)}</strong>
            </div>
            <div className="property-hero-state"><span>CURRENT LIFECYCLE STATE</span><b>{humanize(property.state)}</b><Link to={`/properties/${propertyId}/package`}>OPEN INVENTORY PACKAGE →</Link></div>
          </section>

          {(property.compliance_holds?.length ?? 0) > 0 && <div className="property-hold-banner" role="alert"><strong>ATTENTION / {property.compliance_holds!.length} COMPLIANCE {property.compliance_holds!.length === 1 ? "HOLD" : "HOLDS"}</strong><span>Review the recorded hold before treating this property as ready.</span></div>}

          <div className="property-detail-grid">
            <div className="property-detail-main">
              <Readiness readiness={property.readiness} />
              <ComplianceHolds holds={property.compliance_holds ?? []} />
              <Lifecycle transitions={detail.transitions} />
            </div>
            <aside className="property-facts">
              <section className="property-panel"><header><span>04</span><h2>Property facts</h2><small>SERVER RECORD</small></header><dl><div><dt>MAXIMUM OCCUPANCY</dt><dd>{property.maximum_occupancy}</dd></div><div><dt>TIMEZONE</dt><dd>{property.timezone}</dd></div><div><dt>ACCESS METHOD</dt><dd>{property.access_method ? humanize(property.access_method) : "Not recorded"}</dd></div></dl></section>
              <section className="property-panel property-contacts"><header><span>06</span><h2>Emergency contacts</h2><small>{property.emergency_contacts?.length ?? 0} ON FILE</small></header>{property.emergency_contacts?.length ? <ul>{property.emergency_contacts.map((contact, index) => <li key={`${contact.phone}-${index}`}><strong>{contact.name}</strong><span>{contact.role ? humanize(contact.role) : "Contact"}</span><a href={`tel:${contact.phone}`}>{contact.phone}</a></li>)}</ul> : <div className="property-quiet-empty"><strong>No emergency contacts recorded.</strong><span>Add them during onboarding when the service asks for an owner contact.</span></div>}</section>
            </aside>
          </div>
        </>
      )}
    </main>
  );
}
