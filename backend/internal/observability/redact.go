package observability

import (
	"encoding/json"
	"regexp"
	"strings"
)

// sensitiveTokens are substrings of attribute keys that mark the value as
// sensitive. Any log, trace, metric, alert or error attribute whose key
// contains one of these tokens must be redacted before it leaves the process.
var sensitiveTokens = []string{
	"password", "pass", "secret", "token", "key",
	"authorization", "credential", "dbpass", "db_pass",
	"pan", "cvv", "otp", "bank", "iban", "aadhaar", "license",
	"biometric", "pin", "api_key", "apikey", "access_key",
	"access_code", "passcode", "pass_code", "auth_code",
	"verification_code", "security_code", "one_time",
}

// IsSensitive reports whether an attribute key should be treated as sensitive.
func IsSensitive(key string) bool {
	lower := strings.ToLower(key)
	for _, t := range sensitiveTokens {
		if strings.Contains(lower, t) {
			return true
		}
	}
	return false
}

// RedactValue returns the value unchanged for a non-sensitive key and the
// canonical [redacted] placeholder for a sensitive key.
func RedactValue(key, value string) string {
	if IsSensitive(key) {
		return RedactedValue
	}
	return value
}

// RedactMap returns a copy of the map in which every sensitive value is
// replaced by the canonical [redacted] placeholder. Non-sensitive values are
// preserved so correlation and resource references survive redaction.
func RedactMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = RedactValue(k, v)
	}
	return out
}

// RedactArgs rewrites a flat key/value argument slice (the same shape used by
// slog-style APIs) so sensitive values are replaced by the redacted
// placeholder. The slice length is preserved.
func RedactArgs(args ...any) []any {
	out := make([]any, 0, len(args))
	for i := 0; i+1 < len(args); i += 2 {
		out = append(out, args[i])
		key, ok := args[i].(string)
		if !ok {
			out = append(out, args[i+1])
			continue
		}
		switch v := args[i+1].(type) {
		case string:
			out = append(out, RedactValue(key, v))
		case *string:
			if v == nil {
				out = append(out, nil)
				continue
			}
			r := RedactValue(key, *v)
			out = append(out, &r)
		default:
			if IsSensitive(key) {
				out = append(out, RedactedValue)
			} else {
				out = append(out, args[i+1])
			}
		}
	}
	return out
}

// RedactJSON walks a JSON document and replaces any value whose key is
// sensitive with the canonical [redacted] placeholder. It returns the original
// bytes unchanged when the document cannot be parsed, so callers never lose
// data to a redaction bug.
func RedactJSON(raw []byte) []byte {
	if len(raw) == 0 {
		return raw
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return raw
	}
	redactValue(&v)
	out, err := json.Marshal(v)
	if err != nil {
		return raw
	}
	return out
}

func redactValue(v *any) {
	switch n := (*v).(type) {
	case map[string]any:
		for k, child := range n {
			if IsSensitive(k) {
				n[k] = RedactedValue
				continue
			}
			redactValue(&child)
			n[k] = child
		}
	case []any:
		for i := range n {
			redactValue(&n[i])
		}
	}
}

// RedactMessage replaces the most common inline sensitive patterns in a free
// text message while preserving the surrounding context and any correlation
// identifiers.
func RedactMessage(msg string) string {
	out := msg
	for _, pair := range [][2]string{
		{`("password"|"pass"|"secret"|"token"|"key"|"authorization"|"credential"|"api_key"|"apikey")\s*[:=]\s*"[^"]*"`, `${1}: "[redacted]"`},
		{`(password|pass|secret|token|key|authorization|credential|api[_-]?key)\s*=\s*\S+`, `${1}=[redacted]`},
		{`(Bearer\s+)[A-Za-z0-9._~+/=-]+`, `${1}[redacted]`},
	} {
		out = regexReplace(out, pair[0], pair[1])
	}
	return out
}

func regexReplace(src, pattern, repl string) string {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return src
	}
	return re.ReplaceAllString(src, repl)
}
