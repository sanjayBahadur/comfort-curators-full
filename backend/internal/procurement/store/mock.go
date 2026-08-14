package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"comfort-curators-backend/internal/billing"
)

const mockCurrency = "INR"

// MockProvider is a deterministic local provider for development and tests.
type MockProvider struct {
	catalog []CatalogItem
}

func NewMockProvider() *MockProvider {
	catalog := []CatalogItem{
		{ID: "mock_catalog_floor_cleaner", Name: "Lemon floor cleaner", Category: "cleaning", Provider: ProviderInstamart, Price: money(18900), Unit: "1 litre"},
		{ID: "mock_catalog_dishwash_liquid", Name: "Dishwash liquid", Category: "kitchen", Provider: ProviderInstamart, Price: money(14500), Unit: "500 ml"},
		{ID: "mock_catalog_microfiber_cloths", Name: "Microfiber cleaning cloths", Category: "cleaning", Provider: ProviderInstamart, Price: money(12900), Unit: "pack of 3"},
		{ID: "mock_catalog_basmati_rice", Name: "Basmati rice", Category: "kitchen", Provider: ProviderZepto, Price: money(24900), Unit: "1 kg"},
		{ID: "mock_catalog_tea", Name: "Assam tea", Category: "kitchen", Provider: ProviderZepto, Price: money(17900), Unit: "250 g"},
		{ID: "mock_catalog_cotton_towels", Name: "Cotton bath towels", Category: "linens", Provider: ProviderZepto, Price: money(39900), Unit: "2 towels"},
		{ID: "mock_catalog_toilet_cleaner", Name: "Toilet cleaner", Category: "cleaning", Provider: ProviderBlinkit, Price: money(16500), Unit: "500 ml"},
		{ID: "mock_catalog_paper_towels", Name: "Kitchen paper towels", Category: "kitchen", Provider: ProviderBlinkit, Price: money(21900), Unit: "2 rolls"},
		{ID: "mock_catalog_bed_sheet", Name: "Cotton bed sheet", Category: "linens", Provider: ProviderBlinkit, Price: money(69900), Unit: "double bed"},
	}
	return &MockProvider{catalog: catalog}
}

func money(minorUnits int64) billing.Money {
	return billing.Money{MinorUnits: minorUnits, Currency: mockCurrency}
}

func (m *MockProvider) Search(ctx context.Context, query, provider string) ([]CatalogItem, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	query = strings.ToLower(strings.TrimSpace(query))
	provider = strings.ToUpper(strings.TrimSpace(provider))
	items := make([]CatalogItem, 0)
	for _, item := range m.catalog {
		if provider != "" && item.Provider != provider {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(item.Name), query) && !strings.Contains(strings.ToLower(item.Category), query) {
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

func (m *MockProvider) Quote(ctx context.Context, request QuoteRequest) (Quote, error) {
	if err := ctx.Err(); err != nil {
		return Quote{}, err
	}
	if len(request.Items) == 0 {
		return Quote{}, fmt.Errorf("%w: at least one item is required", ErrInvalidRequest)
	}

	byID := make(map[string]CatalogItem, len(m.catalog))
	for _, item := range m.catalog {
		byID[item.ID] = item
	}
	lines := make([]QuoteLine, 0, len(request.Items))
	var total int64
	provider := ""
	for _, requested := range request.Items {
		if requested.Quantity <= 0 {
			return Quote{}, fmt.Errorf("%w: quantity must be positive", ErrInvalidRequest)
		}
		item, ok := byID[requested.CatalogItemID]
		if !ok {
			return Quote{}, fmt.Errorf("%w: %s", ErrItemNotFound, requested.CatalogItemID)
		}
		if provider == "" {
			provider = item.Provider
		} else if provider != item.Provider {
			return Quote{}, fmt.Errorf("%w: quote cannot mix providers", ErrInvalidRequest)
		}
		lineTotal := item.Price.MinorUnits * requested.Quantity
		total += lineTotal
		lines = append(lines, QuoteLine{
			CatalogItemID: item.ID,
			Name:          item.Name,
			Provider:      item.Provider,
			Quantity:      requested.Quantity,
			UnitPrice:     item.Price,
			LineTotal:     money(lineTotal),
		})
	}

	// The request order is part of the stable quote input, while the ID is
	// content-derived so repeated calls never depend on process state.
	quoteID := stableID("mock_quote", request)
	return Quote{ID: quoteID, Items: lines, Total: money(total), Provider: provider}, nil
}

func (m *MockProvider) PlaceOrder(ctx context.Context, request PlaceOrderRequest) (OrderConfirmation, error) {
	if err := ctx.Err(); err != nil {
		return OrderConfirmation{}, err
	}
	if strings.TrimSpace(request.TenantID) == "" || strings.TrimSpace(request.PropertyID) == "" {
		return OrderConfirmation{}, fmt.Errorf("%w: tenant_id and property_id are required", ErrInvalidRequest)
	}
	if request.Quote.ID == "" || len(request.Quote.Items) == 0 || request.Quote.Total.MinorUnits <= 0 {
		return OrderConfirmation{}, fmt.Errorf("%w: a non-empty quote is required", ErrInvalidRequest)
	}
	return OrderConfirmation{
		OrderID:    stableID("mock_order", request),
		QuoteID:    request.Quote.ID,
		TenantID:   request.TenantID,
		PropertyID: request.PropertyID,
		Provider:   request.Quote.Provider,
		Total:      request.Quote.Total,
		Status:     "confirmed",
		IsMock:     true,
	}, nil
}

func stableID(prefix string, value any) string {
	data := fmt.Sprintf("%#v", value)
	hash := sha256.Sum256([]byte(data))
	return prefix + "_" + hex.EncodeToString(hash[:8])
}

// Catalog returns a copy for deterministic inspection without exposing the
// provider's internal slice to callers.
func (m *MockProvider) Catalog() []CatalogItem {
	items := append([]CatalogItem(nil), m.catalog...)
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items
}
