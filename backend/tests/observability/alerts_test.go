package observability_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"comfort-curators-backend/internal/observability"
)

func testCorr() observability.Correlation {
	c := observability.NewCorrelation()
	c.ID = "corr-alert-001"
	c.TraceID = "trace-alert-001"
	return c
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// TestPropertyReadinessAlertExpressesBusinessEffect verifies the readiness
// alert states the business impact (blocked bookable capacity) and never leaks
// internal signal details.
func TestPropertyReadinessAlertExpressesBusinessEffect(t *testing.T) {
	a := observability.PropertyReadinessAlert("tenant-1", "prop-42", 3, testCorr(), time.Now())

	if a.Kind != observability.AlertPropertyReadiness {
		t.Errorf("expected property_readiness kind, got %s", a.Kind)
	}
	if a.BusinessEffect == "" {
		t.Error("alert must express a business effect")
	}
	if a.BusinessEffect != "Property cannot be offered until 3 readiness blocker(s) clear" {
		t.Errorf("unexpected business effect: %q", a.BusinessEffect)
	}
	if a.CorrelationID != "corr-alert-001" {
		t.Errorf("alert must preserve correlation ID, got %s", a.CorrelationID)
	}
	if a.PropertyID != "prop-42" {
		t.Errorf("alert must carry the property reference, got %s", a.PropertyID)
	}
}

// TestAssignmentAlertExpressesBusinessEffect verifies the assignment alert
// states the delivery risk with crew counts only, never crew identities.
func TestAssignmentAlertExpressesBusinessEffect(t *testing.T) {
	a := observability.AssignmentAlert("tenant-1", "prop-7", "assign-901", 1, 3, testCorr(), time.Now())

	if a.Kind != observability.AlertAssignment {
		t.Errorf("expected assignment kind, got %s", a.Kind)
	}
	if a.Severity != observability.SeverityCritical {
		t.Errorf("assignment understaffing must be critical, got %s", a.Severity)
	}
	if a.BusinessEffect != "Assignment assign-901 at risk: only 1 of 3 required crew available" {
		t.Errorf("unexpected business effect: %q", a.BusinessEffect)
	}
	if _, ok := a.Details["crew_names"]; ok {
		t.Error("assignment alert must not expose crew identities")
	}
	if _, ok := a.Details["available_crew"]; !ok {
		t.Error("assignment alert should expose available crew count")
	}
}

// TestIncidentAlertExpressesBusinessEffect verifies the incident alert states
// the guest safety and response coverage impact.
func TestIncidentAlertExpressesBusinessEffect(t *testing.T) {
	a := observability.IncidentAlert("tenant-1", "prop-3", "incident-77", observability.SeverityCritical, testCorr(), time.Now())

	if a.Kind != observability.AlertIncident {
		t.Errorf("expected incident kind, got %s", a.Kind)
	}
	if a.Severity != observability.SeverityCritical {
		t.Errorf("expected critical severity, got %s", a.Severity)
	}
	if a.BusinessEffect != "Incident incident-77 unresolved; response coverage and guest safety at risk" {
		t.Errorf("unexpected business effect: %q", a.BusinessEffect)
	}
}

// TestStockAlertExpressesBusinessEffect verifies the stock alert states the
// stockout risk that could stop operations.
func TestStockAlertExpressesBusinessEffect(t *testing.T) {
	a := observability.StockAlert("tenant-1", "prop-5", "item-113", 2, 10, testCorr(), time.Now())

	if a.Kind != observability.AlertStock {
		t.Errorf("expected stock kind, got %s", a.Kind)
	}
	if a.BusinessEffect != "Stock for item item-113 below threshold (2 < 10); restock required to keep operations running" {
		t.Errorf("unexpected business effect: %q", a.BusinessEffect)
	}
}

// TestApprovalAlertExpressesBusinessEffect verifies the approval alert states
// the blocked-workflow impact of an approval exceeding its service level.
func TestApprovalAlertExpressesBusinessEffect(t *testing.T) {
	a := observability.ApprovalAlert("tenant-1", "approval-22", "two-person-review", 45, testCorr(), time.Now())

	if a.Kind != observability.AlertApproval {
		t.Errorf("expected approval kind, got %s", a.Kind)
	}
	if a.BusinessEffect != "Approval approval-22 pending 45 min beyond service level (two-person-review); workflow is blocked" {
		t.Errorf("unexpected business effect: %q", a.BusinessEffect)
	}
}

// TestAlertsRemainRedactedAndSensitiveFree pushes alerts through the service
// and verifies no sensitive content can leave and correlation survives.
func TestAlertsRemainRedactedAndSensitiveFree(t *testing.T) {
	svc := observability.NewAlertService()
	corr := testCorr()

	svc.Emit(observability.PropertyReadinessAlert("tenant-1", "prop-42", 3, corr, time.Now()))
	svc.Emit(observability.AssignmentAlert("tenant-1", "prop-7", "assign-901", 1, 3, corr, time.Now()))
	svc.Emit(observability.IncidentAlert("tenant-1", "prop-3", "incident-77", observability.SeverityCritical, corr, time.Now()))
	svc.Emit(observability.StockAlert("tenant-1", "prop-5", "item-113", 2, 10, corr, time.Now()))
	svc.Emit(observability.ApprovalAlert("tenant-1", "approval-22", "two-person-review", 45, corr, time.Now()))

	alerts := svc.Alerts()
	if len(alerts) != 5 {
		t.Fatalf("expected 5 recorded alerts, got %d", len(alerts))
	}

	for _, a := range alerts {
		if a.ID == "" {
			t.Errorf("alert %s must have an assigned ID", a.Kind)
		}
		if a.CorrelationID != corr.ID {
			t.Errorf("alert %s must preserve correlation ID, got %s", a.Kind, a.CorrelationID)
		}
		body := string(mustMarshal(t, a))
		for _, leak := range []string{"password", "token", "secret", "api_key", "authorization", "credential", "Bearer "} {
			if strings.Contains(body, leak) {
				t.Errorf("alert %s leaked sensitive content %q: %s", a.Kind, leak, body)
			}
		}
	}

	unresolved := svc.Unresolved(observability.AlertIncident, observability.AlertAssignment)
	if len(unresolved) != 2 {
		t.Errorf("expected 2 unresolved incident/assignment alerts, got %d", len(unresolved))
	}
}

// TestAlertDetailsAreRedacted verifies the service redacts sensitive detail
// keys while preserving business-relevant counters.
func TestAlertDetailsAreRedacted(t *testing.T) {
	svc := observability.NewAlertService()
	a := observability.PropertyReadinessAlert("tenant-1", "prop-42", 3, testCorr(), time.Now())
	a.Details["access_code"] = "guest-door-1234"
	a.Details["blocked_items"] = "3"

	emitted := svc.Emit(a)
	if emitted.Details["access_code"] != observability.RedactedValue {
		t.Errorf("access_code must be redacted, got %q", emitted.Details["access_code"])
	}
	if emitted.Details["blocked_items"] != "3" {
		t.Errorf("blocked_items must survive redaction, got %q", emitted.Details["blocked_items"])
	}
}
