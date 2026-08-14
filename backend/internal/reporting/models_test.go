package reporting

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestClassifyTicketExceptionOwnerVisible(t *testing.T) {
	class := ClassifyTicketException("incident", "in_progress")
	if !class.OwnerVisible {
		t.Fatal("an active incident must be owner-visible")
	}
	if class.Label != ExceptionSourceIncident {
		t.Fatalf("expected label incident, got %s", class.Label)
	}
}

func TestClassifyTicketExceptionTerminalIsHidden(t *testing.T) {
	for _, status := range []string{"closed", "cancelled"} {
		class := ClassifyTicketException("incident", status)
		if class.OwnerVisible {
			t.Fatalf("incident in terminal state %s must not be owner-visible", status)
		}
	}
}

func TestClassifyTicketExceptionInternalNoise(t *testing.T) {
	// Routine operational work must never be promoted to the owner exception
	// feed. These are the ticket types that are internal noise for an owner.
	for _, ticketType := range []string{
		"turnover", "pre_arrival_inspection", "restock", "inventory_count",
		"document_review", "routine_maintenance", "property_onboarding",
		"specialist_vendor_request",
	} {
		class := ClassifyTicketException(ticketType, "in_progress")
		if class.OwnerVisible {
			t.Fatalf("ticket type %s is internal noise and must not be owner-visible", ticketType)
		}
	}
}

func TestClassifyRecoveryAndFinancialExceptions(t *testing.T) {
	if !ClassifyRecoveryException("open").OwnerVisible {
		t.Fatal("an open service recovery must be owner-visible")
	}
	if ClassifyRecoveryException("closed").OwnerVisible {
		t.Fatal("a closed service recovery must be hidden")
	}
	if !ClassifyFinancialException("open").OwnerVisible {
		t.Fatal("an open financial exception must be owner-visible")
	}
	if ClassifyFinancialException("resolved").OwnerVisible {
		t.Fatal("a resolved financial exception must be hidden")
	}
}

func TestAggregateContribution(t *testing.T) {
	charges := []ChargeSourceRow{
		{ID: "c1", ChargeType: "management_fee", Status: "applied", AmountMinorUnits: 10000, Currency: "INR"},
		{ID: "c2", ChargeType: "task_service", Status: "applied", AmountMinorUnits: 5000, Currency: "INR"},
		{ID: "c3", ChargeType: "purchased_goods", Status: "applied", AmountMinorUnits: 2000, Currency: "INR"},
		{ID: "c4", ChargeType: "rebate", Status: "applied", AmountMinorUnits: 300, Currency: "INR"},
		{ID: "c5", ChargeType: "vendor_fee", Status: "applied", AmountMinorUnits: 800, Currency: "INR"},
		{ID: "c6", ChargeType: "refund", Status: "applied", AmountMinorUnits: 400, Currency: "INR"},
		{ID: "c7", ChargeType: "tax", Status: "applied", AmountMinorUnits: 1200, Currency: "INR"},
		{ID: "c8", ChargeType: "management_fee", Status: "corrected", AmountMinorUnits: 99999, Currency: "INR"},
	}
	credits := []CreditSourceRow{
		{ID: "k1", CreditType: "refund", Status: "issued", AmountMinorUnits: 250, Currency: "INR"},
		{ID: "k2", CreditType: "credit_note", Status: "issued", AmountMinorUnits: 100, Currency: "INR"},
		{ID: "k3", CreditType: "refund", Status: "corrected", AmountMinorUnits: 99999, Currency: "INR"},
	}
	recoveries := []RecoverySourceRow{
		{ID: "r1", Status: "open", ReworkCostMinor: 700, Currency: "INR"},
		{ID: "r2", Status: "closed", ReworkCostMinor: 99999, Currency: "INR"},
	}

	pc := AggregateContribution(charges, credits, recoveries)

	// revenue = 15000 (management fee + task service) minus the 100 credit
	// note issued against it; the corrected/pending rows never count.
	if pc.RevenueMinorUnits != 14900 {
		t.Fatalf("expected revenue 14900, got %d", pc.RevenueMinorUnits)
	}
	if pc.SupplyMarginMinorUnits != 1700 {
		t.Fatalf("expected supply margin 1700, got %d", pc.SupplyMarginMinorUnits)
	}
	if pc.VendorCostMinorUnits != 800 {
		t.Fatalf("expected vendor cost 800, got %d", pc.VendorCostMinorUnits)
	}
	if pc.RefundMinorUnits != 650 {
		t.Fatalf("expected refund 650, got %d", pc.RefundMinorUnits)
	}
	if pc.ExceptionCostMinorUnits != 700 {
		t.Fatalf("expected exception cost 700, got %d", pc.ExceptionCostMinorUnits)
	}
	if pc.TaxMinorUnits != 1200 {
		t.Fatalf("expected tax 1200, got %d", pc.TaxMinorUnits)
	}
	// net = 14900 + 1700 - 800 - 650 - 0 (discount) - 700 = 14450
	if pc.NetContributionMinorUnits != 14450 {
		t.Fatalf("expected net contribution 14450, got %d", pc.NetContributionMinorUnits)
	}
}

func TestAggregateContributionIgnoresCorrectedAndPending(t *testing.T) {
	charges := []ChargeSourceRow{
		{ID: "c1", ChargeType: "management_fee", Status: "applied", AmountMinorUnits: 1000},
		{ID: "c2", ChargeType: "management_fee", Status: "corrected", AmountMinorUnits: 9000},
		{ID: "c3", ChargeType: "management_fee", Status: "pending", AmountMinorUnits: 9000},
	}
	pc := AggregateContribution(charges, nil, nil)
	if pc.RevenueMinorUnits != 1000 {
		t.Fatalf("only applied charges count: expected 1000, got %d", pc.RevenueMinorUnits)
	}
}

func TestContributionSourceHashDeterministic(t *testing.T) {
	charges := []ChargeSourceRow{
		{ID: "c1", ChargeType: "management_fee", Status: "applied", AmountMinorUnits: 1000, Currency: "INR"},
	}
	hashA := encodeContributionHash(charges, nil, nil)
	hashB := encodeContributionHash(charges, nil, nil)
	if hashA != hashB {
		t.Fatalf("same source rows must hash identically, got %s vs %s", hashA, hashB)
	}

	changed := []ChargeSourceRow{
		{ID: "c1", ChargeType: "management_fee", Status: "applied", AmountMinorUnits: 1001, Currency: "INR"},
	}
	hashC := encodeContributionHash(changed, nil, nil)
	if hashA == hashC {
		t.Fatal("a changed source row must change the source hash")
	}
}

func TestValidMetricKind(t *testing.T) {
	if !ValidMetricKind("turnover_time_minutes") {
		t.Fatal("turnover_time_minutes must be a valid metric kind")
	}
	if ValidMetricKind("performance_score") {
		t.Fatal("a score-like metric kind must not exist: worker metrics are development facts")
	}
}

func TestGuardMetricsNonDisciplinary(t *testing.T) {
	obs := []MetricObservation{
		{ID: "m1", WorkerID: "w1", MetricKind: "turnover_time_minutes", Value: 90},
		{ID: "m2", WorkerID: "w1", MetricKind: "resolution_time_minutes", Value: 120},
	}
	if err := GuardMetricsNonDisciplinary(obs); err != nil {
		t.Fatalf("plain development metrics must pass the guard, got %v", err)
	}
	for _, o := range obs {
		if o.HasRank() {
			t.Fatalf("observation %s must never carry a rank", o.ID)
		}
		if o.CanDiscipline() {
			t.Fatalf("observation %s must never be discipline-bound", o.ID)
		}
	}
}

func TestMetricObservationJSONExposesNoRankOrDiscipline(t *testing.T) {
	obs := MetricObservation{
		ID:         "m1",
		WorkerID:   "w1",
		MetricKind: "turnover_time_minutes",
		Value:      90,
		Unit:       "minutes",
		SourceRef:  "ticket/t-1",
	}
	raw, err := json.Marshal(obs)
	if err != nil {
		t.Fatalf("marshal metric observation: %v", err)
	}
	body := string(raw)
	for _, forbidden := range []string{"rank", "discipline", "leaderboard", "score"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("metric observation JSON must not expose %q, got %s", forbidden, body)
		}
	}
}

func TestPeriodValidation(t *testing.T) {
	if p := (*Period)(nil); p.Validate() != nil {
		t.Fatal("a nil period must be valid (unbounded)")
	}

	now := time.Now().UTC()
	bad := &Period{Start: now.Add(2 * time.Hour), End: now}
	if bad.Validate() == nil {
		t.Fatal("a period whose end precedes start must be invalid")
	}

	good := &Period{Start: now, End: now.Add(2 * time.Hour)}
	if good.Validate() != nil {
		t.Fatal("a well-formed period must be valid")
	}
}
