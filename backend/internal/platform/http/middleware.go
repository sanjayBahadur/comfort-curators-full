package http

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"

	"comfort-curators-backend/internal/observability"
	"comfort-curators-backend/internal/platform/logging"
)

func CorrelationID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		corr, ok := observability.Extract(observability.HeaderCarrier{Header: r.Header})
		if !ok {
			cid := r.Header.Get("X-Request-ID")
			if cid == "" {
				cid = newID()
			}
			corr = observability.NewCorrelation()
			corr.ID = cid
			corr.TraceID = cid
			corr.SpanID = cid
		}

		corr.Inject(observability.HeaderCarrier{Header: w.Header()})
		w.Header().Set("X-Request-ID", corr.ID)

		cid := corr.ID
		ctx := logging.WithCorrelationID(r.Context(), cid)
		ctx = logging.WithRequestID(ctx, cid)
		ctx = observability.WithCorrelation(ctx, corr)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func RequestLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		duration := time.Since(start)

		logging.L().LogAttrs(r.Context(), slog.LevelInfo, "http request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", sw.status),
			slog.Duration("duration", duration),
		)
	})
}

func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				logging.Error(r.Context(), "panic recovered",
					"panic", rec,
				)
				http.Error(w, `{"code":"INTERNAL_ERROR","message":"internal server error"}`, http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func ObservabilityMetrics(next http.Handler, m *observability.Metrics) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		duration := time.Since(start)

		m.APICall(r.Method, r.URL.Path, sw.status)
		m.APILatency(r.Method, r.URL.Path, duration)
	})
}

func ObservabilityTracing(next http.Handler, tr *observability.Tracer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		corr := observability.FromContextOrNew(r.Context())
		span := tr.Start(corr, r.Method+" "+r.URL.Path)

		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)

		var spanErr error
		if sw.status >= 500 {
			spanErr = &httpStatusError{code: sw.status}
		}
		span = tr.End(span, spanErr)
		_ = span
	})
}

type httpStatusError struct {
	code int
}

func (e *httpStatusError) Error() string {
	return http.StatusText(e.code)
}

func GracefulShutdown(ctx context.Context, srv *http.Server, timeout time.Duration) error {
	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	logging.Info(ctx, "shutting down http server")
	return srv.Shutdown(shutdownCtx)
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}

// Flush makes *statusWriter satisfy http.Flusher by forwarding to the
// wrapped ResponseWriter, if it supports flushing. Embedding
// http.ResponseWriter only promotes that interface's own methods
// (Header/Write/WriteHeader) -- http.Flusher is a separate interface, so
// without this method a *statusWriter never satisfies it, even though the
// real underlying ResponseWriter from Go's HTTP server does. Three
// middleware layers (ObservabilityTracing, ObservabilityMetrics,
// RequestLogging) each wrap every request in a fresh *statusWriter, so
// every SSE/streaming handler behind them silently lost the ability to
// flush -- confirmed live: GET /v1/superhost/threads/{id}/stream returned
// "streaming not supported" against the real server, despite streaming
// working correctly in every unit test (which calls handlers directly via
// mux.ServeHTTP, bypassing this whole middleware chain, so the bug was
// invisible to every test written against it).
func (sw *statusWriter) Flush() {
	if f, ok := sw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmtHex(time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func fmtHex(n int64) string {
	var b [8]byte
	for i := 7; i >= 0; i-- {
		b[i] = "0123456789abcdef"[n&0xf]
		n >>= 4
	}
	return string(b[:])
}

type RateLimiter interface {
	Allow(key string) bool
}

func RateLimit(next http.Handler, limiter RateLimiter, keyFunc func(*http.Request) string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := keyFunc(r)
		if !limiter.Allow(key) {
			http.Error(w, `{"code":"RATE_LIMITED","message":"too many requests"}`, http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func IPRateLimitKey(r *http.Request) string {
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.Header.Get("X-Real-IP")
	}
	if ip == "" {
		ip = r.RemoteAddr
	}
	return ip
}

func CombinedRateLimitKey(r *http.Request) string {
	return IPRateLimitKey(r) + ":" + r.URL.Path
}
