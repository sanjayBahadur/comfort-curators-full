package reservations

import (
	"errors"
	"time"
)

const (
	FeedStatusActive   = "active"
	FeedStatusPaused   = "paused"
	FeedStatusDisabled = "disabled"
)

const (
	EventStatusConfirmed      = "confirmed"
	EventStatusTentative      = "tentative"
	EventStatusCancelled      = "cancelled"
	EventStatusNoLongerListed = "no_longer_listed"
)

const (
	ExceptionKindFeedFailure          = "feed_failure"
	ExceptionKindStaleFeed            = "stale_feed"
	ExceptionKindDuplicate            = "duplicate"
	ExceptionKindOverlap              = "overlap"
	ExceptionKindImpossibleTurnaround = "impossible_turnaround"
	ExceptionKindTimezoneAmbiguity    = "timezone_ambiguity"
)

const (
	ExceptionSeverityWarning  = "warning"
	ExceptionSeverityCritical = "critical"
)

const (
	ExceptionStatusOpen     = "open"
	ExceptionStatusResolved = "resolved"
)

const (
	RoleJarvis    = "jarvis"
	RoleSuperhost = "superhost"
)

const (
	ReservationStatusActive    = "active"
	ReservationStatusCancelled = "cancelled"
)

const (
	ConflictStatusOpen     = "open"
	ConflictStatusResolved = "resolved"
)

const (
	ResolutionOutcomeConfirm = "confirm"
	ResolutionOutcomeReject  = "reject"
	ResolutionOutcomeMerge   = "merge"
)

const (
	ProposalKindTurnover   = "turnover"
	ProposalKindInspection = "inspection"
)

const (
	ProposalStatusProposed  = "proposed"
	ProposalStatusCancelled = "cancelled"
)

const (
	DefaultStaleAfterMinutes        = 24 * 60
	DefaultMinimumTurnaroundMinutes = 180
	DefaultFeedPollTimeout          = 15 * time.Second
	DefaultMaxFeedBytes             = 5 << 20
)

var (
	ErrFeedNotFound            = errors.New("calendar feed not found")
	ErrEventNotFound           = errors.New("external calendar event not found")
	ErrExceptionNotFound       = errors.New("calendar exception not found")
	ErrInvalidFeed             = errors.New("invalid calendar feed")
	ErrFeedNotActive           = errors.New("calendar feed is not active")
	ErrEmptyFeedContent        = errors.New("calendar feed content is empty")
	ErrSuperhostCannotMutate   = errors.New("superhost cannot mutate external calendars")
	ErrInvalidCalendarContent  = errors.New("invalid iCalendar content")
	ErrEventNotCancellable     = errors.New("external calendar event cannot be cancelled")
	ErrInvalidExceptionKind    = errors.New("invalid calendar exception kind")
	ErrReservationNotFound     = errors.New("reservation not found")
	ErrConflictNotFound        = errors.New("reservation conflict not found")
	ErrConflictAlreadyResolved = errors.New("reservation conflict already resolved")
	ErrInvalidResolution       = errors.New("invalid conflict resolution")
	ErrStaleFeedData           = errors.New("feed is stale; refusing to generate work from possibly stale data")
)

var activeEventStatuses = map[string]bool{
	EventStatusConfirmed: true,
	EventStatusTentative: true,
}

type CalendarFeed struct {
	ID                       string     `json:"id"`
	TenantID                 string     `json:"tenant_id"`
	PropertyID               string     `json:"property_id"`
	Source                   string     `json:"source"`
	URL                      string     `json:"url"`
	Status                   string     `json:"status"`
	PropertyTimezone         string     `json:"property_timezone"`
	StaleAfterMinutes        int        `json:"stale_after_minutes"`
	MinimumTurnaroundMinutes int        `json:"minimum_turnaround_minutes"`
	LastPolledAt             *time.Time `json:"last_polled_at,omitempty"`
	LastSuccessAt            *time.Time `json:"last_success_at,omitempty"`
	LastContentHash          string     `json:"last_content_hash,omitempty"`
	LastError                string     `json:"last_error,omitempty"`
	Version                  int        `json:"version"`
	CreatedAt                time.Time  `json:"created_at"`
	UpdatedAt                time.Time  `json:"updated_at"`
}

type ExternalCalendarEvent struct {
	ID                string    `json:"id"`
	TenantID          string    `json:"tenant_id"`
	PropertyID        string    `json:"property_id"`
	FeedID            string    `json:"feed_id"`
	ExternalEventID   string    `json:"external_event_id"`
	Source            string    `json:"source"`
	Summary           string    `json:"summary,omitempty"`
	Description       string    `json:"description,omitempty"`
	StartAt           time.Time `json:"start_at"`
	EndAt             time.Time `json:"end_at"`
	AllDay            bool      `json:"all_day"`
	Timezone          string    `json:"timezone,omitempty"`
	TimezoneAmbiguous bool      `json:"timezone_ambiguous"`
	Status            string    `json:"status"`
	Sequence          int       `json:"sequence"`
	RawICal           string    `json:"raw_ical,omitempty"`
	FirstSeenAt       time.Time `json:"first_seen_at"`
	LastSeenAt        time.Time `json:"last_seen_at"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func (e *ExternalCalendarEvent) IsActive() bool {
	return activeEventStatuses[e.Status]
}

type CalendarException struct {
	ID         string         `json:"id"`
	TenantID   string         `json:"tenant_id"`
	PropertyID string         `json:"property_id"`
	FeedID     string         `json:"feed_id,omitempty"`
	Kind       string         `json:"kind"`
	Severity   string         `json:"severity"`
	Status     string         `json:"status"`
	Message    string         `json:"message"`
	DedupeKey  string         `json:"dedupe_key,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	ResolvedAt *time.Time     `json:"resolved_at,omitempty"`
}

type FeedParams struct {
	TenantID                 string
	PropertyID               string
	Source                   string
	URL                      string
	Status                   string
	PropertyTimezone         string
	StaleAfterMinutes        int
	MinimumTurnaroundMinutes int
}

type IngestResult struct {
	FeedID            string `json:"feed_id"`
	Unchanged         bool   `json:"unchanged"`
	EventsParsed      int    `json:"events_parsed"`
	EventsSkipped     int    `json:"events_skipped"`
	EventsCreated     int    `json:"events_created"`
	EventsUpdated     int    `json:"events_updated"`
	EventsCancelled   int    `json:"events_cancelled"`
	ExceptionsCreated int    `json:"exceptions_created"`
	StaleFeedResolved bool   `json:"stale_feed_resolved"`

	ReservationsCreated   int `json:"reservations_created"`
	ReservationsUpdated   int `json:"reservations_updated"`
	ReservationsCancelled int `json:"reservations_cancelled"`
	ConflictsCreated      int `json:"conflicts_created"`
	ProposalsProposed     int `json:"proposals_proposed"`
	ProposalsUpdated      int `json:"proposals_updated"`
	ProposalsCancelled    int `json:"proposals_cancelled"`
}

type FeedHealth struct {
	Feed           CalendarFeed `json:"feed"`
	Fresh          bool         `json:"fresh"`
	Stale          bool         `json:"stale"`
	StaleSince     *time.Time   `json:"stale_since,omitempty"`
	LastSuccessAt  *time.Time   `json:"last_success_at,omitempty"`
	LastError      string       `json:"last_error,omitempty"`
	OpenExceptions int          `json:"open_exceptions"`
}

// Reservation is the normalized operational source of truth for a stay. It is
// derived from an external calendar event but lives independently of the feed:
// a changed stay updates the reservation in place and a cancelled stay is
// marked cancelled, never hard-deleted, so accepted work derived from it is
// never silently destroyed.
type Reservation struct {
	ID              string    `json:"id"`
	TenantID        string    `json:"tenant_id"`
	PropertyID      string    `json:"property_id"`
	FeedID          string    `json:"feed_id"`
	ExternalEventID string    `json:"external_event_id"`
	Source          string    `json:"source"`
	GuestSummary    string    `json:"guest_summary"`
	Status          string    `json:"status"`
	StartAt         time.Time `json:"start_at"`
	EndAt           time.Time `json:"end_at"`
	AllDay          bool      `json:"all_day"`
	Timezone        string    `json:"timezone,omitempty"`
	Sequence        int       `json:"sequence"`
	Version         int       `json:"version"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// ReservationConflict is a human-reviewable conflict derived from calendar
// detection (overlap, duplicate, impossible turnaround, timezone ambiguity).
// Conflicts are never auto-resolved; an owner or operator decides and the
// decision is audited.
type ReservationConflict struct {
	ID             string         `json:"id"`
	TenantID       string         `json:"tenant_id"`
	PropertyID     string         `json:"property_id"`
	Kind           string         `json:"kind"`
	Severity       string         `json:"severity"`
	Status         string         `json:"status"`
	Message        string         `json:"message"`
	ReservationIDs []string       `json:"reservation_ids"`
	ExceptionID    string         `json:"exception_id,omitempty"`
	DedupeKey      string         `json:"dedupe_key,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	ResolvedAt     *time.Time     `json:"resolved_at,omitempty"`
}

// ConflictResolution is the immutable, actor-attributed record of a human
// decision on a reservation conflict. It is written atomically with the
// conflict state change and mirrored to the append-only audit log.
type ConflictResolution struct {
	ID         string    `json:"id"`
	TenantID   string    `json:"tenant_id"`
	ConflictID string    `json:"conflict_id"`
	ActorID    string    `json:"actor_id"`
	ActorType  string    `json:"actor_type"`
	Outcome    string    `json:"outcome"`
	Note       string    `json:"note,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// TurnoverProposal is deterministic work derived from a normalized
// reservation: a turnover after checkout and an inspection before check-in.
// A cancelled reservation cancels its proposals in place rather than deleting
// them, and proposal generation never runs against a stale feed.
type TurnoverProposal struct {
	ID            string    `json:"id"`
	TenantID      string    `json:"tenant_id"`
	PropertyID    string    `json:"property_id"`
	ReservationID string    `json:"reservation_id"`
	Kind          string    `json:"kind"`
	Status        string    `json:"status"`
	ScheduledAt   time.Time `json:"scheduled_at"`
	ChecklistHint string    `json:"checklist_hint,omitempty"`
	Version       int       `json:"version"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type ProposalGenerationResult struct {
	Proposed  int    `json:"proposed"`
	Updated   int    `json:"updated"`
	Cancelled int    `json:"cancelled"`
	Skipped   bool   `json:"skipped"`
	Reason    string `json:"reason,omitempty"`
}
