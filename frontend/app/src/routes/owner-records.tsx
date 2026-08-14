import type { ReactNode } from "react";
import { Link, Navigate, useLocation } from "react-router-dom";
import { getRole, getToken } from "../lib/auth/session";

export function OwnerGate({ children }: { children: ReactNode }) {
  if (!getToken() || getRole() !== "owner") return <Navigate to="/login" replace />;
  return children;
}

export function OwnerRecordsHeader({ section }: { section: string }) {
  const location = useLocation();
  const isActive = (to: string) => location.pathname === to || location.pathname.startsWith(`${to}/`);
  return (
    <header className="owner-records-header">
      <Link to="/dashboard" className="owner-records-wordmark">COMFORT CURATORS / OWNER</Link>
      <span>{section}</span>
      <nav aria-label="Owner navigation">
        <Link to="/dashboard" aria-current={isActive("/dashboard") ? "page" : undefined}>DASHBOARD</Link>
        <Link to="/onboarding" aria-current={isActive("/onboarding") ? "page" : undefined}>ONBOARD</Link>
        <Link to="/invoices" aria-current={isActive("/invoices") ? "page" : undefined}>INVOICES</Link>
        <Link to="/documents" aria-current={isActive("/documents") ? "page" : undefined}>DOCUMENTS</Link>
        <Link to="/login" aria-current={isActive("/login") ? "page" : undefined}>ACCESS DESK</Link>
      </nav>
    </header>
  );
}

export function OwnerRecordsSkeleton() {
  return (
    <div className="owner-records-skeleton" aria-label="Loading owner records" aria-busy="true">
      <span /><span /><span /><span />
    </div>
  );
}
