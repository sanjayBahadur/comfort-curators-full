package api_test

import (
	"testing"

	"comfort-curators-backend/internal/api"
	"comfort-curators-backend/internal/platform/security"
	"comfort-curators-backend/internal/property"
)

func sampleProperty(id, authority string) property.Property {
	return property.Property{
		ID:               id,
		TenantID:         "tenant-a",
		OwnerAuthorityID: authority,
		AccessMethod:     "lockbox",
		State:            property.StateLead,
		Version:          1,
	}
}

func resolveAuthorities(actorID string) []string {
	switch actorID {
	case "actor-owner-1":
		return []string{"auth-owner-1"}
	case "actor-owner-2":
		return []string{"auth-owner-2"}
	case "actor-owner-multi":
		return []string{"auth-owner-1", "auth-owner-3"}
	}
	return nil
}

func ownerSubject(actorID string) security.Subject {
	return security.Subject{ActorID: actorID, TenantID: "tenant-a", Roles: []string{api.RoleOwner}}
}

func TestOwnerSeesOwnedPropertiesOnly(t *testing.T) {
	all := []property.Property{
		sampleProperty("prop-0001", "auth-owner-1"),
		sampleProperty("prop-0002", "auth-owner-2"),
		sampleProperty("prop-0003", "auth-owner-3"),
		sampleProperty("prop-0004", "auth-owner-1"),
	}

	got := api.OwnedProperties(ownerSubject("actor-owner-1"), all, resolveAuthorities)
	if len(got) != 2 {
		t.Fatalf("expected 2 owned properties, got %d", len(got))
	}
	for _, p := range got {
		if p.OwnerAuthorityID != "auth-owner-1" {
			t.Errorf("owner-1 received a property owned by %s", p.OwnerAuthorityID)
		}
	}

	multi := api.OwnedProperties(ownerSubject("actor-owner-multi"), all, resolveAuthorities)
	if len(multi) != 3 {
		t.Fatalf("expected 3 owned properties for multi-authority owner, got %d", len(multi))
	}
}

func TestOwnerNeverSeesForeignProperties(t *testing.T) {
	all := []property.Property{
		sampleProperty("prop-0001", "auth-owner-2"),
		sampleProperty("prop-0002", "auth-owner-3"),
	}

	got := api.OwnedProperties(ownerSubject("actor-owner-1"), all, resolveAuthorities)
	if len(got) != 0 {
		t.Fatalf("owner with no matching authority must see no properties, got %d", len(got))
	}

	if api.OwnsProperty(ownerSubject("actor-owner-1"), sampleProperty("prop-0001", "auth-owner-2"), resolveAuthorities) {
		t.Error("OwnsProperty must deny a foreign property")
	}
}

func TestOwnerScopeFailsClosedWithoutResolver(t *testing.T) {
	all := []property.Property{
		sampleProperty("prop-0001", "auth-owner-1"),
	}
	got := api.OwnedProperties(ownerSubject("actor-owner-1"), all, nil)
	if len(got) != 0 {
		t.Fatalf("owner scope with nil resolver must fail closed, got %d properties", len(got))
	}
	if api.OwnsProperty(ownerSubject("actor-owner-1"), sampleProperty("prop-0001", "auth-owner-1"), nil) {
		t.Error("OwnsProperty with nil resolver must deny")
	}
}

func TestOwnerScopeNeverWidensInput(t *testing.T) {
	all := []property.Property{
		sampleProperty("prop-0001", "auth-owner-1"),
		sampleProperty("prop-0002", "auth-owner-2"),
	}
	got := api.OwnedProperties(ownerSubject("actor-owner-1"), all, resolveAuthorities)
	for _, p := range got {
		if p.ID != "prop-0001" {
			t.Errorf("unexpected property in result: %s", p.ID)
		}
	}
}

func TestNonOwnerKeepsTenantScopedSet(t *testing.T) {
	all := []property.Property{
		sampleProperty("prop-0001", "auth-owner-1"),
		sampleProperty("prop-0002", "auth-owner-2"),
	}

	for _, subject := range []security.Subject{
		{ActorID: "actor-staff", TenantID: "tenant-a", Roles: []string{"staff"}},
		{ActorID: "actor-guest", TenantID: "tenant-a", Roles: []string{"guest"}},
		{ActorID: "actor-none", TenantID: "tenant-a", Roles: nil},
	} {
		got := api.OwnedProperties(subject, all, resolveAuthorities)
		if len(got) != len(all) {
			t.Errorf("roles %v: expected tenant-scoped set unchanged, got %d of %d",
				subject.Roles, len(got), len(all))
		}
		if api.OwnsProperty(subject, all[0], resolveAuthorities) != true {
			t.Errorf("roles %v: OwnsProperty must defer to tenant scoping", subject.Roles)
		}
	}
}
