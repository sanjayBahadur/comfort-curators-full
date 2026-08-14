package documents

import (
	"errors"
	"time"
)

var (
	ErrDocumentNotFound          = errors.New("document not found")
	ErrDocumentVersionNotFound   = errors.New("document version not found")
	ErrVersionNotModifiable      = errors.New("existing document version cannot be modified")
	ErrDocumentExpired           = errors.New("document has expired")
	ErrReviewNotFound            = errors.New("review not found")
	ErrReviewAlreadyCompleted    = errors.New("review already completed")
	ErrExtractionNotFound        = errors.New("extraction not found")
	ErrSubmissionPacketNotFound  = errors.New("submission packet not found")
	ErrPacketAlreadySubmitted    = errors.New("submission packet already submitted")
	ErrPacketNotComplete         = errors.New("submission packet is not complete")
	ErrHumanReviewRequired       = errors.New("human review required: low confidence or legal field")
	ErrAICannotCertify           = errors.New("AI cannot certify, sign or file documents")
	ErrDuplicateVersion          = errors.New("version already exists for this document")
	ErrCrossTenantDenied         = errors.New("cross-tenant access denied")
	ErrInvalidDocument           = errors.New("invalid document")
	ErrInvalidVersion            = errors.New("invalid document version")
	ErrInvalidExtraction         = errors.New("invalid extraction")
	ErrInvalidReview             = errors.New("invalid review")
	ErrInvalidSubmissionPacket   = errors.New("invalid submission packet")
	ErrInvalidSubmissionRequest  = errors.New("invalid submission request")
	ErrSignatureAuthorityMissing = errors.New("signature authority required")
)

const (
	DocTypeAgreement        = "agreement"
	DocTypeComplianceCert   = "compliance_cert"
	DocTypeInsurancePolicy  = "insurance_policy"
	DocTypeInspectionReport = "inspection_report"
	DocTypeGovernmentID     = "government_id"
	DocTypePropertyDeed     = "property_deed"
	DocTypeTaxDocument      = "tax_document"
	DocTypeEvidencePhoto    = "evidence_photo"
	DocTypeOther            = "other"
)

var validDocTypes = map[string]bool{
	DocTypeAgreement:        true,
	DocTypeComplianceCert:   true,
	DocTypeInsurancePolicy:  true,
	DocTypeInspectionReport: true,
	DocTypeGovernmentID:     true,
	DocTypePropertyDeed:     true,
	DocTypeTaxDocument:      true,
	DocTypeEvidencePhoto:    true,
	DocTypeOther:            true,
}

func ValidDocType(t string) bool {
	return validDocTypes[t]
}

const (
	DocStatusDraft      = "draft"
	DocStatusActive     = "active"
	DocStatusExpired    = "expired"
	DocStatusRevoked    = "revoked"
	DocStatusSuperseded = "superseded"
)

const (
	ReviewStatusPending   = "pending"
	ReviewStatusApproved  = "approved"
	ReviewStatusRejected  = "rejected"
	ReviewStatusEscalated = "escalated"
)

const (
	PacketStatusDraft     = "draft"
	PacketStatusComplete  = "complete"
	PacketStatusSubmitted = "submitted"
	PacketStatusConfirmed = "confirmed"
	PacketStatusRejected  = "rejected"
)

const (
	ConfidenceHigh   = "high"
	ConfidenceMedium = "medium"
	ConfidenceLow    = "low"
)

const (
	FieldCategoryLegal      = "legal"
	FieldCategoryFinancial  = "financial"
	FieldCategoryIdentity   = "identity"
	FieldCategoryProperty   = "property"
	FieldCategoryGeneral    = "general"
	FieldCategoryCompliance = "compliance"
)

type Document struct {
	ID             string     `json:"id"`
	TenantID       string     `json:"tenant_id"`
	PropertyID     string     `json:"property_id"`
	Title          string     `json:"title"`
	DocumentType   string     `json:"document_type"`
	Status         string     `json:"status"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	CurrentVersion int        `json:"current_version"`
	Version        int        `json:"version"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type DocumentVersion struct {
	ID            string    `json:"id"`
	DocumentID    string    `json:"document_id"`
	TenantID      string    `json:"tenant_id"`
	VersionNumber int       `json:"version_number"`
	ContentHash   string    `json:"content_hash"`
	ObjectKey     string    `json:"object_key"`
	Filename      string    `json:"filename"`
	ContentType   string    `json:"content_type"`
	SizeBytes     int64     `json:"size_bytes"`
	UploadedBy    string    `json:"uploaded_by"`
	Metadata      string    `json:"metadata"`
	CreatedAt     time.Time `json:"created_at"`
}

type Extraction struct {
	ID                string    `json:"id"`
	DocumentVersionID string    `json:"document_version_id"`
	TenantID          string    `json:"tenant_id"`
	FieldName         string    `json:"field_name"`
	FieldValue        string    `json:"field_value"`
	FieldCategory     string    `json:"field_category"`
	SourceLocation    string    `json:"source_location"`
	Confidence        string    `json:"confidence"`
	ConfidenceScore   float64   `json:"confidence_score"`
	ExtractedBy       string    `json:"extracted_by"`
	ExtractedAt       time.Time `json:"extracted_at"`
}

type Review struct {
	ID                string    `json:"id"`
	DocumentID        string    `json:"document_id"`
	DocumentVersionID string    `json:"document_version_id"`
	TenantID          string    `json:"tenant_id"`
	ReviewerID        string    `json:"reviewer_id"`
	Status            string    `json:"status"`
	Decision          string    `json:"decision"`
	Comments          string    `json:"comments"`
	ReviewedAt        time.Time `json:"reviewed_at"`
	CreatedAt         time.Time `json:"created_at"`
}

type SubmissionPacket struct {
	ID          string     `json:"id"`
	TenantID    string     `json:"tenant_id"`
	PropertyID  string     `json:"property_id"`
	Status      string     `json:"status"`
	DocumentIDs []string   `json:"document_ids"`
	CreatedBy   string     `json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
	SubmittedAt *time.Time `json:"submitted_at,omitempty"`
	Version     int        `json:"version"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type SubmissionReceipt struct {
	ID                  string               `json:"id"`
	PacketID            string               `json:"packet_id"`
	TenantID            string               `json:"tenant_id"`
	ConfirmedBy         string               `json:"confirmed_by"`
	ReceiptHash         string               `json:"receipt_hash"`
	DocumentVersionRefs []DocumentVersionRef `json:"document_version_refs"`
	ConfirmedAt         time.Time            `json:"confirmed_at"`
}

type DocumentVersionRef struct {
	DocumentID        string `json:"document_id"`
	DocumentVersionID string `json:"document_version_id"`
	VersionNumber     int    `json:"version_number"`
	ContentHash       string `json:"content_hash"`
}

type CreateDocumentParams struct {
	Title        string `json:"title"`
	DocumentType string `json:"document_type"`
	PropertyID   string `json:"property_id"`
}

type CreateVersionParams struct {
	ContentHash string `json:"content_hash"`
	ObjectKey   string `json:"object_key"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
	Metadata    string `json:"metadata"`
}

type CreateExtractionParams struct {
	FieldName       string  `json:"field_name"`
	FieldValue      string  `json:"field_value"`
	FieldCategory   string  `json:"field_category"`
	SourceLocation  string  `json:"source_location"`
	Confidence      string  `json:"confidence"`
	ConfidenceScore float64 `json:"confidence_score"`
	ExtractedBy     string  `json:"extracted_by"`
}

type CreateReviewParams struct {
	Status   string `json:"status"`
	Decision string `json:"decision"`
	Comments string `json:"comments"`
}

type CreateSubmissionPacketParams struct {
	PropertyID  string   `json:"property_id"`
	DocumentIDs []string `json:"document_ids"`
}

type ConfirmSubmissionParams struct {
	ReviewerAuth string `json:"reviewer_auth"`
}
