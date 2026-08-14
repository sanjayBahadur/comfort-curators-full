package reservations_test

import (
	"testing"
	"time"

	"comfort-curators-backend/internal/reservations"
)

func mustResolve(t *testing.T, content, propertyTZ string) []*reservations.ResolvedEvent {
	t.Helper()
	events, skipErrors, err := reservations.ParseICal(content)
	if err != nil {
		t.Fatalf("ParseICal: %v", err)
	}
	if len(skipErrors) != 0 {
		t.Fatalf("unexpected skip errors: %v", skipErrors)
	}
	resolved := make([]*reservations.ResolvedEvent, 0, len(events))
	for i := range events {
		r, err := reservations.ResolveEvent(events[i], propertyTZ)
		if err != nil {
			t.Fatalf("ResolveEvent: %v", err)
		}
		resolved = append(resolved, r)
	}
	return resolved
}

func TestParseICalPreservesSourceID(t *testing.T) {
	const content = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Example//EN
BEGIN:VEVENT
UID:booking-42@airbnb.example
DTSTAMP:20240101T000000Z
DTSTART:20240301T100000Z
DTEND:20240305T100000Z
SUMMARY:Guest stay near lake
DESCRIPTION:Two adults
STATUS:CONFIRMED
SEQUENCE:2
END:VEVENT
END:VCALENDAR
`

	events, skipErrors, err := reservations.ParseICal(content)
	if err != nil {
		t.Fatalf("ParseICal: %v", err)
	}
	if len(skipErrors) != 0 {
		t.Fatalf("unexpected skip errors: %v", skipErrors)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].UID != "booking-42@airbnb.example" {
		t.Fatalf("source id not preserved: %q", events[0].UID)
	}
	if events[0].Status != reservations.EventStatusConfirmed {
		t.Fatalf("expected confirmed status, got %q", events[0].Status)
	}
	if events[0].Sequence != 2 {
		t.Fatalf("expected sequence 2, got %d", events[0].Sequence)
	}
	if events[0].Summary != "Guest stay near lake" {
		t.Fatalf("summary mismatch: %q", events[0].Summary)
	}
}

func TestParseICalTentativeAndCancelledStatuses(t *testing.T) {
	const content = `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:a@x
DTSTART:20240101T100000Z
DTEND:20240101T110000Z
STATUS:TENTATIVE
END:VEVENT
BEGIN:VEVENT
UID:b@x
DTSTART:20240102T100000Z
DTEND:20240102T110000Z
STATUS:CANCELLED
END:VEVENT
END:VCALENDAR
`
	events, skipErrors, err := reservations.ParseICal(content)
	if err != nil {
		t.Fatalf("ParseICal: %v", err)
	}
	if len(skipErrors) != 0 {
		t.Fatalf("unexpected skip errors: %v", skipErrors)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Status != reservations.EventStatusTentative {
		t.Fatalf("expected tentative, got %q", events[0].Status)
	}
	if events[1].Status != reservations.EventStatusCancelled {
		t.Fatalf("expected cancelled, got %q", events[1].Status)
	}
}

func TestParseICalUnfoldsFoldedLines(t *testing.T) {
	const content = `BEGIN:VCALENDAR
BEGIN:VEVENT
UID:folded@x
DTSTART:20240101T100000Z
DTEND:20240101T110000Z
SUMMARY:Very long summary that needs to be fol
 ded across multiple physical lines
END:VEVENT
END:VCALENDAR
`
	events, skipErrors, err := reservations.ParseICal(content)
	if err != nil {
		t.Fatalf("ParseICal: %v", err)
	}
	if len(skipErrors) != 0 {
		t.Fatalf("unexpected skip errors: %v", skipErrors)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Summary != "Very long summary that needs to be folded across multiple physical lines" {
		t.Fatalf("folded summary not joined: %q", events[0].Summary)
	}
}

func TestParseICalUnescapesText(t *testing.T) {
	content := "BEGIN:VCALENDAR\nBEGIN:VEVENT\nUID:esc@x\nDTSTART:20240101T100000Z\nDTEND:20240101T110000Z\nSUMMARY:Cafe\\, semicolon\\; newline\\nsecond\nEND:VEVENT\nEND:VCALENDAR\n"
	events, _, err := reservations.ParseICal(content)
	if err != nil {
		t.Fatalf("ParseICal: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Summary != "Cafe, semicolon; newline\nsecond" {
		t.Fatalf("text escapes not unescaped: %q", events[0].Summary)
	}
}

func TestParseICalSkipsRecurringEvents(t *testing.T) {
	const content = `BEGIN:VCALENDAR
BEGIN:VEVENT
UID:single@x
DTSTART:20240101T100000Z
DTEND:20240101T110000Z
END:VEVENT
BEGIN:VEVENT
UID:recurring@x
DTSTART:20240101T100000Z
DTEND:20240101T110000Z
RRULE:FREQ=WEEKLY
END:VEVENT
END:VCALENDAR
`
	events, skipErrors, err := reservations.ParseICal(content)
	if err != nil {
		t.Fatalf("ParseICal: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected only the single event, got %d", len(events))
	}
	if len(skipErrors) != 1 {
		t.Fatalf("expected 1 skip error for recurring event, got %d", len(skipErrors))
	}
}

func TestResolveEventUTC(t *testing.T) {
	r := mustResolve(t, `BEGIN:VCALENDAR
BEGIN:VEVENT
UID:utc@x
DTSTART:20240301T100000Z
DTEND:20240305T100000Z
END:VEVENT
END:VCALENDAR
`, "Asia/Kolkata")

	if r[0].StartAt.UTC().Format(time.RFC3339) != "2024-03-01T10:00:00Z" {
		t.Fatalf("start mismatch: %s", r[0].StartAt.UTC().Format(time.RFC3339))
	}
	if r[0].EndAt.UTC().Format(time.RFC3339) != "2024-03-05T10:00:00Z" {
		t.Fatalf("end mismatch: %s", r[0].EndAt.UTC().Format(time.RFC3339))
	}
	if r[0].TimezoneAmbiguous {
		t.Fatal("UTC event must not be ambiguous")
	}
	if r[0].Timezone != "UTC" {
		t.Fatalf("expected UTC timezone, got %q", r[0].Timezone)
	}
}

func TestResolveEventWithTZIDNormalizesToUTC(t *testing.T) {
	r := mustResolve(t, `BEGIN:VCALENDAR
BEGIN:VEVENT
UID:tzid@x
DTSTART;TZID=Asia/Kolkata:20240301T100000
DTEND;TZID=Asia/Kolkata:20240301T110000
END:VEVENT
END:VCALENDAR
`, "Asia/Kolkata")

	if r[0].StartAt.UTC().Format(time.RFC3339) != "2024-03-01T04:30:00Z" {
		t.Fatalf("TZID start not normalized to UTC: %s", r[0].StartAt.UTC().Format(time.RFC3339))
	}
	if r[0].Timezone != "Asia/Kolkata" {
		t.Fatalf("expected timezone preserved as Asia/Kolkata, got %q", r[0].Timezone)
	}
	if r[0].TimezoneAmbiguous {
		t.Fatal("known TZID must not be ambiguous")
	}
}

func TestResolveEventFloatingTimeIsAmbiguous(t *testing.T) {
	r := mustResolve(t, `BEGIN:VCALENDAR
BEGIN:VEVENT
UID:floating@x
DTSTART:20240301T100000
DTEND:20240301T110000
END:VEVENT
END:VCALENDAR
`, "Asia/Kolkata")

	if !r[0].TimezoneAmbiguous {
		t.Fatal("floating local time must be flagged as ambiguous")
	}
}

func TestResolveEventUnknownTZIDIsAmbiguous(t *testing.T) {
	r := mustResolve(t, `BEGIN:VCALENDAR
BEGIN:VEVENT
UID:unknowntz@x
DTSTART;TZID=Mars/Olympus:20240301T100000
DTEND;TZID=Mars/Olympus:20240301T110000
END:VEVENT
END:VCALENDAR
`, "Asia/Kolkata")

	if !r[0].TimezoneAmbiguous {
		t.Fatal("unknown TZID must be flagged as ambiguous")
	}
}

func TestResolveAllDayEvent(t *testing.T) {
	r := mustResolve(t, `BEGIN:VCALENDAR
BEGIN:VEVENT
UID:allday@x
DTSTART;VALUE=DATE:20240301
DTEND;VALUE=DATE:20240304
END:VEVENT
END:VCALENDAR
`, "Asia/Kolkata")

	if !r[0].AllDay {
		t.Fatal("expected all-day event")
	}
	if r[0].TimezoneAmbiguous {
		t.Fatal("all-day events are not timezone ambiguous")
	}
	loc, _ := time.LoadLocation("Asia/Kolkata")
	expectedStart := time.Date(2024, 3, 1, 0, 0, 0, 0, loc).UTC()
	if !r[0].StartAt.Equal(expectedStart) {
		t.Fatalf("all-day start mismatch: %s != %s", r[0].StartAt.Format(time.RFC3339), expectedStart.Format(time.RFC3339))
	}
	expectedEnd := time.Date(2024, 3, 4, 0, 0, 0, 0, loc).UTC()
	if !r[0].EndAt.Equal(expectedEnd) {
		t.Fatalf("all-day end mismatch: %s != %s", r[0].EndAt.Format(time.RFC3339), expectedEnd.Format(time.RFC3339))
	}
}

func TestResolveEventDurationFallback(t *testing.T) {
	r := mustResolve(t, `BEGIN:VCALENDAR
BEGIN:VEVENT
UID:dur@x
DTSTART:20240301T100000Z
DURATION:PT2H30M
END:VEVENT
END:VCALENDAR
`, "Asia/Kolkata")

	if r[0].EndAt.UTC().Format(time.RFC3339) != "2024-03-01T12:30:00Z" {
		t.Fatalf("duration end mismatch: %s", r[0].EndAt.UTC().Format(time.RFC3339))
	}
}

func TestResolveEventDefaultsEndForMissingEnd(t *testing.T) {
	r := mustResolve(t, `BEGIN:VCALENDAR
BEGIN:VEVENT
UID:noend@x
DTSTART:20240301T100000Z
END:VEVENT
END:VCALENDAR
`, "Asia/Kolkata")

	if r[0].EndAt.UTC().Format(time.RFC3339) != "2024-03-01T11:00:00Z" {
		t.Fatalf("default end mismatch: %s", r[0].EndAt.UTC().Format(time.RFC3339))
	}
}

func TestParseICalRejectsEmptyContent(t *testing.T) {
	_, _, err := reservations.ParseICal("   ")
	if err == nil {
		t.Fatal("expected error for empty content")
	}
}
