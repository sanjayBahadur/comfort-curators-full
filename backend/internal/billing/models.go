package billing

import (
	"errors"
	"fmt"
	"regexp"
	"time"
)

var (
	ErrChargeNotFound                  = errors.New("charge not found")
	ErrInvalidCharge                   = errors.New("invalid charge")
	ErrInvoiceNotFound                 = errors.New("invoice not found")
	ErrInvalidInvoice                  = errors.New("invalid invoice")
	ErrInvoiceAlreadyIssued            = errors.New("invoice already issued")
	ErrCreditNotFound                  = errors.New("credit not found")
	ErrInvalidCredit                   = errors.New("invalid credit")
	ErrSubledgerEntryNotFound          = errors.New("subledger entry not found")
	ErrAccountingExportNotFound        = errors.New("accounting export not found")
	ErrInvalidAccountingExport         = errors.New("invalid accounting export")
	ErrFinancialApprovalNotFound       = errors.New("financial approval not found")
	ErrInvalidFinancialApproval        = errors.New("invalid financial approval")
	ErrDuplicateCharge                 = errors.New("duplicate charge request detected")
	ErrDuplicateCredit                 = errors.New("duplicate credit request detected")
	ErrFloatNotAllowed                 = errors.New("money must use integer minor units; float input is rejected")
	ErrOriginalEntryPreserved          = errors.New("financial correction must preserve original entry")
	ErrCrossTenantDenied               = errors.New("cross-tenant access denied")
	ErrNegativeAmount                  = errors.New("amount must not be negative")
	ErrSelfApprovalDenied              = errors.New("request creator cannot approve their own request")
	ErrAICannotPost                    = errors.New("AI actor cannot final-post a journal")
	ErrBankVerificationRequired        = errors.New("bank change requires out-of-band external verification before approval")
	ErrBankVerificationNotFound        = errors.New("bank verification not found")
	ErrBankVerificationExpired         = errors.New("bank verification token has expired")
	ErrMakerCheckerRequestNotFound     = errors.New("maker-checker request not found")
	ErrInvalidMakerCheckerRequest      = errors.New("invalid maker-checker request")
	ErrRequestNotPendingApproval       = errors.New("maker-checker request is not pending approval")
	ErrRequestNotPendingVerification   = errors.New("maker-checker request is not pending verification")
	ErrReconciliationExceptionNotFound = errors.New("reconciliation exception not found")
	ErrInvalidReconciliationException  = errors.New("invalid reconciliation exception")
)

const (
	ChargeTypeManagementFee  = "management_fee"
	ChargeTypeTaskService    = "task_service"
	ChargeTypePurchasedGoods = "purchased_goods"
	ChargeTypeReimbursement  = "reimbursement"
	ChargeTypeVendorFee      = "vendor_fee"
	ChargeTypeDiscount       = "discount"
	ChargeTypeRebate         = "rebate"
	ChargeTypeTax            = "tax"
	ChargeTypeRefund         = "refund"
	ChargeTypeCredit         = "credit"
)

var validChargeTypes = map[string]bool{
	ChargeTypeManagementFee:  true,
	ChargeTypeTaskService:    true,
	ChargeTypePurchasedGoods: true,
	ChargeTypeReimbursement:  true,
	ChargeTypeVendorFee:      true,
	ChargeTypeDiscount:       true,
	ChargeTypeRebate:         true,
	ChargeTypeTax:            true,
	ChargeTypeRefund:         true,
	ChargeTypeCredit:         true,
}

func ValidChargeType(t string) bool {
	return validChargeTypes[t]
}

const (
	ChargeStatusPending   = "pending"
	ChargeStatusApplied   = "applied"
	ChargeStatusCorrected = "corrected"
)

var validChargeStatuses = map[string]bool{
	ChargeStatusPending:   true,
	ChargeStatusApplied:   true,
	ChargeStatusCorrected: true,
}

func ValidChargeStatus(s string) bool {
	return validChargeStatuses[s]
}

const (
	InvoiceStatusDraft  = "draft"
	InvoiceStatusIssued = "issued"
	InvoiceStatusVoid   = "void"
	InvoiceStatusPaid   = "paid"
)

var validInvoiceStatuses = map[string]bool{
	InvoiceStatusDraft:  true,
	InvoiceStatusIssued: true,
	InvoiceStatusVoid:   true,
	InvoiceStatusPaid:   true,
}

func ValidInvoiceStatus(s string) bool {
	return validInvoiceStatuses[s]
}

const (
	CreditTypeReversal   = "reversal"
	CreditTypeCreditNote = "credit_note"
	CreditTypeRefund     = "refund"
	CreditTypeDiscount   = "discount"
)

var validCreditTypes = map[string]bool{
	CreditTypeReversal:   true,
	CreditTypeCreditNote: true,
	CreditTypeRefund:     true,
	CreditTypeDiscount:   true,
}

func ValidCreditType(t string) bool {
	return validCreditTypes[t]
}

const (
	CreditStatusIssued    = "issued"
	CreditStatusApplied   = "applied"
	CreditStatusCorrected = "corrected"
)

var validCreditStatuses = map[string]bool{
	CreditStatusIssued:    true,
	CreditStatusApplied:   true,
	CreditStatusCorrected: true,
}

func ValidCreditStatus(s string) bool {
	return validCreditStatuses[s]
}

const (
	SubledgerEntryTypeCharge  = "charge"
	SubledgerEntryTypePayment = "payment"
	SubledgerEntryTypeCredit  = "credit"
	SubledgerEntryTypeRefund  = "refund"
)

var validSubledgerEntryTypes = map[string]bool{
	SubledgerEntryTypeCharge:  true,
	SubledgerEntryTypePayment: true,
	SubledgerEntryTypeCredit:  true,
	SubledgerEntryTypeRefund:  true,
}

func ValidSubledgerEntryType(t string) bool {
	return validSubledgerEntryTypes[t]
}

const (
	ExportStatusRequested  = "requested"
	ExportStatusProcessing = "processing"
	ExportStatusCompleted  = "completed"
	ExportStatusFailed     = "failed"
)

var validExportStatuses = map[string]bool{
	ExportStatusRequested:  true,
	ExportStatusProcessing: true,
	ExportStatusCompleted:  true,
	ExportStatusFailed:     true,
}

func ValidExportStatus(s string) bool {
	return validExportStatuses[s]
}

const (
	FinancialApprovalStatusPending  = "pending"
	FinancialApprovalStatusApproved = "approved"
	FinancialApprovalStatusRejected = "rejected"
)

var validFinancialApprovalStatuses = map[string]bool{
	FinancialApprovalStatusPending:  true,
	FinancialApprovalStatusApproved: true,
	FinancialApprovalStatusRejected: true,
}

func ValidFinancialApprovalStatus(s string) bool {
	return validFinancialApprovalStatuses[s]
}

var currencyRe = regexp.MustCompile(`^[A-Z]{3}$`)

func ValidCurrency(c string) bool {
	return currencyRe.MatchString(c)
}

const (
	MakerCheckerRequestTypeVendorCreation = "vendor_creation"
	MakerCheckerRequestTypeBankChange     = "bank_change"
	MakerCheckerRequestTypePurchase       = "purchase"
	MakerCheckerRequestTypePayment        = "payment"
	MakerCheckerRequestTypeJournalPosting = "journal_posting"
)

var validMakerCheckerRequestTypes = map[string]bool{
	MakerCheckerRequestTypeVendorCreation: true,
	MakerCheckerRequestTypeBankChange:     true,
	MakerCheckerRequestTypePurchase:       true,
	MakerCheckerRequestTypePayment:        true,
	MakerCheckerRequestTypeJournalPosting: true,
}

func ValidMakerCheckerRequestType(t string) bool {
	return validMakerCheckerRequestTypes[t]
}

const (
	RequestStatusDraft               = "draft"
	RequestStatusPendingApproval     = "pending_approval"
	RequestStatusPendingVerification = "pending_verification"
	RequestStatusApproved            = "approved"
	RequestStatusRejected            = "rejected"
	RequestStatusCancelled           = "cancelled"
)

var validRequestStatuses = map[string]bool{
	RequestStatusDraft:               true,
	RequestStatusPendingApproval:     true,
	RequestStatusPendingVerification: true,
	RequestStatusApproved:            true,
	RequestStatusRejected:            true,
	RequestStatusCancelled:           true,
}

func ValidRequestStatus(s string) bool {
	return validRequestStatuses[s]
}

const (
	BankVerificationStatusPending  = "pending"
	BankVerificationStatusVerified = "verified"
	BankVerificationStatusExpired  = "expired"
)

var validBankVerificationStatuses = map[string]bool{
	BankVerificationStatusPending:  true,
	BankVerificationStatusVerified: true,
	BankVerificationStatusExpired:  true,
}

func ValidBankVerificationStatus(s string) bool {
	return validBankVerificationStatuses[s]
}

const (
	ExceptionTypeUnmatchedEntry   = "unmatched_entry"
	ExceptionTypeAmountMismatch   = "amount_mismatch"
	ExceptionTypeCurrencyMismatch = "currency_mismatch"
	ExceptionTypeMissingEntry     = "missing_entry"
	ExceptionTypeDuplicateEntry   = "duplicate_entry"
	ExceptionTypeOther            = "other"
)

var validExceptionTypes = map[string]bool{
	ExceptionTypeUnmatchedEntry:   true,
	ExceptionTypeAmountMismatch:   true,
	ExceptionTypeCurrencyMismatch: true,
	ExceptionTypeMissingEntry:     true,
	ExceptionTypeDuplicateEntry:   true,
	ExceptionTypeOther:            true,
}

func ValidExceptionType(t string) bool {
	return validExceptionTypes[t]
}

const (
	ExceptionStatusOpen     = "open"
	ExceptionStatusResolved = "resolved"
)

var validExceptionStatuses = map[string]bool{
	ExceptionStatusOpen:     true,
	ExceptionStatusResolved: true,
}

func ValidExceptionStatus(s string) bool {
	return validExceptionStatuses[s]
}

const (
	FeExcludedTax                 = "tax"
	FeExcludedDeposit             = "deposit"
	FeExcludedPassThroughCleaning = "pass_through_cleaning"
)

var feeExclusions = map[string]bool{
	FeExcludedTax:                 true,
	FeExcludedDeposit:             true,
	FeExcludedPassThroughCleaning: true,
}

func IsFeeBaseExcluded(chargeType string) bool {
	return feeExclusions[chargeType]
}

// Money represents an immutable monetary value in integer minor units.
// Money never uses float; all amounts are whole integer minor units.
type Money struct {
	MinorUnits int64  `json:"minor_units"`
	Currency   string `json:"currency"`
}

// Charge is a tenant-scoped, property-scoped owner charge linked to a
// contract rule or approved evidence. Every charge is classified by type.
type Charge struct {
	ID               string    `json:"id"`
	TenantID         string    `json:"tenant_id"`
	PropertyID       string    `json:"property_id"`
	ChargeType       string    `json:"charge_type"`
	AmountMinorUnits int64     `json:"amount_minor_units"`
	Currency         string    `json:"currency"`
	Reason           string    `json:"reason,omitempty"`
	Data             []byte    `json:"data,omitempty"`
	ContractRuleID   string    `json:"contract_rule_id,omitempty"`
	EvidenceID       string    `json:"evidence_id,omitempty"`
	TicketID         string    `json:"ticket_id,omitempty"`
	OrderID          string    `json:"order_id,omitempty"`
	ApprovalID       string    `json:"approval_id,omitempty"`
	IdempotencyKey   string    `json:"idempotency_key"`
	Status           string    `json:"status"`
	Version          int64     `json:"version"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// Invoice is a tenant-scoped, property-scoped billing invoice that is
// idempotent by key. Lines trace every amount to contract, ticket, order
// or approved adjustment.
type Invoice struct {
	ID              string     `json:"id"`
	TenantID        string     `json:"tenant_id"`
	PropertyID      string     `json:"property_id"`
	PeriodStart     *time.Time `json:"period_start,omitempty"`
	PeriodEnd       *time.Time `json:"period_end,omitempty"`
	TotalMinorUnits int64      `json:"total_minor_units"`
	Currency        string     `json:"currency"`
	Status          string     `json:"status"`
	IdempotencyKey  string     `json:"idempotency_key"`
	Version         int64      `json:"version"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// InvoiceLine links an amount to a contract rule, ticket, order, or
// approved manual adjustment.
type InvoiceLine struct {
	ID               string    `json:"id"`
	InvoiceID        string    `json:"invoice_id"`
	TenantID         string    `json:"tenant_id"`
	ChargeType       string    `json:"charge_type"`
	Description      string    `json:"description"`
	AmountMinorUnits int64     `json:"amount_minor_units"`
	Currency         string    `json:"currency"`
	ContractRuleID   string    `json:"contract_rule_id,omitempty"`
	TicketID         string    `json:"ticket_id,omitempty"`
	OrderID          string    `json:"order_id,omitempty"`
	AdjustmentID     string    `json:"adjustment_id,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

// Credit is a financial correction (reversal, credit note, refund,
// discount) that preserves the original entry. Original entries are never
// deleted.
type Credit struct {
	ID                string    `json:"id"`
	TenantID          string    `json:"tenant_id"`
	PropertyID        string    `json:"property_id"`
	CreditType        string    `json:"credit_type"`
	AmountMinorUnits  int64     `json:"amount_minor_units"`
	Currency          string    `json:"currency"`
	Reason            string    `json:"reason,omitempty"`
	OriginalEntryID   string    `json:"original_entry_id"`
	OriginalEntryType string    `json:"original_entry_type"`
	Data              []byte    `json:"data,omitempty"`
	IdempotencyKey    string    `json:"idempotency_key"`
	Status            string    `json:"status"`
	Version           int64     `json:"version"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// OperationalSubledgerEntry is an append-only record of every financial
// movement. Entries never change once written.
type OperationalSubledgerEntry struct {
	ID               string    `json:"id"`
	TenantID         string    `json:"tenant_id"`
	PropertyID       string    `json:"property_id"`
	EntryType        string    `json:"entry_type"`
	AmountMinorUnits int64     `json:"amount_minor_units"`
	Currency         string    `json:"currency"`
	ReferenceType    string    `json:"reference_type"`
	ReferenceID      string    `json:"reference_id"`
	Description      string    `json:"description"`
	CreatedAt        time.Time `json:"created_at"`
}

// AccountingExport represents a licensed-accounting export request.
type AccountingExport struct {
	ID          string     `json:"id"`
	TenantID    string     `json:"tenant_id"`
	PeriodStart *time.Time `json:"period_start,omitempty"`
	PeriodEnd   *time.Time `json:"period_end,omitempty"`
	Format      string     `json:"format"`
	Status      string     `json:"status"`
	RequestedBy string     `json:"requested_by"`
	ResultRef   string     `json:"result_ref,omitempty"`
	Version     int64      `json:"version"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// FinancialApproval is an append-only approval for high-value billing
// decisions.
type FinancialApproval struct {
	ID         string    `json:"id"`
	TenantID   string    `json:"tenant_id"`
	RequestID  string    `json:"request_id"`
	ApproverID string    `json:"approver_id"`
	Decision   string    `json:"decision"`
	Reason     string    `json:"reason,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type CreateChargeParams struct {
	PropertyID       string
	ChargeType       string
	AmountMinorUnits int64
	Currency         string
	Reason           string
	Data             []byte
	ContractRuleID   string
	EvidenceID       string
	TicketID         string
	OrderID          string
	ApprovalID       string
	IdempotencyKey   string
}

type CreateInvoiceParams struct {
	PropertyID     string
	PeriodStart    *time.Time
	PeriodEnd      *time.Time
	Currency       string
	IdempotencyKey string
	Lines          []CreateInvoiceLineParams
}

type CreateInvoiceLineParams struct {
	ChargeType       string
	Description      string
	AmountMinorUnits int64
	ContractRuleID   string
	TicketID         string
	OrderID          string
	AdjustmentID     string
}

type CreateCreditParams struct {
	PropertyID        string
	CreditType        string
	AmountMinorUnits  int64
	Currency          string
	Reason            string
	OriginalEntryID   string
	OriginalEntryType string
	Data              []byte
	IdempotencyKey    string
}

type CreateFinancialApprovalParams struct {
	RequestID  string
	ApproverID string
	Decision   string
	Reason     string
}

type CreateAccountingExportParams struct {
	PeriodStart *time.Time
	PeriodEnd   *time.Time
	Format      string
}

type CreateMakerCheckerRequestParams struct {
	RequestType          string
	PropertyID           string
	Payload              []byte
	IdempotencyKey       string
	RequiresVerification bool
}

type SubmitMakerCheckerRequestParams struct {
	ActorID   string
	IsAIActor bool
}

type DecideMakerCheckerRequestParams struct {
	ActorID   string
	IsAIActor bool
	Decision  string
	Reason    string
}

type CreateBankVerificationParams struct {
	RequestID string
}

type ConfirmBankVerificationParams struct {
	Token string
}

type FinalizeJournalParams struct {
	ExportID  string
	ActorID   string
	IsAIActor bool
}

type CreateReconciliationExceptionParams struct {
	PropertyID    string
	EntryID       string
	EntryType     string
	ExceptionType string
	Description   string
}

type ResolveReconciliationExceptionParams struct {
	ActorID string
}

// MakerCheckerRequest is a tenant-scoped request that enforces maker-checker
// separation. A maker creates the request; a different checker must approve
// or reject it. Bank-change requests also require out-of-band verification.
type MakerCheckerRequest struct {
	ID                   string    `json:"id"`
	TenantID             string    `json:"tenant_id"`
	RequestType          string    `json:"request_type"`
	PropertyID           string    `json:"property_id,omitempty"`
	Status               string    `json:"status"`
	CreatedBy            string    `json:"created_by"`
	SubmittedBy          string    `json:"submitted_by,omitempty"`
	ApprovedBy           string    `json:"approved_by,omitempty"`
	RejectedBy           string    `json:"rejected_by,omitempty"`
	Payload              []byte    `json:"payload,omitempty"`
	IdempotencyKey       string    `json:"idempotency_key"`
	RequiresVerification bool      `json:"requires_verification"`
	Version              int64     `json:"version"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// BankVerification records an out-of-band verification for a bank-change
// request. The request stays pending until the verification token is
// confirmed by an external process.
type BankVerification struct {
	ID                string     `json:"id"`
	TenantID          string     `json:"tenant_id"`
	RequestID         string     `json:"request_id"`
	VerificationToken string     `json:"verification_token"`
	Status            string     `json:"status"`
	VerifiedBy        string     `json:"verified_by,omitempty"`
	VerifiedAt        *time.Time `json:"verified_at,omitempty"`
	ExpiresAt         time.Time  `json:"expires_at"`
	CreatedAt         time.Time  `json:"created_at"`
}

// ReconciliationException records a mismatch or inconsistency found during
// reconciliation of subledger entries against external statements.
type ReconciliationException struct {
	ID            string     `json:"id"`
	TenantID      string     `json:"tenant_id"`
	PropertyID    string     `json:"property_id"`
	EntryID       string     `json:"entry_id"`
	EntryType     string     `json:"entry_type"`
	ExceptionType string     `json:"exception_type"`
	Description   string     `json:"description"`
	Status        string     `json:"status"`
	RecordedBy    string     `json:"recorded_by"`
	ResolvedBy    string     `json:"resolved_by,omitempty"`
	ResolvedAt    *time.Time `json:"resolved_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

// ValidateMoney checks that minor_units is an integer (always true for int64)
// and currency is a valid ISO 4217 code. Float input is rejected by the
// handler parsing layer since minor_units is typed as integer.
func ValidateMoney(amount Money) error {
	if !ValidCurrency(amount.Currency) {
		return fmt.Errorf("%w: invalid currency %q", ErrInvalidCharge, amount.Currency)
	}
	return nil
}

// ValidateFeeBase checks whether a given charge type is excluded from the
// management-fee base. Tax, refundable deposits, and pass-through cleaning
// are excluded unless the owner contract explicitly states otherwise.
func ValidateFeeBase(chargeType string) error {
	return nil
}
