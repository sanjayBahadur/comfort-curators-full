package store

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"comfort-curators-backend/internal/iam"
	"comfort-curators-backend/internal/platform/security"
)

type recordingProvider struct {
	searchQuery, searchProvider string
	quoteRequest                QuoteRequest
	orderRequest                PlaceOrderRequest
}

func (p *recordingProvider) Search(_ context.Context, query, provider string) ([]CatalogItem, error) {
	p.searchQuery, p.searchProvider = query, provider
	return []CatalogItem{{ID: "item-1", Name: "Tea", Provider: ProviderZepto}}, nil
}

func (p *recordingProvider) Quote(_ context.Context, request QuoteRequest) (Quote, error) {
	p.quoteRequest = request
	return Quote{ID: "quote-1", Provider: ProviderZepto}, nil
}

func (p *recordingProvider) PlaceOrder(_ context.Context, request PlaceOrderRequest) (OrderConfirmation, error) {
	p.orderRequest = request
	return OrderConfirmation{OrderID: "mock_order_test", IsMock: true}, nil
}

func authenticatedRequest(method, target, body string) *http.Request {
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	return r.WithContext(iamContext(r.Context()))
}

func iamContext(ctx context.Context) context.Context {
	return iam.WithSubject(ctx, security.Subject{ActorID: "guest-1", TenantID: "tenant-1"})
}

func TestHandlerGuestStoreFlow(t *testing.T) {
	provider := &recordingProvider{}
	h := NewHandler(provider)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	search := httptest.NewRecorder()
	mux.ServeHTTP(search, authenticatedRequest(http.MethodGet, "/v1/store/catalog?property_id=property-1&query=tea&provider=ZEPTO", ""))
	if search.Code != http.StatusOK || provider.searchQuery != "tea" || provider.searchProvider != "ZEPTO" {
		t.Fatalf("unexpected search response/provider call: status=%d query=%q provider=%q", search.Code, provider.searchQuery, provider.searchProvider)
	}
	var catalog struct {
		Items []CatalogItem `json:"items"`
	}
	if err := json.NewDecoder(search.Body).Decode(&catalog); err != nil || len(catalog.Items) != 1 {
		t.Fatalf("search did not return catalog: err=%v body=%s", err, search.Body.String())
	}

	quoteBody := `{"items":[{"catalog_item_id":"mock_catalog_tea","quantity":2}]}`
	quoteResponse := httptest.NewRecorder()
	mux.ServeHTTP(quoteResponse, authenticatedRequest(http.MethodPost, "/v1/store/quotes", quoteBody))
	if quoteResponse.Code != http.StatusOK || !reflect.DeepEqual(provider.quoteRequest, QuoteRequest{Items: []QuoteItemRequest{{CatalogItemID: "mock_catalog_tea", Quantity: 2}}}) {
		t.Fatalf("unexpected quote response/provider call: status=%d request=%+v", quoteResponse.Code, provider.quoteRequest)
	}

	orderResponse := httptest.NewRecorder()
	mux.ServeHTTP(orderResponse, authenticatedRequest(http.MethodPost, "/v1/store/orders", `{"tenant_id":"tenant-1","property_id":"property-1","quote":{"id":"quote-1","provider":"ZEPTO"}}`))
	var confirmation OrderConfirmation
	if err := json.NewDecoder(orderResponse.Body).Decode(&confirmation); err != nil {
		t.Fatal(err)
	}
	if orderResponse.Code != http.StatusCreated || !confirmation.IsMock || !strings.HasPrefix(confirmation.OrderID, "mock_order_") {
		t.Fatalf("unexpected order response: status=%d confirmation=%+v", orderResponse.Code, confirmation)
	}
	if provider.orderRequest.TenantID != "tenant-1" || provider.orderRequest.GuestID != "guest-1" {
		t.Fatalf("order scope/guest not propagated: %+v", provider.orderRequest)
	}
}

func TestHandlerRequiresAuthenticationAndOrderTenantMatch(t *testing.T) {
	h := NewHandler(NewMockProvider())
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	unauthenticated := httptest.NewRecorder()
	mux.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/v1/store/catalog?property_id=property-1", nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated search status = %d", unauthenticated.Code)
	}

	mismatch := httptest.NewRecorder()
	mux.ServeHTTP(mismatch, authenticatedRequest(http.MethodPost, "/v1/store/orders", `{"tenant_id":"other-tenant","property_id":"property-1"}`))
	if mismatch.Code != http.StatusForbidden {
		t.Fatalf("cross-tenant order status = %d", mismatch.Code)
	}
}
