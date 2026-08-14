package reservations

import (
	"testing"
	"time"
)

func rfc(t *testing.T, s string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return parsed
}

func TestReservationFromEventCopiesSourceIdentity(t *testing.T) {
	ev := &ExternalCalendarEvent{
		TenantID: "t", PropertyID: "p", FeedID: "f", ExternalEventID: "u-1",
		Source: "airbnb", Summary: "Guest A", Status: EventStatusTentative,
		StartAt: rfc(t, "2024-03-01T04:30:00Z"), EndAt: rfc(t, "2024-03-05T04:30:00Z"),
		Timezone: "Asia/Kolkata", Sequence: 2,
	}
	r := ReservationFromEvent(ev)
	if r.Status != ReservationStatusActive {
		t.Fatalf("tentative maps to active, got %q", r.Status)
	}
	if r.FeedID != "f" || r.ExternalEventID != "u-1" || r.Source != "airbnb" {
		t.Fatalf("source identity lost: %+v", r)
	}
	if r.GuestSummary != "Guest A" || r.Sequence != 2 {
		t.Fatalf("content lost: %+v", r)
	}
}

func TestReservationContentChangedDetectsRealChanges(t *testing.T) {
	base := &Reservation{
		GuestSummary: "Guest A", Status: ReservationStatusActive,
		StartAt: rfc(t, "2024-03-01T04:30:00Z"), EndAt: rfc(t, "2024-03-05T04:30:00Z"),
		Timezone: "Asia/Kolkata", Sequence: 1,
	}
	same := *base
	if reservationContentChanged(base, &same) {
		t.Fatal("identical content must not count as changed")
	}
	shifted := *base
	shifted.EndAt = rfc(t, "2024-03-06T04:30:00Z")
	if !reservationContentChanged(base, &shifted) {
		t.Fatal("date change must count as changed")
	}
	cancelled := *base
	cancelled.Status = ReservationStatusCancelled
	if !reservationContentChanged(base, &cancelled) {
		t.Fatal("status change must count as changed")
	}
}

func TestProposalSpecsAnchorToStayBounds(t *testing.T) {
	r := &Reservation{
		ID: "res-1", StartAt: rfc(t, "2024-03-01T04:30:00Z"), EndAt: rfc(t, "2024-03-05T04:30:00Z"),
	}
	specs := ProposalSpecs(r)
	if len(specs) != 2 {
		t.Fatalf("expected 2 proposals, got %d", len(specs))
	}
	byKind := map[string]ProposalSpec{}
	for _, s := range specs {
		byKind[s.Kind] = s
	}
	if !byKind[ProposalKindTurnover].ScheduledAt.Equal(r.EndAt) {
		t.Fatalf("turnover must anchor to checkout: %s", byKind[ProposalKindTurnover].ScheduledAt)
	}
	if !byKind[ProposalKindInspection].ScheduledAt.Equal(r.StartAt) {
		t.Fatalf("inspection must anchor to check-in: %s", byKind[ProposalKindInspection].ScheduledAt)
	}
}

func TestProposalFromSpecIsProposed(t *testing.T) {
	r := &Reservation{TenantID: "t", PropertyID: "p", ID: "res-1"}
	p := proposalFromSpec(r, ProposalSpec{Kind: ProposalKindTurnover, ScheduledAt: rfc(t, "2024-03-05T04:30:00Z")})
	if p.Status != ProposalStatusProposed {
		t.Fatalf("new proposal must be proposed, got %q", p.Status)
	}
	if p.ReservationID != "res-1" || p.TenantID != "t" || p.PropertyID != "p" {
		t.Fatalf("proposal lost scope: %+v", p)
	}
}

func TestConflictsFromDetectionLinksReservationIds(t *testing.T) {
	detection := Detection{
		Overlaps: []OverlapIssue{
			{
				EventAID:    "evt-a",
				EventBID:    "evt-b",
				EventAStart: rfc(t, "2024-03-01T04:30:00Z"),
				EventAEnd:   rfc(t, "2024-03-05T04:30:00Z"),
				EventBStart: rfc(t, "2024-03-04T04:30:00Z"),
				EventBEnd:   rfc(t, "2024-03-08T04:30:00Z"),
			},
		},
	}
	items := ConflictsFromDetection("t1", "p1", detection, time.Now().UTC())
	if len(items) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(items))
	}
	item := items[0]
	if item.Kind != ExceptionKindOverlap || item.DedupeKey != "evt-a|evt-b" {
		t.Fatalf("unexpected conflict metadata: kind=%q dedupe=%q", item.Kind, item.DedupeKey)
	}
	if len(item.EventIDs) != 2 || item.EventIDs[0] != "evt-a" || item.EventIDs[1] != "evt-b" {
		t.Fatalf("event ids must be preserved for reservation linking: %v", item.EventIDs)
	}
	if item.Conflict.Status != ConflictStatusOpen {
		t.Fatalf("conflict must start open, got %q", item.Conflict.Status)
	}
}

func TestValidResolutionOutcomeRejectsAutoDecisions(t *testing.T) {
	for _, outcome := range []string{ResolutionOutcomeConfirm, ResolutionOutcomeReject, ResolutionOutcomeMerge} {
		if !ValidResolutionOutcome(outcome) {
			t.Fatalf("outcome %q must be valid", outcome)
		}
	}
	for _, outcome := range []string{"", "auto-accept", "resolve-silently"} {
		if ValidResolutionOutcome(outcome) {
			t.Fatalf("outcome %q must be rejected", outcome)
		}
	}
}
