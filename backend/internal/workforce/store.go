package workforce

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

type WorkforceStore struct {
	pool *pgxpool.Pool
}

func NewWorkforceStore(pool *pgxpool.Pool) *WorkforceStore {
	return &WorkforceStore{pool: pool}
}

const workerColumns = `id, tenant_id, legal_name, verified_identity, date_of_birth,
	age_eligible, contact_method, classification, specialist, service_zone,
	skills, status, version, created_at, updated_at`

func scanWorker(row pgx.Row) (*Worker, error) {
	var w Worker
	var skills []byte
	err := row.Scan(
		&w.ID, &w.TenantID, &w.LegalName, &w.VerifiedIdentity, &w.DateOfBirth,
		&w.AgeEligible, &w.ContactMethod, &w.Classification, &w.Specialist, &w.ServiceZone,
		&skills, &w.Status, &w.Version, &w.CreatedAt, &w.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrWorkerNotFound
		}
		return nil, err
	}
	if len(skills) > 0 {
		_ = json.Unmarshal(skills, &w.Skills)
	}
	return &w, nil
}

func (s *WorkforceStore) InsertWorker(ctx context.Context, q querier, w *Worker) error {
	if w.ID == "" {
		w.ID = newID("wrk")
	}
	now := time.Now().UTC()
	if w.CreatedAt.IsZero() {
		w.CreatedAt = now
	}
	if w.UpdatedAt.IsZero() {
		w.UpdatedAt = now
	}
	if w.Version == 0 {
		w.Version = 1
	}
	if w.Status == "" {
		w.Status = StatusActive
	}

	skillsJSON, err := json.Marshal(w.Skills)
	if err != nil {
		return fmt.Errorf("marshal skills: %w", err)
	}

	_, err = q.Exec(ctx, `INSERT INTO workers (
		id, tenant_id, legal_name, verified_identity, date_of_birth,
		age_eligible, contact_method, classification, specialist, service_zone,
		skills, status, version, created_at, updated_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		w.ID, w.TenantID, w.LegalName, w.VerifiedIdentity, w.DateOfBirth,
		w.AgeEligible, w.ContactMethod, w.Classification, w.Specialist, w.ServiceZone,
		skillsJSON, w.Status, w.Version, w.CreatedAt, w.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert worker: %w", err)
	}
	return nil
}

func (s *WorkforceStore) GetWorker(ctx context.Context, tenantID, workerID string) (*Worker, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+workerColumns+` FROM workers WHERE tenant_id=$1 AND id=$2`, tenantID, workerID)
	return scanWorker(row)
}

func (s *WorkforceStore) GetWorkerForUpdate(ctx context.Context, tx pgx.Tx, tenantID, workerID string) (*Worker, error) {
	row := tx.QueryRow(ctx, `SELECT `+workerColumns+` FROM workers WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenantID, workerID)
	return scanWorker(row)
}

func (s *WorkforceStore) ListWorkers(ctx context.Context, tenantID string) ([]Worker, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+workerColumns+` FROM workers WHERE tenant_id=$1 ORDER BY created_at`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var workers []Worker
	for rows.Next() {
		w, err := scanWorker(rows)
		if err != nil {
			return nil, err
		}
		workers = append(workers, *w)
	}
	return workers, rows.Err()
}

func (s *WorkforceStore) UpdateWorkerStatus(ctx context.Context, tx pgx.Tx, w *Worker) error {
	tag, err := tx.Exec(ctx, `UPDATE workers SET
		status=$3, version=$4, updated_at=$5
		WHERE tenant_id=$1 AND id=$2 AND version=$6`,
		w.TenantID, w.ID, w.Status, w.Version, w.UpdatedAt, w.Version-1,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrConcurrentModification
	}
	return nil
}

func (s *WorkforceStore) InsertCertification(ctx context.Context, q querier, c *Certification) error {
	if c.ID == "" {
		c.ID = newID("cert")
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}
	_, err := q.Exec(ctx, `INSERT INTO worker_certifications (
		id, tenant_id, worker_id, work_type, issuer, issued_at, expires_at, status, created_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		c.ID, c.TenantID, c.WorkerID, c.WorkType, c.Issuer, c.IssuedAt, c.ExpiresAt, c.Status, c.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert certification: %w", err)
	}
	return nil
}

func (s *WorkforceStore) ListCertifications(ctx context.Context, tenantID, workerID string) ([]Certification, error) {
	rows, err := s.pool.Query(ctx, `SELECT
		id, tenant_id, worker_id, work_type, issuer, issued_at, expires_at, status, created_at
		FROM worker_certifications WHERE tenant_id=$1 AND worker_id=$2 ORDER BY expires_at`, tenantID, workerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var certs []Certification
	for rows.Next() {
		var c Certification
		if err := rows.Scan(&c.ID, &c.TenantID, &c.WorkerID, &c.WorkType, &c.Issuer, &c.IssuedAt, &c.ExpiresAt, &c.Status, &c.CreatedAt); err != nil {
			return nil, err
		}
		certs = append(certs, c)
	}
	return certs, rows.Err()
}

func (s *WorkforceStore) InsertRating(ctx context.Context, q querier, r *Rating) error {
	if r.ID == "" {
		r.ID = newID("rate")
	}
	if r.RecordedAt.IsZero() {
		r.RecordedAt = time.Now().UTC()
	}
	_, err := q.Exec(ctx, `INSERT INTO worker_ratings (
		id, tenant_id, worker_id, score, source, comment, recorded_by, recorded_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		r.ID, r.TenantID, r.WorkerID, r.Score, r.Source, nullString(r.Comment), r.RecordedBy, r.RecordedAt,
	)
	if err != nil {
		return fmt.Errorf("insert rating: %w", err)
	}
	return nil
}

func (s *WorkforceStore) ListRatings(ctx context.Context, tenantID, workerID string) ([]Rating, error) {
	rows, err := s.pool.Query(ctx, `SELECT
		id, tenant_id, worker_id, score, source, comment, recorded_by, recorded_at
		FROM worker_ratings WHERE tenant_id=$1 AND worker_id=$2 ORDER BY recorded_at`, tenantID, workerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ratings []Rating
	for rows.Next() {
		var r Rating
		var comment *string
		if err := rows.Scan(&r.ID, &r.TenantID, &r.WorkerID, &r.Score, &r.Source, &comment, &r.RecordedBy, &r.RecordedAt); err != nil {
			return nil, err
		}
		if comment != nil {
			r.Comment = *comment
		}
		ratings = append(ratings, r)
	}
	return ratings, rows.Err()
}

func (s *WorkforceStore) InsertAdverseAction(ctx context.Context, tx pgx.Tx, a *AdverseActionReview) error {
	if a.ID == "" {
		a.ID = newID("aarev")
	}
	if a.DecidedAt.IsZero() {
		a.DecidedAt = time.Now().UTC()
	}
	evidenceJSON, err := json.Marshal(a.EvidenceRefs)
	if err != nil {
		return fmt.Errorf("marshal evidence refs: %w", err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO adverse_action_reviews (
		id, tenant_id, worker_id, action, evidence_refs, reviewer_id, reason, worker_version, decided_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		a.ID, a.TenantID, a.WorkerID, a.Action, evidenceJSON, a.ReviewerID, a.Reason, a.WorkerVersion, a.DecidedAt,
	)
	if err != nil {
		return fmt.Errorf("insert adverse action review: %w", err)
	}
	return nil
}

func (s *WorkforceStore) ListAdverseActions(ctx context.Context, tenantID, workerID string) ([]AdverseActionReview, error) {
	rows, err := s.pool.Query(ctx, `SELECT
		id, tenant_id, worker_id, action, evidence_refs, reviewer_id, reason, worker_version, decided_at
		FROM adverse_action_reviews WHERE tenant_id=$1 AND worker_id=$2 ORDER BY decided_at`, tenantID, workerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var actions []AdverseActionReview
	for rows.Next() {
		var a AdverseActionReview
		var evidenceJSON []byte
		if err := rows.Scan(&a.ID, &a.TenantID, &a.WorkerID, &a.Action, &evidenceJSON, &a.ReviewerID, &a.Reason, &a.WorkerVersion, &a.DecidedAt); err != nil {
			return nil, err
		}
		if len(evidenceJSON) > 0 {
			_ = json.Unmarshal(evidenceJSON, &a.EvidenceRefs)
		}
		actions = append(actions, a)
	}
	return actions, rows.Err()
}

func (s *WorkforceStore) InsertAssignment(ctx context.Context, q querier, a *WorkforceAssignment) error {
	if a.ID == "" {
		a.ID = newID("wfa")
	}
	if a.AssignedAt.IsZero() {
		a.AssignedAt = time.Now().UTC()
	}
	_, err := q.Exec(ctx, `INSERT INTO workforce_assignments (
		id, tenant_id, worker_id, work_type, assigned_by, assigned_at
	) VALUES ($1,$2,$3,$4,$5,$6)`,
		a.ID, a.TenantID, a.WorkerID, a.WorkType, a.AssignedBy, a.AssignedAt,
	)
	if err != nil {
		return fmt.Errorf("insert workforce assignment: %w", err)
	}
	return nil
}

func (s *WorkforceStore) ListAssignments(ctx context.Context, tenantID, workerID string) ([]WorkforceAssignment, error) {
	rows, err := s.pool.Query(ctx, `SELECT
		id, tenant_id, worker_id, work_type, assigned_by, assigned_at
		FROM workforce_assignments WHERE tenant_id=$1 AND worker_id=$2 ORDER BY assigned_at`, tenantID, workerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assignments []WorkforceAssignment
	for rows.Next() {
		var a WorkforceAssignment
		if err := rows.Scan(&a.ID, &a.TenantID, &a.WorkerID, &a.WorkType, &a.AssignedBy, &a.AssignedAt); err != nil {
			return nil, err
		}
		assignments = append(assignments, a)
	}
	return assignments, rows.Err()
}

func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nullTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func (s *WorkforceStore) InsertAvailabilityWindow(ctx context.Context, q querier, a *AvailabilityWindow) error {
	if a.ID == "" {
		a.ID = newID("avail")
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	if a.EffectiveAt.IsZero() {
		a.EffectiveAt = time.Now().UTC()
	}
	_, err := q.Exec(ctx, `INSERT INTO availability_windows (
		id, tenant_id, worker_id, day_of_week, start_minute, end_minute, effective_at, created_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		a.ID, a.TenantID, a.WorkerID, a.DayOfWeek, a.StartMinute, a.EndMinute, a.EffectiveAt, a.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert availability window: %w", err)
	}
	return nil
}

func (s *WorkforceStore) ListAvailabilityWindows(ctx context.Context, tenantID, workerID string) ([]AvailabilityWindow, error) {
	rows, err := s.pool.Query(ctx, `SELECT
		id, tenant_id, worker_id, day_of_week, start_minute, end_minute, effective_at, created_at
		FROM availability_windows WHERE tenant_id=$1 AND worker_id=$2 ORDER BY day_of_week, start_minute`, tenantID, workerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var windows []AvailabilityWindow
	for rows.Next() {
		var a AvailabilityWindow
		if err := rows.Scan(&a.ID, &a.TenantID, &a.WorkerID, &a.DayOfWeek, &a.StartMinute, &a.EndMinute, &a.EffectiveAt, &a.CreatedAt); err != nil {
			return nil, err
		}
		windows = append(windows, a)
	}
	return windows, rows.Err()
}

func (s *WorkforceStore) InsertTimeEntry(ctx context.Context, q querier, e *TimeEntry) error {
	if e.ID == "" {
		e.ID = newID("time")
	}
	if e.RecordedAt.IsZero() {
		e.RecordedAt = time.Now().UTC()
	}
	_, err := q.Exec(ctx, `INSERT INTO time_entries (
		id, tenant_id, worker_id, ticket_id, work_minutes, travel_minutes, overtime_flag, recorded_by, recorded_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		e.ID, e.TenantID, e.WorkerID, nullString(e.TicketID), e.WorkMinutes, e.TravelMinutes, e.OvertimeFlag, e.RecordedBy, e.RecordedAt,
	)
	if err != nil {
		return fmt.Errorf("insert time entry: %w", err)
	}
	return nil
}

func (s *WorkforceStore) ListTimeEntries(ctx context.Context, tenantID, workerID string) ([]TimeEntry, error) {
	rows, err := s.pool.Query(ctx, `SELECT
		id, tenant_id, worker_id, ticket_id, work_minutes, travel_minutes, overtime_flag, recorded_by, recorded_at
		FROM time_entries WHERE tenant_id=$1 AND worker_id=$2 ORDER BY recorded_at DESC`, tenantID, workerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []TimeEntry
	for rows.Next() {
		var e TimeEntry
		var ticketID *string
		if err := rows.Scan(&e.ID, &e.TenantID, &e.WorkerID, &ticketID, &e.WorkMinutes, &e.TravelMinutes, &e.OvertimeFlag, &e.RecordedBy, &e.RecordedAt); err != nil {
			return nil, err
		}
		if ticketID != nil {
			e.TicketID = *ticketID
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (s *WorkforceStore) InsertExpense(ctx context.Context, q querier, e *Expense) error {
	if e.ID == "" {
		e.ID = newID("exp")
	}
	if e.RecordedAt.IsZero() {
		e.RecordedAt = time.Now().UTC()
	}
	_, err := q.Exec(ctx, `INSERT INTO expenses (
		id, tenant_id, worker_id, ticket_id, minor_units, currency, category, receipt_ref, recorded_by, recorded_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		e.ID, e.TenantID, e.WorkerID, nullString(e.TicketID), e.MinorUnits, e.Currency, e.Category, nullString(e.ReceiptRef), e.RecordedBy, e.RecordedAt,
	)
	if err != nil {
		return fmt.Errorf("insert expense: %w", err)
	}
	return nil
}

func (s *WorkforceStore) ListExpenses(ctx context.Context, tenantID, workerID string) ([]Expense, error) {
	rows, err := s.pool.Query(ctx, `SELECT
		id, tenant_id, worker_id, ticket_id, minor_units, currency, category, receipt_ref, recorded_by, recorded_at
		FROM expenses WHERE tenant_id=$1 AND worker_id=$2 ORDER BY recorded_at DESC`, tenantID, workerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var expenses []Expense
	for rows.Next() {
		var e Expense
		var ticketID, receiptRef *string
		if err := rows.Scan(&e.ID, &e.TenantID, &e.WorkerID, &ticketID, &e.MinorUnits, &e.Currency, &e.Category, &receiptRef, &e.RecordedBy, &e.RecordedAt); err != nil {
			return nil, err
		}
		if ticketID != nil {
			e.TicketID = *ticketID
		}
		if receiptRef != nil {
			e.ReceiptRef = *receiptRef
		}
		expenses = append(expenses, e)
	}
	return expenses, rows.Err()
}

func (s *WorkforceStore) InsertGrievance(ctx context.Context, q querier, g *Grievance) error {
	if g.ID == "" {
		g.ID = newID("grv")
	}
	if g.SubmittedAt.IsZero() {
		g.SubmittedAt = time.Now().UTC()
	}
	if g.Status == "" {
		g.Status = GrievanceStatusPending
	}
	evidenceJSON, err := json.Marshal(g.EvidenceRefs)
	if err != nil {
		return fmt.Errorf("marshal evidence refs: %w", err)
	}
	_, err = q.Exec(ctx, `INSERT INTO grievances (
		id, tenant_id, worker_id, kind, reason, evidence_refs, status, submitted_at, resolved_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		g.ID, g.TenantID, g.WorkerID, g.Kind, g.Reason, evidenceJSON, g.Status, g.SubmittedAt, g.ResolvedAt,
	)
	if err != nil {
		return fmt.Errorf("insert grievance: %w", err)
	}
	return nil
}

func (s *WorkforceStore) ListGrievances(ctx context.Context, tenantID, workerID string) ([]Grievance, error) {
	rows, err := s.pool.Query(ctx, `SELECT
		id, tenant_id, worker_id, kind, reason, evidence_refs, status, submitted_at, resolved_at
		FROM grievances WHERE tenant_id=$1 AND worker_id=$2 ORDER BY submitted_at DESC`, tenantID, workerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var grievances []Grievance
	for rows.Next() {
		var g Grievance
		var evidenceJSON []byte
		var resolvedAt *time.Time
		if err := rows.Scan(&g.ID, &g.TenantID, &g.WorkerID, &g.Kind, &g.Reason, &evidenceJSON, &g.Status, &g.SubmittedAt, &resolvedAt); err != nil {
			return nil, err
		}
		if len(evidenceJSON) > 0 {
			_ = json.Unmarshal(evidenceJSON, &g.EvidenceRefs)
		}
		g.ResolvedAt = resolvedAt
		grievances = append(grievances, g)
	}
	return grievances, rows.Err()
}

func (s *WorkforceStore) GetGrievance(ctx context.Context, tenantID, grievanceID string) (*Grievance, error) {
	row := s.pool.QueryRow(ctx, `SELECT
		id, tenant_id, worker_id, kind, reason, evidence_refs, status, submitted_at, resolved_at
		FROM grievances WHERE tenant_id=$1 AND id=$2`, tenantID, grievanceID)

	var g Grievance
	var evidenceJSON []byte
	var resolvedAt *time.Time
	if err := row.Scan(&g.ID, &g.TenantID, &g.WorkerID, &g.Kind, &g.Reason, &evidenceJSON, &g.Status, &g.SubmittedAt, &resolvedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrGrievanceNotFound
		}
		return nil, err
	}
	if len(evidenceJSON) > 0 {
		_ = json.Unmarshal(evidenceJSON, &g.EvidenceRefs)
	}
	g.ResolvedAt = resolvedAt
	return &g, nil
}

func (s *WorkforceStore) InsertSOSEvent(ctx context.Context, q querier, e *SOSEvent) error {
	if e.ID == "" {
		e.ID = newID("sos")
	}
	if e.TriggeredAt.IsZero() {
		e.TriggeredAt = time.Now().UTC()
	}
	_, err := q.Exec(ctx, `INSERT INTO sos_events (
		id, tenant_id, worker_id, ticket_id, location, triggered_at, acknowledged_by, acknowledged_at, resolution, resolved_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		e.ID, e.TenantID, e.WorkerID, nullString(e.TicketID), e.Location, e.TriggeredAt,
		nullString(e.AcknowledgedBy), e.AcknowledgedAt, nullString(e.Resolution), e.ResolvedAt,
	)
	if err != nil {
		return fmt.Errorf("insert SOS event: %w", err)
	}
	return nil
}

func (s *WorkforceStore) ListSOSEvents(ctx context.Context, tenantID, workerID string) ([]SOSEvent, error) {
	rows, err := s.pool.Query(ctx, `SELECT
		id, tenant_id, worker_id, ticket_id, location, triggered_at, acknowledged_by, acknowledged_at, resolution, resolved_at
		FROM sos_events WHERE tenant_id=$1 AND worker_id=$2 ORDER BY triggered_at DESC`, tenantID, workerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []SOSEvent
	for rows.Next() {
		var e SOSEvent
		var ticketID, acknowledgedBy, resolution *string
		var acknowledgedAt, resolvedAt *time.Time
		if err := rows.Scan(&e.ID, &e.TenantID, &e.WorkerID, &ticketID, &e.Location, &e.TriggeredAt,
			&acknowledgedBy, &acknowledgedAt, &resolution, &resolvedAt); err != nil {
			return nil, err
		}
		if ticketID != nil {
			e.TicketID = *ticketID
		}
		if acknowledgedBy != nil {
			e.AcknowledgedBy = *acknowledgedBy
		}
		if resolution != nil {
			e.Resolution = *resolution
		}
		e.AcknowledgedAt = acknowledgedAt
		e.ResolvedAt = resolvedAt
		events = append(events, e)
	}
	return events, rows.Err()
}

func (s *WorkforceStore) InsertEmploymentTerm(ctx context.Context, q querier, t *EmploymentTerm) error {
	if t.ID == "" {
		t.ID = newID("term")
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now().UTC()
	}
	_, err := q.Exec(ctx, `INSERT INTO employment_terms (
		id, tenant_id, worker_id, role, compensation_band, effective_date, end_date, agreement_ref, created_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		t.ID, t.TenantID, t.WorkerID, t.Role, t.CompensationBand, t.EffectiveDate, t.EndDate, nullString(t.AgreementRef), t.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert employment term: %w", err)
	}
	return nil
}

func (s *WorkforceStore) ListEmploymentTerms(ctx context.Context, tenantID, workerID string) ([]EmploymentTerm, error) {
	rows, err := s.pool.Query(ctx, `SELECT
		id, tenant_id, worker_id, role, compensation_band, effective_date, end_date, agreement_ref, created_at
		FROM employment_terms WHERE tenant_id=$1 AND worker_id=$2 ORDER BY effective_date DESC`, tenantID, workerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var terms []EmploymentTerm
	for rows.Next() {
		var t EmploymentTerm
		var endDate *time.Time
		var agreementRef *string
		if err := rows.Scan(&t.ID, &t.TenantID, &t.WorkerID, &t.Role, &t.CompensationBand, &t.EffectiveDate, &endDate, &agreementRef, &t.CreatedAt); err != nil {
			return nil, err
		}
		t.EndDate = endDate
		if agreementRef != nil {
			t.AgreementRef = *agreementRef
		}
		terms = append(terms, t)
	}
	return terms, rows.Err()
}
