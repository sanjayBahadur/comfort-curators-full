package contracts

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"comfort-curators-backend/internal/platform/audit"
	"comfort-curators-backend/internal/platform/database"
)

// ResourceAuthorizer is implemented by the tenancy module. It denies before
// the agreement's existence or any detail is disclosed to the caller.
type ResourceAuthorizer interface {
	RequireResourceAccess(ctx context.Context, resourceTenantID, resourceType, resourceID string) error
}

type Service struct {
	pool       *pgxpool.Pool
	store      *Store
	auditStore *audit.AuditStore
	authorizer ResourceAuthorizer
}

func NewService(pool *pgxpool.Pool, auditStore *audit.AuditStore) *Service {
	return &Service{
		pool:       pool,
		store:      NewStore(pool),
		auditStore: auditStore,
	}
}

// WithAuthorizer attaches the tenancy-scoped resource authorizer. The service
// fails closed: without an authorizer, every operation is refused.
func (s *Service) WithAuthorizer(a ResourceAuthorizer) *Service {
	s.authorizer = a
	return s
}

func (s *Service) authorizeAgreement(ctx context.Context, tenantID, agreementID string) error {
	if s.authorizer == nil {
		return ErrCrossTenantDenied
	}
	if err := s.authorizer.RequireResourceAccess(ctx, tenantID, "service_contract", agreementID); err != nil {
		return ErrCrossTenantDenied
	}
	return nil
}

func (s *Service) authorizeTenant(ctx context.Context, tenantID string) error {
	if s.authorizer == nil {
		return ErrCrossTenantDenied
	}
	if err := s.authorizer.RequireResourceAccess(ctx, tenantID, "service_contract", ""); err != nil {
		return ErrCrossTenantDenied
	}
	return nil
}

// CreateAgreement opens a draft service agreement with its first immutable
// version. The terms are hashed canonically and the version record is
// immutable once persisted.
func (s *Service) CreateAgreement(ctx context.Context, params CreateAgreementParams, actorID string) (*Agreement, error) {
	if err := s.authorizeTenant(ctx, params.TenantID); err != nil {
		return nil, err
	}
	if params.TenantID == "" {
		return nil, fmt.Errorf("tenant_id is required")
	}
	if params.PropertyID == "" {
		return nil, fmt.Errorf("property_id is required")
	}
	if len(params.Terms) == 0 {
		return nil, ErrEmptyTerms
	}

	a := &Agreement{
		TenantID:   params.TenantID,
		PropertyID: params.PropertyID,
		Status:     AgreementStatusDraft,
		Version:    1,
		CreatedAt:  time.Now().UTC(),
	}

	var version *AgreementVersion
	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		v, err := s.store.CreateAgreement(ctx, tx, a, params.Terms, time.Now().UTC())
		if err != nil {
			return err
		}
		version = v
		if s.auditStore != nil {
			if err := s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
				EventType:    audit.EventTypeMutation,
				TenantID:     a.TenantID,
				ActorID:      actorID,
				Action:       "contracts.agreement.create",
				ResourceType: "service_contract",
				ResourceID:   a.ID,
				Metadata:     mustJSON(map[string]any{"version": version.VersionNumber, "content_hash": version.ContentHash}),
			}); err != nil {
				return fmt.Errorf("audit: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	a.Versions = []AgreementVersion{*version}
	return a, nil
}

// AddVersion appends a new immutable version to a draft agreement. An accepted
// agreement cannot mutate, so a correction after acceptance must open a new
// agreement.
func (s *Service) AddVersion(ctx context.Context, tenantID, agreementID string, terms []byte, actorID string) (*Agreement, error) {
	if err := s.authorizeAgreement(ctx, tenantID, agreementID); err != nil {
		return nil, err
	}
	if len(terms) == 0 {
		return nil, ErrEmptyTerms
	}

	var result *Agreement
	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		a, err := s.store.getByID(ctx, tx, tenantID, agreementID)
		if err != nil {
			return err
		}
		version, err := a.AddVersion(terms, time.Now().UTC())
		if err != nil {
			return err
		}
		if err := s.store.AddVersion(ctx, tx, a, version); err != nil {
			return err
		}
		result = a
		if s.auditStore != nil {
			if err := s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
				EventType:    audit.EventTypeMutation,
				TenantID:     tenantID,
				ActorID:      actorID,
				Action:       "contracts.agreement.version",
				ResourceType: "service_contract",
				ResourceID:   agreementID,
				Metadata:     mustJSON(map[string]any{"version": a.CurrentVersion}),
			}); err != nil {
				return fmt.Errorf("audit: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

// Accept accepts the current version of the agreement. Acceptance is terminal:
// the accepted agreement cannot mutate and a correction requires a new
// agreement. The acceptance points to the exact content hash of the accepted
// version.
func (s *Service) Accept(ctx context.Context, tenantID, agreementID, actorID string) (*Agreement, error) {
	if err := s.authorizeAgreement(ctx, tenantID, agreementID); err != nil {
		return nil, err
	}

	var result *Agreement
	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		a, err := s.store.getByID(ctx, tx, tenantID, agreementID)
		if err != nil {
			return err
		}
		acceptance, err := a.Accept(actorID, time.Now().UTC())
		if err != nil {
			return err
		}
		if err := s.store.Accept(ctx, tx, a, acceptance); err != nil {
			return err
		}
		result = a
		if s.auditStore != nil {
			if err := s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
				EventType:    audit.EventTypeMutation,
				TenantID:     tenantID,
				ActorID:      actorID,
				Action:       "contracts.agreement.accept",
				ResourceType: "service_contract",
				ResourceID:   agreementID,
				Metadata:     mustJSON(map[string]any{"version": a.Acceptance.VersionNumber, "content_hash": a.Acceptance.ContentHash}),
			}); err != nil {
				return fmt.Errorf("audit: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

// GetAgreement returns the full agreement aggregate. Reads verify every
// version content hash so accepted terms cannot silently drift.
func (s *Service) GetAgreement(ctx context.Context, tenantID, agreementID string) (*Agreement, error) {
	if err := s.authorizeAgreement(ctx, tenantID, agreementID); err != nil {
		return nil, err
	}
	return s.store.GetAgreement(ctx, tenantID, agreementID)
}

// ListAgreements returns the agreement summaries for the tenant.
func (s *Service) ListAgreements(ctx context.Context, tenantID string) ([]Agreement, error) {
	if err := s.authorizeTenant(ctx, tenantID); err != nil {
		return nil, err
	}
	return s.store.ListAgreements(ctx, tenantID)
}

// Quote computes the deterministic quote for the tenant's property under the
// exact fee rule version. Identical inputs and rule version produce an
// identical quote.
func (s *Service) Quote(ctx context.Context, tenantID string, inputs QuoteInputs, ruleVersion string) (*Quote, error) {
	if err := s.authorizeTenant(ctx, tenantID); err != nil {
		return nil, err
	}
	if inputs.TenantID == "" {
		inputs.TenantID = tenantID
	}
	if err := ValidateQuoteInputs(inputs); err != nil {
		return nil, err
	}
	rule, err := s.store.GetFeeRule(ctx, ruleVersion, inputs.Currency, inputs.ServiceTier)
	if err != nil {
		return nil, err
	}
	return CalculateQuote(inputs, *rule)
}

// SaveFeeRule persists a versioned fee rule. This is operator-maintained
// reference data; the application ships with no commercial rate selected by
// default.
func (s *Service) SaveFeeRule(ctx context.Context, rule *FeeRule) error {
	if err := ValidateFeeRule(*rule); err != nil {
		return err
	}
	if rule.ServiceTier == "" {
		rule.ServiceTier = ServiceTierOperations
	}
	return s.store.SaveFeeRule(ctx, rule)
}

func (s *Service) appendAudit(ctx context.Context, evt audit.AuditEvent) {
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
