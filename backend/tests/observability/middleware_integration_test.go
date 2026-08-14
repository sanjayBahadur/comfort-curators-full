package observability_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"comfort-curators-backend/internal/observability"
	httpplatform "comfort-curators-backend/internal/platform/http"
)

func TestCorrelationIDMiddlewarePropagatesObservabilityCorrelation(t *testing.T) {
	var capturedCorr observability.Correlation
	var capturedOK bool

	handler := httpplatform.CorrelationID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedCorr, capturedOK = observability.FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(observability.HeaderCorrelationID, "corr-mw-001")
	req.Header.Set(observability.HeaderTraceID, "trace-mw-001")
	req.Header.Set(observability.HeaderSource, string(observability.SourceAPI))

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !capturedOK {
		t.Fatal("correlation was not stored in context")
	}
	if capturedCorr.ID != "corr-mw-001" {
		t.Errorf("expected corr-mw-001, got %s", capturedCorr.ID)
	}
	if capturedCorr.TraceID != "trace-mw-001" {
		t.Errorf("expected trace-mw-001, got %s", capturedCorr.TraceID)
	}
	if capturedCorr.Source != observability.SourceAPI {
		t.Errorf("expected source api, got %s", capturedCorr.Source)
	}

	respCorrID := w.Header().Get(observability.HeaderCorrelationID)
	if respCorrID != "corr-mw-001" {
		t.Errorf("response header correlation ID mismatch: got %s", respCorrID)
	}
}

func TestCorrelationIDMiddlewareSynthesizesNewCorrelation(t *testing.T) {
	var capturedCorr observability.Correlation
	var capturedOK bool
	var capturedFromNew observability.Correlation

	handler := httpplatform.CorrelationID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedCorr, capturedOK = observability.FromContext(r.Context())
		capturedFromNew = observability.FromContextOrNew(context.Background())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !capturedOK {
		t.Fatal("correlation was not stored in context")
	}
	if capturedCorr.ID == "" {
		t.Error("synthesized correlation must have a non-empty ID")
	}
	if capturedCorr.TraceID == "" {
		t.Error("synthesized correlation must have a non-empty trace ID")
	}
	if capturedCorr.SpanID == "" {
		t.Error("synthesized correlation must have a non-empty span ID")
	}
	if capturedCorr.Source != observability.SourceAPI {
		t.Errorf("synthesized correlation must source api, got %s", capturedCorr.Source)
	}

	respCorrID := w.Header().Get(observability.HeaderCorrelationID)
	if respCorrID != capturedCorr.ID {
		t.Errorf("response header correlation ID mismatch: got %s want %s", respCorrID, capturedCorr.ID)
	}

	if capturedFromNew.ID == "" {
		t.Error("FromContextOrNew must work on a context without correlation")
	}
}

func TestObservabilityMetricsMiddlewareRecordsAPIMetrics(t *testing.T) {
	m := observability.NewMetrics()

	handler := httpplatform.ObservabilityMetrics(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}), m)

	req := httptest.NewRequest(http.MethodPost, "/v1/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	val := m.CounterValue("api.requests", "method", "POST", "route", "/v1/test", "status", "201")
	if val != 1 {
		t.Errorf("expected api.requests counter 1, got %d", val)
	}

	snap := m.Snapshot()
	hasLatency := false
	for _, metric := range snap {
		if metric.Name == "api.latency" {
			hasLatency = true
			if metric.Count < 1 {
				t.Errorf("api.latency should have at least 1 sample, got %d", metric.Count)
			}
		}
	}
	if !hasLatency {
		t.Error("api.latency histogram missing from snapshot")
	}
}

func TestObservabilityTracingMiddlewareRecordsSpan(t *testing.T) {
	tr := observability.NewTracer()

	handler := httpplatform.CorrelationID(
		httpplatform.ObservabilityTracing(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}), tr),
	)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	spans := tr.Spans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]
	if span.Name != "GET /test" {
		t.Errorf("expected span name 'GET /test', got %q", span.Name)
	}
	if span.Source != observability.SourceAPI {
		t.Errorf("expected source api, got %s", span.Source)
	}
	if span.Status != observability.SpanStatusOK {
		t.Errorf("expected status ok, got %s", span.Status)
	}
	if span.CorrelationID == "" {
		t.Error("span must carry correlation ID")
	}
	if span.TraceID == "" {
		t.Error("span must carry trace ID")
	}
}

func TestObservabilityTracingMarksErrorOn5xx(t *testing.T) {
	tr := observability.NewTracer()

	handler := httpplatform.CorrelationID(
		httpplatform.ObservabilityTracing(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}), tr),
	)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	spans := tr.Spans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Status != observability.SpanStatusError {
		t.Errorf("expected status error for 5xx, got %s", spans[0].Status)
	}
}

func TestMiddlewareChainCorrelationToTracing(t *testing.T) {
	m := observability.NewMetrics()
	tr := observability.NewTracer()

	coreHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		corr, ok := observability.FromContext(r.Context())
		if !ok {
			t.Error("correlation not in context inside handler")
		}
		_ = corr
		w.WriteHeader(http.StatusOK)
	})

	var handler http.Handler = coreHandler
	handler = httpplatform.CorrelationID(handler)
	handler = httpplatform.ObservabilityTracing(handler, tr)
	handler = httpplatform.ObservabilityMetrics(handler, m)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	apiReqs := m.CounterValue("api.requests", "method", "GET", "route", "/test", "status", "200")
	if apiReqs != 1 {
		t.Errorf("expected api.requests 1, got %d", apiReqs)
	}

	spans := tr.Spans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].CorrelationID == "" {
		t.Error("span must have correlation ID from middleware chain")
	}
}
