package observability

import (
	"sync"
	"time"
)

// SpanStatus is the terminal status of a traced hop.
type SpanStatus string

const (
	SpanStatusOK    SpanStatus = "ok"
	SpanStatusError SpanStatus = "error"
)

// Span records one timed hop inside a trace. A trace is the ordered chain of
// spans that share one Correlation.TraceID; every span also carries the parent
// span ID so the full API -> job -> outbox -> tool-call path can be rebuilt.
type Span struct {
	TraceID       string            `json:"trace_id"`
	SpanID        string            `json:"span_id"`
	ParentSpanID  string            `json:"parent_span_id,omitempty"`
	CorrelationID string            `json:"correlation_id"`
	Name          string            `json:"name"`
	Source        HopSource         `json:"source"`
	StartedAt     time.Time         `json:"started_at"`
	EndedAt       time.Time         `json:"ended_at"`
	Duration      time.Duration     `json:"duration"`
	Status        SpanStatus        `json:"status"`
	Attrs         map[string]string `json:"attrs,omitempty"`
}

// Tracer records spans in memory. It is safe for concurrent use and is
// intended as the trace sink for the whole process.
type Tracer struct {
	mu    sync.Mutex
	spans []Span
}

// NewTracer returns an empty trace recorder.
func NewTracer() *Tracer {
	return &Tracer{}
}

// Start opens a new span for the given correlation and hop name. The span ID
// comes from the correlation so the span and the correlation hop are the same
// unit of work.
func (t *Tracer) Start(corr Correlation, name string) Span {
	return Span{
		TraceID:       corr.TraceID,
		SpanID:        corr.SpanID,
		ParentSpanID:  corr.ParentSpanID,
		CorrelationID: corr.ID,
		Name:          name,
		Source:        corr.Source,
		StartedAt:     time.Now().UTC(),
		Attrs:         map[string]string{},
	}
}

// AddAttr attaches a redacted attribute to an open span.
func (t *Tracer) AddAttr(s Span, key, value string) Span {
	if s.Attrs == nil {
		s.Attrs = map[string]string{}
	}
	s.Attrs[key] = RedactValue(key, value)
	return s
}

// End finishes a span, marks it ok or error and records it.
func (t *Tracer) End(s Span, err error) Span {
	s.EndedAt = time.Now().UTC()
	s.Duration = s.EndedAt.Sub(s.StartedAt)
	if err != nil {
		s.Status = SpanStatusError
		s.Attrs["error"] = redactError(err)
	} else {
		s.Status = SpanStatusOK
	}

	t.mu.Lock()
	t.spans = append(t.spans, s)
	t.mu.Unlock()
	return s
}

// Spans returns a copy of all recorded spans in completion order.
func (t *Tracer) Spans() []Span {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]Span, len(t.spans))
	copy(out, t.spans)
	return out
}

// Reset discards all recorded spans.
func (t *Tracer) Reset() {
	t.mu.Lock()
	t.spans = nil
	t.mu.Unlock()
}

// Trace returns all spans belonging to a trace ID, in completion order.
func (t *Tracer) Trace(traceID string) []Span {
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []Span
	for _, s := range t.spans {
		if s.TraceID == traceID {
			out = append(out, s)
		}
	}
	return out
}

// errorSafetyMargin is a guard so redaction stays conservative: an empty error
// is never reported as success.
func redactError(err error) string {
	if err == nil {
		return "error"
	}
	return err.Error()
}
