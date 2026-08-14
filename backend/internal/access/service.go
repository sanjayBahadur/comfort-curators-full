package access

import (
	"context"
	"fmt"
	"time"

	"comfort-curators-backend/internal/platform/audit"
	"comfort-curators-backend/internal/platform/database"
	"comfort-curators-backend/internal/platform/logging"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	pool       *pgxpool.Pool
	store      *AccessStore
	auditStore *audit.AuditStore
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{
		pool:       pool,
		store:      NewAccessStore(pool),
		auditStore: audit.NewAuditStore(pool),
	}
}

func (s *Service) WithAudit(a *audit.AuditStore) *Service {
	s.auditStore = a
	return s
}

func (s *Service) appendAudit(ctx context.Context, event audit.AuditEvent) {
	if s.auditStore == nil {
		return
	}
	if event.ID == "" {
		event.ID = newID("aud")
	}
	if err := s.auditStore.Append(ctx, event); err != nil {
		logging.Error(ctx, "failed to append audit event", "error", err)
	}
}

func (s *Service) recordCustodyEvent(ctx context.Context, q querier, ce *AccessCustodyEvent) error {
	return s.store.InsertCustodyEvent(ctx, q, ce)
}

func (s *Service) StoreSecret(ctx context.Context, tenantID, propertyID string, params CreateSecretParams, actorID string) (*PropertyAccessSecret, error) {
	if params.SecretType == "" || !ValidSecretType(params.SecretType) {
		return nil, fmt.Errorf("%w: invalid secret type", ErrInvalidSecret)
	}
	if params.EncryptedValue == "" {
		return nil, fmt.Errorf("%w: encrypted value is required", ErrInvalidSecret)
	}

	sec := &PropertyAccessSecret{
		TenantID:        tenantID,
		PropertyID:      propertyID,
		SecretType:      params.SecretType,
		Label:           params.Label,
		EncryptedValue:  params.EncryptedValue,
		EncryptionKeyID: params.EncryptionKeyID,
		Metadata:        params.Metadata,
	}

	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertSecret(ctx, tx, sec); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeAccess,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "access.secret.stored",
			ResourceType: "property_access_secret",
			ResourceID:   sec.ID,
			NewState:     marshalJSON(sec),
		})
	})
	if err != nil {
		return nil, err
	}

	return sec, nil
}

func (s *Service) GetSecret(ctx context.Context, tenantID, secretID string) (*PropertyAccessSecret, error) {
	return s.store.GetSecret(ctx, tenantID, secretID)
}

func (s *Service) ListSecrets(ctx context.Context, tenantID, propertyID string) ([]PropertyAccessSecret, error) {
	return s.store.ListSecrets(ctx, tenantID, propertyID)
}

func (s *Service) CreateGrant(ctx context.Context, tenantID, propertyID, secretID string, params CreateGrantParams, actorID string) (*AccessGrant, error) {
	if params.GranteeID == "" {
		return nil, fmt.Errorf("%w: grantee is required", ErrInvalidGrant)
	}
	if params.WindowEnd.Before(params.WindowStart) || params.WindowEnd.Equal(params.WindowStart) {
		return nil, fmt.Errorf("%w: window_end must be after window_start", ErrInvalidWindow)
	}

	if _, err := s.store.GetSecret(ctx, tenantID, secretID); err != nil {
		return nil, err
	}

	held, err := s.store.HasActiveHold(ctx, tenantID, propertyID)
	if err != nil {
		return nil, err
	}
	if held {
		return nil, ErrAccessHeld
	}

	grant := &AccessGrant{
		TenantID:    tenantID,
		PropertyID:  propertyID,
		SecretID:    secretID,
		GranteeID:   params.GranteeID,
		GranterID:   actorID,
		WindowStart: params.WindowStart,
		WindowEnd:   params.WindowEnd,
		Reason:      params.Reason,
		Status:      GrantStatusActive,
	}

	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertGrant(ctx, tx, grant); err != nil {
			return err
		}
		if err := s.recordCustodyEvent(ctx, tx, &AccessCustodyEvent{
			TenantID:   tenantID,
			PropertyID: propertyID,
			GrantID:    grant.ID,
			SecretID:   secretID,
			EventType:  CustodyEventTypeIssued,
			ActorID:    actorID,
			GranteeID:  params.GranteeID,
			Reason:     params.Reason,
		}); err != nil {
			return err
		}
		if err := s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeAccess,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "access.grant.created",
			ResourceType: "access_grant",
			ResourceID:   grant.ID,
			NewState:     marshalJSON(grant),
		}); err != nil {
			return fmt.Errorf("audit: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return grant, nil
}

func (s *Service) GetGrant(ctx context.Context, tenantID, grantID string) (*AccessGrant, error) {
	return s.store.GetGrant(ctx, tenantID, grantID)
}

func (s *Service) ListGrants(ctx context.Context, tenantID, propertyID string) ([]AccessGrant, error) {
	return s.store.ListGrants(ctx, tenantID, propertyID)
}

func (s *Service) DiscloseSecret(ctx context.Context, tenantID, grantID string, requestorID string, now time.Time) (*PropertyAccessSecret, *AccessDisclosure, error) {
	grant, err := s.store.GetGrant(ctx, tenantID, grantID)
	if err != nil {
		return nil, nil, err
	}

	if !grant.IsActive() {
		disclosure := &AccessDisclosure{
			GrantID:      grantID,
			TenantID:     tenantID,
			PropertyID:   grant.PropertyID,
			SecretID:     grant.SecretID,
			RequestorID:  requestorID,
			Result:       DisclosureResultRevoked,
			DenialReason: "grant is " + grant.Status,
			DisclosedAt:  now,
		}
		s.store.InsertDisclosure(ctx, s.pool, disclosure)
		return nil, disclosure, ErrGrantNotActive
	}

	if grant.GranteeID != requestorID {
		disclosure := &AccessDisclosure{
			GrantID:      grantID,
			TenantID:     tenantID,
			PropertyID:   grant.PropertyID,
			SecretID:     grant.SecretID,
			RequestorID:  requestorID,
			Result:       DisclosureResultDenied,
			DenialReason: "requestor is not the grantee",
			DisclosedAt:  now,
		}
		s.store.InsertDisclosure(ctx, s.pool, disclosure)
		return nil, disclosure, ErrUnauthorized
	}

	if !grant.IsWithinWindow(now) {
		disclosure := &AccessDisclosure{
			GrantID:      grantID,
			TenantID:     tenantID,
			PropertyID:   grant.PropertyID,
			SecretID:     grant.SecretID,
			RequestorID:  requestorID,
			Result:       DisclosureResultOutOfWindow,
			DenialReason: fmt.Sprintf("current time %s is outside window [%s, %s)", now.Format(time.RFC3339), grant.WindowStart.Format(time.RFC3339), grant.WindowEnd.Format(time.RFC3339)),
			DisclosedAt:  now,
		}
		s.store.InsertDisclosure(ctx, s.pool, disclosure)
		return nil, disclosure, ErrGrantWindowMismatch
	}

	held, err := s.store.HasActiveHold(ctx, tenantID, grant.PropertyID)
	if err != nil {
		return nil, nil, err
	}
	if held {
		disclosure := &AccessDisclosure{
			GrantID:      grantID,
			TenantID:     tenantID,
			PropertyID:   grant.PropertyID,
			SecretID:     grant.SecretID,
			RequestorID:  requestorID,
			Result:       DisclosureResultHeld,
			DenialReason: "property access is held",
			DisclosedAt:  now,
		}
		s.store.InsertDisclosure(ctx, s.pool, disclosure)
		return nil, disclosure, ErrAccessHeld
	}

	var sec *PropertyAccessSecret
	var disclosure *AccessDisclosure
	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		sec, err = s.store.GetSecret(ctx, tenantID, grant.SecretID)
		if err != nil {
			return err
		}

		disclosure = &AccessDisclosure{
			GrantID:     grantID,
			TenantID:    tenantID,
			PropertyID:  grant.PropertyID,
			SecretID:    grant.SecretID,
			RequestorID: requestorID,
			Result:      DisclosureResultSuccess,
			DisclosedAt: now,
		}
		if err := s.store.InsertDisclosure(ctx, tx, disclosure); err != nil {
			return err
		}

		if err := s.recordCustodyEvent(ctx, tx, &AccessCustodyEvent{
			TenantID:   tenantID,
			PropertyID: grant.PropertyID,
			GrantID:    grantID,
			SecretID:   grant.SecretID,
			EventType:  CustodyEventTypeDisclosed,
			ActorID:    requestorID,
			GranteeID:  grant.GranteeID,
		}); err != nil {
			return err
		}

		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeAccess,
			TenantID:     tenantID,
			ActorID:      requestorID,
			Action:       "access.secret.disclosed",
			ResourceType: "access_grant",
			ResourceID:   grantID,
			NewState:     marshalJSON(disclosure),
		})
	})
	if err != nil {
		return nil, nil, err
	}

	return sec, disclosure, nil
}

func (s *Service) AcknowledgeAccess(ctx context.Context, tenantID, grantID, actorID string) (*AccessGrant, error) {
	var grant *AccessGrant
	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		grant, err = s.store.AcknowledgeGrant(ctx, tx, tenantID, grantID)
		if err != nil {
			return err
		}
		if err := s.recordCustodyEvent(ctx, tx, &AccessCustodyEvent{
			TenantID:   tenantID,
			PropertyID: grant.PropertyID,
			GrantID:    grantID,
			EventType:  CustodyEventTypeAcknowledged,
			ActorID:    actorID,
			GranteeID:  grant.GranteeID,
		}); err != nil {
			return err
		}

		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeAccess,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "access.grant.acknowledged",
			ResourceType: "access_grant",
			ResourceID:   grantID,
			NewState:     marshalJSON(grant),
		})
	})
	if err != nil {
		return nil, err
	}

	return grant, nil
}

func (s *Service) ReturnAccess(ctx context.Context, tenantID, grantID, actorID string) (*AccessGrant, error) {
	var grant *AccessGrant
	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		grant, err = s.store.ReturnGrant(ctx, tx, tenantID, grantID)
		if err != nil {
			return err
		}
		if err := s.recordCustodyEvent(ctx, tx, &AccessCustodyEvent{
			TenantID:   tenantID,
			PropertyID: grant.PropertyID,
			GrantID:    grantID,
			EventType:  CustodyEventTypeReturned,
			ActorID:    actorID,
			GranteeID:  grant.GranteeID,
		}); err != nil {
			return err
		}

		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeAccess,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "access.grant.returned",
			ResourceType: "access_grant",
			ResourceID:   grantID,
			NewState:     marshalJSON(grant),
		})
	})
	if err != nil {
		return nil, err
	}

	return grant, nil
}

func (s *Service) RevokeGrant(ctx context.Context, tenantID, grantID, actorID, reason string) (*AccessGrant, error) {
	var grant *AccessGrant
	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		grant, err = s.store.RevokeGrant(ctx, tx, tenantID, grantID, actorID, reason)
		if err != nil {
			return err
		}
		if err := s.recordCustodyEvent(ctx, tx, &AccessCustodyEvent{
			TenantID:   tenantID,
			PropertyID: grant.PropertyID,
			GrantID:    grantID,
			EventType:  CustodyEventTypeRevoked,
			ActorID:    actorID,
			GranteeID:  grant.GranteeID,
			Reason:     reason,
		}); err != nil {
			return err
		}

		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeAccess,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "access.grant.revoked",
			ResourceType: "access_grant",
			ResourceID:   grantID,
			NewState:     marshalJSON(grant),
		})
	})
	if err != nil {
		return nil, err
	}

	return grant, nil
}

func (s *Service) EmergencyAccess(ctx context.Context, tenantID, propertyID string, params EmergencyAccessParams, actorID string) (*AccessGrant, *PropertyAccessSecret, error) {
	if params.Reason == "" {
		return nil, nil, fmt.Errorf("%w: reason is required", ErrInvalidEmergency)
	}

	secrets, err := s.store.ListSecrets(ctx, tenantID, propertyID)
	if err != nil {
		return nil, nil, err
	}
	if len(secrets) == 0 {
		return nil, nil, fmt.Errorf("%w: no access secrets found for property", ErrSecretNotFound)
	}
	secret := &secrets[0]

	now := time.Now().UTC()
	windowStart := params.WindowStart
	if windowStart.IsZero() {
		windowStart = now
	}
	windowEnd := params.WindowEnd
	if windowEnd.IsZero() {
		windowEnd = now.Add(1 * time.Hour)
	}

	grant := &AccessGrant{
		TenantID:        tenantID,
		PropertyID:      propertyID,
		SecretID:        secret.ID,
		GranteeID:       actorID,
		GranterID:       actorID,
		WindowStart:     windowStart,
		WindowEnd:       windowEnd,
		Reason:          params.Reason,
		Status:          GrantStatusActive,
		IsEmergency:     true,
		EmergencyReason: params.Reason,
	}

	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertGrant(ctx, tx, grant); err != nil {
			return err
		}
		if err := s.recordCustodyEvent(ctx, tx, &AccessCustodyEvent{
			TenantID:   tenantID,
			PropertyID: propertyID,
			GrantID:    grant.ID,
			SecretID:   secret.ID,
			EventType:  CustodyEventTypeEmergencyAccess,
			ActorID:    actorID,
			GranteeID:  actorID,
			Reason:     params.Reason,
		}); err != nil {
			return err
		}

		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeAccess,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "access.grant.emergency",
			ResourceType: "access_grant",
			ResourceID:   grant.ID,
			NewState:     marshalJSON(grant),
		})
	})
	if err != nil {
		return nil, nil, err
	}

	return grant, secret, nil
}

func (s *Service) PlaceHold(ctx context.Context, tenantID, propertyID string, params CreateHoldParams, actorID string) (*AccessHold, error) {
	if params.Reason == "" {
		return nil, fmt.Errorf("%w: reason is required", ErrInvalidSecret)
	}

	hold := &AccessHold{
		TenantID:   tenantID,
		PropertyID: propertyID,
		Reason:     params.Reason,
		PlacedBy:   actorID,
		Status:     HoldStatusActive,
	}

	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertHold(ctx, tx, hold); err != nil {
			return err
		}
		if err := s.recordCustodyEvent(ctx, tx, &AccessCustodyEvent{
			TenantID:   tenantID,
			PropertyID: propertyID,
			EventType:  CustodyEventTypeHoldPlaced,
			ActorID:    actorID,
			Reason:     params.Reason,
		}); err != nil {
			return err
		}

		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeAccess,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "access.hold.placed",
			ResourceType: "access_hold",
			ResourceID:   hold.ID,
			NewState:     marshalJSON(hold),
		})
	})
	if err != nil {
		return nil, err
	}

	return hold, nil
}

func (s *Service) ReleaseHold(ctx context.Context, tenantID, holdID, actorID string) (*AccessHold, error) {
	var hold *AccessHold
	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		hold, err = s.store.ReleaseHold(ctx, tx, tenantID, holdID, actorID)
		if err != nil {
			return err
		}
		if err := s.recordCustodyEvent(ctx, tx, &AccessCustodyEvent{
			TenantID:   tenantID,
			PropertyID: hold.PropertyID,
			EventType:  CustodyEventTypeHoldReleased,
			ActorID:    actorID,
			Reason:     "hold released",
		}); err != nil {
			return err
		}

		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeAccess,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "access.hold.released",
			ResourceType: "access_hold",
			ResourceID:   holdID,
			NewState:     marshalJSON(hold),
		})
	})
	if err != nil {
		return nil, err
	}

	return hold, nil
}

func (s *Service) ListCustodyEvents(ctx context.Context, tenantID, propertyID string) ([]AccessCustodyEvent, error) {
	return s.store.ListCustodyEvents(ctx, tenantID, propertyID)
}

func (s *Service) ListDisclosures(ctx context.Context, tenantID, grantID string) ([]AccessDisclosure, error) {
	return s.store.ListDisclosures(ctx, tenantID, grantID)
}
