import type { Role } from "./session";

export type NavItem = {
  label: string;
  path: string;
};

const ownerNav: readonly NavItem[] = [
  { label: "Dashboard", path: "/dashboard" },
  { label: "Properties", path: "/properties" },
  { label: "Package", path: "/properties/:propertyId/package" },
  { label: "Invoices", path: "/invoices" },
  { label: "Documents", path: "/documents" },
];

const staffNav: readonly NavItem[] = [
  { label: "Tickets", path: "/ops/tickets" },
  { label: "Properties", path: "/ops/properties" },
  { label: "Workers", path: "/ops/workers" },
  { label: "Calendar", path: "/ops/calendar" },
];

const guestNav: readonly NavItem[] = [
  { label: "Stay", path: "/stay" },
  { label: "Store", path: "/stay/store" },
];

export const homeFor = (role: Role): string =>
  role === "owner" ? "/dashboard" : role === "staff" ? "/ops/tickets" : "/stay";

export const navFor = (role: Role): readonly NavItem[] =>
  role === "owner" ? ownerNav : role === "staff" ? staffNav : guestNav;

function isPathOrChild(path: string, base: string): boolean {
  return path === base || path.startsWith(`${base}/`);
}

function isPropertyPath(path: string): boolean {
  return /^\/properties\/[^/]+(?:\/package)?$/.test(path);
}

export const allows = (role: Role, path: string): boolean => {
  if (path === "/" || path === "/login" || path === "/debug") return true;

  if (role === "owner") {
    return (
      path === "/dashboard" ||
      path === "/properties" ||
      isPropertyPath(path) ||
      path === "/onboarding" ||
      path === "/invoices" ||
      path === "/documents"
    );
  }

  if (role === "staff") {
    return (
      isPathOrChild(path, "/ops/tickets") ||
      isPathOrChild(path, "/ops/properties") ||
      isPathOrChild(path, "/ops/workers") ||
      isPathOrChild(path, "/ops/calendar") ||
      isPathOrChild(path, "/jobs")
    );
  }

  return isPathOrChild(path, "/stay");
};
