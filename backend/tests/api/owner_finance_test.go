package api_test

import (
	"encoding/json"
	"testing"
	"time"

	"comfort-curators-backend/internal/api"
	"comfort-curators-backend/internal/billing"
	"comfort-curators-backend/internal/platform/security"
)

func financePropertyAuthority(propertyID string) string {
	switch propertyID {
	case "prop_0000000000000001", "prop_0000000000000003":
		return "auth-owner-1"
	case "prop_0000000000000002":
		return "auth-owner-2"
	}
	return ""
}

func financeOwnerSubject(actorID string) security.Subject {
	return security.Subject{ActorID: actorID, TenantID: "tenant-a", Roles: []string{api.RoleOwner}}
}

func chargePropertyID(c billing.Charge) string { return c.PropertyID }

func sampleCharges() []billing.Charge {
	return []billing.Charge{
		{ID: "chg_0000000000000001", TenantID: "tenant-a", PropertyID: "prop_0000000000000001"},
		{ID: "chg_0000000000000002", TenantID: "tenant-a", PropertyID: "prop_0000000000000002"},
		{ID: "chg_0000000000000003", TenantID: "tenant-a", PropertyID: "prop_0000000000000001"},
	}
}

func TestOwnerCannotAccessAnotherOwnersFinanceRecords(t *testing.T) {
	owner1 := financeOwnerSubject("actor-owner-1")

	got := api.FilterOwnedRecords(owner1, sampleCharges(), chargePropertyID, financePropertyAuthority, ownerAuthorityResolver)
	if len(got) != 2 {
		t.Fatalf("owner-1 must see exactly 2 records, got %d", len(got))
	}
	for _, c := range got {
		if c.PropertyID == "prop_0000000000000002" {
			t.Errorf("owner-1 received a finance record for another owner's property %s", c.PropertyID)
		}
	}

	if api.OwnerControlsPropertyScope(owner1, "prop_0000000000000002", financePropertyAuthority, ownerAuthorityResolver) {
		t.Error("OwnerControlsPropertyScope must deny another owner's property")
	}
	if !api.OwnerControlsPropertyScope(owner1, "prop_0000000000000001", financePropertyAuthority, ownerAuthorityResolver) {
		t.Error("OwnerControlsPropertyScope must allow an owned property")
	}
}

func TestOwnerFinanceGuardNeverWidensInput(t *testing.T) {
	owner2 := financeOwnerSubject("actor-owner-2")

	got := api.FilterOwnedRecords(owner2, sampleCharges(), chargePropertyID, financePropertyAuthority, ownerAuthorityResolver)
	if len(got) != 1 {
		t.Fatalf("owner-2 must see exactly 1 record, got %d", len(got))
	}
	if got[0].PropertyID != "prop_0000000000000002" {
		t.Errorf("owner-2 must only see prop_0000000000000002, got %s", got[0].PropertyID)
	}
}

func TestOwnerFinanceGuardFailsClosedWithoutMappings(t *testing.T) {
	owner1 := financeOwnerSubject("actor-owner-1")

	if api.OwnerControlsPropertyScope(owner1, "prop_0000000000000001", nil, ownerAuthorityResolver) {
		t.Error("nil authority mapping must deny")
	}
	if api.OwnerControlsPropertyScope(owner1, "prop_0000000000000001", financePropertyAuthority, nil) {
		t.Error("nil resolver must deny")
	}

	got := api.FilterOwnedRecords(owner1, sampleCharges(), chargePropertyID, financePropertyAuthority, nil)
	if len(got) != 0 {
		t.Errorf("owner finance guard must fail closed with nil resolver, got %d records", len(got))
	}

	got = api.FilterOwnedRecords(owner1, sampleCharges(), chargePropertyID, nil, ownerAuthorityResolver)
	if len(got) != 0 {
		t.Errorf("owner finance guard must fail closed with nil authority mapping, got %d records", len(got))
	}
}

func TestNonOwnerKeepsTenantScopedFinanceSet(t *testing.T) {
	charges := sampleCharges()
	for _, subject := range []security.Subject{
		{ActorID: "actor-staff", TenantID: "tenant-a", Roles: []string{"staff"}},
		{ActorID: "actor-guest", TenantID: "tenant-a", Roles: []string{"guest"}},
		{ActorID: "actor-none", TenantID: "tenant-a", Roles: nil},
	} {
		got := api.FilterOwnedRecords(subject, charges, chargePropertyID, financePropertyAuthority, ownerAuthorityResolver)
		if len(got) != len(charges) {
			t.Errorf("roles %v: tenant-scoped finance set must be unchanged, got %d of %d",
				subject.Roles, len(got), len(charges))
		}
		if api.OwnerControlsPropertyScope(subject, "prop_0000000000000001", financePropertyAuthority, ownerAuthorityResolver) != true {
			t.Errorf("roles %v: OwnerControlsPropertyScope must defer to tenant scoping", subject.Roles)
		}
	}
}

func TestOwnerFinanceSliceNeverExposesWorkerHR(t *testing.T) {
	charge := billing.Charge{
		ID:               "chg_0000000000000001",
		TenantID:         "tenant-a",
		PropertyID:       "prop_0000000000000001",
		ChargeType:       billing.ChargeTypeManagementFee,
		AmountMinorUnits: 60000000,
		Currency:         "INR",
		ContractRuleID:   "crl_0000000000000001",
		Status:           billing.ChargeStatusApplied,
		Version:          1,
		CreatedAt:        time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC),
	}

	chargePayload, err := json.Marshal(api.ChargeTraceabilityView(charge))
	if err != nil {
		t.Fatalf("marshal charge traceability view: %v", err)
	}
	if err := api.WorkerHRNotExposed(chargePayload); err != nil {
		t.Errorf("charge traceability view leaked worker HR material: %v", err)
	}

	invoice := billing.Invoice{
		ID:              "inv_0000000000000001",
		TenantID:        "tenant-a",
		PropertyID:      "prop_0000000000000001",
		TotalMinorUnits: 60000000,
		Currency:        "INR",
		Status:          billing.InvoiceStatusIssued,
		Version:         1,
	}
	lines := []billing.InvoiceLine{{
		ID:               "lin_0000000000000001",
		ChargeType:       billing.ChargeTypeManagementFee,
		AmountMinorUnits: 60000000,
		ContractRuleID:   "crl_0000000000000001",
	}}
	invoicePayload, err := json.Marshal(api.InvoiceTraceabilityView(invoice, lines))
	if err != nil {
		t.Fatalf("marshal invoice traceability view: %v", err)
	}
	if err := api.WorkerHRNotExposed(invoicePayload); err != nil {
		t.Errorf("invoice traceability view leaked worker HR material: %v", err)
	}

	// A workforce-shaped payload must be rejected by the guard.
	hrPayload := []byte(`{"legal_name":"Asha","date_of_birth":"1995-01-01","contact_method":"phone","compensation_band":"band_c"}`)
	if err := api.WorkerHRNotExposed(hrPayload); err == nil {
		t.Error("WorkerHRNotExposed must reject workforce HR material")
	}
}

func TestFinancialEvidenceIsTraceable(t *testing.T) {
	charge := billing.Charge{
		ID:               "chg_0000000000000001",
		TenantID:         "tenant-a",
		PropertyID:       "prop_0000000000000001",
		ChargeType:       billing.ChargeTypeTaskService,
		AmountMinorUnits: 15000,
		Currency:         "INR",
		ContractRuleID:   "crl_0000000000000001",
		EvidenceID:       "evd_0000000000000001",
		TicketID:         "tkt_0000000000000001",
		OrderID:          "ord_0000000000000001",
		ApprovalID:       "apr_0000000000000001",
		Status:           billing.ChargeStatusApplied,
		Version:          2,
		CreatedAt:        time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC),
	}

	links := api.ChargeEvidenceLinks(charge)
	if len(links) != 5 {
		t.Fatalf("expected 5 evidence links, got %d: %+v", len(links), links)
	}
	want := map[string]string{
		"contract_rule": "crl_0000000000000001",
		"evidence":      "evd_0000000000000001",
		"ticket":        "tkt_0000000000000001",
		"order":         "ord_0000000000000001",
		"approval":      "apr_0000000000000001",
	}
	for _, l := range links {
		if want[l.Kind] != l.ID {
			t.Errorf("evidence link %s: expected id %s, got %s", l.Kind, want[l.Kind], l.ID)
		}
	}

	view := api.ChargeTraceabilityView(charge)
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal charge traceability view: %v", err)
	}
	for _, kind := range []string{"contract_rule", "evidence", "ticket", "order", "approval"} {
		if !jsonHasKey(encoded, kind) {
			t.Errorf("charge traceability view must expose evidence link kind %q", kind)
		}
	}
}

func TestFinancialCorrectionPreservesOriginalEntry(t *testing.T) {
	credit := billing.Credit{
		ID:                "crd_0000000000000001",
		TenantID:          "tenant-a",
		PropertyID:        "prop_0000000000000001",
		CreditType:        billing.CreditTypeCreditNote,
		AmountMinorUnits:  60000000,
		Currency:          "INR",
		OriginalEntryID:   "chg_0000000000000001",
		OriginalEntryType: billing.SubledgerEntryTypeCharge,
		Status:            billing.CreditStatusIssued,
		Version:           1,
	}

	trace := api.CreditTrace(credit)
	if trace.Kind != billing.SubledgerEntryTypeCharge || trace.ID != "chg_0000000000000001" {
		t.Errorf("credit trace must link back to the preserved original entry, got %+v", trace)
	}
}

func TestSubledgerEntryTracesBackToSource(t *testing.T) {
	entry := billing.OperationalSubledgerEntry{
		ID:               "sub_0000000000000001",
		TenantID:         "tenant-a",
		PropertyID:       "prop_0000000000000001",
		EntryType:        billing.SubledgerEntryTypeCharge,
		AmountMinorUnits: 15000,
		Currency:         "INR",
		ReferenceType:    "charge",
		ReferenceID:      "chg_0000000000000001",
	}

	trace := api.SubledgerTrace(entry)
	if trace.Kind != "charge" || trace.ID != "chg_0000000000000001" {
		t.Errorf("subledger trace must link to the source financial record, got %+v", trace)
	}
}

func TestInvoiceLineEvidenceLinks(t *testing.T) {
	line := billing.InvoiceLine{
		ID:               "lin_0000000000000001",
		ChargeType:       billing.ChargeTypePurchasedGoods,
		AmountMinorUnits: 25000,
		ContractRuleID:   "crl_0000000000000001",
		TicketID:         "tkt_0000000000000001",
		OrderID:          "ord_0000000000000001",
		AdjustmentID:     "adj_0000000000000001",
	}

	links := api.InvoiceLineEvidenceLinks(line)
	if len(links) != 4 {
		t.Fatalf("expected 4 invoice line evidence links, got %d: %+v", len(links), links)
	}
	want := map[string]string{
		"contract_rule":       "crl_0000000000000001",
		"ticket":              "tkt_0000000000000001",
		"order":               "ord_0000000000000001",
		"approved_adjustment": "adj_0000000000000001",
	}
	for _, l := range links {
		if want[l.Kind] != l.ID {
			t.Errorf("invoice line link %s: expected id %s, got %s", l.Kind, want[l.Kind], l.ID)
		}
	}
}

func jsonHasKey(payload []byte, key string) bool {
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return false
	}
	links, ok := decoded["evidence_links"].([]any)
	if !ok {
		return false
	}
	for _, raw := range links {
		link, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if kind, _ := link["kind"].(string); kind == key {
			return true
		}
	}
	return false
}
