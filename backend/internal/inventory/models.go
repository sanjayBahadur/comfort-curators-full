package inventory

import (
	"errors"
	"time"
)

var (
	ErrLocationNotFound        = errors.New("stock location not found")
	ErrInvalidLocation         = errors.New("invalid stock location")
	ErrMovementNotFound        = errors.New("inventory movement not found")
	ErrInvalidMovement         = errors.New("invalid inventory movement")
	ErrNegativeStock           = errors.New("unexplained negative stock is rejected")
	ErrExpiredStockCannotIssue = errors.New("expired stock cannot be issued")
	ErrMovementLedgerImmutable = errors.New("inventory movement ledger is append-only and cannot be updated or deleted")
	ErrCountNotFound           = errors.New("inventory count not found")
	ErrInvalidCount            = errors.New("invalid inventory count")
	ErrCountAlreadyReviewed    = errors.New("inventory count already reviewed")
	ErrConcurrentConflict      = errors.New("concurrent movement conflict; retry")
)

const (
	LocationTypeCentral   = "central"
	LocationTypeInTransit = "in_transit"
	LocationTypeWorkerKit = "worker_kit"
	LocationTypeProperty  = "property"
)

var validLocationTypes = map[string]bool{
	LocationTypeCentral:   true,
	LocationTypeInTransit: true,
	LocationTypeWorkerKit: true,
	LocationTypeProperty:  true,
}

func ValidLocationType(t string) bool {
	return validLocationTypes[t]
}

const (
	MovementTypeReceive     = "receive"
	MovementTypeIssue       = "issue"
	MovementTypeTransferIn  = "transfer_in"
	MovementTypeTransferOut = "transfer_out"
	MovementTypeAdjustment  = "adjustment"
	MovementTypeReturn      = "return"
	MovementTypeConsumption = "consumption"
	MovementTypeExpiry      = "expiry"
)

var validMovementTypes = map[string]bool{
	MovementTypeReceive:     true,
	MovementTypeIssue:       true,
	MovementTypeTransferIn:  true,
	MovementTypeTransferOut: true,
	MovementTypeAdjustment:  true,
	MovementTypeReturn:      true,
	MovementTypeConsumption: true,
	MovementTypeExpiry:      true,
}

func ValidMovementType(t string) bool {
	return validMovementTypes[t]
}

const (
	CountStatusDraft      = "draft"
	CountStatusInProgress = "in_progress"
	CountStatusReviewed   = "reviewed"
	CountStatusReconciled = "reconciled"
)

var validCountStatuses = map[string]bool{
	CountStatusDraft:      true,
	CountStatusInProgress: true,
	CountStatusReviewed:   true,
	CountStatusReconciled: true,
}

func ValidCountStatus(s string) bool {
	return validCountStatuses[s]
}

type StockLocation struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id"`
	PropertyID   string    `json:"property_id,omitempty"`
	Name         string    `json:"name"`
	LocationType string    `json:"location_type"`
	Version      int64     `json:"version"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type InventoryMovement struct {
	ID            string     `json:"id"`
	TenantID      string     `json:"tenant_id"`
	LocationID    string     `json:"location_id"`
	CatalogItemID string     `json:"catalog_item_id"`
	MovementType  string     `json:"movement_type"`
	Quantity      int64      `json:"quantity"`
	ReferenceType string     `json:"reference_type,omitempty"`
	ReferenceID   string     `json:"reference_id,omitempty"`
	Reason        string     `json:"reason,omitempty"`
	ActorID       string     `json:"actor_id,omitempty"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

type InventoryCount struct {
	ID         string    `json:"id"`
	TenantID   string    `json:"tenant_id"`
	LocationID string    `json:"location_id"`
	Status     string    `json:"status"`
	CountedBy  string    `json:"counted_by,omitempty"`
	ReviewedBy string    `json:"reviewed_by,omitempty"`
	Version    int64     `json:"version"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type InventoryCountLine struct {
	ID               string    `json:"id"`
	TenantID         string    `json:"tenant_id"`
	CountID          string    `json:"count_id"`
	CatalogItemID    string    `json:"catalog_item_id"`
	ExpectedQuantity int64     `json:"expected_quantity"`
	CountedQuantity  int64     `json:"counted_quantity"`
	Variance         int64     `json:"variance"`
	CreatedAt        time.Time `json:"created_at"`
}

type CreateLocationParams struct {
	Name         string
	PropertyID   string
	LocationType string
}

type RecordMovementParams struct {
	CatalogItemID string
	MovementType  string
	Quantity      int64
	ReferenceType string
	ReferenceID   string
	Reason        string
	ExpiresAt     *time.Time
}

type CreateCountParams struct {
	LocationID string
	CountedBy  string
}

type UpdateCountLineParams struct {
	CatalogItemID   string
	CountedQuantity int64
}

type ReviewCountParams struct {
	ReviewedBy string
}

type ReconcileCountParams struct {
	ReviewedBy string
	Reason     string
}

// Balance rebuilds from the ledger: SUM of all quantities for a location+item.
func ComputeBalance(movements []InventoryMovement) int64 {
	var balance int64
	for _, m := range movements {
		balance += m.Quantity
	}
	return balance
}

// IsNegativeStock checks whether a proposed movement quantity would cause
// the balance to become negative. A negative balance after an adjustment
// is only allowed when the movement is an explicit attributable adjustment.
func IsNegativeStock(balance int64, proposed int64, movementType string) (negative bool, explained bool) {
	newBalance := balance + proposed
	if newBalance >= 0 {
		return false, true
	}
	explained = movementType == MovementTypeAdjustment
	return true, explained
}

// IsExpired checks whether the expiry date has passed relative to now.
func IsExpired(expiresAt *time.Time, now time.Time) bool {
	if expiresAt == nil || expiresAt.IsZero() {
		return false
	}
	return expiresAt.Before(now)
}
