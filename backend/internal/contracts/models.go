package contracts

import (
	"errors"
	"time"
)

var (
	ErrQuoteInputsInvalid       = errors.New("invalid quote inputs")
	ErrFeeRuleNotFound          = errors.New("fee rule not found")
	ErrInvalidFeeRule           = errors.New("invalid fee rule")
	ErrAgreementNotFound        = errors.New("service agreement not found")
	ErrAgreementVersionNotFound = errors.New("service agreement version not found")
	ErrEmptyTerms               = errors.New("agreement terms are required")
	ErrInvalidAgreement         = errors.New("invalid service agreement")
	ErrAcceptedImmutable        = errors.New("accepted service agreement is immutable")
	ErrAlreadyAccepted          = errors.New("service agreement is already accepted")
	ErrCrossTenantDenied        = errors.New("cross-tenant access denied")
)

// Service tier keys. The frozen V0 tiers price differently through the fee
// rules; the tier is part of the quote inputs so the same revenue on a
// different tier is a different quote.
const (
	ServiceTierOperations  = "operations"
	ServiceTierFullService = "full_service"
)

var ValidServiceTiers = []string{ServiceTierOperations, ServiceTierFullService}

// Protected pass-through categories. Under FIN-002 the percentage-fee base
// MUST exclude taxes, refundable deposits, and pass-through cleaning unless the
// owner contract explicitly states otherwise, so these are excluded by default
// and only an explicit opt-in includes them in the fee base.
const (
	PassThroughCategoryTaxes              = "taxes"
	PassThroughCategoryRefundableDeposits = "refundable_deposits"
	PassThroughCategoryCleaning           = "pass_through_cleaning"
)

// ProtectedPassThroughCategories is the frozen default exclusion set for the
// percentage-fee base. An owner contract may explicitly include a category via
// QuoteInputs.IncludedPassThroughs.
var ProtectedPassThroughCategories = []string{
	PassThroughCategoryTaxes,
	PassThroughCategoryRefundableDeposits,
	PassThroughCategoryCleaning,
}

// PassThroughAmount is one reimbursed or on-behalf amount charged within the
// revenue period. Pass-through categories that are protected by default are
// excluded from the percentage-fee base unless the contract opts in.
type PassThroughAmount struct {
	Category   string `json:"category"`
	MinorUnits int64  `json:"minor_units"`
}

// QuoteInputs captures every input the deterministic quote engine consumes.
// The source period and rule version are inputs too, so the same revenue on a
// different period or under a changed rule is a different quote.
type QuoteInputs struct {
	TenantID                       string              `json:"tenant_id"`
	PropertyID                     string              `json:"property_id"`
	ServiceTier                    string              `json:"service_tier"`
	ManagedUnits                   int                 `json:"managed_units"`
	Currency                       string              `json:"currency"`
	RevenuePeriod                  string              `json:"revenue_period"`
	AccommodationRevenueMinorUnits int64               `json:"accommodation_revenue_minor_units"`
	PassThroughs                   []PassThroughAmount `json:"pass_throughs,omitempty"`
	IncludedPassThroughs           []string            `json:"included_pass_throughs,omitempty"`
}

// FeeRule is one versioned commercial rule. Every percentage, minimum, reserve
// and markup is a versioned contract rule and the application ships with no
// commercial rate selected by default. Money is integer minor units plus ISO
// 4217 currency.
type FeeRule struct {
	ID                          string    `json:"id"`
	Version                     string    `json:"version"`
	Currency                    string    `json:"currency"`
	ServiceTier                 string    `json:"service_tier"`
	PercentageBasisPoints       int64     `json:"percentage_basis_points"`
	MinimumMonthlyFeeMinorUnits int64     `json:"minimum_monthly_fee_minor_units"`
	SetupFeeMinorUnits          int64     `json:"setup_fee_minor_units"`
	EffectiveFrom               string    `json:"effective_from,omitempty"`
	EffectiveTo                 string    `json:"effective_to,omitempty"`
	CreatedAt                   time.Time `json:"created_at,omitempty"`
}

// FeeBaseExclusion records one protected pass-through category removed from
// the fee base for a quote.
type FeeBaseExclusion struct {
	Category   string `json:"category"`
	MinorUnits int64  `json:"minor_units"`
}

// FeeBase is the derived management-fee base. Protected pass-throughs are
// excluded by default so Comfort Curators never earns commission on reimbursed
// costs.
type FeeBase struct {
	AccommodationRevenueMinorUnits int64              `json:"accommodation_revenue_minor_units"`
	ExcludedMinorUnits             int64              `json:"excluded_minor_units"`
	Exclusions                     []FeeBaseExclusion `json:"exclusions,omitempty"`
	BaseMinorUnits                 int64              `json:"base_minor_units"`
}

// QuoteLineItem is one deterministic quote line.
type QuoteLineItem struct {
	Code       string `json:"code"`
	Label      string `json:"label"`
	MinorUnits int64  `json:"minor_units"`
	Recurring  bool   `json:"recurring,omitempty"`
}

// Quote is the deterministic quote result. Identical inputs and rule version
// always produce an identical quote, and the input hash makes the input set
// auditable.
type Quote struct {
	InputHash                   string          `json:"input_hash"`
	RuleVersion                 string          `json:"rule_version"`
	Currency                    string          `json:"currency"`
	ServiceTier                 string          `json:"service_tier"`
	ManagedUnits                int             `json:"managed_units"`
	FeeBase                     FeeBase         `json:"fee_base"`
	AppliedBasisPoints          int64           `json:"applied_basis_points"`
	ManagementFeeMinorUnits     int64           `json:"management_fee_minor_units"`
	MinimumMonthlyFeeMinorUnits int64           `json:"minimum_monthly_fee_minor_units"`
	SetupFeeMinorUnits          int64           `json:"setup_fee_minor_units"`
	EstimatedMonthlyMinorUnits  int64           `json:"estimated_monthly_minor_units"`
	LineItems                   []QuoteLineItem `json:"line_items"`
}

// AgreementStatus is the lifecycle of a service agreement.
type AgreementStatus string

const (
	AgreementStatusDraft    AgreementStatus = "draft"
	AgreementStatusAccepted AgreementStatus = "accepted"
)

// Agreement is the versioned service agreement aggregate for one tenant and
// property. Versions are append-only and immutable; once the agreement is
// accepted it is terminal and cannot mutate.
type Agreement struct {
	ID             string              `json:"id"`
	TenantID       string              `json:"tenant_id"`
	PropertyID     string              `json:"property_id"`
	Status         AgreementStatus     `json:"status"`
	CurrentVersion int                 `json:"current_version"`
	Versions       []AgreementVersion  `json:"versions,omitempty"`
	Acceptance     *ContractAcceptance `json:"acceptance,omitempty"`
	Version        int                 `json:"version"`
	CreatedAt      time.Time           `json:"created_at"`
	UpdatedAt      time.Time           `json:"updated_at"`
}

// AgreementVersion is one immutable version of the agreement terms. The
// content hash is computed from the canonical terms and the record cannot be
// updated or deleted after it is persisted.
type AgreementVersion struct {
	ID            string    `json:"id"`
	AgreementID   string    `json:"agreement_id"`
	TenantID      string    `json:"tenant_id"`
	VersionNumber int       `json:"version_number"`
	ContentHash   string    `json:"content_hash"`
	Terms         []byte    `json:"terms"`
	CreatedAt     time.Time `json:"created_at"`
}

// ContractAcceptance records that the owner accepted exactly one agreement
// version. It points to the exact content hash of the accepted version so the
// accepted terms are fixed forever.
type ContractAcceptance struct {
	ID            string    `json:"id"`
	AgreementID   string    `json:"agreement_id"`
	TenantID      string    `json:"tenant_id"`
	VersionNumber int       `json:"version_number"`
	ContentHash   string    `json:"content_hash"`
	AcceptedBy    string    `json:"accepted_by"`
	AcceptedAt    time.Time `json:"accepted_at"`
}

// CreateAgreementParams carries the identity and first-version terms required
// to open a service agreement.
type CreateAgreementParams struct {
	TenantID   string
	PropertyID string
	Terms      []byte
}
