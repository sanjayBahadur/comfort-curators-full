package property_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"comfort-curators-backend/internal/iam"
	"comfort-curators-backend/internal/platform/audit"
	"comfort-curators-backend/internal/platform/testdb"
	"comfort-curators-backend/internal/property"

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

func propertyPostgresAvailable() bool {
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

func propertyDBConnString() string {
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

func propertyPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if !propertyPostgresAvailable() {
		t.Skip("PostgreSQL not available for property integration test")
	}
	pool, err := pgxpool.New(context.Background(), propertyDBConnString())
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(context.Background()); err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}
	return pool
}

func propertyService(pool *pgxpool.Pool, tenantID string) *property.PropertyService {
	auditStore := audit.NewAuditStore(pool)
	return property.NewPropertyService(pool, auditStore).WithAuthorizer(testAuthorizer{tenant: tenantID})
}

func samplePropertyParams(tenantID string) property.CreatePropertyParams {
	return property.CreatePropertyParams{
		TenantID:         tenantID,
		OwnerAuthorityID: "owner-authority-1",
		ServiceAddress: property.Address{
			Line1:      "14 Marine Drive",
			City:       "Noida",
			State:      "Uttar Pradesh",
			PostalCode: "226001",
			Country:    "IN",
		},
		GeolocationZone: "zone-lko-north",
		Timezone:        "Asia/Kolkata",
		EmergencyContacts: []property.EmergencyContact{
			{Name: "Asha", Phone: "+91-9000000000", Role: "neighbour"},
		},
		AccessMethod:     "lockbox",
		MaximumOccupancy: 4,
	}
}

func TestPropertyServiceLifecyclePersistsTransitions(t *testing.T) {
	pool := propertyPool(t)
	ctx := context.Background()
	if err := property.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure property schema: %v", err)
	}
	if err := iam.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure iam schema: %v", err)
	}
	if err := audit.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure audit schema: %v", err)
	}

	tenantID := "tenant-prop-lifecycle"
	svc := propertyService(pool, tenantID)

	p, err := svc.CreateProperty(ctx, samplePropertyParams(tenantID), "ops-1")
	if err != nil {
		t.Fatalf("create property: %v", err)
	}
	if p.State != property.StateLead {
		t.Errorf("new property must be in lead state, got %q", p.State)
	}
	if p.Version != 1 {
		t.Errorf("new property must start at version 1, got %d", p.Version)
	}
	if p.ServiceAddress.City != "Noida" {
		t.Errorf("service address mismatch: %+v", p.ServiceAddress)
	}

	path := []struct {
		to     string
		reason string
	}{
		{property.StateQualifying, "owner submitted qualification answers"},
		{property.StateOnboarding, "property onboarding started"},
		{property.StateRemediation, "safety remediation scheduled"},
		{property.StateReadyInactive, "remediation complete"},
		{property.StateActive, "activated after readiness review"},
	}
	for _, step := range path {
		if step.to == property.StateActive {
			// PROP-002: activation requires the mandatory readiness inputs.
			if _, err := svc.SetReadiness(ctx, tenantID, p.ID, property.Readiness{
				OwnerContractAccepted: true,
				ComplianceComplete:    true,
				MandatoryFieldsSet:    true,
			}, "ops-1"); err != nil {
				t.Fatalf("set readiness: %v", err)
			}
		}
		p, err = svc.TransitionProperty(ctx, tenantID, p.ID, step.to, step.reason, "ops-1")
		if err != nil {
			t.Fatalf("transition to %s failed: %v", step.to, err)
		}
		if p.State != step.to {
			t.Fatalf("expected state %q, got %q", step.to, p.State)
		}
	}

	got, err := svc.GetProperty(ctx, tenantID, p.ID)
	if err != nil {
		t.Fatalf("get property: %v", err)
	}
	if got.State != property.StateActive {
		t.Errorf("persisted state must be active, got %q", got.State)
	}
	if got.Version != len(path)+2 {
		t.Errorf("expected version %d, got %d", len(path)+2, got.Version)
	}

	transitions, err := svc.ListTransitions(ctx, tenantID, p.ID)
	if err != nil {
		t.Fatalf("list transitions: %v", err)
	}
	if len(transitions) != len(path) {
		t.Fatalf("expected %d recorded transitions, got %d", len(path), len(transitions))
	}
	last := transitions[len(transitions)-1]
	if last.FromState != property.StateReadyInactive || last.ToState != property.StateActive {
		t.Errorf("last transition mismatch: %s -> %s", last.FromState, last.ToState)
	}
	if last.ActorID != "ops-1" || last.Reason == "" {
		t.Errorf("transition must record actor and reason: %+v", last)
	}
}

func TestPropertyServiceInvalidTransitionFailsAtomically(t *testing.T) {
	pool := propertyPool(t)
	ctx := context.Background()
	if err := property.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure property schema: %v", err)
	}

	tenantID := "tenant-prop-invalid"
	svc := propertyService(pool, tenantID)

	p, err := svc.CreateProperty(ctx, samplePropertyParams(tenantID), "ops-1")
	if err != nil {
		t.Fatalf("create property: %v", err)
	}

	_, err = svc.TransitionProperty(ctx, tenantID, p.ID, property.StateActive, "skip onboarding", "ops-1")
	if !errors.Is(err, property.ErrInvalidTransition) {
		t.Fatalf("lead -> active must fail with ErrInvalidTransition, got %v", err)
	}

	got, err := svc.GetProperty(ctx, tenantID, p.ID)
	if err != nil {
		t.Fatalf("get property: %v", err)
	}
	if got.State != property.StateLead {
		t.Errorf("invalid transition must not change persisted state, got %q", got.State)
	}
	if got.Version != 1 {
		t.Errorf("invalid transition must not bump persisted version, got %d", got.Version)
	}

	transitions, err := svc.ListTransitions(ctx, tenantID, p.ID)
	if err != nil {
		t.Fatalf("list transitions: %v", err)
	}
	if len(transitions) != 0 {
		t.Errorf("invalid transition must not leave a transition record, got %d", len(transitions))
	}
}

func TestPropertyServiceCriticalHoldBlocksActivation(t *testing.T) {
	pool := propertyPool(t)
	ctx := context.Background()
	if err := property.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure property schema: %v", err)
	}

	tenantID := "tenant-prop-hold"
	svc := propertyService(pool, tenantID)

	p, err := svc.CreateProperty(ctx, samplePropertyParams(tenantID), "ops-1")
	if err != nil {
		t.Fatalf("create property: %v", err)
	}
	propertyID := p.ID

	for _, to := range []string{property.StateQualifying, property.StateOnboarding, property.StateRemediation, property.StateReadyInactive} {
		p, err = svc.TransitionProperty(ctx, tenantID, p.ID, to, "progress", "ops-1")
		if err != nil {
			t.Fatalf("transition to %s: %v", to, err)
		}
	}

	if _, err := svc.SetReadiness(ctx, tenantID, p.ID, property.Readiness{
		OwnerContractAccepted: true,
		ComplianceComplete:    true,
		MandatoryFieldsSet:    true,
	}, "ops-1"); err != nil {
		t.Fatalf("set readiness: %v", err)
	}

	hold, err := svc.AddComplianceHold(ctx, tenantID, p.ID, property.ComplianceHoldParams{
		Kind:     property.HoldKindPermission,
		Severity: property.HoldSeverityCritical,
		Reason:   "expired local authority permission",
	}, "compliance-1")
	if err != nil {
		t.Fatalf("add compliance hold: %v", err)
	}
	if hold.Severity != property.HoldSeverityCritical || hold.Status != property.HoldStatusOpen {
		t.Errorf("hold must be open and critical: %+v", hold)
	}

	p, err = svc.TransitionProperty(ctx, tenantID, propertyID, property.StateActive, "activate", "ops-1")
	if !errors.Is(err, property.ErrComplianceHold) {
		t.Fatalf("critical hold must block activation with ErrComplianceHold, got %v", err)
	}

	// Grant a documented, time-bounded reviewer exception and activation passes.
	p, err = svc.GrantComplianceException(ctx, tenantID, propertyID, hold.ID, "reviewer-1", "permission renewal filed, exception 14 days", 14*24*time.Hour, "ops-lead")
	if err != nil {
		t.Fatalf("grant exception: %v", err)
	}
	for _, h := range p.ComplianceHolds {
		if h.ID == hold.ID && h.Status != property.HoldStatusExcepted {
			t.Errorf("hold must be excepted after exception, got %q", h.Status)
		}
	}

	p, err = svc.TransitionProperty(ctx, tenantID, propertyID, property.StateActive, "activate under exception", "ops-1")
	if err != nil {
		t.Fatalf("activation with valid reviewer exception must succeed: %v", err)
	}
	if p.State != property.StateActive {
		t.Errorf("property must be active, got %q", p.State)
	}
}

func TestPropertyServiceResolvedHoldAllowsActivation(t *testing.T) {
	pool := propertyPool(t)
	ctx := context.Background()
	if err := property.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure property schema: %v", err)
	}

	tenantID := "tenant-prop-resolved"
	svc := propertyService(pool, tenantID)

	p, err := svc.CreateProperty(ctx, samplePropertyParams(tenantID), "ops-1")
	if err != nil {
		t.Fatalf("create property: %v", err)
	}
	for _, to := range []string{property.StateQualifying, property.StateOnboarding, property.StateRemediation, property.StateReadyInactive} {
		p, err = svc.TransitionProperty(ctx, tenantID, p.ID, to, "progress", "ops-1")
		if err != nil {
			t.Fatalf("transition to %s: %v", to, err)
		}
	}
	if _, err := svc.SetReadiness(ctx, tenantID, p.ID, property.Readiness{
		OwnerContractAccepted: true,
		ComplianceComplete:    true,
		MandatoryFieldsSet:    true,
	}, "ops-1"); err != nil {
		t.Fatalf("set readiness: %v", err)
	}

	hold, err := svc.AddComplianceHold(ctx, tenantID, p.ID, property.ComplianceHoldParams{
		Kind:     property.HoldKindSafetyDocument,
		Severity: property.HoldSeverityCritical,
		Reason:   "fire safety certificate missing",
	}, "compliance-1")
	if err != nil {
		t.Fatalf("add hold: %v", err)
	}

	p, err = svc.ResolveComplianceHold(ctx, tenantID, p.ID, hold.ID, "ops-1")
	if err != nil {
		t.Fatalf("resolve hold: %v", err)
	}

	p, err = svc.TransitionProperty(ctx, tenantID, p.ID, property.StateActive, "activate after resolution", "ops-1")
	if err != nil {
		t.Fatalf("activation with resolved hold must succeed: %v", err)
	}
	if p.State != property.StateActive {
		t.Errorf("property must be active, got %q", p.State)
	}
}

func TestPropertyServiceReadyInactiveAndActiveDistinct(t *testing.T) {
	pool := propertyPool(t)
	ctx := context.Background()
	if err := property.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure property schema: %v", err)
	}

	tenantID := "tenant-prop-distinct"
	svc := propertyService(pool, tenantID)

	p, err := svc.CreateProperty(ctx, samplePropertyParams(tenantID), "ops-1")
	if err != nil {
		t.Fatalf("create property: %v", err)
	}
	for _, to := range []string{property.StateQualifying, property.StateOnboarding, property.StateRemediation, property.StateReadyInactive} {
		p, err = svc.TransitionProperty(ctx, tenantID, p.ID, to, "progress", "ops-1")
		if err != nil {
			t.Fatalf("transition to %s: %v", to, err)
		}
	}

	ready, err := svc.GetProperty(ctx, tenantID, p.ID)
	if err != nil {
		t.Fatalf("get property: %v", err)
	}
	if ready.State != property.StateReadyInactive {
		t.Fatalf("expected ready_inactive, got %q", ready.State)
	}
	if ready.State == property.StateActive {
		t.Fatal("ready_inactive must not equal active")
	}
	if _, err := svc.SetReadiness(ctx, tenantID, p.ID, property.Readiness{
		OwnerContractAccepted: true,
		ComplianceComplete:    true,
		MandatoryFieldsSet:    true,
	}, "ops-1"); err != nil {
		t.Fatalf("set readiness: %v", err)
	}

	stillReady, err := svc.GetProperty(ctx, tenantID, p.ID)
	if err != nil {
		t.Fatalf("get property: %v", err)
	}
	if stillReady.State != property.StateReadyInactive {
		t.Errorf("fully ready property without an activation step must remain ready_inactive, got %q", stillReady.State)
	}

	active, err := svc.TransitionProperty(ctx, tenantID, p.ID, property.StateActive, "activate", "ops-1")
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	if active.State != property.StateActive {
		t.Errorf("after activation expected active, got %q", active.State)
	}
}

func TestPropertyServiceCrossTenantDenied(t *testing.T) {
	pool := propertyPool(t)
	ctx := context.Background()
	if err := property.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure property schema: %v", err)
	}

	tenantA := "tenant-prop-cross-a"
	tenantB := "tenant-prop-cross-b"
	svcA := propertyService(pool, tenantA)

	p, err := svcA.CreateProperty(ctx, samplePropertyParams(tenantA), "ops-a")
	if err != nil {
		t.Fatalf("create property A: %v", err)
	}

	svcB := propertyService(pool, tenantB)
	if _, err := svcB.GetProperty(ctx, tenantB, p.ID); !errors.Is(err, property.ErrPropertyNotFound) {
		t.Errorf("cross-tenant read must fail closed with ErrPropertyNotFound, got %v", err)
	}
	if _, err := svcB.TransitionProperty(ctx, tenantB, p.ID, property.StateQualifying, "intrusion", "ops-b"); !errors.Is(err, property.ErrPropertyNotFound) {
		t.Errorf("cross-tenant write must fail closed with ErrPropertyNotFound, got %v", err)
	}

	deniedSvc := property.NewPropertyService(pool, audit.NewAuditStore(pool)).WithAuthorizer(testAuthorizer{deny: true})
	if _, err := deniedSvc.GetProperty(ctx, tenantA, p.ID); !errors.Is(err, property.ErrCrossTenantDenied) {
		t.Errorf("denied authorizer must yield ErrCrossTenantDenied, got %v", err)
	}
	if _, err := deniedSvc.CreateProperty(ctx, samplePropertyParams(tenantA), "ops-a"); !errors.Is(err, property.ErrCrossTenantDenied) {
		t.Errorf("denied authorizer must refuse create, got %v", err)
	}
}

func TestPropertyServiceCreateValidation(t *testing.T) {
	pool := propertyPool(t)
	ctx := context.Background()
	if err := property.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure property schema: %v", err)
	}

	tenantID := "tenant-prop-validation"
	svc := propertyService(pool, tenantID)

	params := samplePropertyParams(tenantID)
	params.OwnerAuthorityID = ""
	if _, err := svc.CreateProperty(ctx, params, "ops-1"); err == nil {
		t.Error("missing owner authority must be rejected")
	}

	// access_method is deliberately optional at the service layer: the
	// property API does not supply it, and the frozen build removed the
	// requirement (docs/development/CHANGELOG.md, Phase 2). Creation with
	// an empty access method must therefore succeed.
	params = samplePropertyParams(tenantID)
	params.AccessMethod = ""
	if _, err := svc.CreateProperty(ctx, params, "ops-1"); err != nil {
		t.Errorf("missing access method must be allowed, got %v", err)
	}

	params = samplePropertyParams(tenantID)
	params.MaximumOccupancy = 0
	if _, err := svc.CreateProperty(ctx, params, "ops-1"); err == nil {
		t.Error("zero maximum occupancy must be rejected")
	}

	params = samplePropertyParams(tenantID)
	params.InitialState = "listed"
	if _, err := svc.CreateProperty(ctx, params, "ops-1"); !errors.Is(err, property.ErrInvalidState) {
		t.Errorf("unknown initial state must be rejected with ErrInvalidState, got %v", err)
	}
}
