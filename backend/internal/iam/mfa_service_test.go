package iam

import (
	"context"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"comfort-curators-backend/internal/platform/audit"
	"comfort-curators-backend/internal/platform/security"
	"comfort-curators-backend/internal/platform/testdb"
)

func iamTestDBConnString() string {
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

func iamTestDBAvailable() bool {
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

func connectTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if !iamTestDBAvailable() {
		t.Skip("PostgreSQL not available for IAM integration test")
	}
	pool, err := pgxpool.New(context.Background(), iamTestDBConnString())
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Fatalf("ping postgres: %v", err)
	}
	return pool
}

func TestMFATOTPEnrollmentAndVerification(t *testing.T) {
	pool := connectTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	if err := EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}

	auditStore := audit.NewAuditStore(pool)
	svc := NewIdentityService(pool, auditStore)

	tenantID := "tenant-mfa-test"
	user, err := svc.EnsureUser(ctx, CreateUserParams{
		TenantID: tenantID,
		Contact:  "mfa-test@example.com",
		Role:     RoleStaff,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	privilegedAction := security.Action("privileged.user.delete")
	subject := security.Subject{ActorID: user.ID, TenantID: tenantID, Roles: []string{RoleStaff}}

	// Enroll returns an unverified method and a usable secret.
	enrollment, err := svc.EnrollMFA(ctx, user.ID)
	if err != nil {
		t.Fatalf("enroll mfa: %v", err)
	}
	if enrollment.Secret == "" {
		t.Fatal("enrollment must return a secret")
	}
	if enrollment.ProvisioningURI == "" {
		t.Fatal("enrollment must return a provisioning URI")
	}
	if enrollment.Method.Verified {
		t.Fatal("newly enrolled method must not be verified")
	}

	// An unconfirmed enrollment must not satisfy MFA required.
	err = svc.CheckMFARequired(ctx, subject, privilegedAction)
	if err != security.ErrMFARequired {
		t.Fatalf("unconfirmed enrollment must require MFA: got %v", err)
	}

	// Confirm with a freshly-generated valid code succeeds.
	code, err := TOTPCode(enrollment.Secret, time.Now().UTC())
	if err != nil {
		t.Fatalf("compute confirmation code: %v", err)
	}
	if err := svc.ConfirmMFA(ctx, ConfirmMFAParams{UserID: user.ID, Code: code}); err != nil {
		t.Fatalf("confirm with valid code: %v", err)
	}

	// Confirm with a wrong code now fails because there is no pending enrollment.
	if err := svc.ConfirmMFA(ctx, ConfirmMFAParams{UserID: user.ID, Code: "000000"}); err != ErrMFAEnrollmentNotFound {
		t.Fatalf("confirm without pending enrollment must fail: got %v", err)
	}

	// Re-enrollment should fail now that a verified method exists.
	if _, err := svc.EnrollMFA(ctx, user.ID); err != ErrMFAAlreadyEnrolled {
		t.Fatalf("enroll after verification must fail: got %v", err)
	}

	// A verified enrollment still requires a fresh challenge for privileged
	// actions until the challenge has been satisfied.
	err = svc.CheckMFARequired(ctx, subject, privilegedAction)
	if err != security.ErrMFARequired {
		t.Fatalf("verified enrollment must require fresh MFA for privileged action: got %v", err)
	}

	// Verify with a freshly-generated valid code succeeds and records
	// satisfaction so that recent-MFA checks pass without another code.
	verifyCode, err := TOTPCode(enrollment.Secret, time.Now().UTC())
	if err != nil {
		t.Fatalf("compute verification code: %v", err)
	}
	if err := svc.VerifyMFA(ctx, VerifyMFAParams{UserID: user.ID, Code: verifyCode}); err != nil {
		t.Fatalf("verify with valid code: %v", err)
	}

	err = svc.CheckMFARequired(ctx, subject, privilegedAction)
	if err != nil {
		t.Fatalf("recently satisfied MFA must not require fresh code: %v", err)
	}

	// Verify with a wrong code fails.
	if err := svc.VerifyMFA(ctx, VerifyMFAParams{UserID: user.ID, Code: "000000"}); err != ErrMFAInvalid {
		t.Fatalf("verify with wrong code must fail: got %v", err)
	}

	// Verify with a stale code (outside the skew window) fails.
	staleTime := time.Now().UTC().Add(-3 * TOTPPeriod)
	staleCode, err := TOTPCode(enrollment.Secret, staleTime)
	if err != nil {
		t.Fatalf("compute stale code: %v", err)
	}
	if err := svc.VerifyMFA(ctx, VerifyMFAParams{UserID: user.ID, Code: staleCode}); err != ErrMFAInvalid {
		t.Fatalf("verify with stale code must fail: got %v", err)
	}
}

func TestMFANoVerifiedMethodRequiresMFAForStaff(t *testing.T) {
	pool := connectTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	if err := EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}

	auditStore := audit.NewAuditStore(pool)
	svc := NewIdentityService(pool, auditStore)

	tenantID := "tenant-mfa-staff"
	user, err := svc.EnsureUser(ctx, CreateUserParams{
		TenantID: tenantID,
		Contact:  "staff-mfa@example.com",
		Role:     RoleStaff,
	})
	if err != nil {
		t.Fatalf("create staff user: %v", err)
	}

	subject := security.Subject{ActorID: user.ID, TenantID: tenantID, Roles: []string{RoleStaff}}
	err = svc.CheckMFARequired(ctx, subject, security.Action("privileged.system.config"))
	if err != security.ErrMFARequired {
		t.Fatalf("staff without verified MFA must require MFA: got %v", err)
	}
}

func TestOTPRequestDoesNotGrantArbitraryRole(t *testing.T) {
	pool := connectTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	if err := EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}

	auditStore := audit.NewAuditStore(pool)
	svc := NewIdentityService(pool, auditStore)

	tenantID := "tenant-otp-role"
	contact := "otp-role@example.com"

	// A brand-new identity cannot grant itself a privileged role through an
	// unauthenticated OTP request. When no owner exists for the tenant, the
	// safe default is owner (first-time signup); otherwise it would be guest.
	_, err := svc.RequestOTP(ctx, RequestOTPParams{
		TenantID: tenantID,
		Contact:  contact,
		Role:     RoleStaff,
	})
	if err != nil {
		t.Fatalf("request otp for new identity: %v", err)
	}

	firstUser, err := svc.getUserByTenantAndContact(ctx, tenantID, contact)
	if err != nil {
		t.Fatalf("lookup first user: %v", err)
	}
	if firstUser.Role != RoleOwner {
		t.Errorf("first user in tenant must be owner, got %s", firstUser.Role)
	}

	// A second request for the same existing user must authenticate, not
	// change the user's role, even if the caller asks for staff.
	_, err = svc.RequestOTP(ctx, RequestOTPParams{
		TenantID: tenantID,
		Contact:  contact,
		Role:     RoleStaff,
	})
	if err != nil {
		t.Fatalf("request otp for existing user: %v", err)
	}

	secondLookup, err := svc.getUserByTenantAndContact(ctx, tenantID, contact)
	if err != nil {
		t.Fatalf("lookup existing user after second otp: %v", err)
	}
	if secondLookup.Role != RoleOwner {
		t.Errorf("existing user role must not change via OTP request, got %s", secondLookup.Role)
	}

	// A brand-new identity in a tenant that already has an owner must not
	// become staff via the OTP request.
	newContact := "otp-role-guest@example.com"
	_, err = svc.RequestOTP(ctx, RequestOTPParams{
		TenantID: tenantID,
		Contact:  newContact,
		Role:     RoleStaff,
	})
	if err != nil {
		t.Fatalf("request otp for second new identity: %v", err)
	}

	newUser, err := svc.getUserByTenantAndContact(ctx, tenantID, newContact)
	if err != nil {
		t.Fatalf("lookup second user: %v", err)
	}
	if newUser.Role == RoleStaff {
		t.Error("unauthenticated OTP request must not grant staff role")
	}
}
