package reservations

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newID(prefix string) string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(b[:])
}

// querier is satisfied by both *pgxpool.Pool and pgx.Tx so store operations
// can run inside a transaction when atomicity requires it.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type CalendarStore struct {
	pool *pgxpool.Pool
}

func NewCalendarStore(pool *pgxpool.Pool) *CalendarStore {
	return &CalendarStore{pool: pool}
}

const feedColumns = `id, tenant_id, property_id, source, url, status,
	property_timezone, stale_after_minutes, minimum_turnaround_minutes,
	last_polled_at, last_success_at, last_content_hash, last_error,
	version, created_at, updated_at`

func scanFeed(row pgx.Row) (*CalendarFeed, error) {
	var f CalendarFeed
	var lastContentHash, lastError *string
	err := row.Scan(
		&f.ID, &f.TenantID, &f.PropertyID, &f.Source, &f.URL, &f.Status,
		&f.PropertyTimezone, &f.StaleAfterMinutes, &f.MinimumTurnaroundMinutes,
		&f.LastPolledAt, &f.LastSuccessAt, &lastContentHash, &lastError,
		&f.Version, &f.CreatedAt, &f.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrFeedNotFound
		}
		return nil, fmt.Errorf("scan calendar feed: %w", err)
	}
	if lastContentHash != nil {
		f.LastContentHash = *lastContentHash
	}
	if lastError != nil {
		f.LastError = *lastError
	}
	return &f, nil
}

func (s *CalendarStore) InsertFeed(ctx context.Context, f *CalendarFeed) error {
	if f.ID == "" {
		f.ID = newID("feed")
	}
	if f.Version == 0 {
		f.Version = 1
	}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO calendar_feeds (
			id, tenant_id, property_id, source, url, status,
			property_timezone, stale_after_minutes, minimum_turnaround_minutes,
			version, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10, NOW(), NOW())
		RETURNING created_at, updated_at
	`,
		f.ID, f.TenantID, f.PropertyID, f.Source, f.URL, f.Status,
		f.PropertyTimezone, f.StaleAfterMinutes, f.MinimumTurnaroundMinutes, f.Version,
	).Scan(&f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert calendar feed: %w", err)
	}
	return nil
}

func (s *CalendarStore) GetFeed(ctx context.Context, tenantID, feedID string) (*CalendarFeed, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+feedColumns+` FROM calendar_feeds WHERE id = $1 AND tenant_id = $2`,
		feedID, tenantID,
	)
	return scanFeed(row)
}

func (s *CalendarStore) ListFeeds(ctx context.Context, tenantID, propertyID string) ([]CalendarFeed, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+feedColumns+` FROM calendar_feeds
		 WHERE tenant_id = $1 AND property_id = $2 ORDER BY created_at ASC`,
		tenantID, propertyID,
	)
	if err != nil {
		return nil, fmt.Errorf("list calendar feeds: %w", err)
	}
	defer rows.Close()

	var feeds []CalendarFeed
	for rows.Next() {
		f, err := scanFeed(rows)
		if err != nil {
			return nil, err
		}
		feeds = append(feeds, *f)
	}
	return feeds, rows.Err()
}

// ListActiveFeeds returns every active feed regardless of tenant. It is the
// worker-facing sweep cursor for feed polling.
func (s *CalendarStore) ListActiveFeeds(ctx context.Context) ([]CalendarFeed, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+feedColumns+` FROM calendar_feeds
		 WHERE status = 'active' ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list active calendar feeds: %w", err)
	}
	defer rows.Close()

	var feeds []CalendarFeed
	for rows.Next() {
		f, err := scanFeed(rows)
		if err != nil {
			return nil, err
		}
		feeds = append(feeds, *f)
	}
	return feeds, rows.Err()
}

// loadFeedForUpdate locks a feed row for the duration of a poll transaction
// so two concurrent polls of the same feed serialize instead of racing the
// content-hash idempotency check.
func loadFeedForUpdate(ctx context.Context, q querier, feedID, tenantID string) (*CalendarFeed, error) {
	row := q.QueryRow(ctx,
		`SELECT `+feedColumns+` FROM calendar_feeds WHERE id = $1 AND tenant_id = $2 FOR UPDATE`,
		feedID, tenantID,
	)
	return scanFeed(row)
}

func (s *CalendarStore) SetFeedStatus(ctx context.Context, tenantID, feedID, status string, at time.Time) (*CalendarFeed, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE calendar_feeds
		SET status = $3, updated_at = $4, version = version + 1
		WHERE id = $1 AND tenant_id = $2
	`, feedID, tenantID, status, at)
	if err != nil {
		return nil, fmt.Errorf("set calendar feed status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrFeedNotFound
	}
	return s.GetFeed(ctx, tenantID, feedID)
}

func (s *CalendarStore) RecordPollFailure(ctx context.Context, q querier, feedID, tenantID, errMsg string, at time.Time) error {
	_, err := q.Exec(ctx, `
		UPDATE calendar_feeds
		SET last_polled_at = $3, last_error = $4, updated_at = $3
		WHERE id = $1 AND tenant_id = $2
	`, feedID, tenantID, at, errMsg)
	if err != nil {
		return fmt.Errorf("record feed poll failure: %w", err)
	}
	return nil
}

func (s *CalendarStore) RecordPollSuccess(ctx context.Context, q querier, feedID, tenantID, hash string, at time.Time) error {
	_, err := q.Exec(ctx, `
		UPDATE calendar_feeds
		SET last_polled_at = $3, last_success_at = $3, last_content_hash = $4,
		    last_error = NULL, updated_at = $3, version = version + 1
		WHERE id = $1 AND tenant_id = $2
	`, feedID, tenantID, at, hash)
	if err != nil {
		return fmt.Errorf("record feed poll success: %w", err)
	}
	return nil
}

func (s *CalendarStore) UpsertEvent(ctx context.Context, q querier, ev *ExternalCalendarEvent, now time.Time) (created bool, err error) {
	if ev.ID == "" {
		ev.ID = newID("evt")
	}
	if ev.FirstSeenAt.IsZero() {
		ev.FirstSeenAt = now
	}
	ev.LastSeenAt = now

	var id string
	var inserted bool
	err = q.QueryRow(ctx, `
		INSERT INTO external_calendar_events (
			id, tenant_id, property_id, feed_id, external_event_id, source,
			summary, description, start_at, end_at, all_day, timezone,
			timezone_ambiguous, status, sequence, raw_ical,
			first_seen_at, last_seen_at, version, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,NOW(),NOW())
		ON CONFLICT (feed_id, external_event_id) DO UPDATE SET
			summary = EXCLUDED.summary,
			description = EXCLUDED.description,
			start_at = EXCLUDED.start_at,
			end_at = EXCLUDED.end_at,
			all_day = EXCLUDED.all_day,
			timezone = EXCLUDED.timezone,
			timezone_ambiguous = EXCLUDED.timezone_ambiguous,
			status = EXCLUDED.status,
			sequence = EXCLUDED.sequence,
			raw_ical = EXCLUDED.raw_ical,
			last_seen_at = EXCLUDED.last_seen_at,
			updated_at = NOW(),
			version = external_calendar_events.version + 1
		RETURNING id, (xmax = 0) AS inserted
	`,
		ev.ID, ev.TenantID, ev.PropertyID, ev.FeedID, ev.ExternalEventID, ev.Source,
		ev.Summary, ev.Description, ev.StartAt, ev.EndAt, ev.AllDay, ev.Timezone,
		ev.TimezoneAmbiguous, ev.Status, ev.Sequence, ev.RawICal,
		ev.FirstSeenAt, ev.LastSeenAt, 1,
	).Scan(&id, &inserted)
	if err != nil {
		return false, fmt.Errorf("upsert external calendar event: %w", err)
	}
	ev.ID = id
	return inserted, nil
}

const eventColumns = `id, tenant_id, property_id, feed_id, external_event_id, source,
	summary, description, start_at, end_at, all_day, timezone,
	timezone_ambiguous, status, sequence, raw_ical,
	first_seen_at, last_seen_at, created_at, updated_at`

func scanEvent(row pgx.Row) (*ExternalCalendarEvent, error) {
	var ev ExternalCalendarEvent
	var summary, description, timezone *string
	err := row.Scan(
		&ev.ID, &ev.TenantID, &ev.PropertyID, &ev.FeedID, &ev.ExternalEventID, &ev.Source,
		&summary, &description, &ev.StartAt, &ev.EndAt, &ev.AllDay, &timezone,
		&ev.TimezoneAmbiguous, &ev.Status, &ev.Sequence, &ev.RawICal,
		&ev.FirstSeenAt, &ev.LastSeenAt, &ev.CreatedAt, &ev.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrEventNotFound
		}
		return nil, fmt.Errorf("scan external calendar event: %w", err)
	}
	if summary != nil {
		ev.Summary = *summary
	}
	if description != nil {
		ev.Description = *description
	}
	if timezone != nil {
		ev.Timezone = *timezone
	}
	return &ev, nil
}

func (s *CalendarStore) ListActiveEventsByProperty(ctx context.Context, q querier, tenantID, propertyID string) ([]*ExternalCalendarEvent, error) {
	rows, err := q.Query(ctx,
		`SELECT `+eventColumns+` FROM external_calendar_events
		 WHERE tenant_id = $1 AND property_id = $2
		   AND status IN ('confirmed', 'tentative')
		 ORDER BY start_at ASC`,
		tenantID, propertyID,
	)
	if err != nil {
		return nil, fmt.Errorf("list active external calendar events: %w", err)
	}
	defer rows.Close()
	return scanEventPtrs(rows)
}

// MarkMissingEventsCancelled flips previously active events that a poll no
// longer lists to no_longer_listed. Records are preserved, never deleted, so
// accepted work derived from them is not silently destroyed.
func (s *CalendarStore) MarkMissingEventsCancelled(ctx context.Context, q querier, feedID, tenantID string, seen map[string]bool, at time.Time) (int, error) {
	tag, err := q.Exec(ctx, `
		UPDATE external_calendar_events
		SET status = 'no_longer_listed', last_seen_at = $4, updated_at = $4,
		    version = version + 1
		WHERE feed_id = $1 AND tenant_id = $2
		  AND status IN ('confirmed', 'tentative')
		  AND external_event_id <> ALL($3::text[])
	`, feedID, tenantID, seenKeys(seen), at)
	if err != nil {
		return 0, fmt.Errorf("mark missing events cancelled: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

func seenKeys(seen map[string]bool) []string {
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	return keys
}

func (s *CalendarStore) ListEventsByProperty(ctx context.Context, tenantID, propertyID string) ([]ExternalCalendarEvent, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+eventColumns+` FROM external_calendar_events
		 WHERE tenant_id = $1 AND property_id = $2 ORDER BY start_at ASC`,
		tenantID, propertyID,
	)
	if err != nil {
		return nil, fmt.Errorf("list external calendar events by property: %w", err)
	}
	defer rows.Close()
	return scanEvents(rows)
}

func scanEvents(rows pgx.Rows) ([]ExternalCalendarEvent, error) {
	var events []ExternalCalendarEvent
	for rows.Next() {
		ev, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, *ev)
	}
	return events, rows.Err()
}

func scanEventPtrs(rows pgx.Rows) ([]*ExternalCalendarEvent, error) {
	var events []*ExternalCalendarEvent
	for rows.Next() {
		ev, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	return events, rows.Err()
}

func (s *CalendarStore) InsertException(ctx context.Context, q querier, exc *CalendarException) (bool, error) {
	if exc.ID == "" {
		exc.ID = newID("exc")
	}
	if exc.Status == "" {
		exc.Status = ExceptionStatusOpen
	}
	meta, err := json.Marshal(exc.Metadata)
	if err != nil {
		return false, fmt.Errorf("marshal exception metadata: %w", err)
	}

	tag, err := q.Exec(ctx, `
		INSERT INTO calendar_exceptions (
			id, tenant_id, property_id, feed_id, kind, severity, status,
			message, dedupe_key, metadata, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (tenant_id, property_id, kind, dedupe_key)
		WHERE status = 'open' DO NOTHING
	`,
		exc.ID, exc.TenantID, exc.PropertyID, exc.FeedID, exc.Kind, exc.Severity,
		exc.Status, exc.Message, exc.DedupeKey, meta, exc.CreatedAt,
	)
	if err != nil {
		return false, fmt.Errorf("insert calendar exception: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

const exceptionColumns = `id, tenant_id, property_id, feed_id, kind, severity, status,
	message, dedupe_key, metadata, created_at, resolved_at`

func scanException(row pgx.Row) (*CalendarException, error) {
	var exc CalendarException
	var meta []byte
	var feedID *string
	err := row.Scan(
		&exc.ID, &exc.TenantID, &exc.PropertyID, &feedID, &exc.Kind, &exc.Severity,
		&exc.Status, &exc.Message, &exc.DedupeKey, &meta, &exc.CreatedAt, &exc.ResolvedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrExceptionNotFound
		}
		return nil, fmt.Errorf("scan calendar exception: %w", err)
	}
	if feedID != nil {
		exc.FeedID = *feedID
	}
	if len(meta) > 0 {
		_ = json.Unmarshal(meta, &exc.Metadata)
	}
	return &exc, nil
}

func (s *CalendarStore) ListExceptions(ctx context.Context, tenantID, propertyID string) ([]CalendarException, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+exceptionColumns+` FROM calendar_exceptions
		 WHERE tenant_id = $1 AND property_id = $2 ORDER BY created_at DESC`,
		tenantID, propertyID,
	)
	if err != nil {
		return nil, fmt.Errorf("list calendar exceptions: %w", err)
	}
	defer rows.Close()

	var exceptions []CalendarException
	for rows.Next() {
		exc, err := scanException(rows)
		if err != nil {
			return nil, err
		}
		exceptions = append(exceptions, *exc)
	}
	return exceptions, rows.Err()
}

func (s *CalendarStore) ResolveException(ctx context.Context, tenantID, exceptionID string, at time.Time) (*CalendarException, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE calendar_exceptions
		SET status = 'resolved', resolved_at = $3
		WHERE id = $1 AND tenant_id = $2 AND status = 'open'
	`, exceptionID, tenantID, at)
	if err != nil {
		return nil, fmt.Errorf("resolve calendar exception: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrExceptionNotFound
	}
	return s.GetException(ctx, tenantID, exceptionID)
}

func (s *CalendarStore) GetException(ctx context.Context, tenantID, exceptionID string) (*CalendarException, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+exceptionColumns+` FROM calendar_exceptions WHERE id = $1 AND tenant_id = $2`,
		exceptionID, tenantID,
	)
	return scanException(row)
}

// ResolveFeedExceptions closes open feed-health exceptions for a feed after
// a successful poll.
func (s *CalendarStore) ResolveFeedExceptions(ctx context.Context, q querier, tenantID, feedID string, kinds []string, at time.Time) (int, error) {
	tag, err := q.Exec(ctx, `
		UPDATE calendar_exceptions
		SET status = 'resolved', resolved_at = $4
		WHERE tenant_id = $1 AND feed_id = $2
		  AND status = 'open' AND kind = ANY($3::text[])
	`, tenantID, feedID, kinds, at)
	if err != nil {
		return 0, fmt.Errorf("resolve feed exceptions: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

func (s *CalendarStore) CountOpenExceptions(ctx context.Context, tenantID, feedID string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM calendar_exceptions
		 WHERE tenant_id = $1 AND feed_id = $2 AND status = 'open'`,
		tenantID, feedID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count open exceptions: %w", err)
	}
	return count, nil
}
