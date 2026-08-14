package reservations_test

import (
	"testing"
	"time"

	"comfort-curators-backend/internal/reservations"
)

func event(id, feed, externalID, summary string, start, end time.Time) *reservations.ExternalCalendarEvent {
	return &reservations.ExternalCalendarEvent{
		ID:              id,
		FeedID:          feed,
		ExternalEventID: externalID,
		Summary:         summary,
		StartAt:         start,
		EndAt:           end,
		Status:          reservations.EventStatusConfirmed,
	}
}

func dt(rfc3339 string) time.Time {
	t, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		panic(err)
	}
	return t
}

func TestDetectOverlap(t *testing.T) {
	events := []*reservations.ExternalCalendarEvent{
		event("e1", "f1", "u1", "guest one", dt("2024-03-01T10:00:00Z"), dt("2024-03-05T10:00:00Z")),
		event("e2", "f1", "u2", "guest two", dt("2024-03-04T10:00:00Z"), dt("2024-03-08T10:00:00Z")),
	}
	d := reservations.DetectIssues(events, 180)
	if len(d.Overlaps) != 1 {
		t.Fatalf("expected 1 overlap, got %d", len(d.Overlaps))
	}
	if len(d.ImpossibleTurnarounds) != 0 {
		t.Fatalf("overlapping pair must not be double counted as turnaround, got %d", len(d.ImpossibleTurnarounds))
	}
}

func TestDetectAdjacentEventsAreNotOverlap(t *testing.T) {
	events := []*reservations.ExternalCalendarEvent{
		event("e1", "f1", "u1", "one", dt("2024-03-01T10:00:00Z"), dt("2024-03-05T10:00:00Z")),
		event("e2", "f1", "u2", "two", dt("2024-03-05T10:00:00Z"), dt("2024-03-08T10:00:00Z")),
	}
	d := reservations.DetectIssues(events, 180)
	if len(d.Overlaps) != 0 {
		t.Fatalf("adjacent events must not overlap, got %d", len(d.Overlaps))
	}
}

func TestDetectImpossibleTurnaround(t *testing.T) {
	events := []*reservations.ExternalCalendarEvent{
		event("e1", "f1", "u1", "one", dt("2024-03-01T10:00:00Z"), dt("2024-03-03T10:00:00Z")),
		event("e2", "f1", "u2", "two", dt("2024-03-03T11:30:00Z"), dt("2024-03-06T10:00:00Z")),
	}
	d := reservations.DetectIssues(events, 180)
	if len(d.ImpossibleTurnarounds) != 1 {
		t.Fatalf("expected 1 impossible turnaround, got %d", len(d.ImpossibleTurnarounds))
	}
	gap := d.ImpossibleTurnarounds[0]
	if gap.GapMinutes != 90 {
		t.Fatalf("expected 90 minute gap, got %d", gap.GapMinutes)
	}
}

func TestDetectSufficientTurnaround(t *testing.T) {
	events := []*reservations.ExternalCalendarEvent{
		event("e1", "f1", "u1", "one", dt("2024-03-01T10:00:00Z"), dt("2024-03-03T10:00:00Z")),
		event("e2", "f1", "u2", "two", dt("2024-03-03T14:00:00Z"), dt("2024-03-06T10:00:00Z")),
	}
	d := reservations.DetectIssues(events, 180)
	if len(d.ImpossibleTurnarounds) != 0 {
		t.Fatalf("a 240 minute gap is sufficient, got %d", len(d.ImpossibleTurnarounds))
	}
}

func TestDetectCancelledEventsAreIgnored(t *testing.T) {
	e2 := event("e2", "f1", "u2", "cancelled guest", dt("2024-03-03T11:00:00Z"), dt("2024-03-06T10:00:00Z"))
	e2.Status = reservations.EventStatusCancelled
	events := []*reservations.ExternalCalendarEvent{
		event("e1", "f1", "u1", "one", dt("2024-03-01T10:00:00Z"), dt("2024-03-03T10:00:00Z")),
		e2,
	}
	d := reservations.DetectIssues(events, 180)
	if len(d.Overlaps) != 0 {
		t.Fatalf("cancelled events must not create overlaps, got %d", len(d.Overlaps))
	}
	if len(d.ImpossibleTurnarounds) != 0 {
		t.Fatalf("cancelled events must not create turnarounds, got %d", len(d.ImpossibleTurnarounds))
	}
}

func TestDetectSameUIDAcrossFeedsIsDuplicate(t *testing.T) {
	events := []*reservations.ExternalCalendarEvent{
		event("e1", "f1", "uid-9", "airbnb", dt("2024-03-01T10:00:00Z"), dt("2024-03-05T10:00:00Z")),
		event("e2", "f2", "uid-9", "booking", dt("2024-03-01T10:00:00Z"), dt("2024-03-05T10:00:00Z")),
	}
	d := reservations.DetectIssues(events, 180)
	if len(d.Duplicates) != 1 {
		t.Fatalf("expected 1 duplicate, got %d", len(d.Duplicates))
	}
}

func TestDetectSameStayDifferentUIDIsDuplicate(t *testing.T) {
	events := []*reservations.ExternalCalendarEvent{
		event("e1", "f1", "a-1", "Family of four", dt("2024-03-01T10:00:00Z"), dt("2024-03-05T10:00:00Z")),
		event("e2", "f2", "b-2", "family of four", dt("2024-03-01T10:00:00Z"), dt("2024-03-05T10:00:00Z")),
	}
	d := reservations.DetectIssues(events, 180)
	if len(d.Duplicates) != 1 {
		t.Fatalf("expected 1 duplicate from identical stay, got %d", len(d.Duplicates))
	}
}

func TestDetectSameFeedSameUIDIsNotDuplicate(t *testing.T) {
	// Two VEVENTs with the same UID inside one feed collapse into a single
	// stored row (unique feed_id + external_event_id), so detection must not
	// treat two identical rows from the same feed as duplicates.
	events := []*reservations.ExternalCalendarEvent{
		event("e1", "f1", "uid-1", "stay", dt("2024-03-01T10:00:00Z"), dt("2024-03-05T10:00:00Z")),
		event("e2", "f1", "uid-2", "stay", dt("2024-03-01T10:00:00Z"), dt("2024-03-05T10:00:00Z")),
	}
	d := reservations.DetectIssues(events, 180)
	if len(d.Duplicates) != 0 {
		t.Fatalf("same feed must not create duplicates, got %d", len(d.Duplicates))
	}
}

func TestDetectTimezoneAmbiguity(t *testing.T) {
	ambiguous := event("e1", "f1", "u1", "floating", dt("2024-03-01T10:00:00Z"), dt("2024-03-05T10:00:00Z"))
	ambiguous.TimezoneAmbiguous = true
	events := []*reservations.ExternalCalendarEvent{
		ambiguous,
		event("e2", "f1", "u2", "clean", dt("2024-03-10T10:00:00Z"), dt("2024-03-12T10:00:00Z")),
	}
	d := reservations.DetectIssues(events, 180)
	if len(d.Ambiguities) != 1 {
		t.Fatalf("expected 1 ambiguity, got %d", len(d.Ambiguities))
	}
}

func TestExceptionsFromDetectionDedupeKeysAreStable(t *testing.T) {
	events := []*reservations.ExternalCalendarEvent{
		event("e1", "f1", "u1", "one", dt("2024-03-01T10:00:00Z"), dt("2024-03-05T10:00:00Z")),
		event("e2", "f1", "u2", "two", dt("2024-03-04T10:00:00Z"), dt("2024-03-08T10:00:00Z")),
	}
	d := reservations.DetectIssues(events, 180)
	exc := reservations.ExceptionsFromDetection("t1", "p1", d, time.Now().UTC())
	if len(exc) != 1 {
		t.Fatalf("expected 1 exception, got %d", len(exc))
	}
	if exc[0].Kind != reservations.ExceptionKindOverlap {
		t.Fatalf("expected overlap exception, got %q", exc[0].Kind)
	}
	if exc[0].DedupeKey != "e1|e2" {
		t.Fatalf("unexpected dedupe key %q", exc[0].DedupeKey)
	}
}
