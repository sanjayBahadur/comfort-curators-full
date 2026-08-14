// Package store defines the application-level boundary for guest procurement.
// Implementations must not be registered as Superhost agent tools: ordering is
// only available to a guest-facing flow after a human confirmation.
package store

import (
	"context"
	"errors"

	"comfort-curators-backend/internal/billing"
)

const (
	ProviderInstamart = "INSTAMART"
	ProviderZepto     = "ZEPTO"
	ProviderBlinkit   = "BLINKIT"
)

var (
	ErrInvalidRequest = errors.New("store: invalid request")
	ErrItemNotFound   = errors.New("store: catalog item not found")
)

// StoreProvider is the application boundary for selectable grocery providers.
// TenantID and PropertyID are supplied at order time so adapters cannot lose
// the scope of the guest's stay.
type StoreProvider interface {
	Search(ctx context.Context, query, provider string) ([]CatalogItem, error)
	Quote(ctx context.Context, request QuoteRequest) (Quote, error)
	PlaceOrder(ctx context.Context, request PlaceOrderRequest) (OrderConfirmation, error)
}

type CatalogItem struct {
	ID       string        `json:"id"`
	Name     string        `json:"name"`
	Category string        `json:"category"`
	Provider string        `json:"provider"`
	Price    billing.Money `json:"price"`
	Unit     string        `json:"unit"`
}

type QuoteRequest struct {
	Items []QuoteItemRequest `json:"items"`
}

type QuoteItemRequest struct {
	CatalogItemID string `json:"catalog_item_id"`
	Quantity      int64  `json:"quantity"`
}

type Quote struct {
	ID       string        `json:"id"`
	Items    []QuoteLine   `json:"items"`
	Total    billing.Money `json:"total"`
	Provider string        `json:"provider"`
}

type QuoteLine struct {
	CatalogItemID string        `json:"catalog_item_id"`
	Name          string        `json:"name"`
	Provider      string        `json:"provider"`
	Quantity      int64         `json:"quantity"`
	UnitPrice     billing.Money `json:"unit_price"`
	LineTotal     billing.Money `json:"line_total"`
}

type PlaceOrderRequest struct {
	TenantID   string `json:"tenant_id"`
	PropertyID string `json:"property_id"`
	GuestID    string `json:"guest_id,omitempty"`
	Quote      Quote  `json:"quote"`
}

type OrderConfirmation struct {
	OrderID    string        `json:"order_id"`
	QuoteID    string        `json:"quote_id"`
	TenantID   string        `json:"tenant_id"`
	PropertyID string        `json:"property_id"`
	Provider   string        `json:"provider"`
	Total      billing.Money `json:"total"`
	Status     string        `json:"status"`
	IsMock     bool          `json:"is_mock"`
}
