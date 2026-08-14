package tests

import (
	"os"
	"strings"
	"testing"

	"comfort-curators-backend/internal/platform/config"
)

func TestConfigValidateMissingRequiredFailsFast(t *testing.T) {
	os.Unsetenv("CC_DB_USER")
	os.Unsetenv("CC_DB_PASS")
	os.Unsetenv("CC_DB_NAME")

	cfg := config.LoadFromEnv()
	err := cfg.Validate()

	if err == nil {
		t.Fatal("expected validation error for missing required env vars")
	}
	msg := err.Error()
	if !strings.Contains(msg, "CC_DB_USER") {
		t.Errorf("error should mention CC_DB_USER, got: %s", msg)
	}
	if !strings.Contains(msg, "CC_DB_PASS") {
		t.Errorf("error should mention CC_DB_PASS, got: %s", msg)
	}
	if !strings.Contains(msg, "CC_DB_NAME") {
		t.Errorf("error should mention CC_DB_NAME, got: %s", msg)
	}
}

func TestConfigValidateSuccess(t *testing.T) {
	os.Setenv("CC_DB_USER", "testuser")
	os.Setenv("CC_DB_PASS", "testpass")
	os.Setenv("CC_DB_NAME", "testdb")
	defer func() {
		os.Unsetenv("CC_DB_USER")
		os.Unsetenv("CC_DB_PASS")
		os.Unsetenv("CC_DB_NAME")
	}()

	cfg := config.LoadFromEnv()
	err := cfg.Validate()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestConfigValidateInvalidPort(t *testing.T) {
	os.Setenv("CC_DB_USER", "testuser")
	os.Setenv("CC_DB_PASS", "testpass")
	os.Setenv("CC_DB_NAME", "testdb")
	os.Setenv("CC_HTTP_PORT", "99999")
	defer func() {
		os.Unsetenv("CC_DB_USER")
		os.Unsetenv("CC_DB_PASS")
		os.Unsetenv("CC_DB_NAME")
		os.Unsetenv("CC_HTTP_PORT")
	}()

	cfg := config.LoadFromEnv()
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for invalid port")
	}
}

func TestConfigValidateInvalidLogLevel(t *testing.T) {
	os.Setenv("CC_DB_USER", "testuser")
	os.Setenv("CC_DB_PASS", "testpass")
	os.Setenv("CC_DB_NAME", "testdb")
	os.Setenv("CC_LOG_LEVEL", "invalid")
	defer func() {
		os.Unsetenv("CC_DB_USER")
		os.Unsetenv("CC_DB_PASS")
		os.Unsetenv("CC_DB_NAME")
		os.Unsetenv("CC_LOG_LEVEL")
	}()

	cfg := config.LoadFromEnv()
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for invalid log level")
	}
}

func TestConfigSecretRedaction(t *testing.T) {
	os.Setenv("CC_DB_USER", "testuser")
	os.Setenv("CC_DB_PASS", "supersecret")
	os.Setenv("CC_DB_NAME", "testdb")
	defer func() {
		os.Unsetenv("CC_DB_USER")
		os.Unsetenv("CC_DB_PASS")
		os.Unsetenv("CC_DB_NAME")
	}()

	cfg := config.LoadFromEnv()
	s := cfg.String()

	if strings.Contains(s, "supersecret") {
		t.Error("Config.String() must not contain the DB password")
	}
	if !strings.Contains(s, "[redacted]") {
		t.Error("Config.String() should contain [redacted] for secret fields")
	}
}

func TestConfigSafeFieldsExcludesPass(t *testing.T) {
	os.Setenv("CC_DB_USER", "testuser")
	os.Setenv("CC_DB_PASS", "supersecret")
	os.Setenv("CC_DB_NAME", "testdb")
	defer func() {
		os.Unsetenv("CC_DB_USER")
		os.Unsetenv("CC_DB_PASS")
		os.Unsetenv("CC_DB_NAME")
	}()

	cfg := config.LoadFromEnv()
	fields := cfg.SafeFields()

	if _, ok := fields["db_pass"]; ok {
		t.Error("SafeFields() must not include db_pass")
	}
	if v, ok := fields["db_user"]; !ok || v != "testuser" {
		t.Error("SafeFields() must include non-secret fields")
	}
}

func TestConfigDBDSN(t *testing.T) {
	os.Setenv("CC_DB_USER", "testuser")
	os.Setenv("CC_DB_PASS", "testpass")
	os.Setenv("CC_DB_NAME", "testdb")
	os.Setenv("CC_DB_HOST", "dbhost")
	os.Setenv("CC_DB_PORT", "15432")
	defer func() {
		os.Unsetenv("CC_DB_USER")
		os.Unsetenv("CC_DB_PASS")
		os.Unsetenv("CC_DB_NAME")
		os.Unsetenv("CC_DB_HOST")
		os.Unsetenv("CC_DB_PORT")
	}()

	cfg := config.LoadFromEnv()
	dsn := cfg.DBDSN()

	if !strings.Contains(dsn, "host=dbhost") {
		t.Errorf("DSN should contain host=dbhost, got: %s", dsn)
	}
	if !strings.Contains(dsn, "port=15432") {
		t.Errorf("DSN should contain port=15432, got: %s", dsn)
	}
}
