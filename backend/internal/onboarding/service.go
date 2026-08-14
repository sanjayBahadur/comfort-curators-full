package onboarding

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
// the case's existence or any detail is disclosed to the caller.
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

func (s *Service) authorizeCase(ctx context.Context, tenantID, caseID string) error {
	if s.authorizer == nil {
		return ErrCrossTenantDenied
	}
	if err := s.authorizer.RequireResourceAccess(ctx, tenantID, "onboarding_case", caseID); err != nil {
		return ErrCrossTenantDenied
	}
	return nil
}

func (s *Service) authorizeTenant(ctx context.Context, tenantID string) error {
	if s.authorizer == nil {
		return ErrCrossTenantDenied
	}
	if err := s.authorizer.RequireResourceAccess(ctx, tenantID, "onboarding_case", ""); err != nil {
		return ErrCrossTenantDenied
	}
	return nil
}

// StartCase opens a new owner and property onboarding case in the in_progress
// state. The case persists each section independently, so it can be interrupted
// and resumed later.
func (s *Service) StartCase(ctx context.Context, params StartCaseParams, actorID string) (*Case, error) {
	if err := s.authorizeTenant(ctx, params.TenantID); err != nil {
		return nil, err
	}
	if params.TenantID == "" {
		return nil, fmt.Errorf("tenant_id is required")
	}
	if params.PropertyID == "" {
		return nil, fmt.Errorf("property_id is required")
	}
	if params.OwnerAuthorityID == "" {
		return nil, fmt.Errorf("owner_authority_id is required")
	}

	c := &Case{
		TenantID:         params.TenantID,
		PropertyID:       params.PropertyID,
		OwnerAuthorityID: params.OwnerAuthorityID,
		Status:           StatusInProgress,
		Version:          1,
	}

	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.CreateCase(ctx, tx, c); err != nil {
			return err
		}
		if s.auditStore != nil {
			if err := s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
				EventType:    audit.EventTypeMutation,
				TenantID:     c.TenantID,
				ActorID:      actorID,
				Action:       "onboarding.case.start",
				ResourceType: "onboarding_case",
				ResourceID:   c.ID,
			}); err != nil {
				return fmt.Errorf("audit: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return c, nil
}

// SaveSection records one typed onboarding section. Sections persist
// independently, so recording a later section after an interruption resumes
// the case from its last committed state.
func (s *Service) SaveSection(ctx context.Context, tenantID, caseID, section string, payload json.RawMessage, actorID string) (*Case, error) {
	if err := s.authorizeCase(ctx, tenantID, caseID); err != nil {
		return nil, err
	}
	column, ok := sectionColumn(section)
	if !ok {
		return nil, ErrInvalidSection
	}

	var result *Case
	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		c, err := s.store.getByID(ctx, tx, tenantID, caseID)
		if err != nil {
			return err
		}
		if c.Status == StatusActivated {
			return ErrCaseActivated
		}
		if err := c.ApplySection(section, payload); err != nil {
			return err
		}
		c.Version++
		c.RecomputeStatus(time.Now().UTC())
		columns, err := sectionColumnsFor(c, column)
		if err != nil {
			return err
		}
		if err := s.store.updateCaseSections(ctx, tx, c, columns); err != nil {
			return err
		}
		c.Holds = c.ActivationHolds(time.Now().UTC())
		result = c
		if s.auditStore != nil {
			if err := s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
				EventType:    audit.EventTypeMutation,
				TenantID:     tenantID,
				ActorID:      actorID,
				Action:       "onboarding.case.section",
				ResourceType: "onboarding_case",
				ResourceID:   caseID,
				Metadata:     mustJSON(map[string]string{"section": section}),
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

// SaveContacts records the owner contact list as a single checklist step.
func (s *Service) SaveContacts(ctx context.Context, tenantID, caseID string, contacts []Contact, actorID string) (*Case, error) {
	if err := s.authorizeCase(ctx, tenantID, caseID); err != nil {
		return nil, err
	}

	var result *Case
	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		c, err := s.store.getByID(ctx, tx, tenantID, caseID)
		if err != nil {
			return err
		}
		if c.Status == StatusActivated {
			return ErrCaseActivated
		}
		for _, contact := range contacts {
			if contact.Name == "" || contact.Phone == "" {
				return fmt.Errorf("contact requires name and phone")
			}
		}
		c.ApplyContacts(contacts)
		c.Version++
		c.RecomputeStatus(time.Now().UTC())
		contactsJSON, err := json.Marshal(c.Contacts)
		if err != nil {
			return err
		}
		if err := s.store.updateCaseSections(ctx, tx, c, map[string]json.RawMessage{"contacts": contactsJSON}); err != nil {
			return err
		}
		c.Holds = c.ActivationHolds(time.Now().UTC())
		result = c
		if s.auditStore != nil {
			if err := s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
				EventType:    audit.EventTypeMutation,
				TenantID:     tenantID,
				ActorID:      actorID,
				Action:       "onboarding.case.contacts",
				ResourceType: "onboarding_case",
				ResourceID:   caseID,
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

// RecordEvidence appends an immutable legal, safety or document evidence
// capture. Recording mandatory legal and safety evidence clears the
// corresponding activation hold.
func (s *Service) RecordEvidence(ctx context.Context, tenantID, caseID string, params EvidenceParams, actorID string) (*Case, error) {
	if err := s.authorizeCase(ctx, tenantID, caseID); err != nil {
		return nil, err
	}
	if !ValidEvidenceKind(params.Kind) {
		return nil, fmt.Errorf("%w: unknown kind %q", ErrInvalidEvidence, params.Kind)
	}
	if params.ContentHash == "" {
		return nil, fmt.Errorf("%w: content_hash is required", ErrInvalidEvidence)
	}
	if params.ObjectRef == "" {
		return nil, fmt.Errorf("%w: object_ref is required", ErrInvalidEvidence)
	}

	var result *Case
	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		c, err := s.store.getByID(ctx, tx, tenantID, caseID)
		if err != nil {
			return err
		}
		if c.Status == StatusActivated {
			return ErrCaseActivated
		}
		evidence := Evidence{
			CaseID:      caseID,
			TenantID:    tenantID,
			Kind:        params.Kind,
			ContentHash: params.ContentHash,
			ObjectRef:   params.ObjectRef,
			CapturedBy:  params.CapturedBy,
			CapturedAt:  params.CapturedAt,
		}
		if evidence.CapturedBy == "" {
			evidence.CapturedBy = actorID
		}
		if evidence.CapturedAt.IsZero() {
			evidence.CapturedAt = time.Now().UTC()
		}
		if err := s.store.InsertEvidence(ctx, tx, &evidence); err != nil {
			return err
		}
		c.ApplyEvidence(evidence)
		c.Version++
		c.RecomputeStatus(time.Now().UTC())
		if err := s.store.updateCaseStatus(ctx, tx, c); err != nil {
			return err
		}
		c.Holds = c.ActivationHolds(time.Now().UTC())
		result = c
		if s.auditStore != nil {
			if err := s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
				EventType:    audit.EventTypeMutation,
				TenantID:     tenantID,
				ActorID:      actorID,
				Action:       "onboarding.case.evidence",
				ResourceType: "onboarding_case",
				ResourceID:   caseID,
				Metadata:     mustJSON(map[string]string{"kind": params.Kind}),
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

// RecordInspection appends a property inspection with its captured evidence.
// Inspection evidence is immutable: the record cannot be updated or deleted
// after it is persisted, and a corrected inspection is a new record.
func (s *Service) RecordInspection(ctx context.Context, tenantID, caseID string, params InspectionParams, actorID string) (*Inspection, error) {
	if err := s.authorizeCase(ctx, tenantID, caseID); err != nil {
		return nil, err
	}
	if params.PropertyID == "" {
		return nil, fmt.Errorf("%w: property_id is required", ErrInvalidInspection)
	}
	if params.EvidenceHash == "" {
		return nil, fmt.Errorf("%w: evidence_hash is required", ErrInvalidInspection)
	}
	if params.InspectedBy == "" {
		return nil, fmt.Errorf("%w: inspected_by is required", ErrInvalidInspection)
	}

	var result *Inspection
	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		c, err := s.store.getByID(ctx, tx, tenantID, caseID)
		if err != nil {
			return err
		}
		if c.Status == StatusActivated {
			return ErrCaseActivated
		}
		inspection := Inspection{
			CaseID:        caseID,
			TenantID:      tenantID,
			PropertyID:    params.PropertyID,
			PerformedAt:   params.PerformedAt,
			InspectedBy:   params.InspectedBy,
			EvidenceHash:  params.EvidenceHash,
			EvidenceRef:   params.EvidenceRef,
			Findings:      params.Findings,
			OverallStatus: params.OverallStatus,
		}
		if inspection.PerformedAt.IsZero() {
			inspection.PerformedAt = time.Now().UTC()
		}
		if inspection.OverallStatus == "" {
			return fmt.Errorf("%w: overall_status is required", ErrInvalidInspection)
		}
		if err := s.store.InsertInspection(ctx, tx, &inspection); err != nil {
			return err
		}
		c.ApplyInspection(inspection)
		c.Version++
		c.RecomputeStatus(time.Now().UTC())
		if err := s.store.updateCaseStatus(ctx, tx, c); err != nil {
			return err
		}
		result = &inspection
		if s.auditStore != nil {
			if err := s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
				EventType:    audit.EventTypeMutation,
				TenantID:     tenantID,
				ActorID:      actorID,
				Action:       "onboarding.case.inspection",
				ResourceType: "onboarding_case",
				ResourceID:   caseID,
				Metadata:     mustJSON(map[string]string{"inspection_id": inspection.ID}),
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

// GetCase returns the full aggregate with evidence, inspections and computed
// activation holds. It is the resume surface: committed sections are returned
// exactly as persisted so an interrupted case can continue.
func (s *Service) GetCase(ctx context.Context, tenantID, caseID string) (*Case, error) {
	if err := s.authorizeCase(ctx, tenantID, caseID); err != nil {
		return nil, err
	}
	return s.store.GetCase(ctx, tenantID, caseID)
}

func (s *Service) ListCases(ctx context.Context, tenantID string) ([]Case, error) {
	if err := s.authorizeTenant(ctx, tenantID); err != nil {
		return nil, err
	}
	return s.store.ListCases(ctx, tenantID)
}

// Progress returns the checklist with completion state so the caller can
// resume exactly where an interrupted case stopped.
func (s *Service) Progress(ctx context.Context, tenantID, caseID string) ([]StepProgress, error) {
	c, err := s.GetCase(ctx, tenantID, caseID)
	if err != nil {
		return nil, err
	}
	return c.Progress(), nil
}

// ActivationHolds returns the holds that currently block activation.
func (s *Service) ActivationHolds(ctx context.Context, tenantID, caseID string) ([]ActivationHold, error) {
	c, err := s.GetCase(ctx, tenantID, caseID)
	if err != nil {
		return nil, err
	}
	return c.ActivationHolds(time.Now().UTC()), nil
}

// Activate marks the case activated once every checklist step is recorded and
// no activation hold stands. Missing legal or safety evidence returns
// ErrActivationBlocked; activation is terminal.
func (s *Service) Activate(ctx context.Context, tenantID, caseID, actorID string) (*Case, error) {
	if err := s.authorizeCase(ctx, tenantID, caseID); err != nil {
		return nil, err
	}

	var result *Case
	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		c, err := s.store.getByID(ctx, tx, tenantID, caseID)
		if err != nil {
			return err
		}
		if c.Status == StatusActivated {
			return ErrCaseActivated
		}
		if err := c.CanActivate(time.Now().UTC()); err != nil {
			return err
		}
		c.Status = StatusActivated
		c.Version++
		if err := s.store.updateCaseStatus(ctx, tx, c); err != nil {
			return err
		}
		result = c
		if s.auditStore != nil {
			if err := s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
				EventType:    audit.EventTypeMutation,
				TenantID:     tenantID,
				ActorID:      actorID,
				Action:       "onboarding.case.activate",
				ResourceType: "onboarding_case",
				ResourceID:   caseID,
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

// sectionColumnsFor serializes the single changed column from the aggregate.
// The column value is re-marshaled from the typed section so persistence is
// always the validated canonical form.
func sectionColumnsFor(c *Case, column string) (map[string]json.RawMessage, error) {
	var data any
	switch column {
	case "portfolio":
		data = c.Portfolio
	case "goals":
		data = c.Goals
	case "service_preferences":
		data = c.ServicePreferences
	case "budgets":
		data = c.Budgets
	case "photographs":
		data = c.Photographs
	case "amenities":
		data = c.Amenities
	case "safety":
		data = c.Safety
	case "furnishing":
		data = c.Furnishing
	case "remediation":
		data = c.Remediation
	case "fit_score_inputs":
		data = c.FitScoreInputs
	default:
		return nil, ErrInvalidSection
	}
	b, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	return map[string]json.RawMessage{column: b}, nil
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}
