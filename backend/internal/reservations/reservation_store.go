package reservations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

const reservationColumns = `id, tenant_id, property_id, feed_id, external_event_id, source,
	guest_summary, status, start_at, end_at, all_day, timezone, sequence,
	version, created_at, updated_at`

func scanReservation(row pgx.Row) (*Reservation, error) {
	var r Reservation
	var summary, timezone *string
	err := row.Scan(
		&r.ID, &r.TenantID, &r.PropertyID, &r.FeedID, &r.ExternalEventID, &r.Source,
		&summary, &r.Status, &r.StartAt, &r.EndAt, &r.AllDay, &timezone, &r.Sequence,
		&r.Version, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrReservationNotFound
		}
		return nil, fmt.Errorf("scan reservation: %w", err)
	}
	if summary != nil {
		r.GuestSummary = *summary
	}
	if timezone != nil {
		r.Timezone = *timezone
	}
	return &r, nil
}

// ListAllEventsByProperty returns every external event for a property
// including cancelled and no-longer-listed ones so reservation sync can
// cancel reservations whose source event stopped being active.
func (s *CalendarStore) ListAllEventsByProperty(ctx context.Context, q querier, tenantID, propertyID string) ([]*ExternalCalendarEvent, error) {
	rows, err := q.Query(ctx,
		`SELECT `+eventColumns+` FROM external_calendar_events
		 WHERE tenant_id = $1 AND property_id = $2 ORDER BY start_at ASC`,
		tenantID, propertyID,
	)
	if err != nil {
		return nil, fmt.Errorf("list all external calendar events: %w", err)
	}
	defer rows.Close()
	return scanEventPtrs(rows)
}

// UpsertReservation synchronizes one normalized reservation with its source
// event. The reservation is matched by (feed_id, external_event_id), updated
// in place when anything changed, and never deleted. It returns whether the
// row was newly created and whether its content or status actually changed.
func (s *CalendarStore) UpsertReservation(ctx context.Context, q querier, r *Reservation, now time.Time) (created, changed bool, err error) {
	existing, err := s.getReservationBySource(ctx, q, r.FeedID, r.ExternalEventID)
	if err != nil {
		if !errors.Is(err, ErrReservationNotFound) {
			return false, false, err
		}
		existing = nil
	}

	if existing == nil {
		if r.ID == "" {
			r.ID = newID("res")
		}
		r.Version = 1
		err := q.QueryRow(ctx, `
			INSERT INTO reservations (
				id, tenant_id, property_id, feed_id, external_event_id, source,
				guest_summary, status, start_at, end_at, all_day, timezone, sequence,
				version, created_at, updated_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,NOW(),NOW())
			RETURNING created_at, updated_at
		`,
			r.ID, r.TenantID, r.PropertyID, r.FeedID, r.ExternalEventID, r.Source,
			nullString(r.GuestSummary), r.Status, r.StartAt, r.EndAt, r.AllDay,
			nullString(r.Timezone), r.Sequence, r.Version,
		).Scan(&r.CreatedAt, &r.UpdatedAt)
		if err != nil {
			return false, false, fmt.Errorf("insert reservation: %w", err)
		}
		return true, true, nil
	}

	changed = reservationContentChanged(existing, r)
	if !changed {
		r.ID = existing.ID
		r.Version = existing.Version
		r.CreatedAt = existing.CreatedAt
		r.UpdatedAt = existing.UpdatedAt
		return false, false, nil
	}

	r.ID = existing.ID
	_, err = q.Exec(ctx, `
		UPDATE reservations
		SET guest_summary = $3, status = $4, start_at = $5, end_at = $6,
		    all_day = $7, timezone = $8, sequence = $9, updated_at = $10,
		    version = version + 1
		WHERE id = $1 AND tenant_id = $2
	`,
		r.ID, r.TenantID, nullString(r.GuestSummary), r.Status, r.StartAt, r.EndAt,
		r.AllDay, nullString(r.Timezone), r.Sequence, now,
	)
	if err != nil {
		return false, false, fmt.Errorf("update reservation: %w", err)
	}
	r.Version = existing.Version + 1
	r.CreatedAt = existing.CreatedAt
	r.UpdatedAt = now
	return false, true, nil
}

func reservationContentChanged(existing, next *Reservation) bool {
	return existing.GuestSummary != next.GuestSummary ||
		existing.Status != next.Status ||
		!existing.StartAt.Equal(next.StartAt) ||
		!existing.EndAt.Equal(next.EndAt) ||
		existing.AllDay != next.AllDay ||
		existing.Timezone != next.Timezone ||
		existing.Sequence != next.Sequence
}

// SetReservationCancelled flips an active reservation to cancelled when its
// source event stops being active. The record and its content are preserved.
// It returns nil when the reservation is already cancelled or absent.
func (s *CalendarStore) SetReservationCancelled(ctx context.Context, q querier, feedID, externalEventID string, now time.Time) (*Reservation, error) {
	var r Reservation
	var summary, timezone *string
	err := q.QueryRow(ctx, `
		UPDATE reservations
		SET status = 'cancelled', updated_at = $3, version = version + 1
		WHERE feed_id = $1 AND external_event_id = $2 AND status = 'active'
		RETURNING `+reservationColumns+`
	`, feedID, externalEventID, now).Scan(
		&r.ID, &r.TenantID, &r.PropertyID, &r.FeedID, &r.ExternalEventID, &r.Source,
		&summary, &r.Status, &r.StartAt, &r.EndAt, &r.AllDay, &timezone, &r.Sequence,
		&r.Version, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("cancel reservation: %w", err)
	}
	if summary != nil {
		r.GuestSummary = *summary
	}
	if timezone != nil {
		r.Timezone = *timezone
	}
	return &r, nil
}

func (s *CalendarStore) getReservationBySource(ctx context.Context, q querier, feedID, externalEventID string) (*Reservation, error) {
	row := q.QueryRow(ctx,
		`SELECT `+reservationColumns+` FROM reservations
		 WHERE feed_id = $1 AND external_event_id = $2`,
		feedID, externalEventID,
	)
	return scanReservation(row)
}

func (s *CalendarStore) GetReservation(ctx context.Context, tenantID, reservationID string) (*Reservation, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+reservationColumns+` FROM reservations
		 WHERE id = $1 AND tenant_id = $2`,
		reservationID, tenantID,
	)
	return scanReservation(row)
}

func (s *CalendarStore) ListAllReservations(ctx context.Context, q querier, tenantID, propertyID string) ([]*Reservation, error) {
	rows, err := q.Query(ctx,
		`SELECT `+reservationColumns+` FROM reservations
		 WHERE tenant_id = $1 AND property_id = $2 ORDER BY start_at ASC`,
		tenantID, propertyID,
	)
	if err != nil {
		return nil, fmt.Errorf("list reservations: %w", err)
	}
	defer rows.Close()

	var reservations []*Reservation
	for rows.Next() {
		r, err := scanReservation(rows)
		if err != nil {
			return nil, err
		}
		reservations = append(reservations, r)
	}
	return reservations, rows.Err()
}

func (s *CalendarStore) ListReservationsByProperty(ctx context.Context, tenantID, propertyID string) ([]Reservation, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+reservationColumns+` FROM reservations
		 WHERE tenant_id = $1 AND property_id = $2 ORDER BY start_at ASC`,
		tenantID, propertyID,
	)
	if err != nil {
		return nil, fmt.Errorf("list reservations by property: %w", err)
	}
	defer rows.Close()

	var reservations []Reservation
	for rows.Next() {
		r, err := scanReservation(rows)
		if err != nil {
			return nil, err
		}
		reservations = append(reservations, *r)
	}
	return reservations, rows.Err()
}

// ReservationIDsForEventIDs maps internal external-calendar-event ids to their
// normalized reservation ids for the same tenant.
func (s *CalendarStore) ReservationIDsForEventIDs(ctx context.Context, q querier, tenantID string, eventIDs []string) (map[string]string, error) {
	if len(eventIDs) == 0 {
		return map[string]string{}, nil
	}
	rows, err := q.Query(ctx, `
		SELECT e.id, r.id FROM reservations r
		JOIN external_calendar_events e
		  ON e.feed_id = r.feed_id AND e.external_event_id = r.external_event_id
		WHERE e.tenant_id = $1 AND e.id = ANY($2::text[])
	`, tenantID, eventIDs)
	if err != nil {
		return nil, fmt.Errorf("resolve reservation ids: %w", err)
	}
	defer rows.Close()

	mapping := make(map[string]string)
	for rows.Next() {
		var eventID, reservationID string
		if err := rows.Scan(&eventID, &reservationID); err != nil {
			return nil, fmt.Errorf("scan reservation id mapping: %w", err)
		}
		mapping[eventID] = reservationID
	}
	return mapping, rows.Err()
}

// GetOpenExceptionByKindDedupe finds the open calendar exception that a
// reservation conflict should be linked to.
func (s *CalendarStore) GetOpenExceptionByKindDedupe(ctx context.Context, q querier, tenantID, kind, dedupeKey string) (*CalendarException, error) {
	row := q.QueryRow(ctx,
		`SELECT `+exceptionColumns+` FROM calendar_exceptions
		 WHERE tenant_id = $1 AND kind = $2 AND dedupe_key = $3 AND status = 'open'
		 ORDER BY created_at ASC LIMIT 1`,
		tenantID, kind, dedupeKey,
	)
	return scanException(row)
}

const conflictColumns = `id, tenant_id, property_id, kind, severity, status,
	message, reservation_ids, exception_id, dedupe_key, metadata, created_at, resolved_at`

func scanConflict(row pgx.Row) (*ReservationConflict, error) {
	var c ReservationConflict
	var meta []byte
	var exceptionID, dedupeKey *string
	err := row.Scan(
		&c.ID, &c.TenantID, &c.PropertyID, &c.Kind, &c.Severity, &c.Status,
		&c.Message, &c.ReservationIDs, &exceptionID, &dedupeKey, &meta,
		&c.CreatedAt, &c.ResolvedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrConflictNotFound
		}
		return nil, fmt.Errorf("scan reservation conflict: %w", err)
	}
	if exceptionID != nil {
		c.ExceptionID = *exceptionID
	}
	if dedupeKey != nil {
		c.DedupeKey = *dedupeKey
	}
	if len(meta) > 0 {
		_ = json.Unmarshal(meta, &c.Metadata)
	}
	return &c, nil
}

// InsertConflict records a human-reviewable conflict. Open conflicts are
// deduplicated by (tenant, property, kind, dedupe key).
func (s *CalendarStore) InsertConflict(ctx context.Context, q querier, c *ReservationConflict) (bool, error) {
	if c.ID == "" {
		c.ID = newID("conf")
	}
	if c.Status == "" {
		c.Status = ConflictStatusOpen
	}
	if c.ReservationIDs == nil {
		c.ReservationIDs = []string{}
	}
	meta, err := json.Marshal(c.Metadata)
	if err != nil {
		return false, fmt.Errorf("marshal conflict metadata: %w", err)
	}
	if len(meta) == 0 || string(meta) == "null" {
		meta = []byte("{}")
	}

	tag, err := q.Exec(ctx, `
		INSERT INTO reservation_conflicts (
			id, tenant_id, property_id, kind, severity, status, message,
			reservation_ids, exception_id, dedupe_key, metadata, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (tenant_id, property_id, kind, dedupe_key)
		WHERE status = 'open' DO NOTHING
	`,
		c.ID, c.TenantID, c.PropertyID, c.Kind, c.Severity, c.Status, c.Message,
		c.ReservationIDs, nullString(c.ExceptionID), c.DedupeKey, meta, c.CreatedAt,
	)
	if err != nil {
		return false, fmt.Errorf("insert reservation conflict: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func (s *CalendarStore) GetConflict(ctx context.Context, tenantID, conflictID string) (*ReservationConflict, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+conflictColumns+` FROM reservation_conflicts
		 WHERE id = $1 AND tenant_id = $2`,
		conflictID, tenantID,
	)
	return scanConflict(row)
}

func (s *CalendarStore) ListConflicts(ctx context.Context, tenantID, propertyID string) ([]ReservationConflict, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+conflictColumns+` FROM reservation_conflicts
		 WHERE tenant_id = $1 AND property_id = $2 ORDER BY created_at DESC`,
		tenantID, propertyID,
	)
	if err != nil {
		return nil, fmt.Errorf("list reservation conflicts: %w", err)
	}
	defer rows.Close()

	var conflicts []ReservationConflict
	for rows.Next() {
		c, err := scanConflict(rows)
		if err != nil {
			return nil, err
		}
		conflicts = append(conflicts, *c)
	}
	return conflicts, rows.Err()
}

// MarkConflictResolved transitions an open conflict to resolved inside a
// transaction. It returns the conflict only when an open row was actually
// closed, and reports ErrConflictAlreadyResolved otherwise.
func (s *CalendarStore) MarkConflictResolved(ctx context.Context, q querier, tenantID, conflictID string, at time.Time) (*ReservationConflict, error) {
	row := q.QueryRow(ctx, `
		UPDATE reservation_conflicts
		SET status = 'resolved', resolved_at = $3
		WHERE id = $1 AND tenant_id = $2 AND status = 'open'
		RETURNING `+conflictColumns,
		conflictID, tenantID, at,
	)
	resolved, err := scanConflict(row)
	if err != nil {
		if errors.Is(err, ErrConflictNotFound) {
			return nil, ErrConflictAlreadyResolved
		}
		return nil, err
	}
	return resolved, nil
}

// ResolveLinkedException closes the calendar exception a conflict is linked
// to so both review surfaces converge.
func (s *CalendarStore) ResolveLinkedException(ctx context.Context, q querier, tenantID, exceptionID string, at time.Time) error {
	if exceptionID == "" {
		return nil
	}
	_, err := q.Exec(ctx, `
		UPDATE calendar_exceptions
		SET status = 'resolved', resolved_at = $3
		WHERE id = $1 AND tenant_id = $2 AND status = 'open'
	`, exceptionID, tenantID, at)
	if err != nil {
		return fmt.Errorf("resolve linked calendar exception: %w", err)
	}
	return nil
}

func (s *CalendarStore) InsertConflictResolution(ctx context.Context, q querier, r *ConflictResolution) error {
	if r.ID == "" {
		r.ID = newID("crec")
	}
	_, err := q.Exec(ctx, `
		INSERT INTO reservation_conflict_resolutions (
			id, tenant_id, conflict_id, actor_id, actor_type, outcome, note, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`,
		r.ID, r.TenantID, r.ConflictID, r.ActorID, r.ActorType, r.Outcome,
		nullString(r.Note), r.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert conflict resolution: %w", err)
	}
	return nil
}

const proposalColumns = `id, tenant_id, property_id, reservation_id, kind, status,
	scheduled_at, checklist_hint, version, created_at, updated_at`

func scanProposal(row pgx.Row) (*TurnoverProposal, error) {
	var p TurnoverProposal
	err := row.Scan(
		&p.ID, &p.TenantID, &p.PropertyID, &p.ReservationID, &p.Kind, &p.Status,
		&p.ScheduledAt, &p.ChecklistHint, &p.Version, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrReservationNotFound
		}
		return nil, fmt.Errorf("scan turnover proposal: %w", err)
	}
	return &p, nil
}

// UpsertProposal synchronizes one deterministic proposal for a reservation.
// A cancelled proposal is reactivated when the reservation is active again;
// a changed schedule is applied in place. Proposals are never hard-deleted.
func (s *CalendarStore) UpsertProposal(ctx context.Context, q querier, p *TurnoverProposal, now time.Time) (created, changed bool, err error) {
	existing, err := s.getProposalByReservationKind(ctx, q, p.ReservationID, p.Kind)
	if err != nil {
		if !errors.Is(err, ErrReservationNotFound) {
			return false, false, err
		}
		existing = nil
	}

	if existing == nil {
		if p.ID == "" {
			p.ID = newID("prop")
		}
		p.Version = 1
		err := q.QueryRow(ctx, `
			INSERT INTO turnover_proposals (
				id, tenant_id, property_id, reservation_id, kind, status,
				scheduled_at, checklist_hint, version, created_at, updated_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NOW(),NOW())
			RETURNING created_at, updated_at
		`,
			p.ID, p.TenantID, p.PropertyID, p.ReservationID, p.Kind, p.Status,
			p.ScheduledAt, p.ChecklistHint, p.Version,
		).Scan(&p.CreatedAt, &p.UpdatedAt)
		if err != nil {
			return false, false, fmt.Errorf("insert turnover proposal: %w", err)
		}
		return true, true, nil
	}

	changed = existing.Status != p.Status || !existing.ScheduledAt.Equal(p.ScheduledAt)
	if !changed {
		p.ID = existing.ID
		p.Version = existing.Version
		p.CreatedAt = existing.CreatedAt
		p.UpdatedAt = existing.UpdatedAt
		return false, false, nil
	}

	p.ID = existing.ID
	_, err = q.Exec(ctx, `
		UPDATE turnover_proposals
		SET status = $3, scheduled_at = $4, checklist_hint = $5,
		    updated_at = $6, version = version + 1
		WHERE id = $1 AND tenant_id = $2
	`,
		p.ID, p.TenantID, p.Status, p.ScheduledAt, p.ChecklistHint, now,
	)
	if err != nil {
		return false, false, fmt.Errorf("update turnover proposal: %w", err)
	}
	p.Version = existing.Version + 1
	p.CreatedAt = existing.CreatedAt
	p.UpdatedAt = now
	return false, true, nil
}

func (s *CalendarStore) getProposalByReservationKind(ctx context.Context, q querier, reservationID, kind string) (*TurnoverProposal, error) {
	row := q.QueryRow(ctx,
		`SELECT `+proposalColumns+` FROM turnover_proposals
		 WHERE reservation_id = $1 AND kind = $2`,
		reservationID, kind,
	)
	return scanProposal(row)
}

// CancelProposalsForReservation marks every open proposal of a cancelled
// reservation cancelled. The rows are preserved, never deleted.
func (s *CalendarStore) CancelProposalsForReservation(ctx context.Context, q querier, reservationID string, now time.Time) (int, error) {
	tag, err := q.Exec(ctx, `
		UPDATE turnover_proposals
		SET status = 'cancelled', updated_at = $2, version = version + 1
		WHERE reservation_id = $1 AND status = 'proposed'
	`, reservationID, now)
	if err != nil {
		return 0, fmt.Errorf("cancel proposals for reservation: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

func (s *CalendarStore) ListProposalsByProperty(ctx context.Context, tenantID, propertyID string) ([]TurnoverProposal, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+proposalColumns+` FROM turnover_proposals
		 WHERE tenant_id = $1 AND property_id = $2 ORDER BY scheduled_at ASC`,
		tenantID, propertyID,
	)
	if err != nil {
		return nil, fmt.Errorf("list turnover proposals: %w", err)
	}
	defer rows.Close()

	var proposals []TurnoverProposal
	for rows.Next() {
		p, err := scanProposal(rows)
		if err != nil {
			return nil, err
		}
		proposals = append(proposals, *p)
	}
	return proposals, rows.Err()
}
