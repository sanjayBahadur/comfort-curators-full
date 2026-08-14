import type { Envelope } from "./api/client";
import type { OpsPropertyData } from "./api/ops";

export type DemoCoordinates = {
  lat: number;
  lng: number;
};

const NOIDA_SECTOR_18: DemoCoordinates = { lat: 28.5708, lng: 77.3210 };
const NOIDA_SECTOR_137: DemoCoordinates = { lat: 28.5098, lng: 77.4031 };

function idJitter(id: string, axis: number) {
  let hash = 0;
  for (let index = axis; index < id.length; index += 2) hash = (hash * 31 + id.charCodeAt(index)) | 0;
  return ((Math.abs(hash) % 2001) - 1000) / 1_000_000;
}

/**
 * Frontend-only demo coordinates. Property ids are generated per seed, so the
 * stable neighborhood anchor comes from the demo address and the returned
 * lookup is keyed by the live property id used everywhere else in the UI.
 */
export function getDemoPropertyLocations(properties: Array<Envelope<OpsPropertyData>>) {
  return Object.fromEntries(properties.map((property, index) => {
    const address = property.data.service_address;
    // The field-routing view deliberately stages the demo portfolio in Noida.
    // Stored service addresses remain untouched and continue to appear in records.
    const anchor = address.line1.toLocaleLowerCase("en-IN").includes("hazratganj")
      ? NOIDA_SECTOR_18
      : NOIDA_SECTOR_137;
    const offset = index * 0.00035;

    return [property.id, {
      lat: anchor.lat + idJitter(property.id, 0) + offset,
      lng: anchor.lng + idJitter(property.id, 1) - offset,
    }];
  })) as Record<string, DemoCoordinates>;
}

export function jitterDemoTicketLocation(location: DemoCoordinates, ticketId: string) {
  return {
    lat: location.lat + idJitter(ticketId, 1) * 0.55,
    lng: location.lng + idJitter(ticketId, 0) * 0.55,
  };
}
