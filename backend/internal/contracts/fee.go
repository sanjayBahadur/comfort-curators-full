package contracts

import "fmt"

// ProtectedPassThrough reports whether category is one of the frozen protected
// pass-through categories that are excluded from the percentage-fee base by
// default under FIN-002.
func ProtectedPassThrough(category string) bool {
	for _, c := range ProtectedPassThroughCategories {
		if c == category {
			return true
		}
	}
	return false
}

// ComputeFeeBase derives the percentage-fee base from accommodation revenue.
// Protected pass-throughs (taxes, refundable deposits, pass-through cleaning)
// are excluded by default so Comfort Curators never earns commission on
// reimbursed costs. An owner contract may explicitly opt in to include a
// protected category through QuoteInputs.IncludedPassThroughs; the exclusion
// then follows the owner contract instead of the default.
func ComputeFeeBase(inputs QuoteInputs) (FeeBase, error) {
	included := make(map[string]bool, len(inputs.IncludedPassThroughs))
	for _, category := range inputs.IncludedPassThroughs {
		included[category] = true
	}

	base := FeeBase{AccommodationRevenueMinorUnits: inputs.AccommodationRevenueMinorUnits}

	excluded := int64(0)
	for _, passThrough := range inputs.PassThroughs {
		if passThrough.MinorUnits <= 0 {
			continue
		}
		if !ProtectedPassThrough(passThrough.Category) {
			continue
		}
		if included[passThrough.Category] {
			continue
		}
		excluded += passThrough.MinorUnits
		base.Exclusions = append(base.Exclusions, FeeBaseExclusion{
			Category:   passThrough.Category,
			MinorUnits: passThrough.MinorUnits,
		})
	}

	if excluded > base.AccommodationRevenueMinorUnits {
		return FeeBase{}, fmt.Errorf("%w: excluded pass-throughs %d exceed revenue %d", ErrQuoteInputsInvalid, excluded, base.AccommodationRevenueMinorUnits)
	}

	base.ExcludedMinorUnits = excluded
	base.BaseMinorUnits = base.AccommodationRevenueMinorUnits - excluded
	return base, nil
}
