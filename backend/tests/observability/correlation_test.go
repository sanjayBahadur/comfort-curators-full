package observability_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"comfort-curators-backend/internal/observability"
)

// TestCorrelationCrossesAPIToJob simulates the API boundary accepting a
// request with a caller correlation header and enqueueing a durable job that
// preserves that identity.
func TestCorrelationCrossesAPIToJob(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "/v1/properties", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set(observability.HeaderCorrelationID, "corr-arrival-001")
	req.Header.Set(observability.HeaderTraceID, "trace-arrival-001")
	req.Header.Set(observability.HeaderSource, string(observability.SourceAPI))

	apiCorr, ok := observability.Extract(observability.HeaderCarrier{Header: req.Header})
	if !ok {
		t.Fatal("expected correlation extracted from API request")
	}
	if apiCorr.ID != "corr-arrival-001" {
		t.Errorf("expected corr-arrival-001, got %s", apiCorr.ID)
	}

	jobPayload := map[string]string{
		"correlation": apiCorr.Encode(),
	}
	raw, err := json.Marshal(jobPayload)
	if err != nil {
		t.Fatalf("marshal job payload: %v", err)
	}

	var decoded map[string]string
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal job payload: %v", err)
	}
	jobCorr, err := observability.Decode(decoded["correlation"])
	if err != nil {
		t.Fatalf("decode job correlation: %v", err)
	}

	if jobCorr.ID != apiCorr.ID {
		t.Errorf("job must carry the API correlation ID, got %s want %s", jobCorr.ID, apiCorr.ID)
	}
	if jobCorr.TraceID != apiCorr.TraceID {
		t.Errorf("job must carry the API trace ID, got %s want %s", jobCorr.TraceID, apiCorr.TraceID)
	}
}

// TestCorrelationFullChainCrossesAllBoundaries exercises the complete chain:
// API -> durable job -> outbox event -> model tool call. Every hop must share
// the same correlation and trace identity and build a valid span hierarchy.
func TestCorrelationFullChainCrossesAllBoundaries(t *testing.T) {
	apiCorr := observability.NewCorrelation()
	if apiCorr.Source != observability.SourceAPI {
		t.Fatalf("root correlation must originate at api, got %s", apiCorr.Source)
	}

	tracer := observability.NewTracer()
	apiSpan := tracer.Start(apiCorr, "handle.request")

	// 1. API -> job: the correlation travels inside the durable job payload.
	jobCorr := apiCorr.Child(observability.SourceJob)
	jobSpan := tracer.Start(jobCorr, "process.job")

	outboxCarrier := observability.MapCarrier{}
	jobCorr.Inject(outboxCarrier)

	// 2. job -> outbox: the worker extracts the job correlation and stores the
	// encoded correlation in the outbox envelope. The outbox hop derives from
	// the job hop.
	fromJob, ok := observability.Extract(outboxCarrier)
	if !ok {
		t.Fatal("expected correlation extracted from job carrier")
	}
	outboxEnvelope := fromJob.Encode()
	outboxCorr := fromJob.Child(observability.SourceOutbox)
	outboxSpan := tracer.Start(outboxCorr, "dispatch.outbox")

	// 3. outbox -> tool call: the dispatcher decodes the stored correlation and
	// injects the next hop into the model provider request headers.
	decoded, err := observability.Decode(outboxEnvelope)
	if err != nil {
		t.Fatalf("decode outbox correlation: %v", err)
	}
	if decoded.ID != apiCorr.ID {
		t.Errorf("outbox envelope must preserve the correlation ID, got %s want %s", decoded.ID, apiCorr.ID)
	}
	if decoded.SpanID != jobCorr.SpanID {
		t.Errorf("outbox envelope must preserve the job span, got %s want %s", decoded.SpanID, jobCorr.SpanID)
	}

	toolCorr := outboxCorr.Child(observability.SourceToolCall)
	providerHeaders := http.Header{}
	toolCorr.Inject(observability.HeaderCarrier{Header: providerHeaders})
	toolSpan := tracer.Start(toolCorr, "call.model_tool")

	for _, c := range []observability.Correlation{apiCorr, jobCorr, outboxCorr, toolCorr} {
		if c.ID != apiCorr.ID {
			t.Errorf("correlation ID lost across boundaries: got %s want %s", c.ID, apiCorr.ID)
		}
		if c.TraceID != apiCorr.TraceID {
			t.Errorf("trace ID lost across boundaries: got %s want %s", c.TraceID, apiCorr.TraceID)
		}
	}

	if jobCorr.ParentSpanID != apiCorr.SpanID {
		t.Errorf("job span must descend from API span: got %s want %s", jobCorr.ParentSpanID, apiCorr.SpanID)
	}
	if outboxCorr.ParentSpanID != jobCorr.SpanID {
		t.Errorf("outbox span must descend from job span: got %s want %s", outboxCorr.ParentSpanID, jobCorr.SpanID)
	}
	if toolCorr.ParentSpanID != outboxCorr.SpanID {
		t.Errorf("tool call span must descend from outbox span: got %s want %s", toolCorr.ParentSpanID, outboxCorr.SpanID)
	}

	// The provider must receive the same correlation in its request headers.
	if got := providerHeaders.Get(observability.HeaderCorrelationID); got != apiCorr.ID {
		t.Errorf("tool call header correlation mismatch: got %s want %s", got, apiCorr.ID)
	}

	apiSpan = tracer.End(apiSpan, nil)
	jobSpan = tracer.End(jobSpan, nil)
	outboxSpan = tracer.End(outboxSpan, nil)
	toolSpan = tracer.End(toolSpan, nil)

	trace := tracer.Trace(apiCorr.TraceID)
	if len(trace) != 4 {
		t.Fatalf("expected 4 spans on one trace, got %d", len(trace))
	}
	seen := map[string]bool{}
	for _, s := range trace {
		if s.CorrelationID != apiCorr.ID {
			t.Errorf("span %s lost correlation ID: got %s want %s", s.Name, s.CorrelationID, apiCorr.ID)
		}
		if seen[s.SpanID] {
			t.Errorf("duplicate span ID %s in trace", s.SpanID)
		}
		seen[s.SpanID] = true
	}
}

// TestCorrelationContextPropagation ensures the correlation rides through
// context so every component in one hop logs with the same identity.
func TestCorrelationContextPropagation(t *testing.T) {
	corr := observability.NewCorrelation()
	ctx := observability.WithCorrelation(context.Background(), corr)

	got, ok := observability.FromContext(ctx)
	if !ok {
		t.Fatal("expected correlation in context")
	}
	if got.ID != corr.ID {
		t.Errorf("context correlation ID mismatch: got %s want %s", got.ID, corr.ID)
	}

	fromNew := observability.FromContextOrNew(context.Background())
	if fromNew.ID == "" {
		t.Error("FromContextOrNew must synthesize a correlation when none is present")
	}
	if observability.FromContextOrNew(ctx).ID != corr.ID {
		t.Error("FromContextOrNew must return the attached correlation when present")
	}
}

// TestCorrelationEncodeDecodeRoundTrip covers JSON encoding, bare-ID decoding
// and malformed input handling.
func TestCorrelationEncodeDecodeRoundTrip(t *testing.T) {
	corr := observability.NewCorrelation().Child(observability.SourceJob)

	encoded := corr.Encode()
	if encoded == "" {
		t.Fatal("expected non-empty encoded correlation")
	}

	decoded, err := observability.Decode(encoded)
	if err != nil {
		t.Fatalf("decode round trip: %v", err)
	}
	if decoded.ID != corr.ID || decoded.TraceID != corr.TraceID || decoded.SpanID != corr.SpanID {
		t.Errorf("round trip mismatch: got %+v want %+v", decoded, corr)
	}

	bare, err := observability.Decode("corr-bare-id-99")
	if err != nil {
		t.Fatalf("decode bare ID: %v", err)
	}
	if bare.ID != "corr-bare-id-99" || bare.TraceID != "corr-bare-id-99" {
		t.Errorf("bare ID decode mismatch: %+v", bare)
	}

	if _, err := observability.Decode(""); err == nil {
		t.Error("empty correlation must be rejected")
	}
}
