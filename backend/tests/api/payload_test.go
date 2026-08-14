package api_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"comfort-curators-backend/internal/api"
	"comfort-curators-backend/internal/property"
)

func propertyWithAccessMaterial() property.Property {
	return property.Property{
		ID:               "prop_0123456789abcdef",
		TenantID:         "tenant-a",
		OwnerAuthorityID: "auth-owner-1",
		ServiceAddress: property.Address{
			Line1:      "14 Marine Drive",
			City:       "Lucknow",
			State:      "Uttar Pradesh",
			PostalCode: "226001",
			Country:    "IN",
		},
		GeolocationZone: "zone-lko-north",
		Timezone:        "Asia/Kolkata",
		EmergencyContacts: []property.EmergencyContact{
			{Name: "Asha", Phone: "+91-9000000000", Role: "neighbour"},
		},
		AccessMethod:     "lockbox",
		MaximumOccupancy: 4,
		State:            property.StateReadyInactive,
		Readiness: property.Readiness{
			OwnerContractAccepted: true,
			ComplianceComplete:    true,
			MandatoryFieldsSet:    true,
		},
		Version:   3,
		CreatedAt: time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC),
	}
}

func TestOrdinaryPropertyExcludesAccessMaterial(t *testing.T) {
	view := api.OrdinaryProperty(propertyWithAccessMaterial())

	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal ordinary property: %v", err)
	}
	payload := string(encoded)

	for _, key := range api.AccessMaterialKeys {
		if strings.Contains(payload, key) {
			t.Errorf("ordinary property payload must not contain access material key %q: %s", key, payload)
		}
	}
	if strings.Contains(payload, "lockbox") {
		t.Errorf("ordinary property payload must not leak the access method value: %s", payload)
	}

	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("decode ordinary property: %v", err)
	}
	if _, present := decoded["access_method"]; present {
		t.Errorf("ordinary property payload includes access_method")
	}
}

func TestOrdinaryPropertyKeepsOrdinaryFields(t *testing.T) {
	p := propertyWithAccessMaterial()
	view := api.OrdinaryProperty(p)

	if view.ID != p.ID || view.Version != p.Version {
		t.Errorf("ordinary view must preserve id and version")
	}
	if view.OwnerAuthorityID != p.OwnerAuthorityID {
		t.Errorf("ordinary view must preserve owner_authority_id")
	}
	if view.State != p.State {
		t.Errorf("ordinary view must preserve state")
	}
	if view.MaximumOccupancy != p.MaximumOccupancy {
		t.Errorf("ordinary view must preserve maximum_occupancy")
	}
	if view.Timezone != p.Timezone {
		t.Errorf("ordinary view must preserve timezone")
	}
	if len(view.EmergencyContacts) != 1 {
		t.Errorf("ordinary view must preserve emergency contacts")
	}
	if !view.Readiness.Ready() {
		t.Errorf("ordinary view must preserve readiness")
	}
}

func TestAccessMaterialOnlyIsNarrow(t *testing.T) {
	p := propertyWithAccessMaterial()
	material := api.AccessMaterialOnly(p)

	if material[api.AccessMaterialKeyAccessMethod] != "lockbox" {
		t.Fatalf("access material must expose access_method, got %v", material)
	}
	if len(material) != 1 {
		t.Errorf("access material must be narrow, got %d fields: %v", len(material), material)
	}
	encoded, _ := json.Marshal(material)
	if strings.Contains(string(encoded), "tenant_id") || strings.Contains(string(encoded), "service_address") {
		t.Errorf("access material must not leak ordinary fields: %s", string(encoded))
	}
}

func TestAccessMaterialKeysDocumented(t *testing.T) {
	for _, key := range api.AccessMaterialKeys {
		if key == "" {
			t.Fatalf("access material key list contains an empty key")
		}
	}
}
