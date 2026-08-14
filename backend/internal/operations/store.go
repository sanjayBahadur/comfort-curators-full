package operations

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

type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type TicketStore struct {
	pool *pgxpool.Pool
}

func NewTicketStore(pool *pgxpool.Pool) *TicketStore {
	return &TicketStore{pool: pool}
}

func (s *TicketStore) InsertTicket(ctx context.Context, q querier, t *Ticket) error {
	return insertTicket(ctx, q, t)
}

func insertTicket(ctx context.Context, q querier, t *Ticket) error {
	if t.ID == "" {
		t.ID = newID("tkt")
	}
	now := time.Now().UTC()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	if t.UpdatedAt.IsZero() {
		t.UpdatedAt = now
	}
	if t.Version == 0 {
		t.Version = 1
	}
	if t.Status == "" {
		t.Status = StateDraft
	}

	var blockerJSON []byte
	if t.Blocker != nil {
		var err error
		blockerJSON, err = json.Marshal(t.Blocker)
		if err != nil {
			return fmt.Errorf("marshal blocker: %w", err)
		}
	}

	windowJSON := []byte(t.RequestedWindow)
	if len(windowJSON) == 0 {
		windowJSON = []byte("{}")
	}

	_, err := q.Exec(ctx, `INSERT INTO tickets (
		id, tenant_id, property_id, type, status, reason,
		requested_window, checklist_version_id, created_by,
		assigned_to, verified_by, verifier_note,
		blocker, follow_up_ticket_id, reopen_reason,
		notification_intent, severity, version, created_at, updated_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)`,
		t.ID, t.TenantID, t.PropertyID, t.Type, t.Status, t.Reason,
		windowJSON, nullString(t.ChecklistVersionID), t.CreatedBy,
		nullString(t.AssignedTo), nullString(t.VerifiedBy), nullString(t.VerifierNote),
		blockerJSON, nullString(t.FollowUpTicketID), nullString(t.ReopenReason),
		nullString(t.NotificationIntent), nullString(t.Severity), t.Version, t.CreatedAt, t.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert ticket: %w", err)
	}
	return nil
}

func (s *TicketStore) GetTicket(ctx context.Context, tenantID, ticketID string) (*Ticket, error) {
	return getTicket(ctx, s.pool, tenantID, ticketID)
}

func getTicket(ctx context.Context, q querier, tenantID, ticketID string) (*Ticket, error) {
	row := q.QueryRow(ctx, `SELECT
		id, tenant_id, property_id, type, status, reason,
		requested_window, checklist_version_id, created_by,
		assigned_to, verified_by, verifier_note,
		blocker, follow_up_ticket_id, reopen_reason,
		notification_intent, severity, version, created_at, updated_at
	FROM tickets WHERE tenant_id=$1 AND id=$2`, tenantID, ticketID)
	return scanTicket(row)
}

func (s *TicketStore) GetTicketForUpdate(ctx context.Context, tx pgx.Tx, tenantID, ticketID string) (*Ticket, error) {
	row := tx.QueryRow(ctx, `SELECT
		id, tenant_id, property_id, type, status, reason,
		requested_window, checklist_version_id, created_by,
		assigned_to, verified_by, verifier_note,
		blocker, follow_up_ticket_id, reopen_reason,
		notification_intent, severity, version, created_at, updated_at
	FROM tickets WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenantID, ticketID)
	return scanTicket(row)
}

func (s *TicketStore) ListTickets(ctx context.Context, tenantID, propertyID string, status string, cursor string, limit int) ([]Ticket, string, error) {
	return listTickets(ctx, s.pool, tenantID, propertyID, status, cursor, limit)
}

func listTickets(ctx context.Context, q querier, tenantID, propertyID, status string, cursor string, limit int) ([]Ticket, string, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	var rows pgx.Rows
	var err error

	if status != "" && cursor != "" {
		rows, err = q.Query(ctx, `SELECT
			id, tenant_id, property_id, type, status, reason,
			requested_window, checklist_version_id, created_by,
			assigned_to, verified_by, verifier_note,
			blocker, follow_up_ticket_id, reopen_reason,
			notification_intent, severity, version, created_at, updated_at
		FROM tickets
		WHERE tenant_id=$1 AND property_id=$2 AND status=$3 AND id > $4
		ORDER BY id ASC LIMIT $5`,
			tenantID, propertyID, status, cursor, limit+1)
	} else if status != "" {
		rows, err = q.Query(ctx, `SELECT
			id, tenant_id, property_id, type, status, reason,
			requested_window, checklist_version_id, created_by,
			assigned_to, verified_by, verifier_note,
			blocker, follow_up_ticket_id, reopen_reason,
			notification_intent, severity, version, created_at, updated_at
		FROM tickets
		WHERE tenant_id=$1 AND property_id=$2 AND status=$3
		ORDER BY id ASC LIMIT $4`,
			tenantID, propertyID, status, limit+1)
	} else if cursor != "" {
		rows, err = q.Query(ctx, `SELECT
			id, tenant_id, property_id, type, status, reason,
			requested_window, checklist_version_id, created_by,
			assigned_to, verified_by, verifier_note,
			blocker, follow_up_ticket_id, reopen_reason,
			notification_intent, severity, version, created_at, updated_at
		FROM tickets
		WHERE tenant_id=$1 AND property_id=$2 AND id > $3
		ORDER BY id ASC LIMIT $4`,
			tenantID, propertyID, cursor, limit+1)
	} else {
		rows, err = q.Query(ctx, `SELECT
			id, tenant_id, property_id, type, status, reason,
			requested_window, checklist_version_id, created_by,
			assigned_to, verified_by, verifier_note,
			blocker, follow_up_ticket_id, reopen_reason,
			notification_intent, severity, version, created_at, updated_at
		FROM tickets
		WHERE tenant_id=$1 AND property_id=$2
		ORDER BY id ASC LIMIT $3`,
			tenantID, propertyID, limit+1)
	}
	if err != nil {
		return nil, "", fmt.Errorf("list tickets: %w", err)
	}
	defer rows.Close()

	var tickets []Ticket
	for rows.Next() {
		t, err := scanTicketFromRow(rows)
		if err != nil {
			return nil, "", err
		}
		tickets = append(tickets, *t)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	var nextCursor string
	if len(tickets) > limit {
		nextCursor = tickets[limit-1].ID
		tickets = tickets[:limit]
	}

	return tickets, nextCursor, nil
}

func (s *TicketStore) UpdateTicketStatus(ctx context.Context, tx pgx.Tx, t *Ticket) error {
	now := time.Now().UTC()
	t.UpdatedAt = now

	var blockerJSON []byte
	if t.Blocker != nil {
		var err error
		blockerJSON, err = json.Marshal(t.Blocker)
		if err != nil {
			return fmt.Errorf("marshal blocker: %w", err)
		}
	}

	tag, err := tx.Exec(ctx, `UPDATE tickets SET
		status=$1, version=$2, assigned_to=$3, verified_by=$4, verifier_note=$5,
		blocker=$6, follow_up_ticket_id=$7, reopen_reason=$8,
		notification_intent=$9, severity=$10, updated_at=$11
	WHERE id=$12 AND tenant_id=$13 AND version=$14`,
		t.Status, t.Version, nullString(t.AssignedTo), nullString(t.VerifiedBy),
		nullString(t.VerifierNote), blockerJSON, nullString(t.FollowUpTicketID),
		nullString(t.ReopenReason), nullString(t.NotificationIntent), nullString(t.Severity), t.UpdatedAt,
		t.ID, t.TenantID, t.Version-1,
	)
	if err != nil {
		return fmt.Errorf("update ticket status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errors.New("ticket state update lost a concurrent write (optimistic version)")
	}
	return nil
}

func (s *TicketStore) SetTicketBlocker(ctx context.Context, tx pgx.Tx, t *Ticket, block *TicketBlock) error {
	blockerJSON, err := json.Marshal(block)
	if err != nil {
		return fmt.Errorf("marshal blocker: %w", err)
	}
	t.Blocker = block
	t.Status = StateBlocked
	t.Version++
	t.UpdatedAt = time.Now().UTC()

	tag, err := tx.Exec(ctx, `UPDATE tickets SET
		status=$1, version=$2, blocker=$3, updated_at=$4
	WHERE id=$5 AND tenant_id=$6 AND version=$7`,
		t.Status, t.Version, blockerJSON, t.UpdatedAt,
		t.ID, t.TenantID, t.Version-1,
	)
	if err != nil {
		return fmt.Errorf("set ticket blocker: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errors.New("ticket blocker update lost a concurrent write (optimistic version)")
	}
	return nil
}

func (s *TicketStore) ClearTicketBlocker(ctx context.Context, tx pgx.Tx, t *Ticket, targetState string) error {
	t.Blocker = nil
	t.Status = targetState
	t.Version++
	t.UpdatedAt = time.Now().UTC()

	tag, err := tx.Exec(ctx, `UPDATE tickets SET
		status=$1, version=$2, blocker=NULL, updated_at=$3
	WHERE id=$4 AND tenant_id=$5 AND version=$6`,
		t.Status, t.Version, t.UpdatedAt,
		t.ID, t.TenantID, t.Version-1,
	)
	if err != nil {
		return fmt.Errorf("clear ticket blocker: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errors.New("ticket blocker clear lost a concurrent write (optimistic version)")
	}
	return nil
}

func (s *TicketStore) InsertStateEvent(ctx context.Context, q querier, e *TicketStateEvent) error {
	if e.ID == "" {
		e.ID = newID("tse")
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}

	evJSON, err := json.Marshal(e.Evidence)
	if err != nil {
		return fmt.Errorf("marshal evidence: %w", err)
	}

	_, err = q.Exec(ctx, `INSERT INTO ticket_state_events (
		id, ticket_id, tenant_id, from_state, to_state,
		actor_id, reason, evidence, version, created_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		e.ID, e.TicketID, e.TenantID, e.FromState, e.ToState,
		e.ActorID, e.Reason, evJSON, e.Version, e.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert state event: %w", err)
	}
	return nil
}

func (s *TicketStore) ListStateEvents(ctx context.Context, tenantID, ticketID string) ([]TicketStateEvent, error) {
	rows, err := s.pool.Query(ctx, `SELECT
		id, ticket_id, tenant_id, from_state, to_state,
		actor_id, reason, evidence, version, created_at
	FROM ticket_state_events
	WHERE tenant_id=$1 AND ticket_id=$2
	ORDER BY created_at ASC`, tenantID, ticketID)
	if err != nil {
		return nil, fmt.Errorf("list state events: %w", err)
	}
	defer rows.Close()

	var events []TicketStateEvent
	for rows.Next() {
		var e TicketStateEvent
		var evBytes []byte
		if err := rows.Scan(
			&e.ID, &e.TicketID, &e.TenantID, &e.FromState, &e.ToState,
			&e.ActorID, &e.Reason, &evBytes, &e.Version, &e.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan state event: %w", err)
		}
		if len(evBytes) > 0 {
			json.Unmarshal(evBytes, &e.Evidence)
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func (s *TicketStore) InsertChecklistItem(ctx context.Context, q querier, item *TicketChecklistItem) error {
	if item.ID == "" {
		item.ID = newID("tci")
	}
	now := time.Now().UTC()
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	}
	if item.UpdatedAt.IsZero() {
		item.UpdatedAt = now
	}
	if item.Version == 0 {
		item.Version = 1
	}

	evJSON, err := json.Marshal(item.EvidenceIDs)
	if err != nil {
		return fmt.Errorf("marshal evidence ids: %w", err)
	}

	_, err = q.Exec(ctx, `INSERT INTO ticket_checklist_items (
		id, ticket_id, tenant_id, template_item_index, label,
		status, completed_by, completed_at, evidence_ids, evidence_required, notes,
		version, created_at, updated_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		item.ID, item.TicketID, item.TenantID, item.TemplateItemIndex,
		item.Label, item.Status, nullString(item.CompletedBy),
		item.CompletedAt, evJSON, item.EvidenceRequired, nullString(item.Notes),
		item.Version, item.CreatedAt, item.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert checklist item: %w", err)
	}
	return nil
}

func (s *TicketStore) UpdateChecklistItem(ctx context.Context, q querier, item *TicketChecklistItem) error {
	item.Version++
	item.UpdatedAt = time.Now().UTC()

	evJSON, err := json.Marshal(item.EvidenceIDs)
	if err != nil {
		return fmt.Errorf("marshal evidence ids: %w", err)
	}

	_, err = q.Exec(ctx, `UPDATE ticket_checklist_items SET
		status=$1, completed_by=$2, completed_at=$3, evidence_ids=$4,
		evidence_required=$5, notes=$6, version=$7, updated_at=$8
	WHERE id=$9 AND tenant_id=$10 AND version=$11`,
		item.Status, nullString(item.CompletedBy), item.CompletedAt,
		evJSON, item.EvidenceRequired, nullString(item.Notes), item.Version, item.UpdatedAt,
		item.ID, item.TenantID, item.Version-1,
	)
	if err != nil {
		return fmt.Errorf("update checklist item: %w", err)
	}
	return nil
}

func (s *TicketStore) ListChecklistItems(ctx context.Context, tenantID, ticketID string) ([]TicketChecklistItem, error) {
	return listChecklistItems(ctx, s.pool, tenantID, ticketID)
}

func listChecklistItems(ctx context.Context, q querier, tenantID, ticketID string) ([]TicketChecklistItem, error) {
	rows, err := q.Query(ctx, `SELECT
		id, ticket_id, tenant_id, template_item_index, label,
		status, completed_by, completed_at, evidence_ids, evidence_required, notes,
		version, created_at, updated_at
	FROM ticket_checklist_items
	WHERE tenant_id=$1 AND ticket_id=$2
	ORDER BY template_item_index ASC`, tenantID, ticketID)
	if err != nil {
		return nil, fmt.Errorf("list checklist items: %w", err)
	}
	defer rows.Close()

	var items []TicketChecklistItem
	for rows.Next() {
		var item TicketChecklistItem
		var evBytes []byte
		var completedBy, notes *string
		if err := rows.Scan(
			&item.ID, &item.TicketID, &item.TenantID, &item.TemplateItemIndex,
			&item.Label, &item.Status, &completedBy, &item.CompletedAt,
			&evBytes, &item.EvidenceRequired, &notes, &item.Version, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan checklist item: %w", err)
		}
		if completedBy != nil {
			item.CompletedBy = *completedBy
		}
		if notes != nil {
			item.Notes = *notes
		}
		if len(evBytes) > 0 {
			json.Unmarshal(evBytes, &item.EvidenceIDs)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *TicketStore) GetChecklistItem(ctx context.Context, tenantID, itemID string) (*TicketChecklistItem, error) {
	return getChecklistItem(ctx, s.pool, tenantID, itemID)
}

func getChecklistItem(ctx context.Context, q querier, tenantID, itemID string) (*TicketChecklistItem, error) {
	row := q.QueryRow(ctx, `SELECT
		id, ticket_id, tenant_id, template_item_index, label,
		status, completed_by, completed_at, evidence_ids, evidence_required, notes,
		version, created_at, updated_at
	FROM ticket_checklist_items
	WHERE tenant_id=$1 AND id=$2`, tenantID, itemID)

	var item TicketChecklistItem
	var evBytes []byte
	var completedBy, notes *string
	if err := row.Scan(
		&item.ID, &item.TicketID, &item.TenantID, &item.TemplateItemIndex,
		&item.Label, &item.Status, &completedBy, &item.CompletedAt,
		&evBytes, &item.EvidenceRequired, &notes, &item.Version, &item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrChecklistItemNotFound
		}
		return nil, fmt.Errorf("scan checklist item: %w", err)
	}
	if completedBy != nil {
		item.CompletedBy = *completedBy
	}
	if notes != nil {
		item.Notes = *notes
	}
	if len(evBytes) > 0 {
		json.Unmarshal(evBytes, &item.EvidenceIDs)
	}
	return &item, nil
}

func scanTicket(row pgx.Row) (*Ticket, error) {
	var t Ticket
	var windowBytes, blockerBytes []byte
	var checklistVersionID, assignedTo, verifiedBy, verifierNote *string
	var followUpTicketID, reopenReason, notificationIntent, severity *string

	err := row.Scan(
		&t.ID, &t.TenantID, &t.PropertyID, &t.Type, &t.Status, &t.Reason,
		&windowBytes, &checklistVersionID, &t.CreatedBy,
		&assignedTo, &verifiedBy, &verifierNote,
		&blockerBytes, &followUpTicketID, &reopenReason,
		&notificationIntent, &severity, &t.Version, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTicketNotFound
		}
		return nil, fmt.Errorf("scan ticket: %w", err)
	}

	if windowBytes != nil {
		t.RequestedWindow = windowBytes
	}
	if checklistVersionID != nil {
		t.ChecklistVersionID = *checklistVersionID
	}
	if assignedTo != nil {
		t.AssignedTo = *assignedTo
	}
	if verifiedBy != nil {
		t.VerifiedBy = *verifiedBy
	}
	if verifierNote != nil {
		t.VerifierNote = *verifierNote
	}
	if len(blockerBytes) > 0 && string(blockerBytes) != "null" {
		var block TicketBlock
		if err := json.Unmarshal(blockerBytes, &block); err == nil {
			t.Blocker = &block
		}
	}
	if followUpTicketID != nil {
		t.FollowUpTicketID = *followUpTicketID
	}
	if reopenReason != nil {
		t.ReopenReason = *reopenReason
	}
	if notificationIntent != nil {
		t.NotificationIntent = *notificationIntent
	}
	if severity != nil {
		t.Severity = *severity
	}

	return &t, nil
}

func scanTicketFromRow(row pgx.Rows) (*Ticket, error) {
	var t Ticket
	var windowBytes, blockerBytes []byte
	var checklistVersionID, assignedTo, verifiedBy, verifierNote *string
	var followUpTicketID, reopenReason, notificationIntent, severity *string

	err := row.Scan(
		&t.ID, &t.TenantID, &t.PropertyID, &t.Type, &t.Status, &t.Reason,
		&windowBytes, &checklistVersionID, &t.CreatedBy,
		&assignedTo, &verifiedBy, &verifierNote,
		&blockerBytes, &followUpTicketID, &reopenReason,
		&notificationIntent, &severity, &t.Version, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan ticket row: %w", err)
	}

	if windowBytes != nil {
		t.RequestedWindow = windowBytes
	}
	if checklistVersionID != nil {
		t.ChecklistVersionID = *checklistVersionID
	}
	if assignedTo != nil {
		t.AssignedTo = *assignedTo
	}
	if verifiedBy != nil {
		t.VerifiedBy = *verifiedBy
	}
	if verifierNote != nil {
		t.VerifierNote = *verifierNote
	}
	if len(blockerBytes) > 0 && string(blockerBytes) != "null" {
		var block TicketBlock
		if err := json.Unmarshal(blockerBytes, &block); err == nil {
			t.Blocker = &block
		}
	}
	if followUpTicketID != nil {
		t.FollowUpTicketID = *followUpTicketID
	}
	if reopenReason != nil {
		t.ReopenReason = *reopenReason
	}
	if notificationIntent != nil {
		t.NotificationIntent = *notificationIntent
	}
	if severity != nil {
		t.Severity = *severity
	}

	return &t, nil
}

func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (s *TicketStore) InsertEvidence(ctx context.Context, q querier, e *EvidenceRecord) error {
	if e.ID == "" {
		e.ID = newID("ev")
	}
	now := time.Now().UTC()
	if e.CreatedAt.IsZero() {
		e.CreatedAt = now
	}
	if e.CapturedAt.IsZero() {
		e.CapturedAt = now
	}
	if e.Version == 0 {
		e.Version = 1
	}

	_, err := q.Exec(ctx, `INSERT INTO ticket_evidence (
		id, tenant_id, ticket_id, checklist_item_id, object_id,
		content_hash, file_name, content_type, size_bytes, status,
		captured_by, captured_at, version, created_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		e.ID, e.TenantID, e.TicketID, nullString(e.ChecklistItemID), nullString(e.ObjectID),
		e.ContentHash, nullString(e.FileName), nullString(e.ContentType), e.SizeBytes, e.Status,
		e.CapturedBy, e.CapturedAt, e.Version, e.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert evidence: %w", err)
	}
	return nil
}

func (s *TicketStore) GetEvidence(ctx context.Context, tenantID, evidenceID string) (*EvidenceRecord, error) {
	row := s.pool.QueryRow(ctx, `SELECT
		id, tenant_id, ticket_id, checklist_item_id, object_id,
		content_hash, file_name, content_type, size_bytes, status,
		captured_by, captured_at, version, created_at
	FROM ticket_evidence
	WHERE tenant_id=$1 AND id=$2`, tenantID, evidenceID)

	var e EvidenceRecord
	var checklistItemID, objectID, fileName, contentType *string
	if err := row.Scan(
		&e.ID, &e.TenantID, &e.TicketID, &checklistItemID, &objectID,
		&e.ContentHash, &fileName, &contentType, &e.SizeBytes, &e.Status,
		&e.CapturedBy, &e.CapturedAt, &e.Version, &e.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrEvidenceNotFound
		}
		return nil, fmt.Errorf("scan evidence: %w", err)
	}
	if checklistItemID != nil {
		e.ChecklistItemID = *checklistItemID
	}
	if objectID != nil {
		e.ObjectID = *objectID
	}
	if fileName != nil {
		e.FileName = *fileName
	}
	if contentType != nil {
		e.ContentType = *contentType
	}
	return &e, nil
}

func (s *TicketStore) FindEvidenceByHash(ctx context.Context, q querier, tenantID, ticketID, contentHash string) (*EvidenceRecord, error) {
	row := q.QueryRow(ctx, `SELECT
		id, tenant_id, ticket_id, checklist_item_id, object_id,
		content_hash, file_name, content_type, size_bytes, status,
		captured_by, captured_at, version, created_at
	FROM ticket_evidence
	WHERE tenant_id=$1 AND ticket_id=$2 AND content_hash=$3
	LIMIT 1`, tenantID, ticketID, contentHash)

	var e EvidenceRecord
	var checklistItemID, objectID, fileName, contentType *string
	if err := row.Scan(
		&e.ID, &e.TenantID, &e.TicketID, &checklistItemID, &objectID,
		&e.ContentHash, &fileName, &contentType, &e.SizeBytes, &e.Status,
		&e.CapturedBy, &e.CapturedAt, &e.Version, &e.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrEvidenceNotFound
		}
		return nil, fmt.Errorf("scan evidence: %w", err)
	}
	if checklistItemID != nil {
		e.ChecklistItemID = *checklistItemID
	}
	if objectID != nil {
		e.ObjectID = *objectID
	}
	if fileName != nil {
		e.FileName = *fileName
	}
	if contentType != nil {
		e.ContentType = *contentType
	}
	return &e, nil
}

func (s *TicketStore) ListEvidence(ctx context.Context, tenantID, ticketID string) ([]EvidenceRecord, error) {
	return listEvidence(ctx, s.pool, tenantID, ticketID)
}

func listEvidence(ctx context.Context, q querier, tenantID, ticketID string) ([]EvidenceRecord, error) {
	rows, err := q.Query(ctx, `SELECT
		id, tenant_id, ticket_id, checklist_item_id, object_id,
		content_hash, file_name, content_type, size_bytes, status,
		captured_by, captured_at, version, created_at
	FROM ticket_evidence
	WHERE tenant_id=$1 AND ticket_id=$2
	ORDER BY captured_at ASC`, tenantID, ticketID)
	if err != nil {
		return nil, fmt.Errorf("list evidence: %w", err)
	}
	defer rows.Close()

	var records []EvidenceRecord
	for rows.Next() {
		var e EvidenceRecord
		var checklistItemID, objectID, fileName, contentType *string
		if err := rows.Scan(
			&e.ID, &e.TenantID, &e.TicketID, &checklistItemID, &objectID,
			&e.ContentHash, &fileName, &contentType, &e.SizeBytes, &e.Status,
			&e.CapturedBy, &e.CapturedAt, &e.Version, &e.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan evidence: %w", err)
		}
		if checklistItemID != nil {
			e.ChecklistItemID = *checklistItemID
		}
		if objectID != nil {
			e.ObjectID = *objectID
		}
		if fileName != nil {
			e.FileName = *fileName
		}
		if contentType != nil {
			e.ContentType = *contentType
		}
		records = append(records, e)
	}
	return records, rows.Err()
}

func (s *TicketStore) InsertIncidentAlert(ctx context.Context, q querier, a *IncidentAlert) error {
	if a.ID == "" {
		a.ID = newID("ial")
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}

	_, err := q.Exec(ctx, `INSERT INTO incident_alerts (
		id, tenant_id, property_id, ticket_id, severity, target, policy, status, created_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		a.ID, a.TenantID, a.PropertyID, a.TicketID, a.Severity, a.Target, a.Policy, a.Status, a.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert incident alert: %w", err)
	}
	return nil
}

func (s *TicketStore) ListIncidentAlerts(ctx context.Context, tenantID, propertyID, status string) ([]IncidentAlert, error) {
	return listIncidentAlerts(ctx, s.pool, tenantID, propertyID, status)
}

func listIncidentAlerts(ctx context.Context, q querier, tenantID, propertyID, status string) ([]IncidentAlert, error) {
	rows, err := q.Query(ctx, `SELECT
		id, tenant_id, property_id, ticket_id, severity, target, policy, status, created_at
	FROM incident_alerts
	WHERE tenant_id=$1 AND property_id=$2 AND ($3 = '' OR status=$3)
	ORDER BY created_at ASC`, tenantID, propertyID, status)
	if err != nil {
		return nil, fmt.Errorf("list incident alerts: %w", err)
	}
	defer rows.Close()

	var alerts []IncidentAlert
	for rows.Next() {
		var a IncidentAlert
		if err := rows.Scan(
			&a.ID, &a.TenantID, &a.PropertyID, &a.TicketID, &a.Severity, &a.Target,
			&a.Policy, &a.Status, &a.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan incident alert: %w", err)
		}
		alerts = append(alerts, a)
	}
	return alerts, rows.Err()
}

func (s *TicketStore) ListIncidentAlertsForTicket(ctx context.Context, q querier, tenantID, ticketID string) ([]IncidentAlert, error) {
	rows, err := q.Query(ctx, `SELECT
		id, tenant_id, property_id, ticket_id, severity, target, policy, status, created_at
	FROM incident_alerts
	WHERE tenant_id=$1 AND ticket_id=$2
	ORDER BY created_at ASC`, tenantID, ticketID)
	if err != nil {
		return nil, fmt.Errorf("list incident alerts for ticket: %w", err)
	}
	defer rows.Close()

	var alerts []IncidentAlert
	for rows.Next() {
		var a IncidentAlert
		if err := rows.Scan(
			&a.ID, &a.TenantID, &a.PropertyID, &a.TicketID, &a.Severity, &a.Target,
			&a.Policy, &a.Status, &a.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan incident alert: %w", err)
		}
		alerts = append(alerts, a)
	}
	return alerts, rows.Err()
}

func (s *TicketStore) InsertServiceRecovery(ctx context.Context, q querier, r *ServiceRecovery) error {
	if r.ID == "" {
		r.ID = newID("rec")
	}
	now := time.Now().UTC()
	if r.CreatedAt.IsZero() {
		r.CreatedAt = now
	}
	if r.UpdatedAt.IsZero() {
		r.UpdatedAt = now
	}

	hashesJSON, err := json.Marshal(r.OriginalEvidenceHashes)
	if err != nil {
		return fmt.Errorf("marshal original evidence hashes: %w", err)
	}

	_, err = q.Exec(ctx, `INSERT INTO service_recoveries (
		id, tenant_id, property_id, incident_ticket_id, follow_up_ticket_id,
		severity, original_reason, original_evidence_hashes, responsibility,
		rework_cost_minor, currency, status, created_by, created_at, updated_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		r.ID, r.TenantID, r.PropertyID, r.IncidentTicketID, nullString(r.FollowUpTicketID),
		r.Severity, r.OriginalReason, hashesJSON, r.Responsibility,
		r.ReworkCostMinor, nullString(r.Currency), r.Status, r.CreatedBy, r.CreatedAt, r.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert service recovery: %w", err)
	}
	return nil
}

func (s *TicketStore) GetServiceRecovery(ctx context.Context, tenantID, recoveryID string) (*ServiceRecovery, error) {
	row := s.pool.QueryRow(ctx, `SELECT
		id, tenant_id, property_id, incident_ticket_id, follow_up_ticket_id,
		severity, original_reason, original_evidence_hashes, responsibility,
		rework_cost_minor, currency, status, created_by, created_at, updated_at
	FROM service_recoveries
	WHERE tenant_id=$1 AND id=$2`, tenantID, recoveryID)
	return scanServiceRecovery(row)
}

func (s *TicketStore) ListServiceRecoveries(ctx context.Context, tenantID, incidentTicketID string) ([]ServiceRecovery, error) {
	rows, err := s.pool.Query(ctx, `SELECT
		id, tenant_id, property_id, incident_ticket_id, follow_up_ticket_id,
		severity, original_reason, original_evidence_hashes, responsibility,
		rework_cost_minor, currency, status, created_by, created_at, updated_at
	FROM service_recoveries
	WHERE tenant_id=$1 AND incident_ticket_id=$2
	ORDER BY created_at ASC`, tenantID, incidentTicketID)
	if err != nil {
		return nil, fmt.Errorf("list service recoveries: %w", err)
	}
	defer rows.Close()

	var recoveries []ServiceRecovery
	for rows.Next() {
		r, err := scanServiceRecoveryFromRow(rows)
		if err != nil {
			return nil, err
		}
		recoveries = append(recoveries, *r)
	}
	return recoveries, rows.Err()
}

func (s *TicketStore) CloseServiceRecovery(ctx context.Context, tx pgx.Tx, r *ServiceRecovery) error {
	r.Status = RecoveryStatusClosed
	r.UpdatedAt = time.Now().UTC()

	tag, err := tx.Exec(ctx, `UPDATE service_recoveries SET
		status=$1, updated_at=$2
	WHERE id=$3 AND tenant_id=$4 AND status=$5`,
		r.Status, r.UpdatedAt, r.ID, r.TenantID, RecoveryStatusOpen,
	)
	if err != nil {
		return fmt.Errorf("close service recovery: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrRecoveryInactive
	}
	return nil
}

func scanServiceRecovery(row pgx.Row) (*ServiceRecovery, error) {
	var r ServiceRecovery
	var hashesBytes []byte
	var followUpTicketID, currency *string
	if err := row.Scan(
		&r.ID, &r.TenantID, &r.PropertyID, &r.IncidentTicketID, &followUpTicketID,
		&r.Severity, &r.OriginalReason, &hashesBytes, &r.Responsibility,
		&r.ReworkCostMinor, &currency, &r.Status, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRecoveryNotFound
		}
		return nil, fmt.Errorf("scan service recovery: %w", err)
	}
	if followUpTicketID != nil {
		r.FollowUpTicketID = *followUpTicketID
	}
	if currency != nil {
		r.Currency = *currency
	}
	if len(hashesBytes) > 0 {
		json.Unmarshal(hashesBytes, &r.OriginalEvidenceHashes)
	}
	return &r, nil
}

func scanServiceRecoveryFromRow(row pgx.Rows) (*ServiceRecovery, error) {
	var r ServiceRecovery
	var hashesBytes []byte
	var followUpTicketID, currency *string
	if err := row.Scan(
		&r.ID, &r.TenantID, &r.PropertyID, &r.IncidentTicketID, &followUpTicketID,
		&r.Severity, &r.OriginalReason, &hashesBytes, &r.Responsibility,
		&r.ReworkCostMinor, &currency, &r.Status, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("scan service recovery row: %w", err)
	}
	if followUpTicketID != nil {
		r.FollowUpTicketID = *followUpTicketID
	}
	if currency != nil {
		r.Currency = *currency
	}
	if len(hashesBytes) > 0 {
		json.Unmarshal(hashesBytes, &r.OriginalEvidenceHashes)
	}
	return &r, nil
}

func (s *TicketStore) InsertChecklistSyncRecord(ctx context.Context, q querier, rec *ChecklistSyncRecord) error {
	if rec.ID == "" {
		rec.ID = newID("csr")
	}
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now().UTC()
	}
	_, err := q.Exec(ctx, `INSERT INTO checklist_sync_records (
		id, sync_key, tenant_id, ticket_id, payload_hash, result, created_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		rec.ID, rec.SyncKey, rec.TenantID, rec.TicketID, rec.PayloadHash, rec.Result, rec.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert checklist sync record: %w", err)
	}
	return nil
}

func (s *TicketStore) FindChecklistSyncRecord(ctx context.Context, q querier, syncKey string) (*ChecklistSyncRecord, error) {
	row := q.QueryRow(ctx, `SELECT
		id, sync_key, tenant_id, ticket_id, payload_hash, result, created_at
	FROM checklist_sync_records WHERE sync_key=$1`, syncKey)

	var rec ChecklistSyncRecord
	if err := row.Scan(
		&rec.ID, &rec.SyncKey, &rec.TenantID, &rec.TicketID,
		&rec.PayloadHash, &rec.Result, &rec.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("find checklist sync record: %w", err)
	}
	return &rec, nil
}

func (s *TicketStore) InsertSyncConflict(ctx context.Context, q querier, c *SyncConflict) error {
	if c.ID == "" {
		c.ID = newID("csc")
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}
	_, err := q.Exec(ctx, `INSERT INTO checklist_sync_conflicts (
		id, tenant_id, ticket_id, checklist_item_id, template_item_index,
		server_label, server_status, server_version,
		client_label, client_status, client_version,
		resolved, created_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		c.ID, c.TenantID, c.TicketID, nullString(c.ChecklistItemID), c.TemplateItemIndex,
		c.ServerLabel, c.ServerStatus, c.ServerVersion,
		c.ClientLabel, c.ClientStatus, c.ClientVersion,
		c.Resolved, c.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert sync conflict: %w", err)
	}
	return nil
}

func (s *TicketStore) ListSyncConflicts(ctx context.Context, tenantID, ticketID string, resolved bool) ([]SyncConflict, error) {
	rows, err := s.pool.Query(ctx, `SELECT
		id, tenant_id, ticket_id, checklist_item_id, template_item_index,
		server_label, server_status, server_version,
		client_label, client_status, client_version,
		resolved, resolution, resolved_by, created_at, resolved_at
	FROM checklist_sync_conflicts
	WHERE tenant_id=$1 AND ticket_id=$2 AND ($3 = false OR resolved=$3)
	ORDER BY created_at ASC`, tenantID, ticketID, resolved)
	if err != nil {
		return nil, fmt.Errorf("list sync conflicts: %w", err)
	}
	defer rows.Close()

	var conflicts []SyncConflict
	for rows.Next() {
		var c SyncConflict
		var checklistItemID, resolution, resolvedBy *string
		if err := rows.Scan(
			&c.ID, &c.TenantID, &c.TicketID, &checklistItemID, &c.TemplateItemIndex,
			&c.ServerLabel, &c.ServerStatus, &c.ServerVersion,
			&c.ClientLabel, &c.ClientStatus, &c.ClientVersion,
			&c.Resolved, &resolution, &resolvedBy, &c.CreatedAt, &c.ResolvedAt,
		); err != nil {
			return nil, fmt.Errorf("scan sync conflict: %w", err)
		}
		if checklistItemID != nil {
			c.ChecklistItemID = *checklistItemID
		}
		if resolution != nil {
			c.Resolution = *resolution
		}
		if resolvedBy != nil {
			c.ResolvedBy = *resolvedBy
		}
		conflicts = append(conflicts, c)
	}
	return conflicts, rows.Err()
}

func (s *TicketStore) GetSyncConflict(ctx context.Context, tenantID, conflictID string) (*SyncConflict, error) {
	row := s.pool.QueryRow(ctx, `SELECT
		id, tenant_id, ticket_id, checklist_item_id, template_item_index,
		server_label, server_status, server_version,
		client_label, client_status, client_version,
		resolved, resolution, resolved_by, created_at, resolved_at
	FROM checklist_sync_conflicts
	WHERE tenant_id=$1 AND id=$2`, tenantID, conflictID)

	var c SyncConflict
	var checklistItemID, resolution, resolvedBy *string
	if err := row.Scan(
		&c.ID, &c.TenantID, &c.TicketID, &checklistItemID, &c.TemplateItemIndex,
		&c.ServerLabel, &c.ServerStatus, &c.ServerVersion,
		&c.ClientLabel, &c.ClientStatus, &c.ClientVersion,
		&c.Resolved, &resolution, &resolvedBy, &c.CreatedAt, &c.ResolvedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSyncConflictNotFound
		}
		return nil, fmt.Errorf("scan sync conflict: %w", err)
	}
	if checklistItemID != nil {
		c.ChecklistItemID = *checklistItemID
	}
	if resolution != nil {
		c.Resolution = *resolution
	}
	if resolvedBy != nil {
		c.ResolvedBy = *resolvedBy
	}
	return &c, nil
}

func (s *TicketStore) ResolveSyncConflict(ctx context.Context, q querier, c *SyncConflict, resolution string, actorID string) error {
	now := time.Now().UTC()
	_, err := q.Exec(ctx, `UPDATE checklist_sync_conflicts SET
		resolved=true, resolution=$1, resolved_by=$2, resolved_at=$3
	WHERE id=$4 AND tenant_id=$5 AND resolved=false`,
		resolution, actorID, now, c.ID, c.TenantID,
	)
	if err != nil {
		return fmt.Errorf("resolve sync conflict: %w", err)
	}
	c.Resolved = true
	c.Resolution = resolution
	c.ResolvedBy = actorID
	c.ResolvedAt = &now
	return nil
}

func (s *TicketStore) InsertQueuedOfflineEvidence(ctx context.Context, q querier, e *QueuedOfflineEvidence) error {
	if e.ID == "" {
		e.ID = newID("qoe")
	}
	now := time.Now().UTC()
	if e.CreatedAt.IsZero() {
		e.CreatedAt = now
	}
	if e.CapturedAt.IsZero() {
		e.CapturedAt = now
	}
	_, err := q.Exec(ctx, `INSERT INTO queued_offline_evidence (
		id, tenant_id, ticket_id, checklist_item_id, content_hash,
		file_name, content_type, size_bytes, status, captured_by, captured_at, created_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		e.ID, e.TenantID, e.TicketID, nullString(e.ChecklistItemID), e.ContentHash,
		nullString(e.FileName), nullString(e.ContentType), e.SizeBytes, e.Status,
		e.CapturedBy, e.CapturedAt, e.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert queued offline evidence: %w", err)
	}
	return nil
}

func (s *TicketStore) FindQueuedOfflineEvidence(ctx context.Context, tenantID, ticketID, contentHash string) (*QueuedOfflineEvidence, error) {
	row := s.pool.QueryRow(ctx, `SELECT
		id, tenant_id, ticket_id, checklist_item_id, content_hash,
		file_name, content_type, size_bytes, status, captured_by, captured_at, created_at
	FROM queued_offline_evidence
	WHERE tenant_id=$1 AND ticket_id=$2 AND content_hash=$3 AND status=$4
	LIMIT 1`, tenantID, ticketID, contentHash, OfflineEvidenceQueued)

	var e QueuedOfflineEvidence
	var checklistItemID, fileName, contentType *string
	if err := row.Scan(
		&e.ID, &e.TenantID, &e.TicketID, &checklistItemID, &e.ContentHash,
		&fileName, &contentType, &e.SizeBytes, &e.Status, &e.CapturedBy, &e.CapturedAt, &e.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOfflineEvidenceNotFound
		}
		return nil, fmt.Errorf("scan queued offline evidence: %w", err)
	}
	if checklistItemID != nil {
		e.ChecklistItemID = *checklistItemID
	}
	if fileName != nil {
		e.FileName = *fileName
	}
	if contentType != nil {
		e.ContentType = *contentType
	}
	return &e, nil
}

func (s *TicketStore) ListQueuedOfflineEvidence(ctx context.Context, tenantID, ticketID, status string) ([]QueuedOfflineEvidence, error) {
	rows, err := s.pool.Query(ctx, `SELECT
		id, tenant_id, ticket_id, checklist_item_id, content_hash,
		file_name, content_type, size_bytes, status, captured_by, captured_at, created_at
	FROM queued_offline_evidence
	WHERE tenant_id=$1 AND ticket_id=$2 AND ($3 = '' OR status=$3)
	ORDER BY captured_at ASC`, tenantID, ticketID, status)
	if err != nil {
		return nil, fmt.Errorf("list queued offline evidence: %w", err)
	}
	defer rows.Close()

	var records []QueuedOfflineEvidence
	for rows.Next() {
		var e QueuedOfflineEvidence
		var checklistItemID, fileName, contentType *string
		if err := rows.Scan(
			&e.ID, &e.TenantID, &e.TicketID, &checklistItemID, &e.ContentHash,
			&fileName, &contentType, &e.SizeBytes, &e.Status, &e.CapturedBy, &e.CapturedAt, &e.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan queued offline evidence: %w", err)
		}
		if checklistItemID != nil {
			e.ChecklistItemID = *checklistItemID
		}
		if fileName != nil {
			e.FileName = *fileName
		}
		if contentType != nil {
			e.ContentType = *contentType
		}
		records = append(records, e)
	}
	return records, rows.Err()
}

func (s *TicketStore) MarkQueuedEvidenceUploaded(ctx context.Context, q querier, e *QueuedOfflineEvidence) error {
	_, err := q.Exec(ctx, `UPDATE queued_offline_evidence SET
		status=$1
	WHERE id=$2 AND tenant_id=$3 AND status=$4`,
		OfflineEvidenceUploaded, e.ID, e.TenantID, OfflineEvidenceQueued,
	)
	if err != nil {
		return fmt.Errorf("mark queued evidence uploaded: %w", err)
	}
	e.Status = OfflineEvidenceUploaded
	return nil
}

func (s *TicketStore) UpdateChecklistItemNoConflict(ctx context.Context, q querier, item *TicketChecklistItem) error {
	item.UpdatedAt = time.Now().UTC()

	evJSON, err := json.Marshal(item.EvidenceIDs)
	if err != nil {
		return fmt.Errorf("marshal evidence ids: %w", err)
	}

	tag, err := q.Exec(ctx, `UPDATE ticket_checklist_items SET
		status=$1, completed_by=$2, completed_at=$3, evidence_ids=$4,
		evidence_required=$5, notes=$6, version=version+1, updated_at=$7
	WHERE id=$8 AND tenant_id=$9`,
		item.Status, nullString(item.CompletedBy), item.CompletedAt,
		evJSON, item.EvidenceRequired, nullString(item.Notes), item.UpdatedAt,
		item.ID, item.TenantID,
	)
	if err != nil {
		return fmt.Errorf("update checklist item no conflict: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errors.New("checklist item update lost a concurrent write")
	}
	item.Version++
	return nil
}
