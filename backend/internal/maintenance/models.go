package maintenance

import (
	"errors"
	"fmt"
	"regexp"
	"time"
)

var (
	ErrRequestNotFound               = errors.New("maintenance request not found")
	ErrInvalidRequest                = errors.New("invalid maintenance request")
	ErrRequestNotTriaged             = errors.New("maintenance request must be triaged first")
	ErrRequestNotApproved            = errors.New("maintenance request has no approved estimate")
	ErrEstimateNotFound              = errors.New("maintenance estimate not found")
	ErrInvalidEstimate               = errors.New("invalid maintenance estimate")
	ErrEstimateImmutable             = errors.New("maintenance estimate is preserved once submitted and cannot be changed")
	ErrEstimateNotPending            = errors.New("maintenance estimate is not pending approval")
	ErrEstimateNotApproved           = errors.New("unapproved estimate cannot start work")
	ErrSelfApprovalDenied            = errors.New("estimate preparer cannot approve their own estimate")
	ErrAICannotApprove               = errors.New("AI actor cannot approve a maintenance estimate")
	ErrApprovalNotFound              = errors.New("maintenance approval not found")
	ErrInvalidApproval               = errors.New("invalid maintenance approval")
	ErrWorkOrderNotFound             = errors.New("vendor work order not found")
	ErrInvalidWorkOrder              = errors.New("invalid vendor work order")
	ErrWorkOrderNotAssigned          = errors.New("vendor work order is not in assigned status")
	ErrWorkOrderNotInProgress        = errors.New("vendor work order is not in progress")
	ErrWorkOrderNotCompleted         = errors.New("vendor work order is not completed")
	ErrCompletionEvidenceRequired    = errors.New("completion evidence is required before completion")
	ErrSelfVerificationDenied        = errors.New("high-risk actor cannot self-verify their own work")
	ErrIndependentVerificationNeeded = errors.New("high-risk work requires an independent verifier")
	ErrVendorScopeDenied             = errors.New("vendor can only access work orders assigned to them")
	ErrWarrantyNotFound              = errors.New("warranty record not found")
	ErrInvalidWarranty               = errors.New("invalid warranty record")
	ErrCrossTenantDenied             = errors.New("cross-tenant access denied")
	ErrInvalidRiskLevel              = errors.New("invalid risk level")
)

const (
	RequestStatusReported         = "reported"
	RequestStatusTriaged          = "triaged"
	RequestStatusEstimateApproved = "estimate_approved"
	RequestStatusEstimateRejected = "estimate_rejected"
	RequestStatusInProgress       = "in_progress"
	RequestStatusCompleted        = "completed"
	RequestStatusCancelled        = "cancelled"
)

var validRequestStatuses = map[string]bool{
	RequestStatusReported:         true,
	RequestStatusTriaged:          true,
	RequestStatusEstimateApproved: true,
	RequestStatusEstimateRejected: true,
	RequestStatusInProgress:       true,
	RequestStatusCompleted:        true,
	RequestStatusCancelled:        true,
}

func ValidRequestStatus(s string) bool {
	return validRequestStatuses[s]
}

const (
	CategoryRoutine    = "routine_maintenance"
	CategorySpecialist = "specialist_vendor_request"
	CategoryIncident   = "incident"
	CategorySafety     = "safety"
)

var validCategories = map[string]bool{
	CategoryRoutine:    true,
	CategorySpecialist: true,
	CategoryIncident:   true,
	CategorySafety:     true,
}

func ValidCategory(c string) bool {
	return validCategories[c]
}

const (
	PriorityLow    = "low"
	PriorityNormal = "normal"
	PriorityHigh   = "high"
	PriorityUrgent = "urgent"
)

var validPriorities = map[string]bool{
	PriorityLow:    true,
	PriorityNormal: true,
	PriorityHigh:   true,
	PriorityUrgent: true,
}

func ValidPriority(p string) bool {
	return validPriorities[p]
}

const (
	RiskLevelStandard = "standard"
	RiskLevelHigh     = "high"
)

var validRiskLevels = map[string]bool{
	RiskLevelStandard: true,
	RiskLevelHigh:     true,
}

func ValidRiskLevel(r string) bool {
	return validRiskLevels[r]
}

const (
	EstimateStatusDraft           = "draft"
	EstimateStatusPendingApproval = "pending_approval"
	EstimateStatusApproved        = "approved"
	EstimateStatusRejected        = "rejected"
)

var validEstimateStatuses = map[string]bool{
	EstimateStatusDraft:           true,
	EstimateStatusPendingApproval: true,
	EstimateStatusApproved:        true,
	EstimateStatusRejected:        true,
}

func ValidEstimateStatus(s string) bool {
	return validEstimateStatuses[s]
}

const (
	ApprovalDecisionApproved = "approved"
	ApprovalDecisionRejected = "rejected"
)

var validApprovalDecisions = map[string]bool{
	ApprovalDecisionApproved: true,
	ApprovalDecisionRejected: true,
}

func ValidApprovalDecision(d string) bool {
	return validApprovalDecisions[d]
}

const (
	WorkOrderStatusAssigned   = "assigned"
	WorkOrderStatusInProgress = "in_progress"
	WorkOrderStatusCompleted  = "completed"
	WorkOrderStatusVerified   = "verified"
	WorkOrderStatusClosed     = "closed"
)

var validWorkOrderStatuses = map[string]bool{
	WorkOrderStatusAssigned:   true,
	WorkOrderStatusInProgress: true,
	WorkOrderStatusCompleted:  true,
	WorkOrderStatusVerified:   true,
	WorkOrderStatusClosed:     true,
}

func ValidWorkOrderStatus(s string) bool {
	return validWorkOrderStatuses[s]
}

const (
	WarrantyStatusActive  = "active"
	WarrantyStatusExpired = "expired"
	WarrantyStatusClaimed = "claimed"
	WarrantyStatusClosed  = "closed"
)

var validWarrantyStatuses = map[string]bool{
	WarrantyStatusActive:  true,
	WarrantyStatusExpired: true,
	WarrantyStatusClaimed: true,
	WarrantyStatusClosed:  true,
}

func ValidWarrantyStatus(s string) bool {
	return validWarrantyStatuses[s]
}

var currencyRe = regexp.MustCompile(`^[A-Z]{3}$`)

func ValidCurrency(c string) bool {
	return currencyRe.MatchString(c)
}

// MaintenanceRequest is a tenant-scoped, property-scoped triage record that
// drives the maintenance pipeline: triage, estimate, approval and vendor work.
type MaintenanceRequest struct {
	ID         string     `json:"id"`
	TenantID   string     `json:"tenant_id"`
	PropertyID string     `json:"property_id"`
	Title      string     `json:"title"`
	Category   string     `json:"category"`
	Priority   string     `json:"priority"`
	RiskLevel  string     `json:"risk_level"`
	Status     string     `json:"status"`
	ReportedBy string     `json:"reported_by"`
	TriagedBy  string     `json:"triaged_by,omitempty"`
	TriagedAt  *time.Time `json:"triaged_at,omitempty"`
	EstimateID string     `json:"estimate_id,omitempty"`
	Notes      string     `json:"notes,omitempty"`
	Version    int64      `json:"version"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// MaintenanceEstimate is the preserved estimate for a request. Once submitted
// for approval it is immutable; only the decision may follow.
type MaintenanceEstimate struct {
	ID               string     `json:"id"`
	TenantID         string     `json:"tenant_id"`
	RequestID        string     `json:"request_id"`
	PropertyID       string     `json:"property_id"`
	PreparedBy       string     `json:"prepared_by"`
	AmountMinorUnits int64      `json:"amount_minor_units"`
	Currency         string     `json:"currency"`
	Scope            string     `json:"scope"`
	Status           string     `json:"status"`
	SubmittedAt      *time.Time `json:"submitted_at,omitempty"`
	ApprovedBy       string     `json:"approved_by,omitempty"`
	ApprovedAt       *time.Time `json:"approved_at,omitempty"`
	RejectedBy       string     `json:"rejected_by,omitempty"`
	RejectedAt       *time.Time `json:"rejected_at,omitempty"`
	Version          int64      `json:"version"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// MaintenanceApproval is an append-only approval decision on an estimate.
type MaintenanceApproval struct {
	ID         string    `json:"id"`
	TenantID   string    `json:"tenant_id"`
	RequestID  string    `json:"request_id"`
	EstimateID string    `json:"estimate_id"`
	ActorID    string    `json:"actor_id"`
	Decision   string    `json:"decision"`
	Reason     string    `json:"reason,omitempty"`
	IsAIActor  bool      `json:"is_ai_actor"`
	CreatedAt  time.Time `json:"created_at"`
}

// VendorWorkOrder is the bounded, vendor-assigned execution scope. A vendor
// sees only the scope assigned to them and never the work of another vendor.
type VendorWorkOrder struct {
	ID                    string     `json:"id"`
	TenantID              string     `json:"tenant_id"`
	RequestID             string     `json:"request_id"`
	EstimateID            string     `json:"estimate_id"`
	PropertyID            string     `json:"property_id"`
	VendorID              string     `json:"vendor_id"`
	Scope                 string     `json:"scope"`
	RiskLevel             string     `json:"risk_level"`
	Status                string     `json:"status"`
	AssignedBy            string     `json:"assigned_by"`
	AssignedAt            time.Time  `json:"assigned_at"`
	StartedAt             *time.Time `json:"started_at,omitempty"`
	CompletedBy           string     `json:"completed_by,omitempty"`
	CompletedAt           *time.Time `json:"completed_at,omitempty"`
	CompletionEvidenceRef string     `json:"completion_evidence_ref,omitempty"`
	VerifiedBy            string     `json:"verified_by,omitempty"`
	VerifiedAt            *time.Time `json:"verified_at,omitempty"`
	Version               int64      `json:"version"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

// WarrantyRecord retains the warranty history attached to verified vendor
// work. Warranty records are append-only and never hard-deleted.
type WarrantyRecord struct {
	ID          string     `json:"id"`
	TenantID    string     `json:"tenant_id"`
	WorkOrderID string     `json:"work_order_id"`
	PropertyID  string     `json:"property_id"`
	VendorID    string     `json:"vendor_id"`
	Provider    string     `json:"provider"`
	Coverage    string     `json:"coverage"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	Status      string     `json:"status"`
	RecordedBy  string     `json:"recorded_by"`
	CreatedAt   time.Time  `json:"created_at"`
}

type CreateRequestParams struct {
	PropertyID string
	Title      string
	Category   string
	Priority   string
	RiskLevel  string
	Notes      string
}

type TriageRequestParams struct {
	Category  string
	Priority  string
	RiskLevel string
	Notes     string
}

type CreateEstimateParams struct {
	AmountMinorUnits int64
	Currency         string
	Scope            string
}

type DecideEstimateParams struct {
	ActorID   string
	Decision  string
	Reason    string
	IsAIActor bool
}

type AssignVendorWorkOrderParams struct {
	VendorID string
	Scope    string
}

type CompleteWorkOrderParams struct {
	CompletedBy           string
	CompletionEvidenceRef string
}

// ValidateStartReady is the "unapproved estimate cannot start" gate. A vendor
// work order may only start when its linked estimate is approved.
func ValidateStartReady(estimateStatus string) error {
	if estimateStatus != EstimateStatusApproved {
		return fmt.Errorf("%w: estimate status is %q", ErrEstimateNotApproved, estimateStatus)
	}
	return nil
}

// ValidateVerifier is the "high-risk actor cannot self-verify" gate. Standard
// work may be verified by any distinct actor; high-risk work requires an
// independent verifier who neither performed the work nor is the assigned
// vendor.
func ValidateVerifier(riskLevel, verifierID, completedBy, vendorID string) error {
	if riskLevel != RiskLevelHigh {
		return nil
	}
	if verifierID == "" {
		return ErrIndependentVerificationNeeded
	}
	if verifierID == completedBy || verifierID == vendorID {
		return ErrSelfVerificationDenied
	}
	return nil
}

// VendorVisibleOrders returns only the work orders assigned to the given
// vendor. A vendor never sees work scoped to another vendor.
func VendorVisibleOrders(orders []VendorWorkOrder, vendorID string) []VendorWorkOrder {
	var out []VendorWorkOrder
	for _, o := range orders {
		if o.VendorID == vendorID {
			out = append(out, o)
		}
	}
	return out
}
