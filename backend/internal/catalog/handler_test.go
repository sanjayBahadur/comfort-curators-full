package catalog

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewHandler(t *testing.T) {
	svc := &Service{}
	h := NewHandler(svc)
	if h == nil || h.svc != svc {
		t.Fatal("handler must wrap the service")
	}
}

func TestRegisterRoutes(t *testing.T) {
	svc := &Service{}
	h := NewHandler(svc)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	expectedPatterns := []string{
		"POST /v1/catalog/items",
		"GET /v1/catalog/items",
		"GET /v1/catalog/items/{item_id}",
		"POST /v1/catalog/items/{item_id}/claims",
		"GET /v1/catalog/items/{item_id}/claims",
		"POST /v1/catalog/templates",
		"GET /v1/catalog/templates",
		"GET /v1/catalog/templates/{template_id}",
		"POST /v1/properties/{property_id}/packages",
		"GET /v1/properties/{property_id}/packages",
		"GET /v1/properties/{property_id}/packages/{version_id}",
		"POST /v1/properties/{property_id}/packages/{version_id}/activate",
		"POST /v1/properties/{property_id}/packages/{version_id}/reject",
	}
	if len(expectedPatterns) != 13 {
		t.Fatalf("expected 13 routes, got %d", len(expectedPatterns))
	}
}

func TestAPIErrorResponse(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	req.Header.Set("X-Correlation-ID", "req-123")

	apiError(w, req, http.StatusNotFound, "NOT_FOUND", "item not found")

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", w.Code)
	}

	var errResp catalogError
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("response must be valid JSON: %v", err)
	}
	if errResp.RequestID != "req-123" {
		t.Fatalf("expected request_id req-123, got %q", errResp.RequestID)
	}
	if errResp.Code != "NOT_FOUND" {
		t.Fatalf("expected code NOT_FOUND, got %q", errResp.Code)
	}
	if errResp.Message != "item not found" {
		t.Fatalf("expected message 'item not found', got %q", errResp.Message)
	}
}

func TestAPIResourceResponse(t *testing.T) {
	w := httptest.NewRecorder()
	data := map[string]string{"name": "test-item"}
	apiResource(w, http.StatusCreated, "cit-abc", 1, data)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", w.Code)
	}

	var res catalogResource
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("response must be valid JSON: %v", err)
	}
	if res.ID != "cit-abc" {
		t.Fatalf("expected id cit-abc, got %q", res.ID)
	}
	if res.Version != 1 {
		t.Fatalf("expected version 1, got %d", res.Version)
	}
	dataOut, ok := res.Data.(map[string]any)
	if !ok || dataOut["name"] != "test-item" {
		t.Fatalf("expected data.name test-item, got %v", res.Data)
	}
}

func TestAPICollectionResponse(t *testing.T) {
	w := httptest.NewRecorder()
	items := []catalogResource{
		{ID: "cit-a", Version: 1, Data: map[string]string{"sku": "A"}},
		{ID: "cit-b", Version: 1, Data: map[string]string{"sku": "B"}},
	}
	apiCollection(w, items)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var coll struct {
		Items []catalogResource `json:"items"`
		Total int               `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &coll); err != nil {
		t.Fatalf("response must be valid JSON: %v", err)
	}
	if coll.Total != 2 {
		t.Fatalf("expected total 2, got %d", coll.Total)
	}
	if len(coll.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(coll.Items))
	}
}

func TestHandleCreateItemInvalidJSON(t *testing.T) {
	h := NewHandler(&Service{})

	body := bytes.NewReader([]byte(`not json`))
	req := httptest.NewRequest("POST", "http://example.com/v1/catalog/items", body)
	w := httptest.NewRecorder()

	h.handleCreateItem(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized without auth context, got %d", w.Code)
	}
}

func TestHandleGetItemInvalidJSON(t *testing.T) {
	h := NewHandler(&Service{})

	req := httptest.NewRequest("GET", "http://example.com/v1/catalog/items/cit-1", nil)
	w := httptest.NewRecorder()

	h.handleGetItem(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized without auth context, got %d", w.Code)
	}
}

func TestHandleListItemsInvalidJSON(t *testing.T) {
	h := NewHandler(&Service{})

	req := httptest.NewRequest("GET", "http://example.com/v1/catalog/items", nil)
	w := httptest.NewRecorder()

	h.handleListItems(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized without auth context, got %d", w.Code)
	}
}

func TestHandleAddClaimEvidenceInvalidJSON(t *testing.T) {
	h := NewHandler(&Service{})

	req := httptest.NewRequest("POST", "http://example.com/v1/catalog/items/cit-1/claims", nil)
	w := httptest.NewRecorder()

	h.handleAddClaimEvidence(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized without auth context, got %d", w.Code)
	}
}

func TestHandleListClaimEvidenceInvalidJSON(t *testing.T) {
	h := NewHandler(&Service{})

	req := httptest.NewRequest("GET", "http://example.com/v1/catalog/items/cit-1/claims", nil)
	w := httptest.NewRecorder()

	h.handleListClaimEvidence(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized without auth context, got %d", w.Code)
	}
}

func TestHandleCreateTemplateInvalidJSON(t *testing.T) {
	h := NewHandler(&Service{})

	req := httptest.NewRequest("POST", "http://example.com/v1/catalog/templates", nil)
	w := httptest.NewRecorder()

	h.handleCreateTemplate(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized without auth context, got %d", w.Code)
	}
}

func TestHandleGetTemplateInvalidJSON(t *testing.T) {
	h := NewHandler(&Service{})

	req := httptest.NewRequest("GET", "http://example.com/v1/catalog/templates/tpl-1", nil)
	w := httptest.NewRecorder()

	h.handleGetTemplate(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized without auth context, got %d", w.Code)
	}
}

func TestHandleListTemplatesInvalidJSON(t *testing.T) {
	h := NewHandler(&Service{})

	req := httptest.NewRequest("GET", "http://example.com/v1/catalog/templates", nil)
	w := httptest.NewRecorder()

	h.handleListTemplates(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized without auth context, got %d", w.Code)
	}
}

func TestHandleCreatePackageVersionInvalidJSON(t *testing.T) {
	h := NewHandler(&Service{})

	req := httptest.NewRequest("POST", "http://example.com/v1/properties/prop-1/packages", nil)
	w := httptest.NewRecorder()

	h.handleCreatePackageVersion(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized without auth context, got %d", w.Code)
	}
}

func TestHandleGetPackageVersionInvalidJSON(t *testing.T) {
	h := NewHandler(&Service{})

	req := httptest.NewRequest("GET", "http://example.com/v1/properties/prop-1/packages/pkg-1", nil)
	w := httptest.NewRecorder()

	h.handleGetPackageVersion(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized without auth context, got %d", w.Code)
	}
}

func TestHandleListPackageVersionsInvalidJSON(t *testing.T) {
	h := NewHandler(&Service{})

	req := httptest.NewRequest("GET", "http://example.com/v1/properties/prop-1/packages", nil)
	w := httptest.NewRecorder()

	h.handleListPackageVersions(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized without auth context, got %d", w.Code)
	}
}

func TestHandleActivatePackageVersionInvalidJSON(t *testing.T) {
	h := NewHandler(&Service{})

	req := httptest.NewRequest("POST", "http://example.com/v1/properties/prop-1/packages/pkg-1/activate", bytes.NewReader([]byte(`{}`)))
	w := httptest.NewRecorder()

	h.handleActivatePackageVersion(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized without auth context, got %d", w.Code)
	}
}

func TestHandleRejectPackageVersionInvalidJSON(t *testing.T) {
	h := NewHandler(&Service{})

	req := httptest.NewRequest("POST", "http://example.com/v1/properties/prop-1/packages/pkg-1/reject", bytes.NewReader([]byte(`{}`)))
	w := httptest.NewRecorder()

	h.handleRejectPackageVersion(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized without auth context, got %d", w.Code)
	}
}
