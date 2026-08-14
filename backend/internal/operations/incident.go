package operations

import (
	"fmt"
)

// IsValidSeverity reports whether s is a supported incident severity.
func IsValidSeverity(s string) bool {
	switch s {
	case SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow:
		return true
	}
	return false
}

// IsHighSeverity reports whether an incident severity requires immediate
// on-call and owner escalation.
func IsHighSeverity(s string) bool {
	return s == SeverityCritical || s == SeverityHigh
}

// SeverityAlertTargets returns the roles that must be queued an alert for a
// given severity under the defined response matrix. High-severity incidents
// alert both the on-call operations role and the owner; medium incidents
// alert only the on-call role; low severity queues no alert.
func SeverityAlertTargets(severity string) []string {
	switch severity {
	case SeverityCritical, SeverityHigh:
		return []string{AlertTargetOnCall, AlertTargetOwner}
	case SeverityMedium:
		return []string{AlertTargetOnCall}
	default:
		return nil
	}
}

// IncidentAlertPolicy returns the stable response-matrix rule that governs a
// severity so alert queue entries are attributable to a defined policy.
func IncidentAlertPolicy(severity string) string {
	switch severity {
	case SeverityCritical:
		return "response_matrix.critical:on_call+owner immediate"
	case SeverityHigh:
		return "response_matrix.high:on_call+owner immediate"
	case SeverityMedium:
		return "response_matrix.medium:on_call notify"
	default:
		return "response_matrix.low:no_alert"
	}
}

// NotificationIntentForSeverity derives the ticket notification intent from
// the classified severity so downstream communication respects quiet hours
// and urgency policy.
func NotificationIntentForSeverity(severity string) string {
	if IsHighSeverity(severity) {
		return NotificationIntentUrgent
	}
	return NotificationIntentNormal
}

// ValidateSeverity wraps ErrInvalidSeverity for stable HTTP mapping.
func ValidateSeverity(severity string) error {
	if severity == "" {
		return ErrSeverityRequired
	}
	if !IsValidSeverity(severity) {
		return fmt.Errorf("%w: %s", ErrInvalidSeverity, severity)
	}
	return nil
}
