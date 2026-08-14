package consumer

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"comfort-curators-backend/internal/catalog"
)

func TestPublicCatalogItemNeverHidesSponsoredStatus(t *testing.T) {
	sponsored := &catalog.CatalogItem{
		ID:                   "cit-1",
		SKU:                  "SKU-SP",
		Name:                 "Sponsored item",
		Category:             "cleaning",
		OwnerPriceMinorUnits: 1200,
		OwnerPriceCurrency:   "INR",
		Label:                catalog.LabelSponsored,
	}
	view := PublicCatalogItemFrom(sponsored)
	if view.Label != catalog.LabelSponsored {
		t.Fatalf("sponsored label must be exposed verbatim, got %q", view.Label)
	}
	if !view.IsSponsored {
		t.Fatal("sponsored status must be derived and cannot be hidden")
	}
}

func TestPublicCatalogItemAlwaysCarriesLabel(t *testing.T) {
	item := &catalog.CatalogItem{
		ID:                   "cit-2",
		SKU:                  "SKU-CS",
		Name:                 "Curators standard item",
		Category:             "toiletries",
		OwnerPriceMinorUnits: 800,
		OwnerPriceCurrency:   "INR",
		TaxClass:             "gst12",
		Supplier:             "supplier-1",
		CountryOfOrigin:      "India",
		Label:                catalog.LabelCuratorsStandard,
	}
	view := PublicCatalogItemFrom(item)
	if view.Label != catalog.LabelCuratorsStandard {
		t.Fatalf("operational label must be exposed, got %q", view.Label)
	}
	if view.IsSponsored {
		t.Fatal("a non-sponsored item must not be flagged sponsored")
	}
	if view.Seller != item.Supplier || view.CountryOfOrigin != "India" || view.TaxClass != "gst12" {
		t.Fatal("consumer view must expose seller, origin and tax class")
	}
}

func TestPublicCatalogItemLabelIsNeverOmittedFromJSON(t *testing.T) {
	view := PublicCatalogItemFrom(&catalog.CatalogItem{Label: ""})
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"label":""`) {
		t.Fatalf("label must always be present in the serialized view, got %s", raw)
	}
	if !strings.Contains(string(raw), `"is_sponsored":false`) {
		t.Fatalf("is_sponsored must always be present in the serialized view, got %s", raw)
	}
}

func TestPublicPackageViewExposesRecurringCostAndSubstitution(t *testing.T) {
	version := &catalog.PropertyPackageVersion{
		ID:                    "pkg-1",
		PropertyID:            "prop-1",
		VersionNumber:         1,
		Status:                catalog.PackageStatusDraft,
		EffectiveDate:         time.Now().UTC().AddDate(0, 0, 7),
		SetupCostMinorUnits:   10000,
		MonthlyCostMinorUnits: 18000,
		Currency:              "INR",
		SubstitutionPolicy:    catalog.SubstitutionOwnerApproval,
		ReviewSummary: catalog.ReviewSummary{
			SubstitutionBehavior: "Any substitution requires owner approval before it can be used.",
		},
		Items: []catalog.PropertyPackageItem{
			{
				SKU:                        "TP-001",
				Name:                       "Toilet Paper",
				Label:                      catalog.LabelCuratorsStandard,
				SubstitutionGroup:          "paper",
				Quantity:                   4,
				ExpectedMonthlyConsumption: 6,
				SetupCostMinorUnits:        10000,
				MonthlyCostMinorUnits:      18000,
			},
		},
	}

	view := PublicPackageViewFrom(version)
	if view.MonthlyCostMinorUnits != 18000 {
		t.Fatalf("recurring monthly cost must be visible, got %d", view.MonthlyCostMinorUnits)
	}
	if view.SetupCostMinorUnits != 10000 {
		t.Fatalf("setup cost must be visible, got %d", view.SetupCostMinorUnits)
	}
	if view.SubstitutionBehavior == "" {
		t.Fatal("substitution behavior must be visible before acceptance")
	}
	if len(view.Items) != 1 || view.Items[0].Label == "" {
		t.Fatal("package items must retain their operational label")
	}
}
