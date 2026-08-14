import { useMemo, useState, type CSSProperties, type ReactNode } from "react";
import type { ReservationData } from "../../lib/api/ops";
import Modal from "../ui/Modal";
import "./CalendarGrid.css";

type CalendarGridProps = {
  reservations: ReservationData[];
  timezone: string;
  renderStatus: (status: string) => ReactNode;
  propertyLabel?: (propertyId: string) => string;
};

type ReservationSegment = {
  reservation: ReservationData;
  startColumn: number;
  span: number;
  lane: number;
  continuesBefore: boolean;
  continuesAfter: boolean;
};

const DAY_NAMES = ["SUN", "MON", "TUE", "WED", "THU", "FRI", "SAT"];
const DATE_KEY_FORMATTERS = new Map<string, Intl.DateTimeFormat>();

function dateKey(value: Date, timezone: string) {
  let formatter = DATE_KEY_FORMATTERS.get(timezone);
  if (!formatter) {
    formatter = new Intl.DateTimeFormat("en-CA", {
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      timeZone: timezone,
    });
    DATE_KEY_FORMATTERS.set(timezone, formatter);
  }
  const parts = formatter.formatToParts(value);
  const part = (type: Intl.DateTimeFormatPartTypes) => parts.find((entry) => entry.type === type)?.value ?? "";
  return `${part("year")}-${part("month")}-${part("day")}`;
}

function dateFromKey(key: string) {
  const [year, month, day] = key.split("-").map(Number);
  return new Date(Date.UTC(year, month - 1, day));
}

function keyFromDate(value: Date) {
  return value.toISOString().slice(0, 10);
}

function addDays(key: string, amount: number) {
  const value = dateFromKey(key);
  value.setUTCDate(value.getUTCDate() + amount);
  return keyFromDate(value);
}

function dayDistance(start: string, end: string) {
  return Math.round((dateFromKey(end).getTime() - dateFromKey(start).getTime()) / 86_400_000);
}

function monthStart(key: string) {
  return `${key.slice(0, 7)}-01`;
}

function moveMonth(key: string, amount: number) {
  const value = dateFromKey(monthStart(key));
  value.setUTCMonth(value.getUTCMonth() + amount);
  return keyFromDate(value);
}

function buildSegments(reservations: ReservationData[], weekStart: string, timezone: string) {
  const weekEnd = addDays(weekStart, 6);
  const candidates = reservations.flatMap((reservation) => {
    const start = dateKey(new Date(reservation.start_at), reservation.timezone || timezone);
    const rawEnd = dateKey(new Date(reservation.end_at), reservation.timezone || timezone);
    const end = rawEnd < start ? start : rawEnd;
    if (end < weekStart || start > weekEnd) return [];
    const segmentStart = start < weekStart ? weekStart : start;
    const segmentEnd = end > weekEnd ? weekEnd : end;
    return [{
      reservation,
      startColumn: dayDistance(weekStart, segmentStart) + 1,
      span: dayDistance(segmentStart, segmentEnd) + 1,
      lane: 0,
      continuesBefore: start < weekStart,
      continuesAfter: end > weekEnd,
    }];
  }).sort((left, right) => left.startColumn - right.startColumn || right.span - left.span);

  const laneEnds: number[] = [];
  return candidates.map((segment) => {
    const lane = laneEnds.findIndex((endColumn) => endColumn < segment.startColumn);
    const assignedLane = lane === -1 ? laneEnds.length : lane;
    laneEnds[assignedLane] = segment.startColumn + segment.span - 1;
    return { ...segment, lane: assignedLane } satisfies ReservationSegment;
  });
}

function formatReservationWindow(reservation: ReservationData, timezone: string) {
  const formatter = new Intl.DateTimeFormat("en-US", {
    weekday: "short",
    month: "short",
    day: "2-digit",
    hour: reservation.all_day ? undefined : "2-digit",
    minute: reservation.all_day ? undefined : "2-digit",
    timeZone: reservation.timezone || timezone,
  });
  return `${formatter.format(new Date(reservation.start_at))} — ${formatter.format(new Date(reservation.end_at))}`;
}

export default function CalendarGrid({ reservations, timezone, renderStatus, propertyLabel }: CalendarGridProps) {
  const today = dateKey(new Date(), timezone);
  const [visibleMonth, setVisibleMonth] = useState(monthStart(today));
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const selected = reservations.find((reservation) => reservation.id === selectedId) ?? null;
  const firstDay = dateFromKey(visibleMonth);
  const gridStart = addDays(visibleMonth, -firstDay.getUTCDay());
  const weeks = useMemo(() => Array.from({ length: 6 }, (_, weekIndex) => {
    const weekStart = addDays(gridStart, weekIndex * 7);
    const days = Array.from({ length: 7 }, (_, dayIndex) => addDays(weekStart, dayIndex));
    const segments = buildSegments(reservations, weekStart, timezone);
    return { weekStart, days, segments, laneCount: Math.max(1, ...segments.map((segment) => segment.lane + 1)) };
  }), [gridStart, reservations, timezone]);
  const monthLabel = new Intl.DateTimeFormat("en-US", {
    month: "long",
    year: "numeric",
    timeZone: "UTC",
  }).format(dateFromKey(visibleMonth));

  return (
    <div className="staff-calendar">
      <div className="staff-calendar-toolbar">
        <div>
          <span>MONTH VIEW</span>
          <strong aria-live="polite">{monthLabel}</strong>
        </div>
        <div className="staff-calendar-navigation" aria-label="Calendar month navigation">
          <button type="button" onClick={() => setVisibleMonth(moveMonth(visibleMonth, -1))} aria-label="Previous month">← PREV</button>
          <button type="button" onClick={() => setVisibleMonth(monthStart(today))}>TODAY</button>
          <button type="button" onClick={() => setVisibleMonth(moveMonth(visibleMonth, 1))} aria-label="Next month">NEXT →</button>
        </div>
      </div>

      <Modal
        open={Boolean(selected)}
        onClose={() => setSelectedId(null)}
        title={selected?.guest_summary ?? "Reservation"}
        label="RESERVATION / STAY DETAIL"
        className="staff-calendar-modal"
        closeLabel="Close reservation detail"
      >
        {selected && (
          <div className="staff-calendar-modal-content">
            <p>BOOKED FOR</p>
            <strong>{selected.guest_summary}</strong>
            <dl>
              <div><dt>PROPERTY</dt><dd>{propertyLabel?.(selected.property_id) ?? selected.property_id}</dd></div>
              <div><dt>STAY WINDOW</dt><dd>{formatReservationWindow(selected, timezone)}</dd></div>
              <div><dt>STATUS</dt><dd>{renderStatus(selected.status)}</dd></div>
              <div><dt>CALENDAR SOURCE</dt><dd>{selected.source}</dd></div>
            </dl>
            <small>RESERVATION {selected.id}</small>
          </div>
        )}
      </Modal>

      <div className="staff-calendar-scroll" tabIndex={0} aria-label={`${monthLabel} reservation calendar`}>
        <div className="staff-calendar-grid">
          <div className="staff-calendar-weekdays" aria-hidden="true">
            {DAY_NAMES.map((day) => <span key={day}>{day}</span>)}
          </div>
          {weeks.map((week) => (
            <div
              className="staff-calendar-week"
              key={week.weekStart}
              style={{ "--calendar-lanes": week.laneCount } as CSSProperties}
            >
              {week.days.map((day) => {
                const inMonth = day.slice(0, 7) === visibleMonth.slice(0, 7);
                return (
                  <div className={`staff-calendar-day${inMonth ? "" : " staff-calendar-day--outside"}${day === today ? " staff-calendar-day--today" : ""}`} key={day}>
                    <time dateTime={day}>{Number(day.slice(-2))}</time>
                    {day === today && <span>TODAY</span>}
                  </div>
                );
              })}
              {week.segments.map((segment) => {
                const isSelected = segment.reservation.id === selectedId;
                const now = Date.now();
                const startsAt = new Date(segment.reservation.start_at).getTime();
                const endsAt = new Date(segment.reservation.end_at).getTime();
                const isPast = endsAt < now;
                const isCurrent = startsAt <= now && endsAt >= now;
                const style = {
                  "--calendar-start": segment.startColumn,
                  "--calendar-span": segment.span,
                  "--calendar-lane": segment.lane,
                } as CSSProperties;
                return (
                  <button
                    className={`staff-calendar-reservation${segment.continuesBefore ? " staff-calendar-reservation--before" : ""}${segment.continuesAfter ? " staff-calendar-reservation--after" : ""}`}
                    style={style}
                    type="button"
                    data-past={isPast}
                    data-current={isCurrent}
                    data-upcoming={!isPast && !isCurrent}
                    data-status={segment.reservation.status.toLowerCase().replaceAll(/[^a-z0-9]+/g, "-")}
                    key={`${week.weekStart}-${segment.reservation.id}`}
                    aria-pressed={isSelected}
                    aria-label={`${segment.reservation.guest_summary}, ${formatReservationWindow(segment.reservation, timezone)}, ${segment.reservation.status}${isPast ? ", past" : ""}`}
                    onClick={() => setSelectedId(segment.reservation.id)}
                  >
                    <span className="staff-calendar-reservation-label">
                      <strong>{segment.reservation.guest_summary}</strong>
                      <em>{segment.reservation.status}</em>
                    </span>
                  </button>
                );
              })}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
