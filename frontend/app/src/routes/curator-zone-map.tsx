import { useEffect, useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import L from "leaflet";
import { Link, Navigate } from "react-router-dom";
import { getTicketQueue, type TicketQueueRow } from "../lib/api/ops";
import type { Envelope } from "../lib/api/client";
import type { OpsPropertyData } from "../lib/api/ops";
import { getRole, getToken } from "../lib/auth/session";
import { getDemoPropertyLocations, jitterDemoTicketLocation, type DemoCoordinates } from "../lib/demo-property-locations";
import { getPropertyImage } from "../lib/property-images";
import { humanize, propertyAddress, propertyName } from "./ops-format";
import { OpsSkeleton } from "./ops-shared";
import "leaflet/dist/leaflet.css";
import "./curator.css";
import CuratorHeader from "./curator-header";

const CLOSED_TICKET_STATES = new Set(["closed", "cancelled", "rejected"]);
const UNZONED = "UNZONED";

type Marker =
  | { kind: "property"; id: string; property: Envelope<OpsPropertyData> }
  | { kind: "ticket"; id: string; ticket: TicketQueueRow };

type ZoneGroup = {
  zone: string;
  properties: Array<Envelope<OpsPropertyData>>;
  tickets: TicketQueueRow[];
};

type TripEstimate = {
  distanceKm: number;
  minutes: number;
  traffic: "LIGHT" | "MODERATE" | "HEAVY";
};

const lucideIcon = (kind: "property" | "ticket" | "curator") => {
  if (kind === "property") return '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M15 22v-4a2 2 0 0 0-2-2h-2a2 2 0 0 0-2 2v4"/><path d="M18 10 12 4l-6 6"/><path d="M6 10v10a2 2 0 0 0 2 2h8a2 2 0 0 0 2-2V10"/></svg>';
  if (kind === "ticket") return '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.8-3.8a6 6 0 0 1-7.9 7.9l-6.9 6.9a2.1 2.1 0 0 1-3-3l6.9-6.9a6 6 0 0 1 7.9-7.9z"/></svg>';
  return '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="m3 11 19-9-9 19-2-8z"/></svg>';
};

function zoneFor(property: Envelope<OpsPropertyData>) {
  return property.data.geolocation_zone?.trim() || UNZONED;
}

function markerKey(marker?: Marker) {
  return marker ? `${marker.kind}:${marker.id}` : null;
}

function distanceKm(from: DemoCoordinates, to: DemoCoordinates) {
  const toRadians = (degrees: number) => degrees * Math.PI / 180;
  const latitudeDelta = toRadians(to.lat - from.lat);
  const longitudeDelta = toRadians(to.lng - from.lng);
  const fromLatitude = toRadians(from.lat);
  const toLatitude = toRadians(to.lat);
  const chord = Math.sin(latitudeDelta / 2) ** 2
    + Math.cos(fromLatitude) * Math.cos(toLatitude) * Math.sin(longitudeDelta / 2) ** 2;
  return 6371 * 2 * Math.atan2(Math.sqrt(chord), Math.sqrt(1 - chord));
}

function estimateTrip(from: DemoCoordinates, to: DemoCoordinates): TripEstimate {
  const distance = distanceKm(from, to);
  // Demo estimate only: deterministic client arithmetic, not GPS, routing, or a live traffic API.
  const minutes = Math.max(4, Math.round((distance / 18) * 60 * 1.35 + 3));
  const traffic = minutes >= 24 ? "HEAVY" : minutes >= 10 ? "MODERATE" : "LIGHT";
  return { distanceKm: distance, minutes, traffic };
}

function markerCoordinates(marker: Marker, locations: Record<string, DemoCoordinates>) {
  const property = marker.kind === "property" ? marker.property : marker.ticket.property;
  return locations[property.id];
}

function MapCanvas({
  properties,
  tickets,
  locations,
  curatorLocation,
  selected,
  zoomEnabled,
  plottedTickets,
  onSelect,
}: {
  properties: Array<Envelope<OpsPropertyData>>;
  tickets: TicketQueueRow[];
  locations: Record<string, DemoCoordinates>;
  curatorLocation: DemoCoordinates;
  selected?: Marker;
  zoomEnabled: boolean;
  plottedTickets: TicketQueueRow[];
  onSelect: (key: string) => void;
}) {
  const containerRef = useRef<HTMLDivElement>(null);
  const mapRef = useRef<L.Map | null>(null);
  const markerLayerRef = useRef<L.LayerGroup | null>(null);
  const routeLayerRef = useRef<L.LayerGroup | null>(null);
  const hasFitRef = useRef(false);

  useEffect(() => {
    if (!containerRef.current) return;

    const map = L.map(containerRef.current, { zoomControl: false, scrollWheelZoom: false, doubleClickZoom: false, touchZoom: false, boxZoom: false });
    mapRef.current = map;
    L.tileLayer("https://{s}.basemaps.cartocdn.com/light_all/{z}/{x}/{y}{r}.png", {
      attribution: "© OpenStreetMap contributors © CARTO",
      maxZoom: 20,
      subdomains: "abcd",
    }).addTo(map);
    markerLayerRef.current = L.layerGroup().addTo(map);
    routeLayerRef.current = L.layerGroup().addTo(map);

    return () => {
      mapRef.current = null;
      markerLayerRef.current = null;
      routeLayerRef.current = null;
      hasFitRef.current = false;
      map.remove();
    };
  }, []);

  useEffect(() => {
    const map = mapRef.current;
    if (!map) return;
    const action = zoomEnabled ? "enable" : "disable";
    map.scrollWheelZoom[action]();
    map.doubleClickZoom[action]();
    map.touchZoom[action]();
    map.boxZoom[action]();
  }, [zoomEnabled]);

  useEffect(() => {
    const map = mapRef.current;
    const markerLayer = markerLayerRef.current;
    if (!map || !markerLayer) return;
    markerLayer.clearLayers();

    const bounds: L.LatLngExpression[] = [[curatorLocation.lat, curatorLocation.lng]];
    const propertyNumber = new Map(properties.map((property, index) => [property.id, index + 1]));

    for (const property of properties) {
      const coordinates = locations[property.id];
      if (!coordinates) continue;
      const marker: Marker = { kind: "property", id: property.id, property };
      const key = markerKey(marker)!;
      const isSelected = key === markerKey(selected);
      const icon = L.divIcon({
        className: "curator-leaflet-icon",
        html: `<span class="curator-map-property${isSelected ? " is-selected" : ""}">${lucideIcon("property")}<b>P${String(propertyNumber.get(property.id)).padStart(2, "0")}</b></span>`,
        iconSize: [42, 50],
        iconAnchor: [21, 50],
      });
      L.marker([coordinates.lat, coordinates.lng], { icon, title: propertyName(property), alt: propertyName(property) })
        .on("click", () => onSelect(key))
        .addTo(markerLayer);
      bounds.push([coordinates.lat, coordinates.lng]);
    }

    tickets.forEach((ticket, index) => {
      const propertyLocation = locations[ticket.property.id];
      if (!propertyLocation) return;
      const coordinates = jitterDemoTicketLocation(propertyLocation, ticket.id);
      const marker: Marker = { kind: "ticket", id: ticket.id, ticket };
      const key = markerKey(marker)!;
      const isSelected = key === markerKey(selected);
      const icon = L.divIcon({
        className: "curator-leaflet-icon",
        html: `<span class="curator-map-ticket${isSelected ? " is-selected" : ""}">${lucideIcon("ticket")}<b>T${String(index + 1).padStart(2, "0")}</b></span>`,
        iconSize: [42, 50],
        iconAnchor: [21, 50],
      });
      L.marker([coordinates.lat, coordinates.lng], {
        icon,
        title: `${humanize(ticket.data.type)} · ${propertyName(ticket.property)}`,
        alt: `${humanize(ticket.data.type)} at ${propertyName(ticket.property)}`,
        zIndexOffset: 200,
      }).on("click", () => onSelect(key)).addTo(markerLayer);
      bounds.push([coordinates.lat, coordinates.lng]);
    });

    const curatorIcon = L.divIcon({
      className: "curator-leaflet-icon",
      html: `<span class="curator-map-current">${lucideIcon("curator")}<b>YOU</b></span>`,
      iconSize: [42, 42],
      iconAnchor: [21, 21],
    });
    L.marker([curatorLocation.lat, curatorLocation.lng], {
      icon: curatorIcon,
      title: "Curator current location · simulated",
      alt: "Simulated curator current location",
      zIndexOffset: 400,
    }).bindTooltip("CURATOR · SIMULATED POSITION", { direction: "bottom", offset: [0, 13] }).addTo(markerLayer);

    if (!hasFitRef.current) {
      map.fitBounds(L.latLngBounds(bounds).pad(0.22), { maxZoom: 14, padding: [34, 34] });
      hasFitRef.current = true;
      window.setTimeout(() => map.invalidateSize(), 0);
    }
  }, [curatorLocation, locations, onSelect, properties, selected, tickets]);

  useEffect(() => {
    const routeLayer = routeLayerRef.current;
    if (!routeLayer) return;
    routeLayer.clearLayers();

    plottedTickets.forEach((ticket, index) => {
      const propertyLocation = locations[ticket.property.id];
      if (!propertyLocation) return;
      const destination = jitterDemoTicketLocation(propertyLocation, ticket.id);
      const isSelectedRoute = selected?.kind === "ticket" && selected.id === ticket.id;
      L.polyline([
        [curatorLocation.lat, curatorLocation.lng],
        [destination.lat, destination.lng],
      ], {
        color: isSelectedRoute ? "#0d3f23" : "#1a6b3c",
        weight: isSelectedRoute ? 5 : Math.max(2, 3.5 - index * 0.35),
        opacity: isSelectedRoute ? 1 : Math.max(0.42, 0.82 - index * 0.09),
        dashArray: isSelectedRoute ? undefined : "6 7",
      }).addTo(routeLayer);
      L.circleMarker([destination.lat, destination.lng], {
        radius: isSelectedRoute ? 7 : 5,
        color: isSelectedRoute ? "#0d3f23" : "#1a6b3c",
        fillColor: isSelectedRoute ? "#0d3f23" : "#1a6b3c",
        fillOpacity: 1,
        weight: isSelectedRoute ? 2 : 1,
      }).addTo(routeLayer);
    });

    const selectedAlreadyPlotted = selected?.kind === "ticket" && plottedTickets.some((ticket) => ticket.id === selected.id);
    if (selected && !selectedAlreadyPlotted) {
      const destination = markerCoordinates(selected, locations);
      if (destination) {
        const estimate = estimateTrip(curatorLocation, destination);
        L.polyline([
          [curatorLocation.lat, curatorLocation.lng],
          [destination.lat, destination.lng],
        ], { color: "#000000", weight: 2, dashArray: "7 7" }).addTo(routeLayer);
        L.circleMarker([destination.lat, destination.lng], {
          radius: 4,
          color: "#ff0000",
          fillColor: "#ff0000",
          fillOpacity: 1,
          weight: 1,
        }).bindTooltip(`~${estimate.minutes} MIN · ${estimate.traffic} TRAFFIC<br><small>DEMO ESTIMATE</small>`, {
          className: "curator-route-tooltip",
          direction: "top",
          offset: [0, -8],
          permanent: true,
        }).addTo(routeLayer);
      }
    }
  }, [curatorLocation, locations, plottedTickets, selected]);

  return <div className="curator-real-map" ref={containerRef} aria-label="Map of Noida properties, open tickets, and simulated curator position" />;
}

export default function CuratorZoneMap() {
  const query = useQuery({ queryKey: ["ops", "ticket-queue"], queryFn: getTicketQueue });
  const [selectionKey, setSelectionKey] = useState<string | null>(null);
  const [zoomEnabled, setZoomEnabled] = useState(false);
  const [proximityOpen, setProximityOpen] = useState(false);

  const groups = useMemo(() => {
    const byZone = new Map<string, ZoneGroup>();
    const ensure = (zone: string) => {
      const current = byZone.get(zone) ?? { zone, properties: [], tickets: [] };
      byZone.set(zone, current);
      return current;
    };

    for (const property of query.data?.properties ?? []) ensure(zoneFor(property)).properties.push(property);
    for (const ticket of query.data?.tickets ?? []) {
      if (!CLOSED_TICKET_STATES.has(ticket.data.status)) ensure(zoneFor(ticket.property)).tickets.push(ticket);
    }

    return Array.from(byZone.values()).sort((left, right) => {
      if (left.zone === UNZONED) return 1;
      if (right.zone === UNZONED) return -1;
      return left.zone.localeCompare(right.zone);
    });
  }, [query.data]);

  const markers = useMemo<Marker[]>(() => groups.flatMap((group) => [
    ...group.properties.map((property): Marker => ({ kind: "property", id: property.id, property })),
    ...group.tickets.map((ticket): Marker => ({ kind: "ticket", id: ticket.id, ticket })),
  ]), [groups]);
  const properties = useMemo(() => query.data?.properties ?? [], [query.data?.properties]);
  const openTickets = useMemo(() => groups.flatMap((group) => group.tickets), [groups]);
  const locations = useMemo(() => getDemoPropertyLocations(properties), [properties]);
  const busiestProperty = useMemo(() => {
    const counts = new Map<string, number>();
    for (const ticket of openTickets) counts.set(ticket.property.id, (counts.get(ticket.property.id) ?? 0) + 1);
    return properties.reduce<Envelope<OpsPropertyData> | undefined>((busiest, property) => {
      if (!busiest) return property;
      return (counts.get(property.id) ?? 0) > (counts.get(busiest.id) ?? 0) ? property : busiest;
    }, undefined);
  }, [openTickets, properties]);
  const curatorLocation = useMemo(() => {
    const anchor = busiestProperty ? locations[busiestProperty.id] : undefined;
    return anchor ? { lat: anchor.lat - 0.0064, lng: anchor.lng - 0.0078 } : { lat: 28.563, lng: 77.334 };
  }, [busiestProperty, locations]);
  const proximityTickets = useMemo(() => [...openTickets].sort((left, right) => {
    const leftAnchor = locations[left.property.id];
    const rightAnchor = locations[right.property.id];
    if (!leftAnchor) return 1;
    if (!rightAnchor) return -1;
    const leftLocation = jitterDemoTicketLocation(leftAnchor, left.id);
    const rightLocation = jitterDemoTicketLocation(rightAnchor, right.id);
    return distanceKm(curatorLocation, leftLocation) - distanceKm(curatorLocation, rightLocation);
  }).slice(0, 5), [curatorLocation, locations, openTickets]);
  const nearestTicket = useMemo(() => openTickets.reduce<TicketQueueRow | undefined>((nearest, ticket) => {
    if (!nearest) return ticket;
    const ticketLocation = locations[ticket.property.id];
    const nearestLocation = locations[nearest.property.id];
    if (!ticketLocation) return nearest;
    if (!nearestLocation) return ticket;
    return distanceKm(curatorLocation, ticketLocation) < distanceKm(curatorLocation, nearestLocation) ? ticket : nearest;
  }, undefined), [curatorLocation, locations, openTickets]);
  const defaultMarker: Marker | undefined = nearestTicket
    ? { kind: "ticket", id: nearestTicket.id, ticket: nearestTicket }
    : markers[0];
  const selected = markers.find((marker) => markerKey(marker) === selectionKey) ?? defaultMarker;
  const selectedCoordinates = selected ? markerCoordinates(selected, locations) : undefined;
  const selectedEstimate = selectedCoordinates ? estimateTrip(curatorLocation, selectedCoordinates) : undefined;

  if (!getToken() || getRole() !== "staff") return <Navigate to="/login" replace />;

  return (
    <main className="curator-shell">
      <CuratorHeader section="08 / ZONE MAP" />

      <section className="curator-map-intro">
        <p className="curator-kicker">08 / FIELD WORK · LIVE ROUTE VIEW</p>
        <h1>Know the<br /><em>route.</em></h1>
        <p>Open work across Noida, staged for the field team. Select a circular marker for its property context, travel distance, and demo traffic estimate.</p>
      </section>

      {query.isLoading ? <OpsSkeleton rows={6} /> : query.isError ? (
        <section className="curator-empty"><strong>ZONE VIEW UNAVAILABLE.</strong><p>The location records could not be read. Try the field view again.</p><button type="button" onClick={() => void query.refetch()}>RETRY</button></section>
      ) : groups.length === 0 ? (
        <section className="curator-empty"><strong>NO LOCATIONS YET.</strong><p>Properties appear here once operations has a location record.</p></section>
      ) : (
        <>
          <section className="curator-map-key" aria-label="Map key"><span><i data-kind="property" />PROPERTY</span><span><i data-kind="ticket" />OPEN TICKET</span><span><i data-kind="current" />CURATOR</span><small>{groups.length} ZONES · {openTickets.length} OPEN TICKETS</small></section>
          <section className="curator-map-stage" data-zoom-enabled={zoomEnabled}>
            <button className="curator-proximity-trigger" type="button" aria-expanded={proximityOpen} aria-controls="curator-proximity-panel" onClick={() => setProximityOpen((current) => !current)}>{proximityOpen ? "NAVIGATING NEARBY WORKS ×" : "NAVIGATE NEARBY WORKS →"}</button>
            {proximityOpen && <aside className="curator-proximity-panel" id="curator-proximity-panel" aria-label="Five nearest open jobs">
              <header><span>PROXIMITY INDEX</span><strong>Nearest work</strong><button type="button" aria-label="Close nearest work" onClick={() => setProximityOpen(false)}>×</button></header>
              <ol>{proximityTickets.map((ticket, index) => {
                const anchor = locations[ticket.property.id];
                const coordinates = anchor ? jitterDemoTicketLocation(anchor, ticket.id) : undefined;
                const estimate = coordinates ? estimateTrip(curatorLocation, coordinates) : undefined;
                const image = getPropertyImage(ticket.property.data.service_address);
                return <li key={ticket.id}><button type="button" onClick={() => setSelectionKey(`ticket:${ticket.id}`)}>{image ? <img src={image.src} alt="" /> : <span className="curator-proximity-fallback">{String(index + 1).padStart(2, "0")}</span>}<span><small>T{String(index + 1).padStart(2, "0")} · {humanize(ticket.data.type)}</small><strong>{propertyName(ticket.property)}</strong><em>{estimate ? `${estimate.distanceKm.toFixed(1)} KM · ~${estimate.minutes} MIN · ${estimate.traffic}` : "ESTIMATE UNAVAILABLE"}</em></span></button></li>;
              })}</ol>
            </aside>}
            <div className="curator-map-mode">
              <span>{zoomEnabled ? "MAP ZOOM ON" : "PAGE SCROLL ON"}</span>
              <button type="button" aria-pressed={zoomEnabled} onClick={() => setZoomEnabled((current) => !current)}>{zoomEnabled ? "SCROLL PAGE" : "ENABLE MAP ZOOM"}</button>
            </div>
            <MapCanvas properties={properties} tickets={proximityTickets.concat(openTickets.filter((ticket) => !proximityTickets.some((nearby) => nearby.id === ticket.id)))} locations={locations} curatorLocation={curatorLocation} selected={selected} zoomEnabled={zoomEnabled} plottedTickets={proximityOpen ? proximityTickets : []} onSelect={setSelectionKey} />
          </section>

          {selected && <aside className="curator-marker-detail" aria-live="polite">
            <div><span>SELECTED / {selected.kind}</span><h2>{selected.kind === "property" ? propertyName(selected.property) : humanize(selected.ticket.data.type)}</h2></div>
            {selected.kind === "property" ? (
              <dl><div><dt>ADDRESS</dt><dd>{propertyAddress(selected.property)}</dd></div><div><dt>ZONE</dt><dd>{zoneFor(selected.property)}</dd></div><div><dt>DISTANCE</dt><dd>{selectedEstimate?.distanceKm.toFixed(1)} KM</dd></div><div><dt>DEMO ETA</dt><dd>~{selectedEstimate?.minutes} MIN · {selectedEstimate?.traffic} TRAFFIC</dd></div></dl>
            ) : (
              <dl><div><dt>PROPERTY</dt><dd>{propertyName(selected.ticket.property)}</dd></div><div><dt>ADDRESS</dt><dd>{propertyAddress(selected.ticket.property)}</dd></div><div><dt>DISTANCE</dt><dd>{selectedEstimate?.distanceKm.toFixed(1)} KM</dd></div><div><dt>DEMO ETA</dt><dd>~{selectedEstimate?.minutes} MIN · {selectedEstimate?.traffic} TRAFFIC</dd></div><div><dt>ZONE</dt><dd>{zoneFor(selected.ticket.property)}</dd></div><div><dt>STATUS</dt><dd>{humanize(selected.ticket.data.status)}</dd></div></dl>
            )}
            <Link to={selected.kind === "property" ? `/jobs/properties/${selected.id}` : `/jobs/${selected.id}`}>OPEN {selected.kind} RECORD →</Link>
          </aside>}
        </>
      )}
    </main>
  );
}
