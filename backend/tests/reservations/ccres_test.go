package reservations_test

import (
	"context"
	"testing"
	"time"

	"comfort-curators-backend/internal/reservations"
)

// TestCCRES001StaleCalendarIsRejected proves a stale or failed feed stays
// visible as an exception and that turnover automation refuses to propose
// work from possibly outdated bookings.
func TestCCRES001StaleCalendarIsRejected(t *testing.T) {
	svc := newTestService(t)
	feed, err := svc.CreateFeed(context.Background(), reservations.FeedParams{
		TenantID: "tenant-a", PropertyID: "prop-1", Source: "airbnb",
		URL: "https://example.invalid/feed.ics", PropertyTimezone: "Asia/Kolkata",
		StaleAfterMinutes: 30,
	}, []string{"operator"})
	if err != nil {
		t.Fatalf("CreateFeed: %v", err)
	}

	now := time.Date(2024, 4, 1, 10, 0, 0, 0, time.UTC)
	if _, err := svc.IngestContent(context.Background(), feed, icalFeed(eventOne), now); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	// A feed that stops succeeding becomes a visible stale exception.
	result, err := svc.ScanStaleFeeds(context.Background(), now.Add(31*time.Minute))
	if err != nil {
		t.Fatalf("ScanStaleFeeds: %v", err)
	}
	if result.StaleFeeds != 1 {
		t.Fatalf("expected 1 stale feed, got %d", result.StaleFeeds)
	}

	// Proposal automation refuses to assume current data while stale.
	gen, err := svc.GenerateTurnoverProposals(context.Background(), "tenant-a", "prop-1", now.Add(31*time.Minute))
	if err != nil {
		t.Fatalf("GenerateTurnoverProposals: %v", err)
	}
	if !gen.Skipped {
		t.Fatalf("stale feed must block proposal generation, got %+v", gen)
	}

	exceptions, err := svc.ListExceptions(context.Background(), "tenant-a", "prop-1")
	if err != nil {
		t.Fatalf("ListExceptions: %v", err)
	}
	found := false
	for _, e := range exceptions {
		if e.Kind == reservations.ExceptionKindStaleFeed && e.Status == reservations.ExceptionStatusOpen {
			found = true
		}
	}
	if !found {
		t.Fatalf("stale feed must remain visible, got %+v", exceptions)
	}
}

// TestCCRES001ConflictIsDetected proves overlaps and duplicates create
// human-reviewable reservation conflicts and that a human resolution is
// audited with the acting actor.
func TestCCRES001ConflictIsDetected(t *testing.T) {
	pool := reservationsPool(t)
	svc := reservations.NewCalendarService(pool).WithAuthorizer(testAuthorizer{tenant: "tenant-a"})
	feed, err := svc.CreateFeed(context.Background(), reservations.FeedParams{
		TenantID: "tenant-a", PropertyID: "prop-1", Source: "booking",
		URL: "https://example.invalid/feed.ics", PropertyTimezone: "Asia/Kolkata",
		MinimumTurnaroundMinutes: 240,
	}, []string{"operator"})
	if err != nil {
		t.Fatalf("CreateFeed: %v", err)
	}

	res, err := svc.IngestContent(context.Background(), feed, icalFeed(eventOne, eventTwo), time.Now().UTC())
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if res.ConflictsCreated == 0 {
		t.Fatalf("overlapping bookings must create a conflict, got %+v", res)
	}

	conflicts, err := svc.ListConflicts(context.Background(), "tenant-a", "prop-1")
	if err != nil {
		t.Fatalf("ListConflicts: %v", err)
	}
	var conflict *reservations.ReservationConflict
	for i := range conflicts {
		if conflicts[i].Kind == reservations.ExceptionKindOverlap {
			conflict = &conflicts[i]
			break
		}
	}
	if conflict == nil {
		t.Fatalf("expected overlap conflict, got %+v", conflicts)
	}

	// Human resolution is audited: actor, outcome and evidence are retained.
	resolved, err := svc.ResolveConflict(context.Background(), "tenant-a", conflict.ID,
		reservations.ResolutionOutcomeConfirm, "verified", "actor-owner-1", "operator",
		[]string{"operator"})
	if err != nil {
		t.Fatalf("ResolveConflict: %v", err)
	}
	if resolved.Status != reservations.ConflictStatusResolved {
		t.Fatalf("expected resolved conflict, got %q", resolved.Status)
	}

	var auditCount int
	if err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM audit_events
		 WHERE action = 'reservation.conflict.resolve' AND resource_id = $1 AND actor_id = $2`,
		conflict.ID, "actor-owner-1",
	).Scan(&auditCount); err != nil {
		t.Fatalf("query audit events: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("conflict resolution must be audited, got %d events", auditCount)
	}
}

// TestCCRES001CancellationUpdatesTurnover proves a cancelled reservation
// updates its turnover proposals in place instead of silently deleting them.
func TestCCRES001CancellationUpdatesTurnover(t *testing.T) {
	svc := newTestService(t)
	feed, err := svc.CreateFeed(context.Background(), reservations.FeedParams{
		TenantID: "tenant-a", PropertyID: "prop-1", Source: "booking",
		URL: "https://example.invalid/feed.ics", PropertyTimezone: "Asia/Kolkata",
	}, []string{"operator"})
	if err != nil {
		t.Fatalf("CreateFeed: %v", err)
	}

	if _, err := svc.IngestContent(context.Background(), feed, icalFeed(eventOne), time.Now().UTC()); err != nil {
		t.Fatalf("first ingest: %v", err)
	}

	// The stay is cancelled; the reservation and its work are updated, not
	// deleted.
	res, err := svc.IngestContent(context.Background(), feed, icalFeed(eventTwo), time.Now().UTC())
	if err != nil {
		t.Fatalf("cancellation ingest: %v", err)
	}
	if res.ReservationsCancelled != 1 {
		t.Fatalf("expected 1 cancelled reservation, got %+v", res)
	}
	if res.ProposalsCancelled != 2 {
		t.Fatalf("expected the turnover proposals cancelled, got %+v", res)
	}

	rsv, err := svc.ListReservations(context.Background(), "tenant-a", "prop-1")
	if err != nil {
		t.Fatalf("ListReservations: %v", err)
	}
	bySource := map[string]reservations.Reservation{}
	for _, r := range rsv {
		bySource[r.ExternalEventID] = r
	}
	if bySource["booking-1@x"].Status != reservations.ReservationStatusCancelled {
		t.Fatalf("cancelled reservation must be updated to cancelled, got %q", bySource["booking-1@x"].Status)
	}
}
