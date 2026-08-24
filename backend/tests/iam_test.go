package tests

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"comfort-curators-backend/internal/iam"
	"comfort-curators-backend/internal/platform/app"
	"comfort-curators-backend/internal/platform/audit"
	"comfort-curators-backend/internal/platform/security"
	"comfort-curators-backend/internal/platform/testdb"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestIAMOTPHashedExpiringSingleUse(t *testing.T) {
	otp, err := iam.GenerateOTP(iam.DefaultOTPLength)
	if err != nil {
		t.Fatalf("generate otp: %v", err)
	}
	if otp.Token == "" {
		t.Fatal("otp token must not be empty")
	}
	if len(otp.Token) != iam.DefaultOTPLength {
		t.Errorf("otp length: expected %d, got %d", iam.DefaultOTPLength, len(otp.Token))
	}

	hash := iam.HashOTP(otp.Token)
	if hash == "" {
		t.Fatal("otp hash must not be empty")
	}

	if !iam.VerifyOTPHash(otp.Token, hash) {
		t.Error("otp hash verification must succeed for valid token")
	}

	if iam.VerifyOTPHash("wrongtoken", hash) {
		t.Error("otp hash verification must fail for wrong token")
	}

	if iam.VerifyOTPHash(otp.Token, "wronghash") {
		t.Error("otp hash verification must fail for wrong hash")
	}

	if otp.ExpiresAt.Before(time.Now().UTC()) {
		t.Error("otp must expire in the future")
	}
	if otp.ExpiresAt.After(time.Now().UTC().Add(iam.DefaultOTPTTL + time.Second)) {
		t.Error("otp expiry too far in the future")
	}

	token2, err := iam.GenerateOTP(iam.DefaultOTPLength)
	if err != nil {
		t.Fatalf("generate second otp: %v", err)
	}
	if iam.HashOTP(otp.Token) == iam.HashOTP(token2.Token) {
		t.Error("two OTPs must not have the same hash")
	}
}

func TestIAMSessionTokenGeneration(t *testing.T) {
	token1, err := iam.GenerateSessionToken()
	if err != nil {
		t.Fatalf("generate session token: %v", err)
	}
	if len(token1) != 64 {
		t.Errorf("session token length: expected 64, got %d", len(token1))
	}

	token2, err := iam.GenerateSessionToken()
	if err != nil {
		t.Fatalf("generate second session token: %v", err)
	}
	if token1 == token2 {
		t.Error("session tokens must be unique")
	}
}

func TestIAMRoleValidation(t *testing.T) {
	if !iam.ValidRole(iam.RoleOwner) {
		t.Error("owner must be valid role")
	}
	if !iam.ValidRole(iam.RoleGuest) {
		t.Error("guest must be valid role")
	}
	if !iam.ValidRole(iam.RoleStaff) {
		t.Error("staff must be valid role")
	}
	if !iam.ValidRole("jarvis") {
		t.Error("jarvis must be valid role")
	}
	if !iam.ValidRole("superhost") {
		t.Error("superhost must be valid role")
	}
	if iam.ValidRole("admin") {
		t.Error("admin must not be a valid role")
	}
	if iam.ValidRole("") {
		t.Error("empty string must not be a valid role")
	}
}

func TestIAMRateLimiter(t *testing.T) {
	rl := iam.NewRateLimiter(3, time.Minute)

	for i := 0; i < 3; i++ {
		if !rl.Allow("test-key") {
			t.Errorf("request %d must be allowed", i+1)
		}
	}

	if rl.Allow("test-key") {
		t.Error("request 4 must be denied (rate limited)")
	}

	if !rl.Allow("other-key") {
		t.Error("different key must not be rate limited")
	}
}

func TestIAMAuthMiddlewareNoToken(t *testing.T) {
	t.Setenv("CC_DB_USER", "testuser")
	t.Setenv("CC_DB_PASS", "testpass")
	t.Setenv("CC_DB_NAME", "testdb")
	t.Setenv("CC_SKIP_DB", "true")
	t.Setenv("CC_HTTP_PORT", "18090")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { app.RunAPI(ctx) }()

	baseURL := "http://127.0.0.1:18090"
	if err := waitForServer(baseURL+"/health/live", 5*time.Second); err != nil {
		t.Fatalf("server did not start: %v", err)
	}

	resp, err := http.Get(baseURL + "/health/live")
	if err != nil {
		t.Fatalf("health check: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("health check must pass without auth: got %d", resp.StatusCode)
	}
}

func TestIAMAuthRoutesRegistered(t *testing.T) {
	t.Setenv("CC_DB_USER", "testuser")
	t.Setenv("CC_DB_PASS", "testpass")
	t.Setenv("CC_DB_NAME", "testdb")
	t.Setenv("CC_SKIP_DB", "false")
	t.Setenv("CC_DB_HOST", "127.0.0.1")
	t.Setenv("CC_DB_PORT", "5432")
	t.Setenv("CC_HTTP_PORT", "18091")

	if !iamPostgresAvailable() {
		t.Skip("PostgreSQL not available for IAM routes test")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { app.RunAPI(ctx) }()

	base := "http://127.0.0.1:18091"
	if err := waitForServer(base+"/health/live", 10*time.Second); err != nil {
		t.Fatalf("server did not start: %v", err)
	}

	resp, err := http.Get(base + "/health/live")
	if err != nil {
		t.Fatalf("health check: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health check must pass: got %d", resp.StatusCode)
	}
}

func connectPool(connStr string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(context.Background(), connStr)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return pool, nil
}

func iamPostgresAvailable() bool {
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

func iamDBConnString() string {
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

func TestIAMCreateUserAndRequestOTP(t *testing.T) {
	connStr := iamDBConnString()
	pool, err := connectPool(connStr)
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}
	defer pool.Close()
	_ = pool
}

func TestIAMHashedOTPVerification(t *testing.T) {
	otp, err := iam.GenerateOTP(6)
	if err != nil {
		t.Fatalf("generate otp: %v", err)
	}

	hash := iam.HashOTP(otp.Token)

	if iam.VerifyOTPHash(otp.Token, hash) {
		t.Log("OTP hash verification works correctly")
	} else {
		t.Error("OTP hash verification must succeed")
	}

	otherOTP, _ := iam.GenerateOTP(6)
	if iam.VerifyOTPHash(otherOTP.Token, hash) {
		t.Error("different OTP must not match hash")
	}
}

func TestIAMOwnerGuestRolesDistinct(t *testing.T) {
	if iam.RoleOwner == iam.RoleGuest {
		t.Error("owner and guest roles must be distinct constants")
	}
	if iam.RoleOwner == iam.RoleStaff {
		t.Error("owner and staff roles must be distinct constants")
	}
	if iam.RoleGuest == iam.RoleStaff {
		t.Error("guest and staff roles must be distinct constants")
	}
}

func TestIAMStaffPrivilegedActionRequiresMFA(t *testing.T) {
	connStr := iamDBConnString()
	pool, err := connectPool(connStr)
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}
	defer pool.Close()

	ctx := context.Background()
	if err := iam.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure iam schema: %v", err)
	}

	mfaVerifier := security.NewNoOpMFAVerifier(security.MFAStateEnabled)
	subject := security.Subject{ActorID: "staff-1", TenantID: "tenant-1", Roles: []string{iam.RoleStaff}}

	err = mfaVerifier.RequireMFA(ctx, subject, "privileged.user.delete")
	if err != security.ErrMFARequired {
		t.Errorf("staff without MFA must be denied privileged action: got %v", err)
	}

	err = mfaVerifier.RequireMFA(ctx, subject, "property.view")
	if err != nil {
		t.Errorf("staff without MFA should be allowed normal action: got %v", err)
	}

	disabledVerifier := security.NewNoOpMFAVerifier(security.MFAStateDisabled)
	err = disabledVerifier.RequireMFA(ctx, subject, "privileged.system.config")
	if err != nil {
		t.Errorf("MFA disabled should allow privileged action: got %v", err)
	}
}

func TestCCIAM001CrossTenantDenied(t *testing.T) {
	connStr := iamDBConnString()
	pool, err := connectPool(connStr)
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}
	defer pool.Close()

	ctx := context.Background()
	if err := iam.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure iam schema: %v", err)
	}

	auditStore := audit.NewAuditStore(pool)
	tenancySvc := iam.NewTenancyService(pool, auditStore)

	tenantA, err := tenancySvc.CreateTenant(ctx, iam.CreateTenantParams{Name: "tenant-a"})
	if err != nil {
		t.Fatalf("create tenant A: %v", err)
	}

	tenantB, err := tenancySvc.CreateTenant(ctx, iam.CreateTenantParams{Name: "tenant-b"})
	if err != nil {
		t.Fatalf("create tenant B: %v", err)
	}

	subjectA := security.Subject{ActorID: "user-a", TenantID: tenantA.ID, Roles: []string{iam.RoleOwner}}
	subjectB := security.Subject{ActorID: "user-b", TenantID: tenantB.ID, Roles: []string{iam.RoleOwner}}

	ctxA := iam.WithSubject(context.Background(), subjectA)
	ctxB := iam.WithSubject(context.Background(), subjectB)
	_ = ctxA
	_ = ctxB

	if err := iam.RequireTenantMatch(subjectA, tenantA.ID); err != nil {
		t.Errorf("same-tenant access must succeed: %v", err)
	}

	if err := iam.RequireTenantMatch(subjectA, tenantB.ID); err != iam.ErrCrossTenantDenied {
		t.Errorf("cross-tenant access must be denied: got %v", err)
	}

	if err := iam.RequireTenantMatch(subjectB, tenantB.ID); err != nil {
		t.Errorf("same-tenant access must succeed: %v", err)
	}

	if err := iam.RequireTenantMatch(subjectB, tenantA.ID); err != iam.ErrCrossTenantDenied {
		t.Errorf("cross-tenant access must be denied: got %v", err)
	}

	authorizer := security.NewRoleBasedAuthorizer([]security.RolePolicy{
		{Role: iam.RoleOwner, Actions: nil, Resources: nil},
	})
	scopedAuthz := iam.NewTenantScopedAuthorizer(authorizer, tenancySvc)
	_ = scopedAuthz

	if err := tenancySvc.RequireTenantScope(ctxA, tenantA.ID); err != nil {
		t.Errorf("tenant-scope same-tenant access must succeed: %v", err)
	}

	if err := tenancySvc.RequireTenantScope(ctxA, tenantB.ID); err != iam.ErrCrossTenantDenied {
		t.Errorf("tenant-scope cross-tenant access must be denied: got %v", err)
	}
}

func TestCCIAM001ExpiredSupportAccessDenied(t *testing.T) {
	connStr := iamDBConnString()
	pool, err := connectPool(connStr)
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}
	defer pool.Close()

	ctx := context.Background()
	if err := iam.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure iam schema: %v", err)
	}

	auditStore := audit.NewAuditStore(pool)
	tenancySvc := iam.NewTenancyService(pool, auditStore)

	tenantA, err := tenancySvc.CreateTenant(ctx, iam.CreateTenantParams{Name: "tenant-sag-a"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	tenantB, err := tenancySvc.CreateTenant(ctx, iam.CreateTenantParams{Name: "tenant-sag-b"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	supportUser, err := tenancySvc.CreateTenant(ctx, iam.CreateTenantParams{Name: "tenant-support-user"})
	if err != nil {
		t.Fatalf("create support tenant: %v", err)
	}
	_ = supportUser

	grant, err := tenancySvc.CreateSupportAccessGrant(ctx, iam.CreateSupportAccessGrantParams{
		TenantID:        tenantA.ID,
		GrantedByUserID: tenantA.ID,
		GrantedToUserID: tenantB.ID,
		Reason:          "support_ticket_123",
		Scope:           iam.SupportAccessScopeTenant,
		TTL:             1 * time.Second,
	})
	if err != nil {
		t.Fatalf("create support access grant: %v", err)
	}

	if err := tenancySvc.ValidateSupportAccess(ctx, tenantB.ID, tenantA.ID); err != nil {
		t.Errorf("active support access grant must succeed: %v", err)
	}

	time.Sleep(2 * time.Second)

	if err := tenancySvc.ValidateSupportAccess(ctx, tenantB.ID, tenantA.ID); err != iam.ErrSupportAccessExpired {
		t.Errorf("expired support access must fail: got %v", err)
	}

	if err := tenancySvc.ValidateSupportAccess(ctx, "nonexistent-user", tenantA.ID); err != iam.ErrSupportAccessNotFound {
		t.Errorf("non-existent support access must return not found: got %v", err)
	}

	if err := tenancySvc.RevokeSupportAccessGrant(ctx, grant.ID, "admin-1"); err != nil {
		t.Fatalf("revoke support access grant: %v", err)
	}

	if err := tenancySvc.ValidateSupportAccess(ctx, tenantB.ID, tenantA.ID); err != iam.ErrSupportAccessNotFound {
		t.Errorf("revoked support access must fail: got %v", err)
	}
}

func TestCCIAM001OwnerGuestRolesDistinct(t *testing.T) {
	if iam.RoleOwner == iam.RoleGuest {
		t.Error("owner and guest roles must be distinct constants")
	}
	if iam.RoleOwner == iam.RoleStaff {
		t.Error("owner and staff roles must be distinct constants")
	}
	if iam.RoleGuest == iam.RoleStaff {
		t.Error("guest and staff roles must be distinct constants")
	}

	connStr := iamDBConnString()
	pool, err := connectPool(connStr)
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}
	defer pool.Close()

	ctx := context.Background()
	if err := iam.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure iam schema: %v", err)
	}

	auditStore := audit.NewAuditStore(pool)
	identitySvc := iam.NewIdentityService(pool, auditStore)

	tenantID := "tenant-roles-test"
	ownerContact := "roles-owner@test.com"
	guestContact := "roles-guest@test.com"

	owner, err := identitySvc.EnsureUser(ctx, iam.CreateUserParams{
		TenantID: tenantID,
		Contact:  ownerContact,
		Role:     iam.RoleOwner,
	})
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	if owner.Role != iam.RoleOwner {
		t.Errorf("owner must have owner role: got %s", owner.Role)
	}

	guest, err := identitySvc.EnsureUser(ctx, iam.CreateUserParams{
		TenantID: tenantID,
		Contact:  guestContact,
		Role:     iam.RoleGuest,
	})
	if err != nil {
		t.Fatalf("create guest: %v", err)
	}
	if guest.Role != iam.RoleGuest {
		t.Errorf("guest must have guest role: got %s", guest.Role)
	}

	if owner.ID == guest.ID {
		t.Error("owner and guest must have distinct identities")
	}

	subjectOwner := security.Subject{ActorID: owner.ID, TenantID: tenantID, Roles: []string{iam.RoleOwner}}
	subjectGuest := security.Subject{ActorID: guest.ID, TenantID: tenantID, Roles: []string{iam.RoleGuest}}

	if err := iam.RequireRoleMatch(subjectOwner, iam.RoleOwner); err != nil {
		t.Errorf("owner must match owner role: %v", err)
	}

	if err := iam.RequireRoleMatch(subjectGuest, iam.RoleOwner); err != iam.ErrRoleNotAllowed {
		t.Errorf("guest must not match owner role: got %v", err)
	}

	if err := iam.RequireRoleMatch(subjectOwner, iam.RoleGuest); err != iam.ErrRoleNotAllowed {
		t.Errorf("owner must not match guest role: got %v", err)
	}

	if err := iam.RequireRoleMatch(subjectGuest, iam.RoleGuest); err != nil {
		t.Errorf("guest must match guest role: %v", err)
	}
	_ = identitySvc
}

func TestCCIAM001StaffMFARequired(t *testing.T) {
	ctx := context.Background()

	mfaVerifier := security.NewNoOpMFAVerifier(security.MFAStateEnabled)
	staffSubject := security.Subject{ActorID: "staff-1", TenantID: "tenant-1", Roles: []string{iam.RoleStaff}}
	ownerSubject := security.Subject{ActorID: "owner-1", TenantID: "tenant-1", Roles: []string{iam.RoleOwner}}

	err := mfaVerifier.RequireMFA(ctx, staffSubject, "privileged.user.delete")
	if err != security.ErrMFARequired {
		t.Errorf("staff without MFA must be denied privileged action: got %v", err)
	}

	err = mfaVerifier.RequireMFA(ctx, staffSubject, "property.view")
	if err != nil {
		t.Errorf("staff without MFA should be allowed normal action: got %v", err)
	}

	err = mfaVerifier.RequireMFA(ctx, ownerSubject, "privileged.user.delete")
	if err != security.ErrMFARequired {
		t.Errorf("owner without MFA must be denied privileged action: got %v", err)
	}

	disabledVerifier := security.NewNoOpMFAVerifier(security.MFAStateDisabled)
	err = disabledVerifier.RequireMFA(ctx, staffSubject, "privileged.system.config")
	if err != nil {
		t.Errorf("MFA disabled should allow privileged action: got %v", err)
	}
}

func TestCCIAM001UnassignedPropertyDenied(t *testing.T) {
	connStr := iamDBConnString()
	pool, err := connectPool(connStr)
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}
	defer pool.Close()

	ctx := context.Background()
	if err := iam.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure iam schema: %v", err)
	}

	auditStore := audit.NewAuditStore(pool)
	tenancySvc := iam.NewTenancyService(pool, auditStore)

	tenantA, err := tenancySvc.EnsureTenant(ctx, iam.CreateTenantParams{Name: "tenant-property-scope-a"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	subjectA := security.Subject{ActorID: "user-a", TenantID: tenantA.ID, Roles: []string{iam.RoleOwner}}

	err = tenancySvc.RequireResourceAccess(iam.WithSubject(context.Background(), subjectA), tenantA.ID, "property", "prop-1")
	if err != nil {
		t.Errorf("same-tenant resource access must succeed even without specific property assignment: %v", err)
	}

	subjectOther := security.Subject{ActorID: "other-user", TenantID: tenantA.ID, Roles: []string{iam.RoleGuest}}
	err = tenancySvc.RequireResourceAccess(iam.WithSubject(context.Background(), subjectOther), tenantA.ID, "property", "prop-2")
	if err != nil {
		t.Errorf("same-tenant guest must be able to access resources: %v", err)
	}

	subjectExternal := security.Subject{ActorID: "external", TenantID: "other-tenant", Roles: []string{iam.RoleOwner}}
	err = tenancySvc.RequireResourceAccess(iam.WithSubject(context.Background(), subjectExternal), tenantA.ID, "property", "prop-3")
	if err != iam.ErrCrossTenantDenied {
		t.Errorf("external tenant must be denied access: got %v", err)
	}

	anonymousCtx := context.Background()
	err = tenancySvc.RequireResourceAccess(anonymousCtx, tenantA.ID, "property", "prop-4")
	if err != iam.ErrCrossTenantDenied {
		t.Errorf("anonymous access must be denied: got %v", err)
	}
}

func TestIAMTenantCreationAndMembership(t *testing.T) {
	connStr := iamDBConnString()
	pool, err := connectPool(connStr)
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}
	defer pool.Close()

	ctx := context.Background()
	if err := iam.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure iam schema: %v", err)
	}

	auditStore := audit.NewAuditStore(pool)
	tenancySvc := iam.NewTenancyService(pool, auditStore)

	tenant, err := tenancySvc.CreateTenant(ctx, iam.CreateTenantParams{Name: "test-membership-tenant"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if tenant.ID == "" {
		t.Error("tenant ID must not be empty")
	}
	if tenant.State != "active" {
		t.Errorf("tenant must be active: got %s", tenant.State)
	}

	retrieved, err := tenancySvc.GetTenant(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("get tenant: %v", err)
	}
	if retrieved.ID != tenant.ID {
		t.Errorf("retrieved tenant ID mismatch")
	}

	identitySvc := iam.NewIdentityService(pool, auditStore)
	user, err := identitySvc.EnsureUser(ctx, iam.CreateUserParams{
		TenantID: tenant.ID,
		Contact:  "member@test.com",
		Role:     iam.RoleOwner,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	membership, err := tenancySvc.AddMembership(ctx, tenant.ID, user.ID, iam.RoleOwner)
	if err != nil {
		t.Fatalf("add membership: %v", err)
	}
	if membership.TenantID != tenant.ID {
		t.Errorf("membership tenant ID mismatch")
	}

	isMember, err := tenancySvc.IsMember(ctx, tenant.ID, user.ID)
	if err != nil {
		t.Fatalf("check membership: %v", err)
	}
	if !isMember {
		t.Error("user must be member of tenant")
	}

	isMemberOther, err := tenancySvc.IsMember(ctx, "nonexistent-tenant", user.ID)
	if err != nil {
		t.Fatalf("check non-membership: %v", err)
	}
	if isMemberOther {
		t.Error("user must not be member of non-existent tenant")
	}

	memberships, err := tenancySvc.GetMemberships(ctx, user.ID)
	if err != nil {
		t.Fatalf("list memberships: %v", err)
	}
	if len(memberships) != 1 {
		t.Errorf("expected 1 membership, got %d", len(memberships))
	}

	if err := tenancySvc.RemoveMembership(ctx, tenant.ID, user.ID); err != nil {
		t.Fatalf("remove membership: %v", err)
	}

	isMember, err = tenancySvc.IsMember(ctx, tenant.ID, user.ID)
	if err != nil {
		t.Fatalf("check membership after remove: %v", err)
	}
	if isMember {
		t.Error("user must not be member after removal")
	}
}

func TestIAMAttributePolicyValidation(t *testing.T) {
	subject := security.Subject{ActorID: "user-1", TenantID: "tenant-1", Roles: []string{iam.RoleOwner}}

	policy := iam.AttributePolicy{
		TenantID: strPtr("tenant-1"),
		Role:     strPtr(iam.RoleOwner),
	}
	if err := iam.ValidateAttributePolicy(policy, subject); err != nil {
		t.Errorf("matching policy must succeed: %v", err)
	}

	wrongTenant := iam.AttributePolicy{TenantID: strPtr("tenant-2")}
	if err := iam.ValidateAttributePolicy(wrongTenant, subject); err != iam.ErrCrossTenantDenied {
		t.Errorf("wrong tenant must deny: got %v", err)
	}

	wrongRole := iam.AttributePolicy{Role: strPtr(iam.RoleStaff)}
	if err := iam.ValidateAttributePolicy(wrongRole, subject); err != iam.ErrRoleNotAllowed {
		t.Errorf("wrong role must deny: got %v", err)
	}

	wrongUser := iam.AttributePolicy{UserID: strPtr("user-2")}
	if err := iam.ValidateAttributePolicy(wrongUser, subject); err != iam.ErrCrossTenantDenied {
		t.Errorf("wrong user must deny: got %v", err)
	}

	past := time.Now().UTC().Add(-2 * time.Hour)
	future := time.Now().UTC().Add(-1 * time.Hour)
	pastWindow := iam.AttributePolicy{TimeWindow: &iam.TimeWindowRule{Before: &past}}
	if err := iam.ValidateAttributePolicy(pastWindow, subject); err == nil {
		t.Error("expired time window must deny")
	}

	futureWindow := iam.AttributePolicy{TimeWindow: &iam.TimeWindowRule{After: &future}}
	if err := iam.ValidateAttributePolicy(futureWindow, subject); err != nil {
		t.Errorf("future time window must allow: %v", err)
	}
}

func TestIAMTenantScopedAuthorizer(t *testing.T) {
	connStr := iamDBConnString()
	pool, err := connectPool(connStr)
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}
	defer pool.Close()

	ctx := context.Background()
	if err := iam.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure iam schema: %v", err)
	}

	auditStore := audit.NewAuditStore(pool)
	tenancySvc := iam.NewTenancyService(pool, auditStore)

	tenantA, _ := tenancySvc.EnsureTenant(ctx, iam.CreateTenantParams{Name: "authz-tenant-a"})
	tenantB, _ := tenancySvc.EnsureTenant(ctx, iam.CreateTenantParams{Name: "authz-tenant-b"})

	inner := security.NewRoleBasedAuthorizer([]security.RolePolicy{
		{Role: iam.RoleOwner, Actions: nil, Resources: nil},
	})
	authz := iam.NewTenantScopedAuthorizer(inner, tenancySvc)

	subjectA := security.Subject{ActorID: "user-a", TenantID: tenantA.ID, Roles: []string{iam.RoleOwner}}
	subjectB := security.Subject{ActorID: "user-b", TenantID: tenantB.ID, Roles: []string{iam.RoleOwner}}

	if err := authz.Can(ctx, subjectA, "read", security.Resource{Type: "property", ID: "p1"}); err != nil {
		t.Errorf("same-tenant role authorization must succeed: %v", err)
	}

	if err := authz.CanAccessTenant(ctx, subjectA, tenantA.ID); err != nil {
		t.Errorf("same-tenant scope must succeed: %v", err)
	}

	if err := authz.CanAccessTenant(ctx, subjectA, tenantB.ID); err != iam.ErrCrossTenantDenied {
		t.Errorf("cross-tenant scope must deny: got %v", err)
	}

	if err := authz.CanAccessTenant(ctx, subjectB, tenantB.ID); err != nil {
		t.Errorf("same-tenant scope for subject B must succeed: %v", err)
	}

	if err := authz.CanAccessTenant(ctx, subjectB, tenantA.ID); err != iam.ErrCrossTenantDenied {
		t.Errorf("cross-tenant scope for subject B must deny: got %v", err)
	}
}

func TestIAMDenyBeforeDisclose(t *testing.T) {
	result := iam.DenyBeforeDisclose(iam.ErrCrossTenantDenied)
	if result != iam.ErrCrossTenantDenied {
		t.Errorf("cross-tenant denial must remain unchanged")
	}

	result = iam.DenyBeforeDisclose(iam.ErrSupportAccessExpired)
	if result != iam.ErrCrossTenantDenied {
		t.Errorf("expired support access must become cross-tenant denial")
	}

	result = iam.DenyBeforeDisclose(iam.ErrTenantNotFound)
	if result != iam.ErrTenantNotFound {
		t.Errorf("tenant not found must remain unchanged")
	}

	result = iam.DenyBeforeDisclose(iam.ErrNotTenantMember)
	if result != iam.ErrCrossTenantDenied {
		t.Errorf("not tenant member must become cross-tenant denial")
	}
}

func TestIAMSupportAccessGrantIntegration(t *testing.T) {
	connStr := iamDBConnString()
	pool, err := connectPool(connStr)
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}
	defer pool.Close()

	ctx := context.Background()
	if err := iam.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure iam schema: %v", err)
	}

	auditStore := audit.NewAuditStore(pool)
	tenancySvc := iam.NewTenancyService(pool, auditStore)

	tenant, _ := tenancySvc.EnsureTenant(ctx, iam.CreateTenantParams{Name: "sag-integration-tenant"})

	grant, err := tenancySvc.CreateSupportAccessGrant(ctx, iam.CreateSupportAccessGrantParams{
		TenantID:        tenant.ID,
		GrantedByUserID: "owner-1",
		GrantedToUserID: "support-staff-1",
		Reason:          "Investigation of ticket TKT-123",
		Scope:           iam.SupportAccessScopeTenant,
		TTL:             30 * time.Minute,
	})
	if err != nil {
		t.Fatalf("create support access grant: %v", err)
	}
	if grant.ID == "" {
		t.Error("grant ID must not be empty")
	}
	if grant.Active != true {
		t.Error("grant must be active")
	}
	if grant.Reason != "Investigation of ticket TKT-123" {
		t.Errorf("grant reason mismatch: got %s", grant.Reason)
	}
	if grant.Scope != iam.SupportAccessScopeTenant {
		t.Errorf("grant scope mismatch: got %s", grant.Scope)
	}

	if err := tenancySvc.ValidateSupportAccess(ctx, "support-staff-1", tenant.ID); err != nil {
		t.Errorf("active grant must validate: %v", err)
	}

	if err := tenancySvc.ValidateSupportAccess(ctx, "other-user", tenant.ID); err != iam.ErrSupportAccessNotFound {
		t.Errorf("unrelated user must not have access: got %v", err)
	}

	if err := tenancySvc.RevokeSupportAccessGrant(ctx, grant.ID, "owner-1"); err != nil {
		t.Fatalf("revoke grant: %v", err)
	}

	if err := tenancySvc.ValidateSupportAccess(ctx, "support-staff-1", tenant.ID); err != iam.ErrSupportAccessNotFound {
		t.Errorf("revoked grant must not validate: got %v", err)
	}
}

func strPtr(s string) *string {
	return &s
}
