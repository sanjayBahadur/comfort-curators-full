package property

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"comfort-curators-backend/internal/platform/audit"
	"comfort-curators-backend/internal/platform/database"
)

var (
	ErrCrossTenantDenied = errors.New("cross-tenant access denied")
)

// ResourceAuthorizer is implemented by the tenancy module. It denies before
// the property's existence or any detail is disclosed to the caller.
type ResourceAuthorizer interface {
	RequireResourceAccess(ctx context.Context, resourceTenantID, resourceType, resourceID string) error
}

type PropertyService struct {
	pool       *pgxpool.Pool
	store      *PropertyStore
	auditStore *audit.AuditStore
	authorizer ResourceAuthorizer
}

func NewPropertyService(pool *pgxpool.Pool, auditStore *audit.AuditStore) *PropertyService {
	return &PropertyService{
		pool:       pool,
		store:      NewPropertyStore(pool),
		auditStore: auditStore,
	}
}

// WithAuthorizer attaches the tenancy-scoped resource authorizer. The service
// fails closed: without an authorizer, every operation is refused.
func (s *PropertyService) WithAuthorizer(a ResourceAuthorizer) *PropertyService {
	s.authorizer = a
	return s
}

func (s *PropertyService) authorizeProperty(ctx context.Context, tenantID, propertyID string) error {
	if s.authorizer == nil {
		return ErrCrossTenantDenied
	}
	if err := s.authorizer.RequireResourceAccess(ctx, tenantID, "property", propertyID); err != nil {
		return ErrCrossTenantDenied
	}
	return nil
}

func (s *PropertyService) authorizeTenant(ctx context.Context, tenantID string) error {
	if s.authorizer == nil {
		return ErrCrossTenantDenied
	}
	if err := s.authorizer.RequireResourceAccess(ctx, tenantID, "property", ""); err != nil {
		return ErrCrossTenantDenied
	}
	return nil
}

func (s *PropertyService) CreateProperty(ctx context.Context, params CreatePropertyParams, actorID string) (*Property, error) {
	if err := s.authorizeTenant(ctx, params.TenantID); err != nil {
		return nil, err
	}
	if params.TenantID == "" {
		return nil, fmt.Errorf("tenant_id is required")
	}
	if params.OwnerAuthorityID == "" {
		return nil, fmt.Errorf("owner_authority_id is required")
	}
	if params.ServiceAddress.Line1 == "" || params.ServiceAddress.City == "" {
		return nil, fmt.Errorf("service_address requires line1 and city")
	}
	if params.MaximumOccupancy <= 0 {
		return nil, fmt.Errorf("maximum_occupancy must be positive")
	}

	initialState := params.InitialState
	if initialState == "" {
		initialState = StateLead
	}
	if !IsValidState(initialState) {
		return nil, ErrInvalidState
	}

	p := &Property{
		TenantID:          params.TenantID,
		OwnerAuthorityID:  params.OwnerAuthorityID,
		ServiceAddress:    params.ServiceAddress,
		GeolocationZone:   params.GeolocationZone,
		Timezone:          params.Timezone,
		EmergencyContacts: params.EmergencyContacts,
		AccessMethod:      params.AccessMethod,
		MaximumOccupancy:  params.MaximumOccupancy,
		State:             initialState,
		Version:           1,
	}
	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := createProperty(ctx, tx, p); err != nil {
			return err
		}
		if err := s.store.GrantOwnerAuthority(ctx, tx, p.TenantID, actorID, p.OwnerAuthorityID); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			ID:           newID("aud"),
			EventType:    audit.EventTypeMutation,
			TenantID:     p.TenantID,
			ActorID:      actorID,
			Action:       "property.create",
			ResourceType: "property",
			ResourceID:   p.ID,
		})
	})
	if err != nil {
		return nil, err
	}
	return p, nil
}

// ResolveActorAuthorities returns the owner authorities actorID controls.
// It is the concrete backing for api.OwnerAuthorities.
func (s *PropertyService) ResolveActorAuthorities(ctx context.Context, actorID string) []string {
	authorities, err := s.store.ListAuthoritiesForActor(ctx, actorID)
	if err != nil {
		return nil
	}
	return authorities
}

func (s *PropertyService) GetProperty(ctx context.Context, tenantID, propertyID string) (*Property, error) {
	if err := s.authorizeProperty(ctx, tenantID, propertyID); err != nil {
		return nil, err
	}
	return s.store.Get(ctx, tenantID, propertyID)
}

func (s *PropertyService) ListProperties(ctx context.Context, tenantID string) ([]Property, error) {
	if err := s.authorizeTenant(ctx, tenantID); err != nil {
		return nil, err
	}
	return s.store.List(ctx, tenantID)
}

func (s *PropertyService) SetReadiness(ctx context.Context, tenantID, propertyID string, readiness Readiness, actorID string) (*Property, error) {
	if err := s.authorizeProperty(ctx, tenantID, propertyID); err != nil {
		return nil, err
	}

	var result *Property
	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		p, err := s.store.getByID(ctx, tx, tenantID, propertyID)
		if err != nil {
			return err
		}
		p.Readiness = readiness
		p.Version++
		if err := setPropertyReadiness(ctx, tx, p); err != nil {
			return err
		}
		result = p
		if err := s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			ID:           newID("aud"),
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "property.readiness.update",
			ResourceType: "property",
			ResourceID:   propertyID,
		}); err != nil {
			return fmt.Errorf("audit: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

func setPropertyReadiness(ctx context.Context, q querier, p *Property) error {
	tag, err := q.Exec(ctx, `
		UPDATE properties
		SET owner_contract_accepted = $3, compliance_complete = $4,
			mandatory_fields_set = $5, version = $6, updated_at = NOW()
		WHERE id = $1 AND tenant_id = $2 AND version = $7
	`, p.ID, p.TenantID, p.Readiness.OwnerContractAccepted, p.Readiness.ComplianceComplete,
		p.Readiness.MandatoryFieldsSet, p.Version, p.Version-1)
	if err != nil {
		return fmt.Errorf("update property readiness: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("property readiness update lost a concurrent write (optimistic version)")
	}
	return nil
}

// TransitionProperty moves a property through the frozen lifecycle. The
// transition, state version and audit record commit atomically. A transition
// into StateActive runs the readiness gate and is refused while a critical
// compliance hold blocks activation.
func (s *PropertyService) TransitionProperty(ctx context.Context, tenantID, propertyID, toState, reason, actorID string) (*Property, error) {
	if err := s.authorizeProperty(ctx, tenantID, propertyID); err != nil {
		return nil, err
	}
	if !IsValidState(toState) {
		return nil, ErrInvalidState
	}
	if reason == "" {
		return nil, fmt.Errorf("transition reason is required")
	}

	var result *Property
	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		p, err := s.store.getByID(ctx, tx, tenantID, propertyID)
		if err != nil {
			return err
		}
		holds, err := listHolds(ctx, tx, tenantID, propertyID)
		if err != nil {
			return err
		}
		p.ComplianceHolds = holds

		if p.State == StateArchived {
			return ErrArchivedTerminal
		}
		if !CanTransition(p.State, toState) {
			return ErrInvalidTransition
		}
		if toState == StateActive {
			if err := CanActivate(p, holds, time.Now().UTC()); err != nil {
				return err
			}
		}

		fromState, fromVersion := p.State, p.Version
		if err := ApplyTransition(p, toState); err != nil {
			return err
		}
		if err := updatePropertyState(ctx, tx, p); err != nil {
			return err
		}
		transition := PropertyTransition{
			ID:          newID("trx"),
			PropertyID:  propertyID,
			TenantID:    tenantID,
			FromState:   fromState,
			ToState:     toState,
			ActorID:     actorID,
			Reason:      reason,
			FromVersion: fromVersion,
			ToVersion:   p.Version,
		}
		if err := insertTransition(ctx, tx, transition); err != nil {
			return err
		}
		p.UpdatedAt = time.Now().UTC()
		result = p
		if err := s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			ID:           newID("aud"),
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "property.transition",
			ResourceType: "property",
			ResourceID:   propertyID,
			NewState:     mustJSON(map[string]string{"state": toState}),
		}); err != nil {
			return fmt.Errorf("audit: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (s *PropertyService) ListTransitions(ctx context.Context, tenantID, propertyID string) ([]PropertyTransition, error) {
	if err := s.authorizeProperty(ctx, tenantID, propertyID); err != nil {
		return nil, err
	}
	return s.store.ListTransitions(ctx, tenantID, propertyID)
}

func (s *PropertyService) AddComplianceHold(ctx context.Context, tenantID, propertyID string, params ComplianceHoldParams, actorID string) (*ComplianceHold, error) {
	if err := s.authorizeProperty(ctx, tenantID, propertyID); err != nil {
		return nil, err
	}

	hold, err := NewComplianceHold(propertyID, tenantID, params, time.Now().UTC())
	if err != nil {
		return nil, err
	}

	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := s.store.getByID(ctx, tx, tenantID, propertyID); err != nil {
			return err
		}
		if err := insertComplianceHold(ctx, tx, hold); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			ID:           newID("aud"),
			EventType:    audit.EventTypeSecurity,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "property.compliance_hold.add",
			ResourceType: "property",
			ResourceID:   propertyID,
		})
	})
	if err != nil {
		return nil, err
	}
	return hold, nil
}

func (s *PropertyService) ResolveComplianceHold(ctx context.Context, tenantID, propertyID, holdID, actorID string) (*Property, error) {
	if err := s.authorizeProperty(ctx, tenantID, propertyID); err != nil {
		return nil, err
	}

	var result *Property
	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		holds, err := listHolds(ctx, tx, tenantID, propertyID)
		if err != nil {
			return err
		}
		hold, err := findHold(holds, holdID)
		if err != nil {
			return err
		}
		if err := hold.Resolve(time.Now().UTC()); err != nil {
			return err
		}
		if err := updateComplianceHold(ctx, tx, hold); err != nil {
			return err
		}
		p, err := s.store.getByID(ctx, tx, tenantID, propertyID)
		if err != nil {
			return err
		}
		p.ComplianceHolds, err = listHolds(ctx, tx, tenantID, propertyID)
		if err != nil {
			return err
		}
		result = p
		if err := s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			ID:           newID("aud"),
			EventType:    audit.EventTypeSecurity,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "property.compliance_hold.resolve",
			ResourceType: "property",
			ResourceID:   propertyID,
			Metadata:     mustJSON(map[string]string{"hold_id": holdID}),
		}); err != nil {
			return fmt.Errorf("audit: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (s *PropertyService) GrantComplianceException(ctx context.Context, tenantID, propertyID, holdID, reviewerID, reason string, ttl time.Duration, actorID string) (*Property, error) {
	if err := s.authorizeProperty(ctx, tenantID, propertyID); err != nil {
		return nil, err
	}
	if reviewerID == "" {
		return nil, ErrExceptionDenied
	}
	if reason == "" {
		return nil, fmt.Errorf("exception reason is required")
	}
	if ttl <= 0 {
		return nil, fmt.Errorf("exception ttl must be positive")
	}

	var result *Property
	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		holds, err := listHolds(ctx, tx, tenantID, propertyID)
		if err != nil {
			return err
		}
		hold, err := findHold(holds, holdID)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		if err := hold.GrantException(reviewerID, now.Add(ttl), now); err != nil {
			return err
		}
		if err := updateComplianceHold(ctx, tx, hold); err != nil {
			return err
		}
		p, err := s.store.getByID(ctx, tx, tenantID, propertyID)
		if err != nil {
			return err
		}
		p.ComplianceHolds, err = listHolds(ctx, tx, tenantID, propertyID)
		if err != nil {
			return err
		}
		result = p
		if err := s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			ID:           newID("aud"),
			EventType:    audit.EventTypeAdmin,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "property.compliance_hold.exception",
			ResourceType: "property",
			ResourceID:   propertyID,
			Metadata:     mustJSON(map[string]string{"hold_id": holdID, "reviewer_id": reviewerID}),
		}); err != nil {
			return fmt.Errorf("audit: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

func findHold(holds []ComplianceHold, holdID string) (*ComplianceHold, error) {
	for i := range holds {
		if holds[i].ID == holdID {
			return &holds[i], nil
		}
	}
	return nil, ErrHoldNotFound
}

func (s *PropertyService) appendAudit(ctx context.Context, evt audit.AuditEvent) {
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
