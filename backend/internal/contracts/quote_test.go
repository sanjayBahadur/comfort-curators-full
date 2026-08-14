package contracts

import (
	"encoding/json"
	"errors"
	"testing"
)

func sampleQuoteInputs() QuoteInputs {
	return QuoteInputs{
		TenantID:                       "tenant-1",
		PropertyID:                     "prop-1",
		ServiceTier:                    ServiceTierFullService,
		ManagedUnits:                   3,
		Currency:                       "INR",
		RevenuePeriod:                  "2026-07",
		AccommodationRevenueMinorUnits: 1_000_000_00,
		PassThroughs: []PassThroughAmount{
			{Category: PassThroughCategoryTaxes, MinorUnits: 100_000_00},
			{Category: PassThroughCategoryCleaning, MinorUnits: 50_000_00},
			{Category: PassThroughCategoryRefundableDeposits, MinorUnits: 20_000_00},
		},
	}
}

func sampleFeeRule() FeeRule {
	return FeeRule{
		Version:                     "2026-07-01",
		Currency:                    "INR",
		ServiceTier:                 ServiceTierFullService,
		PercentageBasisPoints:       1800,
		MinimumMonthlyFeeMinorUnits: 600_000_00,
		SetupFeeMinorUnits:          250_000_00,
		EffectiveFrom:               "2026-07-01",
	}
}

func TestQuoteIsDeterministic(t *testing.T) {
	first, err := CalculateQuote(sampleQuoteInputs(), sampleFeeRule())
	if err != nil {
		t.Fatalf("calculate quote: %v", err)
	}
	second, err := CalculateQuote(sampleQuoteInputs(), sampleFeeRule())
	if err != nil {
		t.Fatalf("calculate quote second run: %v", err)
	}

	if first.InputHash != second.InputHash {
		t.Errorf("identical inputs must produce identical input hash: %s vs %s", first.InputHash, second.InputHash)
	}
	if first.ManagementFeeMinorUnits != second.ManagementFeeMinorUnits {
		t.Errorf("identical inputs must produce identical management fee: %d vs %d", first.ManagementFeeMinorUnits, second.ManagementFeeMinorUnits)
	}

	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	if string(firstJSON) != string(secondJSON) {
		t.Errorf("identical inputs must produce identical quote JSON:\n%s\nvs\n%s", firstJSON, secondJSON)
	}
}

func TestQuoteInputHashChangesWhenInputsChange(t *testing.T) {
	base := sampleQuoteInputs()
	first, err := CalculateQuote(base, sampleFeeRule())
	if err != nil {
		t.Fatalf("calculate base quote: %v", err)
	}

	changed := base
	changed.AccommodationRevenueMinorUnits = 2_000_000_00
	second, err := CalculateQuote(changed, sampleFeeRule())
	if err != nil {
		t.Fatalf("calculate changed quote: %v", err)
	}
	if first.InputHash == second.InputHash {
		t.Error("changed revenue must change the input hash")
	}
}

func TestQuoteChangedRuleVersionIsVisible(t *testing.T) {
	inputs := sampleQuoteInputs()
	inputs.AccommodationRevenueMinorUnits = 5_000_000_00
	v1 := sampleFeeRule()
	v2 := sampleFeeRule()
	v2.Version = "2026-08-01"
	v2.PercentageBasisPoints = 2000

	quoteV1, err := CalculateQuote(inputs, v1)
	if err != nil {
		t.Fatalf("calculate v1 quote: %v", err)
	}
	quoteV2, err := CalculateQuote(inputs, v2)
	if err != nil {
		t.Fatalf("calculate v2 quote: %v", err)
	}

	if quoteV1.RuleVersion == quoteV2.RuleVersion {
		t.Fatal("changed rule version must be visible in the quote")
	}
	if quoteV1.RuleVersion != "2026-07-01" || quoteV2.RuleVersion != "2026-08-01" {
		t.Errorf("quote must expose the exact rule version: %s vs %s", quoteV1.RuleVersion, quoteV2.RuleVersion)
	}
	if quoteV1.InputHash == quoteV2.InputHash {
		t.Error("changed rule version must change the input hash")
	}
	if quoteV1.ManagementFeeMinorUnits == quoteV2.ManagementFeeMinorUnits {
		t.Error("changed percentage must change the management fee")
	}
}

func TestQuoteUsesIntegerMinorUnitsAndCurrency(t *testing.T) {
	inputs := sampleQuoteInputs()
	inputs.AccommodationRevenueMinorUnits = 5_000_000_00
	rule := sampleFeeRule()
	rule.PercentageBasisPoints = 1800

	quote, err := CalculateQuote(inputs, rule)
	if err != nil {
		t.Fatalf("calculate quote: %v", err)
	}

	// Fee base excludes the protected pass-throughs.
	var wantBase int64 = 5_000_000_00 - 100_000_00 - 50_000_00 - 20_000_00
	if quote.FeeBase.BaseMinorUnits != wantBase {
		t.Errorf("fee base = %d, want %d", quote.FeeBase.BaseMinorUnits, wantBase)
	}

	// 18% of base: the base is large enough that the minimum floor does not
	// apply, so the exact integer percentage is charged.
	var wantFee int64 = wantBase * 1800 / 10000
	if quote.ManagementFeeMinorUnits != wantFee {
		t.Errorf("management fee = %d, want %d", quote.ManagementFeeMinorUnits, wantFee)
	}
	if quote.EstimatedMonthlyMinorUnits != quote.ManagementFeeMinorUnits {
		t.Errorf("estimated monthly must equal the management fee, got %d", quote.EstimatedMonthlyMinorUnits)
	}
	if quote.Currency != "INR" {
		t.Errorf("quote must carry ISO 4217 currency, got %q", quote.Currency)
	}
	for _, item := range quote.LineItems {
		if item.MinorUnits <= 0 {
			t.Errorf("no quote line may be zero or negative money: %+v", item)
		}
	}
}

func TestQuoteMinimumMonthlyFeeFloorsManagementFee(t *testing.T) {
	inputs := sampleQuoteInputs()
	inputs.AccommodationRevenueMinorUnits = 300_000_00 // small revenue
	rule := sampleFeeRule()
	rule.MinimumMonthlyFeeMinorUnits = 600_000_00

	quote, err := CalculateQuote(inputs, rule)
	if err != nil {
		t.Fatalf("calculate quote: %v", err)
	}

	// 18% of a small base is below the minimum, so the floor applies.
	if quote.ManagementFeeMinorUnits != rule.MinimumMonthlyFeeMinorUnits {
		t.Errorf("management fee must floor at the minimum, got %d want %d", quote.ManagementFeeMinorUnits, rule.MinimumMonthlyFeeMinorUnits)
	}
}

func TestQuoteSetupFeeIsOneTimeLineItem(t *testing.T) {
	quote, err := CalculateQuote(sampleQuoteInputs(), sampleFeeRule())
	if err != nil {
		t.Fatalf("calculate quote: %v", err)
	}
	if quote.SetupFeeMinorUnits != 250_000_00 {
		t.Errorf("setup fee = %d, want 25000000", quote.SetupFeeMinorUnits)
	}
	found := false
	for _, item := range quote.LineItems {
		if item.Code == LineCodeSetupFee {
			found = true
			if item.Recurring {
				t.Error("setup fee must be a one-time charge, not recurring")
			}
		}
	}
	if !found {
		t.Error("quote must carry a setup fee line item")
	}
}

func TestQuoteValidation(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*QuoteInputs)
		wantErr error
	}{
		{name: "missing tenant", mutate: func(in *QuoteInputs) { in.TenantID = "" }, wantErr: ErrQuoteInputsInvalid},
		{name: "missing property", mutate: func(in *QuoteInputs) { in.PropertyID = "" }, wantErr: ErrQuoteInputsInvalid},
		{name: "unknown tier", mutate: func(in *QuoteInputs) { in.ServiceTier = "luxury" }, wantErr: ErrQuoteInputsInvalid},
		{name: "bad currency", mutate: func(in *QuoteInputs) { in.Currency = "RUPEE" }, wantErr: ErrQuoteInputsInvalid},
		{name: "missing period", mutate: func(in *QuoteInputs) { in.RevenuePeriod = "" }, wantErr: ErrQuoteInputsInvalid},
		{name: "negative revenue", mutate: func(in *QuoteInputs) { in.AccommodationRevenueMinorUnits = -1 }, wantErr: ErrQuoteInputsInvalid},
		{name: "negative units", mutate: func(in *QuoteInputs) { in.ManagedUnits = -2 }, wantErr: ErrQuoteInputsInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inputs := sampleQuoteInputs()
			tc.mutate(&inputs)
			if _, err := CalculateQuote(inputs, sampleFeeRule()); !errors.Is(err, tc.wantErr) {
				t.Errorf("expected %v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestQuoteRejectsMismatchedFeeRule(t *testing.T) {
	rule := sampleFeeRule()
	rule.Currency = "USD"
	if _, err := CalculateQuote(sampleQuoteInputs(), rule); !errors.Is(err, ErrInvalidFeeRule) {
		t.Errorf("currency mismatch must be rejected, got %v", err)
	}

	rule = sampleFeeRule()
	rule.ServiceTier = ServiceTierOperations
	if _, err := CalculateQuote(sampleQuoteInputs(), rule); !errors.Is(err, ErrInvalidFeeRule) {
		t.Errorf("tier mismatch must be rejected, got %v", err)
	}
}

func TestQuoteRejectsRuleWithoutVersion(t *testing.T) {
	rule := sampleFeeRule()
	rule.Version = ""
	if _, err := CalculateQuote(sampleQuoteInputs(), rule); !errors.Is(err, ErrInvalidFeeRule) {
		t.Errorf("rule without version must be rejected, got %v", err)
	}
}

func TestQuoteRejectsInvalidFeeRuleFields(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*FeeRule)
	}{
		{name: "negative percentage", mutate: func(r *FeeRule) { r.PercentageBasisPoints = -1 }},
		{name: "percentage over 100%", mutate: func(r *FeeRule) { r.PercentageBasisPoints = 10001 }},
		{name: "negative minimum", mutate: func(r *FeeRule) { r.MinimumMonthlyFeeMinorUnits = -5 }},
		{name: "negative setup", mutate: func(r *FeeRule) { r.SetupFeeMinorUnits = -5 }},
		{name: "unknown tier", mutate: func(r *FeeRule) { r.ServiceTier = "boutique" }},
		{name: "bad currency", mutate: func(r *FeeRule) { r.Currency = "EURO" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rule := sampleFeeRule()
			tc.mutate(&rule)
			if _, err := CalculateQuote(sampleQuoteInputs(), rule); !errors.Is(err, ErrInvalidFeeRule) {
				t.Errorf("invalid rule must be rejected with ErrInvalidFeeRule, got %v", err)
			}
		})
	}
}

func TestPercentageOfIsExactIntegerArithmetic(t *testing.T) {
	cases := []struct {
		base, bps, want int64
	}{
		{10_000_000, 1800, 1_800_000},   // 18% of ₹100,000 = ₹18,000
		{100_000_000, 1800, 18_000_000}, // 18% of ₹1,000,000 = ₹180,000
		{33_333_333, 1800, 5_999_999},   // 33333333*1800/10000 = 59999999.94, floored
		{0, 1800, 0},
		{1_000_000_00, 0, 0},
	}
	for _, tc := range cases {
		if got := percentageOf(tc.base, tc.bps); got != tc.want {
			t.Errorf("percentageOf(%d, %d) = %d, want %d", tc.base, tc.bps, got, tc.want)
		}
	}
}

func TestParseContentHash(t *testing.T) {
	valid := "sha256:" + "a1b2c3d4e5f60718293a4b5c6d7e8f901a2b3c4d5e6f708192a3b4c5d6e7f809"
	if _, ok := ParseContentHash(valid); !ok {
		t.Error("valid sha256 content hash must parse")
	}
	if _, ok := ParseContentHash("md5:abc"); ok {
		t.Error("non-sha256 hash must not parse")
	}
	if _, ok := ParseContentHash("sha256:short"); ok {
		t.Error("malformed digest must not parse")
	}
}
