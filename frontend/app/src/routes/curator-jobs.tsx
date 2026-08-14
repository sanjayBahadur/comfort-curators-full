import { useMemo } from "react";
import { Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { Navigate } from "react-router-dom";
import { getCuratorJobs, type TicketQueueRow } from "../lib/api/ops";
import { formatMoment, formatWindow, humanize, propertyName } from "./ops-format";
import { OpsSkeleton } from "./ops-shared";
import { getRole, getToken } from "../lib/auth/session";
import CuratorHeader from "./curator-header";
import "./curator.css";

function startLabel(ticket: TicketQueueRow) {
  return ticket.data.requested_window?.start ? formatMoment(ticket.data.requested_window.start) : "TIME TBC";
}

export default function CuratorJobs() {
  const query = useQuery({ queryKey: ["curator", "jobs"], queryFn: getCuratorJobs });
  const grouped = useMemo(() => {
    const jobs = query.data?.tickets ?? [];
    return jobs.reduce<Map<string, TicketQueueRow[]>>((groups, job) => {
      const day = job.data.requested_window?.start ? new Intl.DateTimeFormat("en-IN", { weekday: "long", day: "2-digit", month: "long", timeZone: "Asia/Kolkata" }).format(new Date(job.data.requested_window.start)) : "UNSCHEDULED";
      groups.set(day, [...(groups.get(day) ?? []), job]);
      return groups;
    }, new Map());
  }, [query.data?.tickets]);

  if (!getToken() || getRole() !== "staff") return <Navigate to="/login" replace />;

  return <main className="curator-shell">
    <CuratorHeader section="07 / FIELD WORK" />
    <section className="curator-hero"><p className="curator-kicker">07 / FIELD WORK · TODAY'S ASSIGNMENTS</p><h1>YOUR<br /><em>jobs.</em></h1><p>One clear brief at a time. Check the property notes, work the list, then leave a truthful evidence trail.</p></section>
    {query.isLoading ? <OpsSkeleton rows={5} /> : query.isError ? <section className="curator-empty"><strong>JOBS UNAVAILABLE.</strong><p>The backend message is shown in the toast. Try the field list again.</p></section> : grouped.size === 0 ? <section className="curator-empty"><strong>NO ASSIGNMENTS.</strong><p>There are no live dispatch offers for this staff session yet. Assigned work appears here after operations dispatches it.</p></section> : <div className="curator-list">{Array.from(grouped.entries()).map(([day, jobs]) => <section key={day}><div className="curator-day">{day} · {jobs.length} {jobs.length === 1 ? "JOB" : "JOBS"}</div>{jobs.map((job) => <Link className="curator-job-card" to={`/jobs/${job.id}`} key={job.id}><div className="curator-job-time">{startLabel(job)}<small>{formatWindow(job.data.requested_window)}</small></div><div><h2>{propertyName(job.property)}</h2><p>{humanize(job.data.type)} · {job.data.reason}</p></div><span>{humanize(job.data.status)}</span></Link>)}</section>)}</div>}
  </main>;
}
