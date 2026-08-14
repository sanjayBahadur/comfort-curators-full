package api

import (
	"bytes"
	"fmt"

	"comfort-curators-backend/internal/platform/security"
)

// This file implements the owner-finance guard of the protected document,
// billing, approval and reporting API slice. Every finance, document and
// reporting record is tenant-scoped and property-scoped. The guard restricts
// owner subjects to the properties whose authority they control and guarantees
// the slice never surfaces another owner's records or worker HR details.
// Authorization fails closed: an owner subject is never handed a record for a
// property it does not control, and the slice never emits workforce HR fields.

// PropertyAuthority resolves the owner authority that controls a property.
// The mapping is injected because authority linkage is established by the
// property domain; the guard only consumes the mapping.
type PropertyAuthority func(propertyID string) string

// OwnerControlsPropertyScope reports whether an owner subject may view a
// tenant-scoped, property-scoped finance, document or reporting record for
// the given property. Non-owner subjects defer to the tenant scoping enforced
// upstream. It fails closed when the authority mapping or resolver is absent.
func OwnerControlsPropertyScope(subject security.Subject, propertyID string, ownerAuthority PropertyAuthority, resolve OwnerAuthorities) bool {
	if !IsOwner(subject) {
		return true
	}
	if resolve == nil || ownerAuthority == nil {
		return false
	}
	authority := ownerAuthority(propertyID)
	for _, a := range resolve(subject.ActorID) {
		if a == authority {
			return true
		}
	}
	return false
}

// FilterOwnedRecords filters tenant-scoped, property-scoped records to the
// ones whose property the owner subject controls, preserving order. It never
// widens the input set and fails closed for owner subjects without a
// resolvable authority mapping. Non-owner subjects keep the tenant-scoped set
// unchanged.
func FilterOwnedRecords[T any](subject security.Subject, records []T, propertyID func(T) string, ownerAuthority PropertyAuthority, resolve OwnerAuthorities) []T {
	if !IsOwner(subject) {
		return records
	}
	out := make([]T, 0, len(records))
	for _, r := range records {
		if OwnerControlsPropertyScope(subject, propertyID(r), ownerAuthority, resolve) {
			out = append(out, r)
		}
	}
	return out
}

// WorkerHRFieldKeys are the workforce HR detail fields that the owner-finance
// slice must never surface. Workforce HR records belong to the workforce
// module; the protected finance, document, approval and reporting slices only
// ever carry opaque worker identifiers and property-scoped aggregates.
var WorkerHRFieldKeys = []string{
	"legal_name",
	"date_of_birth",
	"contact_method",
	"compensation_band",
	"employment_term",
	"grievance",
	"adverse_action",
	"sos_event",
}

// WorkerHRNotExposed verifies that a marshaled owner-finance payload does not
// contain any worker HR detail field. It fails closed by returning an error
// naming the offending field, so a finance, document, approval or reporting
// view can never leak worker HR material downstream.
func WorkerHRNotExposed(payload []byte) error {
	for _, key := range WorkerHRFieldKeys {
		if bytes.Contains(payload, []byte(`"`+key+`"`)) {
			return fmt.Errorf("worker HR field %q must not be exposed in the owner-finance slice", key)
		}
	}
	return nil
}
