import type { ReactNode } from "react";
import { Link, Navigate, useLocation } from "react-router-dom";
import { getRole, getToken } from "../lib/auth/session";
import type { TicketStatus } from "../lib/api/ops";
import { humanize } from "./ops-format";

export function StaffGate({ children }: { children: ReactNode }) {
  if (!getToken() || getRole() !== "staff") return <Navigate to="/login" replace />;
  return children;
}

export function OpsHeader({ section, action }: { section: string; action?: ReactNode }) {
  const location = useLocation();
  const isActive = (to: string) => location.pathname === to || location.pathname.startsWith(`${to}/`);
  return (
    <header className="ops-header">
      <Link to="/ops/tickets" className="ops-wordmark">COMFORT CURATORS / OPS</Link>
      <span>{section}</span>
      <nav aria-label="Operations navigation">
        <Link to="/ops/tickets" aria-current={isActive("/ops/tickets") ? "page" : undefined}>TICKETS</Link>
        <Link to="/ops/properties" aria-current={isActive("/ops/properties") ? "page" : undefined}>PROPERTIES</Link>
        <Link to="/ops/workers" aria-current={isActive("/ops/workers") ? "page" : undefined}>WORKERS</Link>
        <Link to="/ops/calendar" aria-current={isActive("/ops/calendar") ? "page" : undefined}>CALENDAR</Link>
        <Link to="/jobs" aria-current={isActive("/jobs") ? "page" : undefined}>CURATOR</Link>
        <Link to="/debug#seed-reset" aria-current={isActive("/debug") ? "page" : undefined}>SEED</Link>
      </nav>
      <nav className="ops-subnav" aria-label="Operations actions">
        <span className="ops-subnav-label">WORKSPACE ACTIONS</span>
        <span className="ops-subnav-actions">
          {action}
          <Link to="/login" aria-current={isActive("/login") ? "page" : undefined}>ACCESS DESK</Link>
        </span>
      </nav>
    </header>
  );
}

export function StatusLabel({ status }: { status: TicketStatus | string }) {
  return <span className="ops-status" data-status={status}><i aria-hidden="true" />{humanize(status)}</span>;
}

export function OpsSkeleton({ rows = 5 }: { rows?: number }) {
  return (
    <div className="ops-skeleton" aria-label="Loading operations data">
      {Array.from({ length: rows }, (_, index) => <span key={index} />)}
    </div>
  );
}
