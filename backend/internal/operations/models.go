package operations

import (
	"encoding/json"
	"errors"
	"time"
)

const (
	TypeTurnover                = "turnover"
	TypePreArrivalInspection    = "pre_arrival_inspection"
	TypeRestock                 = "restock"
	TypeIncident                = "incident"
	TypeRoutineMaintenance      = "routine_maintenance"
	TypeSpecialistVendorRequest = "specialist_vendor_request"
	TypePropertyOnboarding      = "property_onboarding"
	TypeDocumentReview          = "document_review"
	TypeInventoryCount          = "inventory_count"
)

var AllTicketTypes = []string{
	TypeTurnover,
	TypePreArrivalInspection,
	TypeRestock,
	TypeIncident,
	TypeRoutineMaintenance,
	TypeSpecialistVendorRequest,
	TypePropertyOnboarding,
	TypeDocumentReview,
	TypeInventoryCount,
}

const (
	StateDraft             = "draft"
	StateProposed          = "proposed"
	StateApproved          = "approved"
	StateScheduled         = "scheduled"
	StateAssigned          = "assigned"
	StateInProgress        = "in_progress"
	StateEvidenceSubmitted = "evidence_submitted"
	StateVerified          = "verified"
	StateClosed            = "closed"
	StateBlocked           = "blocked"
	StateCancelled         = "cancelled"
	StateRejected          = "rejected"
)

var AllStates = []string{
	StateDraft,
	StateProposed,
	StateApproved,
	StateScheduled,
	StateAssigned,
	StateInProgress,
	StateEvidenceSubmitted,
	StateVerified,
	StateClosed,
	StateBlocked,
	StateCancelled,
	StateRejected,
}

var TerminalStates = []string{StateClosed, StateCancelled}

const (
	BlockerTypeAccess     = "access"
	BlockerTypeSafety     = "safety"
	BlockerTypeParts      = "parts"
	BlockerTypeApproval   = "approval"
	BlockerTypeWeather    = "weather"
	BlockerTypeCompliance = "compliance"
	BlockerTypeExternal   = "external"
)

const (
	NotificationIntentUrgent = "urgent"
	NotificationIntentNormal = "normal"
	NotificationIntentNone   = "none"
)

var HighRiskTicketTypes = []string{
	TypeIncident,
	TypeSpecialistVendorRequest,
}

const (
	ChecklistStatusPending    = "pending"
	ChecklistStatusInProgress = "in_progress"
	ChecklistStatusCompleted  = "completed"
	ChecklistStatusNA         = "not_applicable"
)

const (
	SeverityCritical = "critical"
	SeverityHigh     = "high"
	SeverityMedium   = "medium"
	SeverityLow      = "low"
)

var AllSeverities = []string{SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow}

const (
	AlertTargetOnCall = "on_call"
	AlertTargetOwner  = "owner"
)

const (
	AlertStatusQueued    = "queued"
	AlertStatusProcessed = "processed"
)

const (
	EvidenceStatusAccepted = "accepted"
	EvidenceStatusRejected = "rejected"
)

const (
	RecoveryStatusOpen   = "open"
	RecoveryStatusClosed = "closed"
)

var (
	ErrTicketNotFound           = errors.New("ticket not found")
	ErrInvalidTicketType        = errors.New("invalid ticket type")
	ErrInvalidState             = errors.New("invalid state")
	ErrInvalidTransition        = errors.New("invalid transition")
	ErrTicketTerminal           = errors.New("ticket is in a terminal state and cannot be modified")
	ErrTicketNotBlocked         = errors.New("ticket is not blocked")
	ErrAlreadyBlocked           = errors.New("ticket is already blocked")
	ErrSelfVerification         = errors.New("high-risk verification requires a different actor; cannot self-verify")
	ErrNoHardDelete             = errors.New("tickets cannot be hard-deleted; use cancellation or closure")
	ErrChecklistNotFound        = errors.New("checklist not found")
	ErrChecklistVersionNotFound = errors.New("checklist version not found")
	ErrChecklistItemNotFound    = errors.New("checklist item not found")
	ErrBlockerRequired          = errors.New("blocking a ticket requires a blocker type and reason")
	ErrInvalidBlockerType       = errors.New("invalid blocker type")
	ErrReopenRequiresReason     = errors.New("reopening a closed ticket requires a follow-up reason")
	ErrCrossTenantDenied        = errors.New("cross-tenant access denied")
	ErrEvidenceRequired         = errors.New("required evidence is missing for completion")
	ErrEvidenceNotFound         = errors.New("evidence not found")
	ErrInvalidEvidenceHash      = errors.New("evidence content hash must be a valid sha256 hex digest")
	ErrNotIncident              = errors.New("ticket is not an incident ticket")
	ErrInvalidSeverity          = errors.New("invalid incident severity")
	ErrSeverityRequired         = errors.New("incident severity is required")
	ErrRecoveryNotFound         = errors.New("service recovery not found")
	ErrRecoveryInactive         = errors.New("service recovery is not open")
	ErrResponsibilityRequired   = errors.New("service recovery requires a recorded responsibility")
	ErrInvalidReworkCost        = errors.New("rework cost must be a non-negative integer amount in minor units")
	ErrCurrencyRequired         = errors.New("currency is required when a rework cost is recorded")
	ErrEvidenceRequirementLocks = errors.New("evidence requirement cannot be downgraded once bound to a checklist item")

	ErrSyncConflictNotFound    = errors.New("sync conflict not found")
	ErrSyncConflictNotOpen     = errors.New("sync conflict is already resolved")
	ErrSyncKeyConflict         = errors.New("sync idempotency key already used with a different payload")
	ErrOfflineEvidenceNotFound = errors.New("offline evidence metadata not found")
	ErrOfflineEvidenceQueued   = errors.New("offline evidence is queued and awaiting upload")
)

type TicketBlock struct {
	Type             string     `json:"type"`
	Reason           string     `json:"reason"`
	ResponsibleParty string     `json:"responsible_party,omitempty"`
	NextReviewAt     *time.Time `json:"next_review_at,omitempty"`
	EscalationPolicy string     `json:"escalation_policy,omitempty"`
	CreatedBy        string     `json:"created_by"`
	CreatedAt        time.Time  `json:"created_at"`
}

type Ticket struct {
	ID                 string          `json:"id"`
	TenantID           string          `json:"tenant_id"`
	PropertyID         string          `json:"property_id"`
	Type               string          `json:"type"`
	Status             string          `json:"status"`
	Reason             string          `json:"reason"`
	RequestedWindow    json.RawMessage `json:"requested_window,omitempty"`
	ChecklistVersionID string          `json:"checklist_version_id,omitempty"`
	CreatedBy          string          `json:"created_by"`
	AssignedTo         string          `json:"assigned_to,omitempty"`
	VerifiedBy         string          `json:"verified_by,omitempty"`
	VerifierNote       string          `json:"verifier_note,omitempty"`
	Blocker            *TicketBlock    `json:"blocker,omitempty"`
	FollowUpTicketID   string          `json:"follow_up_ticket_id,omitempty"`
	ReopenReason       string          `json:"reopen_reason,omitempty"`
	NotificationIntent string          `json:"notification_intent,omitempty"`
	Severity           string          `json:"severity,omitempty"`
	Version            int             `json:"version"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
}

type TicketStateEvent struct {
	ID        string    `json:"id"`
	TicketID  string    `json:"ticket_id"`
	TenantID  string    `json:"tenant_id"`
	FromState string    `json:"from_state"`
	ToState   string    `json:"to_state"`
	ActorID   string    `json:"actor_id"`
	Reason    string    `json:"reason"`
	Evidence  []string  `json:"evidence,omitempty"`
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
}

type ChecklistTemplate struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	TicketType string    `json:"ticket_type"`
	CreatedAt  time.Time `json:"created_at"`
}

type ChecklistTemplateVersion struct {
	ID         string    `json:"id"`
	TemplateID string    `json:"template_id"`
	Version    int       `json:"version"`
	Items      []string  `json:"items"`
	CreatedAt  time.Time `json:"created_at"`
}

type TicketChecklistItem struct {
	ID                string     `json:"id"`
	TicketID          string     `json:"ticket_id"`
	TenantID          string     `json:"tenant_id"`
	TemplateItemIndex int        `json:"template_item_index"`
	Label             string     `json:"label"`
	Status            string     `json:"status"`
	CompletedBy       string     `json:"completed_by,omitempty"`
	CompletedAt       *time.Time `json:"completed_at,omitempty"`
	EvidenceIDs       []string   `json:"evidence_ids,omitempty"`
	EvidenceRequired  bool       `json:"evidence_required,omitempty"`
	Notes             string     `json:"notes,omitempty"`
	Version           int        `json:"version"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type CreateTicketParams struct {
	TenantID           string
	PropertyID         string
	Type               string
	RequestedWindow    json.RawMessage
	ChecklistVersionID string
	Reason             string
}

type TransitionParams struct {
	ToState     string
	Reason      string
	EvidenceIDs []string
}

// EvidenceRecord is an immutable, tenant-scoped proof of work bound to a
// ticket and optionally to a checklist item. Its content hash identifies the
// underlying object and never changes after acceptance.
type EvidenceRecord struct {
	ID              string    `json:"id"`
	TenantID        string    `json:"tenant_id"`
	TicketID        string    `json:"ticket_id"`
	ChecklistItemID string    `json:"checklist_item_id,omitempty"`
	ObjectID        string    `json:"object_id,omitempty"`
	ContentHash     string    `json:"content_hash"`
	FileName        string    `json:"file_name,omitempty"`
	ContentType     string    `json:"content_type,omitempty"`
	SizeBytes       int64     `json:"size_bytes"`
	Status          string    `json:"status"`
	CapturedBy      string    `json:"captured_by"`
	CapturedAt      time.Time `json:"captured_at"`
	Version         int       `json:"version"`
	CreatedAt       time.Time `json:"created_at"`
}

type RegisterEvidenceParams struct {
	ChecklistItemID string
	ObjectID        string
	ContentHash     string
	FileName        string
	ContentType     string
	SizeBytes       int64
}

// IncidentAlert is a durable, tenant-scoped notification queue entry created
// by the incident response policy (for example on-call operations role and
// owner) and processed asynchronously.
type IncidentAlert struct {
	ID         string    `json:"id"`
	TenantID   string    `json:"tenant_id"`
	PropertyID string    `json:"property_id"`
	TicketID   string    `json:"ticket_id"`
	Severity   string    `json:"severity"`
	Target     string    `json:"target"`
	Policy     string    `json:"policy"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

// ServiceRecovery records how an incident failure was recovered while
// preserving the original failure (reason, severity and evidence hashes) and
// its rework cost, so downstream reporting can attribute avoidable rework.
type ServiceRecovery struct {
	ID                     string    `json:"id"`
	TenantID               string    `json:"tenant_id"`
	PropertyID             string    `json:"property_id"`
	IncidentTicketID       string    `json:"incident_ticket_id"`
	FollowUpTicketID       string    `json:"follow_up_ticket_id,omitempty"`
	Severity               string    `json:"severity"`
	OriginalReason         string    `json:"original_reason"`
	OriginalEvidenceHashes []string  `json:"original_evidence_hashes,omitempty"`
	Responsibility         string    `json:"responsibility"`
	ReworkCostMinor        int64     `json:"rework_cost_minor"`
	Currency               string    `json:"currency"`
	Status                 string    `json:"status"`
	CreatedBy              string    `json:"created_by"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

type RecoveryParams struct {
	Reason          string
	Responsibility  string
	ReworkCostMinor int64
	Currency        string
}

func IsValidTicketType(t string) bool {
	for _, tt := range AllTicketTypes {
		if tt == t {
			return true
		}
	}
	return false
}

func IsTerminalState(s string) bool {
	for _, ts := range TerminalStates {
		if ts == s {
			return true
		}
	}
	return false
}

// ChecklistSyncRecord is an idempotency guard for checklist sync operations.
// The same sync key with the same payload hash produces the same result without
// side effects; a different payload hash under the same key is rejected as a
// conflict, forcing the caller to generate a fresh sync key.
type ChecklistSyncRecord struct {
	ID          string    `json:"id"`
	SyncKey     string    `json:"sync_key"`
	TenantID    string    `json:"tenant_id"`
	TicketID    string    `json:"ticket_id"`
	PayloadHash string    `json:"payload_hash"`
	Result      string    `json:"result"`
	CreatedAt   time.Time `json:"created_at"`
}

// SyncConflict preserves both the server-side version and the incoming
// client-side version of a checklist item when a sync races with a concurrent
// update, making the divergence visible instead of silently overwriting either
// side.
type SyncConflict struct {
	ID                string     `json:"id"`
	TenantID          string     `json:"tenant_id"`
	TicketID          string     `json:"ticket_id"`
	ChecklistItemID   string     `json:"checklist_item_id"`
	TemplateItemIndex int        `json:"template_item_index"`
	ServerLabel       string     `json:"server_label"`
	ServerStatus      string     `json:"server_status"`
	ServerVersion     int        `json:"server_version"`
	ClientLabel       string     `json:"client_label"`
	ClientStatus      string     `json:"client_status"`
	ClientVersion     int        `json:"client_version"`
	Resolved          bool       `json:"resolved"`
	Resolution        string     `json:"resolution,omitempty"`
	ResolvedBy        string     `json:"resolved_by,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	ResolvedAt        *time.Time `json:"resolved_at,omitempty"`
}

// QueuedOfflineEvidence stores evidence metadata captured on a Curator device
// before the underlying binary has been uploaded. Once the upload completes,
// the queued metadata yields to the immutable evidence record.
type QueuedOfflineEvidence struct {
	ID              string    `json:"id"`
	TenantID        string    `json:"tenant_id"`
	TicketID        string    `json:"ticket_id"`
	ChecklistItemID string    `json:"checklist_item_id,omitempty"`
	ContentHash     string    `json:"content_hash"`
	FileName        string    `json:"file_name,omitempty"`
	ContentType     string    `json:"content_type,omitempty"`
	SizeBytes       int64     `json:"size_bytes"`
	Status          string    `json:"status"`
	CapturedBy      string    `json:"captured_by"`
	CapturedAt      time.Time `json:"captured_at"`
	CreatedAt       time.Time `json:"created_at"`
}

const (
	OfflineEvidenceQueued   = "queued"
	OfflineEvidenceUploaded = "uploaded"
	OfflineEvidenceFailed   = "failed"
)

func IsHighRiskTicketType(t string) bool {
	for _, ht := range HighRiskTicketTypes {
		if ht == t {
			return true
		}
	}
	return false
}
