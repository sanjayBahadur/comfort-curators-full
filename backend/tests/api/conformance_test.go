package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"comfort-curators-backend/internal/api"
	"comfort-curators-backend/internal/property"
)

func contractPath() string {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Dir(filepath.Dir(filepath.Dir(file)))
	return filepath.Join(root, "contracts", "api", "openapi.yaml")
}

func loadSpec(t *testing.T) *api.Spec {
	t.Helper()
	spec, err := api.LoadSpec(contractPath())
	if err != nil {
		t.Fatalf("LoadSpec: %v", err)
	}
	return spec
}

func TestConformanceContractIsOpenAPI31(t *testing.T) {
	spec := loadSpec(t)
	if got := spec.OpenAPIVersion(); got != "3.1.0" {
		t.Fatalf("OpenAPIVersion: expected 3.1.0, got %q", got)
	}
}

func TestConformanceProtectedSliceOperations(t *testing.T) {
	spec := loadSpec(t)
	ops, err := spec.ProtectedOperations()
	if err != nil {
		t.Fatalf("ProtectedOperations: %v", err)
	}

	expected := map[string]struct {
		method    string
		path      string
		tag       string
		responses []string
	}{
		"listProperties": {
			method:    "GET",
			path:      "/v1/properties",
			tag:       "Properties",
			responses: []string{"200"},
		},
		"createProperty": {
			method:    "POST",
			path:      "/v1/properties",
			tag:       "Properties",
			responses: []string{"201", "422"},
		},
		"getProperty": {
			method:    "GET",
			path:      "/v1/properties/{property_id}",
			tag:       "Properties",
			responses: []string{"200", "404"},
		},
		"transitionProperty": {
			method:    "POST",
			path:      "/v1/properties/{property_id}/transitions",
			tag:       "Properties",
			responses: []string{"200", "409", "422"},
		},
		"disclosePropertyAccess": {
			method:    "POST",
			path:      "/v1/properties/{property_id}/access-disclosures",
			tag:       "Properties",
			responses: []string{"200", "403"},
		},
		"startOwnerOnboarding": {
			method:    "POST",
			path:      "/v1/owners/onboarding-cases",
			tag:       "Onboarding",
			responses: []string{"201", "409"},
		},
		"recordPropertyInspection": {
			method:    "POST",
			path:      "/v1/properties/{property_id}/inspections",
			tag:       "Onboarding",
			responses: []string{"201", "422"},
		},
		"createPropertyContractVersion": {
			method:    "POST",
			path:      "/v1/properties/{property_id}/contracts",
			tag:       "Onboarding",
			responses: []string{"201", "409"},
		},
	}

	if len(ops) != len(expected) {
		t.Fatalf("ProtectedOperations: expected %d operations, got %d: %+v", len(expected), len(ops), ops)
	}

	seen := map[string]bool{}
	for _, op := range ops {
		exp, ok := expected[op.OperationID]
		if !ok {
			t.Fatalf("unexpected protected operation %q (%s %s)", op.OperationID, op.Method, op.Path)
		}
		seen[op.OperationID] = true
		if op.Method != exp.method {
			t.Errorf("%s: method expected %s, got %s", op.OperationID, exp.method, op.Method)
		}
		if op.Path != exp.path {
			t.Errorf("%s: path expected %s, got %s", op.OperationID, exp.path, op.Path)
		}
		if op.Tag != exp.tag {
			t.Errorf("%s: tag expected %s, got %s", op.OperationID, exp.tag, op.Tag)
		}
		if !equalStrings(op.Responses, exp.responses) {
			t.Errorf("%s: declared responses expected %v, got %v", op.OperationID, exp.responses, op.Responses)
		}
	}
	for id := range expected {
		if !seen[id] {
			t.Errorf("protected operation %q is missing from the contract slice", id)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestConformanceResourceEnvelope(t *testing.T) {
	spec := loadSpec(t)

	valid := `{"id":"prop_0123456789abcdef","version":3,"data":{"state":"active","tenant_id":"tenant-a"}}`
	if err := spec.ValidateResource([]byte(valid)); err != nil {
		t.Fatalf("valid Resource rejected: %v", err)
	}

	invalid := []struct {
		name string
		body string
	}{
		{"missing id", `{"version":1,"data":{}}`},
		{"short id", `{"id":"short","version":1,"data":{}}`},
		{"bad id charset", `{"id":"prop with spaces","version":1,"data":{}}`},
		{"missing version", `{"id":"prop_0123456789abcdef","data":{}}`},
		{"zero version", `{"id":"prop_0123456789abcdef","version":0,"data":{}}`},
		{"missing data", `{"id":"prop_0123456789abcdef","version":1}`},
		{"data not object", `{"id":"prop_0123456789abcdef","version":1,"data":[1,2]}`},
		{"not json", `not-json`},
		{"not object", `[1,2,3]`},
	}
	for _, tc := range invalid {
		if err := spec.ValidateResource([]byte(tc.body)); err == nil {
			t.Errorf("%s: expected validation failure, got pass for body %s", tc.name, tc.body)
		}
	}
}

func TestConformanceCollectionEnvelope(t *testing.T) {
	spec := loadSpec(t)

	valid := `{"items":[{"id":"prop_0123456789abcdef","version":1,"data":{"state":"lead"}}],"next_cursor":null}`
	if err := spec.ValidateCollection([]byte(valid)); err != nil {
		t.Fatalf("valid Collection rejected: %v", err)
	}

	validCursor := `{"items":[{"id":"prop_0123456789abcdef","version":1,"data":{}}],"next_cursor":"prop_9876543210abcdef"}`
	if err := spec.ValidateCollection([]byte(validCursor)); err != nil {
		t.Fatalf("valid Collection with cursor rejected: %v", err)
	}

	invalid := []struct {
		name string
		body string
	}{
		{"missing items", `{}`},
		{"items not array", `{"items":{}}`},
		{"item missing version", `{"items":[{"id":"prop_0123456789abcdef","data":{}}]}`},
		{"extra field", `{"items":[],"extra":true}`},
	}
	for _, tc := range invalid {
		if err := spec.ValidateCollection([]byte(tc.body)); err == nil {
			t.Errorf("%s: expected validation failure, got pass for body %s", tc.name, tc.body)
		}
	}
}

func TestConformanceErrorEnvelope(t *testing.T) {
	spec := loadSpec(t)

	valid := `{"request_id":"req_0123456789abcdef","code":"NOT_FOUND","message":"property not found"}`
	if err := spec.ValidateError([]byte(valid)); err != nil {
		t.Fatalf("valid Error rejected: %v", err)
	}

	invalid := []struct {
		name string
		body string
	}{
		{"missing request_id", `{"code":"NOT_FOUND","message":"x"}`},
		{"missing code", `{"request_id":"req_0123456789abcdef","message":"x"}`},
		{"missing message", `{"request_id":"req_0123456789abcdef","code":"NOT_FOUND"}`},
		{"code with lowercase", `{"request_id":"req_0123456789abcdef","code":"not_found","message":"x"}`},
		{"extra field", `{"request_id":"req_0123456789abcdef","code":"NOT_FOUND","message":"x","extra":1}`},
	}
	for _, tc := range invalid {
		if err := spec.ValidateError([]byte(tc.body)); err == nil {
			t.Errorf("%s: expected validation failure, got pass for body %s", tc.name, tc.body)
		}
	}
}

func TestConformanceValidatorDetectsLiveHandlerGap(t *testing.T) {
	spec := loadSpec(t)

	handler := property.NewPropertyHandler(nil)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	server := httptest.NewServer(mux)
	defer server.Close()

	// Unauthenticated access to a protected property route fails closed with a
	// 401 Error envelope. The conformance validator must detect that the live
	// handler's request_id is empty (no correlation id is copied onto the
	// request header), proving the check has teeth rather than passing any
	// shape. The parent epic closes this residual gap.
	for _, path := range []string{"/v1/properties", "/v1/properties/prop_0123456789abcdef"} {
		resp, err := http.Get(server.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("GET %s: expected 401, got %d: %s", path, resp.StatusCode, string(body))
		}
		if err := spec.ValidateError(body); err == nil {
			t.Errorf("GET %s: validator must flag the empty request_id error envelope (body: %s)", path, string(body))
		} else if !strings.Contains(err.Error(), "request_id") {
			t.Errorf("GET %s: validator should report the request_id violation, got: %v", path, err)
		}
	}
}

func TestConformanceLiveEnvelopesOnStubServer(t *testing.T) {
	spec := loadSpec(t)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/properties", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{
			"items": []map[string]any{
				{"id": "prop_0123456789abcdef", "version": 1, "data": map[string]any{"state": "lead"}},
			},
			"next_cursor": nil,
		})
	})
	mux.HandleFunc("POST /v1/properties", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		writeJSON(t, w, map[string]any{
			"id":      "prop_0123456789abcdef",
			"version": 1,
			"data":    map[string]any{"state": "lead"},
		})
	})
	mux.HandleFunc("POST /v1/properties/{property_id}/transitions", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		writeJSON(t, w, map[string]any{
			"request_id": "req_0123456789abcdef",
			"code":       "COMPLIANCE_HOLD",
			"message":    "critical compliance hold blocks activation",
		})
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	// Collection
	resp, body := doGet(t, server.URL+"/v1/properties")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list properties: expected 200, got %d", resp.StatusCode)
	}
	if err := spec.ValidateCollection(body); err != nil {
		t.Errorf("list properties: collection does not conform: %v", err)
	}

	// Resource (201)
	resp, body = doPost(t, server.URL+"/v1/properties", `{}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create property: expected 201, got %d", resp.StatusCode)
	}
	if err := spec.ValidateResource(body); err != nil {
		t.Errorf("create property: resource does not conform: %v", err)
	}

	// Error (409)
	resp, body = doPost(t, server.URL+"/v1/properties/prop_0123456789abcdef/transitions", `{}`)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("transition: expected 409, got %d", resp.StatusCode)
	}
	if err := spec.ValidateError(body); err != nil {
		t.Errorf("transition: error envelope does not conform: %v", err)
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Errorf("encode response: %v", err)
	}
}

func doGet(t *testing.T, url string) (*http.Response, []byte) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read GET %s: %v", url, err)
	}
	return resp, body
}

func doPost(t *testing.T, url, payload string) (*http.Response, []byte) {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read POST %s: %v", url, err)
	}
	return resp, body
}
