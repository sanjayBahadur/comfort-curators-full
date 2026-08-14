package observability_test

import (
	"encoding/json"
	"strings"
	"testing"

	"comfort-curators-backend/internal/observability"
)

// TestRedactionProtectsSensitiveKeys verifies attribute keys that look
// sensitive never leave the process with their value, while correlation and
// ordinary fields survive untouched.
func TestRedactionProtectsSensitiveKeys(t *testing.T) {
	cases := map[string]string{
		"password":        "hunter2",
		"db_password":     "supersecret",
		"access_token":    "tok_abc123",
		"Authorization":   "Bearer abc123",
		"api_key":         "sk-live-xyz",
		"secret":          "s3cr3t",
		"credential":      "cred-value",
		"card_pan":        "4111111111111111",
		"cvv":             "123",
		"otp":             "481516",
		"correlation_id":  "corr-visible-001",
		"property_id":     "prop-42",
		"request_id":      "req-999",
		"tenant_id":       "tenant-1",
		"status":          "201",
		"blocked_items":   "3",
		"available_crew":  "1",
		"business_effect": "Property cannot be offered",
	}
	for key, value := range cases {
		got := observability.RedactValue(key, value)
		if observability.IsSensitive(key) {
			if got != observability.RedactedValue {
				t.Errorf("key %q must be redacted, got %q", key, got)
			}
		} else if got != value {
			t.Errorf("key %q must pass through, got %q want %q", key, got, value)
		}
	}
}

// TestRedactionPreservesCorrelationAcrossLogsAndTraces verifies that after
// redaction, the correlation and trace identity remain present and joinable.
func TestRedactionPreservesCorrelationAcrossLogsAndTraces(t *testing.T) {
	corr := observability.NewCorrelation()
	corr.ID = "corr-redact-777"

	line := observability.RedactArgs(
		"correlation_id", corr.ID,
		"trace_id", corr.TraceID,
		"db_password", "supersecret",
		"api_key", "sk-abcdef",
		"method", "POST",
	)
	if len(line) != 10 {
		t.Fatalf("expected 10 redacted args, got %d", len(line))
	}
	assertPair := func(k, v string) {
		for i := 0; i+1 < len(line); i += 2 {
			if line[i] == k {
				if line[i+1] != v {
					t.Errorf("arg %q = %v, want %q", k, line[i+1], v)
				}
				return
			}
		}
		t.Errorf("arg %q missing from redacted output", k)
	}
	assertPair("correlation_id", corr.ID)
	assertPair("trace_id", corr.TraceID)
	assertPair("db_password", observability.RedactedValue)
	assertPair("api_key", observability.RedactedValue)
	assertPair("method", "POST")
}

// TestRedactMapKeepsCorrelationAndReferences verifies map redaction keeps
// business and correlation references while scrubbing sensitive values.
func TestRedactMapKeepsCorrelationAndReferences(t *testing.T) {
	in := map[string]string{
		"correlation_id": "corr-map-1",
		"property_id":    "prop-42",
		"incident_ref":   "incident-77",
		"access_code":    "door-1234",
		"bank_account":   "000123456",
	}
	out := observability.RedactMap(in)

	if out["correlation_id"] != "corr-map-1" {
		t.Errorf("correlation_id must survive redaction, got %q", out["correlation_id"])
	}
	if out["property_id"] != "prop-42" {
		t.Errorf("property_id must survive redaction, got %q", out["property_id"])
	}
	if out["incident_ref"] != "incident-77" {
		t.Errorf("incident_ref must survive redaction, got %q", out["incident_ref"])
	}
	if out["access_code"] != observability.RedactedValue {
		t.Errorf("access_code must be redacted, got %q", out["access_code"])
	}
	if out["bank_account"] != observability.RedactedValue {
		t.Errorf("bank_account must be redacted, got %q", out["bank_account"])
	}
}

// TestRedactJSONScrubsNestedSensitiveValues verifies nested JSON documents are
// walked and sensitive values replaced while correlation stays intact.
func TestRedactJSONScrubsNestedSensitiveValues(t *testing.T) {
	raw := []byte(`{
		"correlation_id": "corr-json-9",
		"tenant_id": "tenant-1",
		"payload": {
			"password": "hunter2",
			"access_token": "tok-x",
			"property_id": "prop-42"
		}
	}`)

	out := observability.RedactJSON(raw)

	var doc map[string]any
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatalf("redacted JSON must remain valid: %v", err)
	}
	if doc["correlation_id"] != "corr-json-9" {
		t.Errorf("correlation_id must survive JSON redaction, got %v", doc["correlation_id"])
	}
	if doc["tenant_id"] != "tenant-1" {
		t.Errorf("tenant_id must survive JSON redaction, got %v", doc["tenant_id"])
	}
	payload := doc["payload"].(map[string]any)
	if payload["password"] != observability.RedactedValue {
		t.Errorf("nested password must be redacted, got %v", payload["password"])
	}
	if payload["access_token"] != observability.RedactedValue {
		t.Errorf("nested access_token must be redacted, got %v", payload["access_token"])
	}
	if payload["property_id"] != "prop-42" {
		t.Errorf("nested property_id must survive, got %v", payload["property_id"])
	}
}

// TestRedactMessageScrubsInlineSecrets verifies inline secret patterns in free
// text are replaced while the surrounding message survives.
func TestRedactMessageScrubsInlineSecrets(t *testing.T) {
	in := "authorization failed for token=abc123 with key \"secret\": Bearer xyz789"
	out := observability.RedactMessage(in)

	if strings.Contains(out, "abc123") {
		t.Errorf("message still contains token value: %s", out)
	}
	if strings.Contains(out, "xyz789") {
		t.Errorf("message still contains bearer value: %s", out)
	}
	if !strings.Contains(out, "authorization failed") {
		t.Errorf("message context must survive redaction: %s", out)
	}
	if !strings.Contains(out, observability.RedactedValue) {
		t.Errorf("message must contain redacted placeholder: %s", out)
	}
}

// TestRedactArgsNilPointer verifies nil pointer values inside arg slices do not
// panic and are preserved.
func TestRedactArgsNilPointer(t *testing.T) {
	var p *string
	out := observability.RedactArgs("correlation_id", "corr-1", "note", p)
	if out[3] != nil {
		t.Errorf("nil pointer must survive redaction, got %v", out[3])
	}
}
