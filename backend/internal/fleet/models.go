package fleet

import (
	"errors"
	"time"
)

var (
	ErrAssetNotFound              = errors.New("fleet asset not found")
	ErrBatteryNotFound            = errors.New("fleet battery not found")
	ErrInvalidAsset               = errors.New("invalid fleet asset")
	ErrPowerLimitExceeded         = errors.New("rated motor power must not exceed 250 W")
	ErrDesignSpeedLimitExceeded   = errors.New("maximum design speed must not exceed 25 km/h")
	ErrComplianceEvidenceRequired = errors.New("compliance and design-speed evidence is required")
	ErrInvalidSafetyKind          = errors.New("invalid safety item kind")
	ErrSafetyItemNotFound         = errors.New("fleet safety item not found")
	ErrSafetyItemAlreadyCompleted = errors.New("fleet safety item is already completed")
	ErrSafetyItemDueRequired      = errors.New("safety item requires a due date")
	ErrInvalidInspection          = errors.New("invalid fleet inspection")
	ErrInspectionResultInvalid    = errors.New("inspection result must be pass or fail")
	ErrCustodyEventInvalid        = errors.New("invalid fleet custody event")
	ErrCustodyNotFound            = errors.New("fleet custody event not found")
	ErrNoActiveCustody            = errors.New("asset has no active custody")
	ErrCustodyMismatch            = errors.New("worker is not the current custodian")
	ErrIncidentNotFound           = errors.New("fleet incident not found")
	ErrIncidentAlreadyResolved    = errors.New("fleet incident is already resolved")
	ErrIncidentRequiresResolution = errors.New("incident resolution is required")
	ErrIncidentSeverityInvalid    = errors.New("invalid incident severity")
	ErrOffDutyTrackingDisabled    = errors.New("off-duty location tracking is disabled and never collected")
	ErrTrackingAssetMismatch      = errors.New("worker may only be tracked on the asset currently in their custody")
	ErrCrossTenantDenied          = errors.New("cross-tenant fleet access denied")
	ErrUnauthorized               = errors.New("unauthorized fleet action")
)

const (
	AssetStatusAvailable = "available"
	AssetStatusAssigned  = "assigned"
	AssetStatusFrozen    = "frozen"
	AssetStatusRetired   = "retired"
)

// RatedMotorPowerWattsLimit is the maximum continuous rated motor power for a
// compliant e-bike (250 W) and MaximumDesignSpeedKmhLimit is the maximum
// design speed (25 km/h). Both are enforced with evidence at creation time.
const (
	RatedMotorPowerWattsLimit  = 250
	MaximumDesignSpeedKmhLimit = 25
)

const (
	SafetyKindInspection = "safety_inspection"
	SafetyKindService    = "service"
	SafetyKindBattery    = "battery"
	SafetyKindBrake      = "brake"
	SafetyKindTire       = "tire"
	SafetyKindLight      = "light"
	SafetyKindCompliance = "compliance"
)

var validSafetyKinds = map[string]bool{
	SafetyKindInspection: true,
	SafetyKindService:    true,
	SafetyKindBattery:    true,
	SafetyKindBrake:      true,
	SafetyKindTire:       true,
	SafetyKindLight:      true,
	SafetyKindCompliance: true,
}

func ValidSafetyKind(k string) bool {
	return validSafetyKinds[k]
}

const (
	ItemStatusOpen      = "open"
	ItemStatusCompleted = "completed"
)

const (
	CustodyEventTypeHandover = "handover"
	CustodyEventTypeReturn   = "return"
)

const (
	InspectionTypePreUse = "pre_use"
	InspectionResultPass = "pass"
	InspectionResultFail = "fail"
)

const (
	IncidentStatusOpen     = "open"
	IncidentStatusResolved = "resolved"
)

const (
	IncidentSeverityLow      = "low"
	IncidentSeverityModerate = "moderate"
	IncidentSeverityHigh     = "high"
)

var validIncidentSeverities = map[string]bool{
	IncidentSeverityLow:      true,
	IncidentSeverityModerate: true,
	IncidentSeverityHigh:     true,
}

func ValidIncidentSeverity(s string) bool {
	return validIncidentSeverities[s]
}

type FleetAsset struct {
	ID                     string     `json:"id"`
	TenantID               string     `json:"tenant_id"`
	Model                  string     `json:"model"`
	SerialNumber           string     `json:"serial_number"`
	RatedMotorPowerWatts   int        `json:"rated_motor_power_watts"`
	MaximumDesignSpeedKmh  int        `json:"maximum_design_speed_kmh"`
	DesignSpeedEvidenceRef string     `json:"design_speed_evidence_ref"`
	ComplianceDocumentRef  string     `json:"compliance_document_ref"`
	BatterySerial          string     `json:"battery_serial"`
	Charger                string     `json:"charger"`
	PurchaseDate           time.Time  `json:"purchase_date"`
	WarrantyExpiresAt      *time.Time `json:"warranty_expires_at,omitempty"`
	WarrantyTerms          string     `json:"warranty_terms"`
	AssignedCustodianID    string     `json:"assigned_custodian_id,omitempty"`
	Status                 string     `json:"status"`
	Version                int        `json:"version"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

func (a *FleetAsset) IsCompliant() bool {
	return a.RatedMotorPowerWatts > 0 &&
		a.RatedMotorPowerWatts <= RatedMotorPowerWattsLimit &&
		a.MaximumDesignSpeedKmh > 0 &&
		a.MaximumDesignSpeedKmh <= MaximumDesignSpeedKmhLimit
}

func (a *FleetAsset) IsFrozen() bool {
	return a.Status == AssetStatusFrozen
}

type FleetBattery struct {
	ID               string     `json:"id"`
	TenantID         string     `json:"tenant_id"`
	AssetID          string     `json:"asset_id"`
	BatterySerial    string     `json:"battery_serial"`
	HealthStatus     string     `json:"health_status"`
	CycleCount       int        `json:"cycle_count"`
	LastServiceAt    *time.Time `json:"last_service_at,omitempty"`
	NextServiceDueAt *time.Time `json:"next_service_due_at,omitempty"`
	Status           string     `json:"status"`
	Version          int        `json:"version"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// FleetMaintenanceRecord is a service or safety/compliance item attached to an
// asset. A safety item whose status is open and whose due date has passed is
// overdue and blocks dispatch (VEH-002).
type FleetMaintenanceRecord struct {
	ID              string     `json:"id"`
	TenantID        string     `json:"tenant_id"`
	AssetID         string     `json:"asset_id"`
	Kind            string     `json:"kind"`
	Title           string     `json:"title"`
	Description     string     `json:"description"`
	DueAt           *time.Time `json:"due_at,omitempty"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	Status          string     `json:"status"`
	ServiceProvider string     `json:"service_provider"`
	PerformedBy     string     `json:"performed_by"`
	Notes           string     `json:"notes"`
	Version         int        `json:"version"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func IsSafetyItemOverdue(now time.Time, item *FleetMaintenanceRecord) bool {
	if item == nil || item.Status != ItemStatusOpen {
		return false
	}
	if item.DueAt == nil || item.DueAt.IsZero() {
		return false
	}
	return item.DueAt.Before(now)
}

type FleetInspection struct {
	ID                string    `json:"id"`
	TenantID          string    `json:"tenant_id"`
	AssetID           string    `json:"asset_id"`
	WorkerID          string    `json:"worker_id"`
	InspectionType    string    `json:"inspection_type"`
	Result            string    `json:"result"`
	DamageReported    bool      `json:"damage_reported"`
	DamageDescription string    `json:"damage_description"`
	CreatedAt         time.Time `json:"created_at"`
}

func (i *FleetInspection) IsPassingPreUse() bool {
	return i.InspectionType == InspectionTypePreUse &&
		i.Result == InspectionResultPass &&
		!i.DamageReported
}

type FleetCustodyEvent struct {
	ID             string     `json:"id"`
	TenantID       string     `json:"tenant_id"`
	AssetID        string     `json:"asset_id"`
	EventType      string     `json:"event_type"`
	FromWorkerID   string     `json:"from_worker_id"`
	ToWorkerID     string     `json:"to_worker_id"`
	Condition      string     `json:"condition"`
	Accessories    string     `json:"accessories"`
	AcknowledgedBy string     `json:"acknowledged_by"`
	AcknowledgedAt *time.Time `json:"acknowledged_at,omitempty"`
	Notes          string     `json:"notes"`
	CreatedAt      time.Time  `json:"created_at"`
}

type FleetIncident struct {
	ID             string     `json:"id"`
	TenantID       string     `json:"tenant_id"`
	AssetID        string     `json:"asset_id"`
	Kind           string     `json:"kind"`
	Severity       string     `json:"severity"`
	Description    string     `json:"description"`
	ReportedBy     string     `json:"reported_by"`
	SafetyTicketID string     `json:"safety_ticket_id"`
	Status         string     `json:"status"`
	ReviewedBy     string     `json:"reviewed_by"`
	ReviewedAt     *time.Time `json:"reviewed_at,omitempty"`
	Resolution     string     `json:"resolution"`
	Version        int        `json:"version"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type FleetTrackingEvent struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenant_id"`
	AssetID        string    `json:"asset_id"`
	WorkerID       string    `json:"worker_id"`
	CustodyEventID string    `json:"custody_event_id"`
	Latitude       float64   `json:"latitude"`
	Longitude      float64   `json:"longitude"`
	CapturedAt     time.Time `json:"captured_at"`
	CreatedAt      time.Time `json:"created_at"`
}

// DispatchBlockReason describes one hard constraint that prevents an asset
// from being dispatched.
type DispatchBlockReason struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

const (
	DispatchBlockFrozen           = "ASSET_FROZEN"
	DispatchBlockIncidentPending  = "INCIDENT_PENDING_REVIEW"
	DispatchBlockSafetyOverdue    = "SAFETY_ITEM_OVERDUE"
	DispatchBlockInspectionFailed = "PRE_USE_INSPECTION_NOT_PASSED"
)

type DispatchBlock struct {
	Allowed bool                  `json:"allowed"`
	Reasons []DispatchBlockReason `json:"reasons"`
}

func (b *DispatchBlock) AddReason(code, message string) {
	b.Allowed = false
	b.Reasons = append(b.Reasons, DispatchBlockReason{Code: code, Message: message})
}

// TrackingStatus reports whether location collection is currently permitted
// for a worker. It is only enabled while the worker holds an active asset
// custody (an accepted active route or task) and is automatically disabled as
// soon as the asset is returned (VEH-009, WFM-009).
type TrackingStatus struct {
	Tracking bool      `json:"tracking"`
	AssetID  string    `json:"asset_id,omitempty"`
	Since    time.Time `json:"since,omitempty"`
}

type CreateAssetParams struct {
	Model                  string
	SerialNumber           string
	RatedMotorPowerWatts   int
	MaximumDesignSpeedKmh  int
	DesignSpeedEvidenceRef string
	ComplianceDocumentRef  string
	BatterySerial          string
	Charger                string
	PurchaseDate           time.Time
	WarrantyExpiresAt      *time.Time
	WarrantyTerms          string
}

type SafetyItemParams struct {
	Kind        string
	Title       string
	Description string
	DueAt       time.Time
}

type CompleteSafetyItemParams struct {
	CompletedAt time.Time
	PerformedBy string
	Notes       string
}

type InspectionParams struct {
	WorkerID          string
	InspectionType    string
	Result            string
	DamageReported    bool
	DamageDescription string
}

type CustodyParams struct {
	FromWorkerID   string
	ToWorkerID     string
	Condition      string
	Accessories    string
	AcknowledgedBy string
	Notes          string
}

type IncidentParams struct {
	Kind           string
	Severity       string
	Description    string
	SafetyTicketID string
}

type ReviewIncidentParams struct {
	Resolution string
}

type LocationParams struct {
	AssetID    string
	Latitude   float64
	Longitude  float64
	CapturedAt time.Time
}
