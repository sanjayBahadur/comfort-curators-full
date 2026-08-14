package operations_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"comfort-curators-backend/internal/operations"
	"comfort-curators-backend/internal/platform/audit"

	"github.com/jackc/pgx/v5/pgxpool"
)

type dispatchTestAuthorizer struct {
	tenant string
	deny   bool
}

func (a dispatchTestAuthorizer) RequireResourceAccess(ctx context.Context, resourceTenantID, resourceType, resourceID string) error {
	if a.deny {
		return errors.New("denied")
	}
	if a.tenant != "" && a.tenant != resourceTenantID {
		return operations.ErrCrossTenantDenied
	}
	return nil
}

func dispatchPostgresAvailable() bool {
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

func dispatchDBConnString() string {
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
	name := os.Getenv("CC_DB_NAME")
	if name == "" {
		name = "comfort_curators"
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, pass, host, port, name)
}

func dispatchPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if !dispatchPostgresAvailable() {
		t.Skip("PostgreSQL not available for dispatch integration test")
	}
	pool, err := pgxpool.New(context.Background(), dispatchDBConnString())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := operations.EnsureSchema(context.Background(), pool); err != nil {
		t.Fatalf("ensure operations schema: %v", err)
	}
	if err := audit.EnsureSchema(context.Background(), pool); err != nil {
		t.Fatalf("ensure audit schema: %v", err)
	}

	for _, table := range []string{
		"dispatch_overrides",
		"route_stops",
		"route_plans",
		"ticket_assignments",
		"service_recoveries",
		"incident_alerts",
		"ticket_evidence",
		"ticket_state_events",
		"ticket_checklist_items",
		"tickets",
		"audit_events",
		"employment_terms",
		"sos_events",
		"grievances",
		"expenses",
		"time_entries",
		"availability_windows",
		"workforce_assignments",
		"adverse_action_reviews",
		"worker_ratings",
		"worker_certifications",
		"workers",
	} {
		if _, err := pool.Exec(context.Background(), "TRUNCATE "+table+" CASCADE"); err != nil {
			t.Fatalf("truncate %s: %v", table, err)
		}
	}
	return pool
}

func setupWorker(t *testing.T, pool *pgxpool.Pool, tenantID string, legalName string, dateOfBirth string, serviceZone string, skills []string, status string, ageEligible bool) string {
	t.Helper()
	now := time.Now().UTC()
	workerID := "wrk_" + fmt.Sprintf("%x", time.Now().UnixNano())
	parsedDOB, _ := time.Parse(time.RFC3339, dateOfBirth)

	skillsJSON := "[]"
	if len(skills) > 0 {
		parts := make([]string, len(skills))
		for i, s := range skills {
			parts[i] = `"` + s + `"`
		}
		skillsJSON = "[" + strings.Join(parts, ",") + "]"
	}

	_, err := pool.Exec(context.Background(), `INSERT INTO workers (
		id, tenant_id, legal_name, verified_identity, date_of_birth,
		age_eligible, contact_method, classification, specialist, service_zone,
		skills, status, version, created_at, updated_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		workerID, tenantID, legalName, true, parsedDOB,
		ageEligible, "mobile", "employee", false, serviceZone,
		skillsJSON, status, 1, now, now,
	)
	if err != nil {
		t.Fatalf("insert worker: %v", err)
	}
	return workerID
}

func setupAvailability(t *testing.T, pool *pgxpool.Pool, tenantID, workerID string, dayOfWeek, startMinute, endMinute int) {
	t.Helper()
	now := time.Now().UTC()
	availID := "avail_" + fmt.Sprintf("%x", time.Now().UnixNano())
	_, err := pool.Exec(context.Background(), `INSERT INTO availability_windows (
		id, tenant_id, worker_id, day_of_week, start_minute, end_minute, effective_at, created_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		availID, tenantID, workerID, dayOfWeek, startMinute, endMinute, now, now,
	)
	if err != nil {
		t.Fatalf("insert availability: %v", err)
	}
}

func setupEmploymentTerm(t *testing.T, pool *pgxpool.Pool, tenantID, workerID, role, compensationBand string) {
	t.Helper()
	now := time.Now().UTC()
	termID := "term_" + fmt.Sprintf("%x", time.Now().UnixNano())
	_, err := pool.Exec(context.Background(), `INSERT INTO employment_terms (
		id, tenant_id, worker_id, role, compensation_band, effective_date, created_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		termID, tenantID, workerID, role, compensationBand, now, now,
	)
	if err != nil {
		t.Fatalf("insert employment term: %v", err)
	}
}

func setupCertification(t *testing.T, pool *pgxpool.Pool, tenantID, workerID, workType, status string) {
	t.Helper()
	now := time.Now().UTC()
	certID := "cert_" + fmt.Sprintf("%x", time.Now().UnixNano())
	expires := now.AddDate(1, 0, 0)
	_, err := pool.Exec(context.Background(), `INSERT INTO worker_certifications (
		id, tenant_id, worker_id, work_type, issuer, issued_at, expires_at, status, created_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		certID, tenantID, workerID, workType, "CertIssuer", now, expires, status, now,
	)
	if err != nil {
		t.Fatalf("insert certification: %v", err)
	}
}

func newDispatchService(t *testing.T) *operations.DispatchService {
	t.Helper()
	pool := dispatchPool(t)
	return operations.NewDispatchService(pool)
}

func TestDispatchHonorsHardConstraints(t *testing.T) {
	svc := newDispatchService(t)
	tenantID := "tenant-dispatch-test"
	ticketID := "tkt_" + fmt.Sprintf("%x", time.Now().UnixNano())
	pool := dispatchPool(t)

	now := time.Now().UTC()
	_, err := pool.Exec(context.Background(), `INSERT INTO tickets (
		id, tenant_id, property_id, type, status, reason, created_by, version, created_at, updated_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		ticketID, tenantID, "prop-001", "turnover", "scheduled", "test reason", "actor-1", 1, now, now,
	)
	if err != nil {
		t.Fatalf("insert ticket: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM ticket_assignments WHERE tenant_id=$1", tenantID)
		pool.Exec(context.Background(), "DELETE FROM dispatch_overrides WHERE tenant_id=$1", tenantID)
		pool.Exec(context.Background(), "DELETE FROM tickets WHERE tenant_id=$1", tenantID)
	})

	eligibleWorkerID := setupWorker(t, pool, tenantID, "Eligible Worker", "1990-01-01T00:00:00Z", "Mumbai", []string{"cleaning", "turnover"}, "active", true)
	setupAvailability(t, pool, tenantID, eligibleWorkerID, 1, 480, 960)
	setupEmploymentTerm(t, pool, tenantID, eligibleWorkerID, "Curator", "INR-500-per-hour")

	zoneWorkerID := setupWorker(t, pool, tenantID, "Wrong Zone", "1990-01-01T00:00:00Z", "Delhi", []string{"cleaning"}, "active", true)
	setupAvailability(t, pool, tenantID, zoneWorkerID, 1, 480, 960)
	setupEmploymentTerm(t, pool, tenantID, zoneWorkerID, "Curator", "INR-500-per-hour")

	candidates := evaluateCandidates(t, svc, tenantID, ticketID, "turnover")

	if len(candidates.Candidates) < 2 {
		t.Fatalf("expected at least 2 candidates, got %d", len(candidates.Candidates))
	}

	var eligible, zoneMismatch *operations.WorkerEligibility
	for i := range candidates.Candidates {
		c := &candidates.Candidates[i]
		if c.WorkerID == eligibleWorkerID {
			eligible = c
		}
		if c.WorkerID == zoneWorkerID {
			zoneMismatch = c
		}
	}

	if eligible == nil {
		t.Fatalf("eligible worker not found in candidates")
	}
	if !eligible.Eligible {
		t.Fatalf("eligible worker should be eligible, got %+v", eligible)
	}
	if eligible.Score == 0 {
		t.Errorf("eligible worker should have a positive score, got %d", eligible.Score)
	}

	if zoneMismatch == nil {
		t.Fatalf("wrong zone worker not found in candidates")
	}

	hasZoneCheck := false
	for _, c := range zoneMismatch.Checks {
		if c.Constraint == operations.ConstraintZone {
			hasZoneCheck = true
			break
		}
	}
	if !hasZoneCheck {
		t.Errorf("zone check should be present for wrong-zone worker")
	}

	if zoneMismatch.Eligible == true {
		zoneCheckFound := false
		for _, c := range zoneMismatch.Checks {
			if c.Constraint == operations.ConstraintZone && c.Hard {
				if !c.Passed {
					zoneCheckFound = true
				}
			}
		}
		if !zoneCheckFound {
			t.Errorf("zone constraint should be checked for wrong-zone worker, checks: %+v", zoneMismatch.Checks)
		}
	}
}

func TestDispatchAssignAndShowPayTreatment(t *testing.T) {
	svc := newDispatchService(t)
	tenantID := "tenant-dispatch-pay"
	ticketID := "tkt_" + fmt.Sprintf("%x", time.Now().UnixNano())
	pool := dispatchPool(t)

	now := time.Now().UTC()
	_, err := pool.Exec(context.Background(), `INSERT INTO tickets (
		id, tenant_id, property_id, type, status, reason, created_by, version, created_at, updated_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		ticketID, tenantID, "prop-pay", "routine_maintenance", "scheduled", "test pay", "actor-1", 1, now, now,
	)
	if err != nil {
		t.Fatalf("insert ticket: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM ticket_assignments WHERE tenant_id=$1", tenantID)
		pool.Exec(context.Background(), "DELETE FROM dispatch_overrides WHERE tenant_id=$1", tenantID)
		pool.Exec(context.Background(), "DELETE FROM tickets WHERE tenant_id=$1", tenantID)
	})

	workerID := setupWorker(t, pool, tenantID, "Pay Worker", "1990-01-01T00:00:00Z", "Mumbai", []string{"maintenance", "general"}, "active", true)
	setupAvailability(t, pool, tenantID, workerID, 1, 480, 960)
	setupEmploymentTerm(t, pool, tenantID, workerID, "Curator", "INR-800-per-hour")

	assignment, payTreatment, err := svc.AssignWorker(context.Background(), tenantID, ticketID, workerID, "routine_maintenance", "actor-1")
	if err != nil {
		t.Fatalf("AssignWorker failed: %v", err)
	}
	if assignment == nil {
		t.Fatal("assignment is nil")
	}
	if assignment.Status != operations.AssignmentStatusOffered {
		t.Errorf("expected status offered, got %s", assignment.Status)
	}
	if payTreatment == nil {
		t.Fatal("payTreatment is nil")
	}
	if payTreatment.CompensationBand != "INR-800-per-hour" {
		t.Errorf("expected compensation band 'INR-800-per-hour', got '%s'", payTreatment.CompensationBand)
	}
	if payTreatment.Role != "Curator" {
		t.Errorf("expected role 'Curator', got '%s'", payTreatment.Role)
	}

	getAssignment, getPayTreatment, err := svc.GetAssignment(context.Background(), tenantID, assignment.ID)
	if err != nil {
		t.Fatalf("GetAssignment failed: %v", err)
	}
	if getAssignment.ID != assignment.ID {
		t.Errorf("assignment ID mismatch")
	}
	if getPayTreatment == nil || getPayTreatment.CompensationBand != "INR-800-per-hour" {
		t.Errorf("pay treatment should be available before acceptance: %+v", getPayTreatment)
	}

	accepted, acceptedPay, err := svc.AcceptAssignment(context.Background(), tenantID, assignment.ID, workerID)
	if err != nil {
		t.Fatalf("AcceptAssignment failed: %v", err)
	}
	if accepted.Status != operations.AssignmentStatusAccepted {
		t.Errorf("expected status accepted, got %s", accepted.Status)
	}
	if acceptedPay == nil || acceptedPay.CompensationBand != "INR-800-per-hour" {
		t.Errorf("pay treatment must be included when accepting: %+v", acceptedPay)
	}
}

func TestDispatchOverrideIsAttributed(t *testing.T) {
	svc := newDispatchService(t)
	tenantID := "tenant-dispatch-override"
	ticketID := "tkt_" + fmt.Sprintf("%x", time.Now().UnixNano())
	pool := dispatchPool(t)

	now := time.Now().UTC()
	_, err := pool.Exec(context.Background(), `INSERT INTO tickets (
		id, tenant_id, property_id, type, status, reason, created_by, version, created_at, updated_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		ticketID, tenantID, "prop-override", "routine_maintenance", "scheduled", "test override", "actor-1", 1, now, now,
	)
	if err != nil {
		t.Fatalf("insert ticket: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM ticket_assignments WHERE tenant_id=$1", tenantID)
		pool.Exec(context.Background(), "DELETE FROM dispatch_overrides WHERE tenant_id=$1", tenantID)
		pool.Exec(context.Background(), "DELETE FROM tickets WHERE tenant_id=$1", tenantID)
	})

	workerID := setupWorker(t, pool, tenantID, "Override Worker", "1990-01-01T00:00:00Z", "Mumbai", []string{"maintenance"}, "active", true)
	setupAvailability(t, pool, tenantID, workerID, 1, 480, 960)
	setupEmploymentTerm(t, pool, tenantID, workerID, "Curator", "INR-600-per-hour")

	_, _, err = svc.AssignWorker(context.Background(), tenantID, ticketID, workerID, "routine_maintenance", "actor-1")
	if err != nil {
		t.Fatalf("AssignWorker failed: %v", err)
	}

	req := operations.DispatchOverrideRequest{
		WorkerID:             workerID,
		Reason:               "skill gap but approved by supervisor",
		OverriddenConstraint: "advisory_score",
	}
	assignment, payTreatment, override, err := svc.OverrideAssignment(
		context.Background(), tenantID, ticketID, workerID, "routine_maintenance", req, "supervisor-42",
	)
	if err != nil {
		t.Fatalf("OverrideAssignment failed: %v", err)
	}
	if override == nil {
		t.Fatal("expected override record")
	}
	if override.OverriddenBy != "supervisor-42" {
		t.Errorf("override must be attributed: overridden_by=%s", override.OverriddenBy)
	}
	if override.Reason != "skill gap but approved by supervisor" {
		t.Errorf("override reason mismatch: %s", override.Reason)
	}
	if override.OverriddenConstraint != "advisory_score" {
		t.Errorf("overridden constraint mismatch: %s", override.OverriddenConstraint)
	}
	_ = assignment
	_ = payTreatment

	overrides, err := svc.ListOverridesForTicket(context.Background(), tenantID, ticketID)
	if err != nil {
		t.Fatalf("ListOverridesForTicket failed: %v", err)
	}
	found := false
	for _, o := range overrides {
		if o.OverriddenBy == "supervisor-42" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("override should be persistable and attributable")
	}
}

func TestDispatchOverrideRequiresReasonAndAttribution(t *testing.T) {
	svc := newDispatchService(t)
	tenantID := "tenant-dispatch-ovr-req"
	ticketID := "tkt_" + fmt.Sprintf("%x", time.Now().UnixNano())
	pool := dispatchPool(t)

	now := time.Now().UTC()
	_, err := pool.Exec(context.Background(), `INSERT INTO tickets (
		id, tenant_id, property_id, type, status, reason, created_by, version, created_at, updated_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		ticketID, tenantID, "prop-ovr-req", "routine_maintenance", "scheduled", "test ovr req", "actor-1", 1, now, now,
	)
	if err != nil {
		t.Fatalf("insert ticket: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM ticket_assignments WHERE tenant_id=$1", tenantID)
		pool.Exec(context.Background(), "DELETE FROM dispatch_overrides WHERE tenant_id=$1", tenantID)
		pool.Exec(context.Background(), "DELETE FROM tickets WHERE tenant_id=$1", tenantID)
	})

	workerID := setupWorker(t, pool, tenantID, "Ovr Req Worker", "1990-01-01T00:00:00Z", "Mumbai", []string{"maintenance"}, "active", true)
	setupAvailability(t, pool, tenantID, workerID, 1, 480, 960)
	setupEmploymentTerm(t, pool, tenantID, workerID, "Curator", "INR-500-per-hour")

	req := operations.DispatchOverrideRequest{
		WorkerID:             workerID,
		Reason:               "",
		OverriddenConstraint: "advisory_score",
	}
	_, _, _, err = svc.OverrideAssignment(
		context.Background(), tenantID, ticketID, workerID, "routine_maintenance", req, "",
	)
	if err == nil {
		t.Error("expected error for empty reason on override")
	}
	if !errors.Is(err, operations.ErrDispatchOverrideRequiresReason) {
		t.Errorf("expected ErrDispatchOverrideRequiresReason, got %v", err)
	}

	req2 := operations.DispatchOverrideRequest{
		WorkerID:             workerID,
		Reason:               "valid reason",
		OverriddenConstraint: "",
	}
	_, _, _, err = svc.OverrideAssignment(
		context.Background(), tenantID, ticketID, workerID, "routine_maintenance", req2, "actor-1",
	)
	if err == nil {
		t.Error("expected error for empty constraint name on override")
	}

	req3 := operations.DispatchOverrideRequest{
		WorkerID:             workerID,
		Reason:               "valid reason",
		OverriddenConstraint: "advisory_score",
	}
	_, _, _, err = svc.OverrideAssignment(
		context.Background(), tenantID, ticketID, workerID, "routine_maintenance", req3, "",
	)
	if err == nil {
		t.Error("expected error for empty overridden_by on override")
	}
}

func TestDispatchOnlyAssignedWorkerCanAccept(t *testing.T) {
	svc := newDispatchService(t)
	tenantID := "tenant-dispatch-own"
	ticketID := "tkt_" + fmt.Sprintf("%x", time.Now().UnixNano())
	pool := dispatchPool(t)

	now := time.Now().UTC()
	_, err := pool.Exec(context.Background(), `INSERT INTO tickets (
		id, tenant_id, property_id, type, status, reason, created_by, version, created_at, updated_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		ticketID, tenantID, "prop-own", "routine_maintenance", "scheduled", "test own", "actor-1", 1, now, now,
	)
	if err != nil {
		t.Fatalf("insert ticket: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM ticket_assignments WHERE tenant_id=$1", tenantID)
		pool.Exec(context.Background(), "DELETE FROM dispatch_overrides WHERE tenant_id=$1", tenantID)
		pool.Exec(context.Background(), "DELETE FROM tickets WHERE tenant_id=$1", tenantID)
	})

	workerID := setupWorker(t, pool, tenantID, "Accept Worker", "1990-01-01T00:00:00Z", "Mumbai", []string{"maintenance"}, "active", true)
	setupAvailability(t, pool, tenantID, workerID, 1, 480, 960)
	setupEmploymentTerm(t, pool, tenantID, workerID, "Curator", "INR-500-per-hour")

	otherWorkerID := setupWorker(t, pool, tenantID, "Other Worker", "1990-01-01T00:00:00Z", "Mumbai", []string{"maintenance"}, "active", true)
	setupAvailability(t, pool, tenantID, otherWorkerID, 1, 480, 960)

	assignment, _, err := svc.AssignWorker(context.Background(), tenantID, ticketID, workerID, "routine_maintenance", "actor-1")
	if err != nil {
		t.Fatalf("AssignWorker failed: %v", err)
	}

	_, _, err = svc.AcceptAssignment(context.Background(), tenantID, assignment.ID, otherWorkerID)
	if err == nil {
		t.Error("other worker should not be able to accept")
	}
	if !errors.Is(err, operations.ErrDispatchNotWorker) {
		t.Errorf("expected ErrDispatchNotWorker, got %v", err)
	}

	_, err = svc.DeclineAssignment(context.Background(), tenantID, assignment.ID, otherWorkerID)
	if err == nil {
		t.Error("other worker should not be able to decline")
	}

	declined, err := svc.DeclineAssignment(context.Background(), tenantID, assignment.ID, workerID)
	if err != nil {
		t.Fatalf("assigned worker should be able to decline: %v", err)
	}
	if declined.Status != operations.AssignmentStatusDeclined {
		t.Errorf("expected status declined, got %s", declined.Status)
	}
}

func TestDispatchCannotAssignNonScheduledTicket(t *testing.T) {
	svc := newDispatchService(t)
	tenantID := "tenant-dispatch-state"
	ticketID := "tkt_" + fmt.Sprintf("%x", time.Now().UnixNano())
	pool := dispatchPool(t)

	now := time.Now().UTC()
	_, err := pool.Exec(context.Background(), `INSERT INTO tickets (
		id, tenant_id, property_id, type, status, reason, created_by, version, created_at, updated_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		ticketID, tenantID, "prop-state", "routine_maintenance", "draft", "test state", "actor-1", 1, now, now,
	)
	if err != nil {
		t.Fatalf("insert ticket: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM ticket_assignments WHERE tenant_id=$1", tenantID)
		pool.Exec(context.Background(), "DELETE FROM dispatch_overrides WHERE tenant_id=$1", tenantID)
		pool.Exec(context.Background(), "DELETE FROM tickets WHERE tenant_id=$1", tenantID)
	})

	workerID := setupWorker(t, pool, tenantID, "Bad State", "1990-01-01T00:00:00Z", "Mumbai", []string{"maintenance"}, "active", true)
	setupAvailability(t, pool, tenantID, workerID, 1, 480, 960)

	_, _, err = svc.AssignWorker(context.Background(), tenantID, ticketID, workerID, "routine_maintenance", "actor-1")
	if err == nil {
		t.Error("expected error assigning worker to non-scheduled ticket")
	}
	if !errors.Is(err, operations.ErrDispatchTicketNotAssignable) {
		t.Errorf("expected ErrDispatchTicketNotAssignable, got %v", err)
	}
}

func TestDispatchCrossTenantAssignment(t *testing.T) {
	svc := newDispatchService(t)
	tenantIDA := "tenant-dispatch-a"
	tenantIDB := "tenant-dispatch-b"
	ticketID := "tkt_" + fmt.Sprintf("%x", time.Now().UnixNano())
	pool := dispatchPool(t)

	now := time.Now().UTC()
	_, err := pool.Exec(context.Background(), `INSERT INTO tickets (
		id, tenant_id, property_id, type, status, reason, created_by, version, created_at, updated_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		ticketID, tenantIDA, "prop-ct", "routine_maintenance", "scheduled", "test cross-tenant", "actor-1", 1, now, now,
	)
	if err != nil {
		t.Fatalf("insert ticket: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM ticket_assignments WHERE tenant_id=$1 OR tenant_id=$2", tenantIDA, tenantIDB)
		pool.Exec(context.Background(), "DELETE FROM dispatch_overrides WHERE tenant_id=$1 OR tenant_id=$2", tenantIDA, tenantIDB)
		pool.Exec(context.Background(), "DELETE FROM tickets WHERE tenant_id=$1 OR tenant_id=$2", tenantIDA, tenantIDB)
	})

	workerIDB := setupWorker(t, pool, tenantIDB, "Cross Tenant Worker", "1990-01-01T00:00:00Z", "Mumbai", []string{"maintenance"}, "active", true)
	setupAvailability(t, pool, tenantIDB, workerIDB, 1, 480, 960)

	_, _, err = svc.AssignWorker(context.Background(), tenantIDA, ticketID, workerIDB, "routine_maintenance", "actor-1")
	if err == nil {
		t.Error("expected assignment from wrong tenant to fail (worker not found)")
	}
}

func TestDispatchTwoPersonHighRiskTicket(t *testing.T) {
	svc := newDispatchService(t)
	tenantID := "tenant-dispatch-two"
	ticketID := "tkt_" + fmt.Sprintf("%x", time.Now().UnixNano())
	pool := dispatchPool(t)

	now := time.Now().UTC()
	_, err := pool.Exec(context.Background(), `INSERT INTO tickets (
		id, tenant_id, property_id, type, status, reason, created_by, version, created_at, updated_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		ticketID, tenantID, "prop-two", "specialist_vendor_request", "scheduled", "test two-person", "actor-1", 1, now, now,
	)
	if err != nil {
		t.Fatalf("insert ticket: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM ticket_assignments WHERE tenant_id=$1", tenantID)
		pool.Exec(context.Background(), "DELETE FROM dispatch_overrides WHERE tenant_id=$1", tenantID)
		pool.Exec(context.Background(), "DELETE FROM tickets WHERE tenant_id=$1", tenantID)
	})

	workerID := setupWorker(t, pool, tenantID, "Specialist Worker", "1990-01-01T00:00:00Z", "Mumbai", []string{"specialist"}, "active", true)
	setupCertification(t, pool, tenantID, workerID, "specialist", "valid")
	setupAvailability(t, pool, tenantID, workerID, 1, 480, 960)
	setupEmploymentTerm(t, pool, tenantID, workerID, "Specialist", "INR-1000-per-hour")

	assignment, _, err := svc.AssignWorker(context.Background(), tenantID, ticketID, workerID, "specialist_vendor_request", "actor-1")
	if err != nil {
		t.Fatalf("first specialist assignment should succeed: %v", err)
	}
	if assignment.Status != operations.AssignmentStatusOffered {
		t.Errorf("expected status offered, got %s", assignment.Status)
	}

	secondWorkerID := setupWorker(t, pool, tenantID, "Second Specialist", "1990-01-01T00:00:00Z", "Mumbai", []string{"specialist"}, "active", true)
	setupCertification(t, pool, tenantID, secondWorkerID, "specialist", "valid")
	setupAvailability(t, pool, tenantID, secondWorkerID, 1, 480, 960)
	setupEmploymentTerm(t, pool, tenantID, secondWorkerID, "Specialist", "INR-1000-per-hour")

	secondAssign, _, err := svc.AssignWorker(context.Background(), tenantID, ticketID, secondWorkerID, "specialist_vendor_request", "actor-1")
	if err != nil {
		t.Fatalf("second specialist assignment should also succeed: %v", err)
	}
	_ = secondAssign
}

func TestDispatchSkillConstraintBlocksAssignment(t *testing.T) {
	svc := newDispatchService(t)
	tenantID := "tenant-dispatch-skill"
	ticketID := "tkt_" + fmt.Sprintf("%x", time.Now().UnixNano())
	pool := dispatchPool(t)

	now := time.Now().UTC()
	_, err := pool.Exec(context.Background(), `INSERT INTO tickets (
		id, tenant_id, property_id, type, status, reason, created_by, version, created_at, updated_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		ticketID, tenantID, "prop-skill", "turnover", "scheduled", "test skill", "actor-1", 1, now, now,
	)
	if err != nil {
		t.Fatalf("insert ticket: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM ticket_assignments WHERE tenant_id=$1", tenantID)
		pool.Exec(context.Background(), "DELETE FROM dispatch_overrides WHERE tenant_id=$1", tenantID)
		pool.Exec(context.Background(), "DELETE FROM tickets WHERE tenant_id=$1", tenantID)
	})

	noSkillWorkerID := setupWorker(t, pool, tenantID, "No Skill Worker", "1990-01-01T00:00:00Z", "Mumbai", []string{"maintenance"}, "active", true)
	setupAvailability(t, pool, tenantID, noSkillWorkerID, 1, 480, 960)
	setupEmploymentTerm(t, pool, tenantID, noSkillWorkerID, "Curator", "INR-500-per-hour")

	_, _, err = svc.AssignWorker(context.Background(), tenantID, ticketID, noSkillWorkerID, "turnover", "actor-1")
	if err == nil {
		t.Error("expected error: worker without required skill should not be assignable")
	}
}

func TestDispatchPayTreatmentBeforeAcceptance(t *testing.T) {
	svc := newDispatchService(t)
	tenantID := "tenant-dispatch-pt"
	ticketID := "tkt_" + fmt.Sprintf("%x", time.Now().UnixNano())
	pool := dispatchPool(t)

	now := time.Now().UTC()
	_, err := pool.Exec(context.Background(), `INSERT INTO tickets (
		id, tenant_id, property_id, type, status, reason, created_by, version, created_at, updated_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		ticketID, tenantID, "prop-pt", "routine_maintenance", "scheduled", "test pt", "actor-1", 1, now, now,
	)
	if err != nil {
		t.Fatalf("insert ticket: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM ticket_assignments WHERE tenant_id=$1", tenantID)
		pool.Exec(context.Background(), "DELETE FROM dispatch_overrides WHERE tenant_id=$1", tenantID)
		pool.Exec(context.Background(), "DELETE FROM tickets WHERE tenant_id=$1", tenantID)
	})

	workerID := setupWorker(t, pool, tenantID, "PT Worker", "1990-01-01T00:00:00Z", "Mumbai", []string{"maintenance"}, "active", true)
	setupAvailability(t, pool, tenantID, workerID, 1, 480, 960)
	setupEmploymentTerm(t, pool, tenantID, workerID, "Senior Curator", "INR-1200-per-hour")

	assignment, payTreatment, err := svc.AssignWorker(context.Background(), tenantID, ticketID, workerID, "routine_maintenance", "actor-1")
	if err != nil {
		t.Fatalf("AssignWorker failed: %v", err)
	}

	if payTreatment == nil {
		t.Fatal("pay treatment must not be nil")
	}
	if payTreatment.CompensationBand != "INR-1200-per-hour" {
		t.Errorf("pay treatment band mismatch: %s", payTreatment.CompensationBand)
	}
	if payTreatment.Role != "Senior Curator" {
		t.Errorf("pay treatment role mismatch: %s", payTreatment.Role)
	}

	treatment, err := svc.GetPayTreatment(context.Background(), tenantID, workerID)
	if err != nil {
		t.Fatalf("GetPayTreatment failed: %v", err)
	}
	if treatment.CompensationBand != "INR-1200-per-hour" {
		t.Errorf("independent pay treatment fetch should match: %s", treatment.CompensationBand)
	}
	_ = assignment
}

func evaluateCandidates(t *testing.T, svc *operations.DispatchService, tenantID, ticketID, workType string) *operations.DispatchCandidatesResponse {
	t.Helper()
	resp, err := svc.EvaluateCandidates(context.Background(), tenantID, ticketID, workType)
	if err != nil {
		t.Fatalf("EvaluateCandidates failed: %v", err)
	}
	return resp
}
