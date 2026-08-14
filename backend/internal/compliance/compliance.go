package compliance

import (
	"fmt"
	"time"
)

func ValidItemKind(kind string) bool {
	for _, k := range ValidItemKinds {
		if k == kind {
			return true
		}
	}
	return false
}

func ValidItemSeverity(severity string) bool {
	return severity == ItemSeverityCritical || severity == ItemSeverityNonCritical
}

func NewComplianceItem(propertyID, tenantID string, params ComplianceItemParams, now time.Time) (*ComplianceItem, error) {
	if propertyID == "" || tenantID == "" {
		return nil, fmt.Errorf("%w: property_id and tenant_id are required", ErrInvalidComplianceItem)
	}
	if !ValidItemKind(params.Kind) {
		return nil, fmt.Errorf("%w: unknown kind %q", ErrInvalidComplianceItem, params.Kind)
	}
	if !ValidItemSeverity(params.Severity) {
		return nil, fmt.Errorf("%w: unknown severity %q", ErrInvalidComplianceItem, params.Severity)
	}
	if params.Name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidComplianceItem)
	}
	if params.ExpiryDate.Before(params.EffectiveDate) {
		return nil, fmt.Errorf("%w: expiry_date must be after effective_date", ErrInvalidComplianceItem)
	}
	item := &ComplianceItem{
		PropertyID:    propertyID,
		TenantID:      tenantID,
		Kind:          params.Kind,
		Severity:      params.Severity,
		Name:          params.Name,
		Description:   params.Description,
		EffectiveDate: params.EffectiveDate,
		ExpiryDate:    params.ExpiryDate,
		Status:        ItemStatusActive,
		EvidenceIDs:   params.EvidenceIDs,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if item.EvidenceIDs == nil {
		item.EvidenceIDs = []string{}
	}
	return item, nil
}

func (item *ComplianceItem) IsExpired(now time.Time) bool {
	return now.After(item.ExpiryDate)
}

func (item *ComplianceItem) DaysUntilExpiry(now time.Time) int {
	if item.IsExpired(now) {
		return 0
	}
	duration := item.ExpiryDate.Sub(now)
	return int(duration.Hours() / 24)
}

func (item *ComplianceItem) IsWithinWarningWindow(now time.Time, daysBeforeExpiry int) bool {
	if item.IsExpired(now) {
		return false
	}
	return item.DaysUntilExpiry(now) <= daysBeforeExpiry
}

func (item *ComplianceItem) Expire(now time.Time) error {
	if item.Status != ItemStatusActive {
		return ErrItemNotActive
	}
	item.Status = ItemStatusExpired
	item.UpdatedAt = now
	return nil
}

func (item *ComplianceItem) Renew(newExpiryDate time.Time, now time.Time) error {
	if item.Status != ItemStatusActive && item.Status != ItemStatusExpired {
		return fmt.Errorf("%w: cannot renew item in status %q", ErrItemNotActive, item.Status)
	}
	item.Status = ItemStatusRenewed
	item.UpdatedAt = now
	return nil
}

func (item *ComplianceItem) Revoke(now time.Time) error {
	if item.Status == ItemStatusRevoked {
		return fmt.Errorf("%w: item is already revoked", ErrItemNotActive)
	}
	item.Status = ItemStatusRevoked
	item.UpdatedAt = now
	return nil
}
