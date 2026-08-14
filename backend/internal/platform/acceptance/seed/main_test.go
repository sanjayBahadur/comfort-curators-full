package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"comfort-curators-backend/internal/platform/acceptance"
)

func TestRunRefusesUnlessAcceptanceEnv(t *testing.T) {
	for _, env := range []string{"", "dev", "production"} {
		getenv := func(key string) string {
			if key == "CC_ENV" {
				return env
			}
			return ""
		}
		err := run(getenv, &bytes.Buffer{})
		if err == nil {
			t.Errorf("expected refusal for CC_ENV=%q", env)
			continue
		}
		if !strings.Contains(err.Error(), "CC_ENV") {
			t.Errorf("refusal must mention CC_ENV, got: %v", err)
		}
	}
}

func TestRunWritesFixture(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.json")

	getenv := func(key string) string {
		switch key {
		case "CC_ENV":
			return "acceptance"
		case "CC_BASE_URL":
			return "http://127.0.0.1:18080"
		case "CC_FIXTURE_PATH":
			return path
		}
		return ""
	}

	if err := run(getenv, &bytes.Buffer{}); err != nil {
		t.Fatalf("run: %v", err)
	}

	encoded, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fixture acceptance.Fixture
	if err := json.Unmarshal(encoded, &fixture); err != nil {
		t.Fatalf("fixture must be valid JSON: %v", err)
	}
	if fixture.BaseURL != "http://127.0.0.1:18080" {
		t.Errorf("base_url must honor CC_BASE_URL, got %q", fixture.BaseURL)
	}
	if err := fixture.Validate(); err != nil {
		t.Fatalf("written fixture failed validation: %v", err)
	}
}

func TestRunDefaultsBaseURLToLocalhostHTTPPort(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.json")

	getenv := func(key string) string {
		switch key {
		case "CC_ENV":
			return "acceptance"
		case "CC_HTTP_PORT":
			return "19090"
		case "CC_FIXTURE_PATH":
			return path
		}
		return ""
	}

	if err := run(getenv, &bytes.Buffer{}); err != nil {
		t.Fatalf("run: %v", err)
	}

	encoded, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fixture acceptance.Fixture
	if err := json.Unmarshal(encoded, &fixture); err != nil {
		t.Fatalf("fixture must be valid JSON: %v", err)
	}
	if fixture.BaseURL != "http://127.0.0.1:19090" {
		t.Errorf("default base_url must use CC_HTTP_PORT, got %q", fixture.BaseURL)
	}
}
