package compliance

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"comfort-curators-backend/internal/platform/audit"
	"comfort-curators-backend/internal/platform/database"
	"comfort-curators-backend/internal/property"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ResourceAuthorizer interface {
	RequireResourceAccess(ctx context.Context, resourceTenantID, resourceType, resourceID string) error
}

type ComplianceService struct {
	pool       *pgxpool.Pool
	store      *ComplianceStore
	auditStore *audit.AuditStore
	authorizer ResourceAuthorizer
	propGetter propertyGetter
}

type propertyGetter interface {
	Get(ctx context.Context, tenantID, propertyID string) (*property.Property, error)
	InsertComplianceHold(ctx context.Context, q pgx.Tx, hold *property.ComplianceHold) error
}

type propStoreAdapter struct {
	*property.PropertyStore
}

func NewComplianceService(pool *pgxpool.Pool, auditStore *audit.AuditStore) *ComplianceService {
	ps := property.NewPropertyStore(pool)
	return &ComplianceService{
		pool:       pool,
		store:      NewComplianceStore(pool),
		auditStore: auditStore,
		propGetter: propStoreAdapter{ps},
	}
}

func (s *ComplianceService) WithAuthorizer(a ResourceAuthorizer) *ComplianceService {
	s.authorizer = a
	return s
}

func (s *ComplianceService) CreateItem(ctx context.Context, itemParams ComplianceItemParams, tenantID, actorID string) (*ComplianceItem, error) {
	if err := s.authorizeProperty(ctx, tenantID, itemParams.PropertyID); err != nil {
		return nil, err
	}

	item, err := NewComplianceItem(itemParams.PropertyID, tenantID, itemParams, time.Now().UTC())
	if err != nil {
		return nil, err
	}

	if _, err := s.propGetter.Get(ctx, tenantID, itemParams.PropertyID); err != nil {
		return nil, err
	}

	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertItem(ctx, tx, item); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "compliance.item.create",
			ResourceType: "compliance_item",
			ResourceID:   item.ID,
		})
	}); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *ComplianceService) GetItem(ctx context.Context, tenantID, itemID string) (*ComplianceItem, error) {
	item, err := s.store.GetItem(ctx, tenantID, itemID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeProperty(ctx, tenantID, item.PropertyID); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *ComplianceService) ListItems(ctx context.Context, tenantID, propertyID string) ([]ComplianceItem, error) {
	if err := s.authorizeProperty(ctx, tenantID, propertyID); err != nil {
		return nil, err
	}
	return s.store.ListItems(ctx, tenantID, propertyID)
}

func (s *ComplianceService) RenewItem(ctx context.Context, itemID string, newExpiryDate time.Time, evidenceIDs []string, tenantID, actorID string) (*ComplianceItem, error) {
	item, err := s.store.GetItem(ctx, tenantID, itemID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeProperty(ctx, tenantID, item.PropertyID); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	if err := item.Renew(newExpiryDate, now); err != nil {
		return nil, err
	}
	item.EvidenceIDs = evidenceIDs
	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.UpdateItem(ctx, tx, item); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     item.TenantID,
			ActorID:      actorID,
			Action:       "compliance.item.renew",
			ResourceType: "compliance_item",
			ResourceID:   itemID,
		})
	}); err != nil {
		return nil, err
	}

	if item.HoldID != nil && *item.HoldID != "" {
		s.resolveHoldDirect(ctx, *item.HoldID, item.PropertyID, item.TenantID)
	}

	return item, nil
}

func (s *ComplianceService) ScanExpired(ctx context.Context, actorID string) (*ScanExpiryResult, error) {
	now := time.Now().UTC()
	result := &ScanExpiryResult{}

	items, err := s.store.ListScannableItems(ctx, now)
	if err != nil {
		return nil, fmt.Errorf("scan expired items: %w", err)
	}
	result.Scanned = len(items)

	for _, item := range items {
		if item.IsExpired(now) {
			if err := item.Expire(now); err == nil {
				_ = s.store.UpdateItem(ctx, s.pool, &item)
				result.Expired++
			}
		}

		p, err := s.propGetter.Get(ctx, item.TenantID, item.PropertyID)
		if err != nil {
			continue
		}

		alreadyHeld := false
		for _, h := range p.ComplianceHolds {
			if h.Kind == item.Kind && h.Status == property.HoldStatusOpen && h.Severity == property.HoldSeverityCritical {
				alreadyHeld = true
				if item.HoldID == nil || *item.HoldID != h.ID {
					item.HoldID = &h.ID
					_ = s.store.UpdateItem(ctx, s.pool, &item)
				}
				break
			}
		}

		if item.IsExpired(now) && item.Severity == ItemSeverityCritical && !alreadyHeld {
			hold, err := s.createCriticalHold(ctx, item.PropertyID, item.TenantID, item.Kind, actorID)
			if err != nil {
				continue
			}
			item.HoldID = &hold.ID
			_ = item.Expire(now)
			_ = s.store.UpdateItem(ctx, s.pool, &item)
			result.HoldsCreated++
		} else if alreadyHeld && item.IsExpired(now) {
			result.HoldsMaintained++
		}

		if !item.IsExpired(now) && item.Severity == ItemSeverityCritical {
			warningWindows := []int{30, 14, 7}
			for _, days := range warningWindows {
				if item.IsWithinWarningWindow(now, days) {
					hasWarning, _ := s.store.HasActiveWarningForItemInWindow(ctx, item.ID, days)
					if !hasWarning {
						warning := &ComplianceRenewalWarning{
							ItemID:           item.ID,
							PropertyID:       item.PropertyID,
							TenantID:         item.TenantID,
							DaysBeforeExpiry: days,
							IssuedAt:         now,
							CreatedAt:        now,
						}
						if err := s.store.InsertRenewalWarning(ctx, warning); err == nil {
							result.WarningsIssued++
						}
					}
				}
			}
		}
	}

	return result, nil
}

func (s *ComplianceService) createCriticalHold(ctx context.Context, propertyID, tenantID, kind, actorID string) (*property.ComplianceHold, error) {
	params := property.ComplianceHoldParams{
		Kind:     kind,
		Severity: property.HoldSeverityCritical,
		Reason:   fmt.Sprintf("expired %s compliance item", kind),
	}

	hold, err := property.NewComplianceHold(propertyID, tenantID, params, time.Now().UTC())
	if err != nil {
		return nil, err
	}

	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.propGetter.InsertComplianceHold(ctx, tx, hold); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeSecurity,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "compliance.hold.auto_create",
			ResourceType: "property",
			ResourceID:   propertyID,
			Metadata:     mustJSON(map[string]string{"hold_id": hold.ID, "kind": kind}),
		})
	}); err != nil {
		return nil, err
	}
	return hold, nil
}

func (s *ComplianceService) resolveHoldDirect(ctx context.Context, holdID, propertyID, tenantID string) {
	_, _ = s.pool.Exec(ctx, `
		UPDATE property_compliance_holds
		SET status = $4, resolved_at = NOW()
		WHERE id = $1 AND property_id = $2 AND tenant_id = $3 AND status = 'open'
	`, holdID, propertyID, tenantID, property.HoldStatusResolved)
}

func (s *ComplianceService) ListRenewalWarnings(ctx context.Context, tenantID, propertyID string) ([]ComplianceRenewalWarning, error) {
	if err := s.authorizeProperty(ctx, tenantID, propertyID); err != nil {
		return nil, err
	}
	return s.store.ListRenewalWarnings(ctx, tenantID, propertyID)
}

func (s *ComplianceService) AcknowledgeWarning(ctx context.Context, warningID string) error {
	return s.store.AcknowledgeWarning(ctx, warningID)
}

func (s *ComplianceService) authorizeProperty(ctx context.Context, tenantID, propertyID string) error {
	if s.authorizer == nil {
		return nil
	}
	if err := s.authorizer.RequireResourceAccess(ctx, tenantID, "property", propertyID); err != nil {
		return err
	}
	return nil
}

func (s *ComplianceService) appendAudit(ctx context.Context, evt audit.AuditEvent) {
	if s.auditStore == nil {
		return
	}
	if evt.ID == "" {
		evt.ID = newID("aud")
	}
	if err := s.auditStore.Append(ctx, evt); err != nil {
		return
	}
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}
