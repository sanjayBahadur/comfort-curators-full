package consumer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewHandler(t *testing.T) {
	svc := &ConsumerService{}
	h := NewHandler(svc)
	if h == nil || h.svc != svc {
		t.Fatal("handler must wrap the service")
	}
}

func TestRegisterRoutes(t *testing.T) {
	svc := &ConsumerService{}
	h := NewHandler(svc)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	expectedPatterns := []string{
		"POST /v1/consumer/disclosures",
		"GET /v1/consumer/disclosures",
		"GET /v1/consumer/disclosures/{disclosure_id}",
		"POST /v1/consumer/acceptances",
		"GET /v1/consumer/acceptances/{acceptance_id}",
		"POST /v1/consumer/history-exports",
		"GET /v1/consumer/history-exports",
		"GET /v1/consumer/history-exports/{export_id}",
	}
	if len(expectedPatterns) != 8 {
		t.Fatalf("expected 8 routes, got %d", len(expectedPatterns))
	}
}

func TestAPIErrorResponse(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	req.Header.Set("X-Correlation-ID", "req-123")

	writeError(w, req, http.StatusNotFound, "NOT_FOUND", "disclosure not found")

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", w.Code)
	}

	var errResp apiError
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("response must be valid JSON: %v", err)
	}
	if errResp.RequestID != "req-123" {
		t.Fatalf("expected request_id req-123, got %q", errResp.RequestID)
	}
	if errResp.Code != "NOT_FOUND" {
		t.Fatalf("expected code NOT_FOUND, got %q", errResp.Code)
	}
	if errResp.Message != "disclosure not found" {
		t.Fatalf("expected message 'disclosure not found', got %q", errResp.Message)
	}
}
