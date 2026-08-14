package api

import (
	"comfort-curators-backend/internal/platform/security"
	"comfort-curators-backend/internal/property"
)

// This file implements the owner-scoping guard of the protected API slice.
// Tenancy scoping is enforced by the service authorizer before this slice
// runs; this guard further restricts owner subjects to the properties they
// actually own. Authorization fails closed: an owner subject is never handed
// a property it does not own.

const RoleOwner = "owner"

// OwnerAuthorities resolves the owner authority IDs controlled by an actor.
// The resolver is injected because authority linkage is established by the
// onboarding domain; the guard itself only consumes the mapping.
type OwnerAuthorities func(actorID string) []string

// IsOwner reports whether the subject holds the owner role.
func IsOwner(subject security.Subject) bool {
	for _, role := range subject.Roles {
		if role == RoleOwner {
			return true
		}
	}
	return false
}

// OwnsProperty reports whether the subject may view property p. Non-owner
// subjects rely on the tenant scoping already enforced upstream and are
// permitted. Owner subjects may only view properties whose owner authority is
// among the authorities the actor controls.
func OwnsProperty(subject security.Subject, p property.Property, resolve OwnerAuthorities) bool {
	if !IsOwner(subject) {
		return true
	}
	if resolve == nil {
		return false
	}
	for _, authority := range resolve(subject.ActorID) {
		if authority == p.OwnerAuthorityID {
			return true
		}
	}
	return false
}

// OwnedProperties filters a tenant-scoped property list to the ones the
// subject is allowed to see, preserving order. It never widens the input set.
func OwnedProperties(subject security.Subject, props []property.Property, resolve OwnerAuthorities) []property.Property {
	out := make([]property.Property, 0, len(props))
	for _, p := range props {
		if OwnsProperty(subject, p, resolve) {
			out = append(out, p)
		}
	}
	return out
}
