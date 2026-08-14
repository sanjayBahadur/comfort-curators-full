package observability

import (
	"strconv"
	"sync"
	"time"
)

// Severity is the business severity of an alert.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// AlertKind is the business category an alert belongs to. The five alert kinds
// correspond to outcomes operators can act on: property readiness, assignment,
// incident, stock and approval.
type AlertKind string

const (
	AlertPropertyReadiness AlertKind = "property_readiness"
	AlertAssignment        AlertKind = "assignment"
	AlertIncident          AlertKind = "incident"
	AlertStock             AlertKind = "stock"
	AlertApproval          AlertKind = "approval"
)

// Alert is a business-effect alert. It never carries sensitive content: only
// opaque resource references, tenant scope, correlation identity and the
// business impact are surfaced.
type Alert struct {
	ID             string            `json:"id"`
	Kind           AlertKind         `json:"kind"`
	Severity       Severity          `json:"severity"`
	Title          string            `json:"title"`
	BusinessEffect string            `json:"business_effect"`
	CorrelationID  string            `json:"correlation_id"`
	TenantID       string            `json:"tenant_id,omitempty"`
	PropertyID     string            `json:"property_id,omitempty"`
	ResourceRef    string            `json:"resource_ref,omitempty"`
	OccurredAt     time.Time         `json:"occurred_at"`
	Details        map[string]string `json:"details,omitempty"`
}

// AlertService records business-effect alerts in memory and exposes them for
// dashboards and operators. It is safe for concurrent use.
type AlertService struct {
	mu     sync.Mutex
	seq    int64
	alerts []Alert
}

// NewAlertService returns an empty alert recorder.
func NewAlertService() *AlertService {
	return &AlertService{}
}

// Emit assigns an identifier and records the alert.
func (s *AlertService) Emit(a Alert) Alert {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	a.ID = "alt-" + newID()
	a.Details = RedactMap(a.Details)
	s.alerts = append(s.alerts, a)
	return a
}

// Alerts returns a copy of every recorded alert.
func (s *AlertService) Alerts() []Alert {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Alert, len(s.alerts))
	copy(out, s.alerts)
	return out
}

// Unresolved returns alerts belonging to the given kinds. With no kinds
// supplied every recorded alert is returned. Alerts are immutable and have no
// close transition in this bounded packet, so every recorded alert is
// unresolved; the filter expresses which business surface needs attention.
func (s *AlertService) Unresolved(kinds ...AlertKind) []Alert {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Alert
	for _, a := range s.alerts {
		if len(kinds) == 0 || containsKind(a.Kind, kinds) {
			out = append(out, a)
		}
	}
	return out
}

func containsKind(kind AlertKind, kinds []AlertKind) bool {
	for _, k := range kinds {
		if k == kind {
			return true
		}
	}
	return false
}

// PropertyReadinessAlert reports that a property cannot be offered because a
// business blocker is still open. The business effect is expressed as blocked
// bookable capacity, not as a raw database or queue signal.
func PropertyReadinessAlert(tenantID, propertyID string, blockedItems int, corr Correlation, ts time.Time) Alert {
	effect := "Property cannot be offered to guests until readiness blockers are cleared"
	title := "Property readiness blocked"
	if blockedItems > 0 {
		effect = "Property cannot be offered until " + redactCount(blockedItems) + " readiness blocker(s) clear"
	}
	return Alert{
		Kind:           AlertPropertyReadiness,
		Severity:       SeverityWarning,
		Title:          title,
		BusinessEffect: effect,
		CorrelationID:  corr.ID,
		TenantID:       tenantID,
		PropertyID:     propertyID,
		ResourceRef:    propertyID,
		OccurredAt:     ts,
		Details: map[string]string{
			"blocked_items": redactCount(blockedItems),
		},
	}
}

// AssignmentAlert reports that a guest work assignment cannot be fulfilled
// with the available crew. The business effect is expressed as unfulfillable
// service, never as crew identities or schedules.
func AssignmentAlert(tenantID, propertyID, assignmentRef string, availableCrew, requiredCrew int, corr Correlation, ts time.Time) Alert {
	effect := "Assignment " + assignmentRef + " cannot be fulfilled by the available crew"
	if availableCrew < requiredCrew {
		effect = "Assignment " + assignmentRef + " at risk: only " + redactCount(availableCrew) + " of " + redactCount(requiredCrew) + " required crew available"
	}
	return Alert{
		Kind:           AlertAssignment,
		Severity:       SeverityCritical,
		Title:          "Assignment cannot be fulfilled",
		BusinessEffect: effect,
		CorrelationID:  corr.ID,
		TenantID:       tenantID,
		PropertyID:     propertyID,
		ResourceRef:    assignmentRef,
		OccurredAt:     ts,
		Details: map[string]string{
			"available_crew": redactCount(availableCrew),
			"required_crew":  redactCount(requiredCrew),
		},
	}
}

// IncidentAlert reports an unresolved incident whose response target is at
// risk. The business effect is expressed as response coverage at risk.
func IncidentAlert(tenantID, propertyID, incidentRef string, severity Severity, corr Correlation, ts time.Time) Alert {
	return Alert{
		Kind:           AlertIncident,
		Severity:       severity,
		Title:          "Incident response at risk",
		BusinessEffect: "Incident " + incidentRef + " unresolved; response coverage and guest safety at risk",
		CorrelationID:  corr.ID,
		TenantID:       tenantID,
		PropertyID:     propertyID,
		ResourceRef:    incidentRef,
		OccurredAt:     ts,
		Details: map[string]string{
			"incident_ref": incidentRef,
		},
	}
}

// StockAlert reports an inventory item below its replenishment threshold. The
// business effect is expressed as a stockout risk that could stop operations.
func StockAlert(tenantID, propertyID, itemRef string, onHand, threshold int, corr Correlation, ts time.Time) Alert {
	effect := "Stock for item " + itemRef + " below threshold"
	if onHand < threshold {
		effect = "Stock for item " + itemRef + " below threshold (" + redactCount(onHand) + " < " + redactCount(threshold) + "); restock required to keep operations running"
	}
	return Alert{
		Kind:           AlertStock,
		Severity:       SeverityWarning,
		Title:          "Stock below threshold",
		BusinessEffect: effect,
		CorrelationID:  corr.ID,
		TenantID:       tenantID,
		PropertyID:     propertyID,
		ResourceRef:    itemRef,
		OccurredAt:     ts,
		Details: map[string]string{
			"item_ref":  itemRef,
			"on_hand":   redactCount(onHand),
			"threshold": redactCount(threshold),
		},
	}
}

// ApprovalAlert reports an approval that has been pending beyond its service
// level. The business effect is expressed as a blocked workflow.
func ApprovalAlert(tenantID, approvalRef, policy string, pendingMinutes int, corr Correlation, ts time.Time) Alert {
	effect := "Approval " + approvalRef + " pending " + redactCount(pendingMinutes) + " min beyond service level (" + policy + "); workflow is blocked"
	return Alert{
		Kind:           AlertApproval,
		Severity:       SeverityWarning,
		Title:          "Approval pending beyond SLA",
		BusinessEffect: effect,
		CorrelationID:  corr.ID,
		TenantID:       tenantID,
		ResourceRef:    approvalRef,
		OccurredAt:     ts,
		Details: map[string]string{
			"approval_ref":    approvalRef,
			"policy":          policy,
			"pending_minutes": redactCount(pendingMinutes),
		},
	}
}

func redactCount(n int) string {
	if n < 0 {
		return "0"
	}
	return strconv.Itoa(n)
}
