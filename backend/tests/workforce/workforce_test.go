package workforce_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"comfort-curators-backend/internal/platform/audit"
	"comfort-curators-backend/internal/workforce"

	"github.com/jackc/pgx/v5/pgxpool"
)

func workforcePostgresAvailable() bool {
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

func workforceDBConnString() string {
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

func workforcePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if !workforcePostgresAvailable() {
		t.Skip("PostgreSQL not available for workforce integration test")
	}
	pool, err := pgxpool.New(context.Background(), workforceDBConnString())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := workforce.EnsureSchema(context.Background(), pool); err != nil {
		t.Fatalf("ensure workforce schema: %v", err)
	}
	if err := audit.EnsureSchema(context.Background(), pool); err != nil {
		t.Fatalf("ensure audit schema: %v", err)
	}
	for _, table := range []string{
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
		"audit_events",
	} {
		if _, err := pool.Exec(context.Background(), "TRUNCATE "+table+" CASCADE"); err != nil {
			t.Fatalf("truncate %s: %v", table, err)
		}
	}
	return pool
}

func newWorkforceService(t *testing.T) *workforce.WorkforceService {
	t.Helper()
	pool := workforcePool(t)
	return workforce.NewWorkforceService(pool).WithAudit(audit.NewAuditStore(pool))
}

func dateAgo(years, months, days int) time.Time {
	return time.Now().UTC().AddDate(-years, -months, -days)
}

func createWorker(t *testing.T, svc *workforce.WorkforceService, tenantID string, classification string, dob time.Time) *workforce.Worker {
	t.Helper()
	w, err := svc.CreateWorker(context.Background(), workforce.CreateWorkerParams{
		TenantID:         tenantID,
		LegalName:        "Asha Verma",
		VerifiedIdentity: true,
		DateOfBirth:      dob,
		ContactMethod:    "+91-9000000000",
		Classification:   classification,
		ServiceZone:      "south-delhi",
		Skills:           []string{"cleaning", "hospitality"},
	}, "actor-hr-1")
	if err != nil {
		t.Fatalf("create worker: %v", err)
	}
	return w
}

func TestWorkforceIsAgeEligible(t *testing.T) {
	now := time.Date(2026, time.July, 1, 12, 0, 0, 0, time.UTC)

	exactly18 := time.Date(2008, time.July, 1, 0, 0, 0, 0, time.UTC)
	if !workforce.IsAgeEligible(exactly18, now) {
		t.Fatal("worker who turned 18 today must be age eligible")
	}

	oneDayBefore := time.Date(2008, time.July, 2, 0, 0, 0, 0, time.UTC)
	if workforce.IsAgeEligible(oneDayBefore, now) {
		t.Fatal("worker who turns 18 tomorrow must not be age eligible")
	}

	future := time.Date(2030, time.January, 1, 0, 0, 0, 0, time.UTC)
	if workforce.IsAgeEligible(future, now) {
		t.Fatal("a future date of birth must never be age eligible")
	}
}

func TestWorkforceUnderageOperationsAssignmentFails(t *testing.T) {
	svc := newWorkforceService(t)
	ctx := context.Background()

	minor := createWorker(t, svc, "tenant-wf-age", workforce.ClassificationEmployee, dateAgo(17, 0, 0))
	if minor.AgeEligible {
		t.Fatal("a 17-year-old worker must be recorded as not age eligible")
	}

	if _, err := svc.AssignOperations(ctx, "tenant-wf-age", minor.ID, workforce.WorkHeavyLoad, "actor-ops-1"); !errors.Is(err, workforce.ErrUnderageForOperations) {
		t.Fatalf("under-18 assignment must fail with ErrUnderageForOperations, got %v", err)
	}

	if err := svc.CheckOperationsAssignment(ctx, "tenant-wf-age", minor.ID, "general"); !errors.Is(err, workforce.ErrUnderageForOperations) {
		t.Fatalf("under-18 eligibility check must fail with ErrUnderageForOperations, got %v", err)
	}

	assignments, err := svc.ListAssignments(ctx, "tenant-wf-age", minor.ID)
	if err != nil {
		t.Fatalf("list assignments: %v", err)
	}
	if len(assignments) != 0 {
		t.Fatalf("failed under-18 assignment must not leave a record, got %d", len(assignments))
	}

	adult := createWorker(t, svc, "tenant-wf-age", workforce.ClassificationEmployee, dateAgo(24, 0, 0))
	if !adult.AgeEligible {
		t.Fatal("an adult worker must be age eligible")
	}
	if _, err := svc.AssignOperations(ctx, "tenant-wf-age", adult.ID, "general", "actor-ops-1"); err != nil {
		t.Fatalf("adult general assignment must succeed, got %v", err)
	}
}

func TestWorkforceRatingCannotDeactivateWorker(t *testing.T) {
	svc := newWorkforceService(t)
	ctx := context.Background()

	w := createWorker(t, svc, "tenant-wf-rating", workforce.ClassificationEmployee, dateAgo(30, 0, 0))

	for _, deactivating := range []string{
		workforce.StatusInactive,
		workforce.StatusSuspended,
		workforce.StatusRejected,
		workforce.StatusTerminated,
	} {
		_, err := svc.RecordRating(ctx, "tenant-wf-rating", w.ID, workforce.RatingParams{
			Score:         4,
			Source:        workforce.RatingSourceAI,
			DesiredStatus: deactivating,
		}, "actor-ops-1")
		if !errors.Is(err, workforce.ErrRatingCannotDeactivate) {
			t.Fatalf("rating proposing %q must be rejected with ErrRatingCannotDeactivate, got %v", deactivating, err)
		}
	}

	rating, err := svc.RecordRating(ctx, "tenant-wf-rating", w.ID, workforce.RatingParams{
		Score:   4,
		Source:  workforce.RatingSourceAI,
		Comment: "slow response on one visit",
	}, "actor-ops-1")
	if err != nil {
		t.Fatalf("a plain rating must be recorded, got %v", err)
	}
	if rating.WorkerStatusAfter != workforce.StatusActive {
		t.Fatalf("rating must never change the worker status, got %q", rating.WorkerStatusAfter)
	}

	reloaded, err := svc.GetWorker(ctx, "tenant-wf-rating", w.ID)
	if err != nil {
		t.Fatalf("reload worker: %v", err)
	}
	if reloaded.Status != workforce.StatusActive {
		t.Fatalf("worker status must remain active after ratings, got %q", reloaded.Status)
	}
	if reloaded.Version != 1 {
		t.Fatalf("ratings must not bump the worker version, got %d", reloaded.Version)
	}

	ratings, err := svc.ListRatings(ctx, "tenant-wf-rating", w.ID)
	if err != nil {
		t.Fatalf("list ratings: %v", err)
	}
	if len(ratings) != 1 {
		t.Fatalf("expected exactly one recorded rating, got %d", len(ratings))
	}
}

func TestWorkforceRestrictedWorkRequiresCertificationOrSpecialistVendor(t *testing.T) {
	svc := newWorkforceService(t)
	ctx := context.Background()

	curator := createWorker(t, svc, "tenant-wf-restricted", workforce.ClassificationEmployee, dateAgo(28, 0, 0))

	// Restricted work without certification must fail even for an adult worker.
	if _, err := svc.AssignOperations(ctx, "tenant-wf-restricted", curator.ID, workforce.WorkElectrical, "actor-ops-1"); !errors.Is(err, workforce.ErrRestrictedWorkRequiresCert) {
		t.Fatalf("restricted work without certification must fail with ErrRestrictedWorkRequiresCert, got %v", err)
	}

	// A non-specialist vendor without certification also fails.
	plainVendor := createWorker(t, svc, "tenant-wf-restricted", workforce.ClassificationVendor, dateAgo(40, 0, 0))
	if plainVendor.Specialist {
		t.Fatal("plain vendor must not default to specialist")
	}
	if _, err := svc.AssignOperations(ctx, "tenant-wf-restricted", plainVendor.ID, workforce.WorkGas, "actor-ops-1"); !errors.Is(err, workforce.ErrRestrictedWorkRequiresCert) {
		t.Fatalf("non-specialist vendor without certification must fail, got %v", err)
	}

	// A valid certification for the exact work type satisfies the requirement.
	cert, err := svc.AddCertification(ctx, "tenant-wf-restricted", curator.ID, workforce.CertificationParams{
		WorkType:  workforce.WorkElectrical,
		Issuer:    "BEE-licensed trainer",
		IssuedAt:  dateAgo(0, 6, 0),
		ExpiresAt: dateAgo(-1, 0, 0),
	}, "actor-hr-1")
	if err != nil {
		t.Fatalf("add certification: %v", err)
	}
	if cert.Status != workforce.CertStatusValid {
		t.Fatalf("future certification must be valid, got %q", cert.Status)
	}
	if _, err := svc.AssignOperations(ctx, "tenant-wf-restricted", curator.ID, workforce.WorkElectrical, "actor-ops-1"); err != nil {
		t.Fatalf("certified worker must be assignable to restricted work, got %v", err)
	}

	// A certification for a different restricted work type does not help.
	if _, err := svc.AssignOperations(ctx, "tenant-wf-restricted", curator.ID, workforce.WorkGas, "actor-ops-1"); !errors.Is(err, workforce.ErrRestrictedWorkRequiresCert) {
		t.Fatalf("mismatched certification must not satisfy a restricted work requirement, got %v", err)
	}

	// An expired certification is not explicit current certification.
	expired, err := svc.AddCertification(ctx, "tenant-wf-restricted", curator.ID, workforce.CertificationParams{
		WorkType:  workforce.WorkGas,
		Issuer:    "gas-safety-board",
		IssuedAt:  dateAgo(0, 20, 0),
		ExpiresAt: dateAgo(0, 2, 0),
	}, "actor-hr-1")
	if err != nil {
		t.Fatalf("add expired certification: %v", err)
	}
	if expired.Status != workforce.CertStatusExpired {
		t.Fatalf("past certification must be expired, got %q", expired.Status)
	}
	if _, err := svc.AssignOperations(ctx, "tenant-wf-restricted", curator.ID, workforce.WorkGas, "actor-ops-1"); !errors.Is(err, workforce.ErrRestrictedWorkRequiresCert) {
		t.Fatalf("expired certification must not satisfy restricted work, got %v", err)
	}

	// A specialist vendor may be routed to restricted work without its own
	// certification (WFM-014).
	specialist, err := svc.CreateWorker(ctx, workforce.CreateWorkerParams{
		TenantID:         "tenant-wf-restricted",
		LegalName:        "Precision Pest Control",
		VerifiedIdentity: true,
		DateOfBirth:      dateAgo(42, 0, 0),
		ContactMethod:    "+91-9000000001",
		Classification:   workforce.ClassificationVendor,
		Specialist:       true,
		ServiceZone:      "south-delhi",
	}, "actor-hr-1")
	if err != nil {
		t.Fatalf("create specialist vendor: %v", err)
	}
	if _, err := svc.AssignOperations(ctx, "tenant-wf-restricted", specialist.ID, workforce.WorkPest, "actor-ops-1"); err != nil {
		t.Fatalf("specialist vendor must be routable to restricted work, got %v", err)
	}
}

func TestWorkforceAdverseActionIsHumanReviewedAndEvidenced(t *testing.T) {
	svc := newWorkforceService(t)
	ctx := context.Background()

	w := createWorker(t, svc, "tenant-wf-aar", workforce.ClassificationEmployee, dateAgo(26, 0, 0))

	// Missing evidence, reviewer or reason blocks the adverse action.
	_, err := svc.ReviewAdverseAction(ctx, "tenant-wf-aar", w.ID, workforce.AdverseActionParams{
		Action:       workforce.AdverseActionSuspend,
		EvidenceRefs: []string{"evidence:harassment-report"},
		ReviewerID:   "hr-reviewer-1",
	}, "actor-ops-1")
	if !errors.Is(err, workforce.ErrAdverseActionRequiresReason) {
		t.Fatalf("adverse action without a reason must be blocked, got %v", err)
	}

	_, err = svc.ReviewAdverseAction(ctx, "tenant-wf-aar", w.ID, workforce.AdverseActionParams{
		Action:     workforce.AdverseActionSuspend,
		ReviewerID: "hr-reviewer-1",
		Reason:     "two verified customer complaints",
	}, "actor-ops-1")
	if !errors.Is(err, workforce.ErrAdverseActionRequiresEvidence) {
		t.Fatalf("adverse action without evidence must be blocked, got %v", err)
	}

	_, err = svc.ReviewAdverseAction(ctx, "tenant-wf-aar", w.ID, workforce.AdverseActionParams{
		Action:       workforce.AdverseActionSuspend,
		EvidenceRefs: []string{"evidence:report"},
		Reason:       "two verified customer complaints",
	}, "actor-ops-1")
	if !errors.Is(err, workforce.ErrAdverseActionRequiresReviewer) {
		t.Fatalf("adverse action without a reviewer must be blocked, got %v", err)
	}

	_, err = svc.ReviewAdverseAction(ctx, "tenant-wf-aar", w.ID, workforce.AdverseActionParams{
		Action:       workforce.AdverseActionSuspend,
		EvidenceRefs: []string{"evidence:report"},
		ReviewerID:   w.ID,
		Reason:       "two verified customer complaints",
	}, "actor-ops-1")
	if !errors.Is(err, workforce.ErrAdverseActionSelfReview) {
		t.Fatalf("the worker cannot review their own adverse action, got %v", err)
	}

	// A rated worker cannot be deactivated by the rating, but a human
	// adverse-action review with evidence can suspend them.
	reviewed, err := svc.ReviewAdverseAction(ctx, "tenant-wf-aar", w.ID, workforce.AdverseActionParams{
		Action:       workforce.AdverseActionSuspend,
		EvidenceRefs: []string{"evidence:complaint-1", "evidence:complaint-2"},
		ReviewerID:   "hr-reviewer-1",
		Reason:       "two verified customer complaints",
	}, "actor-hr-1")
	if err != nil {
		t.Fatalf("human-reviewed adverse action must succeed, got %v", err)
	}
	if reviewed.Status != workforce.StatusSuspended {
		t.Fatalf("adverse action review must suspend the worker, got %q", reviewed.Status)
	}

	reviews, err := svc.ListAdverseActions(ctx, "tenant-wf-aar", w.ID)
	if err != nil {
		t.Fatalf("list adverse actions: %v", err)
	}
	if len(reviews) != 1 {
		t.Fatalf("expected one adverse action review, got %d", len(reviews))
	}
	if len(reviews[0].EvidenceRefs) != 2 {
		t.Fatalf("adverse action must preserve the evidence considered, got %v", reviews[0].EvidenceRefs)
	}

	// A suspended worker can no longer be assigned.
	if _, err := svc.AssignOperations(ctx, "tenant-wf-aar", w.ID, "general", "actor-ops-1"); !errors.Is(err, workforce.ErrWorkerNotAssignable) {
		t.Fatalf("suspended worker must not be assignable, got %v", err)
	}
}

func TestWorkforceCreateWorkerValidation(t *testing.T) {
	svc := newWorkforceService(t)
	ctx := context.Background()

	if _, err := svc.CreateWorker(ctx, workforce.CreateWorkerParams{
		TenantID:       "tenant-wf-validate",
		Classification: workforce.ClassificationEmployee,
		DateOfBirth:    dateAgo(25, 0, 0),
	}, "actor-hr-1"); !errors.Is(err, workforce.ErrMissingLegalName) {
		t.Fatalf("missing legal name must be rejected, got %v", err)
	}

	if _, err := svc.CreateWorker(ctx, workforce.CreateWorkerParams{
		TenantID:    "tenant-wf-validate",
		LegalName:   "Misclassified",
		DateOfBirth: dateAgo(25, 0, 0),
	}, "actor-hr-1"); !errors.Is(err, workforce.ErrInvalidClassification) {
		t.Fatalf("bogus classification must be rejected, got %v", err)
	}

	if _, err := svc.CreateWorker(ctx, workforce.CreateWorkerParams{
		TenantID:       "tenant-wf-validate",
		LegalName:      "Future Born",
		Classification: workforce.ClassificationEmployee,
		DateOfBirth:    dateAgo(-1, 0, 0),
	}, "actor-hr-1"); !errors.Is(err, workforce.ErrInvalidDateOfBirth) {
		t.Fatalf("future date of birth must be rejected, got %v", err)
	}
}

func TestWorkforceCrossTenantFailsClosed(t *testing.T) {
	pool := workforcePool(t)
	ctx := context.Background()

	tenantA := workforce.NewWorkforceService(pool).WithAudit(audit.NewAuditStore(pool))
	w := createWorker(t, tenantA, "tenant-wf-cross-a", workforce.ClassificationEmployee, dateAgo(22, 0, 0))

	tenantB := workforce.NewWorkforceService(pool).WithAudit(audit.NewAuditStore(pool))
	if _, err := tenantB.GetWorker(ctx, "tenant-wf-cross-b", w.ID); !errors.Is(err, workforce.ErrWorkerNotFound) {
		t.Fatalf("cross-tenant worker read must fail closed with ErrWorkerNotFound, got %v", err)
	}
	if _, err := tenantB.AssignOperations(ctx, "tenant-wf-cross-b", w.ID, "general", "actor-b"); !errors.Is(err, workforce.ErrWorkerNotFound) {
		t.Fatalf("cross-tenant assignment must fail closed with ErrWorkerNotFound, got %v", err)
	}
	if _, err := tenantB.RecordRating(ctx, "tenant-wf-cross-b", w.ID, workforce.RatingParams{
		Score:  90,
		Source: workforce.RatingSourceHuman,
	}, "actor-b"); !errors.Is(err, workforce.ErrWorkerNotFound) {
		t.Fatalf("cross-tenant rating must fail closed with ErrWorkerNotFound, got %v", err)
	}
}

func TestWorkforceAvailabilityWindow(t *testing.T) {
	svc := newWorkforceService(t)
	ctx := context.Background()

	w := createWorker(t, svc, "tenant-wf-avail", workforce.ClassificationEmployee, dateAgo(25, 0, 0))

	window, err := svc.CreateAvailabilityWindow(ctx, "tenant-wf-avail", w.ID, workforce.AvailabilityWindowParams{
		DayOfWeek:   1,
		StartMinute: 480,
		EndMinute:   1020,
		EffectiveAt: time.Now().UTC(),
	}, "actor-ops-1")
	if err != nil {
		t.Fatalf("create availability window: %v", err)
	}
	if window.DayOfWeek != 1 || window.StartMinute != 480 || window.EndMinute != 1020 {
		t.Fatalf("window fields mismatch: got day=%d start=%d end=%d", window.DayOfWeek, window.StartMinute, window.EndMinute)
	}

	windows, err := svc.ListAvailabilityWindows(ctx, "tenant-wf-avail", w.ID)
	if err != nil {
		t.Fatalf("list availability windows: %v", err)
	}
	if len(windows) != 1 {
		t.Fatalf("expected 1 availability window, got %d", len(windows))
	}

	_, err = svc.CreateAvailabilityWindow(ctx, "tenant-wf-avail", w.ID, workforce.AvailabilityWindowParams{
		DayOfWeek:   7,
		StartMinute: 480,
		EndMinute:   1020,
	}, "actor-ops-1")
	if err == nil {
		t.Fatal("day_of_week=7 must be rejected")
	}

	_, err = svc.CreateAvailabilityWindow(ctx, "tenant-wf-avail", w.ID, workforce.AvailabilityWindowParams{
		DayOfWeek:   1,
		StartMinute: 1000,
		EndMinute:   800,
	}, "actor-ops-1")
	if err == nil {
		t.Fatal("start > end must be rejected")
	}
}

func TestWorkforceTimeEntry(t *testing.T) {
	svc := newWorkforceService(t)
	ctx := context.Background()

	w := createWorker(t, svc, "tenant-wf-time", workforce.ClassificationEmployee, dateAgo(28, 0, 0))

	entry, err := svc.RecordTimeEntry(ctx, "tenant-wf-time", w.ID, workforce.TimeEntryParams{
		WorkMinutes:   120,
		TravelMinutes: 30,
		OvertimeFlag:  false,
	}, "actor-ops-1")
	if err != nil {
		t.Fatalf("record time entry: %v", err)
	}
	if entry.WorkMinutes != 120 || entry.TravelMinutes != 30 {
		t.Fatalf("time entry fields mismatch: got work=%d travel=%d", entry.WorkMinutes, entry.TravelMinutes)
	}
	if entry.OvertimeFlag {
		t.Fatal("overtime must be false")
	}

	entries, err := svc.ListTimeEntries(ctx, "tenant-wf-time", w.ID)
	if err != nil {
		t.Fatalf("list time entries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 time entry, got %d", len(entries))
	}

	_, err = svc.RecordTimeEntry(ctx, "tenant-wf-time", w.ID, workforce.TimeEntryParams{
		WorkMinutes: -5,
	}, "actor-ops-1")
	if err == nil {
		t.Fatal("negative work minutes must be rejected")
	}

	_, err = svc.RecordTimeEntry(ctx, "tenant-wf-time", w.ID, workforce.TimeEntryParams{
		WorkMinutes:   0,
		TravelMinutes: 0,
	}, "actor-ops-1")
	if err == nil {
		t.Fatal("zero minutes for both work and travel must be rejected")
	}

	// Overtime flag can be set for a time entry.
	overtimeEntry, err := svc.RecordTimeEntry(ctx, "tenant-wf-time", w.ID, workforce.TimeEntryParams{
		WorkMinutes:  60,
		OvertimeFlag: true,
	}, "actor-ops-1")
	if err != nil {
		t.Fatalf("overtime time entry: %v", err)
	}
	if !overtimeEntry.OvertimeFlag {
		t.Fatal("overtime_flag must be true")
	}
}

func TestWorkforceExpense(t *testing.T) {
	svc := newWorkforceService(t)
	ctx := context.Background()

	w := createWorker(t, svc, "tenant-wf-expense", workforce.ClassificationEmployee, dateAgo(30, 0, 0))

	exp, err := svc.RecordExpense(ctx, "tenant-wf-expense", w.ID, workforce.ExpenseParams{
		MinorUnits: 15000,
		Currency:   "INR",
		Category:   "travel",
		ReceiptRef: "recpt-001",
	}, "actor-ops-1")
	if err != nil {
		t.Fatalf("record expense: %v", err)
	}
	if exp.MinorUnits != 15000 || exp.Currency != "INR" {
		t.Fatalf("expense fields mismatch: got %d %s", exp.MinorUnits, exp.Currency)
	}

	expenses, err := svc.ListExpenses(ctx, "tenant-wf-expense", w.ID)
	if err != nil {
		t.Fatalf("list expenses: %v", err)
	}
	if len(expenses) != 1 {
		t.Fatalf("expected 1 expense, got %d", len(expenses))
	}

	_, err = svc.RecordExpense(ctx, "tenant-wf-expense", w.ID, workforce.ExpenseParams{
		MinorUnits: 0,
		Currency:   "INR",
	}, "actor-ops-1")
	if err == nil {
		t.Fatal("zero minor units must be rejected")
	}

	_, err = svc.RecordExpense(ctx, "tenant-wf-expense", w.ID, workforce.ExpenseParams{
		MinorUnits: -10,
		Currency:   "INR",
	}, "actor-ops-1")
	if err == nil {
		t.Fatal("negative minor units must be rejected")
	}
}

func TestWorkforceGrievance(t *testing.T) {
	svc := newWorkforceService(t)
	ctx := context.Background()

	w := createWorker(t, svc, "tenant-wf-griev", workforce.ClassificationEmployee, dateAgo(26, 0, 0))

	g, err := svc.SubmitGrievance(ctx, "tenant-wf-griev", w.ID, workforce.GrievanceParams{
		Kind:         "harassment",
		Reason:       "verbal abuse by supervisor",
		EvidenceRefs: []string{"evidence:msg-001", "evidence:msg-002"},
	}, "actor-ops-1")
	if err != nil {
		t.Fatalf("submit grievance: %v", err)
	}
	if g.Kind != "harassment" || g.Status != workforce.GrievanceStatusPending {
		t.Fatalf("grievance fields mismatch: kind=%s status=%s", g.Kind, g.Status)
	}
	if len(g.EvidenceRefs) != 2 {
		t.Fatalf("expected 2 evidence refs, got %d", len(g.EvidenceRefs))
	}

	grievances, err := svc.ListGrievances(ctx, "tenant-wf-griev", w.ID)
	if err != nil {
		t.Fatalf("list grievances: %v", err)
	}
	if len(grievances) != 1 {
		t.Fatalf("expected 1 grievance, got %d", len(grievances))
	}

	retrieved, err := svc.GetGrievance(ctx, "tenant-wf-griev", g.ID)
	if err != nil {
		t.Fatalf("get grievance: %v", err)
	}
	if retrieved.ID != g.ID {
		t.Fatalf("grievance ID mismatch")
	}

	_, err = svc.SubmitGrievance(ctx, "tenant-wf-griev", w.ID, workforce.GrievanceParams{
		Kind:   "",
		Reason: "a reason",
	}, "actor-ops-1")
	if err == nil {
		t.Fatal("grievance without kind must be rejected")
	}

	_, err = svc.SubmitGrievance(ctx, "tenant-wf-griev", w.ID, workforce.GrievanceParams{
		Kind:   "harassment",
		Reason: "",
	}, "actor-ops-1")
	if err == nil {
		t.Fatal("grievance without reason must be rejected")
	}
}

func TestWorkforceSOSEvent(t *testing.T) {
	svc := newWorkforceService(t)
	ctx := context.Background()

	w := createWorker(t, svc, "tenant-wf-sos", workforce.ClassificationEmployee, dateAgo(29, 0, 0))

	sos, err := svc.TriggerSOS(ctx, "tenant-wf-sos", w.ID, workforce.SOSEventParams{
		TicketID: "ticket-001",
		Location: "Property 12, 3rd floor, Gurgaon",
	}, "actor-ops-1")
	if err != nil {
		t.Fatalf("trigger SOS: %v", err)
	}
	if sos.Location != "Property 12, 3rd floor, Gurgaon" {
		t.Fatalf("SOS location mismatch: got %s", sos.Location)
	}
	if sos.TicketID != "ticket-001" {
		t.Fatalf("SOS ticket ID mismatch: got %s", sos.TicketID)
	}

	events, err := svc.ListSOSEvents(ctx, "tenant-wf-sos", w.ID)
	if err != nil {
		t.Fatalf("list SOS events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 SOS event, got %d", len(events))
	}

	// Worker state may have been frozen by the SOS alert.
	reloaded, err := svc.GetWorker(ctx, "tenant-wf-sos", w.ID)
	if err != nil {
		t.Fatalf("reload worker after SOS: %v", err)
	}
	if reloaded.Status == "" {
		t.Fatal("worker status must not be empty after SOS")
	}
}

func TestWorkforceEmploymentTerm(t *testing.T) {
	svc := newWorkforceService(t)
	ctx := context.Background()

	w := createWorker(t, svc, "tenant-wf-term", workforce.ClassificationEmployee, dateAgo(32, 0, 0))

	term, err := svc.CreateEmploymentTerm(ctx, "tenant-wf-term", w.ID, workforce.EmploymentTermParams{
		Role:             "senior-curator",
		CompensationBand: "B4",
		EffectiveDate:    dateAgo(0, 6, 0),
		AgreementRef:     "agmt-senior-001",
	}, "actor-hr-1")
	if err != nil {
		t.Fatalf("create employment term: %v", err)
	}
	if term.Role != "senior-curator" || term.CompensationBand != "B4" {
		t.Fatalf("term fields mismatch: role=%s band=%s", term.Role, term.CompensationBand)
	}

	terms, err := svc.ListEmploymentTerms(ctx, "tenant-wf-term", w.ID)
	if err != nil {
		t.Fatalf("list employment terms: %v", err)
	}
	if len(terms) != 1 {
		t.Fatalf("expected 1 employment term, got %d", len(terms))
	}

	_, err = svc.CreateEmploymentTerm(ctx, "tenant-wf-term", w.ID, workforce.EmploymentTermParams{
		Role:          "",
		EffectiveDate: dateAgo(0, 3, 0),
	}, "actor-hr-1")
	if err == nil {
		t.Fatal("employment term without role must be rejected")
	}
}
