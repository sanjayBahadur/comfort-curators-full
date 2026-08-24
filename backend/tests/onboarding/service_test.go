package onboarding_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"comfort-curators-backend/internal/onboarding"
	"comfort-curators-backend/internal/platform/audit"
	"comfort-curators-backend/internal/platform/testdb"

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

func onboardingPostgresAvailable() bool {
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

func onboardingDBConnString() string {
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

func onboardingPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if !onboardingPostgresAvailable() {
		t.Skip("PostgreSQL not available for onboarding integration test")
	}
	pool, err := pgxpool.New(context.Background(), onboardingDBConnString())
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(context.Background()); err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}
	return pool
}

func onboardingService(pool *pgxpool.Pool, tenantID string) *onboarding.Service {
	auditStore := audit.NewAuditStore(pool)
	return onboarding.NewService(pool, auditStore).WithAuthorizer(testAuthorizer{tenant: tenantID})
}

func sampleStartParams(tenantID string) onboarding.StartCaseParams {
	return onboarding.StartCaseParams{
		TenantID:         tenantID,
		PropertyID:       "prop-1",
		OwnerAuthorityID: "owner-authority-1",
	}
}

func section(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal section: %v", err)
	}
	return b
}

func completeSections(t *testing.T, ctx context.Context, svc *onboarding.Service, tenantID, caseID string) {
	t.Helper()
	sections := map[string]any{
		onboarding.StepPortfolio:          onboarding.Portfolio{PropertyName: "Sea View Villa", ManagedUnits: 1},
		onboarding.StepGoals:              onboarding.Goals{PrimaryGoal: "maximize occupancy"},
		onboarding.StepServicePreferences: onboarding.ServicePreferences{CommunicationChannel: "email", Currency: "INR"},
		onboarding.StepBudgets:            onboarding.Budgets{Currency: "INR"},
		onboarding.StepPhotographs:        []onboarding.Photograph{{ObjectRef: "obj/photo-1"}},
		onboarding.StepAmenities:          []onboarding.Amenity{{Name: "wifi", Quantity: 1}},
		onboarding.StepSafety:             onboarding.Safety{SmokeDetectorsInstalled: true},
		onboarding.StepFurnishing:         onboarding.Furnishing{FurnishingLevel: "fully_furnished"},
		onboarding.StepRemediation:        onboarding.Remediation{},
		onboarding.StepFitScoreInputs:     onboarding.FitScoreInputs{PropertyScore: 8},
	}
	for name, payload := range sections {
		if _, err := svc.SaveSection(ctx, tenantID, caseID, name, section(t, payload), "ops-1"); err != nil {
			t.Fatalf("save section %s: %v", name, err)
		}
	}
	if _, err := svc.SaveContacts(ctx, tenantID, caseID, []onboarding.Contact{{Name: "Asha", Phone: "+91-9000000000"}}, "ops-1"); err != nil {
		t.Fatalf("save contacts: %v", err)
	}
	if _, err := svc.RecordEvidence(ctx, tenantID, caseID, onboarding.EvidenceParams{
		Kind: onboarding.EvidenceKindDocument, ContentHash: "sha256:doc", ObjectRef: "obj/doc",
	}, "ops-1"); err != nil {
		t.Fatalf("record document evidence: %v", err)
	}
}

func TestOnboardingResumesAfterInterruption(t *testing.T) {
	pool := onboardingPool(t)
	ctx := context.Background()
	if err := onboarding.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure onboarding schema: %v", err)
	}
	if err := audit.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure audit schema: %v", err)
	}

	tenantID := "tenant-onb-resume"
	svc := onboardingService(pool, tenantID)

	started, err := svc.StartCase(ctx, sampleStartParams(tenantID), "ops-1")
	if err != nil {
		t.Fatalf("start case: %v", err)
	}
	if started.Status != onboarding.StatusInProgress {
		t.Errorf("new case must be in_progress, got %q", started.Status)
	}
	if started.Version != 1 {
		t.Errorf("new case must start at version 1, got %d", started.Version)
	}

	if _, err := svc.SaveSection(ctx, tenantID, started.ID, onboarding.StepPortfolio, section(t, onboarding.Portfolio{PropertyName: "Sea View Villa"}), "ops-1"); err != nil {
		t.Fatalf("save portfolio: %v", err)
	}
	if _, err := svc.SaveSection(ctx, tenantID, started.ID, onboarding.StepGoals, section(t, onboarding.Goals{PrimaryGoal: "maximize occupancy"}), "ops-1"); err != nil {
		t.Fatalf("save goals: %v", err)
	}

	// Interrupt: a fresh service instance against the same database simulates
	// a process restart. The committed sections must still be present.
	resumed := onboardingService(pool, tenantID)
	reloaded, err := resumed.GetCase(ctx, tenantID, started.ID)
	if err != nil {
		t.Fatalf("get case after interruption: %v", err)
	}
	if reloaded.Portfolio == nil || reloaded.Portfolio.PropertyName != "Sea View Villa" {
		t.Errorf("portfolio must survive the interruption, got %+v", reloaded.Portfolio)
	}
	if reloaded.Goals == nil || reloaded.Goals.PrimaryGoal != "maximize occupancy" {
		t.Errorf("goals must survive the interruption, got %+v", reloaded.Goals)
	}

	progress, err := resumed.Progress(ctx, tenantID, started.ID)
	if err != nil {
		t.Fatalf("progress: %v", err)
	}
	byKey := map[string]onboarding.StepProgress{}
	for _, p := range progress {
		byKey[p.Key] = p
	}
	if !byKey[onboarding.StepPortfolio].Complete || !byKey[onboarding.StepGoals].Complete {
		t.Error("recorded steps must report complete after resume")
	}
	if byKey[onboarding.StepLegalEvidence].Complete || byKey[onboarding.StepSafetyEvidence].Complete {
		t.Error("unrecorded evidence steps must still report pending after resume")
	}

	// Continue the interrupted case to completion on the resumed instance.
	completeSections(t, ctx, resumed, tenantID, started.ID)
	if _, err := resumed.RecordEvidence(ctx, tenantID, started.ID, onboarding.EvidenceParams{
		Kind: onboarding.EvidenceKindLegal, ContentHash: "sha256:legal", ObjectRef: "obj/legal",
	}, "ops-1"); err != nil {
		t.Fatalf("record legal evidence: %v", err)
	}
	if _, err := resumed.RecordEvidence(ctx, tenantID, started.ID, onboarding.EvidenceParams{
		Kind: onboarding.EvidenceKindSafety, ContentHash: "sha256:safety", ObjectRef: "obj/safety",
	}, "ops-1"); err != nil {
		t.Fatalf("record safety evidence: %v", err)
	}
	if _, err := resumed.RecordInspection(ctx, tenantID, started.ID, onboarding.InspectionParams{
		PropertyID:    "prop-1",
		InspectedBy:   "inspector-1",
		EvidenceHash:  "sha256:inspection",
		EvidenceRef:   "obj/inspection",
		Findings:      "no issues",
		OverallStatus: "pass",
	}, "ops-1"); err != nil {
		t.Fatalf("record inspection: %v", err)
	}

	activated, err := resumed.Activate(ctx, tenantID, started.ID, "ops-1")
	if err != nil {
		t.Fatalf("activation after resume must succeed: %v", err)
	}
	if activated.Status != onboarding.StatusActivated {
		t.Errorf("case must be activated after resume, got %q", activated.Status)
	}
}

func TestOnboardingMissingEvidenceBlocksActivation(t *testing.T) {
	pool := onboardingPool(t)
	ctx := context.Background()
	if err := onboarding.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure onboarding schema: %v", err)
	}
	if err := audit.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure audit schema: %v", err)
	}

	tenantID := "tenant-onb-evidence"
	svc := onboardingService(pool, tenantID)

	started, err := svc.StartCase(ctx, sampleStartParams(tenantID), "ops-1")
	if err != nil {
		t.Fatalf("start case: %v", err)
	}

	// Everything except the gating legal and safety evidence.
	completeSections(t, ctx, svc, tenantID, started.ID)
	if _, err := svc.RecordInspection(ctx, tenantID, started.ID, onboarding.InspectionParams{
		PropertyID:    "prop-1",
		InspectedBy:   "inspector-1",
		EvidenceHash:  "sha256:inspection",
		EvidenceRef:   "obj/inspection",
		Findings:      "no issues",
		OverallStatus: "pass",
	}, "ops-1"); err != nil {
		t.Fatalf("record inspection: %v", err)
	}

	holds, err := svc.ActivationHolds(ctx, tenantID, started.ID)
	if err != nil {
		t.Fatalf("activation holds: %v", err)
	}
	if len(holds) != 2 {
		t.Fatalf("missing legal and safety evidence must produce 2 holds, got %+v", holds)
	}

	if _, err := svc.Activate(ctx, tenantID, started.ID, "ops-1"); !errors.Is(err, onboarding.ErrActivationBlocked) {
		t.Fatalf("activation without legal or safety evidence must be blocked, got %v", err)
	}

	// Legal evidence alone still blocks on safety evidence.
	if _, err := svc.RecordEvidence(ctx, tenantID, started.ID, onboarding.EvidenceParams{
		Kind: onboarding.EvidenceKindLegal, ContentHash: "sha256:legal", ObjectRef: "obj/legal",
	}, "ops-1"); err != nil {
		t.Fatalf("record legal evidence: %v", err)
	}
	if _, err := svc.Activate(ctx, tenantID, started.ID, "ops-1"); !errors.Is(err, onboarding.ErrActivationBlocked) {
		t.Fatalf("activation with only legal evidence must still be blocked, got %v", err)
	}
	holds, err = svc.ActivationHolds(ctx, tenantID, started.ID)
	if err != nil {
		t.Fatalf("activation holds: %v", err)
	}
	if len(holds) != 1 || holds[0].Code != onboarding.HoldMissingSafetyEvidence {
		t.Fatalf("only safety hold must remain, got %+v", holds)
	}

	// Recording safety evidence clears the hold and allows activation.
	if _, err := svc.RecordEvidence(ctx, tenantID, started.ID, onboarding.EvidenceParams{
		Kind: onboarding.EvidenceKindSafety, ContentHash: "sha256:safety", ObjectRef: "obj/safety",
	}, "ops-1"); err != nil {
		t.Fatalf("record safety evidence: %v", err)
	}
	activated, err := svc.Activate(ctx, tenantID, started.ID, "ops-1")
	if err != nil {
		t.Fatalf("activation with complete evidence must succeed: %v", err)
	}
	if activated.Status != onboarding.StatusActivated {
		t.Errorf("case must be activated, got %q", activated.Status)
	}
}

func TestOnboardingInspectionEvidenceIsImmutable(t *testing.T) {
	pool := onboardingPool(t)
	ctx := context.Background()
	if err := onboarding.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure onboarding schema: %v", err)
	}
	if err := audit.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure audit schema: %v", err)
	}

	tenantID := "tenant-onb-insp"
	svc := onboardingService(pool, tenantID)

	started, err := svc.StartCase(ctx, sampleStartParams(tenantID), "ops-1")
	if err != nil {
		t.Fatalf("start case: %v", err)
	}

	insp, err := svc.RecordInspection(ctx, tenantID, started.ID, onboarding.InspectionParams{
		PropertyID:    "prop-1",
		PerformedAt:   time.Now().UTC().Add(-24 * time.Hour),
		InspectedBy:   "inspector-1",
		EvidenceHash:  "sha256:inspection-original",
		EvidenceRef:   "obj/inspection-original",
		Findings:      "no issues found",
		OverallStatus: "pass",
	}, "ops-1")
	if err != nil {
		t.Fatalf("record inspection: %v", err)
	}

	got, err := svc.GetCase(ctx, tenantID, started.ID)
	if err != nil {
		t.Fatalf("get case: %v", err)
	}
	if len(got.Inspections) != 1 {
		t.Fatalf("expected 1 inspection, got %d", len(got.Inspections))
	}
	if got.Inspections[0].EvidenceHash != "sha256:inspection-original" {
		t.Errorf("inspection evidence hash must round-trip unchanged, got %q", got.Inspections[0].EvidenceHash)
	}

	// The database rejects any UPDATE or DELETE of an inspection record so the
	// immutability invariant holds even against direct SQL.
	_, updateErr := pool.Exec(ctx, `
		UPDATE onboarding_inspections SET findings = 'tampered' WHERE id = $1
	`, insp.ID)
	if updateErr == nil {
		t.Fatal("UPDATE of an inspection record must be rejected by the database")
	}
	if !strings.Contains(updateErr.Error(), "immutable") {
		t.Errorf("update rejection must reference immutability, got %v", updateErr)
	}

	_, deleteErr := pool.Exec(ctx, `
		DELETE FROM onboarding_inspections WHERE id = $1
	`, insp.ID)
	if deleteErr == nil {
		t.Fatal("DELETE of an inspection record must be rejected by the database")
	}
	if !strings.Contains(deleteErr.Error(), "immutable") {
		t.Errorf("delete rejection must reference immutability, got %v", deleteErr)
	}

	// The record is still present and its evidence hash is unchanged.
	after, err := svc.GetCase(ctx, tenantID, started.ID)
	if err != nil {
		t.Fatalf("get case after rejected mutation: %v", err)
	}
	if len(after.Inspections) != 1 || after.Inspections[0].EvidenceHash != "sha256:inspection-original" {
		t.Errorf("inspection evidence must remain intact after rejected mutation, got %+v", after.Inspections)
	}

	// A corrected inspection appends a new record instead of mutating the old.
	correction, err := svc.RecordInspection(ctx, tenantID, started.ID, onboarding.InspectionParams{
		PropertyID:    "prop-1",
		InspectedBy:   "inspector-1",
		EvidenceHash:  "sha256:inspection-reinspection",
		EvidenceRef:   "obj/inspection-reinspection",
		Findings:      "minor fix scheduled",
		OverallStatus: "conditional",
	}, "ops-1")
	if err != nil {
		t.Fatalf("record corrected inspection: %v", err)
	}
	if correction.ID == insp.ID {
		t.Error("corrected inspection must be a new record, not a mutation")
	}
	final, err := svc.GetCase(ctx, tenantID, started.ID)
	if err != nil {
		t.Fatalf("get case: %v", err)
	}
	if len(final.Inspections) != 2 {
		t.Fatalf("expected 2 inspection records, got %d", len(final.Inspections))
	}
	if final.Inspections[0].EvidenceHash != "sha256:inspection-original" {
		t.Errorf("original inspection evidence must stay stable, got %q", final.Inspections[0].EvidenceHash)
	}
}

func TestOnboardingCrossTenantDenied(t *testing.T) {
	pool := onboardingPool(t)
	ctx := context.Background()
	if err := onboarding.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure onboarding schema: %v", err)
	}
	if err := audit.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure audit schema: %v", err)
	}

	tenantA := "tenant-onb-cross-a"
	tenantB := "tenant-onb-cross-b"
	svcA := onboardingService(pool, tenantA)

	started, err := svcA.StartCase(ctx, sampleStartParams(tenantA), "ops-a")
	if err != nil {
		t.Fatalf("start case A: %v", err)
	}

	svcB := onboardingService(pool, tenantB)
	if _, err := svcB.GetCase(ctx, tenantB, started.ID); !errors.Is(err, onboarding.ErrCaseNotFound) {
		t.Errorf("cross-tenant read must fail closed with ErrCaseNotFound, got %v", err)
	}
	if _, err := svcB.Activate(ctx, tenantB, started.ID, "ops-b"); !errors.Is(err, onboarding.ErrCaseNotFound) {
		t.Errorf("cross-tenant write must fail closed with ErrCaseNotFound, got %v", err)
	}
	if _, err := svcB.RecordEvidence(ctx, tenantB, started.ID, onboarding.EvidenceParams{
		Kind: onboarding.EvidenceKindLegal, ContentHash: "x", ObjectRef: "y",
	}, "ops-b"); !errors.Is(err, onboarding.ErrCaseNotFound) {
		t.Errorf("cross-tenant evidence write must fail closed with ErrCaseNotFound, got %v", err)
	}

	deniedSvc := onboarding.NewService(pool, audit.NewAuditStore(pool)).WithAuthorizer(testAuthorizer{deny: true})
	if _, err := deniedSvc.GetCase(ctx, tenantA, started.ID); !errors.Is(err, onboarding.ErrCrossTenantDenied) {
		t.Errorf("denied authorizer must yield ErrCrossTenantDenied, got %v", err)
	}
}

func TestOnboardingStartCaseValidation(t *testing.T) {
	pool := onboardingPool(t)
	ctx := context.Background()
	if err := onboarding.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure onboarding schema: %v", err)
	}

	tenantID := "tenant-onb-validation"
	svc := onboardingService(pool, tenantID)

	params := sampleStartParams(tenantID)
	params.PropertyID = ""
	if _, err := svc.StartCase(ctx, params, "ops-1"); err == nil {
		t.Error("missing property must be rejected")
	}

	params = sampleStartParams(tenantID)
	params.OwnerAuthorityID = ""
	if _, err := svc.StartCase(ctx, params, "ops-1"); err == nil {
		t.Error("missing owner authority must be rejected")
	}
}

func TestOnboardingSectionValidationPersists(t *testing.T) {
	pool := onboardingPool(t)
	ctx := context.Background()
	if err := onboarding.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure onboarding schema: %v", err)
	}

	tenantID := "tenant-onb-section"
	svc := onboardingService(pool, tenantID)

	started, err := svc.StartCase(ctx, sampleStartParams(tenantID), "ops-1")
	if err != nil {
		t.Fatalf("start case: %v", err)
	}

	if _, err := svc.SaveSection(ctx, tenantID, started.ID, "bogus", section(t, map[string]any{}), "ops-1"); !errors.Is(err, onboarding.ErrInvalidSection) {
		t.Errorf("unknown section must be rejected, got %v", err)
	}
	if _, err := svc.SaveSection(ctx, tenantID, started.ID, onboarding.StepPortfolio, section(t, map[string]any{}), "ops-1"); err == nil {
		t.Error("invalid section payload must be rejected")
	}

	got, err := svc.GetCase(ctx, tenantID, started.ID)
	if err != nil {
		t.Fatalf("get case: %v", err)
	}
	if got.Portfolio != nil {
		t.Errorf("rejected section must not be persisted, got %+v", got.Portfolio)
	}
	if got.Version != 1 {
		t.Errorf("rejected section must not bump the version, got %d", got.Version)
	}
}
