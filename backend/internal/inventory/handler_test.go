package inventory

import (
	"net/http"
	"testing"
	"time"
)

func TestHandlerNew(t *testing.T) {
	svc := &Service{}
	h := NewHandler(svc)
	if h == nil || h.svc != svc {
		t.Fatal("handler must wrap the service")
	}
}

func TestRegisterRoutes(t *testing.T) {
	svc := &Service{}
	h := NewHandler(svc)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
}

func TestLocationView(t *testing.T) {
	now := time.Now().UTC()
	loc := &StockLocation{
		ID:           "loc-test",
		TenantID:     "tenant-1",
		PropertyID:   "prop-1",
		Name:         "Central Warehouse",
		LocationType: LocationTypeCentral,
		Version:      1,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	v := locationView(loc)
	if v["id"] != "loc-test" {
		t.Fatalf("expected id loc-test, got %v", v["id"])
	}
	if v["location_type"] != LocationTypeCentral {
		t.Fatalf("expected central, got %v", v["location_type"])
	}
	if v["property_id"] != "prop-1" {
		t.Fatalf("expected property_id prop-1, got %v", v["property_id"])
	}
}

func TestMovementView(t *testing.T) {
	now := time.Now().UTC()
	future := now.Add(30 * 24 * time.Hour)
	m := &InventoryMovement{
		ID:            "mov-1",
		TenantID:      "tenant-1",
		LocationID:    "loc-1",
		CatalogItemID: "item-1",
		MovementType:  MovementTypeReceive,
		Quantity:      100,
		ReferenceType: "purchase_order",
		ReferenceID:   "po-1",
		Reason:        "initial stock",
		ActorID:       "actor-1",
		ExpiresAt:     &future,
		CreatedAt:     now,
	}

	v := movementView(m)
	if v["id"] != "mov-1" {
		t.Fatalf("expected id mov-1, got %v", v["id"])
	}
	if v["movement_type"] != MovementTypeReceive {
		t.Fatalf("expected receive, got %v", v["movement_type"])
	}
	if v["quantity"] != int64(100) {
		t.Fatalf("expected quantity 100, got %v", v["quantity"])
	}
	if _, ok := v["expires_at"]; !ok {
		t.Fatal("expires_at must be present when set")
	}

	m2 := &InventoryMovement{ID: "mov-2"}
	v2 := movementView(m2)
	if _, ok := v2["expires_at"]; ok {
		t.Fatal("expires_at must be absent when nil")
	}
}

func TestCountView(t *testing.T) {
	now := time.Now().UTC()
	c := &InventoryCount{
		ID:         "cnt-1",
		TenantID:   "tenant-1",
		LocationID: "loc-1",
		Status:     CountStatusInProgress,
		CountedBy:  "worker-1",
		ReviewedBy: "",
		Version:    2,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	v := countView(c)
	if v["id"] != "cnt-1" {
		t.Fatalf("expected id cnt-1, got %v", v["id"])
	}
	if v["status"] != CountStatusInProgress {
		t.Fatalf("expected in_progress, got %v", v["status"])
	}
	if v["reviewed_by"] != "" {
		t.Fatalf("expected empty reviewed_by, got %v", v["reviewed_by"])
	}
}

func TestCountLineView(t *testing.T) {
	now := time.Now().UTC()
	line := &InventoryCountLine{
		ID:               "cnl-1",
		TenantID:         "tenant-1",
		CountID:          "cnt-1",
		CatalogItemID:    "item-1",
		ExpectedQuantity: 100,
		CountedQuantity:  95,
		Variance:         -5,
		CreatedAt:        now,
	}

	v := countLineView(line)
	if v["id"] != "cnl-1" {
		t.Fatalf("expected id cnl-1, got %v", v["id"])
	}
	if v["variance"] != int64(-5) {
		t.Fatalf("expected variance -5, got %v", v["variance"])
	}
}
