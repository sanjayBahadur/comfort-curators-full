package api

import (
	"time"

	"comfort-curators-backend/internal/property"
)

// This file defines the ordinary property payload of the protected API slice.
// Per the architecture contract, access material is never returned in general
// property payloads; it is only released through a narrow disclosure flow.
// OrdinaryProperty therefore drops every access-material field, while
// AccessMaterialOnly exposes exactly the fields the disclosure flow needs.

// AccessMaterialKeyAccessMethod is the property field that records how a
// property is entered (for example a lockbox) and is treated as access
// material.
const AccessMaterialKeyAccessMethod = "access_method"

// AccessMaterialKeys lists the property payload fields that constitute access
// material and MUST NOT appear in ordinary property payloads.
var AccessMaterialKeys = []string{AccessMaterialKeyAccessMethod}

// PropertyView is the ordinary (non-access) representation of a property.
type PropertyView struct {
	ID                string                      `json:"id"`
	Version           int                         `json:"version"`
	TenantID          string                      `json:"tenant_id"`
	OwnerAuthorityID  string                      `json:"owner_authority_id"`
	ServiceAddress    property.Address            `json:"service_address"`
	GeolocationZone   string                      `json:"geolocation_zone"`
	Timezone          string                      `json:"timezone"`
	EmergencyContacts []property.EmergencyContact `json:"emergency_contacts"`
	MaximumOccupancy  int                         `json:"maximum_occupancy"`
	State             string                      `json:"state"`
	Readiness         property.Readiness          `json:"readiness"`
	ComplianceHolds   []property.ComplianceHold   `json:"compliance_holds,omitempty"`
	CreatedAt         time.Time                   `json:"created_at"`
	UpdatedAt         time.Time                   `json:"updated_at"`
}

// OrdinaryProperty renders property p as the ordinary payload, excluding all
// access material.
func OrdinaryProperty(p property.Property) PropertyView {
	return PropertyView{
		ID:                p.ID,
		Version:           p.Version,
		TenantID:          p.TenantID,
		OwnerAuthorityID:  p.OwnerAuthorityID,
		ServiceAddress:    p.ServiceAddress,
		GeolocationZone:   p.GeolocationZone,
		Timezone:          p.Timezone,
		EmergencyContacts: p.EmergencyContacts,
		MaximumOccupancy:  p.MaximumOccupancy,
		State:             p.State,
		Readiness:         p.Readiness,
		ComplianceHolds:   p.ComplianceHolds,
		CreatedAt:         p.CreatedAt,
		UpdatedAt:         p.UpdatedAt,
	}
}

// AccessMaterialOnly returns the access-material fields of property p for the
// narrow disclosure flow. It contains nothing else.
func AccessMaterialOnly(p property.Property) map[string]string {
	return map[string]string{
		AccessMaterialKeyAccessMethod: p.AccessMethod,
	}
}
