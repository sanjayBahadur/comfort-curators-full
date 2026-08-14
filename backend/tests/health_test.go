package tests

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"comfort-curators-backend/internal/platform/health"
)

type mockChecker struct {
	name    string
	checkFn func() error
}

func (m *mockChecker) Name() string { return m.name }
func (m *mockChecker) Check() error { return m.checkFn() }

func TestHealthLivenessReturnsOK(t *testing.T) {
	h := health.NewHandler()
	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	w := httptest.NewRecorder()

	h.Liveness().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp health.HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != health.StatusOK {
		t.Errorf("expected status ok, got %s", resp.Status)
	}
	if resp.Time == "" {
		t.Error("response must include time")
	}
}

func TestHealthLivenessShapeConformsToContract(t *testing.T) {
	h := health.NewHandler()
	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	w := httptest.NewRecorder()

	h.Liveness().ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected application/json content-type, got %s", ct)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if _, ok := resp["status"]; !ok {
		t.Error("response must have status field")
	}
	if _, ok := resp["time"]; !ok {
		t.Error("response must have time field")
	}

	status, _ := resp["status"].(string)
	if status != string(health.StatusOK) {
		t.Errorf("expected status ok, got %s", status)
	}
}

func TestHealthReadinessAllHealthyReturns200(t *testing.T) {
	h := health.NewHandler(
		&mockChecker{name: "db", checkFn: func() error { return nil }},
		&mockChecker{name: "storage", checkFn: func() error { return nil }},
	)
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()

	h.Readiness().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp health.HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != health.StatusOK {
		t.Errorf("expected status ok, got %s", resp.Status)
	}
	if v, ok := resp.Checks["db"]; !ok || v != "ok" {
		t.Errorf("expected db check ok, got %v", resp.Checks["db"])
	}
	if v, ok := resp.Checks["storage"]; !ok || v != "ok" {
		t.Errorf("expected storage check ok, got %v", resp.Checks["storage"])
	}
}

func TestHealthReadinessDegradedReturns503(t *testing.T) {
	h := health.NewHandler(
		&mockChecker{name: "db", checkFn: func() error { return nil }},
		&mockChecker{name: "storage", checkFn: func() error { return errors.New("unreachable") }},
	)
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()

	h.Readiness().ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}

	var resp health.HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != health.StatusDegraded {
		t.Errorf("expected status degraded, got %s", resp.Status)
	}
	if v, ok := resp.Checks["storage"]; !ok || v == "ok" {
		t.Errorf("expected storage check to report error, got %v", resp.Checks["storage"])
	}
}

func TestHealthReadinessEmptyChecksReturns200(t *testing.T) {
	h := health.NewHandler()
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()

	h.Readiness().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with no checks, got %d", w.Code)
	}

	var resp health.HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != health.StatusOK {
		t.Errorf("expected status ok, got %s", resp.Status)
	}

	var raw map[string]any
	json.NewDecoder(strings.NewReader(w.Body.String())).Decode(&raw)
	if _, ok := raw["checks"]; ok {
		t.Error("empty checks should not appear in response (omitempty)")
	}
}

func TestHealthErrorShapeConformsToContract(t *testing.T) {
	resp := health.ErrorResponse{
		RequestID: "req-abc123def456",
		Code:      "INTERNAL_ERROR",
		Message:   "something went wrong",
	}

	body, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal error response: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if _, ok := parsed["request_id"]; !ok {
		t.Error("error response must have request_id")
	}
	if _, ok := parsed["code"]; !ok {
		t.Error("error response must have code")
	}
	if _, ok := parsed["message"]; !ok {
		t.Error("error response must have message")
	}
}
