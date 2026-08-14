package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"comfort-curators-backend/internal/platform/audit"
	"comfort-curators-backend/internal/platform/database"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ConsumerService struct {
	pool       *pgxpool.Pool
	store      *ConsumerStore
	auditStore *audit.AuditStore
}

func NewConsumerService(pool *pgxpool.Pool, auditStore *audit.AuditStore) *ConsumerService {
	return &ConsumerService{
		pool:       pool,
		store:      NewConsumerStore(pool),
		auditStore: auditStore,
	}
}

// RecordDisclosure records the pre-acceptance price, tax, recurrence,
// substitution, cancellation, refund, seller and origin disclosure for a
// consumer resource (CON-001, CON-002). A recurring disclosure must carry an
// explicit recurring cost, so a hidden recurring charge is impossible
// (CON-004). The recurring cost is always visible on the returned record.
func (s *ConsumerService) RecordDisclosure(ctx context.Context, tenantID string, params DisclosureParams, actorID string) (*Disclosure, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidDisclosure)
	}
	if err := ValidateDisclosureParams(params); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	d := &Disclosure{
		TenantID:                   tenantID,
		PropertyID:                 params.PropertyID,
		ResourceType:               params.ResourceType,
		ResourceID:                 params.ResourceID,
		PriceMinorUnits:            params.PriceMinorUnits,
		TaxMinorUnits:              params.TaxMinorUnits,
		Currency:                   params.Currency,
		Recurrence:                 params.Recurrence,
		RecurrenceAmountMinorUnits: params.RecurrenceAmountMinorUnits,
		SubstitutionPolicy:         params.SubstitutionPolicy,
		CancellationPolicy:         params.CancellationPolicy,
		RefundPolicy:               params.RefundPolicy,
		Seller:                     params.Seller,
		CountryOfOrigin:            params.CountryOfOrigin,
		GrievanceContact:           params.GrievanceContact,
		RecurringCostVisible:       true,
		CreatedAt:                  now,
	}

	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertDisclosure(ctx, tx, d); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "consumer.disclosure.recorded",
			ResourceType: "consumer_disclosure",
			ResourceID:   d.ID,
		})
	}); err != nil {
		return nil, err
	}

	return d, nil
}

func (s *ConsumerService) GetDisclosure(ctx context.Context, tenantID, disclosureID string) (*Disclosure, error) {
	return s.store.GetDisclosure(ctx, tenantID, disclosureID)
}

func (s *ConsumerService) ListDisclosures(ctx context.Context, tenantID string) ([]Disclosure, error) {
	disclosures, err := s.store.ListDisclosures(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if disclosures == nil {
		disclosures = []Disclosure{}
	}
	return disclosures, nil
}

// Accept records that a resource was accepted. Acceptance is only possible
// after a disclosure for the exact same resource exists with a visible
// recurring cost, so recurring cost is always visible before acceptance
// (CON-001). Acceptance without a prior disclosure is refused.
func (s *ConsumerService) Accept(ctx context.Context, tenantID string, params AcceptanceParams, actorID string) (*Acceptance, error) {
	if tenantID == "" || params.DisclosureID == "" || params.ResourceType == "" || params.ResourceID == "" {
		return nil, fmt.Errorf("%w: tenant_id, disclosure_id, resource_type and resource_id are required", ErrInvalidAcceptance)
	}
	if !ValidResourceType(params.ResourceType) {
		return nil, fmt.Errorf("%w: invalid resource_type %q", ErrInvalidResourceType, params.ResourceType)
	}

	disclosure, err := s.store.GetDisclosure(ctx, tenantID, params.DisclosureID)
	if err != nil {
		if err == ErrDisclosureNotFound {
			return nil, ErrNoDisclosureBeforeAccept
		}
		return nil, err
	}
	if err := DisclosureIsAcceptable(disclosure); err != nil {
		return nil, err
	}
	if disclosure.ResourceType != params.ResourceType || disclosure.ResourceID != params.ResourceID {
		return nil, ErrDisclosureResourceMismatch
	}

	now := time.Now().UTC()
	a := &Acceptance{
		TenantID:     tenantID,
		PropertyID:   params.PropertyID,
		DisclosureID: disclosure.ID,
		ResourceType: params.ResourceType,
		ResourceID:   params.ResourceID,
		AcceptedBy:   actorID,
		AcceptedAt:   now,
		CreatedAt:    now,
	}

	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertAcceptance(ctx, tx, a); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "consumer.acceptance.recorded",
			ResourceType: "consumer_acceptance",
			ResourceID:   a.ID,
			Metadata:     mustJSON(map[string]string{"disclosure_id": disclosure.ID}),
		})
	}); err != nil {
		return nil, err
	}

	return a, nil
}

func (s *ConsumerService) GetAcceptance(ctx context.Context, tenantID, acceptanceID string) (*Acceptance, error) {
	return s.store.GetAcceptance(ctx, tenantID, acceptanceID)
}

// CreateHistoryExport builds and retains a tenant-scoped export of owner
// order, invoice, package and service history (CON-006). Every source query is
// filtered by tenant_id, so no other tenant's history can enter the export.
func (s *ConsumerService) CreateHistoryExport(ctx context.Context, tenantID, propertyID, requestedBy string) (*HistoryExport, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidExport)
	}

	data, err := s.store.CollectHistory(ctx, tenantID, propertyID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	exp := &HistoryExport{
		TenantID:    tenantID,
		PropertyID:  propertyID,
		RequestedBy: requestedBy,
		Status:      ExportStatusCompleted,
		Data:        data,
		CreatedAt:   now,
	}

	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertHistoryExport(ctx, tx, exp); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      requestedBy,
			Action:       "consumer.history_export.created",
			ResourceType: "consumer_history_export",
			ResourceID:   exp.ID,
		})
	}); err != nil {
		return nil, err
	}

	return exp, nil
}

func (s *ConsumerService) GetHistoryExport(ctx context.Context, tenantID, exportID string) (*HistoryExport, error) {
	return s.store.GetHistoryExport(ctx, tenantID, exportID)
}

func (s *ConsumerService) ListHistoryExports(ctx context.Context, tenantID string) ([]HistoryExport, error) {
	exports, err := s.store.ListHistoryExports(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if exports == nil {
		exports = []HistoryExport{}
	}
	return exports, nil
}

func (s *ConsumerService) appendAudit(ctx context.Context, evt audit.AuditEvent) {
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
