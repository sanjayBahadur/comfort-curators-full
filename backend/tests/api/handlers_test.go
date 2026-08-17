package api_test

import (
	"context"
	"encoding/json"
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

type stubPropService struct {
	properties    []property.Property
	transitions   []property.PropertyTransition
	createErr     error
	getErr        error
	listErr       error
	transitionErr error
}

func (s *stubPropService) CreateProperty(ctx context.Context, params property.CreatePropertyParams, actorID string) (*property.Property, error) {
	if s.createErr != nil {
		return nil, s.createErr
	}
	p := &property.Property{
		ID:                "prop_test00000000001",
		TenantID:          params.TenantID,
		OwnerAuthorityID:  params.OwnerAuthorityID,
		ServiceAddress:    params.ServiceAddress,
		GeolocationZone:   params.GeolocationZone,
		Timezone:          params.Timezone,
		EmergencyContacts: params.EmergencyContacts,
		AccessMethod:      params.AccessMethod,
		MaximumOccupancy:  params.MaximumOccupancy,
		State:             params.InitialState,
		Readiness:         property.Readiness{},
		Version:           1,
		CreatedAt:         time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC),
		UpdatedAt:         time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC),
	}
	if p.State == "" {
		p.State = property.StateLead
	}
	return p, nil
}

func (s *stubPropService) GetProperty(ctx context.Context, tenantID, propertyID string) (*property.Property, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	for i := range s.properties {
		if s.properties[i].ID == propertyID {
			return &s.properties[i], nil
		}
	}
	return nil, property.ErrPropertyNotFound
}

func (s *stubPropService) ListProperties(ctx context.Context, tenantID string) ([]property.Property, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.properties, nil
}

func (s *stubPropService) TransitionProperty(ctx context.Context, tenantID, propertyID, toState, reason, actorID string) (*property.Property, error) {
	if s.transitionErr != nil {
		return nil, s.transitionErr
	}
	p, err := s.GetProperty(ctx, tenantID, propertyID)
	if err != nil {
		return nil, err
	}
	p.State = toState
	p.Version++
	return p, nil
}

func (s *stubPropService) ListTransitions(ctx context.Context, tenantID, propertyID string) ([]property.PropertyTransition, error) {
	return s.transitions, nil
}

func (s *stubPropService) SetReadiness(ctx context.Context, tenantID, propertyID string, readiness property.Readiness, actorID string) (*property.Property, error) {
	p, err := s.GetProperty(ctx, tenantID, propertyID)
	if err != nil {
		return nil, err
	}
	p.Readiness = readiness
	p.Version++
	return p, nil
}

func (s *stubPropService) AddComplianceHold(ctx context.Context, tenantID, propertyID string, params property.ComplianceHoldParams, actorID string) (*property.ComplianceHold, error) {
	return &property.ComplianceHold{
		ID:         "hold_000000000000001",
		PropertyID: propertyID,
		TenantID:   tenantID,
		Kind:       params.Kind,
		Severity:   params.Severity,
		Status:     "open",
		Reason:     params.Reason,
		CreatedAt:  time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC),
	}, nil
}

func (s *stubPropService) ResolveComplianceHold(ctx context.Context, tenantID, propertyID, holdID, actorID string) (*property.Property, error) {
	p, err := s.GetProperty(ctx, tenantID, propertyID)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (s *stubPropService) GrantComplianceException(ctx context.Context, tenantID, propertyID, holdID, reviewerID, reason string, ttl time.Duration, actorID string) (*property.Property, error) {
	p, err := s.GetProperty(ctx, tenantID, propertyID)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func ownerAuthorityResolver(actorID string) []string {
	switch actorID {
	case "actor-owner-1":
		return []string{"auth-owner-1"}
	case "actor-owner-2":
		return []string{"auth-owner-2"}
	}
	return nil
}

func sampleProperties() api.PropService {
	return &stubPropService{
		properties: []property.Property{
			{
				ID:               "prop_0000000000000001",
				TenantID:         "tenant-a",
				OwnerAuthorityID: "auth-owner-1",
				ServiceAddress: property.Address{
					Line1: "14 Marine Drive", City: "Noida", State: "Uttar Pradesh",
					PostalCode: "226001", Country: "IN",
				},
				GeolocationZone: "zone-lko-north",
				Timezone:        "Asia/Kolkata",
				EmergencyContacts: []property.EmergencyContact{
					{Name: "Asha", Phone: "+91-9000000000", Role: "neighbour"},
				},
				AccessMethod:     "lockbox",
				MaximumOccupancy: 4,
				State:            property.StateReadyInactive,
				Readiness: property.Readiness{
					OwnerContractAccepted: true, ComplianceComplete: true, MandatoryFieldsSet: true,
				},
				Version:   3,
				CreatedAt: time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC),
			},
			{
				ID:               "prop_0000000000000002",
				TenantID:         "tenant-a",
				OwnerAuthorityID: "auth-owner-2",
				ServiceAddress: property.Address{
					Line1: "22 Park Street", City: "Mumbai", State: "Maharashtra",
					PostalCode: "400001", Country: "IN",
				},
				GeolocationZone: "zone-mum-south",
				Timezone:        "Asia/Kolkata",
				EmergencyContacts: []property.EmergencyContact{
					{Name: "Raj", Phone: "+91-8000000000", Role: "manager"},
				},
				AccessMethod:     "keypad-code",
				MaximumOccupancy: 6,
				State:            property.StateActive,
				Readiness: property.Readiness{
					OwnerContractAccepted: true, ComplianceComplete: true, MandatoryFieldsSet: true,
				},
				Version:   5,
				CreatedAt: time.Date(2026, 6, 15, 8, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC),
			},
		},
	}
}

func setupMux(svc api.PropService) *http.ServeMux {
	handler := api.NewPropertySliceHandler(svc, ownerAuthorityResolver)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	return mux
}

func doServe(mux *http.ServeMux, method, path, body string, subject security.Subject) *httptest.ResponseRecorder {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Correlation-ID", "req_0123456789abcdef")
	if subject.TenantID != "" {
		ctx := iam.WithSubject(req.Context(), subject)
		req = req.WithContext(ctx)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func doServeNoAuth(mux *http.ServeMux, method, path string) *httptest.ResponseRecorder {
	return doServe(mux, method, path, "", security.Subject{})
}

func decodeBody(w *httptest.ResponseRecorder) []byte {
	return w.Body.Bytes()
}

func TestHandlerUnauthenticatedReturns401(t *testing.T) {
	svc := sampleProperties()
	mux := setupMux(svc)

	paths := []string{
		"/v1/properties",
		"/v1/properties/prop_0000000000000001",
		"/v1/properties/prop_0000000000000001/transitions",
	}
	for _, path := range paths {
		w := doServeNoAuth(mux, "GET", path)
		body := decodeBody(w)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("GET %s: expected 401, got %d: %s", path, w.Code, string(body))
		}
		var errBody api.ErrorBody
		if err := json.Unmarshal(body, &errBody); err != nil {
			t.Errorf("GET %s: error response not valid JSON: %v", path, err)
		} else if errBody.Code != "UNAUTHORIZED" {
			t.Errorf("GET %s: expected code UNAUTHORIZED, got %s", path, errBody.Code)
		}
	}
}

func TestOwnerSeesOwnedPropertiesInList(t *testing.T) {
	svc := sampleProperties()
	mux := setupMux(svc)

	subject := security.Subject{ActorID: "actor-owner-1", TenantID: "tenant-a", Roles: []string{api.RoleOwner}}
	w := doServe(mux, "GET", "/v1/properties", "", subject)
	body := decodeBody(w)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, string(body))
	}

	var collection api.Collection
	if err := json.Unmarshal(body, &collection); err != nil {
		t.Fatalf("decode collection: %v", err)
	}

	if len(collection.Items) != 1 {
		t.Fatalf("owner-1 should see 1 property, got %d", len(collection.Items))
	}

	if collection.Items[0].ID != "prop_0000000000000001" {
		t.Errorf("owner-1 should see prop_0000000000000001, got %s", collection.Items[0].ID)
	}
}

func TestOwnerDoesNotSeeForeignProperties(t *testing.T) {
	svc := sampleProperties()
	mux := setupMux(svc)

	subject := security.Subject{ActorID: "actor-owner-2", TenantID: "tenant-a", Roles: []string{api.RoleOwner}}
	w := doServe(mux, "GET", "/v1/properties", "", subject)
	body := decodeBody(w)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, string(body))
	}

	var collection api.Collection
	if err := json.Unmarshal(body, &collection); err != nil {
		t.Fatalf("decode collection: %v", err)
	}

	if len(collection.Items) != 1 {
		t.Fatalf("owner-2 should see 1 property, got %d", len(collection.Items))
	}

	if collection.Items[0].ID != "prop_0000000000000002" {
		t.Errorf("owner-2 should see prop_0000000000000002, got %s", collection.Items[0].ID)
	}
}

func TestGetPropertyExcludesAccessMaterial(t *testing.T) {
	svc := sampleProperties()
	mux := setupMux(svc)

	subject := security.Subject{ActorID: "actor-owner-1", TenantID: "tenant-a", Roles: []string{api.RoleOwner}}
	w := doServe(mux, "GET", "/v1/properties/prop_0000000000000001", "", subject)
	body := decodeBody(w)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, string(body))
	}

	payload := string(body)
	if strings.Contains(payload, "access_method") {
		t.Errorf("property response must not contain access_method: %s", payload)
	}
	if strings.Contains(payload, "lockbox") {
		t.Errorf("property response must not leak access method value: %s", payload)
	}

	var resource api.Resource
	if err := json.Unmarshal(body, &resource); err != nil {
		t.Fatalf("decode resource: %v", err)
	}

	dataMap, ok := resource.Data.(map[string]any)
	if !ok {
		t.Fatalf("resource data is not a map")
	}
	if _, exists := dataMap["access_method"]; exists {
		t.Errorf("resource data must not contain access_method")
	}
}

func TestGetForeignPropertyReturns404(t *testing.T) {
	svc := sampleProperties()
	mux := setupMux(svc)

	subject := security.Subject{ActorID: "actor-owner-1", TenantID: "tenant-a", Roles: []string{api.RoleOwner}}
	w := doServe(mux, "GET", "/v1/properties/prop_0000000000000002", "", subject)
	body := decodeBody(w)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for foreign property, got %d: %s", w.Code, string(body))
	}

	var errBody api.ErrorBody
	if err := json.Unmarshal(body, &errBody); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if errBody.Code != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND, got %s", errBody.Code)
	}
}

func TestCreatePropertyReturnsOrdinaryPayload(t *testing.T) {
	svc := sampleProperties()
	mux := setupMux(svc)

	reqBody := `{
		"owner_authority_id": "auth-owner-1",
		"service_address": {"line1": "1 Test St", "city": "Test", "state": "TS", "postal_code": "12345", "country": "IN"},
		"timezone": "Asia/Kolkata",
		"access_method": "lockbox"
	}`

	subject := security.Subject{ActorID: "actor-owner-1", TenantID: "tenant-a", Roles: []string{api.RoleOwner}}
	w := doServe(mux, "POST", "/v1/properties", reqBody, subject)
	body := decodeBody(w)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, string(body))
	}

	payload := string(body)
	if strings.Contains(payload, "access_method") {
		t.Errorf("create response must not contain access_method: %s", payload)
	}
	if strings.Contains(payload, "lockbox") {
		t.Errorf("create response must not leak access method value: %s", payload)
	}
}

func TestAccessDisclosureNarrow(t *testing.T) {
	svc := sampleProperties()
	mux := setupMux(svc)

	subject := security.Subject{ActorID: "actor-owner-1", TenantID: "tenant-a", Roles: []string{api.RoleOwner}}
	w := doServe(mux, "POST", "/v1/properties/prop_0000000000000001/access-disclosures", "", subject)
	body := decodeBody(w)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, string(body))
	}

	var result map[string]string
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("decode access disclosure: %v", err)
	}
	if result["access_method"] != "lockbox" {
		t.Errorf("access disclosure should expose access_method=lockbox, got %v", result)
	}
	if len(result) != 1 {
		t.Errorf("access disclosure should be narrow (1 field), got %d: %v", len(result), result)
	}
}

func TestAccessDisclosureForeignProperty404(t *testing.T) {
	svc := sampleProperties()
	mux := setupMux(svc)

	subject := security.Subject{ActorID: "actor-owner-1", TenantID: "tenant-a", Roles: []string{api.RoleOwner}}
	w := doServe(mux, "POST", "/v1/properties/prop_0000000000000002/access-disclosures", "", subject)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for foreign property access disclosure, got %d", w.Code)
	}
}

func TestNonOwnerCannotDiscloseAccess(t *testing.T) {
	svc := sampleProperties()
	mux := setupMux(svc)

	subject := security.Subject{ActorID: "actor-guest", TenantID: "tenant-a", Roles: []string{"guest"}}
	w := doServe(mux, "POST", "/v1/properties/prop_0000000000000001/access-disclosures", "", subject)
	body := decodeBody(w)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-owner, got %d: %s", w.Code, string(body))
	}
}

func TestErrorResponseContainsRequestID(t *testing.T) {
	svc := sampleProperties()
	mux := setupMux(svc)

	w := doServeNoAuth(mux, "GET", "/v1/properties")
	body := decodeBody(w)

	var errBody api.ErrorBody
	if err := json.Unmarshal(body, &errBody); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if errBody.RequestID != "req_0123456789abcdef" {
		t.Errorf("expected request_id req_0123456789abcdef, got %s", errBody.RequestID)
	}
}

func TestErrorResponseEnvelopeConforms(t *testing.T) {
	svc := sampleProperties()
	mux := setupMux(svc)

	spec, err := api.LoadSpec(contractPath())
	if err != nil {
		t.Fatalf("LoadSpec: %v", err)
	}

	w := doServeNoAuth(mux, "GET", "/v1/properties")
	body := decodeBody(w)

	if err := spec.ValidateError(body); err != nil {
		t.Errorf("error envelope does not conform: %v", err)
	}
}

func TestCollectionEnvelopeConforms(t *testing.T) {
	svc := sampleProperties()
	mux := setupMux(svc)

	spec, err := api.LoadSpec(contractPath())
	if err != nil {
		t.Fatalf("LoadSpec: %v", err)
	}

	subject := security.Subject{ActorID: "actor-owner-1", TenantID: "tenant-a", Roles: []string{api.RoleOwner}}
	w := doServe(mux, "GET", "/v1/properties", "", subject)
	body := decodeBody(w)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, string(body))
	}

	if err := spec.ValidateCollection(body); err != nil {
		t.Errorf("collection envelope does not conform: %v", err)
	}
}

func TestResourceEnvelopeConforms(t *testing.T) {
	svc := sampleProperties()
	mux := setupMux(svc)

	spec, err := api.LoadSpec(contractPath())
	if err != nil {
		t.Fatalf("LoadSpec: %v", err)
	}

	reqBody := `{
		"owner_authority_id": "auth-owner-1",
		"service_address": {"line1": "1 Test St", "city": "Test", "state": "TS", "postal_code": "12345", "country": "IN"},
		"timezone": "Asia/Kolkata",
		"access_method": "lockbox"
	}`

	subject := security.Subject{ActorID: "actor-owner-1", TenantID: "tenant-a", Roles: []string{api.RoleOwner}}
	w := doServe(mux, "POST", "/v1/properties", reqBody, subject)
	body := decodeBody(w)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, string(body))
	}

	if err := spec.ValidateResource(body); err != nil {
		t.Errorf("resource envelope does not conform: %v", err)
	}
}

func TestCollectionPreservesOrder(t *testing.T) {
	svc := &stubPropService{
		properties: []property.Property{
			{ID: "prop_C", TenantID: "tenant-a", OwnerAuthorityID: "auth-owner-1", ServiceAddress: property.Address{Line1: "A", City: "A"}, State: property.StateLead, Version: 1},
			{ID: "prop_B", TenantID: "tenant-a", OwnerAuthorityID: "auth-owner-2", ServiceAddress: property.Address{Line1: "B", City: "B"}, State: property.StateLead, Version: 1},
			{ID: "prop_A", TenantID: "tenant-a", OwnerAuthorityID: "auth-owner-1", ServiceAddress: property.Address{Line1: "C", City: "C"}, State: property.StateLead, Version: 1},
		},
	}
	mux := setupMux(svc)

	subject := security.Subject{ActorID: "actor-owner-1", TenantID: "tenant-a", Roles: []string{api.RoleOwner}}
	w := doServe(mux, "GET", "/v1/properties", "", subject)
	body := decodeBody(w)

	var collection api.Collection
	json.Unmarshal(body, &collection)

	if len(collection.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(collection.Items))
	}
	if collection.Items[0].ID != "prop_C" {
		t.Errorf("expected prop_C first, got %s", collection.Items[0].ID)
	}
	if collection.Items[1].ID != "prop_A" {
		t.Errorf("expected prop_A second, got %s", collection.Items[1].ID)
	}
}

func TestListPropertiesCursorPagination(t *testing.T) {
	svc := &stubPropService{
		properties: []property.Property{
			{ID: "prop_P01", TenantID: "tenant-a", OwnerAuthorityID: "auth-owner-1", ServiceAddress: property.Address{Line1: "1", City: "C"}, State: property.StateLead, Version: 1},
			{ID: "prop_P02", TenantID: "tenant-a", OwnerAuthorityID: "auth-owner-1", ServiceAddress: property.Address{Line1: "2", City: "C"}, State: property.StateLead, Version: 1},
			{ID: "prop_P03", TenantID: "tenant-a", OwnerAuthorityID: "auth-owner-1", ServiceAddress: property.Address{Line1: "3", City: "C"}, State: property.StateLead, Version: 1},
		},
	}
	mux := setupMux(svc)

	subject := security.Subject{ActorID: "actor-owner-1", TenantID: "tenant-a", Roles: []string{api.RoleOwner}}

	w := doServe(mux, "GET", "/v1/properties?limit=1", "", subject)
	body := decodeBody(w)

	var page1 api.Collection
	json.Unmarshal(body, &page1)
	if len(page1.Items) != 1 || page1.Items[0].ID != "prop_P01" {
		t.Fatalf("page 1: expected 1 item prop_P01, got %d items", len(page1.Items))
	}
	if page1.NextCursor == nil || *page1.NextCursor != "prop_P02" {
		t.Fatalf("page 1: expected next_cursor prop_P02, got %v", page1.NextCursor)
	}

	w = doServe(mux, "GET", "/v1/properties?limit=1&cursor="+*page1.NextCursor, "", subject)
	body = decodeBody(w)

	var page2 api.Collection
	json.Unmarshal(body, &page2)
	if len(page2.Items) != 1 || page2.Items[0].ID != "prop_P03" {
		t.Fatalf("page 2: expected prop_P03 (after cursor prop_P02), got %v", page2.Items)
	}
}

func TestListPropertiesEmptyResult(t *testing.T) {
	svc := &stubPropService{
		properties: []property.Property{},
	}
	mux := setupMux(svc)

	subject := security.Subject{ActorID: "actor-owner-1", TenantID: "tenant-a", Roles: []string{api.RoleOwner}}
	w := doServe(mux, "GET", "/v1/properties", "", subject)
	body := decodeBody(w)

	var collection api.Collection
	json.Unmarshal(body, &collection)

	if len(collection.Items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(collection.Items))
	}
	if collection.NextCursor != nil {
		t.Errorf("expected nil next_cursor for empty result")
	}
	payload := string(body)
	if !strings.Contains(payload, "items") {
		t.Errorf("collection must include items field: %s", payload)
	}
}

func TestHandlerRegistration(t *testing.T) {
	svc := sampleProperties()
	handler := api.NewPropertySliceHandler(svc, ownerAuthorityResolver)
	if handler == nil {
		t.Fatal("NewPropertySliceHandler returned nil")
	}

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
}

func TestCreateRequestValidation(t *testing.T) {
	svc := sampleProperties()
	mux := setupMux(svc)

	tests := []struct {
		name   string
		body   string
		expect int
	}{
		{"missing owner_authority", `{"service_address":{"line1":"A","city":"C"}}`, http.StatusUnprocessableEntity},
		{"missing address", `{"owner_authority_id":"auth-1"}`, http.StatusUnprocessableEntity},
		{"bad json", `not-json`, http.StatusBadRequest},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			subject := security.Subject{ActorID: "actor-owner-1", TenantID: "tenant-a", Roles: []string{api.RoleOwner}}
			w := doServe(mux, "POST", "/v1/properties", tc.body, subject)
			if w.Code != tc.expect {
				t.Errorf("expected %d, got %d: %s", tc.expect, w.Code, string(decodeBody(w)))
			}
		})
	}
	_ = context.Background
}
