package operations

import (
	"fmt"
)

// BuildRecovery snapshots an incident's original failure (reason, severity and
// the content hashes of the evidence bound to the incident) into a service
// recovery record. The original failure is preserved on the record even as the
// follow-up recovery work progresses, so avoidable rework stays attributable.
func BuildRecovery(incident *Ticket, evidence []EvidenceRecord, params RecoveryParams) ServiceRecovery {
	hashes := make([]string, 0, len(evidence))
	for _, e := range evidence {
		hashes = append(hashes, e.ContentHash)
	}

	severity := incident.Severity
	if severity == "" {
		severity = SeverityLow
	}

	return ServiceRecovery{
		TenantID:               incident.TenantID,
		PropertyID:             incident.PropertyID,
		IncidentTicketID:       incident.ID,
		Severity:               severity,
		OriginalReason:         incident.Reason,
		OriginalEvidenceHashes: hashes,
		Responsibility:         params.Responsibility,
		ReworkCostMinor:        params.ReworkCostMinor,
		Currency:               params.Currency,
		Status:                 RecoveryStatusOpen,
	}
}

// PreservesOriginalFailure reports whether a recovery record still references
// the same incident and the same original failure reason that started it.
func PreservesOriginalFailure(r ServiceRecovery, incident *Ticket) bool {
	return r.IncidentTicketID == incident.ID && r.OriginalReason == incident.Reason
}

// ValidateRecoveryParams enforces the accountability fields of a service
// recovery: a recorded responsibility and a non-negative rework cost in
// integer minor units with an ISO currency when a cost is present.
func ValidateRecoveryParams(p RecoveryParams) error {
	if p.Responsibility == "" {
		return ErrResponsibilityRequired
	}
	if p.ReworkCostMinor < 0 {
		return ErrInvalidReworkCost
	}
	if p.ReworkCostMinor > 0 && p.Currency == "" {
		return ErrCurrencyRequired
	}
	if p.Currency != "" && len(p.Currency) != 3 {
		return fmt.Errorf("%w: %s", ErrInvalidReworkCost, p.Currency)
	}
	return nil
}
