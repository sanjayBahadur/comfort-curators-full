package catalog

import (
	"errors"
	"testing"
)

func int64Ptr(v int64) *int64 { return &v }

func TestValidLabelSponsoredIsNotAssignable(t *testing.T) {
	if ValidLabel(LabelSponsored) {
		t.Fatal("sponsored label must not be assignable while sponsored placement is disabled")
	}
	for _, label := range []string{LabelCuratorsStandard, LabelOwnerPreferred, LabelAlternative} {
		if !ValidLabel(label) {
			t.Fatalf("label %q must be assignable", label)
		}
	}
	if ValidLabel("") || ValidLabel("premium") {
		t.Fatal("unknown or empty labels must be invalid")
	}
}

func TestCatalogItemLabelIsAlwaysVisible(t *testing.T) {
	for _, label := range []string{LabelCuratorsStandard, LabelOwnerPreferred, LabelAlternative} {
		item := &CatalogItem{Label: label}
		if !item.IsOperationalLabelVisible() {
			t.Fatalf("assignable label %q must never be concealed", label)
		}
	}
}

func TestValidClaimType(t *testing.T) {
	for _, claimType := range []string{ClaimTypeQuality, ClaimTypeSustainability, ClaimTypePerformance, ClaimTypeOrigin} {
		if !ValidClaimType(claimType) {
			t.Fatalf("claim type %q must be valid", claimType)
		}
	}
	if ValidClaimType("free") || ValidClaimType("") {
		t.Fatal("unknown or empty claim types must be invalid")
	}
}

func TestValidSubstitutionPolicy(t *testing.T) {
	for _, policy := range []string{SubstitutionOwnerApproval, SubstitutionAutomatic, SubstitutionRestricted} {
		if !ValidSubstitutionPolicy(policy) {
			t.Fatalf("policy %q must be valid", policy)
		}
	}
	if ValidSubstitutionPolicy("ask_first") {
		t.Fatal("unknown policy must be invalid")
	}
}

func TestValidCurrency(t *testing.T) {
	if !ValidCurrency("INR") || !ValidCurrency("USD") || !ValidCurrency("EUR") {
		t.Fatal("ISO 4217 alphabetic codes must be valid")
	}
	for _, bad := range []string{"", "inr", "IN", "INR1", "INRS"} {
		if ValidCurrency(bad) {
			t.Fatalf("currency %q must be invalid", bad)
		}
	}
}

func TestBuildReviewSummaryAggregatesCostsAndConsumption(t *testing.T) {
	items := []ReviewItem{
		{
			SKU:                        "TP-001",
			Quantity:                   4,
			ExpectedMonthlyConsumption: 6,
			SetupCostMinorUnits:        4 * 2500,
			MonthlyCostMinorUnits:      6 * 2500,
		},
		{
			SKU:                        "SOAP-001",
			Quantity:                   2,
			ExpectedMonthlyConsumption: 2,
			SetupCostMinorUnits:        2 * 4000,
			MonthlyCostMinorUnits:      2 * 4000,
		},
	}

	summary, err := BuildReviewSummary(SubstitutionOwnerApproval, nil, true, true, items, "INR")
	if err != nil {
		t.Fatalf("build review summary: %v", err)
	}

	wantSetup := int64(4*2500 + 2*4000)
	wantMonthly := int64(6*2500 + 2*4000)
	if summary.SetupCostMinorUnits != wantSetup {
		t.Fatalf("setup cost = %d, want %d", summary.SetupCostMinorUnits, wantSetup)
	}
	if summary.MonthlyCostMinorUnits != wantMonthly {
		t.Fatalf("monthly cost = %d, want %d", summary.MonthlyCostMinorUnits, wantMonthly)
	}
	if summary.MonthlyConsumptionUnits != 8 {
		t.Fatalf("monthly consumption = %d, want 8", summary.MonthlyConsumptionUnits)
	}
	if !summary.SubstitutionApprovalRequired {
		t.Fatal("owner_approval policy must require approval for substitution")
	}
	if !summary.PriceIncreaseRequiresApproval || !summary.NewSKURequiresApproval {
		t.Fatal("price-increase and new-SKU approval flags must be reflected in the summary")
	}
	if len(summary.Items) != 2 {
		t.Fatalf("summary must retain %d items, got %d", 2, len(summary.Items))
	}
}

func TestBuildReviewSummarySubstitutionBehavior(t *testing.T) {
	items := []ReviewItem{{SKU: "TP-001"}}

	restricted, err := BuildReviewSummary(SubstitutionRestricted, nil, false, false, items, "INR")
	if err != nil {
		t.Fatalf("build restricted summary: %v", err)
	}
	if !restricted.SubstitutionApprovalRequired || restricted.SubstitutionBehavior == "" {
		t.Fatal("restricted policy must require approval and describe behavior")
	}

	automatic, err := BuildReviewSummary(SubstitutionAutomatic, nil, false, false, items, "INR")
	if err != nil {
		t.Fatalf("build automatic summary: %v", err)
	}
	if automatic.SubstitutionApprovalRequired {
		t.Fatal("automatic policy must not require approval for in-group substitution")
	}
	if automatic.SubstitutionBehavior == "" {
		t.Fatal("automatic policy must still describe substitution behavior")
	}
}

func TestBuildReviewSummaryValidation(t *testing.T) {
	items := []ReviewItem{{SKU: "TP-001"}}

	if _, err := BuildReviewSummary("ask_first", nil, false, false, items, "INR"); !errors.Is(err, ErrInvalidSubstitutionPolicy) {
		t.Fatalf("unknown policy must be rejected, got %v", err)
	}
	if _, err := BuildReviewSummary(SubstitutionOwnerApproval, int64Ptr(-1), false, false, items, "INR"); !errors.Is(err, ErrInvalidPackageVersion) {
		t.Fatalf("negative budget must be rejected, got %v", err)
	}
	if _, err := BuildReviewSummary(SubstitutionOwnerApproval, nil, false, false, items, "inr"); !errors.Is(err, ErrInvalidCurrency) {
		t.Fatalf("invalid currency must be rejected, got %v", err)
	}
}
