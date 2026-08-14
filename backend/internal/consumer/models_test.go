package consumer

import (
	"errors"
	"testing"
)

func int64Ptr(v int64) *int64 { return &v }

func TestValidRecurrence(t *testing.T) {
	for _, r := range []string{RecurrenceOneTime, RecurrenceWeekly, RecurrenceMonthly, RecurrenceAnnual} {
		if !ValidRecurrence(r) {
			t.Fatalf("recurrence %q must be valid", r)
		}
	}
	if ValidRecurrence("") || ValidRecurrence("daily") || ValidRecurrence("monthly!") {
		t.Fatal("unknown or empty recurrences must be invalid")
	}
}

func TestValidResourceType(t *testing.T) {
	for _, rt := range []string{ResourceTypePackage, ResourceTypeOrder, ResourceTypeService} {
		if !ValidResourceType(rt) {
			t.Fatalf("resource type %q must be valid", rt)
		}
	}
	if ValidResourceType("") || ValidResourceType("subscription") {
		t.Fatal("unknown or empty resource types must be invalid")
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

func TestValidateDisclosureParamsRequiresExplicitRecurringCost(t *testing.T) {
	base := DisclosureParams{
		ResourceType:    ResourceTypePackage,
		ResourceID:      "pkg-1",
		PriceMinorUnits: 25000,
		Currency:        "INR",
		Recurrence:      RecurrenceMonthly,
	}

	// A recurring disclosure without an explicit recurring cost is a hidden
	// recurring charge (CON-004).
	if err := ValidateDisclosureParams(base); !errors.Is(err, ErrHiddenRecurringCost) {
		t.Fatalf("recurring disclosure without recurring cost must be rejected with ErrHiddenRecurringCost, got %v", err)
	}

	withCost := base
	withCost.RecurrenceAmountMinorUnits = int64Ptr(1500)
	if err := ValidateDisclosureParams(withCost); err != nil {
		t.Fatalf("recurring disclosure with explicit recurring cost must pass, got %v", err)
	}

	negative := base
	negative.RecurrenceAmountMinorUnits = int64Ptr(-1)
	if err := ValidateDisclosureParams(negative); !errors.Is(err, ErrInvalidDisclosure) {
		t.Fatalf("negative recurring cost must be rejected, got %v", err)
	}
}

func TestValidateDisclosureParamsOneTimeRejectsRecurringAmount(t *testing.T) {
	p := DisclosureParams{
		ResourceType:               ResourceTypeService,
		ResourceID:                 "svc-1",
		PriceMinorUnits:            5000,
		Currency:                   "INR",
		Recurrence:                 RecurrenceOneTime,
		RecurrenceAmountMinorUnits: int64Ptr(1000),
	}
	if err := ValidateDisclosureParams(p); !errors.Is(err, ErrInvalidDisclosure) {
		t.Fatalf("one-time disclosure with a recurring amount must be rejected, got %v", err)
	}

	p.RecurrenceAmountMinorUnits = nil
	if err := ValidateDisclosureParams(p); err != nil {
		t.Fatalf("valid one-time disclosure must pass, got %v", err)
	}
}

func TestValidateDisclosureParamsStructuralValidation(t *testing.T) {
	p := DisclosureParams{
		ResourceType:    ResourceTypePackage,
		ResourceID:      "pkg-1",
		PriceMinorUnits: 1000,
		Currency:        "INR",
		Recurrence:      RecurrenceOneTime,
	}

	if err := ValidateDisclosureParams(p); err != nil {
		t.Fatalf("valid disclosure must pass, got %v", err)
	}

	missingResource := p
	missingResource.ResourceID = ""
	if err := ValidateDisclosureParams(missingResource); !errors.Is(err, ErrInvalidDisclosure) {
		t.Fatalf("missing resource_id must be rejected, got %v", err)
	}

	badType := p
	badType.ResourceType = "subscription"
	if err := ValidateDisclosureParams(badType); !errors.Is(err, ErrInvalidResourceType) {
		t.Fatalf("unknown resource type must be rejected, got %v", err)
	}

	badCurrency := p
	badCurrency.Currency = "inr"
	if err := ValidateDisclosureParams(badCurrency); !errors.Is(err, ErrInvalidCurrency) {
		t.Fatalf("invalid currency must be rejected, got %v", err)
	}

	negativePrice := p
	negativePrice.PriceMinorUnits = -1
	if err := ValidateDisclosureParams(negativePrice); !errors.Is(err, ErrInvalidDisclosure) {
		t.Fatalf("negative price must be rejected, got %v", err)
	}

	badRecurrence := p
	badRecurrence.Recurrence = "daily"
	if err := ValidateDisclosureParams(badRecurrence); !errors.Is(err, ErrInvalidRecurrence) {
		t.Fatalf("unknown recurrence must be rejected, got %v", err)
	}
}

func TestDisclosureIsAcceptableRequiresVisibleRecurringCost(t *testing.T) {
	if err := DisclosureIsAcceptable(nil); !errors.Is(err, ErrNoDisclosureBeforeAccept) {
		t.Fatalf("nil disclosure must not be acceptable, got %v", err)
	}

	hidden := &Disclosure{
		Recurrence:           RecurrenceMonthly,
		RecurringCostVisible: false,
	}
	if err := DisclosureIsAcceptable(hidden); !errors.Is(err, ErrRecurringCostNotVisible) {
		t.Fatalf("disclosure with hidden recurring cost must not be acceptable, got %v", err)
	}

	recurringWithoutAmount := &Disclosure{
		Recurrence:           RecurrenceMonthly,
		RecurringCostVisible: true,
	}
	if err := DisclosureIsAcceptable(recurringWithoutAmount); !errors.Is(err, ErrHiddenRecurringCost) {
		t.Fatalf("recurring disclosure without an amount must not be acceptable, got %v", err)
	}

	oneTime := &Disclosure{
		Recurrence:           RecurrenceOneTime,
		RecurringCostVisible: true,
	}
	if err := DisclosureIsAcceptable(oneTime); err != nil {
		t.Fatalf("one-time disclosure with visible price must be acceptable, got %v", err)
	}

	recurring := &Disclosure{
		Recurrence:                 RecurrenceMonthly,
		RecurrenceAmountMinorUnits: int64Ptr(1500),
		RecurringCostVisible:       true,
	}
	if err := DisclosureIsAcceptable(recurring); err != nil {
		t.Fatalf("recurring disclosure with visible recurring cost must be acceptable, got %v", err)
	}
}

func TestDisclosureRecurringCostAccessors(t *testing.T) {
	d := &Disclosure{Recurrence: RecurrenceOneTime}
	if d.HasRecurringCharge() {
		t.Fatal("one_time disclosure must not have a recurring charge")
	}
	if d.RecurringCost() != 0 {
		t.Fatalf("one_time disclosure recurring cost must be zero, got %d", d.RecurringCost())
	}

	recurring := &Disclosure{
		Recurrence:                 RecurrenceMonthly,
		RecurrenceAmountMinorUnits: int64Ptr(4200),
	}
	if !recurring.HasRecurringCharge() {
		t.Fatal("monthly disclosure must have a recurring charge")
	}
	if recurring.RecurringCost() != 4200 {
		t.Fatalf("monthly disclosure recurring cost must be 4200, got %d", recurring.RecurringCost())
	}
}
