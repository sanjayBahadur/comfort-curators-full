package contracts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// Quote line item codes. The management fee is recurring monthly and the setup
// fee is a one-time charge; both are integer minor units.
const (
	LineCodeManagementFee = "management_fee"
	LineCodeSetupFee      = "setup_fee"
)

// ValidServiceTier reports whether tier is a frozen service tier.
func ValidServiceTier(tier string) bool {
	for _, t := range ValidServiceTiers {
		if t == tier {
			return true
		}
	}
	return false
}

// HashQuoteInputs returns the canonical content hash of the quote inputs and
// the rule version. It is a pure function of its arguments, so identical
// inputs always produce an identical hash and the hash exposes any changed
// input or changed rule version.
func HashQuoteInputs(inputs QuoteInputs, ruleVersion string) (string, error) {
	payload := struct {
		QuoteInputs
		RuleVersion string `json:"rule_version"`
	}{
		QuoteInputs: inputs,
		RuleVersion: ruleVersion,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("canonicalize quote inputs: %w", err)
	}
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// ValidateQuoteInputs enforces the mandatory quote inputs. Money is integer
// minor units and currency is ISO 4217.
func ValidateQuoteInputs(inputs QuoteInputs) error {
	if inputs.TenantID == "" {
		return fmt.Errorf("%w: tenant_id is required", ErrQuoteInputsInvalid)
	}
	if inputs.PropertyID == "" {
		return fmt.Errorf("%w: property_id is required", ErrQuoteInputsInvalid)
	}
	if !ValidServiceTier(inputs.ServiceTier) {
		return fmt.Errorf("%w: unknown service tier %q", ErrQuoteInputsInvalid, inputs.ServiceTier)
	}
	if len(inputs.Currency) != 3 {
		return fmt.Errorf("%w: currency must be ISO 4217", ErrQuoteInputsInvalid)
	}
	if inputs.RevenuePeriod == "" {
		return fmt.Errorf("%w: revenue_period is required", ErrQuoteInputsInvalid)
	}
	if inputs.ManagedUnits < 0 {
		return fmt.Errorf("%w: managed_units cannot be negative", ErrQuoteInputsInvalid)
	}
	if inputs.AccommodationRevenueMinorUnits < 0 {
		return fmt.Errorf("%w: accommodation revenue cannot be negative", ErrQuoteInputsInvalid)
	}
	for _, pt := range inputs.PassThroughs {
		if pt.Category == "" {
			return fmt.Errorf("%w: pass-through category is required", ErrQuoteInputsInvalid)
		}
		if pt.MinorUnits < 0 {
			return fmt.Errorf("%w: pass-through %q cannot be negative", ErrQuoteInputsInvalid, pt.Category)
		}
	}
	return nil
}

// ValidateFeeRule enforces the fee rule invariants. A fee rule without a rule
// version is refused because the quote result must always expose which rule
// version produced it.
func ValidateFeeRule(rule FeeRule) error {
	if rule.Version == "" {
		return fmt.Errorf("%w: rule version is required", ErrInvalidFeeRule)
	}
	if len(rule.Currency) != 3 {
		return fmt.Errorf("%w: currency must be ISO 4217", ErrInvalidFeeRule)
	}
	if rule.ServiceTier != "" && !ValidServiceTier(rule.ServiceTier) {
		return fmt.Errorf("%w: unknown service tier %q", ErrInvalidFeeRule, rule.ServiceTier)
	}
	if rule.PercentageBasisPoints < 0 || rule.PercentageBasisPoints > 10000 {
		return fmt.Errorf("%w: percentage basis points must be between 0 and 10000", ErrInvalidFeeRule)
	}
	if rule.MinimumMonthlyFeeMinorUnits < 0 {
		return fmt.Errorf("%w: minimum monthly fee cannot be negative", ErrInvalidFeeRule)
	}
	if rule.SetupFeeMinorUnits < 0 {
		return fmt.Errorf("%w: setup fee cannot be negative", ErrInvalidFeeRule)
	}
	return nil
}

// percentageOf computes floor(base * basisPoints / 10000) with exact integer
// arithmetic so the result is deterministic and never uses floating point.
func percentageOf(base, basisPoints int64) int64 {
	if base <= 0 || basisPoints <= 0 {
		return 0
	}
	q := base / 10000
	r := base % 10000
	return q*basisPoints + r*basisPoints/10000
}

// CalculateQuote computes the deterministic quote for the given inputs and fee
// rule. Identical inputs under the same rule version always produce an
// identical quote; a changed rule version produces a visibly different quote.
// The management fee is the rule percentage of the fee base (which excludes
// protected pass-throughs by default), floored at the rule minimum.
func CalculateQuote(inputs QuoteInputs, rule FeeRule) (*Quote, error) {
	if err := ValidateQuoteInputs(inputs); err != nil {
		return nil, err
	}
	if err := ValidateFeeRule(rule); err != nil {
		return nil, err
	}
	if rule.Currency != inputs.Currency {
		return nil, fmt.Errorf("%w: fee rule currency %s does not match inputs %s", ErrInvalidFeeRule, rule.Currency, inputs.Currency)
	}
	if rule.ServiceTier != "" && rule.ServiceTier != inputs.ServiceTier {
		return nil, fmt.Errorf("%w: fee rule tier %s does not match inputs %s", ErrInvalidFeeRule, rule.ServiceTier, inputs.ServiceTier)
	}

	feeBase, err := ComputeFeeBase(inputs)
	if err != nil {
		return nil, err
	}

	managementFee := percentageOf(feeBase.BaseMinorUnits, rule.PercentageBasisPoints)
	if managementFee < rule.MinimumMonthlyFeeMinorUnits {
		managementFee = rule.MinimumMonthlyFeeMinorUnits
	}

	inputHash, err := HashQuoteInputs(inputs, rule.Version)
	if err != nil {
		return nil, err
	}

	lineItems := []QuoteLineItem{
		{
			Code:       LineCodeManagementFee,
			Label:      "management fee on fee base",
			MinorUnits: managementFee,
			Recurring:  true,
		},
	}
	if rule.SetupFeeMinorUnits > 0 {
		lineItems = append(lineItems, QuoteLineItem{
			Code:       LineCodeSetupFee,
			Label:      "one-time setup fee",
			MinorUnits: rule.SetupFeeMinorUnits,
		})
	}

	return &Quote{
		InputHash:                   inputHash,
		RuleVersion:                 rule.Version,
		Currency:                    inputs.Currency,
		ServiceTier:                 inputs.ServiceTier,
		ManagedUnits:                inputs.ManagedUnits,
		FeeBase:                     feeBase,
		AppliedBasisPoints:          rule.PercentageBasisPoints,
		ManagementFeeMinorUnits:     managementFee,
		MinimumMonthlyFeeMinorUnits: rule.MinimumMonthlyFeeMinorUnits,
		SetupFeeMinorUnits:          rule.SetupFeeMinorUnits,
		EstimatedMonthlyMinorUnits:  managementFee,
		LineItems:                   lineItems,
	}, nil
}

// ParseContentHash returns the digest of a canonical "sha256:<hex>" content
// hash and reports whether the format is valid.
func ParseContentHash(hash string) (string, bool) {
	if !strings.HasPrefix(hash, "sha256:") {
		return "", false
	}
	digest := strings.TrimPrefix(hash, "sha256:")
	if len(digest) != 64 {
		return "", false
	}
	if _, err := hex.DecodeString(digest); err != nil {
		return "", false
	}
	return digest, true
}
