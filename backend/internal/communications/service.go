package communications

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
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CommunicationsService struct {
	pool       *pgxpool.Pool
	store      *CommunicationsStore
	auditStore *audit.AuditStore
}

func NewCommunicationsService(pool *pgxpool.Pool) *CommunicationsService {
	return &CommunicationsService{
		pool:       pool,
		store:      NewCommunicationsStore(pool),
		auditStore: audit.NewAuditStore(pool),
	}
}

func (s *CommunicationsService) WithAudit(a *audit.AuditStore) *CommunicationsService {
	s.auditStore = a
	return s
}

func (s *CommunicationsService) appendAudit(ctx context.Context, event audit.AuditEvent) {
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

func marshalJSON(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("{}")
	}
	return data
}

func defaultPreferences(tenantID, recipientID, audience string) *CommunicationPreferences {
	return &CommunicationPreferences{
		TenantID:             tenantID,
		RecipientID:          recipientID,
		Audience:             audience,
		ConsentTransactional: true,
		ConsentUrgent:        true,
		ConsentMarketing:     false,
		ConsentSponsored:     false,
		Channel:              ChannelPush,
		Severity:             SeverityNormal,
	}
}

// --- templates (COM-004) ---

// CreateTemplate registers a curated template with its fixed audience and
// consent class. Owner and guest templates are separate records and are never
// interchangeable.
func (s *CommunicationsService) CreateTemplate(ctx context.Context, tenantID string, params TemplateParams, actorID string) (*MessageTemplate, error) {
	if params.TemplateKey == "" {
		return nil, ErrTemplateKeyRequired
	}
	if !IsValidAudience(params.Audience) {
		return nil, ErrInvalidAudience
	}
	if !IsValidConsentClass(params.ConsentClass) {
		return nil, ErrInvalidConsentClass
	}
	if params.Channel != "" && !IsValidChannel(params.Channel) {
		return nil, ErrInvalidChannel
	}
	if params.Severity != "" && !IsValidSeverity(params.Severity) {
		return nil, ErrInvalidSeverity
	}

	t := &MessageTemplate{
		TenantID:     tenantID,
		TemplateKey:  params.TemplateKey,
		Audience:     params.Audience,
		ConsentClass: params.ConsentClass,
		Channel:      params.Channel,
		Severity:     params.Severity,
		Status:       TemplateStatusActive,
	}

	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertTemplate(ctx, tx, t); err != nil {
			if isUniqueViolation(err) {
				return ErrTemplateExists
			}
			return err
		}
		if s.auditStore != nil {
			if err := s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
				EventType:    audit.EventTypeMutation,
				TenantID:     tenantID,
				ActorID:      actorID,
				Action:       "communications.template.created",
				ResourceType: "message_template",
				ResourceID:   t.ID,
				NewState:     marshalJSON(t),
			}); err != nil {
				return fmt.Errorf("audit: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return t, nil
}

// AddTemplateVersion appends the next version of a template in one language.
// The highest version per language is the active rendering (COM-004).
func (s *CommunicationsService) AddTemplateVersion(ctx context.Context, tenantID, templateID string, params TemplateVersionParams, actorID string) (*TemplateVersion, error) {
	if !IsValidLanguage(params.Language) {
		return nil, ErrInvalidLanguage
	}
	if params.Subject == "" || params.Body == "" {
		return nil, ErrTemplateContentRequired
	}

	t, err := s.store.GetTemplateByID(ctx, tenantID, templateID)
	if err != nil {
		return nil, err
	}

	version, err := s.store.NextTemplateVersion(ctx, tenantID, templateID)
	if err != nil {
		return nil, err
	}

	v := &TemplateVersion{
		TenantID:   tenantID,
		TemplateID: t.ID,
		Version:    version,
		Language:   params.Language,
		Subject:    params.Subject,
		Body:       params.Body,
	}

	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertTemplateVersion(ctx, tx, v); err != nil {
			return err
		}
		if s.auditStore != nil {
			if err := s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
				EventType:    audit.EventTypeMutation,
				TenantID:     tenantID,
				ActorID:      actorID,
				Action:       "communications.template.versioned",
				ResourceType: "message_template_version",
				ResourceID:   v.ID,
				NewState:     marshalJSON(v),
			}); err != nil {
				return fmt.Errorf("audit: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return v, nil
}

// GetTemplateByKey loads a curated template. Cross-tenant access fails closed.
func (s *CommunicationsService) GetTemplateByKey(ctx context.Context, tenantID, templateKey string) (*MessageTemplate, error) {
	return s.store.GetTemplateByKey(ctx, tenantID, templateKey)
}

// ListTemplates returns templates, optionally restricted to one audience so
// owner and guest templates never mix in a single listing.
func (s *CommunicationsService) ListTemplates(ctx context.Context, tenantID, audience string) ([]MessageTemplate, error) {
	if audience != "" && !IsValidAudience(audience) {
		return nil, ErrInvalidAudience
	}
	return s.store.ListTemplates(ctx, tenantID, audience)
}

// ResolveTemplateContent returns the active localized version of a template.
// English is used when the requested language has no version, and the highest
// version number wins.
func (s *CommunicationsService) ResolveTemplateContent(ctx context.Context, tenantID, templateKey, language string) (*ResolvedTemplate, error) {
	if language != "" && !IsValidLanguage(language) {
		return nil, ErrInvalidLanguage
	}
	t, err := s.store.GetTemplateByKey(ctx, tenantID, templateKey)
	if err != nil {
		return nil, err
	}

	lang := language
	if lang == "" {
		lang = LanguageEnglish
	}

	v, err := s.store.LatestTemplateVersion(ctx, tenantID, t.ID, lang)
	if err != nil {
		if errors.Is(err, ErrTemplateVersionMissing) && lang != LanguageEnglish {
			fallback, ferr := s.store.LatestTemplateVersion(ctx, tenantID, t.ID, LanguageEnglish)
			if ferr != nil {
				return nil, ferr
			}
			return &ResolvedTemplate{
				Template:     *t,
				Version:      fallback.Version,
				Language:     fallback.Language,
				Subject:      fallback.Subject,
				Body:         fallback.Body,
				FallbackFrom: lang,
			}, nil
		}
		return nil, err
	}

	return &ResolvedTemplate{
		Template: *t,
		Version:  v.Version,
		Language: v.Language,
		Subject:  v.Subject,
		Body:     v.Body,
	}, nil
}

// --- preferences (COM-001, COM-002) ---

// SetPreferences records or updates separate consent and routing preferences
// for one (recipient, audience) pair. Owner and guest preferences never share
// a row, so owner consent can never authorize guest communication.
func (s *CommunicationsService) SetPreferences(ctx context.Context, tenantID string, params PreferencesParams, actorID string) (*CommunicationPreferences, error) {
	if params.RecipientID == "" {
		return nil, fmt.Errorf("%w: recipient is required", ErrInvalidPreferences)
	}
	if !IsValidAudience(params.Audience) {
		return nil, ErrInvalidAudience
	}
	if params.Channel != "" && !IsValidChannel(params.Channel) {
		return nil, ErrInvalidChannel
	}
	if params.Severity != "" && !IsValidSeverity(params.Severity) {
		return nil, ErrInvalidSeverity
	}
	if params.QuietHoursStartMinute < 0 || params.QuietHoursStartMinute > 1439 ||
		params.QuietHoursEndMinute < 0 || params.QuietHoursEndMinute > 1439 {
		return nil, fmt.Errorf("%w: quiet hours must be minutes of day 0-1439", ErrInvalidPreferences)
	}

	prefs := &CommunicationPreferences{
		TenantID:              tenantID,
		RecipientID:           params.RecipientID,
		Audience:              params.Audience,
		ConsentTransactional:  params.ConsentTransactional,
		ConsentUrgent:         params.ConsentUrgent,
		ConsentMarketing:      params.ConsentMarketing,
		ConsentSponsored:      params.ConsentSponsored,
		Channel:               params.Channel,
		Severity:              params.Severity,
		QuietHoursStartMinute: params.QuietHoursStartMinute,
		QuietHoursEndMinute:   params.QuietHoursEndMinute,
		EscalationContacts:    params.EscalationContacts,
	}

	var stored *CommunicationPreferences
	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		stored, err = s.store.UpsertPreferences(ctx, tx, prefs)
		if err != nil {
			return err
		}
		if s.auditStore != nil {
			if err := s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
				EventType:    audit.EventTypeMutation,
				TenantID:     tenantID,
				ActorID:      actorID,
				Action:       "communications.preferences.updated",
				ResourceType: "communication_preferences",
				ResourceID:   stored.ID,
				NewState:     marshalJSON(stored),
			}); err != nil {
				return fmt.Errorf("audit: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return stored, nil
}

// GetPreferences returns the stored preferences for a (recipient, audience)
// pair, or the safe defaults when none were configured.
func (s *CommunicationsService) GetPreferences(ctx context.Context, tenantID, recipientID, audience string) (*CommunicationPreferences, error) {
	if !IsValidAudience(audience) {
		return nil, ErrInvalidAudience
	}
	prefs, err := s.store.GetPreferences(ctx, tenantID, recipientID, audience)
	if errors.Is(err, ErrPreferencesNotFound) {
		return defaultPreferences(tenantID, recipientID, audience), nil
	}
	if err != nil {
		return nil, err
	}
	return prefs, nil
}

func (s *CommunicationsService) preferencesForDelivery(ctx context.Context, tenantID, recipientID, audience string) (*CommunicationPreferences, error) {
	return s.GetPreferences(ctx, tenantID, recipientID, audience)
}

// --- drafts, review and delivery (COM-005, COM-006) ---

// CreateDraft stages a message. Template drafts resolve curated content and
// start approved; free-form AI drafts always require human review and start
// under review (COM-005). The template audience must match the message
// audience, keeping owner and guest audiences separate.
func (s *CommunicationsService) CreateDraft(ctx context.Context, tenantID string, params DraftParams, actorID string) (*CommunicationDraft, error) {
	if params.RecipientID == "" {
		return nil, ErrRecipientRequired
	}
	if !IsValidAudience(params.Audience) {
		return nil, ErrInvalidAudience
	}
	if !IsValidSource(params.Source) {
		return nil, ErrInvalidSource
	}
	if params.ConsentClass != "" && !IsValidConsentClass(params.ConsentClass) {
		return nil, ErrInvalidConsentClass
	}
	if params.Channel != "" && !IsValidChannel(params.Channel) {
		return nil, ErrInvalidChannel
	}
	if params.Severity != "" && !IsValidSeverity(params.Severity) {
		return nil, ErrInvalidSeverity
	}

	draft := &CommunicationDraft{
		TenantID:     tenantID,
		Audience:     params.Audience,
		RecipientID:  params.RecipientID,
		Source:       params.Source,
		ConsentClass: params.ConsentClass,
		Channel:      params.Channel,
		Severity:     params.Severity,
	}

	switch params.Source {
	case SourceTemplate:
		if params.TemplateKey == "" {
			return nil, ErrTemplateKeyRequired
		}
		resolved, err := s.ResolveTemplateContent(ctx, tenantID, params.TemplateKey, params.Language)
		if err != nil {
			return nil, err
		}
		if resolved.Template.Audience != params.Audience {
			return nil, ErrAudienceMismatch
		}
		draft.TemplateKey = params.TemplateKey
		draft.ConsentClass = resolved.Template.ConsentClass
		draft.Subject = resolved.Subject
		draft.Body = resolved.Body
		if draft.Channel == "" {
			draft.Channel = resolved.Template.Channel
		}
		if draft.Severity == "" {
			draft.Severity = resolved.Template.Severity
		}
		draft.Status = DraftStatusApproved
		draft.RequiresReview = false
	case SourceAI:
		if !IsValidConsentClass(params.ConsentClass) {
			return nil, ErrInvalidConsentClass
		}
		if params.Subject == "" || params.Body == "" {
			return nil, fmt.Errorf("%w: AI message requires subject and body", ErrDraftRequiresReview)
		}
		draft.Subject = params.Subject
		draft.Body = params.Body
		if draft.Channel == "" {
			draft.Channel = ChannelPush
		}
		if draft.Severity == "" {
			draft.Severity = SeverityNormal
		}
		draft.Status = DraftStatusUnderReview
		draft.RequiresReview = true
	}

	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertDraft(ctx, tx, draft); err != nil {
			return err
		}
		if s.auditStore != nil {
			if err := s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
				EventType:    audit.EventTypeMutation,
				TenantID:     tenantID,
				ActorID:      actorID,
				Action:       "communications.draft.created",
				ResourceType: "communication_draft",
				ResourceID:   draft.ID,
				NewState:     marshalJSON(draft),
			}); err != nil {
				return fmt.Errorf("audit: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return draft, nil
}

// ReviewDraft records a distinct human review decision. Only drafts that
// require review (free-form AI messages) accept review, and only once.
func (s *CommunicationsService) ReviewDraft(ctx context.Context, tenantID, draftID string, params ReviewParams, actorID string) (*CommunicationDraft, error) {
	if params.ReviewerID == "" {
		return nil, ErrInvalidReviewer
	}
	if params.Decision != ReviewDecisionApproved && params.Decision != ReviewDecisionRejected {
		return nil, ErrReviewDecisionRequired
	}

	draft, err := s.store.GetDraft(ctx, tenantID, draftID)
	if err != nil {
		return nil, err
	}
	if !draft.RequiresReview {
		return nil, ErrReviewNotRequired
	}
	switch draft.Status {
	case DraftStatusApproved, DraftStatusRejected, DraftStatusDelivered:
		return nil, ErrDraftAlreadyReviewed
	}

	review := &CommunicationReview{
		TenantID:   tenantID,
		DraftID:    draftID,
		ReviewerID: params.ReviewerID,
		Decision:   params.Decision,
		Reason:     params.Reason,
	}

	newStatus := DraftStatusApproved
	if params.Decision == ReviewDecisionRejected {
		newStatus = DraftStatusRejected
	}

	var updated *CommunicationDraft
	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertReview(ctx, tx, review); err != nil {
			return err
		}
		var err error
		updated, err = s.store.UpdateDraftStatus(ctx, tx, tenantID, draftID, newStatus)
		if err != nil {
			return err
		}
		if s.auditStore != nil {
			if err := s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
				EventType:    audit.EventTypeMutation,
				TenantID:     tenantID,
				ActorID:      actorID,
				Action:       "communications.draft.reviewed",
				ResourceType: "communication_draft",
				ResourceID:   draftID,
				NewState:     marshalJSON(review),
			}); err != nil {
				return fmt.Errorf("audit: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return updated, nil
}

func (s *CommunicationsService) ListReviews(ctx context.Context, tenantID, draftID string) ([]CommunicationReview, error) {
	return s.store.ListReviews(ctx, tenantID, draftID)
}

// Deliver sends an approved draft now. It fails closed when the draft is a
// free-form AI message that has not been reviewed, when consent for the
// message class is missing, or during quiet hours for a non-urgent message.
// Every attempt is recorded as a delivery-history row (COM-006).
func (s *CommunicationsService) Deliver(ctx context.Context, tenantID, draftID string, actorID string) (*Delivery, error) {
	return s.DeliverAt(ctx, tenantID, draftID, actorID, time.Now().UTC())
}

// DeliverAt is the deterministic form of Deliver, evaluated at the given
// instant so quiet-hours behavior is testable.
func (s *CommunicationsService) DeliverAt(ctx context.Context, tenantID, draftID, actorID string, now time.Time) (*Delivery, error) {
	draft, err := s.store.GetDraft(ctx, tenantID, draftID)
	if err != nil {
		return nil, err
	}

	switch draft.Status {
	case DraftStatusRejected:
		return nil, fmt.Errorf("%w: draft was rejected in review", ErrDraftNotApproved)
	case DraftStatusDelivered:
		return nil, ErrDraftAlreadyDelivered
	case DraftStatusApproved:
	default:
		if draft.RequiresReview {
			return nil, ErrDraftRequiresReview
		}
		return nil, ErrDraftNotApproved
	}

	prefs, err := s.preferencesForDelivery(ctx, tenantID, draft.RecipientID, draft.Audience)
	if err != nil {
		return nil, err
	}
	if !ConsentByClass(prefs, draft.ConsentClass) {
		return nil, ErrConsentNotGranted
	}

	// Quiet hours block non-urgent classes; urgent and critical safety
	// messages always go through (COM-002, COM-003).
	if draft.ConsentClass != ConsentClassUrgent && IsWithinQuietHours(now, prefs.QuietHoursStartMinute, prefs.QuietHoursEndMinute) {
		return nil, ErrQuietHours
	}

	delivery := &Delivery{
		TenantID:     tenantID,
		DraftID:      draft.ID,
		RecipientID:  draft.RecipientID,
		Audience:     draft.Audience,
		ConsentClass: draft.ConsentClass,
		Channel:      draft.Channel,
		Status:       DeliveryStatusQueued,
	}

	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertDelivery(ctx, tx, delivery); err != nil {
			return err
		}
		if _, err := s.store.UpdateDraftStatus(ctx, tx, tenantID, draftID, DraftStatusDelivered); err != nil {
			return err
		}
		if s.auditStore != nil {
			if err := s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
				EventType:    audit.EventTypeMutation,
				TenantID:     tenantID,
				ActorID:      actorID,
				Action:       "communications.delivery.queued",
				ResourceType: "delivery",
				ResourceID:   delivery.ID,
				NewState:     marshalJSON(delivery),
			}); err != nil {
				return fmt.Errorf("audit: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return delivery, nil
}

// RecordDeliveryResult records a delivery outcome. Failures are stored with
// their error so delivery history stays visible (COM-006).
func (s *CommunicationsService) RecordDeliveryResult(ctx context.Context, tenantID, deliveryID, status, failure string) (*Delivery, error) {
	if !IsValidDeliveryStatus(status) {
		return nil, ErrInvalidDeliveryStatus
	}
	var updated *Delivery
	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		updated, err = s.store.UpdateDeliveryStatus(ctx, tx, tenantID, deliveryID, status, failure)
		if err != nil {
			return err
		}
		if s.auditStore != nil {
			if err := s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
				EventType:    audit.EventTypeMutation,
				TenantID:     tenantID,
				ActorID:      "delivery-channel",
				Action:       "communications.delivery.updated",
				ResourceType: "delivery",
				ResourceID:   deliveryID,
				NewState:     marshalJSON(updated),
			}); err != nil {
				return fmt.Errorf("audit: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (s *CommunicationsService) ListDeliveries(ctx context.Context, tenantID, recipientID string) ([]Delivery, error) {
	return s.store.ListDeliveries(ctx, tenantID, recipientID)
}

// --- preview (COM-007) ---

// PreviewTemplate renders the redacted preview of a template. Sensitive access
// details are hidden from the preview (COM-007).
func (s *CommunicationsService) PreviewTemplate(ctx context.Context, tenantID, templateKey, language string) (*Preview, error) {
	resolved, err := s.ResolveTemplateContent(ctx, tenantID, templateKey, language)
	if err != nil {
		return nil, err
	}
	return &Preview{
		TemplateKey: resolved.Template.TemplateKey,
		Audience:    resolved.Template.Audience,
		Language:    resolved.Language,
		Subject:     RedactAccessDetails(resolved.Subject),
		Body:        RedactAccessDetails(resolved.Body),
	}, nil
}

// PreviewDraft renders the redacted preview of a staged draft. Access details
// are hidden from the preview (COM-007).
func (s *CommunicationsService) PreviewDraft(ctx context.Context, tenantID, draftID string) (*Preview, error) {
	draft, err := s.store.GetDraft(ctx, tenantID, draftID)
	if err != nil {
		return nil, err
	}
	return &Preview{
		TemplateKey: draft.TemplateKey,
		Audience:    draft.Audience,
		Subject:     RedactAccessDetails(draft.Subject),
		Body:        RedactAccessDetails(draft.Body),
	}, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}
