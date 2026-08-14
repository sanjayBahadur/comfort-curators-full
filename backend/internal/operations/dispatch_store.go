package operations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type dispatchStore struct {
	pool *pgxpool.Pool
}

func newDispatchStore(pool *pgxpool.Pool) *dispatchStore {
	return &dispatchStore{pool: pool}
}

const assignmentColumns = `id, tenant_id, ticket_id, worker_id, assigned_by,
	status, accept_until, accepted_at, version, created_at, updated_at`

func (s *dispatchStore) scanAssignment(row pgx.Row) (*TicketAssignment, error) {
	var a TicketAssignment
	var acceptUntil, acceptedAt *time.Time
	err := row.Scan(
		&a.ID, &a.TenantID, &a.TicketID, &a.WorkerID, &a.AssignedBy,
		&a.Status, &acceptUntil, &acceptedAt, &a.Version, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrDispatchAssignmentNotFound
		}
		return nil, err
	}
	a.AcceptUntil = acceptUntil
	a.AcceptedAt = acceptedAt
	return &a, nil
}

func (s *dispatchStore) InsertAssignment(ctx context.Context, a *TicketAssignment) error {
	if a.ID == "" {
		a.ID = newID("asgn")
	}
	now := time.Now().UTC()
	if a.CreatedAt.IsZero() {
		a.CreatedAt = now
	}
	if a.UpdatedAt.IsZero() {
		a.UpdatedAt = now
	}
	if a.Version == 0 {
		a.Version = 1
	}
	if a.Status == "" {
		a.Status = AssignmentStatusOffered
	}

	_, err := s.pool.Exec(ctx, `INSERT INTO ticket_assignments (
		id, tenant_id, ticket_id, worker_id, assigned_by,
		status, accept_until, accepted_at, version, created_at, updated_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		a.ID, a.TenantID, a.TicketID, a.WorkerID, a.AssignedBy,
		a.Status, nullTimeP(a.AcceptUntil), nullTimeP(a.AcceptedAt), a.Version, a.CreatedAt, a.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert ticket assignment: %w", err)
	}
	return nil
}

func (s *dispatchStore) GetAssignment(ctx context.Context, tenantID, assignmentID string) (*TicketAssignment, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+assignmentColumns+` FROM ticket_assignments WHERE tenant_id=$1 AND id=$2`, tenantID, assignmentID)
	return s.scanAssignment(row)
}

func (s *dispatchStore) ListAssignmentsForTicket(ctx context.Context, tenantID, ticketID string) ([]TicketAssignment, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+assignmentColumns+` FROM ticket_assignments WHERE tenant_id=$1 AND ticket_id=$2 ORDER BY created_at`, tenantID, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assignments []TicketAssignment
	for rows.Next() {
		a, err := s.scanAssignment(rows)
		if err != nil {
			return nil, err
		}
		assignments = append(assignments, *a)
	}
	return assignments, rows.Err()
}

func (s *dispatchStore) ListAssignmentsForWorker(ctx context.Context, tenantID, workerID string) ([]TicketAssignment, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+assignmentColumns+` FROM ticket_assignments WHERE tenant_id=$1 AND worker_id=$2 ORDER BY created_at`, tenantID, workerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assignments []TicketAssignment
	for rows.Next() {
		a, err := s.scanAssignment(rows)
		if err != nil {
			return nil, err
		}
		assignments = append(assignments, *a)
	}
	return assignments, rows.Err()
}

func (s *dispatchStore) UpdateAssignmentStatus(ctx context.Context, a *TicketAssignment) error {
	a.Version++
	a.UpdatedAt = time.Now().UTC()
	tag, err := s.pool.Exec(ctx, `UPDATE ticket_assignments SET
		status=$3, accepted_at=$4, version=$5, updated_at=$6
		WHERE tenant_id=$1 AND id=$2 AND version=$7`,
		a.TenantID, a.ID, a.Status, nullTimeP(a.AcceptedAt), a.Version, a.UpdatedAt, a.Version-1,
	)
	if err != nil {
		return fmt.Errorf("update assignment status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("concurrent modification on ticket assignment")
	}
	return nil
}

func (s *dispatchStore) InsertOverride(ctx context.Context, o *DispatchOverride) error {
	if o.ID == "" {
		o.ID = newID("dovr")
	}
	if o.CreatedAt.IsZero() {
		o.CreatedAt = time.Now().UTC()
	}

	_, err := s.pool.Exec(ctx, `INSERT INTO dispatch_overrides (
		id, tenant_id, ticket_id, worker_id, overridden_by, reason, overridden_constraint, created_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		o.ID, o.TenantID, o.TicketID, o.WorkerID, o.OverriddenBy, o.Reason, o.OverriddenConstraint, o.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert dispatch override: %w", err)
	}
	return nil
}

func (s *dispatchStore) ListOverridesForTicket(ctx context.Context, tenantID, ticketID string) ([]DispatchOverride, error) {
	rows, err := s.pool.Query(ctx, `SELECT
		id, tenant_id, ticket_id, worker_id, overridden_by, reason, overridden_constraint, created_at
		FROM dispatch_overrides WHERE tenant_id=$1 AND ticket_id=$2 ORDER BY created_at`, tenantID, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var overrides []DispatchOverride
	for rows.Next() {
		var o DispatchOverride
		if err := rows.Scan(&o.ID, &o.TenantID, &o.TicketID, &o.WorkerID, &o.OverriddenBy, &o.Reason, &o.OverriddenConstraint, &o.CreatedAt); err != nil {
			return nil, err
		}
		overrides = append(overrides, o)
	}
	return overrides, rows.Err()
}

type workforceWorker struct {
	ID             string   `json:"id"`
	TenantID       string   `json:"tenant_id"`
	LegalName      string   `json:"legal_name"`
	AgeEligible    bool     `json:"age_eligible"`
	Classification string   `json:"classification"`
	Specialist     bool     `json:"specialist"`
	ServiceZone    string   `json:"service_zone"`
	Skills         []string `json:"skills"`
	Status         string   `json:"status"`
}

type workforceCertification struct {
	WorkType  string
	Status    string
	ExpiresAt time.Time
}

type workforceAvailability struct {
	DayOfWeek   int
	StartMinute int
	EndMinute   int
}

type workforceTimeEntry struct {
	TicketID      string
	WorkMinutes   int
	TravelMinutes int
	RecordedAt    time.Time
}

type workforceEmploymentTerm struct {
	Role             string
	CompensationBand string
}

func (s *dispatchStore) listActiveWorkers(ctx context.Context, tenantID string) ([]workforceWorker, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, tenant_id, legal_name, age_eligible, classification, specialist, service_zone, skills, status
		FROM workers WHERE tenant_id=$1 AND status='active' ORDER BY created_at`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var workers []workforceWorker
	for rows.Next() {
		var w workforceWorker
		var skillsJSON []byte
		if err := rows.Scan(&w.ID, &w.TenantID, &w.LegalName, &w.AgeEligible, &w.Classification, &w.Specialist, &w.ServiceZone, &skillsJSON, &w.Status); err != nil {
			return nil, err
		}
		if len(skillsJSON) > 0 {
			_ = json.Unmarshal(skillsJSON, &w.Skills)
		}
		workers = append(workers, w)
	}
	return workers, rows.Err()
}

func (s *dispatchStore) listWorkerCertifications(ctx context.Context, tenantID, workerID string) ([]workforceCertification, error) {
	rows, err := s.pool.Query(ctx, `SELECT work_type, status, expires_at
		FROM worker_certifications WHERE tenant_id=$1 AND worker_id=$2`, tenantID, workerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var certs []workforceCertification
	for rows.Next() {
		var c workforceCertification
		if err := rows.Scan(&c.WorkType, &c.Status, &c.ExpiresAt); err != nil {
			return nil, err
		}
		certs = append(certs, c)
	}
	return certs, rows.Err()
}

func (s *dispatchStore) listWorkerAvailability(ctx context.Context, tenantID, workerID string) ([]workforceAvailability, error) {
	rows, err := s.pool.Query(ctx, `SELECT day_of_week, start_minute, end_minute
		FROM availability_windows WHERE tenant_id=$1 AND worker_id=$2 ORDER BY day_of_week, start_minute`, tenantID, workerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var windows []workforceAvailability
	for rows.Next() {
		var w workforceAvailability
		if err := rows.Scan(&w.DayOfWeek, &w.StartMinute, &w.EndMinute); err != nil {
			return nil, err
		}
		windows = append(windows, w)
	}
	return windows, rows.Err()
}

func (s *dispatchStore) listWorkerTimeEntries(ctx context.Context, tenantID, workerID string, since time.Time) ([]workforceTimeEntry, error) {
	rows, err := s.pool.Query(ctx, `SELECT ticket_id, work_minutes, travel_minutes, recorded_at
		FROM time_entries WHERE tenant_id=$1 AND worker_id=$2 AND recorded_at >= $3 ORDER BY recorded_at`, tenantID, workerID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []workforceTimeEntry
	for rows.Next() {
		var e workforceTimeEntry
		var ticketID *string
		if err := rows.Scan(&ticketID, &e.WorkMinutes, &e.TravelMinutes, &e.RecordedAt); err != nil {
			return nil, err
		}
		if ticketID != nil {
			e.TicketID = *ticketID
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (s *dispatchStore) getWorkerEmploymentTerm(ctx context.Context, tenantID, workerID string) (*workforceEmploymentTerm, error) {
	row := s.pool.QueryRow(ctx, `SELECT role, compensation_band
		FROM employment_terms WHERE tenant_id=$1 AND worker_id=$2 ORDER BY effective_date DESC LIMIT 1`, tenantID, workerID)
	var t workforceEmploymentTerm
	var compBand *string
	if err := row.Scan(&t.Role, &compBand); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if compBand != nil {
		t.CompensationBand = *compBand
	}
	return &t, nil
}

func nullTimeP(t *time.Time) *time.Time {
	if t == nil || t.IsZero() {
		return nil
	}
	return t
}
