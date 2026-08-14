package procurement

import (
	"errors"
	"regexp"
	"time"
)

var (
	ErrSupplierNotFound         = errors.New("supplier not found")
	ErrInvalidSupplier          = errors.New("invalid supplier")
	ErrSupplierAlreadyApproved  = errors.New("supplier already approved")
	ErrSupplierAlreadyRejected  = errors.New("supplier already rejected")
	ErrSupplierItemNotFound     = errors.New("supplier item not found")
	ErrInvalidSupplierItem      = errors.New("invalid supplier item")
	ErrRequisitionNotFound      = errors.New("requisition not found")
	ErrInvalidRequisition       = errors.New("invalid requisition")
	ErrRequisitionNotDraft      = errors.New("requisition is not in draft status")
	ErrRequisitionNotPending    = errors.New("requisition is not pending approval")
	ErrApprovalNotFound         = errors.New("requisition approval not found")
	ErrAICannotApprove          = errors.New("AI actor cannot approve its own requisition")
	ErrNewSupplierRequiresHuman = errors.New("a new supplier requires human approval before purchase")
	ErrSelfApprovalDenied       = errors.New("requisition creator cannot approve their own requisition")
	ErrPriceVarianceApproval    = errors.New("price variance exceeds approved threshold and requires approval")
	ErrBudgetExceeded           = errors.New("requisition exceeds the approved budget limit")
	ErrFoodSKUDisabled          = errors.New("food SKU is disabled without recorded claim approval")
	ErrPurchaseOrderNotFound    = errors.New("purchase order not found")
	ErrInvalidPurchaseOrder     = errors.New("invalid purchase order")
	ErrGoodsReceiptNotFound     = errors.New("goods receipt not found")
	ErrInvalidGoodsReceipt      = errors.New("invalid goods receipt")
	ErrGoodsReceiptImmutable    = errors.New("goods receipt is immutable once finalised")
	ErrRebateNotFound           = errors.New("supplier rebate not found")
	ErrInvalidRebate            = errors.New("invalid supplier rebate")
	ErrRequisitionItemNotFound  = errors.New("requisition item not found")
)

const (
	SupplierStatusActive          = "active"
	SupplierStatusPendingApproval = "pending_approval"
	SupplierStatusDisabled        = "disabled"
)

var validSupplierStatuses = map[string]bool{
	SupplierStatusActive:          true,
	SupplierStatusPendingApproval: true,
	SupplierStatusDisabled:        true,
}

func ValidSupplierStatus(s string) bool {
	return validSupplierStatuses[s]
}

const (
	RequisitionStatusDraft           = "draft"
	RequisitionStatusPendingApproval = "pending_approval"
	RequisitionStatusApproved        = "approved"
	RequisitionStatusRejected        = "rejected"
	RequisitionStatusOrdered         = "ordered"
)

var validRequisitionStatuses = map[string]bool{
	RequisitionStatusDraft:           true,
	RequisitionStatusPendingApproval: true,
	RequisitionStatusApproved:        true,
	RequisitionStatusRejected:        true,
	RequisitionStatusOrdered:         true,
}

func ValidRequisitionStatus(s string) bool {
	return validRequisitionStatuses[s]
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
	PurchaseOrderStatusDraft             = "draft"
	PurchaseOrderStatusIssued            = "issued"
	PurchaseOrderStatusPartiallyReceived = "partially_received"
	PurchaseOrderStatusReceived          = "received"
	PurchaseOrderStatusCancelled         = "cancelled"
)

var validPOStatuses = map[string]bool{
	PurchaseOrderStatusDraft:             true,
	PurchaseOrderStatusIssued:            true,
	PurchaseOrderStatusPartiallyReceived: true,
	PurchaseOrderStatusReceived:          true,
	PurchaseOrderStatusCancelled:         true,
}

func ValidPOStatus(s string) bool {
	return validPOStatuses[s]
}

const (
	ReceiptStatusDraft       = "draft"
	ReceiptStatusReceived    = "received"
	ReceiptStatusQuarantined = "quarantined"
	ReceiptStatusRejected    = "rejected"
)

var validReceiptStatuses = map[string]bool{
	ReceiptStatusDraft:       true,
	ReceiptStatusReceived:    true,
	ReceiptStatusQuarantined: true,
	ReceiptStatusRejected:    true,
}

func ValidReceiptStatus(s string) bool {
	return validReceiptStatuses[s]
}

const (
	ConditionGood      = "good"
	ConditionDamaged   = "damaged"
	ConditionShort     = "short"
	ConditionWrongItem = "wrong_item"
)

var validConditions = map[string]bool{
	ConditionGood:      true,
	ConditionDamaged:   true,
	ConditionShort:     true,
	ConditionWrongItem: true,
}

func ValidCondition(c string) bool {
	return validConditions[c]
}

const (
	RebateStatusOffered  = "offered"
	RebateStatusAccepted = "accepted"
	RebateStatusSettled  = "settled"
)

var validRebateStatuses = map[string]bool{
	RebateStatusOffered:  true,
	RebateStatusAccepted: true,
	RebateStatusSettled:  true,
}

func ValidRebateStatus(s string) bool {
	return validRebateStatuses[s]
}

const (
	CategoryFood = "food"
)

var currencyRe = regexp.MustCompile(`^[A-Z]{3}$`)

func ValidCurrency(c string) bool {
	return currencyRe.MatchString(c)
}

type Supplier struct {
	ID          string     `json:"id"`
	TenantID    string     `json:"tenant_id"`
	Name        string     `json:"name"`
	ContactInfo string     `json:"contact_info"`
	Status      string     `json:"status"`
	CreatedBy   string     `json:"created_by"`
	ApprovedBy  string     `json:"approved_by,omitempty"`
	ApprovedAt  *time.Time `json:"approved_at,omitempty"`
	Version     int64      `json:"version"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (s *Supplier) IsNew() bool {
	return s.Status == SupplierStatusPendingApproval
}

type SupplierItem struct {
	ID                   string    `json:"id"`
	TenantID             string    `json:"tenant_id"`
	SupplierID           string    `json:"supplier_id"`
	CatalogItemID        string    `json:"catalog_item_id"`
	SupplierSKU          string    `json:"supplier_sku"`
	UnitCostMinorUnits   int64     `json:"unit_cost_minor_units"`
	UnitCostCurrency     string    `json:"unit_cost_currency"`
	LeadTimeDays         int       `json:"lead_time_days"`
	MinimumOrderQuantity int64     `json:"minimum_order_quantity"`
	IsPreferred          bool      `json:"is_preferred"`
	Version              int64     `json:"version"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type Requisition struct {
	ID                  string            `json:"id"`
	TenantID            string            `json:"tenant_id"`
	PropertyID          string            `json:"property_id"`
	Status              string            `json:"status"`
	CreatedBy           string            `json:"created_by"`
	ApprovedBy          string            `json:"approved_by,omitempty"`
	RejectedBy          string            `json:"rejected_by,omitempty"`
	TotalCostMinorUnits int64             `json:"total_cost_minor_units"`
	Currency            string            `json:"currency"`
	Notes               string            `json:"notes"`
	NewSupplierIDs      []string          `json:"new_supplier_ids,omitempty"`
	Version             int64             `json:"version"`
	CreatedAt           time.Time         `json:"created_at"`
	UpdatedAt           time.Time         `json:"updated_at"`
	Items               []RequisitionItem `json:"items,omitempty"`
}

type RequisitionItem struct {
	ID                  string    `json:"id"`
	TenantID            string    `json:"tenant_id"`
	RequisitionID       string    `json:"requisition_id"`
	CatalogItemID       string    `json:"catalog_item_id"`
	SupplierItemID      string    `json:"supplier_item_id"`
	Quantity            int64     `json:"quantity"`
	UnitCostMinorUnits  int64     `json:"unit_cost_minor_units"`
	UnitCostCurrency    string    `json:"unit_cost_currency"`
	LineTotalMinorUnits int64     `json:"line_total_minor_units"`
	CreatedAt           time.Time `json:"created_at"`
}

type RequisitionApproval struct {
	ID            string    `json:"id"`
	TenantID      string    `json:"tenant_id"`
	RequisitionID string    `json:"requisition_id"`
	ActorID       string    `json:"actor_id"`
	Decision      string    `json:"decision"`
	Reason        string    `json:"reason,omitempty"`
	IsAIActor     bool      `json:"is_ai_actor"`
	CreatedAt     time.Time `json:"created_at"`
}

type PurchaseOrder struct {
	ID               string              `json:"id"`
	TenantID         string              `json:"tenant_id"`
	RequisitionID    string              `json:"requisition_id"`
	SupplierID       string              `json:"supplier_id"`
	Status           string              `json:"status"`
	OrderedBy        string              `json:"ordered_by"`
	TotalMinorUnits  int64               `json:"total_minor_units"`
	Currency         string              `json:"currency"`
	OrderDate        time.Time           `json:"order_date"`
	ExpectedDelivery *time.Time          `json:"expected_delivery,omitempty"`
	Version          int64               `json:"version"`
	CreatedAt        time.Time           `json:"created_at"`
	UpdatedAt        time.Time           `json:"updated_at"`
	Items            []PurchaseOrderItem `json:"items,omitempty"`
}

type PurchaseOrderItem struct {
	ID                  string    `json:"id"`
	TenantID            string    `json:"tenant_id"`
	PurchaseOrderID     string    `json:"purchase_order_id"`
	RequisitionItemID   string    `json:"requisition_item_id"`
	CatalogItemID       string    `json:"catalog_item_id"`
	Quantity            int64     `json:"quantity"`
	UnitCostMinorUnits  int64     `json:"unit_cost_minor_units"`
	Currency            string    `json:"currency"`
	LineTotalMinorUnits int64     `json:"line_total_minor_units"`
	CreatedAt           time.Time `json:"created_at"`
}

type GoodsReceipt struct {
	ID              string             `json:"id"`
	TenantID        string             `json:"tenant_id"`
	PurchaseOrderID string             `json:"purchase_order_id"`
	ReceivedBy      string             `json:"received_by"`
	Status          string             `json:"status"`
	Condition       string             `json:"condition"`
	ConditionNotes  string             `json:"condition_notes"`
	EvidenceRef     string             `json:"evidence_ref"`
	ReceivedAt      time.Time          `json:"received_at"`
	Version         int64              `json:"version"`
	CreatedAt       time.Time          `json:"created_at"`
	UpdatedAt       time.Time          `json:"updated_at"`
	Items           []GoodsReceiptItem `json:"items,omitempty"`
}

type GoodsReceiptItem struct {
	ID                  string    `json:"id"`
	TenantID            string    `json:"tenant_id"`
	GoodsReceiptID      string    `json:"goods_receipt_id"`
	PurchaseOrderItemID string    `json:"purchase_order_item_id"`
	CatalogItemID       string    `json:"catalog_item_id"`
	QuantityOrdered     int64     `json:"quantity_ordered"`
	QuantityReceived    int64     `json:"quantity_received"`
	CreatedAt           time.Time `json:"created_at"`
}

type SupplierRebate struct {
	ID               string     `json:"id"`
	TenantID         string     `json:"tenant_id"`
	SupplierID       string     `json:"supplier_id"`
	PurchaseOrderID  string     `json:"purchase_order_id"`
	Description      string     `json:"description"`
	AmountMinorUnits int64      `json:"amount_minor_units"`
	Currency         string     `json:"currency"`
	Status           string     `json:"status"`
	OfferedAt        time.Time  `json:"offered_at"`
	SettledAt        *time.Time `json:"settled_at,omitempty"`
	Version          int64      `json:"version"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type CreateSupplierParams struct {
	Name        string
	ContactInfo string
}

type CreateSupplierItemParams struct {
	CatalogItemID        string
	SupplierSKU          string
	UnitCostMinorUnits   int64
	UnitCostCurrency     string
	LeadTimeDays         int
	MinimumOrderQuantity int64
	IsPreferred          bool
}

type CreateRequisitionParams struct {
	PropertyID string
	Notes      string
	Items      []RequisitionItemInput
}

type RequisitionItemInput struct {
	SupplierItemID string
	Quantity       int64
}

type SubmitRequisitionParams struct {
	ActorID   string
	IsAIActor bool
}

type ApproveRequisitionParams struct {
	ActorID   string
	IsAIActor bool
	Reason    string
}

type RejectRequisitionParams struct {
	ActorID   string
	IsAIActor bool
	Reason    string
}

type CreatePurchaseOrderParams struct {
	SupplierID       string
	OrderedBy        string
	ExpectedDelivery *time.Time
}

type CreateGoodsReceiptParams struct {
	ReceivedBy     string
	Condition      string
	ConditionNotes string
	EvidenceRef    string
	Items          []GoodsReceiptItemInput
}

type GoodsReceiptItemInput struct {
	PurchaseOrderItemID string
	QuantityReceived    int64
}

type CreateRebateParams struct {
	Description      string
	AmountMinorUnits int64
	Currency         string
}

type ReorderBasis struct {
	CatalogItemID       string
	CurrentStock        int64
	AverageDailyUsage   float64
	LeadTimeDays        int
	SafetyStockDays     int
	ReorderPoint        int64
	ReorderQuantity     int64
	SupplierItemID      string
	UnitCostMinorUnits  int64
	UnitCostCurrency    string
	LineTotalMinorUnits int64
}
