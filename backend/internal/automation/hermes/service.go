package hermes

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// Store is the durable storage boundary for Hermes drafts, reviews and
// deliveries. It keeps owner and guest records tenant-scoped and never
// hard-deletes.
type Store interface {
	InsertDraft(ctx context.Context, d *HermesDraft) error
	GetDraft(ctx context.Context, tenantID, draftID string) (*HermesDraft, error)
	UpdateDraftState(ctx context.Context, tenantID, draftID, state string) (*HermesDraft, error)
	InsertReview(ctx context.Context, r *HermesReview) error
	InsertDelivery(ctx context.Context, d *HermesDelivery) error
	GetDeliveryByDraft(ctx context.Context, tenantID, draftID string) (*HermesDelivery, error)
	GetDeliveryByKey(ctx context.Context, tenantID, idempotencyKey string) (*HermesDelivery, error)
	GetDelivery(ctx context.Context, tenantID, deliveryID string) (*HermesDelivery, error)
	ListDeliveries(ctx context.Context, tenantID string) ([]HermesDelivery, error)
}

// DraftParams bounds a Hermes draft to approved facts, one audience, a
// purpose, an optional curated template, language, channel and review policy.
// The model supplies none of these beyond the declared purpose and content;
// facts and template come from approved application records.
type DraftParams struct {
	RunID        string         `json:"run_id,omitempty"`
	TenantID     string         `json:"tenant_id,omitempty"`
	PropertyID   string         `json:"property_id,omitempty"`
	ActorID      string         `json:"actor_id,omitempty"`
	Audience     string         `json:"audience,omitempty"`
	Purpose      string         `json:"purpose,omitempty"`
	TemplateKey  string         `json:"template_key,omitempty"`
	Language     string         `json:"language,omitempty"`
	Channel      string         `json:"channel,omitempty"`
	ReviewPolicy string         `json:"review_policy,omitempty"`
	Facts        []ApprovedFact `json:"facts,omitempty"`
	Subject      string         `json:"subject,omitempty"`
	Body         string         `json:"body,omitempty"`
}

// ReviewParams records a distinct human review decision for a free-form draft.
type ReviewParams struct {
	ReviewerID string `json:"reviewer_id,omitempty"`
	Decision   string `json:"decision,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

// DeliveryParams is the application action input. Replaying the same draft or
// the same idempotency key returns the existing delivery.
type DeliveryParams struct {
	TenantID       string    `json:"tenant_id,omitempty"`
	DraftID        string    `json:"draft_id,omitempty"`
	RecipientID    string    `json:"recipient_id,omitempty"`
	ActorID        string    `json:"actor_id,omitempty"`
	IdempotencyKey string    `json:"idempotency_key,omitempty"`
	Now            time.Time `json:"-"`
}

func newID(prefix string) string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("hermes: crypto/rand failed: " + err.Error())
	}
	return prefix + hex.EncodeToString(b[:])
}

type HermesService struct {
	store  Store
	policy *HermesPolicyEngine
}

func NewService(store Store) *HermesService {
	return &HermesService{store: store, policy: NewPolicyEngine()}
}

func (s *HermesService) Policy() *HermesPolicyEngine {
	return s.policy
}

// Draft creates a bounded draft. Approved-template drafts start approved
// (pre-approved curated content) and free-form drafts start under review; an
// unreviewed free-form draft can never deliver.
func (s *HermesService) Draft(ctx context.Context, params DraftParams) (*HermesDraft, error) {
	if !IsValidAudience(params.Audience) {
		return nil, ErrInvalidAudience
	}
	if params.Purpose == "" {
		return nil, ErrPurposeRequired
	}
	if len(params.Facts) == 0 {
		return nil, ErrFactsRequired
	}
	for _, f := range params.Facts {
		if f.Source == "" || f.RecordID == "" || f.RecordKind == "" || f.EffectiveAt.IsZero() {
			return nil, ErrUnapprovedFact
		}
	}
	if err := FactsAcceptableForAudience(params.Audience, params.Facts); err != nil {
		return nil, err
	}

	policy := params.ReviewPolicy
	if policy == "" {
		if params.TemplateKey != "" {
			policy = ReviewPolicyApprovedTemplate
		} else {
			policy = ReviewPolicyHumanReview
		}
	}
	if !IsValidReviewPolicy(policy) {
		return nil, fmt.Errorf("hermes: review policy must be approved_template or human_review")
	}

	state := DraftStateApproved
	switch policy {
	case ReviewPolicyApprovedTemplate:
		if params.TemplateKey == "" {
			return nil, ErrTemplateKeyRequired
		}
	case ReviewPolicyHumanReview:
		if params.Subject == "" || params.Body == "" {
			return nil, ErrFreeFormContentRequired
		}
		state = DraftStateUnderReview
	}

	if params.Language == "" {
		params.Language = "en"
	}
	if params.Channel == "" {
		params.Channel = "push"
	}

	now := time.Now().UTC()
	draft := &HermesDraft{
		DraftID:      newID("herd"),
		RunID:        params.RunID,
		TenantID:     params.TenantID,
		PropertyID:   params.PropertyID,
		ActorID:      params.ActorID,
		Audience:     params.Audience,
		Purpose:      params.Purpose,
		TemplateKey:  params.TemplateKey,
		Language:     params.Language,
		Channel:      params.Channel,
		Facts:        params.Facts,
		ReviewPolicy: policy,
		State:        state,
		Subject:      params.Subject,
		Body:         params.Body,
		Version:      1,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.store.InsertDraft(ctx, draft); err != nil {
		return nil, err
	}
	return draft, nil
}

// Review records a distinct human review decision for a free-form draft. A
// requester can never review their own draft (maker-checker separation).
func (s *HermesService) Review(ctx context.Context, tenantID, draftID string, params ReviewParams) (*HermesDraft, error) {
	if params.ReviewerID == "" {
		return nil, ErrReviewerRequired
	}
	if params.Decision != ReviewDecisionApproved && params.Decision != ReviewDecisionRejected {
		return nil, ErrReviewDecisionRequired
	}

	draft, err := s.store.GetDraft(ctx, tenantID, draftID)
	if err != nil {
		return nil, err
	}
	if draft.ReviewPolicy != ReviewPolicyHumanReview {
		return nil, ErrReviewNotRequired
	}
	switch draft.State {
	case DraftStateUnderReview, DraftStateDraft:
	default:
		return nil, fmt.Errorf("hermes: draft in state %s is not reviewable", draft.State)
	}
	if params.ReviewerID == draft.ActorID {
		return nil, ErrReviewerIsRequester
	}

	review := &HermesReview{
		ReviewID:   newID("herr"),
		TenantID:   tenantID,
		DraftID:    draftID,
		ReviewerID: params.ReviewerID,
		Decision:   params.Decision,
		Reason:     params.Reason,
		ReviewedAt: time.Now().UTC(),
	}
	if err := s.store.InsertReview(ctx, review); err != nil {
		return nil, err
	}

	newState := DraftStateApproved
	if params.Decision == ReviewDecisionRejected {
		newState = DraftStateRejected
	}
	return s.store.UpdateDraftState(ctx, tenantID, draftID, newState)
}

// Deliver applies the reviewed, idempotent delivery action. Delivery fails
// closed for an unreviewed free-form draft and for a rejected draft. Replaying
// delivery for an already delivered draft or for the same idempotency key
// returns the existing delivery rather than creating a second one.
func (s *HermesService) Deliver(ctx context.Context, params DeliveryParams) (*HermesDelivery, error) {
	if params.Now.IsZero() {
		params.Now = time.Now().UTC()
	}

	if params.IdempotencyKey != "" {
		existing, err := s.store.GetDeliveryByKey(ctx, params.TenantID, params.IdempotencyKey)
		if err == nil {
			return existing, nil
		}
		if !errors.Is(err, ErrDeliveryNotFound) {
			return nil, err
		}
	}

	draft, err := s.store.GetDraft(ctx, params.TenantID, params.DraftID)
	if err != nil {
		return nil, err
	}

	existing, err := s.store.GetDeliveryByDraft(ctx, params.TenantID, params.DraftID)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, ErrDeliveryNotFound) {
		return nil, err
	}

	switch draft.State {
	case DraftStateRejected:
		return nil, ErrDraftNotApproved
	case DraftStateDelivered:
		// Replay of a delivered draft resolves to the existing delivery.
		return s.store.GetDeliveryByDraft(ctx, params.TenantID, params.DraftID)
	case DraftStateApproved:
	default:
		if draft.ReviewPolicy == ReviewPolicyHumanReview {
			return nil, ErrDraftRequiresReview
		}
		return nil, ErrDraftNotApproved
	}

	delivery := &HermesDelivery{
		DeliveryID:     newID("herd"),
		TenantID:       params.TenantID,
		DraftID:        params.DraftID,
		Audience:       draft.Audience,
		RecipientID:    params.RecipientID,
		IdempotencyKey: params.IdempotencyKey,
		Status:         DeliveryStateQueued,
		CreatedAt:      params.Now,
		UpdatedAt:      params.Now,
	}

	if err := s.store.InsertDelivery(ctx, delivery); err != nil {
		return nil, err
	}
	if _, err := s.store.UpdateDraftState(ctx, params.TenantID, params.DraftID, DraftStateDelivered); err != nil {
		return nil, err
	}
	return delivery, nil
}

// GetDelivery returns one delivery by ID.
func (s *HermesService) GetDelivery(ctx context.Context, tenantID, deliveryID string) (*HermesDelivery, error) {
	return s.store.GetDelivery(ctx, tenantID, deliveryID)
}

// ListDeliveries returns all deliveries for a tenant (delivery history is
// retained, never hard-deleted).
func (s *HermesService) ListDeliveries(ctx context.Context, tenantID string) ([]HermesDelivery, error) {
	return s.store.ListDeliveries(ctx, tenantID)
}
