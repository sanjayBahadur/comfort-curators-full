package communications

import (
	"errors"
	"regexp"
	"strings"
	"time"
)

// Audiences stay separate (COM-001, COM-007): every communication record is
// scoped to exactly one audience and owner consent never applies to guests.
const (
	AudienceOwner = "owner"
	AudienceGuest = "guest"
)

// Consent classes are tracked separately. Transactional and urgent consent
// default to granted; marketing and sponsored consent are opt-in (COM-001).
const (
	ConsentClassTransactional = "transactional"
	ConsentClassUrgent        = "urgent"
	ConsentClassMarketing     = "marketing"
	ConsentClassSponsored     = "sponsored"
)

// Template and message sources. Free-form AI messages always require review
// (COM-005); curated templates are pre-approved content.
const (
	SourceTemplate = "template"
	SourceAI       = "ai"
)

const (
	LanguageEnglish = "en"
	LanguageHindi   = "hi"
)

const (
	ChannelPush  = "push"
	ChannelSMS   = "sms"
	ChannelEmail = "email"
)

const (
	SeverityLow      = "low"
	SeverityNormal   = "normal"
	SeverityHigh     = "high"
	SeverityCritical = "critical"
)

const (
	DraftStatusDraft       = "draft"
	DraftStatusUnderReview = "under_review"
	DraftStatusApproved    = "approved"
	DraftStatusRejected    = "rejected"
	DraftStatusDelivered   = "delivered"
)

const (
	ReviewDecisionApproved = "approved"
	ReviewDecisionRejected = "rejected"
)

const (
	DeliveryStatusQueued    = "queued"
	DeliveryStatusSent      = "sent"
	DeliveryStatusDelivered = "delivered"
	DeliveryStatusFailed    = "failed"
)

const (
	LinkStatusActive  = "active"
	LinkStatusUsed    = "used"
	LinkStatusExpired = "expired"
	LinkStatusRevoked = "revoked"
)

const (
	TemplateStatusActive   = "active"
	TemplateStatusArchived = "archived"
)

var (
	ErrInvalidAudience         = errors.New("audience must be owner or guest")
	ErrInvalidConsentClass     = errors.New("consent class must be transactional, urgent, marketing or sponsored")
	ErrInvalidSource           = errors.New("source must be template or ai")
	ErrInvalidLanguage         = errors.New("template language must be en or hi")
	ErrInvalidChannel          = errors.New("channel must be push, sms or email")
	ErrInvalidSeverity         = errors.New("severity must be low, normal, high or critical")
	ErrInvalidReviewer         = errors.New("review requires a distinct human reviewer")
	ErrReviewDecisionRequired  = errors.New("review decision must be approved or rejected")
	ErrTemplateExists          = errors.New("template key already exists for this tenant")
	ErrTemplateNotFound        = errors.New("template not found")
	ErrTemplateVersionMissing  = errors.New("template has no version in the requested language")
	ErrTemplateContentRequired = errors.New("template version requires subject and body")
	ErrCrossTenantDenied       = errors.New("cross-tenant access denied")
	ErrDraftNotFound           = errors.New("communication draft not found")
	ErrDraftRequiresReview     = errors.New("free-form AI message requires human review before delivery")
	ErrDraftNotApproved        = errors.New("only approved drafts can be delivered")
	ErrDraftAlreadyDelivered   = errors.New("draft has already been delivered")
	ErrReviewNotRequired       = errors.New("curated template messages do not require review")
	ErrDraftAlreadyReviewed    = errors.New("draft has already been reviewed")
	ErrConsentNotGranted       = errors.New("recipient has not consented to this communication class")
	ErrQuietHours              = errors.New("delivery is blocked by quiet hours for a non-urgent message")
	ErrInvalidPreferences      = errors.New("preferences require a recipient and valid quiet hours")
	ErrPreferencesNotFound     = errors.New("communication preferences not found")
	ErrInvalidSecureLink       = errors.New("secure stay link requires a property, audience, recipient and future expiry")
	ErrLinkNotFound            = errors.New("secure stay link not found")
	ErrLinkExpired             = errors.New("secure stay link has expired")
	ErrLinkAlreadyUsed         = errors.New("secure stay link has already been redeemed")
	ErrLinkRevoked             = errors.New("secure stay link has been revoked")
	ErrAudienceMismatch        = errors.New("template audience does not match the message audience")
	ErrDeliveryNotFound        = errors.New("delivery not found")
	ErrInvalidDeliveryStatus   = errors.New("delivery status must be queued, sent, delivered or failed")
	ErrRecipientRequired       = errors.New("recipient is required")
	ErrTemplateKeyRequired     = errors.New("template key is required")
)

func IsValidAudience(a string) bool {
	return a == AudienceOwner || a == AudienceGuest
}

func IsValidConsentClass(c string) bool {
	switch c {
	case ConsentClassTransactional, ConsentClassUrgent, ConsentClassMarketing, ConsentClassSponsored:
		return true
	default:
		return false
	}
}

func IsValidSource(s string) bool {
	return s == SourceTemplate || s == SourceAI
}

func IsValidLanguage(l string) bool {
	return l == LanguageEnglish || l == LanguageHindi
}

func IsValidChannel(c string) bool {
	switch c {
	case ChannelPush, ChannelSMS, ChannelEmail:
		return true
	default:
		return false
	}
}

func IsValidSeverity(s string) bool {
	switch s {
	case SeverityLow, SeverityNormal, SeverityHigh, SeverityCritical:
		return true
	default:
		return false
	}
}

func IsValidDraftStatus(s string) bool {
	switch s {
	case DraftStatusDraft, DraftStatusUnderReview, DraftStatusApproved, DraftStatusRejected, DraftStatusDelivered:
		return true
	default:
		return false
	}
}

func IsValidDeliveryStatus(s string) bool {
	switch s {
	case DeliveryStatusQueued, DeliveryStatusSent, DeliveryStatusDelivered, DeliveryStatusFailed:
		return true
	default:
		return false
	}
}

// ConsentByClass maps a consent class to the matching preference flag.
func ConsentByClass(prefs *CommunicationPreferences, class string) bool {
	switch class {
	case ConsentClassUrgent:
		return prefs.ConsentUrgent
	case ConsentClassMarketing:
		return prefs.ConsentMarketing
	case ConsentClassSponsored:
		return prefs.ConsentSponsored
	default:
		return prefs.ConsentTransactional
	}
}

// IsWithinQuietHours reports whether now falls inside the configured quiet
// hours window. A zero-length window (start == end) disables quiet hours.
// Windows may wrap midnight (e.g. 22:00 to 06:00).
func IsWithinQuietHours(now time.Time, startMinute, endMinute int) bool {
	if startMinute == endMinute {
		return false
	}
	current := now.Hour()*60 + now.Minute()
	if startMinute < endMinute {
		return current >= startMinute && current < endMinute
	}
	return current >= startMinute || current < endMinute
}

// accessPlaceholderNames are template placeholders that resolve to sensitive
// property access details. They must never appear in insecure previews
// (COM-007).
var accessPlaceholderNames = []string{
	"access_code", "access_pin", "access_key", "access_secret",
	"door_code", "door_pin", "gate_code", "lockbox_code",
	"key_code", "key_pin", "wifi_password", "property_secret",
	"smart_lock_code", "entry_code",
}

var (
	accessPlaceholderRE = regexp.MustCompile(`\{([a-zA-Z_][a-zA-Z0-9_]*)\}`)
	accessSecretValueRE = regexp.MustCompile(`(?i)\b((?:smart\s*lock\s*code|access\s*code|door\s*code|gate\s*code|lockbox\s*code|entry\s*code|key\s*code|wifi\s*password|access\s*key|door\s*pin|passcode|pin|password))\b(\s*[:=]\s*|\s+is\s+|\s+)[A-Z0-9#*]{4,}`)
)

func isAccessPlaceholder(name string) bool {
	n := strings.ToLower(name)
	for _, p := range accessPlaceholderNames {
		if p == n {
			return true
		}
	}
	return false
}

// RedactAccessDetails removes sensitive access details from insecure preview
// text (COM-007). Placeholders that resolve to access secrets are hidden and
// literal code/pin values are masked.
func RedactAccessDetails(s string) string {
	out := accessPlaceholderRE.ReplaceAllStringFunc(s, func(m string) string {
		name := strings.TrimSuffix(strings.TrimPrefix(m, "{"), "}")
		if isAccessPlaceholder(name) {
			return "{access_details_hidden}"
		}
		return m
	})
	out = accessSecretValueRE.ReplaceAllString(out, "$1$2[hidden]")
	return out
}

// MessageTemplate is a curated, tenant-scoped message definition. It has a
// fixed audience and consent class; its content is versioned per language
// (COM-004).
type MessageTemplate struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id"`
	TemplateKey  string    `json:"template_key"`
	Audience     string    `json:"audience"`
	ConsentClass string    `json:"consent_class"`
	Channel      string    `json:"channel"`
	Severity     string    `json:"severity"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// TemplateVersion is one localized, versioned rendering of a template. A new
// version supersedes older versions of the same language.
type TemplateVersion struct {
	ID         string    `json:"id"`
	TenantID   string    `json:"tenant_id"`
	TemplateID string    `json:"template_id"`
	Version    int       `json:"version"`
	Language   string    `json:"language"`
	Subject    string    `json:"subject"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
}

// ResolvedTemplate is the content selected for a template key and language.
type ResolvedTemplate struct {
	Template     MessageTemplate
	Version      int
	Language     string
	Subject      string
	Body         string
	FallbackFrom string `json:"-"`
}

// CommunicationPreferences records separate consent for transactional,
// urgent, marketing and sponsored communication, plus channel, severity,
// quiet hours and escalation contacts (COM-001, COM-002). Preferences are
// keyed by (recipient, audience) so owner and guest audiences never share a
// consent record.
type CommunicationPreferences struct {
	ID                    string    `json:"id"`
	TenantID              string    `json:"tenant_id"`
	RecipientID           string    `json:"recipient_id"`
	Audience              string    `json:"audience"`
	ConsentTransactional  bool      `json:"consent_transactional"`
	ConsentUrgent         bool      `json:"consent_urgent"`
	ConsentMarketing      bool      `json:"consent_marketing"`
	ConsentSponsored      bool      `json:"consent_sponsored"`
	Channel               string    `json:"channel"`
	Severity              string    `json:"severity"`
	QuietHoursStartMinute int       `json:"quiet_hours_start_minute"`
	QuietHoursEndMinute   int       `json:"quiet_hours_end_minute"`
	EscalationContacts    []string  `json:"escalation_contacts,omitempty"`
	Version               int       `json:"version"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

// CommunicationDraft is a pending message. Template drafts are pre-approved;
// free-form AI drafts always require human review before delivery (COM-005).
type CommunicationDraft struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenant_id"`
	Audience       string    `json:"audience"`
	RecipientID    string    `json:"recipient_id"`
	Source         string    `json:"source"`
	TemplateKey    string    `json:"template_key,omitempty"`
	ConsentClass   string    `json:"consent_class"`
	Channel        string    `json:"channel"`
	Severity       string    `json:"severity"`
	Subject        string    `json:"subject"`
	Body           string    `json:"body"`
	Status         string    `json:"status"`
	RequiresReview bool      `json:"requires_review"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// CommunicationReview records a human review decision for a draft (COM-005).
type CommunicationReview struct {
	ID         string    `json:"id"`
	TenantID   string    `json:"tenant_id"`
	DraftID    string    `json:"draft_id"`
	ReviewerID string    `json:"reviewer_id"`
	Decision   string    `json:"decision"`
	Reason     string    `json:"reason,omitempty"`
	ReviewedAt time.Time `json:"reviewed_at"`
}

// Delivery is the durable delivery history record. Status transitions and
// failures are recorded and never hard-deleted (COM-006).
type Delivery struct {
	ID           string     `json:"id"`
	TenantID     string     `json:"tenant_id"`
	DraftID      string     `json:"draft_id,omitempty"`
	RecipientID  string     `json:"recipient_id"`
	Audience     string     `json:"audience"`
	ConsentClass string     `json:"consent_class"`
	Channel      string     `json:"channel"`
	Status       string     `json:"status"`
	Error        string     `json:"error,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	DeliveredAt  *time.Time `json:"delivered_at,omitempty"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// SecureLink is a short-lived, single-use stay link. The raw token is never
// stored; only its hash and a display tail are persisted. Redemption is
// atomic so a used link cannot be replayed (COM secure link behavior).
type SecureLink struct {
	ID          string     `json:"id"`
	TenantID    string     `json:"tenant_id"`
	PropertyID  string     `json:"property_id"`
	Audience    string     `json:"audience"`
	RecipientID string     `json:"recipient_id"`
	Purpose     string     `json:"purpose"`
	TokenTail   string     `json:"token_tail"`
	TokenHash   string     `json:"-"`
	ExpiresAt   time.Time  `json:"expires_at"`
	UsedAt      *time.Time `json:"used_at,omitempty"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
}

// TemplateParams is the input for creating a curated template.
type TemplateParams struct {
	TemplateKey  string
	Audience     string
	ConsentClass string
	Channel      string
	Severity     string
}

// TemplateVersionParams is the input for adding one localized version.
type TemplateVersionParams struct {
	Language string
	Subject  string
	Body     string
}

// PreferencesParams is the input for setting recipient preferences.
type PreferencesParams struct {
	RecipientID           string
	Audience              string
	ConsentTransactional  bool
	ConsentUrgent         bool
	ConsentMarketing      bool
	ConsentSponsored      bool
	Channel               string
	Severity              string
	QuietHoursStartMinute int
	QuietHoursEndMinute   int
	EscalationContacts    []string
}

// DraftParams is the input for creating a communication draft.
type DraftParams struct {
	Audience     string
	RecipientID  string
	Source       string
	TemplateKey  string
	ConsentClass string
	Channel      string
	Severity     string
	Language     string
	Subject      string
	Body         string
}

// ReviewParams is the input for a human review decision.
type ReviewParams struct {
	ReviewerID string
	Decision   string
	Reason     string
}

// SecureLinkParams is the input for issuing a secure stay link.
type SecureLinkParams struct {
	PropertyID  string
	Audience    string
	RecipientID string
	Purpose     string
	ExpiresAt   time.Time
}

// Preview is the redacted rendering shown to humans before any send. Access
// details are hidden (COM-007).
type Preview struct {
	TemplateKey string `json:"template_key"`
	Audience    string `json:"audience"`
	Language    string `json:"language"`
	Subject     string `json:"subject"`
	Body        string `json:"body"`
}
