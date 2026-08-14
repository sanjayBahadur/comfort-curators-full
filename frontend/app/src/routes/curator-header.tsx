import { Link, useLocation } from "react-router-dom";

export default function CuratorHeader({ section }: { section: string }) {
  const location = useLocation();
  const isActive = (to: string) => location.pathname === to || location.pathname.startsWith(`${to}/`);

  return (
    <header className="curator-header">
      <Link className="curator-wordmark" to="/jobs">COMFORT CURATORS / CURATOR</Link>
      <span>{section}</span>
      <nav aria-label="Curator navigation">
        <Link to="/jobs" aria-current={location.pathname === "/jobs" || location.pathname.startsWith("/jobs/") && !location.pathname.startsWith("/jobs/map") && !location.pathname.startsWith("/jobs/properties/") ? "page" : undefined}>JOBS</Link>
        <Link to="/jobs/map" aria-current={isActive("/jobs/map") || isActive("/jobs/properties") ? "page" : undefined}>ZONE MAP</Link>
        <Link to="/ops/tickets" aria-current={isActive("/ops") ? "page" : undefined}>OPS</Link>
        <Link to="/login" aria-current={isActive("/login") ? "page" : undefined}>ACCESS</Link>
      </nav>
    </header>
  );
}
