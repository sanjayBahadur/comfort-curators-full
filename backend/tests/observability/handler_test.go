package observability_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"comfort-curators-backend/internal/iam"
	"comfort-curators-backend/internal/observability"
	"comfort-curators-backend/internal/platform/security"
)

// withStaffSubject simulates a request that already passed through
// iam.AuthMiddleware as an authenticated staff session: RegisterRoutes now
// gates every route in this package to staff (they disclose operational
// internals), and this package's own tests intentionally don't stand up a
// full IdentityService/DB just to exercise that -- IAM's auth machinery
// has its own tests. This only asserts what this package does once a
// staff subject is already present.
func withStaffSubject(r *http.Request) *http.Request {
	ctx := iam.WithSubject(r.Context(), security.Subject{
		ActorID:  "test-staff",
		TenantID: "test-tenant",
		Roles:    []string{iam.RoleStaff},
	})
	return r.WithContext(ctx)
}

func TestHandlerMetricsEndpointReturnsSnapshot(t *testing.T) {
	m := observability.NewMetrics()
	tr := observability.NewTracer()
	a := observability.NewAlertService()

	m.APICall("GET", "/health/live", 200)
	m.DBQuery("select", "ok")

	h := observability.NewHandler(m, tr, a)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux := http.NewServeMux()
		h.RegisterRoutes(mux)
		mux.ServeHTTP(w, withStaffSubject(r))
	}))
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	metricsRaw, ok := body["metrics"]
	if !ok {
		t.Fatal("response must have metrics key")
	}
	metrics, ok := metricsRaw.([]any)
	if !ok {
		t.Fatalf("metrics must be an array, got %T", metricsRaw)
	}
	if len(metrics) < 2 {
		t.Fatalf("expected at least 2 metrics, got %d", len(metrics))
	}
}

func TestHandlerAlertsEndpointReturnsAlerts(t *testing.T) {
	m := observability.NewMetrics()
	tr := observability.NewTracer()
	a := observability.NewAlertService()

	corr := observability.NewCorrelation()
	a.Emit(observability.PropertyReadinessAlert("tenant-1", "prop-42", 3, corr, time.Now()))
	a.Emit(observability.IncidentAlert("tenant-2", "prop-7", "incident-1", observability.SeverityCritical, corr, time.Now()))

	h := observability.NewHandler(m, tr, a)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux := http.NewServeMux()
		h.RegisterRoutes(mux)
		mux.ServeHTTP(w, withStaffSubject(r))
	}))
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/alerts")
	if err != nil {
		t.Fatalf("GET /alerts: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	count, _ := body["count"].(float64)
	if int(count) != 2 {
		t.Fatalf("expected 2 alerts, got %v", count)
	}
}

func TestHandlerUnresolvedAlertsFilteredByKind(t *testing.T) {
	m := observability.NewMetrics()
	tr := observability.NewTracer()
	a := observability.NewAlertService()

	corr := observability.NewCorrelation()
	a.Emit(observability.PropertyReadinessAlert("tenant-1", "prop-42", 3, corr, time.Now()))
	a.Emit(observability.IncidentAlert("tenant-2", "prop-7", "incident-1", observability.SeverityCritical, corr, time.Now()))
	a.Emit(observability.StockAlert("tenant-3", "prop-9", "item-1", 2, 10, corr, time.Now()))

	h := observability.NewHandler(m, tr, a)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux := http.NewServeMux()
		h.RegisterRoutes(mux)
		mux.ServeHTTP(w, withStaffSubject(r))
	}))
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/alerts/unresolved?kind=incident,stock")
	if err != nil {
		t.Fatalf("GET /alerts/unresolved: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	count, _ := body["count"].(float64)
	if int(count) != 2 {
		t.Fatalf("expected 2 unresolved (incident+stock), got %v", count)
	}
}

func TestHandlerTracesEndpointReturnsSpans(t *testing.T) {
	m := observability.NewMetrics()
	tr := observability.NewTracer()
	a := observability.NewAlertService()

	corr := observability.NewCorrelation()
	span := tr.Start(corr, "test.operation")
	tr.End(span, nil)

	h := observability.NewHandler(m, tr, a)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux := http.NewServeMux()
		h.RegisterRoutes(mux)
		mux.ServeHTTP(w, withStaffSubject(r))
	}))
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/traces")
	if err != nil {
		t.Fatalf("GET /traces: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	count, _ := body["count"].(float64)
	if int(count) != 1 {
		t.Fatalf("expected 1 span, got %v", count)
	}
}

func TestHandlerTraceByID(t *testing.T) {
	m := observability.NewMetrics()
	tr := observability.NewTracer()
	a := observability.NewAlertService()

	corr := observability.NewCorrelation()
	span := tr.Start(corr, "test.operation")
	tr.End(span, nil)

	h := observability.NewHandler(m, tr, a)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux := http.NewServeMux()
		h.RegisterRoutes(mux)
		mux.ServeHTTP(w, withStaffSubject(r))
	}))
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/traces/" + corr.TraceID)
	if err != nil {
		t.Fatalf("GET /traces/%s: %v", corr.TraceID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if got, ok := body["trace_id"].(string); !ok || got != corr.TraceID {
		t.Errorf("trace_id mismatch: got %v want %s", body["trace_id"], corr.TraceID)
	}

	count, _ := body["count"].(float64)
	if int(count) != 1 {
		t.Fatalf("expected 1 span for trace, got %v", count)
	}
}

func TestHandlerTraceByIDReturnsEmptyForUnknownTrace(t *testing.T) {
	m := observability.NewMetrics()
	tr := observability.NewTracer()
	a := observability.NewAlertService()

	h := observability.NewHandler(m, tr, a)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux := http.NewServeMux()
		h.RegisterRoutes(mux)
		mux.ServeHTTP(w, withStaffSubject(r))
	}))
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/traces/nonexistent")
	if err != nil {
		t.Fatalf("GET /traces/nonexistent: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	count, _ := body["count"].(float64)
	if int(count) != 0 {
		t.Fatalf("expected 0 spans, got %v", count)
	}
}

func TestHandlerEmptyTraceIDReturnsBadRequest(t *testing.T) {
	m := observability.NewMetrics()
	tr := observability.NewTracer()
	a := observability.NewAlertService()

	h := observability.NewHandler(m, tr, a)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux := http.NewServeMux()
		h.RegisterRoutes(mux)
		mux.ServeHTTP(w, withStaffSubject(r))
	}))
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/traces/")
	if err != nil {
		t.Fatalf("GET /traces/: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for empty traceID path, got %d", resp.StatusCode)
	}
}

func TestHandlerAllEndpointsUseJSONContentType(t *testing.T) {
	m := observability.NewMetrics()
	tr := observability.NewTracer()
	a := observability.NewAlertService()

	h := observability.NewHandler(m, tr, a)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux := http.NewServeMux()
		h.RegisterRoutes(mux)
		mux.ServeHTTP(w, withStaffSubject(r))
	}))
	defer srv.Close()

	endpoints := []string{"/metrics", "/alerts", "/alerts/unresolved", "/traces"}
	for _, ep := range endpoints {
		resp, err := srv.Client().Get(srv.URL + ep)
		if err != nil {
			t.Fatalf("GET %s: %v", ep, err)
		}
		ct := resp.Header.Get("Content-Type")
		if !strings.Contains(ct, "application/json") {
			t.Errorf("%s: expected application/json, got %s", ep, ct)
		}
		resp.Body.Close()
	}
}

func TestHandlerAlertsResponseIsRedacted(t *testing.T) {
	m := observability.NewMetrics()
	tr := observability.NewTracer()
	a := observability.NewAlertService()

	corr := observability.NewCorrelation()
	a.Emit(observability.PropertyReadinessAlert("tenant-1", "prop-42", 3, corr, time.Now()))

	h := observability.NewHandler(m, tr, a)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux := http.NewServeMux()
		h.RegisterRoutes(mux)
		mux.ServeHTTP(w, withStaffSubject(r))
	}))
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/alerts")
	if err != nil {
		t.Fatalf("GET /alerts: %v", err)
	}
	defer resp.Body.Close()

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	raw, _ := json.Marshal(body)
	for _, leak := range []string{"password", "token", "secret", "api_key", "authorization", "credential", "Bearer "} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("alerts response leaked sensitive content %q", leak)
		}
	}
}

func TestHandlerMetricsEmptySnapshotReturnsEmptyArray(t *testing.T) {
	m := observability.NewMetrics()
	tr := observability.NewTracer()
	a := observability.NewAlertService()

	h := observability.NewHandler(m, tr, a)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux := http.NewServeMux()
		h.RegisterRoutes(mux)
		mux.ServeHTTP(w, withStaffSubject(r))
	}))
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	metricsRaw, ok := body["metrics"]
	if !ok {
		t.Fatal("response must have metrics key")
	}
	metrics, ok := metricsRaw.([]any)
	if !ok {
		t.Fatalf("metrics must be an array, got %T", metricsRaw)
	}
	if len(metrics) != 0 {
		t.Fatalf("expected empty metrics array, got %d items", len(metrics))
	}
}
