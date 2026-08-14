package onboarding

import (
	"errors"
	"time"
)

var (
	ErrCaseNotFound        = errors.New("onboarding case not found")
	ErrCrossTenantDenied   = errors.New("cross-tenant access denied")
	ErrCaseActivated       = errors.New("onboarding case is already activated")
	ErrActivationBlocked   = errors.New("activation holds block onboarding activation")
	ErrIncomplete          = errors.New("onboarding checklist is incomplete")
	ErrInvalidSection      = errors.New("unknown onboarding section")
	ErrInvalidEvidence     = errors.New("invalid evidence record")
	ErrInvalidInspection   = errors.New("invalid inspection record")
	ErrInspectionImmutable = errors.New("inspection evidence is immutable")
	ErrInspectionNotFound  = errors.New("inspection record not found")
)

// Status is the state of an owner or property onboarding case.
type Status string

const (
	StatusInProgress Status = "in_progress"
	StatusReady      Status = "ready"
	StatusActivated  Status = "activated"
)

// Section and step keys. Every recordable onboarding input maps to one step
// in the checklist so an interrupted case reports exactly what remains.
const (
	StepPortfolio          = "portfolio"
	StepGoals              = "goals"
	StepServicePreferences = "service_preferences"
	StepBudgets            = "budgets"
	StepContacts           = "contacts"
	StepPhotographs        = "photographs"
	StepAmenities          = "amenities"
	StepSafety             = "safety"
	StepFurnishing         = "furnishing"
	StepRemediation        = "remediation"
	StepFitScoreInputs     = "fit_score_inputs"
	StepDocuments          = "documents"
	StepLegalEvidence      = "legal_evidence"
	StepSafetyEvidence     = "safety_evidence"
	StepInspections        = "inspections"
)

// AllSteps is the frozen onboarding checklist in a stable order.
var AllSteps = []string{
	StepPortfolio,
	StepGoals,
	StepServicePreferences,
	StepBudgets,
	StepContacts,
	StepPhotographs,
	StepAmenities,
	StepSafety,
	StepFurnishing,
	StepRemediation,
	StepFitScoreInputs,
	StepDocuments,
	StepLegalEvidence,
	StepSafetyEvidence,
	StepInspections,
}

// Evidence kinds. Legal and safety evidence gate activation.
const (
	EvidenceKindLegal    = "legal"
	EvidenceKindSafety   = "safety"
	EvidenceKindDocument = "document"
)

var ValidEvidenceKinds = []string{EvidenceKindLegal, EvidenceKindSafety, EvidenceKindDocument}

// Activation hold codes reported while mandatory evidence is missing.
const (
	HoldMissingLegalEvidence  = "missing_legal_evidence"
	HoldMissingSafetyEvidence = "missing_safety_evidence"
)

// ActivationHold explains one reason an onboarding case cannot be activated.
type ActivationHold struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// StepProgress reports whether one checklist step is complete. It is the
// resume surface: an interrupted case can be reconstructed from what remains.
type StepProgress struct {
	Key      string `json:"key"`
	Complete bool   `json:"complete"`
}

type Portfolio struct {
	PropertyName     string `json:"property_name"`
	PropertyType     string `json:"property_type"`
	PurchaseYear     int    `json:"purchase_year"`
	ManagedUnits     int    `json:"managed_units"`
	PrimaryResidence bool   `json:"primary_residence"`
}

type Goals struct {
	PrimaryGoal     string   `json:"primary_goal"`
	SecondaryGoals  []string `json:"secondary_goals,omitempty"`
	RentalStrategy  string   `json:"rental_strategy"`
	OccupancyTarget int      `json:"occupancy_target,omitempty"`
}

type ServicePreferences struct {
	FurnishingPreference string   `json:"furnishing_preference"`
	CommunicationChannel string   `json:"communication_channel"`
	ServiceLanguage      string   `json:"service_language"`
	GuestAccessPolicy    string   `json:"guest_access_policy"`
	ApprovalThreshold    int64    `json:"approval_threshold_minor_units"`
	Currency             string   `json:"currency"`
	AutomationLimits     []string `json:"automation_limits,omitempty"`
}

type Budgets struct {
	MonthlyBudgetMinorUnits    int64  `json:"monthly_budget_minor_units"`
	SetupBudgetMinorUnits      int64  `json:"setup_budget_minor_units"`
	RenovationBudgetMinorUnits int64  `json:"renovation_budget_minor_units"`
	Currency                   string `json:"currency"`
	OverspendPolicy            string `json:"overspend_policy"`
}

type Contact struct {
	Name  string `json:"name"`
	Role  string `json:"role,omitempty"`
	Phone string `json:"phone"`
	Email string `json:"email,omitempty"`
}

type Photograph struct {
	ObjectRef  string    `json:"object_ref"`
	Caption    string    `json:"caption,omitempty"`
	CapturedAt time.Time `json:"captured_at"`
}

type Amenity struct {
	Name     string `json:"name"`
	Quantity int    `json:"quantity"`
}

type Safety struct {
	SmokeDetectorsInstalled   bool   `json:"smoke_detectors_installed"`
	FireExtinguisherPresent   bool   `json:"fire_extinguisher_present"`
	GasLeakCheckDone          bool   `json:"gas_leak_check_done"`
	ElectricalSafetyCertified bool   `json:"electrical_safety_certified"`
	Notes                     string `json:"notes,omitempty"`
}

type Furnishing struct {
	FurnishingLevel string `json:"furnishing_level"`
	InventoryCount  int    `json:"inventory_count"`
	Notes           string `json:"notes,omitempty"`
}

type RemediationItem struct {
	Description string `json:"description"`
	Resolved    bool   `json:"resolved"`
}

type Remediation struct {
	OpenItems      []RemediationItem `json:"open_items,omitempty"`
	CompletedItems []RemediationItem `json:"completed_items,omitempty"`
}

type FitScoreInputs struct {
	PropertyScore   int `json:"property_score"`
	MarketScore     int `json:"market_score"`
	OperationsScore int `json:"operations_score"`
	RenovationScore int `json:"renovation_score"`
	OccupancyScore  int `json:"occupancy_score"`
}

// Evidence is an append-only record of captured legal, safety or document
// material. Evidence is never updated or deleted in place; a corrected or
// superseding capture is a new record.
type Evidence struct {
	ID          string    `json:"id"`
	CaseID      string    `json:"case_id"`
	TenantID    string    `json:"tenant_id"`
	Kind        string    `json:"kind"`
	ContentHash string    `json:"content_hash"`
	ObjectRef   string    `json:"object_ref"`
	CapturedBy  string    `json:"captured_by"`
	CapturedAt  time.Time `json:"captured_at"`
}

// Inspection records the performed property inspection and the captured
// evidence that supports it. Inspection evidence is immutable: the record and
// its content hash cannot be updated or deleted once persisted.
type Inspection struct {
	ID            string    `json:"id"`
	CaseID        string    `json:"case_id"`
	TenantID      string    `json:"tenant_id"`
	PropertyID    string    `json:"property_id"`
	PerformedAt   time.Time `json:"performed_at"`
	InspectedBy   string    `json:"inspected_by"`
	EvidenceHash  string    `json:"evidence_hash"`
	EvidenceRef   string    `json:"evidence_ref"`
	Findings      string    `json:"findings"`
	OverallStatus string    `json:"overall_status"`
	CreatedAt     time.Time `json:"created_at"`
}

// Case is the onboarding aggregate for one owner and property. It records
// authority, portfolio, goals, service preferences, budgets, contacts,
// documents, inspections, photographs, amenities, safety, furnishing,
// remediation and fit score inputs. Every section persists independently so an
// interrupted case resumes from its last committed state instead of
// restarting.
type Case struct {
	ID                 string              `json:"id"`
	TenantID           string              `json:"tenant_id"`
	PropertyID         string              `json:"property_id"`
	OwnerAuthorityID   string              `json:"owner_authority_id"`
	Status             Status              `json:"status"`
	Portfolio          *Portfolio          `json:"portfolio,omitempty"`
	Goals              *Goals              `json:"goals,omitempty"`
	ServicePreferences *ServicePreferences `json:"service_preferences,omitempty"`
	Budgets            *Budgets            `json:"budgets,omitempty"`
	Contacts           []Contact           `json:"contacts,omitempty"`
	Photographs        []Photograph        `json:"photographs,omitempty"`
	Amenities          []Amenity           `json:"amenities,omitempty"`
	Safety             *Safety             `json:"safety,omitempty"`
	Furnishing         *Furnishing         `json:"furnishing,omitempty"`
	Remediation        *Remediation        `json:"remediation,omitempty"`
	FitScoreInputs     *FitScoreInputs     `json:"fit_score_inputs,omitempty"`
	Evidence           []Evidence          `json:"evidence,omitempty"`
	Inspections        []Inspection        `json:"inspections,omitempty"`
	Holds              []ActivationHold    `json:"activation_holds,omitempty"`
	Version            int                 `json:"version"`
	CreatedAt          time.Time           `json:"created_at"`
	UpdatedAt          time.Time           `json:"updated_at"`
}

// StartCaseParams carries the identity inputs required to open a case.
type StartCaseParams struct {
	TenantID         string
	PropertyID       string
	OwnerAuthorityID string
}

// EvidenceParams describes one captured evidence record.
type EvidenceParams struct {
	Kind        string
	ContentHash string
	ObjectRef   string
	CapturedBy  string
	CapturedAt  time.Time
}

// InspectionParams describes one performed property inspection.
type InspectionParams struct {
	PropertyID    string
	PerformedAt   time.Time
	InspectedBy   string
	EvidenceHash  string
	EvidenceRef   string
	Findings      string
	OverallStatus string
}
