package tests

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"comfort-curators-backend/internal/access"
	"comfort-curators-backend/internal/billing"
	"comfort-curators-backend/internal/iam"
	"comfort-curators-backend/internal/platform/audit"
	"comfort-curators-backend/internal/platform/database"
	"comfort-curators-backend/internal/platform/testdb"
	"comfort-curators-backend/internal/property"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func auditPostgresAvailable() bool {
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

func auditPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if !auditPostgresAvailable() {
		t.Skip("PostgreSQL not available for audit atomicity test")
	}
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
	connStr := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, pass, host, port, name)
	pool, err := pgxpool.New(context.Background(), connStr)
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(context.Background()); err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}
	return pool
}

func auditCount(t *testing.T, pool *pgxpool.Pool, action, resourceID string) int {
	t.Helper()
	var count int
	err := pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM audit_events WHERE action = $1 AND resource_id = $2`, action, resourceID).Scan(&count)
	if err != nil {
		t.Fatalf("audit count: %v", err)
	}
	return count
}

// ---------------------------------------------------------------------------
// Property module — atomicity
// ---------------------------------------------------------------------------

type testAuthorizer struct {
	tenant string
}

func (a testAuthorizer) RequireResourceAccess(ctx context.Context, resourceTenantID, resourceType, resourceID string) error {
	if a.tenant != "" && a.tenant != resourceTenantID {
		return errors.New("cross-tenant access denied")
	}
	return nil
}

func TestPropertyAuditAtomicitySuccess(t *testing.T) {
	pool := auditPool(t)
	ctx := context.Background()
	if err := property.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure property schema: %v", err)
	}
	if err := audit.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure audit schema: %v", err)
	}

	tenantID := "tenant-prop-audit-success"
	auditStore := audit.NewAuditStore(pool)
	svc := property.NewPropertyService(pool, auditStore).WithAuthorizer(testAuthorizer{tenant: tenantID})

	params := property.CreatePropertyParams{
		TenantID:         tenantID,
		OwnerAuthorityID: "owner-auth-1",
		ServiceAddress: property.Address{
			Line1: "1 Test St",
			City:  "TestCity",
		},
		MaximumOccupancy: 4,
	}

	p, err := svc.CreateProperty(ctx, params, "actor-1")
	if err != nil {
		t.Fatalf("create property: %v", err)
	}
	if p.ID == "" {
		t.Fatal("expected property ID")
	}

	c := auditCount(t, pool, "property.create", p.ID)
	if c != 1 {
		t.Errorf("expected 1 audit event for property.create, got %d", c)
	}
}

func TestPropertyAuditAtomicityRollback(t *testing.T) {
	pool := auditPool(t)
	ctx := context.Background()
	if err := property.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure property schema: %v", err)
	}
	if err := audit.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure audit schema: %v", err)
	}

	tenantID := "tenant-prop-audit-rollback"
	auditStore := audit.NewAuditStore(pool)
	svc := property.NewPropertyService(pool, auditStore).WithAuthorizer(testAuthorizer{tenant: tenantID})

	params := property.CreatePropertyParams{
		TenantID:         tenantID,
		OwnerAuthorityID: "owner-auth-1",
		ServiceAddress: property.Address{
			Line1: "1 Test St",
			City:  "TestCity",
		},
		MaximumOccupancy: 4,
	}

	p, err := svc.CreateProperty(ctx, params, "actor-1")
	if err != nil {
		t.Fatalf("create property: %v", err)
	}

	// invalid transition: Lead -> Active skips onboarding, must fail
	_, err = svc.TransitionProperty(ctx, tenantID, p.ID, property.StateActive, "bad transition", "actor-1")
	if !errors.Is(err, property.ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}

	c := auditCount(t, pool, "property.transition", p.ID)
	if c != 0 {
		t.Errorf("invalid transition must not leave audit event, got %d", c)
	}

	// verify property state wasn't changed
	got, err := svc.GetProperty(ctx, tenantID, p.ID)
	if err != nil {
		t.Fatalf("get property: %v", err)
	}
	if got.State != property.StateLead {
		t.Errorf("invalid transition must not change property state, got %q", got.State)
	}
}

// ---------------------------------------------------------------------------
// Billing module — atomicity
// ---------------------------------------------------------------------------

func TestBillingAuditAtomicitySuccess(t *testing.T) {
	pool := auditPool(t)
	ctx := context.Background()
	if err := billing.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure billing schema: %v", err)
	}
	if err := audit.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure audit schema: %v", err)
	}

	tenantID := "tenant-bill-audit-success"
	svc := billing.NewService(pool)

	charge, err := svc.CreateCharge(ctx, tenantID, billing.CreateChargeParams{
		PropertyID:       "prop-1",
		ChargeType:       billing.ChargeTypeManagementFee,
		AmountMinorUnits: 1000,
		Currency:         "INR",
		Reason:           "test charge",
		Data:             []byte("{}"),
		IdempotencyKey:   "ik-charge-1",
	}, "actor-1")
	if err != nil {
		t.Fatalf("create charge: %v", err)
	}
	if charge.ID == "" {
		t.Fatal("expected charge ID")
	}

	c := auditCount(t, pool, "billing.charge.created", charge.ID)
	if c != 1 {
		t.Errorf("expected 1 audit event for billing.charge.created, got %d", c)
	}
}

func TestBillingAuditAtomicityRollback(t *testing.T) {
	pool := auditPool(t)
	ctx := context.Background()
	if err := billing.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure billing schema: %v", err)
	}
	if err := audit.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure audit schema: %v", err)
	}

	tenantID := "tenant-bill-audit-rollback"
	svc := billing.NewService(pool)

	// duplicate idempotency key with different amount must fail
	firstCharge, err := svc.CreateCharge(ctx, tenantID, billing.CreateChargeParams{
		PropertyID:       "prop-1",
		ChargeType:       billing.ChargeTypeManagementFee,
		AmountMinorUnits: 1000,
		Currency:         "INR",
		Reason:           "first",
		Data:             []byte("{}"),
		IdempotencyKey:   "ik-dup-1",
	}, "actor-1")
	if err != nil {
		t.Fatalf("first charge: %v", err)
	}

	_, err = svc.CreateCharge(ctx, tenantID, billing.CreateChargeParams{
		PropertyID:       "prop-1",
		ChargeType:       billing.ChargeTypeManagementFee,
		AmountMinorUnits: 2000,
		Currency:         "INR",
		Reason:           "second",
		Data:             []byte("{}"),
		IdempotencyKey:   "ik-dup-1",
	}, "actor-1")
	if !errors.Is(err, billing.ErrDuplicateCharge) {
		t.Fatalf("expected ErrDuplicateCharge, got %v", err)
	}

	// there should be exactly 1 audit event (from the first charge)
	c := auditCount(t, pool, "billing.charge.created", firstCharge.ID)
	if c != 1 {
		t.Errorf("duplicate charge must not create extra audit event, expected 1 got %d", c)
	}
}

// ---------------------------------------------------------------------------
// IAM module — atomicity
// ---------------------------------------------------------------------------

func TestIAMAuditAtomicitySuccess(t *testing.T) {
	pool := auditPool(t)
	ctx := context.Background()
	if err := iam.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure iam schema: %v", err)
	}
	if err := audit.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure audit schema: %v", err)
	}

	tenantID := "tenant-iam-audit-success"
	auditStore := audit.NewAuditStore(pool)
	svc := iam.NewIdentityService(pool, auditStore)

	user, err := svc.EnsureUser(ctx, iam.CreateUserParams{
		TenantID: tenantID,
		Contact:  "+919999000001",
		Role:     iam.RoleStaff,
	})
	if err != nil {
		t.Fatalf("ensure user: %v", err)
	}
	if user.ID == "" {
		t.Fatal("expected user ID")
	}

	c := auditCount(t, pool, "user.create", user.ID)
	if c != 1 {
		t.Errorf("expected 1 audit event for user.create, got %d", c)
	}
}

func TestIAMAuditAtomicityRollback(t *testing.T) {
	pool := auditPool(t)
	ctx := context.Background()
	if err := iam.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure iam schema: %v", err)
	}
	if err := audit.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure audit schema: %v", err)
	}

	tenantID := "tenant-iam-audit-rollback"
	auditStore := audit.NewAuditStore(pool)
	svc := iam.NewIdentityService(pool, auditStore)

	// ensure user succeeds
	firstUser, err := svc.EnsureUser(ctx, iam.CreateUserParams{
		TenantID: tenantID,
		Contact:  "+919999000001",
		Role:     iam.RoleStaff,
	})
	if err != nil {
		t.Fatalf("ensure user: %v", err)
	}

	// re-ensure the same user (should be idempotent, not an error)
	user2, err := svc.EnsureUser(ctx, iam.CreateUserParams{
		TenantID: tenantID,
		Contact:  "+919999000001",
		Role:     iam.RoleStaff,
	})
	if err != nil {
		t.Fatalf("re-ensure user: %v", err)
	}
	if user2 == nil {
		t.Fatal("expected existing user")
	}

	// first create should have 1 audit, re-ensure is idempotent (no extra audit)
	c := auditCount(t, pool, "user.create", firstUser.ID)
	if c != 1 {
		t.Errorf("idempotent re-ensure must not create extra audit, expected 1 got %d", c)
	}
}

// ---------------------------------------------------------------------------
// Access module — atomicity
// ---------------------------------------------------------------------------

func TestAccessAuditAtomicitySuccess(t *testing.T) {
	pool := auditPool(t)
	ctx := context.Background()
	if err := access.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure access schema: %v", err)
	}
	if err := audit.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure audit schema: %v", err)
	}

	tenantID := "tenant-acc-audit-success"
	svc := access.NewService(pool)

	sec, err := svc.StoreSecret(ctx, tenantID, "prop-1", access.CreateSecretParams{
		SecretType:     access.SecretTypeKeyCode,
		EncryptedValue: "enc-value-1",
	}, "actor-1")
	if err != nil {
		t.Fatalf("store secret: %v", err)
	}

	c := auditCount(t, pool, "access.secret.stored", sec.ID)
	if c != 1 {
		t.Errorf("expected 1 audit event for access.secret.stored, got %d", c)
	}
}

func TestAccessAuditAtomicityRollback(t *testing.T) {
	pool := auditPool(t)
	ctx := context.Background()
	if err := access.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure access schema: %v", err)
	}
	if err := audit.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure audit schema: %v", err)
	}

	tenantID := "tenant-acc-audit-rollback"
	svc := access.NewService(pool)

	// create a grant without an underlying secret must fail
	_, err := svc.CreateGrant(ctx, tenantID, "prop-1", "nonexistent-secret", access.CreateGrantParams{
		GranteeID:   "grantee-1",
		WindowStart: time.Now(),
		WindowEnd:   time.Now().Add(time.Hour),
	}, "actor-1")
	if err == nil {
		t.Fatal("expected error for non-existent secret")
	}

	c := auditCount(t, pool, "access.grant.created", "nonexistent-secret")
	if c != 0 {
		t.Errorf("failed grant creation must not leave audit event, got %d", c)
	}
}

// ---------------------------------------------------------------------------
// Direct transaction-level atomicity proof
// ---------------------------------------------------------------------------

func TestAuditFailureInsideTxRollsBackBusinessMutation(t *testing.T) {
	pool := auditPool(t)
	ctx := context.Background()
	if err := audit.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure audit schema: %v", err)
	}

	// create scratch table
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS tx_atom_test (id TEXT PRIMARY KEY, val TEXT)`); err != nil {
		t.Fatalf("create scratch table: %v", err)
	}

	// use unique IDs per test run
	ts := fmt.Sprintf("%d", time.Now().UnixNano())
	successEvtID := "evt-tx-success-" + ts
	dupeEvtID := "evt-tx-dupe-" + ts
	bizID1 := "tx-1-" + ts
	bizID2 := "tx-2-" + ts

	t.Cleanup(func() {
		pool.Exec(context.Background(), `DROP TABLE IF EXISTS tx_atom_test`)
		pool.Exec(context.Background(), `DELETE FROM audit_events WHERE id = $1`, successEvtID)
		pool.Exec(context.Background(), `DELETE FROM audit_events WHERE id = $1`, dupeEvtID)
	})

	auditStore := audit.NewAuditStore(pool)

	// First: successful tx — both business data AND audit committed
	err := database.WithTx(ctx, pool, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO tx_atom_test (id, val) VALUES ($1, $2)`, bizID1, "success"); err != nil {
			return err
		}
		return auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			ID:           successEvtID,
			EventType:    audit.EventTypeMutation,
			Action:       "tx.success",
			ResourceType: "test",
			ResourceID:   bizID1,
			ActorID:      "test",
		})
	})
	if err != nil {
		t.Fatalf("successful tx: %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM tx_atom_test WHERE id = $1`, bizID1).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("business row must persist after successful commit, got %d", count)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE id = $1`, successEvtID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("audit event must persist after successful commit, got %d", count)
	}

	// Second: force audit INSERT to fail with a duplicate ID — verify business row also rolls back
	// Pre-insert a row into audit_events with the same ID we'll use inside tx
	_, err = pool.Exec(ctx, `INSERT INTO audit_events (id, event_type, actor_id, action, resource_type, resource_id)
		VALUES ($1, 'mutation', 'test', 'system.seed', 'test', 'test')`, dupeEvtID)
	if err != nil {
		t.Fatalf("seed audit row: %v", err)
	}

	err = database.WithTx(ctx, pool, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO tx_atom_test (id, val) VALUES ($1, $2)`, bizID2, "rollback-test"); err != nil {
			return err
		}
		// This audit insert must fail due to duplicate ID
		return auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			ID:           dupeEvtID,
			EventType:    audit.EventTypeMutation,
			Action:       "tx.failure",
			ResourceType: "test",
			ResourceID:   bizID2,
			ActorID:      "test",
		})
	})
	if err == nil {
		t.Fatal("expected error from duplicate audit key, got nil")
	}

	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM tx_atom_test WHERE id = $1`, bizID2).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("business row must NOT persist when audit insert fails in same tx, got %d rows", count)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE action = 'tx.failure'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("no audit event for tx.failure must exist, got %d", count)
	}
}
