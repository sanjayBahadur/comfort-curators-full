import { useEffect } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useSearchParams } from "react-router-dom";
import {
  generateTurnoverProposals,
  getCalendarHealth,
  getOpsProperties,
  getReservations,
  getTurnoverProposals,
  pollFeed,
} from "../lib/api/ops";
import { formatMoment, humanize, propertyName } from "./ops-format";
import { setCurrentProperty, clearCurrentProperty } from "../lib/current-property";
import { OpsHeader, OpsSkeleton, StaffGate, StatusLabel } from "./ops-shared";
import Select from "../components/ui/Select";
import CalendarGrid from "../components/calendar/CalendarGrid";
import "./ops.css";
import "./ops-calendar.css";

async function collectPropertyData<T>(propertyIds: string[], load: (propertyId: string) => Promise<T[]>) {
  const results = await Promise.allSettled(propertyIds.map(load));
  const successful = results.filter((result): result is PromiseFulfilledResult<T[]> => result.status === "fulfilled");
  if (propertyIds.length && successful.length === 0) {
    const failure = results.find((result): result is PromiseRejectedResult => result.status === "rejected");
    throw failure?.reason ?? new Error("Property data unavailable");
  }
  return {
    items: successful.flatMap((result) => result.value),
    failedProperties: results.length - successful.length,
  };
}

export default function OpsCalendar() {
  const [searchParams, setSearchParams] = useSearchParams();
  const queryClient = useQueryClient();
  const propertyQuery = useQuery({ queryKey: ["ops", "properties"], queryFn: getOpsProperties });
  const propertyId = searchParams.get("property") ?? "";
  const properties = propertyQuery.data?.items ?? [];
  const selectedProperty = properties.find((entry) => entry.id === propertyId);
  const scopedPropertyIds = propertyId ? [propertyId] : properties.map((property) => property.id);
  const scopeKey = scopedPropertyIds.join(",");
  const healthQuery = useQuery({
    queryKey: ["ops", "calendar-health-scope", scopeKey],
    queryFn: () => collectPropertyData(scopedPropertyIds, async (id) => (await getCalendarHealth(id)).feeds),
    enabled: !propertyQuery.isLoading && scopedPropertyIds.length > 0,
  });
  const reservationsQuery = useQuery({
    queryKey: ["ops", "reservations-scope", scopeKey],
    queryFn: () => collectPropertyData(scopedPropertyIds, async (id) => (await getReservations(id)).items),
    enabled: !propertyQuery.isLoading && scopedPropertyIds.length > 0,
  });
  const proposalsQuery = useQuery({
    queryKey: ["ops", "turnover-proposals-scope", scopeKey],
    queryFn: () => collectPropertyData(scopedPropertyIds, async (id) => (await getTurnoverProposals(id)).items),
    enabled: !propertyQuery.isLoading && scopedPropertyIds.length > 0,
  });
  const pollMutation = useMutation({
    mutationFn: pollFeed,
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["ops", "calendar-health-scope"] }),
        queryClient.invalidateQueries({ queryKey: ["ops", "reservations-scope"] }),
      ]);
    },
  });
  const generateMutation = useMutation({
    mutationFn: generateTurnoverProposals,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["ops", "turnover-proposals-scope"] });
    },
  });

  useEffect(() => {
    if (selectedProperty) setCurrentProperty({ id: selectedProperty.id, label: propertyName(selectedProperty) });
    else clearCurrentProperty();
    return clearCurrentProperty;
  }, [selectedProperty]);
  const feeds = healthQuery.data?.items ?? [];
  const reservations = reservationsQuery.data?.items ?? [];
  const proposals = proposalsQuery.data?.items ?? [];
  const summary = (() => {
    const now = Date.now();
    const activeReservations = reservations.filter((entry) => {
      const status = entry.data.status.toLowerCase();
      return !status.includes("cancel") && new Date(entry.data.start_at).getTime() <= now && new Date(entry.data.end_at).getTime() >= now;
    });
    const occupiedProperties = new Set(activeReservations.map((entry) => entry.data.property_id)).size;
    const upcomingReservations = reservations.filter((entry) => !entry.data.status.toLowerCase().includes("cancel") && new Date(entry.data.start_at).getTime() > now).length;
    const freshFeeds = feeds.filter((feed) => feed.fresh && !feed.stale).length;
    const openExceptions = feeds.reduce((total, feed) => total + feed.open_exceptions, 0);
    const propertyCount = scopedPropertyIds.length;
    const occupancy = propertyCount ? Math.round((occupiedProperties / propertyCount) * 100) : 0;
    const failedProperties = Math.max(
      healthQuery.data?.failedProperties ?? 0,
      reservationsQuery.data?.failedProperties ?? 0,
      proposalsQuery.data?.failedProperties ?? 0,
    );
    return { activeReservations: activeReservations.length, upcomingReservations, freshFeeds, openExceptions, propertyCount, occupancy, failedProperties };
  })();
  const scopeLabel = selectedProperty ? propertyName(selectedProperty) : "ALL PROPERTIES";
  const calendarLoading = propertyQuery.isLoading || reservationsQuery.isLoading;

  function setProperty(value: string) {
    setSearchParams((current) => {
      const next = new URLSearchParams(current);
      if (value) next.set("property", value);
      else next.delete("property");
      return next;
    }, { replace: true });
  }

  return (
    <StaffGate>
      <main className="ops-shell registration-frame">
        <OpsHeader section="04 / CALENDAR" />
        <section className="ops-title-row">
          <div>
            <p>RESERVATION IN. TURNOVER OUT.</p>
            <h1>Calendar</h1>
          </div>
          <strong>{propertyId ? "PROPERTY SCOPED" : "PORTFOLIO VIEW"}</strong>
        </section>

        <div className="calendar-control-deck">
          <section className="ops-filter-bar ops-calendar-selector" aria-label="Calendar property selector">
            <label>
              <span>PROPERTY</span>
              <Select
                value={propertyId}
                onChange={setProperty}
                options={[
                  { value: "", label: "ALL PROPERTIES" },
                  ...properties.map((property) => ({ value: property.id, label: propertyName(property) })),
                ]}
              />
            </label>
          </section>

          <section className="calendar-ops-terminal" aria-label="Property occupancy overview">
          <header><strong>SUPERHOST / OCCUPANCY OVERVIEW</strong><small>{scopeLabel}</small></header>
          <div className="calendar-ops-terminal-grid">
            <div><span>PROPERTY SCOPE</span><strong>{summary.propertyCount}</strong></div>
            <div><span>OCCUPIED NOW</span><strong>{summary.occupancy}%</strong></div>
            <div><span>ACTIVE STAYS</span><strong>{summary.activeReservations}</strong></div>
            <div><span>UPCOMING STAYS</span><strong>{summary.upcomingReservations}</strong></div>
            <div><span>FRESH FEEDS</span><strong>{summary.freshFeeds}/{feeds.length}</strong></div>
            <div><span>OPEN EXCEPTIONS</span><strong>{summary.openExceptions}</strong></div>
          </div>
          <p aria-live="polite">› {calendarLoading ? "READING CALENDAR SIGNALS..." : summary.propertyCount === 0 ? "NO PROPERTIES ARE AVAILABLE; THE CALENDAR REMAINS OPEN FOR PLANNING." : `${scopeLabel}: ${summary.activeReservations} active stay${summary.activeReservations === 1 ? "" : "s"} across ${summary.propertyCount} ${summary.propertyCount === 1 ? "property" : "properties"}, with ${summary.upcomingReservations} upcoming. ${summary.failedProperties ? `${summary.failedProperties} property data source${summary.failedProperties === 1 ? " is" : "s are"} currently unavailable.` : "All scoped property sources responded."}`}</p>
          </section>
        </div>

        {propertyQuery.isError && (
          <section className="ops-empty calendar-empty"><strong>PROPERTY LIST UNAVAILABLE.</strong><p>The backend message is shown in the toast. Retry to load the property list.</p><button type="button" onClick={() => void propertyQuery.refetch()}>RETRY</button></section>
        )}

        <section className="calendar-section">
          <header className="calendar-section-header"><div><span>01</span><h2>Reservations</h2></div><small>{`${reservations.length} RESERVATION${reservations.length === 1 ? "" : "S"}`}</small></header>
          {calendarLoading ? <OpsSkeleton rows={4} /> : reservationsQuery.isError ? <section className="ops-empty calendar-empty"><strong>RESERVATIONS UNAVAILABLE.</strong><p>The backend message is shown in the toast.</p><button type="button" onClick={() => void reservationsQuery.refetch()}>RETRY</button></section> : (
            <>
              <CalendarGrid key={propertyId || "portfolio"} reservations={reservations.map((reservation) => reservation.data)} timezone={selectedProperty?.data.timezone || "Asia/Kolkata"} renderStatus={(status) => <StatusLabel status={status} />} propertyLabel={(id) => propertyName(properties.find((property) => property.id === id))} />
              {reservations.length === 0 && <p className="calendar-grid-note">NO RESERVATIONS IN THE FEED YET. THE MONTH REMAINS OPEN FOR PLANNING.</p>}
            </>
          )}
        </section>

        <section className="calendar-section">
          <header className="calendar-section-header"><div><span>02</span><h2>Feed health</h2></div><small>{`${feeds.length} FEED${feeds.length === 1 ? "" : "S"}`}</small></header>
          {healthQuery.isLoading || propertyQuery.isLoading ? <OpsSkeleton rows={3} /> : healthQuery.isError ? (
            <section className="ops-empty calendar-empty"><strong>FEED HEALTH UNAVAILABLE.</strong><p>The backend message is shown in the toast.</p><button type="button" onClick={() => void healthQuery.refetch()}>RETRY</button></section>
          ) : feeds.length === 0 ? (
            <section className="ops-empty calendar-empty"><strong>NO CALENDAR FEEDS.</strong><p>This property has no connected calendar feed yet.</p></section>
          ) : (
            <div className="calendar-feed-list">
              {feeds.map((health) => (
                <article className={`calendar-feed-card${health.stale ? " calendar-feed-card--stale" : ""}`} key={health.feed.id}>
                  <header><div><strong>{health.feed.source}</strong><small>{health.feed.id}</small></div><StatusLabel status={health.feed.status} /></header>
                  <div className="calendar-metrics">
                    <div><span>FRESHNESS</span><strong className={health.stale ? "calendar-alert" : ""}>{health.stale ? "STALE" : health.fresh ? "FRESH" : "UNKNOWN"}</strong></div>
                    <div><span>OPEN EXCEPTIONS</span><strong>{health.open_exceptions}</strong></div>
                    <div><span>LAST SUCCESS</span><strong>{health.last_success_at ? formatMoment(health.last_success_at) : "NEVER"}</strong></div>
                  </div>
                  {health.last_error && <p className="calendar-error">{health.last_error}</p>}
                  <button className="calendar-action" type="button" disabled={pollMutation.isPending} onClick={() => pollMutation.mutate(health.feed.id)}>
                    {pollMutation.isPending ? "POLLING..." : "POLL FEED"}
                  </button>
                  {pollMutation.data?.result.feed_id === health.feed.id && (
                    <div className="calendar-result" role="status"><strong>POLL RESULT</strong><span>{pollMutation.data.result.events_parsed} parsed · {pollMutation.data.result.events_created} created · {pollMutation.data.result.events_updated} updated · {pollMutation.data.result.events_skipped} skipped · {pollMutation.data.result.events_cancelled} cancelled</span></div>
                  )}
                </article>
              ))}
            </div>
          )}
        </section>

        <section className="calendar-section">
          <header className="calendar-section-header"><div><span>03</span><h2>Turnover proposals</h2></div>{propertyId && <button className="calendar-generate" type="button" disabled={generateMutation.isPending} onClick={() => generateMutation.mutate(propertyId)}>{generateMutation.isPending ? "GENERATING..." : "GENERATE TURNOVER PROPOSALS"}</button>}</header>
          {proposalsQuery.isLoading || propertyQuery.isLoading ? <OpsSkeleton rows={4} /> : proposalsQuery.isError ? <section className="ops-empty calendar-empty"><strong>PROPOSALS UNAVAILABLE.</strong><p>The backend message is shown in the toast.</p><button type="button" onClick={() => void proposalsQuery.refetch()}>RETRY</button></section> : proposals.length === 0 ? <section className="ops-empty calendar-empty"><strong>NO TURNOVER PROPOSALS YET.</strong><p>{propertyId ? "Generate them once reservations exist for this property." : "Select one property to generate proposals, or review the portfolio calendar above."}</p></section> : (
            <div className="ops-table-wrap calendar-table-wrap"><table className="ops-table"><thead><tr><th>KIND</th><th>STATUS</th><th>SCHEDULED</th><th>CHECKLIST HINT</th></tr></thead><tbody>{proposals.map((proposal) => <tr key={proposal.id}><td data-label="KIND"><strong>{humanize(proposal.data.kind)}</strong><small>{proposal.data.id}</small></td><td data-label="STATUS"><StatusLabel status={proposal.data.status} /></td><td data-label="SCHEDULED">{formatMoment(proposal.data.scheduled_at)}</td><td data-label="CHECKLIST HINT">{proposal.data.checklist_hint || "No checklist hint"}</td></tr>)}</tbody></table></div>
          )}
          {generateMutation.data && <div className="calendar-result calendar-generation-result" role="status"><strong>GENERATION RESULT</strong><span>{generateMutation.data.result.proposed} proposed · {generateMutation.data.result.updated} updated · {generateMutation.data.result.cancelled} cancelled{generateMutation.data.result.skipped ? " · skipped" : ""}{generateMutation.data.result.reason ? ` · ${generateMutation.data.result.reason}` : ""}</span></div>}
        </section>
      </main>
    </StaffGate>
  );
}
