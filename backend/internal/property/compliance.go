package property

import (
	"fmt"
	"time"
)

// ValidHoldKind reports whether kind is a supported compliance kind.
func ValidHoldKind(kind string) bool {
	for _, k := range ValidHoldKinds {
		if k == kind {
			return true
		}
	}
	return false
}

// ValidHoldSeverity reports whether severity is critical or non-critical.
func ValidHoldSeverity(severity string) bool {
	return severity == HoldSeverityCritical || severity == HoldSeverityNonCritical
}

// NewComplianceHold constructs an open compliance hold for a property. Kind,
// severity and reason are mandatory; an optional hard expiry is preserved so
// an expired critical permission (PROP-004) keeps blocking activation.
func NewComplianceHold(propertyID, tenantID string, params ComplianceHoldParams, createdAt time.Time) (*ComplianceHold, error) {
	if propertyID == "" || tenantID == "" {
		return nil, ErrInvalidComplianceHold
	}
	if !ValidHoldKind(params.Kind) {
		return nil, fmt.Errorf("%w: unknown kind %q", ErrInvalidComplianceHold, params.Kind)
	}
	if !ValidHoldSeverity(params.Severity) {
		return nil, fmt.Errorf("%w: unknown severity %q", ErrInvalidComplianceHold, params.Severity)
	}
	if params.Reason == "" {
		return nil, fmt.Errorf("%w: reason is required", ErrInvalidComplianceHold)
	}
	return &ComplianceHold{
		PropertyID: propertyID,
		TenantID:   tenantID,
		Kind:       params.Kind,
		Severity:   params.Severity,
		Status:     HoldStatusOpen,
		Reason:     params.Reason,
		ExpiresAt:  params.ExpiresAt,
		CreatedAt:  createdAt,
	}, nil
}

// OpenCriticalHolds returns every hold that blocks activation at now, sorted
// by creation time. A hold blocks activation when it is critical, unresolved
// (open or excepted with an expired exception), and its optional hard expiry
// has not passed.
func OpenCriticalHolds(holds []ComplianceHold, now time.Time) []ComplianceHold {
	var open []ComplianceHold
	for _, h := range holds {
		if h.BlocksActivation(now) {
			open = append(open, h)
		}
	}
	return open
}

// HasValidException reports whether the hold carries a reviewer-attributed,
// time-bounded exception that is still in effect at now.
func (h *ComplianceHold) HasValidException(now time.Time) bool {
	if h.Status != HoldStatusExcepted {
		return false
	}
	if h.ExceptionBy == "" || h.ExceptionAt == nil {
		return false
	}
	if h.ExceptionExpiresAt != nil && now.After(*h.ExceptionExpiresAt) {
		return false
	}
	return true
}

// BlocksActivation reports whether an open critical hold prevents activation
// at now, accounting for any valid reviewer exception. A hold remains in force
// until it is resolved by an operator or covered by a valid reviewer
// exception; the underlying item's ExpiresAt is preserved for expiry tracking
// (PROP-003) and does not silently release the hold.
func (h *ComplianceHold) BlocksActivation(now time.Time) bool {
	if h.Severity != HoldSeverityCritical {
		return false
	}
	if h.Status == HoldStatusResolved {
		return false
	}
	if h.HasValidException(now) {
		return false
	}
	return true
}

// GrantException marks the hold as excepted by a compliance reviewer. The
// exception is explicit (reviewer identity) and time-bounded; it cannot be
// granted after the hold has been resolved.
func (h *ComplianceHold) GrantException(reviewerID string, expiresAt time.Time, at time.Time) error {
	if h.Status != HoldStatusOpen {
		return ErrHoldNotOpen
	}
	if reviewerID == "" {
		return ErrExceptionDenied
	}
	if expiresAt.Before(at) {
		return fmt.Errorf("%w: exception must expire in the future", ErrExceptionDenied)
	}
	h.Status = HoldStatusExcepted
	h.ExceptionBy = reviewerID
	h.ExceptionAt = &at
	exp := expiresAt
	h.ExceptionExpiresAt = &exp
	return nil
}

// Resolve closes the hold. A resolved hold no longer blocks activation.
func (h *ComplianceHold) Resolve(at time.Time) error {
	if h.Status != HoldStatusOpen {
		return ErrHoldNotOpen
	}
	h.Status = HoldStatusResolved
	h.ResolvedAt = &at
	return nil
}
