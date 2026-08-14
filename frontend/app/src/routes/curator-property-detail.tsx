import { useQuery } from "@tanstack/react-query";
import { Link, Navigate, useParams } from "react-router-dom";
import { getPropertyDetail } from "../lib/api/property";
import { getRole, getToken } from "../lib/auth/session";
import { OpsSkeleton, StatusLabel } from "./ops-shared";
import CuratorHeader from "./curator-header";
import "./curator.css";

function addressLine(address: { line1: string; line2?: string; city: string; state: string; postal_code: string; country: string }) {
  return [address.line1, address.line2, address.city, address.state, address.postal_code, address.country].filter(Boolean).join(", ");
}

export default function CuratorPropertyDetail() {
  const { propertyId = "" } = useParams();
  const query = useQuery({ queryKey: ["curator", "property", propertyId], queryFn: () => getPropertyDetail(propertyId), enabled: Boolean(propertyId) });
  const property = query.data?.property;

  if (!getToken() || getRole() !== "staff") return <Navigate to="/login" replace />;
  if (!propertyId) return <Navigate to="/jobs/map" replace />;
  if (query.isLoading) return <main className="curator-shell"><OpsSkeleton rows={6} /></main>;
  if (query.isError || !property) return <main className="curator-shell"><section className="curator-empty"><strong>PROPERTY UNAVAILABLE.</strong><p>The location record could not be read.</p><Link to="/jobs/map">RETURN TO ZONE MAP</Link></section></main>;

  const title = property.data.service_address.line2 || property.data.service_address.line1 || "Managed property";
  const zone = property.data.geolocation_zone?.trim() || "UNZONED";
  return <main className="curator-shell">
    <CuratorHeader section="08 / PROPERTY" />
    <section className="curator-detail-hero"><p className="curator-kicker"><Link to="/jobs/map">← ZONE MAP</Link> · {property.id}</p><h1>{title}</h1><StatusLabel status={property.data.state} /></section>
    <section className="curator-detail-meta"><div><span>TRAVEL ZONE</span><strong>{zone}</strong></div><div><span>ADDRESS</span><strong>{addressLine(property.data.service_address)}</strong></div><div><span>TIMEZONE</span><strong>{property.data.timezone}</strong></div><div><span>OCCUPANCY</span><strong>{property.data.maximum_occupancy}</strong></div></section>
    <div className="curator-detail-body">
      <section className="curator-panel"><header><span>01</span><strong>ACCESS</strong></header><div className="curator-access"><strong>{title}</strong><p>{property.data.access_method || "Check with operations for the current access method before arrival."}</p></div></section>
      <section className="curator-panel"><header><span>02</span><strong>EMERGENCY CONTACTS</strong><small>{property.data.emergency_contacts?.length ?? 0} ON FILE</small></header><div className="curator-access">{property.data.emergency_contacts?.length ? property.data.emergency_contacts.map((contact) => <p key={`${contact.name}-${contact.phone}`}><strong>{contact.name}</strong>{contact.role ? ` · ${contact.role}` : ""} · <a href={`tel:${contact.phone}`}>{contact.phone}</a></p>) : <p>No emergency contacts are recorded for this property.</p>}</div></section>
      <div className="curator-actions"><Link to="/jobs/map">BACK TO ZONE MAP</Link><Link to={`/ops/tickets?property=${property.id}`}>VIEW PROPERTY TICKETS →</Link></div>
    </div>
  </main>;
}
