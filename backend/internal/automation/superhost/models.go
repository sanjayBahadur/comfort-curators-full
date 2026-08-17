package superhost

import (
	"errors"
	"time"
)

var (
	ErrPropertyNotFound    = errors.New("superhost: property not found")
	ErrCrossPropertyDenied = errors.New("superhost: cross-property access denied")
	ErrTenantScopeRequired = errors.New("superhost: tenant scope is required")
)

type FactReference struct {
	Source      string    `json:"source"`
	EffectiveAt time.Time `json:"effective_at"`
	RecordID    string    `json:"record_id"`
	RecordKind  string    `json:"record_kind"`
}

type ContextProperty struct {
	ID               string           `json:"id"`
	State            string           `json:"state"`
	ServiceAddress   ContextAddress   `json:"service_address"`
	GeolocationZone  string           `json:"geolocation_zone"`
	Timezone         string           `json:"timezone"`
	MaximumOccupancy int              `json:"maximum_occupancy"`
	Readiness        ContextReadiness `json:"readiness"`
	ComplianceHolds  []ContextHold    `json:"compliance_holds"`
	Fact             FactReference    `json:"_fact"`
}

type ContextAddress struct {
	Line1      string `json:"line1"`
	Line2      string `json:"line2,omitempty"`
	City       string `json:"city"`
	State      string `json:"state"`
	PostalCode string `json:"postal_code"`
	Country    string `json:"country"`
}

type ContextReadiness struct {
	OwnerContractAccepted bool `json:"owner_contract_accepted"`
	ComplianceComplete    bool `json:"compliance_complete"`
	MandatoryFieldsSet    bool `json:"mandatory_fields_set"`
}

type ContextHold struct {
	Kind       string        `json:"kind"`
	Severity   string        `json:"severity"`
	Status     string        `json:"status"`
	Reason     string        `json:"reason"`
	ExpiresAt  *time.Time    `json:"expires_at,omitempty"`
	ResolvedAt *time.Time    `json:"resolved_at,omitempty"`
	Fact       FactReference `json:"_fact"`
}

type ContextReservation struct {
	ID       string        `json:"id"`
	Status   string        `json:"status"`
	StartAt  time.Time     `json:"start_at"`
	EndAt    time.Time     `json:"end_at"`
	Timezone string        `json:"timezone,omitempty"`
	AllDay   bool          `json:"all_day"`
	Fact     FactReference `json:"_fact"`
}

type ContextTicket struct {
	ID       string        `json:"id"`
	Type     string        `json:"type"`
	Status   string        `json:"status"`
	Reason   string        `json:"reason"`
	Severity string        `json:"severity,omitempty"`
	Fact     FactReference `json:"_fact"`
}

type ContextStockLocation struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	LocationType string        `json:"location_type"`
	Balance      int64         `json:"balance"`
	Fact         FactReference `json:"_fact"`
}

type ContextAgreement struct {
	ID      string        `json:"id"`
	Status  string        `json:"status"`
	Version int           `json:"version"`
	Fact    FactReference `json:"_fact"`
}

type ContextPreference struct {
	RecipientID           string        `json:"recipient_id"`
	Audience              string        `json:"audience"`
	ConsentTransactional  bool          `json:"consent_transactional"`
	ConsentUrgent         bool          `json:"consent_urgent"`
	Channel               string        `json:"channel"`
	QuietHoursStartMinute int           `json:"quiet_hours_start_minute"`
	QuietHoursEndMinute   int           `json:"quiet_hours_end_minute"`
	Fact                  FactReference `json:"_fact"`
}

type ContextSummary struct {
	Kind  string        `json:"kind"`
	Label string        `json:"label,omitempty"`
	Value float64       `json:"value"`
	Fact  FactReference `json:"_fact"`
}

type UISurfaceInput struct {
	ID      string   `json:"id"`
	Label   string   `json:"label"`
	Actions []string `json:"actions"`
}

type PropertyContext struct {
	TenantID   string `json:"tenant_id"`
	PropertyID string `json:"property_id"`
	// ActorRole is the requesting account's real role (owner/staff/guest),
	// resolved server-side from the authenticated session -- never client-
	// supplied. Nothing before this read the role at all: the same
	// operational data (stock balances, ticket queues, compliance holds)
	// was assembled and handed to Superhost regardless of who was asking,
	// which is exactly right for owner/staff but meant a guest's own
	// thread got the same staff-facing operational summary a guest has no
	// reason to see or act on. The prompt is what actually changes guest
	// behavior; this field is what lets it know it's talking to a guest
	// in the first place.
	ActorRole    string                 `json:"actor_role,omitempty"`
	AssembledAt  time.Time              `json:"assembled_at"`
	Property     ContextProperty        `json:"property"`
	Reservations []ContextReservation   `json:"reservations"`
	Tickets      []ContextTicket        `json:"tickets"`
	Stock        []ContextStockLocation `json:"stock"`
	Agreement    *ContextAgreement      `json:"agreement,omitempty"`
	Preferences  []ContextPreference    `json:"preferences,omitempty"`
	Summaries    []ContextSummary       `json:"summaries"`
	AccountTasks []ContextAccountTask   `json:"account_tasks,omitempty"`
}

// PortfolioContext is the multi-property counterpart to PropertyContext --
// see ContextAssembler.AssemblePortfolio. Used for a thread that isn't
// locked to one property, so Superhost can reason about and act across
// every property on the tenant in the same conversation.
type PortfolioContext struct {
	TenantID     string               `json:"tenant_id"`
	ActorRole    string               `json:"actor_role,omitempty"`
	AssembledAt  time.Time            `json:"assembled_at"`
	Properties   []PropertyContext    `json:"properties"`
	AccountTasks []ContextAccountTask `json:"account_tasks,omitempty"`
}

// ContextAccountTask is the context-assembly projection of an AccountTask
// (see account_tasks.go) -- Superhost's own notes for this specific
// (tenant, actor), not verified business state.
type ContextAccountTask struct {
	ID           string `json:"id"`
	Description  string `json:"description"`
	Status       string `json:"status"`
	ResolvedNote string `json:"resolved_note,omitempty"`
}
