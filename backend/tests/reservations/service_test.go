package reservations_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"comfort-curators-backend/internal/platform/audit"
	"comfort-curators-backend/internal/platform/testdb"
	"comfort-curators-backend/internal/reservations"

	"github.com/jackc/pgx/v5/pgxpool"
)

type testAuthorizer struct {
	tenant string
	deny   bool
}

func (a testAuthorizer) RequireResourceAccess(ctx context.Context, resourceTenantID, resourceType, resourceID string) error {
	if a.deny {
		return errors.New("denied")
	}
	if a.tenant != "" && a.tenant != resourceTenantID {
		return errors.New("cross-tenant access denied")
	}
	return nil
}

func postgresAvailable() bool {
	host := os.Getenv("CC_DB_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("CC_DB_PORT")
	if port == "" {
		port = "5432"
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func dbConnString() string {
	host := os.Getenv("CC_DB_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("CC_DB_PORT")
	if port == "" {
		port = "5432"
	}
	user := os.Getenv("CC_DB_USER")
	if user == "" {
		user = "ccuser"
	}
	pass := os.Getenv("CC_DB_PASS")
	if pass == "" {
		pass = "ccpass"
	}
	name := testdb.MustName()
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, pass, host, port, name)
}

func reservationsPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if !postgresAvailable() {
		t.Skip("PostgreSQL not available for reservations integration test")
	}
	pool, err := pgxpool.New(context.Background(), dbConnString())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := reservations.EnsureSchema(context.Background(), pool); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}
	if err := audit.EnsureSchema(context.Background(), pool); err != nil {
		t.Fatalf("ensure audit schema: %v", err)
	}
	for _, table := range []string{
		"reservation_conflict_resolutions",
		"reservation_conflicts",
		"turnover_proposals",
		"reservations",
		"calendar_exceptions",
		"external_calendar_events",
		"calendar_feeds",
	} {
		if _, err := pool.Exec(context.Background(), "TRUNCATE "+table+" CASCADE"); err != nil {
			t.Fatalf("truncate %s: %v", table, err)
		}
	}
	return pool
}

func newTestService(t *testing.T) *reservations.CalendarService {
	t.Helper()
	pool := reservationsPool(t)
	return reservations.NewCalendarService(pool).WithAuthorizer(testAuthorizer{tenant: "tenant-a"})
}

func icalFeed(events ...string) string {
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\nVERSION:2.0\nPRODID:-//Test//EN\n")
	for _, e := range events {
		b.WriteString(e)
	}
	b.WriteString("END:VCALENDAR\n")
	return b.String()
}

const (
	eventOne = `BEGIN:VEVENT
UID:booking-1@x
DTSTART;TZID=Asia/Kolkata:20240301T100000
DTEND;TZID=Asia/Kolkata:20240305T100000
SUMMARY:Guest one
END:VEVENT
`
	eventTwo = `BEGIN:VEVENT
UID:booking-2@x
DTSTART;TZID=Asia/Kolkata:20240304T100000
DTEND;TZID=Asia/Kolkata:20240308T100000
SUMMARY:Guest two
END:VEVENT
`
	eventThree = `BEGIN:VEVENT
UID:booking-3@x
DTSTART;TZID=Asia/Kolkata:20240308T103000
DTEND;TZID=Asia/Kolkata:20240310T100000
SUMMARY:Guest three
END:VEVENT
`
)

func TestIngestDuplicatePollIsIdempotent(t *testing.T) {
	svc := newTestService(t)
	feed, err := svc.CreateFeed(context.Background(), reservations.FeedParams{
		TenantID: "tenant-a", PropertyID: "prop-1", Source: "airbnb",
		URL: "https://example.invalid/feed.ics", PropertyTimezone: "Asia/Kolkata",
	}, []string{"operator"})
	if err != nil {
		t.Fatalf("CreateFeed: %v", err)
	}

	content := icalFeed(eventOne)
	first, err := svc.IngestContent(context.Background(), feed, content, time.Now().UTC())
	if err != nil {
		t.Fatalf("first ingest: %v", err)
	}
	if first.EventsCreated != 1 {
		t.Fatalf("expected 1 event created, got %+v", first)
	}

	second, err := svc.IngestContent(context.Background(), feed, content, time.Now().UTC())
	if err != nil {
		t.Fatalf("second ingest: %v", err)
	}
	if !second.Unchanged {
		t.Fatalf("duplicate poll must be a no-op, got %+v", second)
	}

	events, err := svc.ListEvents(context.Background(), "tenant-a", "prop-1")
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("duplicate poll created extra events: got %d", len(events))
	}

	exceptions, err := svc.ListExceptions(context.Background(), "tenant-a", "prop-1")
	if err != nil {
		t.Fatalf("ListExceptions: %v", err)
	}
	if len(exceptions) != 0 {
		t.Fatalf("clean duplicate poll must not create exceptions, got %d", len(exceptions))
	}
}

func TestIngestChangePreservesEventIdentity(t *testing.T) {
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
	before, err := svc.ListEvents(context.Background(), "tenant-a", "prop-1")
	if err != nil || len(before) != 1 {
		t.Fatalf("expected 1 event after first ingest, got %d err=%v", len(before), err)
	}
	originalID := before[0].ID

	changed := strings.Replace(eventOne, "20240305T100000", "20240306T100000", 1)
	changed = strings.Replace(changed, "Guest one", "Guest one (extended)", 1)
	res, err := svc.IngestContent(context.Background(), feed, icalFeed(changed), time.Now().UTC())
	if err != nil {
		t.Fatalf("changed ingest: %v", err)
	}
	if res.EventsCreated != 0 || res.EventsUpdated != 1 {
		t.Fatalf("expected 1 update, 0 creates, got %+v", res)
	}

	after, err := svc.ListEvents(context.Background(), "tenant-a", "prop-1")
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(after) != 1 {
		t.Fatalf("change must update in place, not duplicate: got %d events", len(after))
	}
	if after[0].ID != originalID {
		t.Fatalf("event identity changed across polls: %s != %s", after[0].ID, originalID)
	}
	if after[0].ExternalEventID != "booking-1@x" {
		t.Fatalf("source id not preserved: %q", after[0].ExternalEventID)
	}
	if after[0].EndAt.UTC().Format(time.RFC3339) != "2024-03-06T04:30:00Z" {
		t.Fatalf("changed end not applied: %s", after[0].EndAt.UTC().Format(time.RFC3339))
	}
}

func TestIngestCancellationPreservesAcceptedWork(t *testing.T) {
	svc := newTestService(t)
	feed, err := svc.CreateFeed(context.Background(), reservations.FeedParams{
		TenantID: "tenant-a", PropertyID: "prop-1", Source: "vrbo",
		URL: "https://example.invalid/feed.ics", PropertyTimezone: "Asia/Kolkata",
	}, []string{"operator"})
	if err != nil {
		t.Fatalf("CreateFeed: %v", err)
	}

	if _, err := svc.IngestContent(context.Background(), feed, icalFeed(eventOne, eventTwo), time.Now().UTC()); err != nil {
		t.Fatalf("first ingest: %v", err)
	}

	// The feed no longer lists booking-1.
	if _, err := svc.IngestContent(context.Background(), feed, icalFeed(eventTwo), time.Now().UTC()); err != nil {
		t.Fatalf("second ingest: %v", err)
	}

	events, err := svc.ListEvents(context.Background(), "tenant-a", "prop-1")
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("cancellation must preserve both records, got %d", len(events))
	}

	byID := map[string]reservations.ExternalCalendarEvent{}
	for _, e := range events {
		byID[e.ExternalEventID] = e
	}
	gone := byID["booking-1@x"]
	if gone.Status != reservations.EventStatusNoLongerListed {
		t.Fatalf("removed event must be no_longer_listed, got %q", gone.Status)
	}
	if gone.Summary != "Guest one" {
		t.Fatalf("cancelled record must keep its content, got %q", gone.Summary)
	}
	kept := byID["booking-2@x"]
	if kept.Status != reservations.EventStatusConfirmed {
		t.Fatalf("still-listed event must remain confirmed, got %q", kept.Status)
	}
}

func TestIngestExplicitCancellationStatus(t *testing.T) {
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

	cancelled := strings.Replace(eventOne, "SUMMARY:Guest one", "STATUS:CANCELLED\nSUMMARY:Guest one", 1)
	if _, err := svc.IngestContent(context.Background(), feed, icalFeed(cancelled), time.Now().UTC()); err != nil {
		t.Fatalf("cancelled ingest: %v", err)
	}

	events, err := svc.ListEvents(context.Background(), "tenant-a", "prop-1")
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 record, got %d", len(events))
	}
	if events[0].Status != reservations.EventStatusCancelled {
		t.Fatalf("expected cancelled status, got %q", events[0].Status)
	}
}

func TestIngestCreatesOverlapAndTurnaroundExceptions(t *testing.T) {
	svc := newTestService(t)
	feed, err := svc.CreateFeed(context.Background(), reservations.FeedParams{
		TenantID: "tenant-a", PropertyID: "prop-1", Source: "booking",
		URL: "https://example.invalid/feed.ics", PropertyTimezone: "Asia/Kolkata",
		MinimumTurnaroundMinutes: 240,
	}, []string{"operator"})
	if err != nil {
		t.Fatalf("CreateFeed: %v", err)
	}

	// eventOne overlaps eventTwo, and eventThree leaves only a 30 minute
	// window after eventTwo which is below the 240 minute minimum.
	_, err = svc.IngestContent(context.Background(), feed, icalFeed(eventOne, eventTwo, eventThree), time.Now().UTC())
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}

	exceptions, err := svc.ListExceptions(context.Background(), "tenant-a", "prop-1")
	if err != nil {
		t.Fatalf("ListExceptions: %v", err)
	}

	kinds := map[string]bool{}
	for _, e := range exceptions {
		kinds[e.Kind] = true
	}
	if !kinds[reservations.ExceptionKindOverlap] {
		t.Fatalf("expected overlap exception, got %v", kinds)
	}
	if !kinds[reservations.ExceptionKindImpossibleTurnaround] {
		t.Fatalf("expected turnaround exception, got %v", kinds)
	}
}

func TestIngestCreatesDuplicateExceptionAcrossFeeds(t *testing.T) {
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

	exceptions, err := svc.ListExceptions(context.Background(), "tenant-a", "prop-1")
	if err != nil {
		t.Fatalf("ListExceptions: %v", err)
	}
	var dup *reservations.CalendarException
	for i := range exceptions {
		if exceptions[i].Kind == reservations.ExceptionKindDuplicate {
			dup = &exceptions[i]
			break
		}
	}
	if dup == nil {
		t.Fatalf("expected duplicate exception, got %+v", exceptions)
	}
	if dup.Status != reservations.ExceptionStatusOpen {
		t.Fatalf("expected open exception, got %q", dup.Status)
	}
}

func TestIngestCreatesTimezoneAmbiguityException(t *testing.T) {
	svc := newTestService(t)
	feed, err := svc.CreateFeed(context.Background(), reservations.FeedParams{
		TenantID: "tenant-a", PropertyID: "prop-1", Source: "airbnb",
		URL: "https://example.invalid/feed.ics", PropertyTimezone: "Asia/Kolkata",
	}, []string{"operator"})
	if err != nil {
		t.Fatalf("CreateFeed: %v", err)
	}

	floating := strings.Replace(eventOne, "DTSTART;TZID=Asia/Kolkata:", "DTSTART:", 1)
	floating = strings.Replace(floating, "DTEND;TZID=Asia/Kolkata:", "DTEND:", 1)
	if _, err := svc.IngestContent(context.Background(), feed, icalFeed(floating), time.Now().UTC()); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	exceptions, err := svc.ListExceptions(context.Background(), "tenant-a", "prop-1")
	if err != nil {
		t.Fatalf("ListExceptions: %v", err)
	}
	found := false
	for _, e := range exceptions {
		if e.Kind == reservations.ExceptionKindTimezoneAmbiguity {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected timezone ambiguity exception, got %+v", exceptions)
	}
}

func TestDuplicatePollDoesNotDuplicateExceptions(t *testing.T) {
	svc := newTestService(t)
	feed, err := svc.CreateFeed(context.Background(), reservations.FeedParams{
		TenantID: "tenant-a", PropertyID: "prop-1", Source: "booking",
		URL: "https://example.invalid/feed.ics", PropertyTimezone: "Asia/Kolkata",
		MinimumTurnaroundMinutes: 240,
	}, []string{"operator"})
	if err != nil {
		t.Fatalf("CreateFeed: %v", err)
	}

	content := icalFeed(eventOne, eventTwo)
	for i := 0; i < 3; i++ {
		if _, err := svc.IngestContent(context.Background(), feed, content, time.Now().UTC()); err != nil {
			t.Fatalf("ingest %d: %v", i, err)
		}
	}

	exceptions, err := svc.ListExceptions(context.Background(), "tenant-a", "prop-1")
	if err != nil {
		t.Fatalf("ListExceptions: %v", err)
	}
	counts := map[string]int{}
	for _, e := range exceptions {
		if e.Status == reservations.ExceptionStatusOpen {
			counts[e.Kind]++
		}
	}
	for kind, n := range counts {
		if n > 1 {
			t.Fatalf("kind %q duplicated across polls: %d open exceptions", kind, n)
		}
	}
}

func TestPollFeedReadsOnlyAndRecordsFailure(t *testing.T) {
	svc := newTestService(t)

	var gotRequests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRequests++
		w.Header().Set("Content-Type", "text/calendar")
		w.Write([]byte(icalFeed(eventOne)))
	}))
	defer srv.Close()

	feed, err := svc.CreateFeed(context.Background(), reservations.FeedParams{
		TenantID: "tenant-a", PropertyID: "prop-1", Source: "airbnb",
		URL: srv.URL + "/calendar.ics", PropertyTimezone: "Asia/Kolkata",
	}, []string{"operator"})
	if err != nil {
		t.Fatalf("CreateFeed: %v", err)
	}

	if _, err := svc.PollFeed(context.Background(), feed.ID); err != nil {
		t.Fatalf("PollFeed: %v", err)
	}
	if gotRequests != 1 {
		t.Fatalf("expected 1 GET request, got %d", gotRequests)
	}

	events, err := svc.ListEvents(context.Background(), "tenant-a", "prop-1")
	if err != nil || len(events) != 1 {
		t.Fatalf("expected 1 event after poll, got %d err=%v", len(events), err)
	}
	if events[0].Source != "airbnb" {
		t.Fatalf("source not preserved: %q", events[0].Source)
	}
}

func TestPollFeedFailureCreatesVisibleException(t *testing.T) {
	svc := newTestService(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream down", http.StatusBadGateway)
	}))
	defer srv.Close()

	feed, err := svc.CreateFeed(context.Background(), reservations.FeedParams{
		TenantID: "tenant-a", PropertyID: "prop-1", Source: "airbnb",
		URL: srv.URL + "/calendar.ics", PropertyTimezone: "Asia/Kolkata",
	}, []string{"operator"})
	if err != nil {
		t.Fatalf("CreateFeed: %v", err)
	}

	if _, err := svc.PollFeed(context.Background(), feed.ID); err == nil {
		t.Fatal("expected poll failure")
	}

	exceptions, err := svc.ListExceptions(context.Background(), "tenant-a", "prop-1")
	if err != nil {
		t.Fatalf("ListExceptions: %v", err)
	}
	found := false
	for _, e := range exceptions {
		if e.Kind == reservations.ExceptionKindFeedFailure && e.Status == reservations.ExceptionStatusOpen {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected feed_failure exception, got %+v", exceptions)
	}
}

func TestScanStaleFeedsCreatesVisibleException(t *testing.T) {
	svc := newTestService(t)
	feed, err := svc.CreateFeed(context.Background(), reservations.FeedParams{
		TenantID: "tenant-a", PropertyID: "prop-1", Source: "airbnb",
		URL: "https://example.invalid/feed.ics", PropertyTimezone: "Asia/Kolkata",
		StaleAfterMinutes: 30,
	}, []string{"operator"})
	if err != nil {
		t.Fatalf("CreateFeed: %v", err)
	}

	// A successful poll makes the feed fresh at "now".
	now := time.Date(2024, 4, 1, 10, 0, 0, 0, time.UTC)
	if _, err := svc.IngestContent(context.Background(), feed, icalFeed(eventOne), now); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	result, err := svc.ScanStaleFeeds(context.Background(), now.Add(15*time.Minute))
	if err != nil {
		t.Fatalf("ScanStaleFeeds: %v", err)
	}
	if result.StaleFeeds != 0 {
		t.Fatalf("fresh feed must not be stale, got %d", result.StaleFeeds)
	}

	result, err = svc.ScanStaleFeeds(context.Background(), now.Add(31*time.Minute))
	if err != nil {
		t.Fatalf("ScanStaleFeeds: %v", err)
	}
	if result.StaleFeeds != 1 {
		t.Fatalf("expected 1 stale feed, got %d", result.StaleFeeds)
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
		t.Fatalf("expected stale_feed exception, got %+v", exceptions)
	}

	health, err := svc.FeedHealth(context.Background(), "tenant-a", "prop-1", now.Add(31*time.Minute))
	if err != nil {
		t.Fatalf("FeedHealth: %v", err)
	}
	if len(health) != 1 || !health[0].Stale {
		t.Fatalf("expected stale feed health, got %+v", health)
	}
}

func TestSuccessfulPollResolvesStaleException(t *testing.T) {
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
	if _, err := svc.ScanStaleFeeds(context.Background(), now.Add(31*time.Minute)); err != nil {
		t.Fatalf("ScanStaleFeeds: %v", err)
	}

	if _, err := svc.IngestContent(context.Background(), feed, icalFeed(eventOne), now.Add(32*time.Minute)); err != nil {
		t.Fatalf("recovery ingest: %v", err)
	}

	exceptions, err := svc.ListExceptions(context.Background(), "tenant-a", "prop-1")
	if err != nil {
		t.Fatalf("ListExceptions: %v", err)
	}
	for _, e := range exceptions {
		if e.Kind == reservations.ExceptionKindStaleFeed && e.Status == reservations.ExceptionStatusOpen {
			t.Fatalf("stale exception must be resolved after a fresh poll, got %+v", e)
		}
	}
}

func TestJarvisCannotMutateCalendarConfiguration(t *testing.T) {
	svc := newTestService(t)

	_, err := svc.CreateFeed(context.Background(), reservations.FeedParams{
		TenantID: "tenant-a", PropertyID: "prop-1", Source: "airbnb",
		URL: "https://example.invalid/feed.ics", PropertyTimezone: "Asia/Kolkata",
	}, []string{reservations.RoleJarvis})
	if !errors.Is(err, reservations.ErrSuperhostCannotMutate) {
		t.Fatalf("jarvis must not create feeds, got %v", err)
	}

	feed, err := svc.CreateFeed(context.Background(), reservations.FeedParams{
		TenantID: "tenant-a", PropertyID: "prop-1", Source: "airbnb",
		URL: "https://example.invalid/feed.ics", PropertyTimezone: "Asia/Kolkata",
	}, []string{"operator"})
	if err != nil {
		t.Fatalf("CreateFeed: %v", err)
	}

	_, err = svc.SetFeedStatus(context.Background(), "tenant-a", feed.ID, reservations.FeedStatusPaused, []string{reservations.RoleJarvis})
	if !errors.Is(err, reservations.ErrSuperhostCannotMutate) {
		t.Fatalf("jarvis must not change feed status, got %v", err)
	}

	// An operator can manage the feed; the external calendar itself is never
	// written back to.
	updated, err := svc.SetFeedStatus(context.Background(), "tenant-a", feed.ID, reservations.FeedStatusPaused, []string{"operator"})
	if err != nil {
		t.Fatalf("operator SetFeedStatus: %v", err)
	}
	if updated.Status != reservations.FeedStatusPaused {
		t.Fatalf("expected paused, got %q", updated.Status)
	}

	_, err = svc.CreateFeed(context.Background(), reservations.FeedParams{
		TenantID: "tenant-a", PropertyID: "prop-1", Source: "airbnb",
		URL: "https://example.invalid/feed.ics", PropertyTimezone: "Asia/Kolkata",
	}, []string{reservations.RoleSuperhost})
	if !errors.Is(err, reservations.ErrSuperhostCannotMutate) {
		t.Fatalf("superhost must not create feeds, got %v", err)
	}

	_, err = svc.SetFeedStatus(context.Background(), "tenant-a", feed.ID, reservations.FeedStatusPaused, []string{reservations.RoleSuperhost})
	if !errors.Is(err, reservations.ErrSuperhostCannotMutate) {
		t.Fatalf("superhost must not change feed status, got %v", err)
	}
}

func TestJarvisCannotResolveCalendarExceptions(t *testing.T) {
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
	exceptions, err := svc.ListExceptions(context.Background(), "tenant-a", "prop-1")
	if err != nil || len(exceptions) == 0 {
		t.Fatalf("expected exceptions, got %d err=%v", len(exceptions), err)
	}

	_, err = svc.ResolveException(context.Background(), "tenant-a", exceptions[0].ID, []string{reservations.RoleJarvis})
	if !errors.Is(err, reservations.ErrSuperhostCannotMutate) {
		t.Fatalf("jarvis must not resolve calendar exceptions, got %v", err)
	}

	resolved, err := svc.ResolveException(context.Background(), "tenant-a", exceptions[0].ID, []string{"operator"})
	if err != nil {
		t.Fatalf("operator ResolveException: %v", err)
	}
	if resolved.Status != reservations.ExceptionStatusResolved {
		t.Fatalf("expected resolved, got %q", resolved.Status)
	}

	_, err = svc.ResolveException(context.Background(), "tenant-a", exceptions[0].ID, []string{reservations.RoleSuperhost})
	if !errors.Is(err, reservations.ErrSuperhostCannotMutate) {
		t.Fatalf("superhost must not resolve calendar exceptions, got %v", err)
	}
}

func TestCrossTenantAccessIsDenied(t *testing.T) {
	pool := reservationsPool(t)

	// A tenant-a authorizer rejects a feed created for tenant-b.
	tenantScoped := reservations.NewCalendarService(pool).WithAuthorizer(testAuthorizer{tenant: "tenant-a"})
	_, err := tenantScoped.CreateFeed(context.Background(), reservations.FeedParams{
		TenantID: "tenant-b", PropertyID: "prop-9", Source: "airbnb",
		URL: "https://example.invalid/feed.ics", PropertyTimezone: "Asia/Kolkata",
	}, []string{"operator"})
	if err == nil {
		t.Fatal("expected cross-tenant create to be denied")
	}

	// A deny-all authorizer proves reads fail closed before any data is
	// disclosed, including on the read surfaces.
	denyAll := reservations.NewCalendarService(pool).WithAuthorizer(testAuthorizer{deny: true})
	if _, err := denyAll.ListEvents(context.Background(), "tenant-a", "prop-1"); err == nil {
		t.Fatal("expected authorization denial to precede read disclosure")
	}
	if _, err := denyAll.ListExceptions(context.Background(), "tenant-a", "prop-1"); err == nil {
		t.Fatal("expected authorization denial to precede exception disclosure")
	}
	if _, err := denyAll.FeedHealth(context.Background(), "tenant-a", "prop-1", time.Now().UTC()); err == nil {
		t.Fatal("expected authorization denial to precede health disclosure")
	}
}

func TestResolveExceptionIsIdempotentForUnknown(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.ResolveException(context.Background(), "tenant-a", "does-not-exist", []string{"operator"})
	if !errors.Is(err, reservations.ErrExceptionNotFound) {
		t.Fatalf("expected ErrExceptionNotFound, got %v", err)
	}
}
