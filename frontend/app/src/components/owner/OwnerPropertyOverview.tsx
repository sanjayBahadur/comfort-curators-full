import { useEffect, useMemo, useRef } from "react";
import { useQuery } from "@tanstack/react-query";
import L from "leaflet";
import { Link } from "react-router-dom";
import Money from "../money";
import { getReservations, type ReservationData } from "../../lib/api/ops";
import type { DashboardProperty } from "../../lib/api/dashboard";
import "leaflet/dist/leaflet.css";
import "./OwnerPropertyOverview.css";

const CLOSED_WORK = new Set(["verified", "closed", "cancelled", "rejected"]);
const DAY_MS = 86_400_000;

function nameOf(property: DashboardProperty) {
  return property.property.data.service_address.line2 || property.property.data.service_address.line1 || "Managed property";
}

function propertyCoordinates(property: DashboardProperty, index: number) {
  const address = property.property.data.service_address;
  const text = `${address.line1} ${address.line2 ?? ""} ${address.city}`.toLocaleLowerCase("en-IN");
  // Real Noida coordinates -- same anchors as demo-property-locations.ts
  // (the field-routing map). These were still real Lucknow lat/lng
  // (26.8x/80.9x) after the address text itself moved to Noida: the map
  // tiles behind the pin didn't match anything the labels claimed.
  const anchor = text.includes("hazratganj") ? { lat: 28.5708, lng: 77.3210 } : { lat: 28.5098, lng: 77.4031 };
  let hash = 0;
  for (const character of property.property.id) hash = (hash * 31 + character.charCodeAt(0)) | 0;
  const offset = ((Math.abs(hash) % 1001) - 500) / 100_000;
  return { lat: anchor.lat + offset + index * .0004, lng: anchor.lng - offset - index * .0003 };
}

function OwnerMap({ property, index }: { property: DashboardProperty; index: number }) {
  const elementRef = useRef<HTMLDivElement>(null);
  const mapRef = useRef<L.Map | null>(null);
  const markerRef = useRef<L.Marker | null>(null);

  useEffect(() => {
    if (!elementRef.current) return;
    const map = L.map(elementRef.current, { zoomControl: false, scrollWheelZoom: false, dragging: true, doubleClickZoom: false });
    L.tileLayer("https://{s}.basemaps.cartocdn.com/light_all/{z}/{x}/{y}{r}.png", {
      attribution: "© OpenStreetMap contributors © CARTO",
      maxZoom: 20,
      subdomains: "abcd",
    }).addTo(map);
    L.control.zoom({ position: "bottomright" }).addTo(map);
    mapRef.current = map;
    return () => { markerRef.current = null; mapRef.current = null; map.remove(); };
  }, []);

  useEffect(() => {
    const map = mapRef.current;
    if (!map) return;
    const coordinates = propertyCoordinates(property, index);
    markerRef.current?.remove();
    const icon = L.divIcon({
      className: "owner-map-marker-wrap",
      html: '<span class="owner-map-marker"><i></i><b>HOME</b></span>',
      iconSize: [54, 58],
      iconAnchor: [27, 58],
    });
    markerRef.current = L.marker([coordinates.lat, coordinates.lng], { icon, title: nameOf(property) }).addTo(map);
    map.setView([coordinates.lat, coordinates.lng], 14, { animate: false });
    window.setTimeout(() => map.invalidateSize(), 0);
  }, [index, property]);

  return <div ref={elementRef} className="owner-overview-map" aria-label={`Approximate map position for ${nameOf(property)}`} />;
}

function dateKey(value: Date, timezone: string) {
  return new Intl.DateTimeFormat("en-CA", { year: "numeric", month: "2-digit", day: "2-digit", timeZone: timezone }).format(value);
}

function MiniCalendar({ reservations, timezone }: { reservations: ReservationData[]; timezone: string }) {
  const today = dateKey(new Date(), timezone);
  const [year, month] = today.split("-").map(Number);
  const first = new Date(Date.UTC(year, month - 1, 1));
  const start = new Date(first);
  start.setUTCDate(start.getUTCDate() - first.getUTCDay());
  const days = Array.from({ length: 42 }, (_, offset) => {
    const date = new Date(start);
    date.setUTCDate(start.getUTCDate() + offset);
    return date.toISOString().slice(0, 10);
  });
  const occupied = useMemo(() => new Set(days.filter((day) => reservations.some((reservation) => {
    if (reservation.status.toLowerCase().includes("cancel")) return false;
    const startDay = dateKey(new Date(reservation.start_at), reservation.timezone || timezone);
    const endDay = dateKey(new Date(reservation.end_at), reservation.timezone || timezone);
    return day >= startDay && day <= endDay;
  }))), [days, reservations, timezone]);
  const monthLabel = new Intl.DateTimeFormat("en-US", { month: "long", year: "numeric", timeZone: "UTC" }).format(first);

  return (
    <div className="owner-mini-calendar">
      <header><span>BOOKING CALENDAR</span><strong>{monthLabel}</strong></header>
      <div className="owner-mini-weekdays" aria-hidden="true">{["S", "M", "T", "W", "T", "F", "S"].map((day, index) => <span key={`${day}-${index}`}>{day}</span>)}</div>
      <div className="owner-mini-days">
        {days.map((day) => <time key={day} dateTime={day} data-outside={Number(day.slice(5, 7)) !== month} data-today={day === today} data-occupied={occupied.has(day)}><b>{Number(day.slice(-2))}</b></time>)}
      </div>
      <footer><span><i /> RESERVED</span><span>{reservations.length} RECORDED STAY{reservations.length === 1 ? "" : "S"}</span></footer>
    </div>
  );
}

export default function OwnerPropertyOverview({ properties, selectedIndex }: { properties: DashboardProperty[]; selectedIndex: number }) {
  const property = properties[selectedIndex];
  const reservationsQuery = useQuery({
    queryKey: ["owner", "property-reservations", property.property.id],
    queryFn: () => getReservations(property.property.id),
  });
  const reservations = reservationsQuery.data?.items.map((entry) => entry.data) ?? [];
  const now = Date.now();
  const activeStays = reservations.filter((reservation) => !reservation.status.toLowerCase().includes("cancel") && new Date(reservation.start_at).getTime() <= now && new Date(reservation.end_at).getTime() >= now).length;
  const upcomingStays = reservations.filter((reservation) => {
    const starts = new Date(reservation.start_at).getTime();
    return !reservation.status.toLowerCase().includes("cancel") && starts > now && starts <= now + 30 * DAY_MS;
  }).length;
  const openWork = property.tickets.filter((ticket) => !CLOSED_WORK.has(ticket.data.status)).length;
  const completedWork = property.tickets.filter((ticket) => ticket.data.status === "verified" || ticket.data.status === "closed").length;
  const readiness = Object.values(property.property.data.readiness).filter(Boolean).length;
  const address = property.property.data.service_address;

  return (
    <section className="owner-overview" aria-labelledby="owner-overview-title">
      <header className="owner-overview-selector">
        <div><span>PROPERTY WORKSPACE</span><strong id="owner-overview-title">{nameOf(property)}</strong><small>{address.line1}, {address.city}</small></div>
        <div className="owner-overview-state"><span>OPERATING STATE</span><strong>{property.property.data.state.replaceAll("_", " ")}</strong><small>{readiness}/3 READY</small></div>
      </header>

      <div className="owner-overview-metrics">
        <div><span>ACTIVE STAYS</span><strong>{reservationsQuery.isLoading ? "—" : activeStays}</strong><small>NOW</small></div>
        <div><span>UPCOMING</span><strong>{reservationsQuery.isLoading ? "—" : upcomingStays}</strong><small>NEXT 30 DAYS</small></div>
        <div><span>OPEN WORK</span><strong>{openWork}</strong><small>OWNER VISIBLE</small></div>
        <div><span>COMPLETED</span><strong>{completedWork}</strong><small>RECORDED WORK</small></div>
        <div><span>READINESS</span><strong>{readiness}/3</strong><small>{property.property.data.state.replaceAll("_", " ")}</small></div>
        <div><span>NET CONTRIBUTION</span><Money value={property.contribution.data.net_contribution_minor_units} currency={property.contribution.data.currency} /><small>THIS MONTH</small></div>
      </div>

      <div className="owner-overview-body">
        <div className="owner-overview-map-panel">
          <OwnerMap property={property} index={selectedIndex} />
          <div><span>LIVE MAP TILES / APPROXIMATE ADDRESS POSITION</span><Link to={`/properties/${property.property.id}`}>OPEN PROPERTY →</Link></div>
        </div>
        <div className="owner-overview-calendar-panel">
          {reservationsQuery.isError ? <div className="owner-overview-calendar-state"><strong>BOOKINGS UNAVAILABLE</strong><button type="button" onClick={() => void reservationsQuery.refetch()}>RETRY →</button></div> : <MiniCalendar reservations={reservations} timezone={property.property.data.timezone || "Asia/Kolkata"} />}
          <div className="owner-overview-essentials">
            <div><span>PACKAGE</span><strong>{property.activePackage ? `ACTIVE V${property.activePackage.data.version_number}` : "NOT SET"}</strong></div>
            <div><span>DOCUMENTS</span><strong>{property.documents.length} ON FILE</strong></div>
            <Link to={`/properties/${property.property.id}/package`}>{property.activePackage ? "VIEW PACKAGE" : "BUILD PACKAGE"} →</Link>
          </div>
        </div>
      </div>
    </section>
  );
}
