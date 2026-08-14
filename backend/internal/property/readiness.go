package property

import (
	"time"
)

const (
	BlockOwnerContractNotAccepted      = "owner_contract_not_accepted"
	BlockMandatoryComplianceIncomplete = "mandatory_compliance_incomplete"
	BlockMandatoryFieldsMissing        = "mandatory_fields_missing"
	BlockCriticalComplianceHold        = "critical_compliance_hold"
)

// ActivationBlock describes one reason an activation transition is refused.
type ActivationBlock struct {
	Code   string `json:"code"`
	HoldID string `json:"hold_id,omitempty"`
}

// EvaluateActivation runs the readiness gate for a transition into
// StateActive. It returns the blocked reasons; an empty list means the
// activation may proceed. This keeps ready_inactive and active distinct: a
// property can be fully ready and still inactive while a critical hold or a
// missing mandatory input stands.
func EvaluateActivation(p *Property, holds []ComplianceHold, now time.Time) []ActivationBlock {
	var blocks []ActivationBlock

	if !p.Readiness.OwnerContractAccepted {
		blocks = append(blocks, ActivationBlock{Code: BlockOwnerContractNotAccepted})
	}
	if !p.Readiness.ComplianceComplete {
		blocks = append(blocks, ActivationBlock{Code: BlockMandatoryComplianceIncomplete})
	}
	if !p.Readiness.MandatoryFieldsSet {
		blocks = append(blocks, ActivationBlock{Code: BlockMandatoryFieldsMissing})
	}

	for _, hold := range OpenCriticalHolds(holds, now) {
		blocks = append(blocks, ActivationBlock{Code: BlockCriticalComplianceHold, HoldID: hold.ID})
	}

	return blocks
}

// CanActivate returns nil only when the property may transition into
// StateActive. It is the enforcement point for PROP-002 (mandatory readiness)
// and PROP-004 (an expired critical permission raises a mandatory hold).
// A critical hold blocks activation with ErrComplianceHold; any other unmet
// mandatory condition blocks with ErrNotReady.
func CanActivate(p *Property, holds []ComplianceHold, now time.Time) error {
	for _, block := range EvaluateActivation(p, holds, now) {
		if block.Code == BlockCriticalComplianceHold {
			return ErrComplianceHold
		}
	}
	if !p.Readiness.Ready() {
		return ErrNotReady
	}
	return nil
}
