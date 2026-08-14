package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
)

// HopSource identifies which boundary a correlation hop belongs to. The four
// boundaries that must share one correlation identity are the API, the durable
// job queue, the transactional outbox and model tool calls.
type HopSource string

const (
	SourceAPI      HopSource = "api"
	SourceJob      HopSource = "job"
	SourceOutbox   HopSource = "outbox"
	SourceToolCall HopSource = "tool_call"
)

// HTTP header names used to carry correlation state across process boundaries
// (API <-> clients, worker <-> model provider).
const (
	HeaderCorrelationID = "X-Correlation-ID"
	HeaderTraceID       = "X-Trace-ID"
	HeaderSpanID        = "X-Span-ID"
	HeaderParentSpanID  = "X-Parent-Span-ID"
	HeaderSource        = "X-Correlation-Source"
)

var (
	// ErrCorrelationMalformed is returned when an encoded correlation value
	// cannot be decoded.
	ErrCorrelationMalformed = errors.New("observability: malformed correlation")
)

// Correlation is the identity of one logical operation. The ID stays constant
// across the API, job, outbox and tool-call boundaries; the trace and span IDs
// build the hierarchy of hops within that operation.
type Correlation struct {
	ID           string    `json:"correlation_id"`
	TraceID      string    `json:"trace_id"`
	SpanID       string    `json:"span_id"`
	ParentSpanID string    `json:"parent_span_id,omitempty"`
	Source       HopSource `json:"source"`
}

// NewCorrelation starts a root correlation for a new operation.
func NewCorrelation() Correlation {
	id := newID()
	return Correlation{
		ID:      id,
		TraceID: id,
		SpanID:  id,
		Source:  SourceAPI,
	}
}

// Child returns the next hop in the same operation: it keeps the correlation
// and trace IDs, records this hop as the parent span and opens a new span for
// the given boundary.
func (c Correlation) Child(source HopSource) Correlation {
	return Correlation{
		ID:           c.ID,
		TraceID:      c.TraceID,
		SpanID:       newID(),
		ParentSpanID: c.SpanID,
		Source:       source,
	}
}

// WithSource returns a copy of the correlation attributed to the given hop.
func (c Correlation) WithSource(source HopSource) Correlation {
	c.Source = source
	return c
}

// Attrs renders the correlation as structured log fields so logs, traces and
// alerts always carry the same joinable identity.
func (c Correlation) Attrs() []slog.Attr {
	attrs := []slog.Attr{
		slog.String("correlation_id", c.ID),
		slog.String("trace_id", c.TraceID),
		slog.String("span_id", c.SpanID),
	}
	if c.ParentSpanID != "" {
		attrs = append(attrs, slog.String("parent_span_id", c.ParentSpanID))
	}
	if c.Source != "" {
		attrs = append(attrs, slog.String("source", string(c.Source)))
	}
	return attrs
}

// Encode serializes the correlation to a single string suitable for storage in
// a durable job payload, an outbox correlation_id column or a tool-call
// envelope.
func (c Correlation) Encode() string {
	b, err := json.Marshal(c)
	if err != nil {
		return c.ID
	}
	return string(b)
}

// Decode restores a correlation previously produced by Encode. It also accepts
// a bare ID so values written by earlier code that only stored the correlation
// ID remain decodable.
func Decode(s string) (Correlation, error) {
	if s == "" {
		return Correlation{}, ErrCorrelationMalformed
	}
	var c Correlation
	if err := json.Unmarshal([]byte(s), &c); err == nil {
		if c.ID == "" || c.TraceID == "" || c.SpanID == "" {
			return Correlation{}, ErrCorrelationMalformed
		}
		return c, nil
	}
	// Not JSON: treat the whole value as a bare correlation ID.
	c.ID = s
	c.TraceID = s
	c.SpanID = s
	return c, nil
}

// Carrier is a minimal key-value abstraction for injecting and extracting
// correlation state. HTTP headers and plain maps both satisfy it.
type Carrier interface {
	Get(key string) string
	Set(key string, value string)
}

// HeaderCarrier adapts http.Header.
type HeaderCarrier struct {
	Header http.Header
}

// Get returns the value for a header key.
func (c HeaderCarrier) Get(key string) string { return c.Header.Get(key) }

// Set stores a header key value.
func (c HeaderCarrier) Set(key string, value string) { c.Header.Set(key, value) }

// MapCarrier adapts map[string]string.
type MapCarrier map[string]string

// Get returns the value for a key.
func (c MapCarrier) Get(key string) string { return c[key] }

// Set stores a key value.
func (c MapCarrier) Set(key string, value string) { c[key] = value }

// Inject writes the correlation into the carrier so a downstream boundary can
// extract it and continue the same operation.
func (c Correlation) Inject(carrier Carrier) {
	carrier.Set(HeaderCorrelationID, c.ID)
	carrier.Set(HeaderTraceID, c.TraceID)
	carrier.Set(HeaderSpanID, c.SpanID)
	if c.ParentSpanID != "" {
		carrier.Set(HeaderParentSpanID, c.ParentSpanID)
	}
	carrier.Set(HeaderSource, string(c.Source))
}

// Extract reads a correlation from a carrier. When the carrier carries only a
// correlation ID (the minimum contract), a root trace is synthesized around it
// so downstream hops can still build a trace. It returns false when the carrier
// holds no correlation state at all.
func Extract(carrier Carrier) (Correlation, bool) {
	id := carrier.Get(HeaderCorrelationID)
	if id == "" {
		return Correlation{}, false
	}
	traceID := carrier.Get(HeaderTraceID)
	if traceID == "" {
		traceID = id
	}
	spanID := carrier.Get(HeaderSpanID)
	if spanID == "" {
		spanID = id
	}
	parentSpanID := carrier.Get(HeaderParentSpanID)
	source := HopSource(carrier.Get(HeaderSource))
	if source == "" {
		source = SourceAPI
	}
	return Correlation{
		ID:           id,
		TraceID:      traceID,
		SpanID:       spanID,
		ParentSpanID: parentSpanID,
		Source:       source,
	}, true
}

type contextKey struct{}

var correlationContextKey contextKey

// WithCorrelation attaches a correlation to the context so every hop in the
// operation logs, traces and alerts with the same identity.
func WithCorrelation(ctx context.Context, c Correlation) context.Context {
	return context.WithValue(ctx, correlationContextKey, c)
}

// FromContext returns the correlation attached to the context.
func FromContext(ctx context.Context) (Correlation, bool) {
	c, ok := ctx.Value(correlationContextKey).(Correlation)
	return c, ok
}

// FromContextOrNew returns the attached correlation or a fresh root correlation
// when the context carries none.
func FromContextOrNew(ctx context.Context) Correlation {
	if c, ok := FromContext(ctx); ok {
		return c
	}
	return NewCorrelation()
}

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return hex.EncodeToString(randFallback())
	}
	return hex.EncodeToString(b[:])
}

// randFallback provides a deterministic fallback so ID generation never blocks
// on a broken entropy source. It is only used when crypto/rand fails.
func randFallback() []byte {
	return []byte("observability-fallback-id-0000000000000000")
}
