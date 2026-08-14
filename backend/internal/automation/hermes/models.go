package hermes

import (
	"errors"
	"time"
)

// Hermes is a narrow communication and service-recovery specialist. Its
// drafts stay bounded by approved facts, one audience, a purpose, a template
// or free-form policy, a language and a channel. It never decides liability,
// creates conflicting operational work, or delivers unreviewed free-form
// messages (COM-005, CC-HER-001).

// Audiences stay separate. Every Hermes draft and delivery is scoped to
// exactly one audience and owner and guest context never mix.
const (
	AudienceOwner = "owner"
	AudienceGuest = "guest"
)

// ReviewPolicy selects how a draft is gated. Curated templates are pre-approved
// content; free-form drafts always require a distinct human review before
// delivery can be applied.
const (
	ReviewPolicyApprovedTemplate = "approved_template"
	ReviewPolicyHumanReview      = "human_review"
)

// Draft lifecycle.
const (
	DraftStateDraft       = "draft"
	DraftStateUnderReview = "under_review"
	DraftStateApproved    = "approved"
	DraftStateRejected    = "rejected"
	DraftStateDelivered   = "delivered"
)

// Review decisions.
const (
	ReviewDecisionApproved = "approved"
	ReviewDecisionRejected = "rejected"
)

// Delivery lifecycle (COM-006).
const (
	DeliveryStateQueued    = "queued"
	DeliveryStateSent      = "sent"
	DeliveryStateDelivered = "delivered"
	DeliveryStateFailed    = "failed"
)

// Fact audiences control which context may feed a draft. Owner facts never
// reach guest drafts and guest facts never reach owner drafts; public facts
// may feed either audience.
const (
	FactAudienceOwner  = "owner"
	FactAudienceGuest  = "guest"
	FactAudiencePublic = "public"
)

var (
	ErrInvalidAudience         = errors.New("hermes: audience must be owner or guest")
	ErrInvalidFactAudience     = errors.New("hermes: fact audience must be owner, guest or public")
	ErrAudienceMismatch        = errors.New("hermes: owner and guest context cannot mix in one draft or delivery")
	ErrPurposeRequired         = errors.New("hermes: purpose is required")
	ErrFactsRequired           = errors.New("hermes: draft requires at least one approved fact")
	ErrUnapprovedFact          = errors.New("hermes: every fact must be an approved record reference")
	ErrLiabilityDenied         = errors.New("hermes: Hermes cannot decide financial liability")
	ErrTemplateKeyRequired     = errors.New("hermes: approved template draft requires a template key")
	ErrFreeFormContentRequired = errors.New("hermes: free-form draft requires subject and body")
	ErrDraftNotFound           = errors.New("hermes: draft not found")
	ErrDraftNotApproved        = errors.New("hermes: only approved drafts can be delivered")
	ErrDraftRequiresReview     = errors.New("hermes: unreviewed free-form draft cannot deliver")
	ErrReviewNotRequired       = errors.New("hermes: approved template drafts do not require review")
	ErrReviewDecisionRequired  = errors.New("hermes: review decision must be approved or rejected")
	ErrReviewerRequired        = errors.New("hermes: review requires a distinct human reviewer")
	ErrReviewerIsRequester     = errors.New("hermes: maker-checker separation required, requester cannot review")
	ErrDeliveryNotFound        = errors.New("hermes: delivery not found")
	ErrInvalidDeliveryStatus   = errors.New("hermes: delivery status must be queued, sent, delivered or failed")
)

// ApprovedFact is an immutable reference to an approved business record that
// Hermes may cite. Facts are supplied by the application from approved sources;
// the model never invents them. The audience tag keeps owner and guest context
// separate.
type ApprovedFact struct {
	Source      string    `json:"source"`
	RecordID    string    `json:"record_id"`
	RecordKind  string    `json:"record_kind"`
	Audience    string    `json:"audience"`
	EffectiveAt time.Time `json:"effective_at"`
}

// HermesDraft is a bounded communication draft assembled from approved facts
// with a single audience, purpose, optional curated template, language and
// channel, and an explicit review policy.
type HermesDraft struct {
	DraftID      string         `json:"draft_id"`
	RunID        string         `json:"run_id"`
	TenantID     string         `json:"tenant_id"`
	PropertyID   string         `json:"property_id"`
	ActorID      string         `json:"actor_id"`
	Audience     string         `json:"audience"`
	Purpose      string         `json:"purpose"`
	TemplateKey  string         `json:"template_key,omitempty"`
	Language     string         `json:"language"`
	Channel      string         `json:"channel"`
	Facts        []ApprovedFact `json:"facts"`
	ReviewPolicy string         `json:"review_policy"`
	State        string         `json:"state"`
	Subject      string         `json:"subject,omitempty"`
	Body         string         `json:"body,omitempty"`
	Version      int            `json:"version"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

// HermesReview records a distinct human review decision for a free-form draft.
type HermesReview struct {
	ReviewID   string    `json:"review_id"`
	TenantID   string    `json:"tenant_id"`
	DraftID    string    `json:"draft_id"`
	ReviewerID string    `json:"reviewer_id"`
	Decision   string    `json:"decision"`
	Reason     string    `json:"reason,omitempty"`
	ReviewedAt time.Time `json:"reviewed_at"`
}

// HermesDelivery is a durable delivery-history record. Delivery is a reviewed
// idempotent application action: replaying a delivery for the same draft or the
// same idempotency key returns the existing delivery instead of creating a
// second one.
type HermesDelivery struct {
	DeliveryID     string     `json:"delivery_id"`
	TenantID       string     `json:"tenant_id"`
	DraftID        string     `json:"draft_id"`
	Audience       string     `json:"audience"`
	RecipientID    string     `json:"recipient_id"`
	IdempotencyKey string     `json:"idempotency_key,omitempty"`
	Status         string     `json:"status"`
	Error          string     `json:"error,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	DeliveredAt    *time.Time `json:"delivered_at,omitempty"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func IsValidAudience(a string) bool {
	return a == AudienceOwner || a == AudienceGuest
}

func IsValidFactAudience(a string) bool {
	switch a {
	case FactAudienceOwner, FactAudienceGuest, FactAudiencePublic:
		return true
	default:
		return false
	}
}

func IsValidReviewPolicy(p string) bool {
	return p == ReviewPolicyApprovedTemplate || p == ReviewPolicyHumanReview
}

func IsValidDraftState(s string) bool {
	switch s {
	case DraftStateDraft, DraftStateUnderReview, DraftStateApproved,
		DraftStateRejected, DraftStateDelivered:
		return true
	default:
		return false
	}
}

func IsValidDeliveryStatus(s string) bool {
	switch s {
	case DeliveryStateQueued, DeliveryStateSent, DeliveryStateDelivered, DeliveryStateFailed:
		return true
	default:
		return false
	}
}

// FactsAcceptableForAudience reports whether the supplied approved facts are
// consistent with a draft for the given audience. Owner and guest context are
// separated: a draft may never combine an owner fact and a guest fact, and
// owner facts never feed guest drafts (and vice versa). Public facts feed both.
func FactsAcceptableForAudience(audience string, facts []ApprovedFact) error {
	if !IsValidAudience(audience) {
		return ErrInvalidAudience
	}
	seenOwner, seenGuest := false, false
	for _, f := range facts {
		if !IsValidFactAudience(f.Audience) {
			return ErrInvalidFactAudience
		}
		switch f.Audience {
		case FactAudienceOwner:
			seenOwner = true
			if audience != AudienceOwner {
				return ErrAudienceMismatch
			}
		case FactAudienceGuest:
			seenGuest = true
			if audience != AudienceGuest {
				return ErrAudienceMismatch
			}
		}
	}
	if seenOwner && seenGuest {
		return ErrAudienceMismatch
	}
	return nil
}
