package reservations_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"comfort-curators-backend/internal/reservations"
)

func TestReservationFromEventMapsActiveStatus(t *testing.T) {
	ev := &reservations.ExternalCalendarEvent{
		TenantID: "t", PropertyID: "p", FeedID: "f", ExternalEventID: "u1",
		Source: "airbnb", Summary: "Guest one", Status: reservations.EventStatusConfirmed,
		StartAt: dt("2024-03-01T04:30:00Z"), EndAt: dt("2024-03-05T04:30:00Z"),
	}
	r := reservations.ReservationFromEvent(ev)
	if r.Status != reservations.ReservationStatusActive {
		t.Fatalf("expected active, got %q", r.Status)
	}
	if r.GuestSummary != "Guest one" || r.ExternalEventID != "u1" {
		t.Fatalf("reservation lost source identity: %+v", r)
	}
	if !r.StartAt.Equal(ev.StartAt) || !r.EndAt.Equal(ev.EndAt) {
		t.Fatalf("reservation dates drifted: %+v", r)
	}
}

func TestReservationFromEventMapsCancelledStatus(t *testing.T) {
	ev := &reservations.ExternalCalendarEvent{
		TenantID: "t", PropertyID: "p", FeedID: "f", ExternalEventID: "u1",
		Source: "booking", Summary: "Guest two", Status: reservations.EventStatusCancelled,
	}
	r := reservations.ReservationFromEvent(ev)
	if r.Status != reservations.ReservationStatusCancelled {
		t.Fatalf("expected cancelled, got %q", r.Status)
	}
	if r.GuestSummary != "Guest two" {
		t.Fatalf("cancelled reservation must keep its content, got %q", r.GuestSummary)
	}
}

func TestReservationFromEventMapsNoLongerListedAsCancelled(t *testing.T) {
	ev := &reservations.ExternalCalendarEvent{
		Status: reservations.EventStatusNoLongerListed,
	}
	if r := reservations.ReservationFromEvent(ev); r.Status != reservations.ReservationStatusCancelled {
		t.Fatalf("no_longer_listed must map to cancelled reservation, got %q", r.Status)
	}
}

func TestConflictsFromDetectionCoversEveryIssueKind(t *testing.T) {
	events := []*reservations.ExternalCalendarEvent{
		event("e1", "f1", "u1", "Guest one", dt("2024-03-01T04:30:00Z"), dt("2024-03-05T04:30:00Z")),
		event("e2", "f1", "u2", "Guest two", dt("2024-03-04T04:30:00Z"), dt("2024-03-08T04:30:00Z")),
		event("e3", "f2", "u1", "Guest one", dt("2024-03-01T04:30:00Z"), dt("2024-03-05T04:30:00Z")),
	}
	amb := event("e4", "f1", "u4", "floating", dt("2024-03-10T04:30:00Z"), dt("2024-03-12T04:30:00Z"))
	amb.TimezoneAmbiguous = true
	events = append(events, amb)

	events = append(events, event("e5", "f1", "u5", "quick follow-up", dt("2024-03-08T05:00:00Z"), dt("2024-03-09T04:30:00Z")))

	d := reservations.DetectIssues(events, 180)
	conflicts := reservations.ConflictsFromDetection("t1", "p1", d, time.Now().UTC())

	kinds := map[string]bool{}
	for _, item := range conflicts {
		kinds[item.Conflict.Kind] = true
		if item.Conflict.Status != reservations.ConflictStatusOpen {
			t.Fatalf("conflict %q must start open", item.Conflict.Kind)
		}
		if item.Conflict.DedupeKey == "" {
			t.Fatalf("conflict %q must carry a dedupe key", item.Conflict.Kind)
		}
	}
	for _, expected := range []string{
		reservations.ExceptionKindOverlap,
		reservations.ExceptionKindDuplicate,
		reservations.ExceptionKindImpossibleTurnaround,
		reservations.ExceptionKindTimezoneAmbiguity,
	} {
		if !kinds[expected] {
			t.Fatalf("expected conflict kind %q, got %v", expected, kinds)
		}
	}
}

func TestProposalSpecsAreDeterministic(t *testing.T) {
	r := &reservations.Reservation{
		ID: "res-1", TenantID: "t", PropertyID: "p",
		StartAt: dt("2024-03-01T04:30:00Z"), EndAt: dt("2024-03-05T04:30:00Z"),
	}
	specs := reservations.ProposalSpecs(r)
	if len(specs) != 2 {
		t.Fatalf("expected exactly 2 deterministic proposals, got %d", len(specs))
	}
	if specs[0].Kind != reservations.ProposalKindTurnover || specs[0].ScheduledAt != r.EndAt {
		t.Fatalf("turnover proposal must anchor to checkout: %+v", specs[0])
	}
	if specs[1].Kind != reservations.ProposalKindInspection || specs[1].ScheduledAt != r.StartAt {
		t.Fatalf("inspection proposal must anchor to check-in: %+v", specs[1])
	}
}

func TestValidResolutionOutcome(t *testing.T) {
	for _, ok := range []string{
		reservations.ResolutionOutcomeConfirm,
		reservations.ResolutionOutcomeReject,
		reservations.ResolutionOutcomeMerge,
	} {
		if !reservations.ValidResolutionOutcome(ok) {
			t.Fatalf("outcome %q must be valid", ok)
		}
	}
	if reservations.ValidResolutionOutcome("auto-fix") {
		t.Fatal("unsupported outcome must be rejected")
	}
}

func TestIngestCreatesNormalizedReservations(t *testing.T) {
	svc := newTestService(t)
	feed, err := svc.CreateFeed(context.Background(), reservations.FeedParams{
		TenantID: "tenant-a", PropertyID: "prop-1", Source: "airbnb",
		URL: "https://example.invalid/feed.ics", PropertyTimezone: "Asia/Kolkata",
	}, []string{"operator"})
	if err != nil {
		t.Fatalf("CreateFeed: %v", err)
	}

	res, err := svc.IngestContent(context.Background(), feed, icalFeed(eventOne), time.Now().UTC())
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if res.ReservationsCreated != 1 {
		t.Fatalf("expected 1 reservation created, got %+v", res)
	}

	rsv, err := svc.ListReservations(context.Background(), "tenant-a", "prop-1")
	if err != nil {
		t.Fatalf("ListReservations: %v", err)
	}
	if len(rsv) != 1 {
		t.Fatalf("expected 1 reservation, got %d", len(rsv))
	}
	if rsv[0].Status != reservations.ReservationStatusActive {
		t.Fatalf("expected active reservation, got %q", rsv[0].Status)
	}
	if rsv[0].ExternalEventID != "booking-1@x" || rsv[0].GuestSummary != "Guest one" {
		t.Fatalf("reservation lost source data: %+v", rsv[0])
	}
}

func TestCancelledReservationUpdatesRatherThanDeletes(t *testing.T) {
	svc := newTestService(t)
	feed, err := svc.CreateFeed(context.Background(), reservations.FeedParams{
		TenantID: "tenant-a", PropertyID: "prop-1", Source: "booking",
		URL: "https://example.invalid/feed.ics", PropertyTimezone: "Asia/Kolkata",
	}, []string{"operator"})
	if err != nil {
		t.Fatalf("CreateFeed: %v", err)
	}

	if _, err := svc.IngestContent(context.Background(), feed, icalFeed(eventOne, eventTwo), time.Now().UTC()); err != nil {
		t.Fatalf("first ingest: %v", err)
	}

	res, err := svc.IngestContent(context.Background(), feed, icalFeed(eventTwo), time.Now().UTC())
	if err != nil {
		t.Fatalf("second ingest: %v", err)
	}
	if res.ReservationsCancelled != 1 {
		t.Fatalf("expected 1 cancelled reservation, got %+v", res)
	}

	rsv, err := svc.ListReservations(context.Background(), "tenant-a", "prop-1")
	if err != nil {
		t.Fatalf("ListReservations: %v", err)
	}
	if len(rsv) != 2 {
		t.Fatalf("cancellation must preserve the reservation record, got %d", len(rsv))
	}
	bySource := map[string]reservations.Reservation{}
	for _, r := range rsv {
		bySource[r.ExternalEventID] = r
	}
	cancelled := bySource["booking-1@x"]
	if cancelled.Status != reservations.ReservationStatusCancelled {
		t.Fatalf("removed stay must be cancelled, got %q", cancelled.Status)
	}
	if cancelled.GuestSummary != "Guest one" {
		t.Fatalf("cancelled reservation must keep its content, got %q", cancelled.GuestSummary)
	}
	if bySource["booking-2@x"].Status != reservations.ReservationStatusActive {
		t.Fatalf("still-listed stay must remain active, got %q", bySource["booking-2@x"].Status)
	}
}

func TestExplicitCancellationUpdatesReservationInPlace(t *testing.T) {
	svc := newTestService(t)
	feed, err := svc.CreateFeed(context.Background(), reservations.FeedParams{
		TenantID: "tenant-a", PropertyID: "prop-1", Source: "airbnb",
		URL: "https://example.invalid/feed.ics", PropertyTimezone: "Asia/Kolkata",
	}, []string{"operator"})
	if err != nil {
		t.Fatalf("CreateFeed: %v", err)
	}
	if _, err := svc.IngestContent(context.Background(), feed, icalFeed(eventOne), time.Now().UTC()); err != nil {
		t.Fatalf("first ingest: %v", err)
	}

	cancelledEvent := strings.Replace(eventOne, "SUMMARY:Guest one", "STATUS:CANCELLED\nSUMMARY:Guest one", 1)
	if _, err := svc.IngestContent(context.Background(), feed, icalFeed(cancelledEvent), time.Now().UTC()); err != nil {
		t.Fatalf("cancelled ingest: %v", err)
	}

	rsv, err := svc.ListReservations(context.Background(), "tenant-a", "prop-1")
	if err != nil {
		t.Fatalf("ListReservations: %v", err)
	}
	if len(rsv) != 1 {
		t.Fatalf("explicit cancellation must update the reservation in place, got %d", len(rsv))
	}
	if rsv[0].Status != reservations.ReservationStatusCancelled {
		t.Fatalf("expected cancelled reservation, got %q", rsv[0].Status)
	}
}

func TestIngestCreatesDeterministicTurnoverProposals(t *testing.T) {
	svc := newTestService(t)
	feed, err := svc.CreateFeed(context.Background(), reservations.FeedParams{
		TenantID: "tenant-a", PropertyID: "prop-1", Source: "booking",
		URL: "https://example.invalid/feed.ics", PropertyTimezone: "Asia/Kolkata",
	}, []string{"operator"})
	if err != nil {
		t.Fatalf("CreateFeed: %v", err)
	}

	res, err := svc.IngestContent(context.Background(), feed, icalFeed(eventOne), time.Now().UTC())
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if res.ProposalsProposed != 2 {
		t.Fatalf("expected 2 proposals proposed, got %+v", res)
	}

	proposals, err := svc.ListProposals(context.Background(), "tenant-a", "prop-1")
	if err != nil {
		t.Fatalf("ListProposals: %v", err)
	}
	if len(proposals) != 2 {
		t.Fatalf("expected 2 proposals, got %d", len(proposals))
	}
	kinds := map[string]reservations.TurnoverProposal{}
	for _, p := range proposals {
		kinds[p.Kind] = p
		if p.Status != reservations.ProposalStatusProposed {
			t.Fatalf("proposal %q must be proposed, got %q", p.Kind, p.Status)
		}
	}
	if kinds[reservations.ProposalKindTurnover].ScheduledAt.UTC().Format(time.RFC3339) != "2024-03-05T04:30:00Z" {
		t.Fatalf("turnover must schedule at checkout: %s", kinds[reservations.ProposalKindTurnover].ScheduledAt)
	}
	if kinds[reservations.ProposalKindInspection].ScheduledAt.UTC().Format(time.RFC3339) != "2024-03-01T04:30:00Z" {
		t.Fatalf("inspection must schedule at check-in: %s", kinds[reservations.ProposalKindInspection].ScheduledAt)
	}
}

func TestCancelledReservationCancelsProposalsInsteadOfDeleting(t *testing.T) {
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

	res, err := svc.IngestContent(context.Background(), feed, icalFeed(eventTwo), time.Now().UTC())
	if err != nil {
		t.Fatalf("cancellation ingest: %v", err)
	}
	if res.ProposalsCancelled != 2 {
		t.Fatalf("expected 2 proposals cancelled, got %+v", res)
	}

	rsv, err := svc.ListReservations(context.Background(), "tenant-a", "prop-1")
	if err != nil {
		t.Fatalf("ListReservations: %v", err)
	}
	var cancelledReservationID string
	for _, r := range rsv {
		if r.ExternalEventID == "booking-1@x" {
			cancelledReservationID = r.ID
		}
	}
	if cancelledReservationID == "" {
		t.Fatal("cancelled reservation record must still exist")
	}

	proposals, err := svc.ListProposals(context.Background(), "tenant-a", "prop-1")
	if err != nil {
		t.Fatalf("ListProposals: %v", err)
	}
	cancelledProposals := 0
	for _, p := range proposals {
		if p.ReservationID != cancelledReservationID {
			continue
		}
		cancelledProposals++
		if p.Status != reservations.ProposalStatusCancelled {
			t.Fatalf("proposal %q of the cancelled stay must be cancelled, got %q", p.Kind, p.Status)
		}
	}
	if cancelledProposals != 2 {
		t.Fatalf("proposals of the cancelled stay must be preserved as cancelled, got %d", cancelledProposals)
	}
}

func TestReservationDateChangeUpdatesProposalInPlace(t *testing.T) {
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

	changed := strings.Replace(eventOne, "20240305T100000", "20240306T100000", 1)
	res, err := svc.IngestContent(context.Background(), feed, icalFeed(changed), time.Now().UTC())
	if err != nil {
		t.Fatalf("changed ingest: %v", err)
	}
	if res.ReservationsUpdated != 1 {
		t.Fatalf("expected 1 reservation updated, got %+v", res)
	}

	proposals, err := svc.ListProposals(context.Background(), "tenant-a", "prop-1")
	if err != nil {
		t.Fatalf("ListProposals: %v", err)
	}
	if len(proposals) != 2 {
		t.Fatalf("date change must not duplicate proposals, got %d", len(proposals))
	}
	for _, p := range proposals {
		if p.Kind == reservations.ProposalKindTurnover {
			if p.Status != reservations.ProposalStatusProposed {
				t.Fatalf("proposal must stay proposed, got %q", p.Status)
			}
			if p.ScheduledAt.UTC().Format(time.RFC3339) != "2024-03-06T04:30:00Z" {
				t.Fatalf("turnover proposal must follow the new checkout: %s", p.ScheduledAt)
			}
		}
	}
}

func TestIngestCreatesConflictLinkedToReservations(t *testing.T) {
	svc := newTestService(t)
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
		t.Fatalf("expected a conflict to be created, got %+v", res)
	}

	conflicts, err := svc.ListConflicts(context.Background(), "tenant-a", "prop-1")
	if err != nil {
		t.Fatalf("ListConflicts: %v", err)
	}
	var overlap *reservations.ReservationConflict
	for i := range conflicts {
		if conflicts[i].Kind == reservations.ExceptionKindOverlap {
			overlap = &conflicts[i]
			break
		}
	}
	if overlap == nil {
		t.Fatalf("expected overlap conflict, got %+v", conflicts)
	}
	if overlap.Status != reservations.ConflictStatusOpen {
		t.Fatalf("expected open conflict, got %q", overlap.Status)
	}
	if len(overlap.ReservationIDs) != 2 {
		t.Fatalf("conflict must reference both reservations, got %v", overlap.ReservationIDs)
	}
	if overlap.ExceptionID == "" {
		t.Fatalf("conflict must link to its calendar exception")
	}
}

func TestResolveConflictIsAudited(t *testing.T) {
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
	if _, err := svc.IngestContent(context.Background(), feed, icalFeed(eventOne, eventTwo), time.Now().UTC()); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	conflicts, err := svc.ListConflicts(context.Background(), "tenant-a", "prop-1")
	if err != nil || len(conflicts) == 0 {
		t.Fatalf("expected conflicts, got %d err=%v", len(conflicts), err)
	}
	conflict := conflicts[0]

	resolved, err := svc.ResolveConflict(context.Background(), "tenant-a", conflict.ID,
		reservations.ResolutionOutcomeConfirm, "verified against the listing", "actor-owner-1", "operator",
		[]string{"operator"})
	if err != nil {
		t.Fatalf("ResolveConflict: %v", err)
	}
	if resolved.Status != reservations.ConflictStatusResolved {
		t.Fatalf("expected resolved conflict, got %q", resolved.Status)
	}
	if resolved.ResolvedAt == nil {
		t.Fatalf("resolved conflict must carry a resolved_at timestamp")
	}

	var resolutionCount int
	if err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM reservation_conflict_resolutions
		 WHERE conflict_id = $1 AND actor_id = $2 AND outcome = $3`,
		conflict.ID, "actor-owner-1", reservations.ResolutionOutcomeConfirm,
	).Scan(&resolutionCount); err != nil {
		t.Fatalf("query resolutions: %v", err)
	}
	if resolutionCount != 1 {
		t.Fatalf("expected 1 audited resolution record, got %d", resolutionCount)
	}

	var auditCount int
	if err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM audit_events
		 WHERE action = 'reservation.conflict.resolve'
		   AND resource_id = $1 AND actor_id = $2`,
		conflict.ID, "actor-owner-1",
	).Scan(&auditCount); err != nil {
		t.Fatalf("query audit events: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("expected 1 audit event for the resolution, got %d", auditCount)
	}

	// The linked calendar exception converges on resolved too.
	var exceptionStatus string
	if err := pool.QueryRow(context.Background(),
		`SELECT status FROM calendar_exceptions WHERE id = $1`, conflict.ExceptionID,
	).Scan(&exceptionStatus); err != nil {
		t.Fatalf("query linked exception: %v", err)
	}
	if exceptionStatus != reservations.ExceptionStatusResolved {
		t.Fatalf("linked exception must be resolved, got %q", exceptionStatus)
	}
}

func TestConflictResolutionActorIsRequiredAndValidated(t *testing.T) {
	svc := newTestService(t)
	feed, err := svc.CreateFeed(context.Background(), reservations.FeedParams{
		TenantID: "tenant-a", PropertyID: "prop-1", Source: "booking",
		URL: "https://example.invalid/feed.ics", PropertyTimezone: "Asia/Kolkata",
		MinimumTurnaroundMinutes: 240,
	}, []string{"operator"})
	if err != nil {
		t.Fatalf("CreateFeed: %v", err)
	}
	if _, err := svc.IngestContent(context.Background(), feed, icalFeed(eventOne, eventTwo), time.Now().UTC()); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	conflicts, err := svc.ListConflicts(context.Background(), "tenant-a", "prop-1")
	if err != nil || len(conflicts) == 0 {
		t.Fatalf("expected conflicts, got %d err=%v", len(conflicts), err)
	}

	_, err = svc.ResolveConflict(context.Background(), "tenant-a", conflicts[0].ID,
		"auto-accept", "", "actor-1", "operator", []string{"operator"})
	if !errors.Is(err, reservations.ErrInvalidResolution) {
		t.Fatalf("unsupported outcome must be rejected, got %v", err)
	}

	_, err = svc.ResolveConflict(context.Background(), "tenant-a", conflicts[0].ID,
		reservations.ResolutionOutcomeConfirm, "", "", "operator", []string{"operator"})
	if !errors.Is(err, reservations.ErrInvalidResolution) {
		t.Fatalf("missing actor must be rejected, got %v", err)
	}
}

func TestJarvisCannotResolveConflicts(t *testing.T) {
	svc := newTestService(t)
	feed, err := svc.CreateFeed(context.Background(), reservations.FeedParams{
		TenantID: "tenant-a", PropertyID: "prop-1", Source: "booking",
		URL: "https://example.invalid/feed.ics", PropertyTimezone: "Asia/Kolkata",
		MinimumTurnaroundMinutes: 240,
	}, []string{"operator"})
	if err != nil {
		t.Fatalf("CreateFeed: %v", err)
	}
	if _, err := svc.IngestContent(context.Background(), feed, icalFeed(eventOne, eventTwo), time.Now().UTC()); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	conflicts, err := svc.ListConflicts(context.Background(), "tenant-a", "prop-1")
	if err != nil || len(conflicts) == 0 {
		t.Fatalf("expected conflicts, got %d err=%v", len(conflicts), err)
	}

	_, err = svc.ResolveConflict(context.Background(), "tenant-a", conflicts[0].ID,
		reservations.ResolutionOutcomeConfirm, "", "actor-hm", "jarvis",
		[]string{reservations.RoleJarvis})
	if !errors.Is(err, reservations.ErrSuperhostCannotMutate) {
		t.Fatalf("jarvis must not resolve conflicts, got %v", err)
	}

	_, err = svc.ResolveConflict(context.Background(), "tenant-a", conflicts[0].ID,
		reservations.ResolutionOutcomeConfirm, "", "actor-sh", "superhost",
		[]string{reservations.RoleSuperhost})
	if !errors.Is(err, reservations.ErrSuperhostCannotMutate) {
		t.Fatalf("superhost must not resolve conflicts, got %v", err)
	}
}

func TestMergeResolutionCancelsDuplicateReservation(t *testing.T) {
	svc := newTestService(t)
	feedA, err := svc.CreateFeed(context.Background(), reservations.FeedParams{
		TenantID: "tenant-a", PropertyID: "prop-1", Source: "airbnb",
		URL: "https://a.invalid/feed.ics", PropertyTimezone: "Asia/Kolkata",
	}, []string{"operator"})
	if err != nil {
		t.Fatalf("CreateFeed A: %v", err)
	}
	feedB, err := svc.CreateFeed(context.Background(), reservations.FeedParams{
		TenantID: "tenant-a", PropertyID: "prop-1", Source: "booking",
		URL: "https://b.invalid/feed.ics", PropertyTimezone: "Asia/Kolkata",
	}, []string{"operator"})
	if err != nil {
		t.Fatalf("CreateFeed B: %v", err)
	}
	if _, err := svc.IngestContent(context.Background(), feedA, icalFeed(eventOne), time.Now().UTC()); err != nil {
		t.Fatalf("ingest A: %v", err)
	}
	if _, err := svc.IngestContent(context.Background(), feedB, icalFeed(eventOne), time.Now().UTC()); err != nil {
		t.Fatalf("ingest B: %v", err)
	}

	conflicts, err := svc.ListConflicts(context.Background(), "tenant-a", "prop-1")
	if err != nil {
		t.Fatalf("ListConflicts: %v", err)
	}
	var duplicate *reservations.ReservationConflict
	for i := range conflicts {
		if conflicts[i].Kind == reservations.ExceptionKindDuplicate {
			duplicate = &conflicts[i]
			break
		}
	}
	if duplicate == nil {
		t.Fatalf("expected duplicate conflict, got %+v", conflicts)
	}

	resolved, err := svc.ResolveConflict(context.Background(), "tenant-a", duplicate.ID,
		reservations.ResolutionOutcomeMerge, "same guest booked through both channels", "actor-owner-1", "operator",
		[]string{"operator"})
	if err != nil {
		t.Fatalf("ResolveConflict(merge): %v", err)
	}
	if resolved.Status != reservations.ConflictStatusResolved {
		t.Fatalf("expected resolved conflict, got %q", resolved.Status)
	}

	rsv, err := svc.ListReservations(context.Background(), "tenant-a", "prop-1")
	if err != nil {
		t.Fatalf("ListReservations: %v", err)
	}
	if len(rsv) != 2 {
		t.Fatalf("merge must preserve both reservation records, got %d", len(rsv))
	}
	byFeed := map[string]reservations.Reservation{}
	for _, r := range rsv {
		byFeed[r.FeedID] = r
	}
	if byFeed[feedA.ID].Status != reservations.ReservationStatusActive {
		t.Fatalf("primary reservation must stay active, got %q", byFeed[feedA.ID].Status)
	}
	if byFeed[feedB.ID].Status != reservations.ReservationStatusCancelled {
		t.Fatalf("duplicate reservation must be cancelled in place, got %q", byFeed[feedB.ID].Status)
	}

	proposals, err := svc.ListProposals(context.Background(), "tenant-a", "prop-1")
	if err != nil {
		t.Fatalf("ListProposals: %v", err)
	}
	for _, p := range proposals {
		if p.ReservationID == byFeed[feedB.ID].ID && p.Status != reservations.ProposalStatusCancelled {
			t.Fatalf("duplicate reservation proposals must be cancelled, got %q", p.Status)
		}
	}
}

func TestDuplicateConflictsAreNotDuplicatedAcrossPolls(t *testing.T) {
	svc := newTestService(t)
	feedA, err := svc.CreateFeed(context.Background(), reservations.FeedParams{
		TenantID: "tenant-a", PropertyID: "prop-1", Source: "airbnb",
		URL: "https://a.invalid/feed.ics", PropertyTimezone: "Asia/Kolkata",
	}, []string{"operator"})
	if err != nil {
		t.Fatalf("CreateFeed A: %v", err)
	}
	feedB, err := svc.CreateFeed(context.Background(), reservations.FeedParams{
		TenantID: "tenant-a", PropertyID: "prop-1", Source: "booking",
		URL: "https://b.invalid/feed.ics", PropertyTimezone: "Asia/Kolkata",
	}, []string{"operator"})
	if err != nil {
		t.Fatalf("CreateFeed B: %v", err)
	}
	for i := 0; i < 2; i++ {
		if _, err := svc.IngestContent(context.Background(), feedA, icalFeed(eventOne), time.Now().UTC()); err != nil {
			t.Fatalf("ingest A: %v", err)
		}
		if _, err := svc.IngestContent(context.Background(), feedB, icalFeed(eventOne), time.Now().UTC()); err != nil {
			t.Fatalf("ingest B: %v", err)
		}
	}

	conflicts, err := svc.ListConflicts(context.Background(), "tenant-a", "prop-1")
	if err != nil {
		t.Fatalf("ListConflicts: %v", err)
	}
	open := 0
	for _, c := range conflicts {
		if c.Status == reservations.ConflictStatusOpen && c.Kind == reservations.ExceptionKindDuplicate {
			open++
		}
	}
	if open != 1 {
		t.Fatalf("expected exactly 1 open duplicate conflict, got %d", open)
	}
}

func TestGenerateTurnoverProposalsRefusesStaleFeed(t *testing.T) {
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

	result, err := svc.GenerateTurnoverProposals(context.Background(), "tenant-a", "prop-1", now.Add(31*time.Minute))
	if err != nil {
		t.Fatalf("GenerateTurnoverProposals: %v", err)
	}
	if !result.Skipped {
		t.Fatalf("stale feed must refuse proposal generation, got %+v", result)
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
		t.Fatalf("stale feed must remain visible as an exception, got %+v", exceptions)
	}
}

func TestGenerateTurnoverProposalsRunsWhenFeedIsFresh(t *testing.T) {
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

	result, err := svc.GenerateTurnoverProposals(context.Background(), "tenant-a", "prop-1", now.Add(5*time.Minute))
	if err != nil {
		t.Fatalf("GenerateTurnoverProposals: %v", err)
	}
	if result.Skipped {
		t.Fatalf("fresh feed must not skip proposal generation, got %+v", result)
	}

	proposals, err := svc.ListProposals(context.Background(), "tenant-a", "prop-1")
	if err != nil {
		t.Fatalf("ListProposals: %v", err)
	}
	if len(proposals) != 2 {
		t.Fatalf("expected 2 proposals for a fresh feed, got %d", len(proposals))
	}
}
