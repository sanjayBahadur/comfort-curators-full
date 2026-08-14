package operations

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestSeverityConstantsAreDistinct(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range AllSeverities {
		if s == "" {
			t.Error("severity must not be empty")
		}
		if seen[s] {
			t.Errorf("duplicate severity: %s", s)
		}
		seen[s] = true
	}
	if len(AllSeverities) != 4 {
		t.Errorf("expected 4 severities, got %d", len(AllSeverities))
	}
}

func TestIsValidSeverity(t *testing.T) {
	for _, s := range AllSeverities {
		if !IsValidSeverity(s) {
			t.Errorf("expected %q to be valid", s)
		}
	}
	for _, s := range []string{"", "severe", "p0"} {
		if IsValidSeverity(s) {
			t.Errorf("expected %q to be invalid", s)
		}
	}
}

func TestValidateSeverity(t *testing.T) {
	if err := ValidateSeverity(""); !errors.Is(err, ErrSeverityRequired) {
		t.Fatalf("expected ErrSeverityRequired, got %v", err)
	}
	if err := ValidateSeverity("bogus"); !errors.Is(err, ErrInvalidSeverity) {
		t.Fatalf("expected ErrInvalidSeverity, got %v", err)
	}
	if err := ValidateSeverity(SeverityHigh); err != nil {
		t.Fatalf("high severity should validate: %v", err)
	}
}

func TestIsHighSeverity(t *testing.T) {
	if !IsHighSeverity(SeverityCritical) {
		t.Error("critical must be high severity")
	}
	if !IsHighSeverity(SeverityHigh) {
		t.Error("high must be high severity")
	}
	if IsHighSeverity(SeverityMedium) {
		t.Error("medium must not be high severity")
	}
	if IsHighSeverity(SeverityLow) {
		t.Error("low must not be high severity")
	}
}

func TestSeverityAlertTargets(t *testing.T) {
	if got := SeverityAlertTargets(SeverityHigh); !reflect.DeepEqual(got, []string{AlertTargetOnCall, AlertTargetOwner}) {
		t.Errorf("high severity must alert on_call and owner, got %v", got)
	}
	if got := SeverityAlertTargets(SeverityCritical); !reflect.DeepEqual(got, []string{AlertTargetOnCall, AlertTargetOwner}) {
		t.Errorf("critical severity must alert on_call and owner, got %v", got)
	}
	if got := SeverityAlertTargets(SeverityMedium); !reflect.DeepEqual(got, []string{AlertTargetOnCall}) {
		t.Errorf("medium severity must alert on_call only, got %v", got)
	}
	if got := SeverityAlertTargets(SeverityLow); len(got) != 0 {
		t.Errorf("low severity must queue no alert, got %v", got)
	}
}

func TestIncidentAlertPolicyIsStableAndAttributable(t *testing.T) {
	// Every severity maps to a stable response-matrix policy.
	if IncidentAlertPolicy(SeverityHigh) == "" {
		t.Error("high severity must carry a policy")
	}
	if !strings.Contains(IncidentAlertPolicy(SeverityHigh), "response_matrix") {
		t.Errorf("alert policy must name the response matrix, got %q", IncidentAlertPolicy(SeverityHigh))
	}
}

func TestNotificationIntentForSeverity(t *testing.T) {
	if got := NotificationIntentForSeverity(SeverityCritical); got != NotificationIntentUrgent {
		t.Errorf("critical severity must be urgent, got %q", got)
	}
	if got := NotificationIntentForSeverity(SeverityHigh); got != NotificationIntentUrgent {
		t.Errorf("high severity must be urgent, got %q", got)
	}
	if got := NotificationIntentForSeverity(SeverityMedium); got == NotificationIntentUrgent {
		t.Error("medium severity must not be urgent")
	}
}

// TestCCOPS001IncidentEscalates proves the named acceptance behavior: a
// high-severity incident queues on-call and owner alerts by policy, and the
// service recovery links the original incident.
func TestCCOPS001IncidentEscalates(t *testing.T) {
	targets := SeverityAlertTargets(SeverityHigh)
	if len(targets) != 2 {
		t.Fatalf("high severity must queue two targets, got %v", targets)
	}
	has := func(target string) bool {
		for _, x := range targets {
			if x == target {
				return true
			}
		}
		return false
	}
	if !has(AlertTargetOnCall) {
		t.Error("high severity incident must alert the on-call operations role")
	}
	if !has(AlertTargetOwner) {
		t.Error("high severity incident must alert the owner")
	}

	incident := &Ticket{ID: "tkt_inc_1", TenantID: "tenant-a", PropertyID: "prop-1",
		Type: TypeIncident, Status: StateInProgress, Severity: SeverityHigh, Reason: "burst pipe"}
	evidence := []EvidenceRecord{
		{ID: "ev_1", ContentHash: ComputeEvidenceHash([]byte("damage"))},
	}
	recovery := BuildRecovery(incident, evidence, RecoveryParams{
		Responsibility:  "ops-supervisor-1",
		ReworkCostMinor: 2500,
		Currency:        "INR",
	})
	if !PreservesOriginalFailure(recovery, incident) {
		t.Fatal("service recovery must preserve the original incident failure")
	}
}
