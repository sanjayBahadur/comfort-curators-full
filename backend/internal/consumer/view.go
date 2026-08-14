package consumer

import (
	"time"

	"comfort-curators-backend/internal/catalog"
)

// PublicCatalogItem is the consumer-facing view of a catalog item. It always
// carries the operational label and a derived sponsored flag with no omitempty
// tag, so a sponsored placement (CON-003) can never be hidden from consumers.
// Price, tax class, seller and country of origin are always present where the
// underlying item carries them (CON-001, CON-002).
type PublicCatalogItem struct {
	ID                   string `json:"id"`
	SKU                  string `json:"sku"`
	Name                 string `json:"name"`
	Category             string `json:"category"`
	Brand                string `json:"brand,omitempty"`
	PackSize             string `json:"pack_size,omitempty"`
	OwnerPriceMinorUnits int64  `json:"owner_price_minor_units"`
	OwnerPriceCurrency   string `json:"owner_price_currency"`
	TaxClass             string `json:"tax_class,omitempty"`
	Seller               string `json:"seller,omitempty"`
	CountryOfOrigin      string `json:"country_of_origin,omitempty"`
	SubstitutionGroup    string `json:"substitution_group,omitempty"`
	Label                string `json:"label"`
	IsSponsored          bool   `json:"is_sponsored"`
}

// PublicCatalogItemFrom builds the consumer-facing view. The label is copied
// verbatim and never omitted; if the item were ever sponsored, the view would
// expose both the label and the derived sponsored flag.
func PublicCatalogItemFrom(item *catalog.CatalogItem) PublicCatalogItem {
	if item == nil {
		return PublicCatalogItem{}
	}
	return PublicCatalogItem{
		ID:                   item.ID,
		SKU:                  item.SKU,
		Name:                 item.Name,
		Category:             item.Category,
		Brand:                item.Brand,
		PackSize:             item.PackSize,
		OwnerPriceMinorUnits: item.OwnerPriceMinorUnits,
		OwnerPriceCurrency:   item.OwnerPriceCurrency,
		TaxClass:             item.TaxClass,
		Seller:               item.Supplier,
		CountryOfOrigin:      item.CountryOfOrigin,
		SubstitutionGroup:    item.SubstitutionGroup,
		Label:                item.Label,
		IsSponsored:          item.Label == catalog.LabelSponsored,
	}
}

// PublicPackageView exposes the recurring monthly cost and substitution
// behavior of a property package version before acceptance (CON-001). The
// monthly cost and setup cost are integer minor units throughout.
type PublicPackageView struct {
	ID                           string              `json:"id"`
	PropertyID                   string              `json:"property_id"`
	VersionNumber                int                 `json:"version_number"`
	Status                       string              `json:"status"`
	EffectiveDate                time.Time           `json:"effective_date"`
	SetupCostMinorUnits          int64               `json:"setup_cost_minor_units"`
	MonthlyCostMinorUnits        int64               `json:"recurring_monthly_cost_minor_units"`
	Currency                     string              `json:"currency"`
	SubstitutionPolicy           string              `json:"substitution_policy"`
	SubstitutionBehavior         string              `json:"substitution_behavior"`
	MonthlyBudgetLimitMinorUnits *int64              `json:"monthly_budget_limit_minor_units,omitempty"`
	Items                        []PublicPackageItem `json:"items"`
}

type PublicPackageItem struct {
	SKU                        string `json:"sku"`
	Name                       string `json:"name"`
	Label                      string `json:"label"`
	SubstitutionGroup          string `json:"substitution_group,omitempty"`
	Quantity                   int    `json:"quantity"`
	ExpectedMonthlyConsumption int    `json:"expected_monthly_consumption"`
	SetupCostMinorUnits        int64  `json:"setup_cost_minor_units"`
	MonthlyCostMinorUnits      int64  `json:"monthly_cost_minor_units"`
}

// PublicPackageViewFrom maps a property package version to its consumer-facing
// view. Package items carry their operational label verbatim so a sponsored
// placement inside a package cannot be hidden either (CON-003).
func PublicPackageViewFrom(v *catalog.PropertyPackageVersion) PublicPackageView {
	if v == nil {
		return PublicPackageView{}
	}
	view := PublicPackageView{
		ID:                           v.ID,
		PropertyID:                   v.PropertyID,
		VersionNumber:                v.VersionNumber,
		Status:                       v.Status,
		EffectiveDate:                v.EffectiveDate,
		SetupCostMinorUnits:          v.SetupCostMinorUnits,
		MonthlyCostMinorUnits:        v.MonthlyCostMinorUnits,
		Currency:                     v.Currency,
		SubstitutionPolicy:           v.SubstitutionPolicy,
		SubstitutionBehavior:         v.ReviewSummary.SubstitutionBehavior,
		MonthlyBudgetLimitMinorUnits: v.MonthlyBudgetLimitMinorUnits,
		Items:                        make([]PublicPackageItem, 0, len(v.Items)),
	}
	for _, it := range v.Items {
		view.Items = append(view.Items, PublicPackageItem{
			SKU:                        it.SKU,
			Name:                       it.Name,
			Label:                      it.Label,
			SubstitutionGroup:          it.SubstitutionGroup,
			Quantity:                   it.Quantity,
			ExpectedMonthlyConsumption: it.ExpectedMonthlyConsumption,
			SetupCostMinorUnits:        it.SetupCostMinorUnits,
			MonthlyCostMinorUnits:      it.MonthlyCostMinorUnits,
		})
	}
	return view
}
