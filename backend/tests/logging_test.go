package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"comfort-curators-backend/internal/platform/logging"
)

func TestLoggingSecretRedaction(t *testing.T) {
	var buf bytes.Buffer
	h := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			lower := strings.ToLower(a.Key)
			for _, rk := range []string{"password", "pass", "secret", "token", "key", "authorization", "credential"} {
				if strings.Contains(lower, rk) {
					a.Value = slog.StringValue("[redacted]")
					return a
				}
			}
			return a
		},
	})
	l := slog.New(h)

	ctx := context.Background()
	l.LogAttrs(ctx, slog.LevelInfo, "test message",
		slog.String("db_password", "supersecret123"),
		slog.String("api_key", "sk-abcdef"),
		slog.String("normal_field", "visible_value"),
		slog.String("Authorization", "Bearer token123"),
		slog.String("user", "testuser"),
	)

	output := buf.String()
	if output == "" {
		t.Fatal("expected log output, got empty")
	}

	var entry map[string]any
	if err := json.Unmarshal([]byte(output), &entry); err != nil {
		t.Fatalf("failed to unmarshal log: %v (raw: %s)", err, output)
	}

	if v, ok := entry["db_password"].(string); !ok || v != "[redacted]" {
		t.Errorf("db_password should be redacted, got %v", entry["db_password"])
	}
	if v, ok := entry["api_key"].(string); !ok || v != "[redacted]" {
		t.Errorf("api_key should be redacted, got %v", entry["api_key"])
	}
	if v, ok := entry["Authorization"].(string); !ok || v != "[redacted]" {
		t.Errorf("Authorization should be redacted, got %v", entry["Authorization"])
	}
	if v, ok := entry["normal_field"].(string); !ok || v != "visible_value" {
		t.Errorf("normal_field should be visible, got %v", entry["normal_field"])
	}
	if v, ok := entry["user"].(string); !ok || v != "testuser" {
		t.Errorf("user should be visible, got %v", entry["user"])
	}
}

func TestLoggingCorrelationIDInContext(t *testing.T) {
	ctx := context.Background()
	ctx = logging.WithCorrelationID(ctx, "corr-abc-123")
	ctx = logging.WithRequestID(ctx, "req-def-456")

	if id := logging.CorrelationIDFromCtx(ctx); id != "corr-abc-123" {
		t.Errorf("expected corr-abc-123, got %s", id)
	}
	if id := logging.RequestIDFromCtx(ctx); id != "req-def-456" {
		t.Errorf("expected req-def-456, got %s", id)
	}
}

func TestLoggingMissingCorrelationIDReturnsEmpty(t *testing.T) {
	ctx := context.Background()

	if id := logging.CorrelationIDFromCtx(ctx); id != "" {
		t.Errorf("expected empty, got %s", id)
	}
	if id := logging.RequestIDFromCtx(ctx); id != "" {
		t.Errorf("expected empty, got %s", id)
	}
}
