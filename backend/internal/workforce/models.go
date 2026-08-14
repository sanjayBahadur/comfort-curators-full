package workforce

import (
	"errors"
	"time"
)

const (
	ClassificationEmployee = "employee"
	ClassificationVendor   = "vendor"
)

const (
	StatusActive     = "active"
	StatusInactive   = "inactive"
	StatusSuspended  = "suspended"
	StatusRejected   = "rejected"
	StatusTerminated = "terminated"
)

// Restricted work types (WFM-014): chemical, ladder, electrical, gas, pest,
// heavy-load, and other restricted work require explicit certification or
// routing to a specialist vendor.
const (
	WorkChemical   = "chemical"
	WorkLadder     = "ladder"
	WorkElectrical = "electrical"
	WorkGas        = "gas"
	WorkPest       = "pest"
	WorkHeavyLoad  = "heavy_load"
)

var RestrictedWorkTypes = []string{
	WorkChemical,
	WorkLadder,
	WorkElectrical,
	WorkGas,
	WorkPest,
	WorkHeavyLoad,
}

const (
	CertStatusValid   = "valid"
	CertStatusExpired = "expired"
)

const (
	RatingSourceHuman = "human"
	RatingSourceAI    = "ai"
)

const (
	AdverseActionReject    = "reject"
	AdverseActionSuspend   = "suspend"
	AdverseActionTerminate = "terminate"
)

const (
	GrievanceStatusPending   = "pending"
	GrievanceStatusReviewed  = "reviewed"
	GrievanceStatusResolved  = "resolved"
	GrievanceStatusDismissed = "dismissed"
)

const (
	SOSStatusTriggered    = "triggered"
	SOSStatusAcknowledged = "acknowledged"
	SOSStatusResolved     = "resolved"
)

var (
	ErrWorkerNotFound                = errors.New("worker not found")
	ErrInvalidClassification         = errors.New("worker classification must be employee or vendor")
	ErrMissingLegalName              = errors.New("legal name is required")
	ErrInvalidDateOfBirth            = errors.New("date of birth must be in the past")
	ErrUnderageForOperations         = errors.New("under-18 workers cannot be assigned to operations")
	ErrWorkerNotAssignable           = errors.New("worker is not in an assignable state")
	ErrInvalidWorkType               = errors.New("invalid work type")
	ErrRestrictedWorkRequiresCert    = errors.New("restricted work requires explicit certification or a specialist vendor")
	ErrCertificationExpired          = errors.New("certification has expired")
	ErrInvalidCertification          = errors.New("certification requires a work type, issuer and a future expiry")
	ErrRatingCannotDeactivate        = errors.New("a rating or AI score cannot reject, suspend, or terminate a worker")
	ErrInvalidRatingScore            = errors.New("rating score must be between 0 and 100")
	ErrInvalidRatingSource           = errors.New("rating source must be human or ai")
	ErrInvalidAdverseAction          = errors.New("invalid adverse action")
	ErrAdverseActionRequiresReviewer = errors.New("adverse action requires a distinct human reviewer")
	ErrAdverseActionRequiresEvidence = errors.New("adverse action must show the evidence considered")
	ErrAdverseActionRequiresReason   = errors.New("adverse action requires a reason")
	ErrAdverseActionSelfReview       = errors.New("the worker being reviewed cannot review their own adverse action")
	ErrCrossTenantDenied             = errors.New("cross-tenant access denied")
	ErrConcurrentModification        = errors.New("worker state update lost a concurrent write (optimistic version)")
	ErrInvalidAvailabilityWindow     = errors.New("availability window requires valid day of week and start before end")
	ErrInvalidTimeEntry              = errors.New("time entry requires non-negative minutes and a valid worker")
	ErrInvalidExpense                = errors.New("expense requires positive minor units and a valid currency")
	ErrInvalidGrievance              = errors.New("grievance requires kind and reason")
	ErrInvalidSOSEvent               = errors.New("SOS event requires a valid worker")
	ErrInvalidEmploymentTerm         = errors.New("employment term requires role and effective date")
	ErrGrievanceNotFound             = errors.New("grievance not found")
	ErrSOSNotFound                   = errors.New("SOS event not found")
	ErrExpenseNotFound               = errors.New("expense not found")
)

func IsRestrictedWorkType(t string) bool {
	for _, w := range RestrictedWorkTypes {
		if w == t {
			return true
		}
	}
	return false
}

// IsAgeEligible reports whether a person born on dob is at least 18 years old
// at the given instant.
func IsAgeEligible(dob, now time.Time) bool {
	if dob.After(now) {
		return false
	}
	years := now.Year() - dob.Year()
	if now.Month() < dob.Month() || (now.Month() == dob.Month() && now.Day() < dob.Day()) {
		years--
	}
	return years >= 18
}

func IsValidClassification(c string) bool {
	return c == ClassificationEmployee || c == ClassificationVendor
}

func IsValidWorkerStatus(s string) bool {
	switch s {
	case StatusActive, StatusInactive, StatusSuspended, StatusRejected, StatusTerminated:
		return true
	default:
		return false
	}
}

func IsDeactivatingStatus(s string) bool {
	switch s {
	case StatusInactive, StatusSuspended, StatusRejected, StatusTerminated:
		return true
	default:
		return false
	}
}

func CertificationStatus(expiresAt time.Time) string {
	if expiresAt.Before(time.Now().UTC()) {
		return CertStatusExpired
	}
	return CertStatusValid
}

func IsValidAdverseAction(a string) bool {
	switch a {
	case AdverseActionReject, AdverseActionSuspend, AdverseActionTerminate:
		return true
	default:
		return false
	}
}

// Worker is a tenant-scoped workforce record. Employees and genuine vendors
// stay distinct: the classification is mandatory and the record never converts
// one into the other.
type Worker struct {
	ID               string    `json:"id"`
	TenantID         string    `json:"tenant_id"`
	LegalName        string    `json:"legal_name"`
	VerifiedIdentity bool      `json:"verified_identity"`
	DateOfBirth      time.Time `json:"date_of_birth"`
	AgeEligible      bool      `json:"age_eligible"`
	ContactMethod    string    `json:"contact_method"`
	Classification   string    `json:"classification"`
	Specialist       bool      `json:"specialist,omitempty"`
	ServiceZone      string    `json:"service_zone"`
	Skills           []string  `json:"skills,omitempty"`
	Status           string    `json:"status"`
	Version          int       `json:"version"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// Certification is an explicit credential for a work type. Only a valid (not
// expired) certification satisfies a restricted-work requirement.
type Certification struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	WorkerID  string    `json:"worker_id"`
	WorkType  string    `json:"work_type"`
	Issuer    string    `json:"issuer"`
	IssuedAt  time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// Rating is a recorded human or AI score. It is advisory only and can never
// change the worker status (WFM-011).
type Rating struct {
	ID                string    `json:"id"`
	TenantID          string    `json:"tenant_id"`
	WorkerID          string    `json:"worker_id"`
	Score             int       `json:"score"`
	Source            string    `json:"source"`
	Comment           string    `json:"comment,omitempty"`
	RecordedBy        string    `json:"recorded_by"`
	RecordedAt        time.Time `json:"recorded_at"`
	WorkerStatusAfter string    `json:"worker_status_after"`
}

// AdverseActionReview is the only path that rejects, suspends, or terminates a
// worker. It shows the evidence considered and is decided by a distinct human
// reviewer (WFM-011, WFM-012).
type AdverseActionReview struct {
	ID            string    `json:"id"`
	TenantID      string    `json:"tenant_id"`
	WorkerID      string    `json:"worker_id"`
	Action        string    `json:"action"`
	EvidenceRefs  []string  `json:"evidence_refs"`
	ReviewerID    string    `json:"reviewer_id"`
	Reason        string    `json:"reason"`
	WorkerVersion int       `json:"worker_version"`
	DecidedAt     time.Time `json:"decided_at"`
}

// WorkforceAssignment is the durable record of an operations assignment after
// the hard eligibility checks pass.
type WorkforceAssignment struct {
	ID         string    `json:"id"`
	TenantID   string    `json:"tenant_id"`
	WorkerID   string    `json:"worker_id"`
	WorkType   string    `json:"work_type"`
	AssignedBy string    `json:"assigned_by"`
	AssignedAt time.Time `json:"assigned_at"`
}

type CreateWorkerParams struct {
	TenantID         string
	LegalName        string
	VerifiedIdentity bool
	DateOfBirth      time.Time
	ContactMethod    string
	Classification   string
	Specialist       bool
	ServiceZone      string
	Skills           []string
	InitialStatus    string
}

type CertificationParams struct {
	WorkType  string
	Issuer    string
	IssuedAt  time.Time
	ExpiresAt time.Time
}

type RatingParams struct {
	Score         int
	Source        string
	Comment       string
	DesiredStatus string
}

type AdverseActionParams struct {
	Action       string
	EvidenceRefs []string
	ReviewerID   string
	Reason       string
}

// AvailabilityWindow records a window (day of week + time range) when a
// worker is available for operations assignment.
type AvailabilityWindow struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	WorkerID    string    `json:"worker_id"`
	DayOfWeek   int       `json:"day_of_week"`
	StartMinute int       `json:"start_minute"`
	EndMinute   int       `json:"end_minute"`
	EffectiveAt time.Time `json:"effective_at"`
	CreatedAt   time.Time `json:"created_at"`
}

type AvailabilityWindowParams struct {
	DayOfWeek   int
	StartMinute int
	EndMinute   int
	EffectiveAt time.Time
}

// TimeEntry records a discrete block of work, travel, or overtime for a
// worker. Overtime is flagged separately and always attributed to a
// human-approved ticket or dispatch.
type TimeEntry struct {
	ID            string    `json:"id"`
	TenantID      string    `json:"tenant_id"`
	WorkerID      string    `json:"worker_id"`
	TicketID      string    `json:"ticket_id,omitempty"`
	WorkMinutes   int       `json:"work_minutes"`
	TravelMinutes int       `json:"travel_minutes"`
	OvertimeFlag  bool      `json:"overtime_flag"`
	RecordedBy    string    `json:"recorded_by"`
	RecordedAt    time.Time `json:"recorded_at"`
}

type TimeEntryParams struct {
	TicketID      string
	WorkMinutes   int
	TravelMinutes int
	OvertimeFlag  bool
}

// Expense records an out-of-pocket or reimbursable expense incurred by a
// worker, linked to a ticket or dispatch when applicable.
type Expense struct {
	ID         string    `json:"id"`
	TenantID   string    `json:"tenant_id"`
	WorkerID   string    `json:"worker_id"`
	TicketID   string    `json:"ticket_id,omitempty"`
	MinorUnits int64     `json:"minor_units"`
	Currency   string    `json:"currency"`
	Category   string    `json:"category"`
	ReceiptRef string    `json:"receipt_ref,omitempty"`
	RecordedBy string    `json:"recorded_by"`
	RecordedAt time.Time `json:"recorded_at"`
}

type ExpenseParams struct {
	TicketID   string
	MinorUnits int64
	Currency   string
	Category   string
	ReceiptRef string
}

// Grievance records a worker grievance or complaint with mandatory reason
// and reference evidence. It is tenant-scoped and never hard-deleted.
type Grievance struct {
	ID           string     `json:"id"`
	TenantID     string     `json:"tenant_id"`
	WorkerID     string     `json:"worker_id"`
	Kind         string     `json:"kind"`
	Reason       string     `json:"reason"`
	EvidenceRefs []string   `json:"evidence_refs,omitempty"`
	Status       string     `json:"status"`
	SubmittedAt  time.Time  `json:"submitted_at"`
	ResolvedAt   *time.Time `json:"resolved_at,omitempty"`
}

type GrievanceParams struct {
	Kind         string
	Reason       string
	EvidenceRefs []string
}

// SOSEvent records a worker-triggered safety alert (SOS). It freezes the
// worker state and queues an immediate human review.
type SOSEvent struct {
	ID             string     `json:"id"`
	TenantID       string     `json:"tenant_id"`
	WorkerID       string     `json:"worker_id"`
	TicketID       string     `json:"ticket_id,omitempty"`
	Location       string     `json:"location,omitempty"`
	TriggeredAt    time.Time  `json:"triggered_at"`
	AcknowledgedBy string     `json:"acknowledged_by,omitempty"`
	AcknowledgedAt *time.Time `json:"acknowledged_at,omitempty"`
	Resolution     string     `json:"resolution,omitempty"`
	ResolvedAt     *time.Time `json:"resolved_at,omitempty"`
}

type SOSEventParams struct {
	TicketID string
	Location string
}

// EmploymentTerm records a versioned worker agreement or contract row. It
// captures the role, compensation band, effective dates, and any signed
// document references.
type EmploymentTerm struct {
	ID               string     `json:"id"`
	TenantID         string     `json:"tenant_id"`
	WorkerID         string     `json:"worker_id"`
	Role             string     `json:"role"`
	CompensationBand string     `json:"compensation_band,omitempty"`
	EffectiveDate    time.Time  `json:"effective_date"`
	EndDate          *time.Time `json:"end_date,omitempty"`
	AgreementRef     string     `json:"agreement_ref,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

type EmploymentTermParams struct {
	Role             string
	CompensationBand string
	EffectiveDate    time.Time
	EndDate          *time.Time
	AgreementRef     string
}
