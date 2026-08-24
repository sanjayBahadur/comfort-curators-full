package compliance

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"comfort-curators-backend/internal/iam"
	"comfort-curators-backend/internal/platform/audit"
	"comfort-curators-backend/internal/platform/security"
	"comfort-curators-backend/internal/platform/testdb"

	"github.com/jackc/pgx/v5/pgxpool"
)

type testAuthorizer struct{}

func (testAuthorizer) RequireResourceAccess(ctx context.Context, resourceTenantID, resourceType, resourceID string) error {
	return nil
}

func postgresAvailable() bool {
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

func dbConnString() string {
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
	return "postgres://" + user + ":" + pass + "@" + host + ":" + port + "/" + name + "?sslmode=disable"
}

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if !postgresAvailable() {
		t.Skip("PostgreSQL not available")
	}
	pool, err := pgxpool.New(context.Background(), dbConnString())
	if err != nil {
		t.Skipf("PostgreSQL connect failed: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(context.Background()); err != nil {
		t.Skipf("PostgreSQL ping failed: %v", err)
	}
	return pool
}

func testHandlerWithService(t *testing.T) *ComplianceHandler {
	t.Helper()
	pool := testPool(t)
	ctx := context.Background()
	if err := EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure compliance schema: %v", err)
	}
	if err := audit.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure audit schema: %v", err)
	}
	auditStore := audit.NewAuditStore(pool)
	svc := NewComplianceService(pool, auditStore).WithAuthorizer(testAuthorizer{})
	return NewComplianceHandler(svc)
}

func TestComplianceHandlerRoleDenied(t *testing.T) {
	handler := NewComplianceHandler(nil)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	cases := []struct {
		method string
		path   string
		role   string
	}{
		{http.MethodPost, "/v1/compliance/items", RoleJarvis},
		{http.MethodPost, "/v1/compliance/items", RoleSuperhost},
		{http.MethodPost, "/v1/compliance/items/test-item/renew", RoleJarvis},
		{http.MethodPost, "/v1/compliance/items/test-item/renew", RoleSuperhost},
		{http.MethodPost, "/v1/compliance/scan-expiry", RoleJarvis},
		{http.MethodPost, "/v1/compliance/scan-expiry", RoleSuperhost},
	}

	for _, tc := range cases {
		testName := tc.role + "_" + tc.method + "_" + pathLabel(tc.path)
		t.Run(testName, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			ctx := iam.WithSubject(req.Context(), security.Subject{
				ActorID:  "actor-1",
				TenantID: "tenant-1",
				Roles:    []string{tc.role},
			})
			req = req.WithContext(ctx)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			resp := rec.Result()
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusForbidden {
				body, _ := io.ReadAll(resp.Body)
				t.Errorf("expected 403 Forbidden for role %q on %s %s, got %d: %s",
					tc.role, tc.method, tc.path, resp.StatusCode, string(body))
			}
		})
	}
}

func TestComplianceHandlerRoleAllowedCreateAndRenew(t *testing.T) {
	handler := NewComplianceHandler(nil)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	subject := security.Subject{
		ActorID:  "actor-1",
		TenantID: "tenant-1",
		Roles:    []string{"owner"},
	}

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/v1/compliance/items"},
		{http.MethodPost, "/v1/compliance/items/test-item/renew"},
	}

	for _, tc := range cases {
		testName := "allowed_owner_" + pathLabel(tc.path)
		t.Run(testName, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			ctx := iam.WithSubject(req.Context(), subject)
			req = req.WithContext(ctx)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			resp := rec.Result()
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusForbidden {
				body, _ := io.ReadAll(resp.Body)
				t.Errorf("expected non-gated role to pass role check on %s %s, got 403: %s",
					tc.method, tc.path, string(body))
			}
		})
	}
}

func TestComplianceHandlerRoleAllowedScanExpiry(t *testing.T) {
	handler := testHandlerWithService(t)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	subject := security.Subject{
		ActorID:  "actor-1",
		TenantID: "tenant-1",
		Roles:    []string{"owner"},
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/compliance/scan-expiry", nil)
	ctx := iam.WithSubject(req.Context(), subject)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected non-gated role to pass role check on scanExpiry, got 403: %s", string(body))
	}
}

func pathLabel(path string) string {
	s := path
	if len(s) > 0 && s[0] == '/' {
		s = s[1:]
	}
	result := ""
	for _, c := range s {
		if c == '/' {
			result += "_"
		} else if c == '-' {
			result += "_"
		} else if c == '{' || c == '}' {
			continue
		} else {
			result += string(c)
		}
	}
	return result
}
