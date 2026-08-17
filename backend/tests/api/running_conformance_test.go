package api_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"comfort-curators-backend/internal/api"
	"comfort-curators-backend/internal/iam"
	"comfort-curators-backend/internal/platform/security"
	"comfort-curators-backend/internal/property"
)

// tenantAwarePropService models the running API's tenant-scoped property
// service: authorization is checked before existence disclosure, so a
// cross-tenant lookup yields ErrCrossTenantDenied (never a found resource)
// and a missing id yields ErrPropertyNotFound.
type tenantAwarePropService struct {
	properties []property.Property
}

func (s *tenantAwarePropService) CreateProperty(ctx context.Context, params property.CreatePropertyParams, actorID string) (*property.Property, error) {
	return nil, nil
}

func (s *tenantAwarePropService) GetProperty(ctx context.Context, tenantID, propertyID string) (*property.Property, error) {
	for i := range s.properties {
		if s.properties[i].ID != propertyID {
			continue
		}
		if s.properties[i].TenantID != tenantID {
			return nil, property.ErrCrossTenantDenied
		}
		return &s.properties[i], nil
	}
	return nil, property.ErrPropertyNotFound
}

func (s *tenantAwarePropService) ListProperties(ctx context.Context, tenantID string) ([]property.Property, error) {
	var out []property.Property
	for _, p := range s.properties {
		if p.TenantID == tenantID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (s *tenantAwarePropService) TransitionProperty(ctx context.Context, tenantID, propertyID, toState, reason, actorID string) (*property.Property, error) {
	for i := range s.properties {
		if s.properties[i].ID != propertyID {
			continue
		}
		if s.properties[i].TenantID != tenantID {
			return nil, property.ErrCrossTenantDenied
		}
		return &s.properties[i], nil
	}
	return nil, property.ErrPropertyNotFound
}

func (s *tenantAwarePropService) ListTransitions(ctx context.Context, tenantID, propertyID string) ([]property.PropertyTransition, error) {
	return nil, nil
}

func (s *tenantAwarePropService) SetReadiness(ctx context.Context, tenantID, propertyID string, readiness property.Readiness, actorID string) (*property.Property, error) {
	return nil, nil
}

func (s *tenantAwarePropService) AddComplianceHold(ctx context.Context, tenantID, propertyID string, params property.ComplianceHoldParams, actorID string) (*property.ComplianceHold, error) {
	return nil, nil
}

func (s *tenantAwarePropService) ResolveComplianceHold(ctx context.Context, tenantID, propertyID, holdID, actorID string) (*property.Property, error) {
	return nil, nil
}

func (s *tenantAwarePropService) GrantComplianceException(ctx context.Context, tenantID, propertyID, holdID, reviewerID, reason string, ttl time.Duration, actorID string) (*property.Property, error) {
	return nil, nil
}

func tenantAwareSubject(actorID, tenantID string, roles ...string) security.Subject {
	return security.Subject{ActorID: actorID, TenantID: tenantID, Roles: roles}
}

// doServeRaw serves a request without injecting a correlation id header, so
// the handler itself must produce a conformant request_id.
func doServeRaw(mux *http.ServeMux, method, path string, subject security.Subject) *httptest.ResponseRecorder {
	var req *http.Request
	if method == http.MethodGet {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(`{}`))
	}
	if subject.TenantID != "" {
		ctx := iam.WithSubject(req.Context(), subject)
		req = req.WithContext(ctx)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func TestRunningAPISliceConformsWithoutCorrelationHeader(t *testing.T) {
	spec := loadSpec(t)
	svc := sampleProperties()
	mux := setupMux(svc)

	subject := security.Subject{ActorID: "actor-owner-1", TenantID: "tenant-a", Roles: []string{api.RoleOwner}}

	// GET collection and GET resource without any correlation id header. The
	// live slice handler must emit envelopes that conform to the contract.
	for _, path := range []string{"/v1/properties", "/v1/properties/prop_0000000000000001"} {
		w := doServeRaw(mux, http.MethodGet, path, subject)
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s: expected 200, got %d: %s", path, w.Code, w.Body.String())
		}
		body := w.Body.Bytes()
		var validate func([]byte) error
		if path == "/v1/properties" {
			validate = spec.ValidateCollection
		} else {
			validate = spec.ValidateResource
		}
		if err := validate(body); err != nil {
			t.Errorf("GET %s: envelope does not conform: %v (body: %s)", path, err, string(body))
		}
	}
}

func TestRunningAPISliceErrorEnvelopeConformsWithoutCorrelationHeader(t *testing.T) {
	spec := loadSpec(t)
	svc := sampleProperties()
	mux := setupMux(svc)

	// Unauthenticated and cross-tenant paths must fail closed with a 401/403
	// Error envelope that the validator accepts even though no correlation id
	// header was sent. This closes the residual live-handler gap.
	cases := []struct {
		path string
		want int
	}{
		{"/v1/properties", http.StatusUnauthorized},
		{"/v1/properties/prop_0000000000000001", http.StatusUnauthorized},
		{"/v1/properties/prop_0000000000000001/transitions", http.StatusUnauthorized},
	}

	for _, tc := range cases {
		w := doServeRaw(mux, http.MethodGet, tc.path, security.Subject{})
		if w.Code != tc.want {
			t.Fatalf("GET %s: expected %d, got %d: %s", tc.path, tc.want, w.Code, w.Body.String())
		}
		if err := spec.ValidateError(w.Body.Bytes()); err != nil {
			t.Errorf("GET %s: error envelope does not conform: %v (body: %s)", tc.path, err, w.Body.String())
		}
		var errBody api.ErrorBody
		if err := json.Unmarshal(w.Body.Bytes(), &errBody); err != nil {
			t.Fatalf("GET %s: decode error: %v", tc.path, err)
		}
		if errBody.RequestID == "" {
			t.Errorf("GET %s: request_id must be non-empty (residual gap closed), body: %s", tc.path, w.Body.String())
		}
	}
}

func TestCrossTenantGetDeniesBeforeExistenceDisclosure(t *testing.T) {
	spec := loadSpec(t)
	svc := &tenantAwarePropService{
		properties: []property.Property{
			{
				ID:               "prop_0000000000000001",
				TenantID:         "tenant-a",
				OwnerAuthorityID: "auth-owner-1",
				ServiceAddress: property.Address{
					Line1: "14 Marine Drive", City: "Noida", State: "Uttar Pradesh",
					PostalCode: "226001", Country: "IN",
				},
				AccessMethod: "lockbox",
				State:        property.StateLead,
				Version:      1,
			},
		},
	}
	mux := setupMux(svc)

	subjectA := tenantAwareSubject("actor-owner-1", "tenant-a", api.RoleOwner)
	subjectB := tenantAwareSubject("actor-owner-b", "tenant-b", api.RoleOwner)

	// In-tenant access resolves the resource.
	wIn := doServe(mux, "GET", "/v1/properties/prop_0000000000000001", "", subjectA)
	if wIn.Code != http.StatusOK {
		t.Fatalf("in-tenant GET: expected 200, got %d: %s", wIn.Code, wIn.Body.String())
	}

	// Cross-tenant access to the SAME existing property must be denied with a
	// NOT_FOUND that is indistinguishable from a genuinely missing property:
	// authorization precedes and replaces existence disclosure.
	wCross := doServe(mux, "GET", "/v1/properties/prop_0000000000000001", "", subjectB)
	if wCross.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant GET: expected 404, got %d: %s", wCross.Code, wCross.Body.String())
	}
	if err := spec.ValidateError(wCross.Body.Bytes()); err != nil {
		t.Errorf("cross-tenant error envelope does not conform: %v (body: %s)", err, wCross.Body.String())
	}
	for _, leak := range []string{`"data"`, `"access_method"`, `"service_address"`, `prop_0000000000000001`} {
		if strings.Contains(wCross.Body.String(), leak) {
			t.Errorf("cross-tenant denial must not disclose existence, leaked %q: %s", leak, wCross.Body.String())
		}
	}

	wMissing := doServe(mux, "GET", "/v1/properties/prop_0000000000009999", "", subjectB)
	if wMissing.Code != http.StatusNotFound {
		t.Fatalf("missing GET: expected 404, got %d", wMissing.Code)
	}
	if wCross.Body.String() != wMissing.Body.String() {
		t.Errorf("cross-tenant denial must be indistinguishable from a missing property:\ncross: %s\nmissing: %s",
			wCross.Body.String(), wMissing.Body.String())
	}
}

func TestCrossTenantListOnlyExposesOwnTenant(t *testing.T) {
	svc := &tenantAwarePropService{
		properties: []property.Property{
			{ID: "prop_0000000000000001", TenantID: "tenant-a", OwnerAuthorityID: "auth-owner-1", State: property.StateLead, Version: 1},
			{ID: "prop_0000000000000002", TenantID: "tenant-a", OwnerAuthorityID: "auth-owner-2", State: property.StateLead, Version: 1},
			{ID: "prop_0000000000000003", TenantID: "tenant-b", OwnerAuthorityID: "auth-owner-b", State: property.StateLead, Version: 1},
		},
	}
	mux := setupMux(svc)

	subjectA := tenantAwareSubject("actor-owner-1", "tenant-a", api.RoleOwner)
	w := doServe(mux, "GET", "/v1/properties", "", subjectA)
	if w.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var collection api.Collection
	if err := json.Unmarshal(w.Body.Bytes(), &collection); err != nil {
		t.Fatalf("decode collection: %v", err)
	}
	for _, item := range collection.Items {
		if item.ID == "prop_0000000000000003" {
			t.Errorf("tenant-a must not see tenant-b property %s: %s", item.ID, w.Body.String())
		}
	}
}

func TestCrossTenantTransitionDeniedBeforeExistenceDisclosure(t *testing.T) {
	svc := &tenantAwarePropService{
		properties: []property.Property{
			{ID: "prop_0000000000000001", TenantID: "tenant-a", OwnerAuthorityID: "auth-owner-1", State: property.StateLead, Version: 1},
		},
	}
	mux := setupMux(svc)

	subjectB := tenantAwareSubject("actor-owner-b", "tenant-b", api.RoleOwner)
	w := doServe(mux, "POST", "/v1/properties/prop_0000000000000001/transitions", `{"to_state":"qualifying","reason":"x"}`, subjectB)
	if w.Code != http.StatusForbidden {
		t.Fatalf("cross-tenant transition: expected 403, got %d: %s", w.Code, w.Body.String())
	}
	var errBody api.ErrorBody
	if err := json.Unmarshal(w.Body.Bytes(), &errBody); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if errBody.Code != "FORBIDDEN" {
		t.Errorf("expected FORBIDDEN, got %s", errBody.Code)
	}
	if strings.Contains(w.Body.String(), `"data"`) {
		t.Errorf("cross-tenant transition denial must not disclose resource: %s", w.Body.String())
	}
}

func TestFinanceSliceConformsWithoutCorrelationHeader(t *testing.T) {
	spec := loadSpec(t)

	mux := http.NewServeMux()
	api.NewFinanceSliceHandler(nil, nil, nil, noOpAuthorityResolver).RegisterRoutes(mux)

	server := httptest.NewServer(mux)
	defer server.Close()

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/v1/billing/charges"},
		{http.MethodPost, "/v1/billing/invoices"},
		{http.MethodPost, "/v1/billing/credits"},
		{http.MethodPost, "/v1/accounting-exports"},
		{http.MethodGet, "/v1/reports/property-contribution"},
	} {
		req, err := http.NewRequest(tc.method, server.URL+tc.path, strings.NewReader(`{}`))
		if err != nil {
			t.Fatalf("create %s %s: %v", tc.method, tc.path, err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", tc.method, tc.path, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("%s %s: expected 401, got %d: %s", tc.method, tc.path, resp.StatusCode, string(body))
		}
		// No correlation id header was sent; the live slice handler must still
		// emit a conformant error envelope with a non-empty request_id.
		if err := spec.ValidateError(body); err != nil {
			t.Errorf("%s %s: error envelope does not conform: %v (body: %s)", tc.method, tc.path, err, string(body))
		}
	}
}
