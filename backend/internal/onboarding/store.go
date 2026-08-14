package onboarding

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
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

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) CreateCase(ctx context.Context, q querier, c *Case) error {
	if c.ID == "" {
		c.ID = newID("case")
	}
	if c.Version == 0 {
		c.Version = 1
	}
	if c.Status == "" {
		c.Status = StatusInProgress
	}
	err := q.QueryRow(ctx, `
		INSERT INTO onboarding_cases (
			id, tenant_id, property_id, owner_authority_id, status,
			version, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
		RETURNING created_at, updated_at
	`,
		c.ID, c.TenantID, c.PropertyID, c.OwnerAuthorityID, string(c.Status), c.Version,
	).Scan(&c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert onboarding case: %w", err)
	}
	return nil
}

func (s *Store) getByID(ctx context.Context, q querier, tenantID, caseID string) (*Case, error) {
	var c Case
	var portfolioJSON, goalsJSON, prefsJSON, budgetsJSON []byte
	var contactsJSON, photosJSON, amenitiesJSON []byte
	var safetyJSON, furnishingJSON, remediationJSON, fitJSON []byte
	err := q.QueryRow(ctx, `
		SELECT id, tenant_id, property_id, owner_authority_id, status,
			portfolio, goals, service_preferences, budgets,
			contacts, photographs, amenities, safety, furnishing,
			remediation, fit_score_inputs,
			version, created_at, updated_at
		FROM onboarding_cases
		WHERE id = $1 AND tenant_id = $2
	`, caseID, tenantID).Scan(
		&c.ID, &c.TenantID, &c.PropertyID, &c.OwnerAuthorityID, &c.Status,
		&portfolioJSON, &goalsJSON, &prefsJSON, &budgetsJSON,
		&contactsJSON, &photosJSON, &amenitiesJSON, &safetyJSON, &furnishingJSON,
		&remediationJSON, &fitJSON,
		&c.Version, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrCaseNotFound
		}
		return nil, fmt.Errorf("get onboarding case: %w", err)
	}
	if len(portfolioJSON) > 0 {
		var v Portfolio
		if err := json.Unmarshal(portfolioJSON, &v); err != nil {
			return nil, fmt.Errorf("decode portfolio: %w", err)
		}
		c.Portfolio = &v
	}
	if len(goalsJSON) > 0 {
		var v Goals
		if err := json.Unmarshal(goalsJSON, &v); err != nil {
			return nil, fmt.Errorf("decode goals: %w", err)
		}
		c.Goals = &v
	}
	if len(prefsJSON) > 0 {
		var v ServicePreferences
		if err := json.Unmarshal(prefsJSON, &v); err != nil {
			return nil, fmt.Errorf("decode service preferences: %w", err)
		}
		c.ServicePreferences = &v
	}
	if len(budgetsJSON) > 0 {
		var v Budgets
		if err := json.Unmarshal(budgetsJSON, &v); err != nil {
			return nil, fmt.Errorf("decode budgets: %w", err)
		}
		c.Budgets = &v
	}
	if len(contactsJSON) > 0 {
		if err := json.Unmarshal(contactsJSON, &c.Contacts); err != nil {
			return nil, fmt.Errorf("decode contacts: %w", err)
		}
	}
	if len(photosJSON) > 0 {
		if err := json.Unmarshal(photosJSON, &c.Photographs); err != nil {
			return nil, fmt.Errorf("decode photographs: %w", err)
		}
	}
	if len(amenitiesJSON) > 0 {
		if err := json.Unmarshal(amenitiesJSON, &c.Amenities); err != nil {
			return nil, fmt.Errorf("decode amenities: %w", err)
		}
	}
	if len(safetyJSON) > 0 {
		var v Safety
		if err := json.Unmarshal(safetyJSON, &v); err != nil {
			return nil, fmt.Errorf("decode safety: %w", err)
		}
		c.Safety = &v
	}
	if len(furnishingJSON) > 0 {
		var v Furnishing
		if err := json.Unmarshal(furnishingJSON, &v); err != nil {
			return nil, fmt.Errorf("decode furnishing: %w", err)
		}
		c.Furnishing = &v
	}
	if len(remediationJSON) > 0 {
		var v Remediation
		if err := json.Unmarshal(remediationJSON, &v); err != nil {
			return nil, fmt.Errorf("decode remediation: %w", err)
		}
		c.Remediation = &v
	}
	if len(fitJSON) > 0 {
		var v FitScoreInputs
		if err := json.Unmarshal(fitJSON, &v); err != nil {
			return nil, fmt.Errorf("decode fit score inputs: %w", err)
		}
		c.FitScoreInputs = &v
	}
	// Evidence and inspection records are part of the aggregate: status
	// recomputation and activation gates read them, so every load returns the
	// full aggregate with its children.
	if c.Evidence, err = s.listEvidence(ctx, q, tenantID, caseID); err != nil {
		return nil, err
	}
	if c.Inspections, err = s.listInspections(ctx, q, tenantID, caseID); err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Store) GetCase(ctx context.Context, tenantID, caseID string) (*Case, error) {
	c, err := s.getByID(ctx, s.pool, tenantID, caseID)
	if err != nil {
		return nil, err
	}
	c.Holds = c.ActivationHolds(time.Now().UTC())
	return c, nil
}

func (s *Store) ListCases(ctx context.Context, tenantID string) ([]Case, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, property_id, owner_authority_id, status,
			version, created_at, updated_at
		FROM onboarding_cases
		WHERE tenant_id = $1
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list onboarding cases: %w", err)
	}
	defer rows.Close()

	var cases []Case
	for rows.Next() {
		var c Case
		if err := rows.Scan(
			&c.ID, &c.TenantID, &c.PropertyID, &c.OwnerAuthorityID, &c.Status,
			&c.Version, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan onboarding case: %w", err)
		}
		cases = append(cases, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list onboarding cases rows: %w", err)
	}
	return cases, nil
}

// updateCaseSections persists the given JSONB section columns and bumps the
// aggregate version using an optimistic concurrency check. The status is
// updated from the in-memory aggregate so a completed checklist is reflected
// immediately.
func (s *Store) updateCaseSections(ctx context.Context, q querier, c *Case, columns map[string]json.RawMessage) error {
	if len(columns) == 0 {
		return nil
	}
	setParts := make([]string, 0, len(columns)+1)
	args := make([]any, 0, len(columns)+4)
	arg := 1
	for _, name := range orderedColumns(columns) {
		setParts = append(setParts, fmt.Sprintf("%s = $%d", name, arg))
		args = append(args, columns[name])
		arg++
	}
	setParts = append(setParts, fmt.Sprintf("status = $%d", arg), fmt.Sprintf("version = $%d", arg+1), "updated_at = NOW()")
	args = append(args, string(c.Status), c.Version)
	args = append(args, c.ID, c.TenantID, c.Version-1)
	tag, err := q.Exec(ctx, fmt.Sprintf(`
		UPDATE onboarding_cases
		SET %s
		WHERE id = $%d AND tenant_id = $%d AND version = $%d
	`, strings.Join(setParts, ", "), arg+2, arg+3, arg+4), args...)
	if err != nil {
		return fmt.Errorf("update onboarding case sections: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("onboarding case update lost a concurrent write (optimistic version)")
	}
	return nil
}

// updateCaseStatus persists only the aggregate status and version.
func (s *Store) updateCaseStatus(ctx context.Context, q querier, c *Case) error {
	tag, err := q.Exec(ctx, `
		UPDATE onboarding_cases
		SET status = $3, version = $4, updated_at = NOW()
		WHERE id = $1 AND tenant_id = $2 AND version = $5
	`, c.ID, c.TenantID, string(c.Status), c.Version, c.Version-1)
	if err != nil {
		return fmt.Errorf("update onboarding case status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("onboarding case status update lost a concurrent write (optimistic version)")
	}
	return nil
}

// InsertEvidence appends one immutable evidence capture. It runs inside the
// caller's transaction so the evidence row and the case status change commit
// atomically.
func (s *Store) InsertEvidence(ctx context.Context, q querier, e *Evidence) error {
	if e.ID == "" {
		e.ID = newID("evid")
	}
	if e.CapturedAt.IsZero() {
		e.CapturedAt = time.Now().UTC()
	}
	_, err := q.Exec(ctx, `
		INSERT INTO onboarding_evidence (
			id, case_id, tenant_id, kind, content_hash, object_ref, captured_by, captured_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, e.ID, e.CaseID, e.TenantID, e.Kind, e.ContentHash, e.ObjectRef, e.CapturedBy, e.CapturedAt)
	if err != nil {
		return fmt.Errorf("insert onboarding evidence: %w", err)
	}
	return nil
}

func (s *Store) listEvidence(ctx context.Context, q querier, tenantID, caseID string) ([]Evidence, error) {
	rows, err := q.Query(ctx, `
		SELECT id, case_id, tenant_id, kind, content_hash, object_ref, captured_by, captured_at
		FROM onboarding_evidence
		WHERE case_id = $1 AND tenant_id = $2
		ORDER BY captured_at ASC
	`, caseID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list onboarding evidence: %w", err)
	}
	defer rows.Close()

	var evidence []Evidence
	for rows.Next() {
		var e Evidence
		if err := rows.Scan(
			&e.ID, &e.CaseID, &e.TenantID, &e.Kind, &e.ContentHash, &e.ObjectRef,
			&e.CapturedBy, &e.CapturedAt,
		); err != nil {
			return nil, fmt.Errorf("scan onboarding evidence: %w", err)
		}
		evidence = append(evidence, e)
	}
	return evidence, rows.Err()
}

// InsertInspection persists a new inspection record inside the caller's
// transaction. There is no update or delete operation for inspection records
// anywhere in the store; the schema trigger additionally rejects UPDATE and
// DELETE at the database.
func (s *Store) InsertInspection(ctx context.Context, q querier, i *Inspection) error {
	if i.ID == "" {
		i.ID = newID("insp")
	}
	if i.CreatedAt.IsZero() {
		i.CreatedAt = time.Now().UTC()
	}
	_, err := q.Exec(ctx, `
		INSERT INTO onboarding_inspections (
			id, case_id, tenant_id, property_id, performed_at, inspected_by,
			evidence_hash, evidence_ref, findings, overall_status, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, i.ID, i.CaseID, i.TenantID, i.PropertyID, i.PerformedAt, i.InspectedBy,
		i.EvidenceHash, i.EvidenceRef, i.Findings, i.OverallStatus, i.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert onboarding inspection: %w", err)
	}
	return nil
}

func (s *Store) listInspections(ctx context.Context, q querier, tenantID, caseID string) ([]Inspection, error) {
	rows, err := q.Query(ctx, `
		SELECT id, case_id, tenant_id, property_id, performed_at, inspected_by,
			evidence_hash, evidence_ref, findings, overall_status, created_at
		FROM onboarding_inspections
		WHERE case_id = $1 AND tenant_id = $2
		ORDER BY performed_at ASC
	`, caseID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list onboarding inspections: %w", err)
	}
	defer rows.Close()

	var inspections []Inspection
	for rows.Next() {
		var i Inspection
		if err := rows.Scan(
			&i.ID, &i.CaseID, &i.TenantID, &i.PropertyID, &i.PerformedAt, &i.InspectedBy,
			&i.EvidenceHash, &i.EvidenceRef, &i.Findings, &i.OverallStatus, &i.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan onboarding inspection: %w", err)
		}
		inspections = append(inspections, i)
	}
	return inspections, rows.Err()
}

// orderedColumns returns a deterministic column order for building a SET
// clause so the generated SQL is stable.
func orderedColumns(m map[string]json.RawMessage) []string {
	cols := make([]string, 0, len(m))
	for name := range m {
		cols = append(cols, name)
	}
	for i := 1; i < len(cols); i++ {
		for j := i; j > 0 && cols[j] < cols[j-1]; j-- {
			cols[j], cols[j-1] = cols[j-1], cols[j]
		}
	}
	return cols
}
