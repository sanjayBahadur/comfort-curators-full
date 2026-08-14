import type { Envelope } from "../lib/api/client";
import type { OpsPropertyData, RequestedWindow, WorkerData } from "../lib/api/ops";

const DATE_TIME = new Intl.DateTimeFormat("en-IN", {
  day: "2-digit",
  month: "short",
  hour: "2-digit",
  minute: "2-digit",
  hour12: false,
  timeZone: "Asia/Kolkata",
});

export const humanize = (value: string) => value.replaceAll("_", " ");

export const propertyName = (property?: Envelope<OpsPropertyData>) =>
  property?.data.service_address.line2 || property?.data.service_address.line1 || "Unknown property";

export const propertyAddress = (property: Envelope<OpsPropertyData>) => {
  const address = property.data.service_address;
  return [address.line1, address.city, address.state, address.postal_code].filter(Boolean).join(", ");
};

export const workerName = (workerId: string | undefined, workers: Array<Envelope<WorkerData>>) =>
  workers.find((worker) => worker.id === workerId)?.data.legal_name ?? (workerId ? "Unknown worker" : "Unassigned");

export function formatWindow(window?: RequestedWindow) {
  if (!window?.start || !window.end) return "Window not set";
  return `${DATE_TIME.format(new Date(window.start))} — ${DATE_TIME.format(new Date(window.end))}`;
}

export function formatMoment(value: string) {
  return DATE_TIME.format(new Date(value));
}

export function ageLabel(value: string) {
  const minutes = Math.max(0, Math.floor((Date.now() - new Date(value).getTime()) / 60_000));
  if (minutes < 60) return `${minutes}m`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h`;
  return `${Math.floor(hours / 24)}d`;
}
