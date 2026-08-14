package contracts

import (
	"errors"
	"testing"
)

func TestFeeBaseExcludesProtectedPassThroughsByDefault(t *testing.T) {
	inputs := sampleQuoteInputs()
	base, err := ComputeFeeBase(inputs)
	if err != nil {
		t.Fatalf("compute fee base: %v", err)
	}

	var wantBase int64 = 1_000_000_00 - 100_000_00 - 50_000_00 - 20_000_00
	if base.BaseMinorUnits != wantBase {
		t.Errorf("fee base = %d, want %d", base.BaseMinorUnits, wantBase)
	}
	if base.ExcludedMinorUnits != 170_000_00 {
		t.Errorf("excluded minor units = %d, want 17000000", base.ExcludedMinorUnits)
	}
	if len(base.Exclusions) != 3 {
		t.Fatalf("all protected categories must be excluded, got %+v", base.Exclusions)
	}

	byCategory := map[string]int64{}
	for _, e := range base.Exclusions {
		byCategory[e.Category] = e.MinorUnits
	}
	if byCategory[PassThroughCategoryTaxes] != 100_000_00 {
		t.Errorf("taxes exclusion = %d, want 10000000", byCategory[PassThroughCategoryTaxes])
	}
	if byCategory[PassThroughCategoryCleaning] != 50_000_00 {
		t.Errorf("cleaning exclusion = %d, want 5000000", byCategory[PassThroughCategoryCleaning])
	}
	if byCategory[PassThroughCategoryRefundableDeposits] != 20_000_00 {
		t.Errorf("deposits exclusion = %d, want 2000000", byCategory[PassThroughCategoryRefundableDeposits])
	}
}

func TestFeeBaseNoPassThroughsLeavesRevenueIntact(t *testing.T) {
	inputs := sampleQuoteInputs()
	inputs.PassThroughs = nil
	base, err := ComputeFeeBase(inputs)
	if err != nil {
		t.Fatalf("compute fee base: %v", err)
	}
	if base.BaseMinorUnits != inputs.AccommodationRevenueMinorUnits {
		t.Errorf("fee base = %d, want %d", base.BaseMinorUnits, inputs.AccommodationRevenueMinorUnits)
	}
	if base.ExcludedMinorUnits != 0 || len(base.Exclusions) != 0 {
		t.Errorf("no pass-throughs must exclude nothing, got %+v", base)
	}
}

func TestFeeBaseOptInIncludesProtectedPassThrough(t *testing.T) {
	inputs := sampleQuoteInputs()
	inputs.IncludedPassThroughs = []string{PassThroughCategoryTaxes}
	base, err := ComputeFeeBase(inputs)
	if err != nil {
		t.Fatalf("compute fee base: %v", err)
	}

	// Taxes stay in the fee base because the owner contract explicitly opts in;
	// the other protected categories remain excluded.
	var wantBase int64 = 1_000_000_00 - 50_000_00 - 20_000_00
	if base.BaseMinorUnits != wantBase {
		t.Errorf("fee base = %d, want %d", base.BaseMinorUnits, wantBase)
	}
	for _, e := range base.Exclusions {
		if e.Category == PassThroughCategoryTaxes {
			t.Error("opted-in taxes must not appear as an exclusion")
		}
	}
}

func TestFeeBaseOptInAllProtectedCategories(t *testing.T) {
	inputs := sampleQuoteInputs()
	inputs.IncludedPassThroughs = []string{
		PassThroughCategoryTaxes,
		PassThroughCategoryCleaning,
		PassThroughCategoryRefundableDeposits,
	}
	base, err := ComputeFeeBase(inputs)
	if err != nil {
		t.Fatalf("compute fee base: %v", err)
	}
	if base.BaseMinorUnits != inputs.AccommodationRevenueMinorUnits {
		t.Errorf("fee base = %d, want full revenue %d", base.BaseMinorUnits, inputs.AccommodationRevenueMinorUnits)
	}
	if len(base.Exclusions) != 0 {
		t.Errorf("no exclusions when every protected category is opted in, got %+v", base.Exclusions)
	}
}

func TestFeeBaseIgnoresUnprotectedPassThroughs(t *testing.T) {
	inputs := sampleQuoteInputs()
	inputs.PassThroughs = append(inputs.PassThroughs, PassThroughAmount{
		Category:   "laundry",
		MinorUnits: 40_000_00,
	})
	base, err := ComputeFeeBase(inputs)
	if err != nil {
		t.Fatalf("compute fee base: %v", err)
	}
	if base.ExcludedMinorUnits != 170_000_00 {
		t.Errorf("unprotected pass-through must not be excluded, got excluded %d", base.ExcludedMinorUnits)
	}
}

func TestFeeBaseExclusionsCannotExceedRevenue(t *testing.T) {
	inputs := sampleQuoteInputs()
	inputs.AccommodationRevenueMinorUnits = 100_000_00
	inputs.PassThroughs = []PassThroughAmount{
		{Category: PassThroughCategoryCleaning, MinorUnits: 200_000_00},
	}
	if _, err := ComputeFeeBase(inputs); !errors.Is(err, ErrQuoteInputsInvalid) {
		t.Errorf("exclusions above revenue must be rejected, got %v", err)
	}
}

func TestFeeBaseZeroAmountsAreIgnored(t *testing.T) {
	inputs := sampleQuoteInputs()
	inputs.PassThroughs = append(inputs.PassThroughs, PassThroughAmount{
		Category:   PassThroughCategoryTaxes,
		MinorUnits: 0,
	})
	base, err := ComputeFeeBase(inputs)
	if err != nil {
		t.Fatalf("compute fee base: %v", err)
	}
	if base.ExcludedMinorUnits != 170_000_00 {
		t.Errorf("zero pass-through must not be excluded, got %d", base.ExcludedMinorUnits)
	}
}

func TestProtectedPassThrough(t *testing.T) {
	if !ProtectedPassThrough(PassThroughCategoryTaxes) {
		t.Error("taxes must be a protected pass-through")
	}
	if !ProtectedPassThrough(PassThroughCategoryRefundableDeposits) {
		t.Error("refundable deposits must be a protected pass-through")
	}
	if !ProtectedPassThrough(PassThroughCategoryCleaning) {
		t.Error("pass-through cleaning must be a protected pass-through")
	}
	if ProtectedPassThrough("laundry") {
		t.Error("laundry must not be protected")
	}
}
