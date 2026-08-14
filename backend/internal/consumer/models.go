package consumer

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"
)

var (
	ErrDisclosureNotFound         = errors.New("consumer disclosure not found")
	ErrInvalidDisclosure          = errors.New("invalid consumer disclosure")
	ErrHiddenRecurringCost        = errors.New("recurring cost must be visible and disclosed")
	ErrRecurringCostNotVisible    = errors.New("recurring cost is not visible before acceptance")
	ErrAcceptanceNotFound         = errors.New("consumer acceptance not found")
	ErrInvalidAcceptance          = errors.New("invalid consumer acceptance")
	ErrNoDisclosureBeforeAccept   = errors.New("acceptance requires a prior disclosure with visible cost")
	ErrDisclosureResourceMismatch = errors.New("disclosure does not match the accepted resource")
	ErrExportNotFound             = errors.New("history export not found")
	ErrInvalidExport              = errors.New("invalid history export")
	ErrInvalidCurrency            = errors.New("invalid ISO 4217 currency")
	ErrInvalidRecurrence          = errors.New("invalid recurrence")
	ErrInvalidResourceType        = errors.New("invalid consumer resource type")
	ErrCrossTenantDenied          = errors.New("cross-tenant consumer access denied")
)

// Recurrence describes how a cost recurs. A non-one-time recurrence is a
// recurring charge, which the system requires to be explicitly disclosed
// before any acceptance (CON-001, CON-004).
const (
	RecurrenceOneTime = "one_time"
	RecurrenceWeekly  = "weekly"
	RecurrenceMonthly = "monthly"
	RecurrenceAnnual  = "annual"
)

var validRecurrences = map[string]bool{
	RecurrenceOneTime: true,
	RecurrenceWeekly:  true,
	RecurrenceMonthly: true,
	RecurrenceAnnual:  true,
}

func ValidRecurrence(r string) bool {
	return validRecurrences[r]
}

// Consumer resources that can be disclosed and accepted.
const (
	ResourceTypePackage = "package"
	ResourceTypeOrder   = "order"
	ResourceTypeService = "service"
)

var validResourceTypes = map[string]bool{
	ResourceTypePackage: true,
	ResourceTypeOrder:   true,
	ResourceTypeService: true,
}

func ValidResourceType(t string) bool {
	return validResourceTypes[t]
}

const (
	ExportStatusCompleted = "completed"
)

var currencyRe = regexp.MustCompile(`^[A-Z]{3}$`)

// ValidCurrency reports whether c is a well-formed ISO 4217 alphabetic code.
func ValidCurrency(c string) bool {
	return currencyRe.MatchString(c)
}

// Disclosure is the pre-acceptance disclosure of price, tax, recurrence,
// substitution, cancellation, refund, seller and origin (CON-001, CON-002).
// Every recurring disclosure must carry an explicit recurring cost so a
// recurring charge can never be hidden (CON-004), and the recurring cost is
// always visible on the record.
type Disclosure struct {
	ID                         string    `json:"id"`
	TenantID                   string    `json:"tenant_id"`
	PropertyID                 string    `json:"property_id,omitempty"`
	ResourceType               string    `json:"resource_type"`
	ResourceID                 string    `json:"resource_id"`
	PriceMinorUnits            int64     `json:"price_minor_units"`
	TaxMinorUnits              int64     `json:"tax_minor_units"`
	Currency                   string    `json:"currency"`
	Recurrence                 string    `json:"recurrence"`
	RecurrenceAmountMinorUnits *int64    `json:"recurring_cost_minor_units,omitempty"`
	SubstitutionPolicy         string    `json:"substitution_policy,omitempty"`
	CancellationPolicy         string    `json:"cancellation_policy,omitempty"`
	RefundPolicy               string    `json:"refund_policy,omitempty"`
	Seller                     string    `json:"seller,omitempty"`
	CountryOfOrigin            string    `json:"country_of_origin,omitempty"`
	GrievanceContact           string    `json:"grievance_contact,omitempty"`
	RecurringCostVisible       bool      `json:"recurring_cost_visible"`
	CreatedAt                  time.Time `json:"created_at"`
}

// HasRecurringCharge reports whether the disclosure carries a recurring
// (non-one-time) charge.
func (d *Disclosure) HasRecurringCharge() bool {
	return d.Recurrence != RecurrenceOneTime
}

// RecurringCost returns the disclosed recurring cost, or zero for one-time
// disclosures.
func (d *Disclosure) RecurringCost() int64 {
	if d.RecurrenceAmountMinorUnits == nil {
		return 0
	}
	return *d.RecurrenceAmountMinorUnits
}

// DisclosureIsAcceptable enforces the CON-001 invariant that a recurring cost
// is visible before acceptance. One-time disclosures only need a recorded
// disclosure; recurring disclosures additionally need the recurring amount to
// be present and visible.
func DisclosureIsAcceptable(d *Disclosure) error {
	if d == nil {
		return ErrNoDisclosureBeforeAccept
	}
	if !d.RecurringCostVisible {
		return ErrRecurringCostNotVisible
	}
	if d.HasRecurringCharge() && d.RecurrenceAmountMinorUnits == nil {
		return ErrHiddenRecurringCost
	}
	return nil
}

// Acceptance records that a resource was accepted only after its disclosure
// had a visible recurring cost.
type Acceptance struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id"`
	PropertyID   string    `json:"property_id,omitempty"`
	DisclosureID string    `json:"disclosure_id"`
	ResourceType string    `json:"resource_type"`
	ResourceID   string    `json:"resource_id"`
	AcceptedBy   string    `json:"accepted_by"`
	AcceptedAt   time.Time `json:"accepted_at"`
	CreatedAt    time.Time `json:"created_at"`
}

// HistoryExport is a tenant-scoped snapshot of order, invoice, package and
// service history (CON-006). The data is captured from tenant-scoped queries
// and the record itself is tenant-scoped, so it can never leak another
// tenant's history.
type HistoryExport struct {
	ID          string          `json:"id"`
	TenantID    string          `json:"tenant_id"`
	PropertyID  string          `json:"property_id,omitempty"`
	RequestedBy string          `json:"requested_by"`
	Status      string          `json:"status"`
	Data        json.RawMessage `json:"data"`
	CreatedAt   time.Time       `json:"created_at"`
}

// HistoryExportData is the typed body of an owner history export.
type HistoryExportData struct {
	Packages []ExportedPackage `json:"packages"`
	Invoices []ExportedInvoice `json:"invoices"`
	Orders   []ExportedOrder   `json:"orders"`
	Services []ExportedService `json:"services"`
}

type ExportedPackage struct {
	ID                    string    `json:"id"`
	PropertyID            string    `json:"property_id"`
	VersionNumber         int       `json:"version_number"`
	Status                string    `json:"status"`
	EffectiveDate         time.Time `json:"effective_date"`
	Currency              string    `json:"currency"`
	SetupCostMinorUnits   int64     `json:"setup_cost_minor_units"`
	MonthlyCostMinorUnits int64     `json:"monthly_cost_minor_units"`
	CreatedAt             time.Time `json:"created_at"`
}

type ExportedInvoice struct {
	ID              string     `json:"id"`
	PropertyID      string     `json:"property_id"`
	PeriodStart     *time.Time `json:"period_start,omitempty"`
	PeriodEnd       *time.Time `json:"period_end,omitempty"`
	TotalMinorUnits int64      `json:"total_minor_units"`
	Currency        string     `json:"currency"`
	Status          string     `json:"status"`
	CreatedAt       time.Time  `json:"created_at"`
}

type ExportedOrder struct {
	ID               string    `json:"id"`
	PropertyID       string    `json:"property_id"`
	OrderID          string    `json:"order_id"`
	ChargeType       string    `json:"charge_type"`
	AmountMinorUnits int64     `json:"amount_minor_units"`
	Currency         string    `json:"currency"`
	Status           string    `json:"status"`
	CreatedAt        time.Time `json:"created_at"`
}

type ExportedService struct {
	ID         string    `json:"id"`
	PropertyID string    `json:"property_id"`
	Type       string    `json:"type"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

type DisclosureParams struct {
	PropertyID                 string
	ResourceType               string
	ResourceID                 string
	PriceMinorUnits            int64
	TaxMinorUnits              int64
	Currency                   string
	Recurrence                 string
	RecurrenceAmountMinorUnits *int64
	SubstitutionPolicy         string
	CancellationPolicy         string
	RefundPolicy               string
	Seller                     string
	CountryOfOrigin            string
	GrievanceContact           string
}

type AcceptanceParams struct {
	PropertyID   string
	DisclosureID string
	ResourceType string
	ResourceID   string
}

// ValidateDisclosureParams enforces CON-001/CON-004 for a disclosure. A
// recurring disclosure without an explicit recurring cost is a hidden
// recurring charge and is rejected.
func ValidateDisclosureParams(p DisclosureParams) error {
	if p.ResourceType == "" || p.ResourceID == "" {
		return fmt.Errorf("%w: resource_type and resource_id are required", ErrInvalidDisclosure)
	}
	if !ValidResourceType(p.ResourceType) {
		return fmt.Errorf("%w: invalid resource_type %q", ErrInvalidResourceType, p.ResourceType)
	}
	if !ValidCurrency(p.Currency) {
		return fmt.Errorf("%w: invalid currency %q", ErrInvalidCurrency, p.Currency)
	}
	if p.PriceMinorUnits < 0 || p.TaxMinorUnits < 0 {
		return fmt.Errorf("%w: price and tax must not be negative", ErrInvalidDisclosure)
	}
	if !ValidRecurrence(p.Recurrence) {
		return fmt.Errorf("%w: invalid recurrence %q", ErrInvalidRecurrence, p.Recurrence)
	}
	if p.Recurrence != RecurrenceOneTime {
		if p.RecurrenceAmountMinorUnits == nil {
			return ErrHiddenRecurringCost
		}
		if *p.RecurrenceAmountMinorUnits < 0 {
			return fmt.Errorf("%w: recurring cost must not be negative", ErrInvalidDisclosure)
		}
	} else if p.RecurrenceAmountMinorUnits != nil && *p.RecurrenceAmountMinorUnits != 0 {
		return fmt.Errorf("%w: a one-time disclosure must not carry a recurring amount", ErrInvalidDisclosure)
	}
	return nil
}
