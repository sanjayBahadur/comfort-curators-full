package privacy

import (
	"errors"
	"time"
)

var (
	ErrPurposeNotFound             = errors.New("privacy purpose not found")
	ErrNoticeNotFound              = errors.New("privacy notice not found")
	ErrConsentNotFound             = errors.New("privacy consent not found")
	ErrRightsRequestNotFound       = errors.New("privacy rights request not found")
	ErrRetentionRecordNotFound     = errors.New("retention record not found")
	ErrProcessorNotFound           = errors.New("processor contract not found")
	ErrSecurityLogSettingNotFound  = errors.New("security log setting not found")
	ErrAadhaarPreferenceNotFound   = errors.New("aadhaar preference not found")
	ErrIdentityAlternativeNotFound = errors.New("identity alternative not found")
	ErrEvalExportNotFound          = errors.New("evaluation export not found")
	ErrInvalidPurpose              = errors.New("invalid privacy purpose")
	ErrInvalidNotice               = errors.New("invalid privacy notice")
	ErrInvalidConsent              = errors.New("invalid privacy consent")
	ErrInvalidRightsRequest        = errors.New("invalid rights request")
	ErrInvalidRetentionRecord      = errors.New("invalid retention record")
	ErrInvalidProcessor            = errors.New("invalid processor contract")
	ErrInvalidIdentityAlt          = errors.New("invalid identity alternative")
	ErrInvalidEvalExport           = errors.New("invalid evaluation export")
	ErrErasureBlockedByRetention   = errors.New("erasure blocked by retention obligation")
	ErrAadhaarRequired             = errors.New("aadhaar is optional; use alternate identity method")
	ErrProductionDataInEval        = errors.New("production data cannot enter evaluation export by default")
	ErrConsentWithdrawn            = errors.New("consent has been withdrawn")
)

const (
	RightsRequestStatusPending    = "pending"
	RightsRequestStatusInProgress = "in_progress"
	RightsRequestStatusCompleted  = "completed"
	RightsRequestStatusRejected   = "rejected"
	RightsRequestStatusBlocked    = "blocked"
	RightsRequestStatusWithdrawn  = "withdrawn"

	RightsRequestTypeAccess     = "access"
	RightsRequestTypeCorrection = "correction"
	RightsRequestTypeWithdrawal = "withdrawal"
	RightsRequestTypeGrievance  = "grievance"
	RightsRequestTypeErasure    = "erasure"

	ConsentStatusActive    = "active"
	ConsentStatusWithdrawn = "withdrawn"
	ConsentStatusExpired   = "expired"

	RetentionRecordStatusActive   = "active"
	RetentionRecordStatusExpired  = "expired"
	RetentionRecordStatusReleased = "released"

	ProcessorStatusPending  = "pending_review"
	ProcessorStatusApproved = "approved"
	ProcessorStatusRevoked  = "revoked"
	ProcessorStatusExpired  = "expired"

	EvalExportStatusCreated  = "created"
	EvalExportStatusDenied   = "denied"
	EvalExportStatusApproved = "approved"
	EvalExportStatusExported = "exported"

	SecurityLogRegionIndia           = "IN"
	SecurityLogRetentionYearsDefault = 5

	IdentityTypeAadhaar        = "aadhaar"
	IdentityTypePAN            = "pan"
	IdentityTypePassport       = "passport"
	IdentityTypeVoterID        = "voter_id"
	IdentityTypeDrivingLicence = "driving_licence"
	IdentityTypeOther          = "other"
)

var ValidRightsRequestTypes = []string{
	RightsRequestTypeAccess,
	RightsRequestTypeCorrection,
	RightsRequestTypeWithdrawal,
	RightsRequestTypeGrievance,
	RightsRequestTypeErasure,
}

var ValidRightsRequestStatuses = []string{
	RightsRequestStatusPending,
	RightsRequestStatusInProgress,
	RightsRequestStatusCompleted,
	RightsRequestStatusRejected,
	RightsRequestStatusBlocked,
	RightsRequestStatusWithdrawn,
}

var ValidIdentityTypes = []string{
	IdentityTypeAadhaar,
	IdentityTypePAN,
	IdentityTypePassport,
	IdentityTypeVoterID,
	IdentityTypeDrivingLicence,
	IdentityTypeOther,
}

// Purpose defines a documented purpose for collecting personal data (SEC-001).
type Purpose struct {
	ID                  string    `json:"id"`
	TenantID            string    `json:"tenant_id"`
	Name                string    `json:"name"`
	Description         string    `json:"description"`
	DataCategories      []string  `json:"data_categories"`
	LawfulBasis         string    `json:"lawful_basis"`
	RetentionPeriodDays int       `json:"retention_period_days"`
	Active              bool      `json:"active"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// Notice records that a privacy notice was issued to a data subject (SEC-002).
type Notice struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	ActorID     string    `json:"actor_id"`
	PurposeID   string    `json:"purpose_id"`
	NoticeText  string    `json:"notice_text"`
	Version     string    `json:"version"`
	Language    string    `json:"language"`
	DeliveredAt time.Time `json:"delivered_at"`
	CreatedAt   time.Time `json:"created_at"`
}

// Consent records a data subject's consent or a lawful-basis record (SEC-002).
type Consent struct {
	ID          string     `json:"id"`
	TenantID    string     `json:"tenant_id"`
	ActorID     string     `json:"actor_id"`
	PurposeID   string     `json:"purpose_id"`
	NoticeID    string     `json:"notice_id"`
	Status      string     `json:"status"`
	LawfulBasis string     `json:"lawful_basis"`
	GrantedAt   time.Time  `json:"granted_at"`
	WithdrawnAt *time.Time `json:"withdrawn_at,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// RightsRequest records a data subject's rights exercise (access, correction,
// withdrawal, grievance, erasure) per SEC-003.
type RightsRequest struct {
	ID             string     `json:"id"`
	TenantID       string     `json:"tenant_id"`
	ActorID        string     `json:"actor_id"`
	RequestType    string     `json:"request_type"`
	Status         string     `json:"status"`
	Description    string     `json:"description"`
	RelatedData    string     `json:"related_data,omitempty"`
	CorrectionData string     `json:"correction_data,omitempty"`
	ResponseData   string     `json:"response_data,omitempty"`
	BlockReason    string     `json:"block_reason,omitempty"`
	ReviewedBy     string     `json:"reviewed_by,omitempty"`
	ReviewedAt     *time.Time `json:"reviewed_at,omitempty"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// RetentionRecord records a retention obligation that can block erasure (SEC-003).
type RetentionRecord struct {
	ID                string    `json:"id"`
	TenantID          string    `json:"tenant_id"`
	ActorID           string    `json:"actor_id"`
	RecordType        string    `json:"record_type"`
	RecordDescription string    `json:"record_description"`
	LawfulBasis       string    `json:"lawful_basis"`
	RetainUntil       time.Time `json:"retain_until"`
	Status            string    `json:"status"`
	Reason            string    `json:"reason"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// ProcessorContract records a vendor handling personal data and their
// processor contract / security review per SEC-011.
type ProcessorContract struct {
	ID                   string     `json:"id"`
	TenantID             string     `json:"tenant_id"`
	VendorName           string     `json:"vendor_name"`
	VendorContact        string     `json:"vendor_contact"`
	ContractReference    string     `json:"contract_reference"`
	ProcessingScope      string     `json:"processing_scope"`
	DataCategories       []string   `json:"data_categories"`
	SecurityReviewStatus string     `json:"security_review_status"`
	SecurityReviewDate   *time.Time `json:"security_review_date,omitempty"`
	ReviewerID           string     `json:"reviewer_id,omitempty"`
	Status               string     `json:"status"`
	ExpiresAt            *time.Time `json:"expires_at,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

// SecurityLogSetting records the retention region and period for security
// logs per SEC-010.
type SecurityLogSetting struct {
	ID                    string    `json:"id"`
	TenantID              string    `json:"tenant_id"`
	Region                string    `json:"region"`
	RetentionYears        int       `json:"retention_years"`
	IncidentReportProcess string    `json:"incident_report_process"`
	Active                bool      `json:"active"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

// IdentityAlternative provides an alternate identity path when Aadhaar
// is not provided per SEC-004.
type IdentityAlternative struct {
	ID               string    `json:"id"`
	TenantID         string    `json:"tenant_id"`
	ActorID          string    `json:"actor_id"`
	IdentityType     string    `json:"identity_type"`
	IdentityValue    string    `json:"identity_value"`
	MaskedValue      string    `json:"masked_value"`
	VerificationHash string    `json:"verification_hash,omitempty"`
	Verified         bool      `json:"verified"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// AadhaarPreference records the data subject's preference for Aadhaar
// usage (masked, optional) per SEC-004, SEC-005.
type AadhaarPreference struct {
	ID                 string    `json:"id"`
	TenantID           string    `json:"tenant_id"`
	ActorID            string    `json:"actor_id"`
	AadhaarProvided    bool      `json:"aadhaar_provided"`
	AadhaarMasked      string    `json:"aadhaar_masked,omitempty"`
	VerificationResult bool      `json:"verification_result,omitempty"`
	AlternateIDType    string    `json:"alternate_id_type,omitempty"`
	AlternateIDValue   string    `json:"alternate_id_value,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// EvalExport records a model-evaluation export attempt and its disposition
// per SEC-012. Production data cannot enter evaluation export by default.
type EvalExport struct {
	ID                     string    `json:"id"`
	TenantID               string    `json:"tenant_id"`
	ActorID                string    `json:"actor_id"`
	DatasetName            string    `json:"dataset_name"`
	DatasetScope           string    `json:"dataset_scope"`
	IsDeidentified         bool      `json:"is_deidentified"`
	DeidentificationMethod string    `json:"deidentification_method,omitempty"`
	ApprovedBy             string    `json:"approved_by,omitempty"`
	Status                 string    `json:"status"`
	DenialReason           string    `json:"denial_reason,omitempty"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

// Params types for creating records.

type CreatePurposeParams struct {
	Name                string
	Description         string
	DataCategories      []string
	LawfulBasis         string
	RetentionPeriodDays int
}

type CreateNoticeParams struct {
	ActorID    string
	PurposeID  string
	NoticeText string
	Version    string
	Language   string
}

type CreateConsentParams struct {
	ActorID     string
	PurposeID   string
	NoticeID    string
	LawfulBasis string
	ExpiresAt   *time.Time
}

type CreateRightsRequestParams struct {
	ActorID        string
	RequestType    string
	Description    string
	RelatedData    string
	CorrectionData string
}

type CreateRetentionRecordParams struct {
	ActorID           string
	RecordType        string
	RecordDescription string
	LawfulBasis       string
	RetainUntil       time.Time
	Reason            string
}

type CreateProcessorContractParams struct {
	VendorName        string
	VendorContact     string
	ContractReference string
	ProcessingScope   string
	DataCategories    []string
	ExpiresAt         *time.Time
}

type CreateIdentityAlternativeParams struct {
	IdentityType  string
	IdentityValue string
	MaskedValue   string
}

type CreateEvalExportParams struct {
	DatasetName            string
	DatasetScope           string
	IsDeidentified         bool
	DeidentificationMethod string
}
