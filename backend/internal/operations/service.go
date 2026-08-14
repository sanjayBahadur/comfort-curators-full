package operations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"comfort-curators-backend/internal/platform/audit"
	"comfort-curators-backend/internal/platform/database"
	"comfort-curators-backend/internal/platform/logging"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ResourceAuthorizer interface {
	RequireResourceAccess(ctx context.Context, resourceTenantID, resourceType, resourceID string) error
}

type TicketService struct {
	pool       *pgxpool.Pool
	store      *TicketStore
	authorizer ResourceAuthorizer
	auditStore *audit.AuditStore
}

func NewTicketService(pool *pgxpool.Pool) *TicketService {
	return &TicketService{
		pool:       pool,
		store:      NewTicketStore(pool),
		auditStore: audit.NewAuditStore(pool),
	}
}

func (s *TicketService) WithAuthorizer(a ResourceAuthorizer) *TicketService {
	s.authorizer = a
	return s
}

func (s *TicketService) WithAudit(a *audit.AuditStore) *TicketService {
	s.auditStore = a
	return s
}

func (s *TicketService) CreateTicket(ctx context.Context, params CreateTicketParams, actorID string) (*Ticket, error) {
	if !IsValidTicketType(params.Type) {
		return nil, fmt.Errorf("%w: %s", ErrInvalidTicketType, params.Type)
	}
	if params.TenantID == "" || params.PropertyID == "" {
		return nil, fmt.Errorf("tenant_id and property_id are required")
	}
	if params.Reason == "" {
		return nil, fmt.Errorf("reason is required")
	}

	if err := s.authorizeProperty(ctx, params.TenantID, params.PropertyID); err != nil {
		return nil, err
	}

	t := &Ticket{
		TenantID:           params.TenantID,
		PropertyID:         params.PropertyID,
		Type:               params.Type,
		Status:             StateDraft,
		Reason:             params.Reason,
		RequestedWindow:    params.RequestedWindow,
		ChecklistVersionID: params.ChecklistVersionID,
		CreatedBy:          actorID,
	}

	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertTicket(ctx, tx, t); err != nil {
			return err
		}
		if err := s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			ID:           newID("aud"),
			EventType:    audit.EventTypeMutation,
			TenantID:     t.TenantID,
			ActorID:      actorID,
			Action:       "ticket.created",
			ResourceType: "ticket",
			ResourceID:   t.ID,
			NewState:     marshalJSON(t),
		}); err != nil {
			return fmt.Errorf("audit: %w", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return t, nil
}

func (s *TicketService) GetTicket(ctx context.Context, tenantID, ticketID string) (*Ticket, error) {
	t, err := s.store.GetTicket(ctx, tenantID, ticketID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeProperty(ctx, tenantID, t.PropertyID); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *TicketService) ListTickets(ctx context.Context, tenantID, propertyID string, status string, cursor string, limit int) ([]Ticket, string, error) {
	if err := s.authorizeProperty(ctx, tenantID, propertyID); err != nil {
		return nil, "", err
	}
	return s.store.ListTickets(ctx, tenantID, propertyID, status, cursor, limit)
}

func (s *TicketService) TransitionTicket(ctx context.Context, tenantID, ticketID string, params TransitionParams, actorID string) (*Ticket, error) {
	if params.ToState == StateBlocked {
		return nil, fmt.Errorf("%w: use BlockTicket to set a blocker", ErrInvalidTransition)
	}

	var result *Ticket

	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		t, err := s.store.GetTicketForUpdate(ctx, tx, tenantID, ticketID)
		if err != nil {
			return err
		}

		if err := s.authorizeProperty(ctx, tenantID, t.PropertyID); err != nil {
			return err
		}

		fromState := t.Status

		if err := ApplyTransition(t, params.ToState); err != nil {
			return err
		}

		if params.ToState == StateEvidenceSubmitted || params.ToState == StateClosed {
			if err := s.assertEvidenceGates(ctx, tx, t); err != nil {
				return err
			}
		}

		if params.ToState == StateVerified && IsHighRiskTicketType(t.Type) && t.AssignedTo == actorID {
			return ErrSelfVerification
		}

		if params.ToState == StateVerified {
			t.VerifiedBy = actorID
		}

		if err := s.store.UpdateTicketStatus(ctx, tx, t); err != nil {
			return err
		}

		event := &TicketStateEvent{
			TicketID:  t.ID,
			TenantID:  t.TenantID,
			FromState: fromState,
			ToState:   params.ToState,
			ActorID:   actorID,
			Reason:    params.Reason,
			Evidence:  params.EvidenceIDs,
			Version:   t.Version,
		}
		if err := s.store.InsertStateEvent(ctx, tx, event); err != nil {
			return err
		}

		if err := s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			ID:           newID("aud"),
			EventType:    audit.EventTypeMutation,
			TenantID:     t.TenantID,
			ActorID:      actorID,
			Action:       "ticket.transitioned",
			ResourceType: "ticket",
			ResourceID:   t.ID,
			NewState:     marshalJSON(t),
		}); err != nil {
			return fmt.Errorf("audit: %w", err)
		}

		result = t
		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

func (s *TicketService) BlockTicket(ctx context.Context, tenantID, ticketID string, block TicketBlock, actorID string) (*Ticket, error) {
	if block.Type == "" || block.Reason == "" {
		return nil, ErrBlockerRequired
	}
	if !isValidBlockerType(block.Type) {
		return nil, fmt.Errorf("%w: %s", ErrInvalidBlockerType, block.Type)
	}

	block.CreatedBy = actorID
	block.CreatedAt = time.Now().UTC()

	var result *Ticket

	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		t, err := s.store.GetTicketForUpdate(ctx, tx, tenantID, ticketID)
		if err != nil {
			return err
		}

		if err := s.authorizeProperty(ctx, tenantID, t.PropertyID); err != nil {
			return err
		}

		if t.Status == StateBlocked {
			return ErrAlreadyBlocked
		}
		if IsTerminalState(t.Status) {
			return ErrTicketTerminal
		}

		fromState := t.Status

		if err := s.store.SetTicketBlocker(ctx, tx, t, &block); err != nil {
			return err
		}

		event := &TicketStateEvent{
			TicketID:  t.ID,
			TenantID:  t.TenantID,
			FromState: fromState,
			ToState:   StateBlocked,
			ActorID:   actorID,
			Reason:    block.Reason,
			Version:   t.Version,
		}
		if err := s.store.InsertStateEvent(ctx, tx, event); err != nil {
			return err
		}

		if err := s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			ID:           newID("aud"),
			EventType:    audit.EventTypeMutation,
			TenantID:     t.TenantID,
			ActorID:      actorID,
			Action:       "ticket.blocked",
			ResourceType: "ticket",
			ResourceID:   t.ID,
			NewState:     marshalJSON(t),
		}); err != nil {
			return fmt.Errorf("audit: %w", err)
		}

		result = t
		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

func (s *TicketService) UnblockTicket(ctx context.Context, tenantID, ticketID string, reason string, actorID string) (*Ticket, error) {
	var result *Ticket

	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		t, err := s.store.GetTicketForUpdate(ctx, tx, tenantID, ticketID)
		if err != nil {
			return err
		}

		if err := s.authorizeProperty(ctx, tenantID, t.PropertyID); err != nil {
			return err
		}

		if t.Status != StateBlocked {
			return ErrTicketNotBlocked
		}

		targetState := StateScheduled
		if t.Type == TypeIncident {
			targetState = StateInProgress
		}

		if err := s.store.ClearTicketBlocker(ctx, tx, t, targetState); err != nil {
			return err
		}

		event := &TicketStateEvent{
			TicketID:  t.ID,
			TenantID:  t.TenantID,
			FromState: StateBlocked,
			ToState:   targetState,
			ActorID:   actorID,
			Reason:    reason,
			Version:   t.Version,
		}
		if err := s.store.InsertStateEvent(ctx, tx, event); err != nil {
			return err
		}

		if err := s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			ID:           newID("aud"),
			EventType:    audit.EventTypeMutation,
			TenantID:     t.TenantID,
			ActorID:      actorID,
			Action:       "ticket.unblocked",
			ResourceType: "ticket",
			ResourceID:   t.ID,
			NewState:     marshalJSON(t),
		}); err != nil {
			return fmt.Errorf("audit: %w", err)
		}

		result = t
		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

func (s *TicketService) CancelTicket(ctx context.Context, tenantID, ticketID string, reason string, actorID string) (*Ticket, error) {
	var result *Ticket

	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		t, err := s.store.GetTicketForUpdate(ctx, tx, tenantID, ticketID)
		if err != nil {
			return err
		}

		if err := s.authorizeProperty(ctx, tenantID, t.PropertyID); err != nil {
			return err
		}

		if IsTerminalState(t.Status) {
			return ErrTicketTerminal
		}

		if !CanTransition(t.Status, StateCancelled) {
			return ErrInvalidTransition
		}

		fromState := t.Status
		t.Status = StateCancelled
		t.Version++

		if err := s.store.UpdateTicketStatus(ctx, tx, t); err != nil {
			return err
		}

		event := &TicketStateEvent{
			TicketID:  t.ID,
			TenantID:  t.TenantID,
			FromState: fromState,
			ToState:   StateCancelled,
			ActorID:   actorID,
			Reason:    reason,
			Version:   t.Version,
		}
		if err := s.store.InsertStateEvent(ctx, tx, event); err != nil {
			return err
		}

		if err := s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			ID:           newID("aud"),
			EventType:    audit.EventTypeMutation,
			TenantID:     t.TenantID,
			ActorID:      actorID,
			Action:       "ticket.cancelled",
			ResourceType: "ticket",
			ResourceID:   t.ID,
			NewState:     marshalJSON(t),
		}); err != nil {
			return fmt.Errorf("audit: %w", err)
		}

		result = t
		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

func (s *TicketService) ReopenTicket(ctx context.Context, tenantID, ticketID string, reason string, actorID string) (*Ticket, error) {
	if reason == "" {
		return nil, ErrReopenRequiresReason
	}

	original, err := s.store.GetTicket(ctx, tenantID, ticketID)
	if err != nil {
		return nil, err
	}

	if err := s.authorizeProperty(ctx, tenantID, original.PropertyID); err != nil {
		return nil, err
	}

	if original.Status != StateClosed && original.Status != StateCancelled {
		return nil, fmt.Errorf("only closed or cancelled tickets can be reopened, current state: %s", original.Status)
	}

	followUp := &Ticket{
		TenantID:           original.TenantID,
		PropertyID:         original.PropertyID,
		Type:               original.Type,
		Status:             StateDraft,
		Reason:             reason,
		RequestedWindow:    original.RequestedWindow,
		ChecklistVersionID: original.ChecklistVersionID,
		CreatedBy:          actorID,
	}

	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertTicket(ctx, tx, followUp); err != nil {
			return err
		}

		orig, err := s.store.GetTicketForUpdate(ctx, tx, tenantID, ticketID)
		if err != nil {
			return err
		}

		orig.FollowUpTicketID = followUp.ID
		orig.ReopenReason = reason
		orig.Version++

		if err := s.store.UpdateTicketStatus(ctx, tx, orig); err != nil {
			return err
		}

		if err := s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			ID:           newID("aud"),
			EventType:    audit.EventTypeMutation,
			TenantID:     followUp.TenantID,
			ActorID:      actorID,
			Action:       "ticket.reopened",
			ResourceType: "ticket",
			ResourceID:   followUp.ID,
			NewState:     marshalJSON(followUp),
		}); err != nil {
			return fmt.Errorf("audit: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return followUp, nil
}

func (s *TicketService) ListStateEvents(ctx context.Context, tenantID, ticketID string) ([]TicketStateEvent, error) {
	t, err := s.store.GetTicket(ctx, tenantID, ticketID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeProperty(ctx, tenantID, t.PropertyID); err != nil {
		return nil, err
	}
	return s.store.ListStateEvents(ctx, tenantID, ticketID)
}

func (s *TicketService) SyncChecklist(ctx context.Context, tenantID, ticketID string, items []TicketChecklistItem, actorID string) ([]TicketChecklistItem, error) {
	t, err := s.store.GetTicket(ctx, tenantID, ticketID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeProperty(ctx, tenantID, t.PropertyID); err != nil {
		return nil, err
	}

	existing, err := s.store.ListChecklistItems(ctx, tenantID, ticketID)
	if err != nil {
		return nil, err
	}

	var result []TicketChecklistItem

	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		for i := range items {
			items[i].TicketID = ticketID
			items[i].TenantID = tenantID

			found := false
			for _, existingItem := range existing {
				if existingItem.TemplateItemIndex == items[i].TemplateItemIndex {
					if existingItem.EvidenceRequired && !items[i].EvidenceRequired {
						return fmt.Errorf("%w: %s", ErrEvidenceRequirementLocks, items[i].Label)
					}
					items[i].ID = existingItem.ID
					items[i].Version = existingItem.Version
					if err := s.store.UpdateChecklistItem(ctx, tx, &items[i]); err != nil {
						return err
					}
					found = true
					break
				}
			}
			if !found {
				if err := s.store.InsertChecklistItem(ctx, tx, &items[i]); err != nil {
					return err
				}
			}
			result = append(result, items[i])
		}
		if err := s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			ID:           newID("aud"),
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "ticket.checklist_synced",
			ResourceType: "ticket",
			ResourceID:   ticketID,
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

func (s *TicketService) ListChecklistItems(ctx context.Context, tenantID, ticketID string) ([]TicketChecklistItem, error) {
	t, err := s.store.GetTicket(ctx, tenantID, ticketID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeProperty(ctx, tenantID, t.PropertyID); err != nil {
		return nil, err
	}
	return s.store.ListChecklistItems(ctx, tenantID, ticketID)
}

func (s *TicketService) RegisterEvidence(ctx context.Context, tenantID, ticketID string, params RegisterEvidenceParams, actorID string) (*EvidenceRecord, error) {
	if err := ValidateEvidenceRegistrationParams(params); err != nil {
		return nil, err
	}

	var result *EvidenceRecord

	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		t, err := s.store.GetTicketForUpdate(ctx, tx, tenantID, ticketID)
		if err != nil {
			return err
		}
		if err := s.authorizeProperty(ctx, tenantID, t.PropertyID); err != nil {
			return err
		}
		if IsTerminalState(t.Status) {
			return ErrTicketTerminal
		}

		var item *TicketChecklistItem
		if params.ChecklistItemID != "" {
			var err error
			item, err = getChecklistItem(ctx, tx, tenantID, params.ChecklistItemID)
			if err != nil {
				return err
			}
			if item.TicketID != ticketID {
				return ErrChecklistItemNotFound
			}
		}

		// Registering the same accepted content again is a no-op: the same
		// immutable record and its stable hash are returned. It still links
		// the existing record to the checklist item if that link is new, so
		// evidence submitted before an item's requirement was known can be
		// backfilled onto it.
		existing, err := s.store.FindEvidenceByHash(ctx, tx, tenantID, ticketID, params.ContentHash)
		if err == nil {
			result = existing
			if item != nil {
				if err := linkEvidenceToChecklistItem(ctx, s.store, tx, item, existing.ID); err != nil {
					return err
				}
			}
			if err := s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
				ID:           newID("aud"),
				EventType:    audit.EventTypeMutation,
				TenantID:     existing.TenantID,
				ActorID:      actorID,
				Action:       "ticket.evidence.registered",
				ResourceType: "ticket",
				ResourceID:   existing.TicketID,
				NewState:     marshalJSON(existing),
			}); err != nil {
				return fmt.Errorf("audit: %w", err)
			}
			return nil
		}
		if err != ErrEvidenceNotFound {
			return err
		}

		rec := &EvidenceRecord{
			TenantID:        tenantID,
			TicketID:        ticketID,
			ChecklistItemID: params.ChecklistItemID,
			ObjectID:        params.ObjectID,
			ContentHash:     params.ContentHash,
			FileName:        params.FileName,
			ContentType:     params.ContentType,
			SizeBytes:       params.SizeBytes,
			Status:          EvidenceStatusAccepted,
			CapturedBy:      actorID,
		}
		if err := s.store.InsertEvidence(ctx, tx, rec); err != nil {
			return err
		}
		result = rec
		if item != nil {
			if err := linkEvidenceToChecklistItem(ctx, s.store, tx, item, rec.ID); err != nil {
				return err
			}
		}
		if err := s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			ID:           newID("aud"),
			EventType:    audit.EventTypeMutation,
			TenantID:     rec.TenantID,
			ActorID:      actorID,
			Action:       "ticket.evidence.registered",
			ResourceType: "ticket",
			ResourceID:   rec.TicketID,
			NewState:     marshalJSON(rec),
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

// linkEvidenceToChecklistItem records evidenceID on the checklist item's own
// evidence_ids so RequiredEvidenceBlocking can resolve it at completion time.
// Registering evidence with a checklist_item_id otherwise had no effect on
// the item itself, leaving evidence-required completion permanently blocked.
// The link is idempotent: an evidence id already present is left as-is.
func linkEvidenceToChecklistItem(ctx context.Context, store *TicketStore, q querier, item *TicketChecklistItem, evidenceID string) error {
	for _, id := range item.EvidenceIDs {
		if id == evidenceID {
			return nil
		}
	}
	item.EvidenceIDs = append(item.EvidenceIDs, evidenceID)
	if err := store.UpdateChecklistItem(ctx, q, item); err != nil {
		return fmt.Errorf("link evidence to checklist item: %w", err)
	}
	return nil
}

func (s *TicketService) GetEvidence(ctx context.Context, tenantID, evidenceID string) (*EvidenceRecord, error) {
	e, err := s.store.GetEvidence(ctx, tenantID, evidenceID)
	if err != nil {
		return nil, err
	}
	t, err := s.store.GetTicket(ctx, tenantID, e.TicketID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeProperty(ctx, tenantID, t.PropertyID); err != nil {
		return nil, err
	}
	return e, nil
}

func (s *TicketService) ListEvidence(ctx context.Context, tenantID, ticketID string) ([]EvidenceRecord, error) {
	t, err := s.store.GetTicket(ctx, tenantID, ticketID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeProperty(ctx, tenantID, t.PropertyID); err != nil {
		return nil, err
	}
	return s.store.ListEvidence(ctx, tenantID, ticketID)
}

// ClassifyIncident assigns a severity to an incident ticket, derives the
// notification intent from the response policy, and durably queues the
// on-call and owner alerts inside the same transaction as the ticket update.
func (s *TicketService) ClassifyIncident(ctx context.Context, tenantID, ticketID, severity string, actorID string) (*Ticket, error) {
	if err := ValidateSeverity(severity); err != nil {
		return nil, err
	}

	var result *Ticket
	queued := 0

	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		t, err := s.store.GetTicketForUpdate(ctx, tx, tenantID, ticketID)
		if err != nil {
			return err
		}
		if err := s.authorizeProperty(ctx, tenantID, t.PropertyID); err != nil {
			return err
		}
		if t.Type != TypeIncident {
			return fmt.Errorf("%w: ticket type %s", ErrNotIncident, t.Type)
		}
		if IsTerminalState(t.Status) {
			return ErrTicketTerminal
		}

		t.Severity = severity
		t.NotificationIntent = NotificationIntentForSeverity(severity)
		t.Version++

		if err := s.store.UpdateTicketStatus(ctx, tx, t); err != nil {
			return err
		}

		for _, target := range SeverityAlertTargets(severity) {
			alert := &IncidentAlert{
				TenantID:   t.TenantID,
				PropertyID: t.PropertyID,
				TicketID:   t.ID,
				Severity:   severity,
				Target:     target,
				Policy:     IncidentAlertPolicy(severity),
				Status:     AlertStatusQueued,
			}
			if err := s.store.InsertIncidentAlert(ctx, tx, alert); err != nil {
				return err
			}
			queued++
		}

		if err := s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			ID:           newID("aud"),
			EventType:    audit.EventTypeMutation,
			TenantID:     t.TenantID,
			ActorID:      actorID,
			Action:       "ticket.incident.classified",
			ResourceType: "ticket",
			ResourceID:   t.ID,
			NewState:     marshalJSON(t),
		}); err != nil {
			return fmt.Errorf("audit: %w", err)
		}

		result = t
		return nil
	})
	if err != nil {
		return nil, err
	}

	logging.Info(ctx, "incident classified and alerts queued",
		"ticket_id", result.ID,
		"severity", severity,
		"alerts_queued", queued,
	)

	return result, nil
}

func (s *TicketService) ListIncidentAlerts(ctx context.Context, tenantID, propertyID, status string) ([]IncidentAlert, error) {
	if err := s.authorizeProperty(ctx, tenantID, propertyID); err != nil {
		return nil, err
	}
	return s.store.ListIncidentAlerts(ctx, tenantID, propertyID, status)
}

func (s *TicketService) ListIncidentAlertsForTicket(ctx context.Context, tenantID, ticketID string) ([]IncidentAlert, error) {
	t, err := s.store.GetTicket(ctx, tenantID, ticketID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeProperty(ctx, tenantID, t.PropertyID); err != nil {
		return nil, err
	}
	return s.store.ListIncidentAlertsForTicket(ctx, s.pool, tenantID, ticketID)
}

// StartServiceRecovery opens a service recovery for an incident. It preserves
// the original failure (reason, severity and evidence hashes) on the recovery
// record, creates a linked recovery ticket for the rework, and records the
// responsibility and rework cost atomically with the original incident link.
func (s *TicketService) StartServiceRecovery(ctx context.Context, tenantID, incidentTicketID string, params RecoveryParams, actorID string) (*ServiceRecovery, error) {
	if params.Reason == "" {
		return nil, fmt.Errorf("recovery reason is required")
	}
	if err := ValidateRecoveryParams(params); err != nil {
		return nil, err
	}

	var result *ServiceRecovery

	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		t, err := s.store.GetTicketForUpdate(ctx, tx, tenantID, incidentTicketID)
		if err != nil {
			return err
		}
		if err := s.authorizeProperty(ctx, tenantID, t.PropertyID); err != nil {
			return err
		}
		if t.Type != TypeIncident {
			return fmt.Errorf("%w: ticket type %s", ErrNotIncident, t.Type)
		}
		if IsTerminalState(t.Status) {
			return ErrTicketTerminal
		}

		evidence, err := listEvidence(ctx, tx, tenantID, incidentTicketID)
		if err != nil {
			return err
		}

		followUp := &Ticket{
			TenantID:   t.TenantID,
			PropertyID: t.PropertyID,
			Type:       t.Type,
			Status:     StateDraft,
			Reason:     params.Reason,
			CreatedBy:  actorID,
		}
		if err := insertTicket(ctx, tx, followUp); err != nil {
			return err
		}

		rec := BuildRecovery(t, evidence, params)
		rec.FollowUpTicketID = followUp.ID
		rec.CreatedBy = actorID
		if err := s.store.InsertServiceRecovery(ctx, tx, &rec); err != nil {
			return err
		}

		t.FollowUpTicketID = followUp.ID
		t.Version++
		if err := s.store.UpdateTicketStatus(ctx, tx, t); err != nil {
			return err
		}

		if err := s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			ID:           newID("aud"),
			EventType:    audit.EventTypeMutation,
			TenantID:     rec.TenantID,
			ActorID:      actorID,
			Action:       "incident.recovery.started",
			ResourceType: "service_recovery",
			ResourceID:   rec.ID,
			NewState:     marshalJSON(rec),
		}); err != nil {
			return fmt.Errorf("audit: %w", err)
		}

		result = &rec
		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (s *TicketService) GetServiceRecovery(ctx context.Context, tenantID, recoveryID string) (*ServiceRecovery, error) {
	r, err := s.store.GetServiceRecovery(ctx, tenantID, recoveryID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeProperty(ctx, tenantID, r.PropertyID); err != nil {
		return nil, err
	}
	return r, nil
}

func (s *TicketService) ListServiceRecoveries(ctx context.Context, tenantID, incidentTicketID string) ([]ServiceRecovery, error) {
	t, err := s.store.GetTicket(ctx, tenantID, incidentTicketID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeProperty(ctx, tenantID, t.PropertyID); err != nil {
		return nil, err
	}
	return s.store.ListServiceRecoveries(ctx, tenantID, incidentTicketID)
}

func (s *TicketService) CloseServiceRecovery(ctx context.Context, tenantID, recoveryID string, actorID string) (*ServiceRecovery, error) {
	var result *ServiceRecovery

	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		r, err := s.store.GetServiceRecovery(ctx, tenantID, recoveryID)
		if err != nil {
			return err
		}
		if err := s.authorizeProperty(ctx, tenantID, r.PropertyID); err != nil {
			return err
		}
		if r.Status != RecoveryStatusOpen {
			return ErrRecoveryInactive
		}
		if err := s.store.CloseServiceRecovery(ctx, tx, r); err != nil {
			return err
		}
		if err := s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			ID:           newID("aud"),
			EventType:    audit.EventTypeMutation,
			TenantID:     r.TenantID,
			ActorID:      actorID,
			Action:       "incident.recovery.closed",
			ResourceType: "service_recovery",
			ResourceID:   r.ID,
			NewState:     marshalJSON(r),
		}); err != nil {
			return fmt.Errorf("audit: %w", err)
		}
		result = r
		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

// assertEvidenceGates blocks ticket submission and completion until every
// checklist item that requires evidence carries clean, accepted evidence.
func (s *TicketService) assertEvidenceGates(ctx context.Context, tx pgx.Tx, t *Ticket) error {
	items, err := listChecklistItems(ctx, tx, t.TenantID, t.ID)
	if err != nil {
		return err
	}

	hasRequired := false
	for _, item := range items {
		if item.EvidenceRequired {
			hasRequired = true
			break
		}
	}
	if !hasRequired {
		return nil
	}

	evidence, err := listEvidence(ctx, tx, t.TenantID, t.ID)
	if err != nil {
		return err
	}

	return RequiredEvidenceBlocking(items, evidenceByIDIndex(evidence))
}

func (s *TicketService) IdempotentSyncChecklist(ctx context.Context, tenantID, ticketID, syncKey string, items []TicketChecklistItem, actorID string) (*SyncResultView, error) {
	if err := validateSyncItems(items); err != nil {
		return nil, err
	}

	t, err := s.store.GetTicket(ctx, tenantID, ticketID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeProperty(ctx, tenantID, t.PropertyID); err != nil {
		return nil, err
	}

	payloadHash := syncPayloadHash(items)

	var result *SyncResultView

	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		existing, err := s.store.FindChecklistSyncRecord(ctx, tx, syncKey)
		if err != nil {
			return err
		}
		if existing != nil {
			if existing.PayloadHash != payloadHash {
				return fmt.Errorf("%w: key %s", ErrSyncKeyConflict, syncKey)
			}
			ids := parseSyncResult(existing.Result)
			synced := make([]TicketChecklistItem, len(ids))
			for i, id := range ids {
				item, err := getChecklistItem(ctx, tx, tenantID, id)
				if err != nil {
					return err
				}
				synced[i] = *item
			}
			result = &SyncResultView{Items: synced, Replay: true}
			if err := s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
				ID:           newID("aud"),
				EventType:    audit.EventTypeMutation,
				TenantID:     tenantID,
				ActorID:      actorID,
				Action:       "ticket.checklist.idempotent_synced",
				ResourceType: "ticket",
				ResourceID:   ticketID,
			}); err != nil {
				return fmt.Errorf("audit: %w", err)
			}
			return nil
		}

		serverItems, err := listChecklistItems(ctx, tx, tenantID, ticketID)
		if err != nil {
			return err
		}

		merged, conflicts := MergeSync(items, serverItems)

		for i := range merged {
			merged[i].TicketID = ticketID
			merged[i].TenantID = tenantID

			if merged[i].ID == "" {
				if err := s.store.InsertChecklistItem(ctx, tx, &merged[i]); err != nil {
					return err
				}
			} else {
				if err := s.store.UpdateChecklistItemNoConflict(ctx, tx, &merged[i]); err != nil {
					return err
				}
			}
		}

		for i := range conflicts {
			conflicts[i].TenantID = tenantID
			conflicts[i].TicketID = ticketID
			if err := s.store.InsertSyncConflict(ctx, tx, &conflicts[i]); err != nil {
				return err
			}
		}

		rec := &ChecklistSyncRecord{
			SyncKey:     syncKey,
			TenantID:    tenantID,
			TicketID:    ticketID,
			PayloadHash: payloadHash,
			Result:      buildSyncResult(merged),
		}
		if err := s.store.InsertChecklistSyncRecord(ctx, tx, rec); err != nil {
			return err
		}

		result = &SyncResultView{Items: merged, Conflicts: conflicts}
		if err := s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			ID:           newID("aud"),
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "ticket.checklist.idempotent_synced",
			ResourceType: "ticket",
			ResourceID:   ticketID,
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

func (s *TicketService) ListSyncConflicts(ctx context.Context, tenantID, ticketID string, resolved bool) ([]SyncConflict, error) {
	t, err := s.store.GetTicket(ctx, tenantID, ticketID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeProperty(ctx, tenantID, t.PropertyID); err != nil {
		return nil, err
	}
	return s.store.ListSyncConflicts(ctx, tenantID, ticketID, resolved)
}

func (s *TicketService) ResolveSyncConflict(ctx context.Context, tenantID, conflictID, resolution string, actorID string) (*SyncConflict, error) {
	var result *SyncConflict

	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		c, err := s.store.GetSyncConflict(ctx, tenantID, conflictID)
		if err != nil {
			return err
		}
		if c.Resolved {
			return ErrSyncConflictNotOpen
		}

		t, err := getTicket(ctx, tx, tenantID, c.TicketID)
		if err != nil {
			return err
		}
		if err := s.authorizeProperty(ctx, tenantID, t.PropertyID); err != nil {
			return err
		}

		if err := s.store.ResolveSyncConflict(ctx, tx, c, resolution, actorID); err != nil {
			return err
		}

		if err := s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			ID:           newID("aud"),
			EventType:    audit.EventTypeMutation,
			TenantID:     c.TenantID,
			ActorID:      actorID,
			Action:       "ticket.sync_conflict.resolved",
			ResourceType: "sync_conflict",
			ResourceID:   c.ID,
		}); err != nil {
			return fmt.Errorf("audit: %w", err)
		}

		result = c
		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

func (s *TicketService) QueueOfflineEvidence(ctx context.Context, tenantID, ticketID string, payload OfflineEvidencePayload, actorID string) (*QueuedOfflineEvidence, error) {
	if err := ValidateOfflineEvidenceParams(payload.ContentHash, payload.SizeBytes); err != nil {
		return nil, err
	}
	if err := ValidateLanguage(payload.Language); err != nil {
		return nil, err
	}

	t, err := s.store.GetTicket(ctx, tenantID, ticketID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeProperty(ctx, tenantID, t.PropertyID); err != nil {
		return nil, err
	}

	existing, _ := s.store.FindQueuedOfflineEvidence(ctx, tenantID, ticketID, payload.ContentHash)
	if existing != nil {
		return existing, nil
	}

	e := &QueuedOfflineEvidence{
		TenantID:    tenantID,
		TicketID:    ticketID,
		ContentHash: payload.ContentHash,
		FileName:    payload.FileName,
		ContentType: payload.ContentType,
		SizeBytes:   payload.SizeBytes,
		Status:      OfflineEvidenceQueued,
		CapturedBy:  actorID,
	}

	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertQueuedOfflineEvidence(ctx, tx, e); err != nil {
			return err
		}
		if err := s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			ID:           newID("aud"),
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "ticket.offline_evidence.queued",
			ResourceType: "ticket",
			ResourceID:   ticketID,
		}); err != nil {
			return fmt.Errorf("audit: %w", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return e, nil
}

func (s *TicketService) SyncOfflineEvidence(ctx context.Context, tenantID, ticketID string, contentHashes []string, actorID string) ([]EvidenceRecord, error) {
	t, err := s.store.GetTicket(ctx, tenantID, ticketID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeProperty(ctx, tenantID, t.PropertyID); err != nil {
		return nil, err
	}

	var results []EvidenceRecord

	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		for _, hash := range contentHashes {
			queued, err := s.store.FindQueuedOfflineEvidence(ctx, tenantID, ticketID, hash)
			if err != nil {
				if errors.Is(err, ErrOfflineEvidenceNotFound) {
					continue
				}
				return err
			}

			existing, err := s.store.FindEvidenceByHash(ctx, tx, tenantID, ticketID, hash)
			if err == nil {
				results = append(results, *existing)
				if markErr := s.store.MarkQueuedEvidenceUploaded(ctx, tx, queued); markErr != nil {
					return markErr
				}
				if err := s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
					ID:           newID("aud"),
					EventType:    audit.EventTypeMutation,
					TenantID:     existing.TenantID,
					ActorID:      actorID,
					Action:       "ticket.offline_evidence.synced",
					ResourceType: "ticket",
					ResourceID:   existing.TicketID,
				}); err != nil {
					return fmt.Errorf("audit: %w", err)
				}
				continue
			}
			if !errors.Is(err, ErrEvidenceNotFound) {
				return err
			}

			rec := &EvidenceRecord{
				TenantID:    tenantID,
				TicketID:    ticketID,
				ContentHash: queued.ContentHash,
				FileName:    queued.FileName,
				ContentType: queued.ContentType,
				SizeBytes:   queued.SizeBytes,
				Status:      EvidenceStatusAccepted,
				CapturedBy:  queued.CapturedBy,
			}
			if err := s.store.InsertEvidence(ctx, tx, rec); err != nil {
				return err
			}
			if err := s.store.MarkQueuedEvidenceUploaded(ctx, tx, queued); err != nil {
				return err
			}
			results = append(results, *rec)
			if err := s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
				ID:           newID("aud"),
				EventType:    audit.EventTypeMutation,
				TenantID:     rec.TenantID,
				ActorID:      actorID,
				Action:       "ticket.offline_evidence.synced",
				ResourceType: "ticket",
				ResourceID:   rec.TicketID,
			}); err != nil {
				return fmt.Errorf("audit: %w", err)
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return results, nil
}

func (s *TicketService) ListQueuedOfflineEvidence(ctx context.Context, tenantID, ticketID, status string) ([]QueuedOfflineEvidence, error) {
	t, err := s.store.GetTicket(ctx, tenantID, ticketID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeProperty(ctx, tenantID, t.PropertyID); err != nil {
		return nil, err
	}
	return s.store.ListQueuedOfflineEvidence(ctx, tenantID, ticketID, status)
}

func (s *TicketService) authorizeProperty(ctx context.Context, tenantID, propertyID string) error {
	if s.authorizer == nil {
		return nil
	}
	return s.authorizer.RequireResourceAccess(ctx, tenantID, "property", propertyID)
}

func (s *TicketService) appendAudit(ctx context.Context, event audit.AuditEvent) {
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

func isValidBlockerType(t string) bool {
	switch t {
	case BlockerTypeAccess, BlockerTypeSafety, BlockerTypeParts,
		BlockerTypeApproval, BlockerTypeWeather, BlockerTypeCompliance,
		BlockerTypeExternal:
		return true
	default:
		return false
	}
}

func marshalJSON(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("{}")
	}
	return data
}
