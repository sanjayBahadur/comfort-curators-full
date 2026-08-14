package acceptance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateIsDeterministic(t *testing.T) {
	a := Generate(GenerateOptions{BaseURL: "http://127.0.0.1:8080"})
	b := Generate(GenerateOptions{BaseURL: "http://127.0.0.1:8080"})

	ab, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("marshal first fixture: %v", err)
	}
	bb, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("marshal second fixture: %v", err)
	}
	if string(ab) != string(bb) {
		t.Error("fixture generation must be deterministic for a clean database")
	}
}

func TestFixtureContainsAllRequiredKeys(t *testing.T) {
	f := Generate(GenerateOptions{BaseURL: "http://127.0.0.1:8080"})
	if err := f.Validate(); err != nil {
		t.Fatalf("valid fixture failed validation: %v", err)
	}

	flat, err := f.flatten()
	if err != nil {
		t.Fatalf("flatten fixture: %v", err)
	}
	for _, key := range RequiredKeys {
		if value := flat[key]; value == "" {
			t.Errorf("required key %q is missing or empty", key)
		}
	}
}

func TestFixtureEntityIDsAreUUIDs(t *testing.T) {
	f := Generate(GenerateOptions{BaseURL: "http://127.0.0.1:8080"})
	for _, item := range []struct {
		name string
		id   string
	}{
		{"tenant_a.id", f.TenantA.ID},
		{"tenant_b.id", f.TenantB.ID},
		{"property_a.id", f.PropertyA.ID},
		{"property_b.id", f.PropertyB.ID},
	} {
		if !isUUID(item.id) {
			t.Errorf("%s must be a UUID, got %q", item.name, item.id)
		}
	}
}

func TestFixtureEntityIDsAreDistinct(t *testing.T) {
	f := Generate(GenerateOptions{BaseURL: "http://127.0.0.1:8080"})
	ids := []string{f.TenantA.ID, f.TenantB.ID, f.PropertyA.ID, f.PropertyB.ID}
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			t.Errorf("duplicate entity id %q", id)
		}
		seen[id] = true
	}
}

func TestFixtureTokensAreDeterministicAndDistinct(t *testing.T) {
	f := Generate(GenerateOptions{BaseURL: "http://127.0.0.1:8080"})
	if f.OwnerA.Token != tokenFor("owner_a") {
		t.Error("owner_a.token must be derived deterministically")
	}
	if f.OwnerA.Token == f.OwnerB.Token {
		t.Error("distinct principals must receive distinct tokens")
	}
	if f.CuratorA.Token == f.CuratorB.Token {
		t.Error("distinct principals must receive distinct tokens")
	}
	if f.ActiveSupport.Token == f.ExpiredSupport.Token {
		t.Error("active and expired support grants must receive distinct tokens")
	}
}

func TestWriteFixtureCreatesOnlyExpectedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "acceptance-fixture.json")

	f := Generate(GenerateOptions{BaseURL: "http://127.0.0.1:8080"})
	if err := WriteFixture(path, f); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly one file, found %d", len(entries))
	}
	if entries[0].Name() != "acceptance-fixture.json" {
		t.Errorf("unexpected file %q", entries[0].Name())
	}

	encoded, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var parsed Fixture
	if err := json.Unmarshal(encoded, &parsed); err != nil {
		t.Fatalf("fixture must be valid JSON: %v", err)
	}
	if err := parsed.Validate(); err != nil {
		t.Fatalf("written fixture failed validation: %v", err)
	}
	if parsed.BaseURL != "http://127.0.0.1:8080" {
		t.Errorf("base_url mismatch, got %q", parsed.BaseURL)
	}
}

func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, r := range s {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
				return false
			}
		}
	}
	return true
}
