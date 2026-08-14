package catalog

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"
)

var (
	ErrItemNotFound                = errors.New("catalog item not found")
	ErrInvalidItem                 = errors.New("invalid catalog item")
	ErrSKUAlreadyExists            = errors.New("catalog SKU already exists in tenant")
	ErrInvalidCurrency             = errors.New("invalid ISO 4217 currency")
	ErrInvalidLabel                = errors.New("invalid operational label")
	ErrSponsoredDisabled           = errors.New("sponsored catalog placement is disabled")
	ErrClaimEvidenceRequired       = errors.New("claim evidence is required and must be retained")
	ErrInvalidClaimType            = errors.New("invalid claim type")
	ErrClaimNotFound               = errors.New("claim evidence not found")
	ErrTemplateNotFound            = errors.New("package template not found")
	ErrInvalidTemplate             = errors.New("invalid package template")
	ErrPackageVersionNotFound      = errors.New("property package version not found")
	ErrInvalidPackageVersion       = errors.New("invalid property package version")
	ErrPackageVersionItemNotFound  = errors.New("package version references an unknown catalog item")
	ErrPackageItemDisabled         = errors.New("package version references a disabled catalog item")
	ErrDuplicatePackageSKU         = errors.New("package version contains a duplicate SKU")
	ErrInvalidSubstitutionPolicy   = errors.New("invalid substitution policy")
	ErrPackageVersionNotDraft      = errors.New("only a draft package version can be approved or rejected")
	ErrPackageVersionAlreadyActive = errors.New("package version is already active")
	ErrEffectiveDateRequired       = errors.New("package version requires an effective date")
	ErrCrossTenantDenied           = errors.New("cross-tenant catalog access denied")
	ErrUnauthorized                = errors.New("unauthorized catalog action")
	ErrNoPackageItems              = errors.New("package version requires at least one item or bundle")
)

// ResourceAuthorizer is implemented by the tenancy module. It denies access
// before a property's existence or any detail is disclosed to the caller.
type ResourceAuthorizer interface {
	RequireResourceAccess(ctx context.Context, resourceTenantID, resourceType, resourceID string) error
}

const (
	ItemStatusActive   = "active"
	ItemStatusDisabled = "disabled"
)

// Operational labels (CAT-002). Sponsored placement remains disabled, so the
// sponsored label is never assignable (CAT-003: it cannot be concealed either,
// which is why it is rejected outright rather than hidden).
const (
	LabelCuratorsStandard = "curators_standard"
	LabelOwnerPreferred   = "owner_preferred"
	LabelAlternative      = "alternative"
	LabelSponsored        = "sponsored"
)

var validLabels = map[string]bool{
	LabelCuratorsStandard: true,
	LabelOwnerPreferred:   true,
	LabelAlternative:      true,
}

func ValidLabel(label string) bool {
	return validLabels[label]
}

// Claim types whose statements require retained source evidence (CAT-010).
const (
	ClaimTypeQuality        = "quality"
	ClaimTypeSustainability = "sustainability"
	ClaimTypePerformance    = "performance"
	ClaimTypeOrigin         = "origin"
)

var validClaimTypes = map[string]bool{
	ClaimTypeQuality:        true,
	ClaimTypeSustainability: true,
	ClaimTypePerformance:    true,
	ClaimTypeOrigin:         true,
}

func ValidClaimType(t string) bool {
	return validClaimTypes[t]
}

// Substitution policies for a property package version (CAT-005, CAT-007).
const (
	SubstitutionOwnerApproval = "owner_approval"
	SubstitutionAutomatic     = "automatic"
	SubstitutionRestricted    = "restricted"
)

var validSubstitutionPolicies = map[string]bool{
	SubstitutionOwnerApproval: true,
	SubstitutionAutomatic:     true,
	SubstitutionRestricted:    true,
}

func ValidSubstitutionPolicy(p string) bool {
	return validSubstitutionPolicies[p]
}

const (
	PackageStatusDraft      = "draft"
	PackageStatusActive     = "active"
	PackageStatusSuperseded = "superseded"
	PackageStatusRejected   = "rejected"
)

var currencyRe = regexp.MustCompile(`^[A-Z]{3}$`)

// ValidCurrency reports whether c is a well-formed ISO 4217 alphabetic code.
func ValidCurrency(c string) bool {
	return currencyRe.MatchString(c)
}

type CatalogItem struct {
	ID                     string    `json:"id"`
	TenantID               string    `json:"tenant_id"`
	SKU                    string    `json:"sku"`
	Name                   string    `json:"name"`
	Category               string    `json:"category"`
	Brand                  string    `json:"brand"`
	PackSize               string    `json:"pack_size"`
	UnitCostMinorUnits     int64     `json:"unit_cost_minor_units"`
	UnitCostCurrency       string    `json:"unit_cost_currency"`
	OwnerPriceMinorUnits   int64     `json:"owner_price_minor_units"`
	OwnerPriceCurrency     string    `json:"owner_price_currency"`
	TaxClass               string    `json:"tax_class"`
	Supplier               string    `json:"supplier"`
	CountryOfOrigin        string    `json:"country_of_origin"`
	Status                 string    `json:"status"`
	ShelfLifeRule          string    `json:"shelf_life_rule"`
	SubstitutionGroup      string    `json:"substitution_group"`
	OperationalSuitability string    `json:"operational_suitability"`
	Label                  string    `json:"label"`
	Version                int       `json:"version"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

// IsOperationalLabelVisible is true for every assignable label. Sponsored is
// never produced by the system, so it can never be concealed (CAT-003).
func (i *CatalogItem) IsOperationalLabelVisible() bool {
	return ValidLabel(i.Label)
}

// ClaimEvidence is a retained, append-only record of the source evidence behind
// a quality, sustainability, performance, or origin claim (CAT-010). Records
// are never updated or deleted.
type ClaimEvidence struct {
	ID                 string    `json:"id"`
	TenantID           string    `json:"tenant_id"`
	CatalogItemID      string    `json:"catalog_item_id"`
	ClaimType          string    `json:"claim_type"`
	ClaimStatement     string    `json:"claim_statement"`
	EvidenceRef        string    `json:"evidence_ref"`
	EvidenceRetainedAt time.Time `json:"evidence_retained_at"`
	CreatedAt          time.Time `json:"created_at"`
}

type PackageTemplateItem struct {
	CatalogItemID string `json:"catalog_item_id"`
	Quantity      int    `json:"quantity"`
}

type PackageTemplate struct {
	ID          string                `json:"id"`
	TenantID    string                `json:"tenant_id"`
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Status      string                `json:"status"`
	Items       []PackageTemplateItem `json:"items"`
	Version     int                   `json:"version"`
	CreatedAt   time.Time             `json:"created_at"`
	UpdatedAt   time.Time             `json:"updated_at"`
}

// PropertyPackageItem is one line of a property package version. Costs are
// derived deterministically from the catalog item's owner price at version
// creation time and are retained with the version.
type PropertyPackageItem struct {
	ID                         string    `json:"id"`
	TenantID                   string    `json:"tenant_id"`
	PackageVersionID           string    `json:"package_version_id"`
	CatalogItemID              string    `json:"catalog_item_id"`
	SKU                        string    `json:"sku"`
	Name                       string    `json:"name"`
	Label                      string    `json:"label"`
	SubstitutionGroup          string    `json:"substitution_group"`
	Quantity                   int       `json:"quantity"`
	OrderIndex                 int       `json:"order_index"`
	ExpectedMonthlyConsumption int       `json:"expected_monthly_consumption"`
	SetupCostMinorUnits        int64     `json:"setup_cost_minor_units"`
	MonthlyCostMinorUnits      int64     `json:"monthly_cost_minor_units"`
	CreatedAt                  time.Time `json:"created_at"`
}

// PropertyPackageVersion is a versioned snapshot of a property's package. Every
// change creates a new version with its own effective date; prior versions are
// retained and never deleted (CAT-008). A version is created as a draft with a
// computed review summary and can only be activated from that draft state, so
// the owner always sees cost and substitution before activation (CAT-005,
// CAT-009).
type PropertyPackageVersion struct {
	ID                              string                `json:"id"`
	TenantID                        string                `json:"tenant_id"`
	PropertyID                      string                `json:"property_id"`
	VersionNumber                   int                   `json:"version_number"`
	Status                          string                `json:"status"`
	EffectiveDate                   time.Time             `json:"effective_date"`
	MonthlyBudgetLimitMinorUnits    *int64                `json:"monthly_budget_limit_minor_units,omitempty"`
	SubstitutionPolicy              string                `json:"substitution_policy"`
	RequireApprovalForPriceIncrease bool                  `json:"require_approval_for_price_increase"`
	RequireApprovalForNewSKU        bool                  `json:"require_approval_for_new_sku"`
	SetupCostMinorUnits             int64                 `json:"setup_cost_minor_units"`
	MonthlyCostMinorUnits           int64                 `json:"monthly_cost_minor_units"`
	MonthlyConsumptionUnits         int64                 `json:"monthly_consumption_units"`
	Currency                        string                `json:"currency"`
	ReviewSummary                   ReviewSummary         `json:"review_summary"`
	CreatedBy                       string                `json:"created_by"`
	ActivatedAt                     *time.Time            `json:"activated_at,omitempty"`
	Version                         int                   `json:"version"`
	CreatedAt                       time.Time             `json:"created_at"`
	UpdatedAt                       time.Time             `json:"updated_at"`
	Items                           []PropertyPackageItem `json:"items"`
}

type ReviewItem struct {
	CatalogItemID              string `json:"catalog_item_id"`
	SKU                        string `json:"sku"`
	Name                       string `json:"name"`
	Label                      string `json:"label"`
	SubstitutionGroup          string `json:"substitution_group"`
	Quantity                   int    `json:"quantity"`
	ExpectedMonthlyConsumption int    `json:"expected_monthly_consumption"`
	SetupCostMinorUnits        int64  `json:"setup_cost_minor_units"`
	MonthlyCostMinorUnits      int64  `json:"monthly_cost_minor_units"`
}

// ReviewSummary is the deterministic cost and substitution disclosure shown to
// the owner before a package version can be activated (CAT-005, CAT-009). It is
// computed at version creation and retained with the version so activation can
// never happen without a prior review summary.
type ReviewSummary struct {
	SetupCostMinorUnits           int64        `json:"setup_cost_minor_units"`
	MonthlyCostMinorUnits         int64        `json:"monthly_cost_minor_units"`
	MonthlyConsumptionUnits       int64        `json:"monthly_consumption_units"`
	Currency                      string       `json:"currency"`
	SubstitutionPolicy            string       `json:"substitution_policy"`
	SubstitutionBehavior          string       `json:"substitution_behavior"`
	SubstitutionApprovalRequired  bool         `json:"substitution_approval_required"`
	PriceIncreaseRequiresApproval bool         `json:"price_increase_requires_approval"`
	NewSKURequiresApproval        bool         `json:"new_sku_requires_approval"`
	MonthlyBudgetLimitMinorUnits  *int64       `json:"monthly_budget_limit_minor_units,omitempty"`
	Items                         []ReviewItem `json:"items"`
}

func substitutionBehavior(policy string) (string, bool) {
	switch policy {
	case SubstitutionAutomatic:
		return "Automatic substitution within the same substitution group is permitted; any substitution outside the group requires owner approval.", false
	case SubstitutionRestricted:
		return "No substitutions are permitted for this package.", true
	case SubstitutionOwnerApproval:
		return "Any substitution requires owner approval before it can be used.", true
	default:
		return "", true
	}
}

// BuildReviewSummary computes the estimated one-time setup cost, estimated
// monthly consumption, estimated monthly cost, and substitution behavior for a
// package version from its line items. It is pure so the disclosure logic is
// unit-testable without a database. Money is integer minor units throughout.
func BuildReviewSummary(policy string, budget *int64, requirePriceIncrease, requireNewSKU bool, items []ReviewItem, currency string) (ReviewSummary, error) {
	if !ValidSubstitutionPolicy(policy) {
		return ReviewSummary{}, fmt.Errorf("%w: %q", ErrInvalidSubstitutionPolicy, policy)
	}
	if budget != nil && *budget < 0 {
		return ReviewSummary{}, fmt.Errorf("%w: monthly budget must not be negative", ErrInvalidPackageVersion)
	}
	if !ValidCurrency(currency) {
		return ReviewSummary{}, fmt.Errorf("%w: %q", ErrInvalidCurrency, currency)
	}

	behavior, approvalRequired := substitutionBehavior(policy)

	summary := ReviewSummary{
		Currency:                      currency,
		SubstitutionPolicy:            policy,
		SubstitutionBehavior:          behavior,
		SubstitutionApprovalRequired:  approvalRequired,
		PriceIncreaseRequiresApproval: requirePriceIncrease,
		NewSKURequiresApproval:        requireNewSKU,
		MonthlyBudgetLimitMinorUnits:  budget,
		Items:                         make([]ReviewItem, len(items)),
	}

	for i, it := range items {
		summary.SetupCostMinorUnits += it.SetupCostMinorUnits
		summary.MonthlyCostMinorUnits += it.MonthlyCostMinorUnits
		summary.MonthlyConsumptionUnits += int64(it.ExpectedMonthlyConsumption)
		summary.Items[i] = it
	}

	return summary, nil
}

type CreateItemParams struct {
	SKU                    string
	Name                   string
	Category               string
	Brand                  string
	PackSize               string
	UnitCostMinorUnits     int64
	UnitCostCurrency       string
	OwnerPriceMinorUnits   int64
	OwnerPriceCurrency     string
	TaxClass               string
	Supplier               string
	CountryOfOrigin        string
	Status                 string
	ShelfLifeRule          string
	SubstitutionGroup      string
	OperationalSuitability string
	Label                  string
}

type ClaimEvidenceParams struct {
	ClaimType      string
	ClaimStatement string
	EvidenceRef    string
}

type CreateTemplateParams struct {
	Name        string
	Description string
	Items       []PackageTemplateItem
}

type PackageItemInput struct {
	CatalogItemID              string
	Quantity                   int
	ExpectedMonthlyConsumption int
	OrderIndex                 int
}

type PackageBundleInput struct {
	PackageTemplateID string
	OrderIndex        int
}

type CreatePackageVersionParams struct {
	EffectiveDate                   time.Time
	MonthlyBudgetLimitMinorUnits    *int64
	SubstitutionPolicy              string
	RequireApprovalForPriceIncrease bool
	RequireApprovalForNewSKU        bool
	Items                           []PackageItemInput
	Bundles                         []PackageBundleInput
}
