package compliance

import (
	"errors"
	"time"
)

const (
	ItemStatusActive  = "active"
	ItemStatusExpired = "expired"
	ItemStatusRenewed = "renewed"
	ItemStatusRevoked = "revoked"

	ItemKindPermission     = "permission"
	ItemKindRegistration   = "registration"
	ItemKindInsurance      = "insurance"
	ItemKindSafetyDocument = "safety_document"

	ItemSeverityCritical    = "critical"
	ItemSeverityNonCritical = "non_critical"

	RoleJarvis    = "jarvis"
	RoleSuperhost = "superhost"
)

var (
	ErrComplianceItemNotFound    = errors.New("compliance item not found")
	ErrInvalidComplianceItem     = errors.New("invalid compliance item")
	ErrItemNotActive             = errors.New("compliance item is not active")
	ErrSuperhostDenied           = errors.New("superhost cannot clear compliance holds")
	ErrComplianceRenewalNotFound = errors.New("compliance renewal warning not found")
)

var ValidItemKinds = []string{
	ItemKindPermission,
	ItemKindRegistration,
	ItemKindInsurance,
	ItemKindSafetyDocument,
}

var ValidItemSeverities = []string{ItemSeverityCritical, ItemSeverityNonCritical}

type ComplianceItem struct {
	ID            string    `json:"id"`
	PropertyID    string    `json:"property_id"`
	TenantID      string    `json:"tenant_id"`
	Kind          string    `json:"kind"`
	Severity      string    `json:"severity"`
	Name          string    `json:"name"`
	Description   string    `json:"description,omitempty"`
	EffectiveDate time.Time `json:"effective_date"`
	ExpiryDate    time.Time `json:"expiry_date"`
	Status        string    `json:"status"`
	EvidenceIDs   []string  `json:"evidence_ids,omitempty"`
	RenewedFromID *string   `json:"renewed_from_id,omitempty"`
	HoldID        *string   `json:"hold_id,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type ComplianceItemParams struct {
	PropertyID    string
	Kind          string
	Severity      string
	Name          string
	Description   string
	EffectiveDate time.Time
	ExpiryDate    time.Time
	EvidenceIDs   []string
}

type ComplianceRenewalWarning struct {
	ID               string     `json:"id"`
	ItemID           string     `json:"item_id"`
	PropertyID       string     `json:"property_id"`
	TenantID         string     `json:"tenant_id"`
	DaysBeforeExpiry int        `json:"days_before_expiry"`
	IssuedAt         time.Time  `json:"issued_at"`
	Acknowledged     *time.Time `json:"acknowledged_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

type ScanExpiryResult struct {
	Scanned         int `json:"scanned"`
	Expired         int `json:"expired"`
	HoldsCreated    int `json:"holds_created"`
	HoldsMaintained int `json:"holds_maintained"`
	WarningsIssued  int `json:"warnings_issued"`
}
