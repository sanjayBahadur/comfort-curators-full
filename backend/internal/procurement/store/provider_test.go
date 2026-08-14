package store

import (
	"context"
	"reflect"
	"testing"
)

func TestMockSearchIncludesAllNamedProviders(t *testing.T) {
	provider := NewMockProvider()
	items, err := provider.Search(context.Background(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, item := range items {
		seen[item.Provider] = true
	}
	for _, name := range []string{ProviderInstamart, ProviderZepto, ProviderBlinkit} {
		if !seen[name] {
			t.Fatalf("provider %s missing from catalog", name)
		}
	}
}

func TestMockOperationsAreDeterministic(t *testing.T) {
	provider := NewMockProvider()
	request := QuoteRequest{Items: []QuoteItemRequest{{CatalogItemID: "mock_catalog_dishwash_liquid", Quantity: 2}}}
	firstSearch, err := provider.Search(context.Background(), "clean", ProviderInstamart)
	if err != nil {
		t.Fatal(err)
	}
	secondSearch, err := provider.Search(context.Background(), "clean", ProviderInstamart)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(firstSearch, secondSearch) {
		t.Fatal("repeated search changed result")
	}
	firstQuote, err := provider.Quote(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	secondQuote, err := provider.Quote(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(firstQuote, secondQuote) {
		t.Fatal("repeated quote changed result")
	}
	orderRequest := PlaceOrderRequest{TenantID: "tenant-demo", PropertyID: "property-demo", Quote: firstQuote}
	firstOrder, err := provider.PlaceOrder(context.Background(), orderRequest)
	if err != nil {
		t.Fatal(err)
	}
	secondOrder, err := provider.PlaceOrder(context.Background(), orderRequest)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(firstOrder, secondOrder) {
		t.Fatal("repeated order changed result")
	}
	if !firstOrder.IsMock || firstOrder.OrderID[:len("mock_order_")] != "mock_order_" {
		t.Fatalf("order is not unambiguously mock: %+v", firstOrder)
	}
}

func TestMockQuoteUsesINRMinorUnits(t *testing.T) {
	quote, err := NewMockProvider().Quote(context.Background(), QuoteRequest{Items: []QuoteItemRequest{
		{CatalogItemID: "mock_catalog_dishwash_liquid", Quantity: 2},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if quote.Total.MinorUnits != 29000 || quote.Total.Currency != "INR" {
		t.Fatalf("unexpected quote total: %+v", quote.Total)
	}
}
