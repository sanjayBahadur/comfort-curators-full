package acceptance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const DefaultFixturePath = ".runtime/acceptance-fixture.json"

var RequiredKeys = []string{
	"base_url",
	"tenant_a.id",
	"tenant_b.id",
	"owner_a.token",
	"owner_b.token",
	"supervisor_a.token",
	"curator_a.token",
	"curator_b.token",
	"vendor_a.token",
	"property_a.id",
	"property_b.id",
	"active_support.token",
	"expired_support.token",
}

var (
	tenantAID   = "11111111-1111-4111-8111-111111111111"
	tenantBID   = "22222222-2222-4222-8222-222222222222"
	propertyAID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	propertyBID = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
)

type FixtureItem struct {
	ID    string `json:"id,omitempty"`
	Token string `json:"token,omitempty"`
}

type Fixture struct {
	BaseURL        string      `json:"base_url"`
	TenantA        FixtureItem `json:"tenant_a"`
	TenantB        FixtureItem `json:"tenant_b"`
	OwnerA         FixtureItem `json:"owner_a"`
	OwnerB         FixtureItem `json:"owner_b"`
	SupervisorA    FixtureItem `json:"supervisor_a"`
	CuratorA       FixtureItem `json:"curator_a"`
	CuratorB       FixtureItem `json:"curator_b"`
	VendorA        FixtureItem `json:"vendor_a"`
	PropertyA      FixtureItem `json:"property_a"`
	PropertyB      FixtureItem `json:"property_b"`
	ActiveSupport  FixtureItem `json:"active_support"`
	ExpiredSupport FixtureItem `json:"expired_support"`
}

type GenerateOptions struct {
	BaseURL string
}

func Generate(opts GenerateOptions) Fixture {
	return Fixture{
		BaseURL: opts.BaseURL,
		TenantA: FixtureItem{ID: tenantAID},
		TenantB: FixtureItem{ID: tenantBID},
		OwnerA:  FixtureItem{Token: tokenFor("owner_a")},
		OwnerB:  FixtureItem{Token: tokenFor("owner_b")},
		SupervisorA: FixtureItem{
			Token: tokenFor("supervisor_a"),
		},
		CuratorA: FixtureItem{Token: tokenFor("curator_a")},
		CuratorB: FixtureItem{Token: tokenFor("curator_b")},
		VendorA:  FixtureItem{Token: tokenFor("vendor_a")},
		PropertyA: FixtureItem{
			ID: propertyAID,
		},
		PropertyB: FixtureItem{
			ID: propertyBID,
		},
		ActiveSupport:  FixtureItem{Token: tokenFor("active_support")},
		ExpiredSupport: FixtureItem{Token: tokenFor("expired_support")},
	}
}

func tokenFor(role string) string {
	sum := sha256.Sum256([]byte("cc-acceptance/" + role))
	return hex.EncodeToString(sum[:])
}

func (f Fixture) Validate() error {
	flat, err := f.flatten()
	if err != nil {
		return err
	}
	var missing []string
	for _, key := range RequiredKeys {
		if flat[key] == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("acceptance fixture is missing required keys: %s", strings.Join(missing, ", "))
	}
	return nil
}

func (f Fixture) flatten() (map[string]string, error) {
	encoded, err := json.Marshal(f)
	if err != nil {
		return nil, fmt.Errorf("acceptance fixture: marshal for validation: %w", err)
	}
	var top map[string]any
	if err := json.Unmarshal(encoded, &top); err != nil {
		return nil, fmt.Errorf("acceptance fixture: unmarshal for validation: %w", err)
	}
	flat := make(map[string]string)
	for key, value := range top {
		switch v := value.(type) {
		case string:
			flat[key] = v
		case map[string]any:
			for nestedKey, nestedValue := range v {
				if s, ok := nestedValue.(string); ok {
					flat[key+"."+nestedKey] = s
				}
			}
		}
	}
	return flat, nil
}

func WriteFixture(path string, f Fixture) error {
	if err := f.Validate(); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("acceptance fixture: marshal: %w", err)
	}
	encoded = append(encoded, '\n')
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("acceptance fixture: create directory: %w", err)
	}
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		return fmt.Errorf("acceptance fixture: write: %w", err)
	}
	return nil
}
