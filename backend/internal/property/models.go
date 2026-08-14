package property

import (
	"errors"
	"time"
)

var (
	ErrPropertyNotFound      = errors.New("property not found")
	ErrInvalidState          = errors.New("invalid lifecycle state")
	ErrInvalidTransition     = errors.New("invalid lifecycle transition")
	ErrArchivedTerminal      = errors.New("archived properties cannot transition")
	ErrComplianceHold        = errors.New("critical compliance hold blocks activation")
	ErrNotReady              = errors.New("property is not ready for activation")
	ErrInvalidComplianceHold = errors.New("invalid compliance hold")
	ErrHoldNotFound          = errors.New("compliance hold not found")
	ErrHoldNotOpen           = errors.New("compliance hold is not open")
	ErrExceptionDenied       = errors.New("compliance exception is denied")
)

const (
	StateLead          = "lead"
	StateQualifying    = "qualifying"
	StateOnboarding    = "onboarding"
	StateRemediation   = "remediation"
	StateReadyInactive = "ready_inactive"
	StateActive        = "active"
	StatePaused        = "paused"
	StateSuspended     = "suspended"
	StateOffboarding   = "offboarding"
	StateArchived      = "archived"
)

// AllStates mirrors the frozen V0 lifecycle exactly.
var AllStates = []string{
	StateLead,
	StateQualifying,
	StateOnboarding,
	StateRemediation,
	StateReadyInactive,
	StateActive,
	StatePaused,
	StateSuspended,
	StateOffboarding,
	StateArchived,
}

const (
	HoldSeverityCritical    = "critical"
	HoldSeverityNonCritical = "non_critical"

	HoldKindPermission     = "permission"
	HoldKindRegistration   = "registration"
	HoldKindInsurance      = "insurance"
	HoldKindSafetyDocument = "safety_document"

	HoldStatusOpen     = "open"
	HoldStatusResolved = "resolved"
	HoldStatusExcepted = "excepted"
)

var ValidHoldKinds = []string{
	HoldKindPermission,
	HoldKindRegistration,
	HoldKindInsurance,
	HoldKindSafetyDocument,
}

var ValidHoldSeverities = []string{HoldSeverityCritical, HoldSeverityNonCritical}

type Address struct {
	Line1      string `json:"line1"`
	Line2      string `json:"line2,omitempty"`
	City       string `json:"city"`
	State      string `json:"state"`
	PostalCode string `json:"postal_code"`
	Country    string `json:"country"`
}

type EmergencyContact struct {
	Name  string `json:"name"`
	Phone string `json:"phone"`
	Role  string `json:"role,omitempty"`
}

type Readiness struct {
	OwnerContractAccepted bool `json:"owner_contract_accepted"`
	ComplianceComplete    bool `json:"compliance_complete"`
	MandatoryFieldsSet    bool `json:"mandatory_fields_set"`
}

// Ready reports whether the frozen mandatory readiness inputs are satisfied.
// PROP-002: a property MUST NOT become active until mandatory compliance
// fields and the owner contract are complete.
func (r Readiness) Ready() bool {
	return r.OwnerContractAccepted && r.ComplianceComplete && r.MandatoryFieldsSet
}

type Property struct {
	ID                string             `json:"id"`
	TenantID          string             `json:"tenant_id"`
	OwnerAuthorityID  string             `json:"owner_authority_id"`
	ServiceAddress    Address            `json:"service_address"`
	GeolocationZone   string             `json:"geolocation_zone"`
	Timezone          string             `json:"timezone"`
	EmergencyContacts []EmergencyContact `json:"emergency_contacts"`
	AccessMethod      string             `json:"access_method"`
	MaximumOccupancy  int                `json:"maximum_occupancy"`
	State             string             `json:"state"`
	Readiness         Readiness          `json:"readiness"`
	ComplianceHolds   []ComplianceHold   `json:"compliance_holds,omitempty"`
	Version           int                `json:"version"`
	CreatedAt         time.Time          `json:"created_at"`
	UpdatedAt         time.Time          `json:"updated_at"`
}

type ComplianceHold struct {
	ID                 string     `json:"id"`
	PropertyID         string     `json:"property_id"`
	TenantID           string     `json:"tenant_id"`
	Kind               string     `json:"kind"`
	Severity           string     `json:"severity"`
	Status             string     `json:"status"`
	Reason             string     `json:"reason"`
	ExpiresAt          *time.Time `json:"expires_at,omitempty"`
	ExceptionBy        string     `json:"exception_by,omitempty"`
	ExceptionAt        *time.Time `json:"exception_at,omitempty"`
	ExceptionExpiresAt *time.Time `json:"exception_expires_at,omitempty"`
	ResolvedAt         *time.Time `json:"resolved_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
}

type CreatePropertyParams struct {
	TenantID          string
	OwnerAuthorityID  string
	ServiceAddress    Address
	GeolocationZone   string
	Timezone          string
	EmergencyContacts []EmergencyContact
	AccessMethod      string
	MaximumOccupancy  int
	InitialState      string
}

type ComplianceHoldParams struct {
	Kind      string
	Severity  string
	Reason    string
	ExpiresAt *time.Time
}

type PropertyTransition struct {
	ID          string    `json:"id"`
	PropertyID  string    `json:"property_id"`
	TenantID    string    `json:"tenant_id"`
	FromState   string    `json:"from_state"`
	ToState     string    `json:"to_state"`
	ActorID     string    `json:"actor_id"`
	Reason      string    `json:"reason"`
	FromVersion int       `json:"from_version"`
	ToVersion   int       `json:"to_version"`
	CreatedAt   time.Time `json:"created_at"`
}
