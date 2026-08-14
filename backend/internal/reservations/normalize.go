package reservations

import (
	"fmt"
	"time"
)

// ResolvedEvent is a VEVENT whose dates have been normalized to UTC and
// whose timezone provenance has been preserved. Source identity is kept in
// ExternalEventID so that a later poll can match and update the same record.
type ResolvedEvent struct {
	ExternalEventID   string
	Summary           string
	Description       string
	Status            string
	Sequence          int
	StartAt           time.Time
	EndAt             time.Time
	AllDay            bool
	Timezone          string
	TimezoneAmbiguous bool
	Raw               string
}

// ResolveEvent normalizes an iCalendar VEVENT into a resolved external
// calendar event. All timestamps are stored in UTC. All-day events use the
// property timezone to anchor their boundaries. Floating date-times and
// unknown TZID values produce a timezone ambiguity flag instead of a wrong
// instant being treated as authoritative.
func ResolveEvent(ev icalEvent, propertyTimezone string) (*ResolvedEvent, error) {
	resolved := &ResolvedEvent{
		ExternalEventID: ev.UID,
		Summary:         ev.Summary,
		Description:     ev.Description,
		Status:          ev.Status,
		Sequence:        ev.Sequence,
		AllDay:          ev.AllDay,
		Raw:             ev.Raw,
	}

	loc := time.UTC
	if propertyTimezone != "" {
		if l, err := time.LoadLocation(propertyTimezone); err == nil {
			loc = l
		} else {
			return nil, fmt.Errorf("%w: unknown property timezone %q", ErrInvalidCalendarContent, propertyTimezone)
		}
	}

	if ev.AllDay {
		start, err := parseICalDate(ev.StartRaw)
		if err != nil {
			return nil, err
		}
		startUTC := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, loc).UTC()

		end := start.AddDate(0, 0, 1)
		if ev.EndRaw != "" {
			parsedEnd, err := parseICalDate(ev.EndRaw)
			if err != nil {
				return nil, err
			}
			end = parsedEnd
		} else if ev.Duration != "" {
			d, err := parseDuration(ev.Duration)
			if err != nil {
				return nil, err
			}
			end = start.Add(d)
		}
		endUTC := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, loc).UTC()

		resolved.StartAt = startUTC
		resolved.EndAt = endUTC
		resolved.Timezone = propertyTimezone
		if !resolved.EndAt.After(resolved.StartAt) {
			return nil, fmt.Errorf("%w: %s", errEndBeforeStart, ev.UID)
		}
		return resolved, nil
	}

	start, startTZ, startAmbiguous, err := resolveDateTime(ev.StartRaw, ev.StartTZID)
	if err != nil {
		return nil, err
	}
	resolved.StartAt = start
	resolved.Timezone = startTZ
	resolved.TimezoneAmbiguous = startAmbiguous

	if ev.Duration != "" {
		d, err := parseDuration(ev.Duration)
		if err != nil {
			return nil, err
		}
		resolved.EndAt = resolved.StartAt.Add(d)
	} else if ev.EndRaw != "" {
		end, _, endAmbiguous, err := resolveDateTime(ev.EndRaw, ev.EndTZID)
		if err != nil {
			return nil, err
		}
		resolved.EndAt = end
		resolved.TimezoneAmbiguous = resolved.TimezoneAmbiguous || endAmbiguous
	} else {
		resolved.EndAt = resolved.StartAt.Add(time.Hour)
	}

	if !resolved.EndAt.After(resolved.StartAt) {
		return nil, fmt.Errorf("%w: %s", errEndBeforeStart, ev.UID)
	}
	return resolved, nil
}

// resolveDateTime parses a raw iCalendar date-time into UTC. It reports the
// originating timezone name and whether the instant is timezone-ambiguous
// (floating local time or an unknown TZID that cannot be mapped).
func resolveDateTime(raw, tzid string) (time.Time, string, bool, error) {
	upper := raw
	isUTC := len(upper) > 0 && upper[len(upper)-1] == 'Z'

	if tzid != "" {
		loc, err := time.LoadLocation(tzid)
		if err == nil {
			t, err := parseICalDateTime(raw)
			if err != nil {
				return time.Time{}, "", false, err
			}
			// parseICalDateTime yields a UTC wall clock; reinterpret those
			// components in the TZID location so 10:00 in Kolkata becomes
			// 04:30Z instead of 15:30 in the source zone.
			local := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, loc)
			return local.UTC(), tzid, false, nil
		}
		// Unknown TZID: cannot map to a real instant. Keep the wall clock
		// as a UTC placeholder and mark the event ambiguous.
		t, perr := parseICalDateTime(raw)
		if perr != nil {
			return time.Time{}, "", false, perr
		}
		return t.UTC(), tzid, true, nil
	}

	if isUTC {
		t, err := parseICalDateTime(raw)
		if err != nil {
			return time.Time{}, "", false, err
		}
		return t.UTC(), "UTC", false, nil
	}

	// Floating local time with no zone and no TZID is ambiguous.
	t, err := parseICalDateTime(raw)
	if err != nil {
		return time.Time{}, "", false, err
	}
	return t.UTC(), "", true, nil
}
