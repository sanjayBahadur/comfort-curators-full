package logging

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"sync"
)

type contextKey string

const (
	correlationIDKey contextKey = "correlation_id"
	requestIDKey     contextKey = "request_id"
)

var (
	mu     sync.Mutex
	logger *slog.Logger
)

func Init(level string) {
	mu.Lock()
	defer mu.Unlock()
	initLocked(level)
}

// initLocked builds the logger. Callers must already hold mu; it never
// locks itself so it is safe to call from other functions in this file
// that already hold the lock. (Init used to call itself indirectly via
// L(), and L() locked mu before calling Init() -- which locked mu again
// on the same goroutine. sync.Mutex is not reentrant, so that deadlocked
// unconditionally the first time L() ran before Init() had ever been
// called.)
func initLocked(level string) {
	var l slog.Level
	switch strings.ToLower(level) {
	case "debug":
		l = slog.LevelDebug
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: l,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == "" {
				return a
			}
			lower := strings.ToLower(a.Key)
			if containsRedacted(lower) {
				a.Value = slog.StringValue("[redacted]")
			}
			return a
		},
	}
	handler := slog.NewJSONHandler(os.Stderr, opts)
	logger = slog.New(handler)
}

func L() *slog.Logger {
	mu.Lock()
	defer mu.Unlock()
	if logger == nil {
		initLocked("info")
	}
	return logger
}

func WithCorrelationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, correlationIDKey, id)
}

func CorrelationIDFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(correlationIDKey).(string); ok {
		return v
	}
	return ""
}

func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

func RequestIDFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

func attrsFromCtx(ctx context.Context) []slog.Attr {
	var attrs []slog.Attr
	if cid := CorrelationIDFromCtx(ctx); cid != "" {
		attrs = append(attrs, slog.String("correlation_id", cid))
	}
	if rid := RequestIDFromCtx(ctx); rid != "" {
		attrs = append(attrs, slog.String("request_id", rid))
	}
	return attrs
}

func Info(ctx context.Context, msg string, args ...any) {
	a := append(attrsFromCtx(ctx), argsToAttrs(args)...)
	L().LogAttrs(ctx, slog.LevelInfo, msg, a...)
}

func Warn(ctx context.Context, msg string, args ...any) {
	a := append(attrsFromCtx(ctx), argsToAttrs(args)...)
	L().LogAttrs(ctx, slog.LevelWarn, msg, a...)
}

func Error(ctx context.Context, msg string, args ...any) {
	a := append(attrsFromCtx(ctx), argsToAttrs(args)...)
	L().LogAttrs(ctx, slog.LevelError, msg, a...)
}

func Debug(ctx context.Context, msg string, args ...any) {
	a := append(attrsFromCtx(ctx), argsToAttrs(args)...)
	L().LogAttrs(ctx, slog.LevelDebug, msg, a...)
}

func argsToAttrs(args []any) []slog.Attr {
	attrs := make([]slog.Attr, 0, len(args)/2)
	for i := 0; i+1 < len(args); i += 2 {
		key, ok := args[i].(string)
		if !ok {
			continue
		}
		attrs = append(attrs, slog.Any(key, args[i+1]))
	}
	return attrs
}

var redactedKeys = []string{
	"password", "pass", "secret", "token", "key",
	"authorization", "credential", "dbpass", "db_pass",
}

func containsRedacted(key string) bool {
	for _, rk := range redactedKeys {
		if strings.Contains(key, rk) {
			return true
		}
	}
	return false
}

func RedactedValue(val string) slog.Value {
	return slog.StringValue("[redacted]")
}
