package operations

import (
	"errors"
	"testing"
)

func TestBuildRecoveryPreservesOriginalFailure(t *testing.T) {
	incident := &Ticket{
		ID:         "tkt_inc_9",
		TenantID:   "tenant-a",
		PropertyID: "prop-1",
		Type:       TypeIncident,
		Status:     StateInProgress,
		Severity:   SeverityCritical,
		Reason:     "water heater flooded the bedroom",
	}
	evidence := []EvidenceRecord{
		{ID: "ev_1", ContentHash: ComputeEvidenceHash([]byte("flood-photo-1"))},
		{ID: "ev_2", ContentHash: ComputeEvidenceHash([]byte("flood-photo-2"))},
	}

	rec := BuildRecovery(incident, evidence, RecoveryParams{
		Responsibility:  "ops-supervisor-1",
		ReworkCostMinor: 7500,
		Currency:        "INR",
	})

	if rec.IncidentTicketID != incident.ID {
		t.Errorf("recovery must link the original incident, got %q", rec.IncidentTicketID)
	}
	if rec.PropertyID != incident.PropertyID {
		t.Errorf("recovery must carry the property scope, got %q", rec.PropertyID)
	}
	if rec.OriginalReason != incident.Reason {
		t.Errorf("recovery must preserve the original failure reason, got %q", rec.OriginalReason)
	}
	if rec.Severity != SeverityCritical {
		t.Errorf("recovery must preserve the original severity, got %q", rec.Severity)
	}
	if len(rec.OriginalEvidenceHashes) != 2 {
		t.Fatalf("recovery must preserve original evidence hashes, got %v", rec.OriginalEvidenceHashes)
	}
	for i, e := range evidence {
		if rec.OriginalEvidenceHashes[i] != e.ContentHash {
			t.Errorf("evidence hash %d not preserved: %s != %s", i, rec.OriginalEvidenceHashes[i], e.ContentHash)
		}
	}
	if rec.Responsibility != "ops-supervisor-1" {
		t.Errorf("responsibility not recorded: %q", rec.Responsibility)
	}
	if rec.ReworkCostMinor != 7500 || rec.Currency != "INR" {
		t.Errorf("rework cost not preserved: %d %s", rec.ReworkCostMinor, rec.Currency)
	}
	if rec.Status != RecoveryStatusOpen {
		t.Errorf("recovery must start open, got %q", rec.Status)
	}
}

func TestBuildRecoveryDefaultsSeverity(t *testing.T) {
	incident := &Ticket{ID: "tkt_inc_1", Reason: "unclassified failure"}
	rec := BuildRecovery(incident, nil, RecoveryParams{Responsibility: "ops"})
	if rec.Severity != SeverityLow {
		t.Errorf("unclassified incident recovery should default to low severity, got %q", rec.Severity)
	}
	if !PreservesOriginalFailure(rec, incident) {
		t.Error("recovery must preserve the original failure")
	}
}

func TestPreservesOriginalFailureLinksOnlyItsIncident(t *testing.T) {
	incident := &Ticket{ID: "tkt_inc_1", Reason: "burst pipe"}
	other := &Ticket{ID: "tkt_inc_2", Reason: "burst pipe"}
	rec := BuildRecovery(incident, nil, RecoveryParams{Responsibility: "ops"})

	if !PreservesOriginalFailure(rec, incident) {
		t.Error("recovery must preserve its own incident failure")
	}
	if PreservesOriginalFailure(rec, other) {
		t.Error("recovery must not be attributed to another incident")
	}
}

func TestValidateRecoveryParams(t *testing.T) {
	if err := ValidateRecoveryParams(RecoveryParams{}); !errors.Is(err, ErrResponsibilityRequired) {
		t.Fatalf("expected ErrResponsibilityRequired, got %v", err)
	}
	if err := ValidateRecoveryParams(RecoveryParams{Responsibility: "ops", ReworkCostMinor: -1}); !errors.Is(err, ErrInvalidReworkCost) {
		t.Fatalf("expected ErrInvalidReworkCost for negative cost, got %v", err)
	}
	if err := ValidateRecoveryParams(RecoveryParams{Responsibility: "ops", ReworkCostMinor: 100}); !errors.Is(err, ErrCurrencyRequired) {
		t.Fatalf("expected ErrCurrencyRequired when cost is present, got %v", err)
	}
	if err := ValidateRecoveryParams(RecoveryParams{Responsibility: "ops", ReworkCostMinor: 100, Currency: "INDIA"}); !errors.Is(err, ErrInvalidReworkCost) {
		t.Fatalf("expected ErrInvalidReworkCost for bad currency, got %v", err)
	}
	if err := ValidateRecoveryParams(RecoveryParams{Responsibility: "ops", ReworkCostMinor: 100, Currency: "INR"}); err != nil {
		t.Fatalf("valid recovery params rejected: %v", err)
	}
	if err := ValidateRecoveryParams(RecoveryParams{Responsibility: "ops", ReworkCostMinor: 0}); err != nil {
		t.Fatalf("zero rework cost without currency must validate: %v", err)
	}
}

// TestRecoveryPreservesOriginalFailureForReworkCost proves the rework cost of
// an incident failure is preserved on the recovery record, so downstream
// reporting can attribute avoidable rework without losing the original event.
func TestRecoveryPreservesOriginalFailureForReworkCost(t *testing.T) {
	incident := &Ticket{ID: "tkt_inc_1", TenantID: "tenant-a", PropertyID: "prop-1",
		Type: TypeIncident, Status: StateInProgress, Severity: SeverityHigh, Reason: "leak under sink"}

	first := BuildRecovery(incident, []EvidenceRecord{{ID: "ev_1", ContentHash: ComputeEvidenceHash([]byte("leak"))}},
		RecoveryParams{Responsibility: "ops", ReworkCostMinor: 5000, Currency: "INR"})
	if first.ReworkCostMinor != 5000 {
		t.Fatalf("rework cost not preserved, got %d", first.ReworkCostMinor)
	}

	// A second recovery attempt never destroys the first: each record is a
	// separate, append-only snapshot of the same original failure.
	second := BuildRecovery(incident, nil, RecoveryParams{Responsibility: "ops", ReworkCostMinor: 5000, Currency: "INR"})
	if second.ID == first.ID && second.ID != "" {
		t.Fatal("recoveries must be distinct records")
	}
	if !PreservesOriginalFailure(first, incident) || !PreservesOriginalFailure(second, incident) {
		t.Fatal("every recovery record must preserve the original failure")
	}
}
