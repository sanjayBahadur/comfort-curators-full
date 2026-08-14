package privacy

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"comfort-curators-backend/internal/platform/audit"
	"comfort-curators-backend/internal/platform/database"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PrivacyService struct {
	pool       *pgxpool.Pool
	store      *PrivacyStore
	auditStore *audit.AuditStore
}

func NewPrivacyService(pool *pgxpool.Pool, auditStore *audit.AuditStore) *PrivacyService {
	return &PrivacyService{
		pool:       pool,
		store:      NewPrivacyStore(pool),
		auditStore: auditStore,
	}
}

// --- Purposes ---

func (s *PrivacyService) CreatePurpose(ctx context.Context, params CreatePurposeParams, tenantID, actorID string) (*Purpose, error) {
	if tenantID == "" || params.Name == "" || params.LawfulBasis == "" {
		return nil, fmt.Errorf("%w: tenant_id, name, and lawful_basis are required", ErrInvalidPurpose)
	}
	now := time.Now().UTC()
	p := &Purpose{
		TenantID:            tenantID,
		Name:                params.Name,
		Description:         params.Description,
		DataCategories:      params.DataCategories,
		LawfulBasis:         params.LawfulBasis,
		RetentionPeriodDays: params.RetentionPeriodDays,
		Active:              true,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if p.DataCategories == nil {
		p.DataCategories = []string{}
	}
	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertPurpose(ctx, tx, p); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "privacy.purpose.create",
			ResourceType: "privacy_purpose",
			ResourceID:   p.ID,
		})
	}); err != nil {
		return nil, err
	}
	return p, nil
}

func (s *PrivacyService) GetPurpose(ctx context.Context, tenantID, purposeID string) (*Purpose, error) {
	return s.store.GetPurpose(ctx, tenantID, purposeID)
}

func (s *PrivacyService) ListPurposes(ctx context.Context, tenantID string) ([]Purpose, error) {
	purposes, err := s.store.ListPurposes(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if purposes == nil {
		purposes = []Purpose{}
	}
	return purposes, nil
}

// --- Notices ---

func (s *PrivacyService) RecordNotice(ctx context.Context, params CreateNoticeParams, tenantID, actorID string) (*Notice, error) {
	if tenantID == "" || params.PurposeID == "" || params.NoticeText == "" {
		return nil, fmt.Errorf("%w: tenant_id, purpose_id, and notice_text are required", ErrInvalidNotice)
	}
	now := time.Now().UTC()
	n := &Notice{
		TenantID:    tenantID,
		ActorID:     params.ActorID,
		PurposeID:   params.PurposeID,
		NoticeText:  params.NoticeText,
		Version:     params.Version,
		Language:    params.Language,
		DeliveredAt: now,
		CreatedAt:   now,
	}
	if n.Version == "" {
		n.Version = "1.0"
	}
	if n.Language == "" {
		n.Language = "en"
	}
	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertNotice(ctx, tx, n); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "privacy.notice.record",
			ResourceType: "privacy_notice",
			ResourceID:   n.ID,
		})
	}); err != nil {
		return nil, err
	}
	return n, nil
}

func (s *PrivacyService) GetNotice(ctx context.Context, tenantID, noticeID string) (*Notice, error) {
	return s.store.GetNotice(ctx, tenantID, noticeID)
}

func (s *PrivacyService) ListNotices(ctx context.Context, tenantID, actorID string) ([]Notice, error) {
	notices, err := s.store.ListNotices(ctx, tenantID, actorID)
	if err != nil {
		return nil, err
	}
	if notices == nil {
		notices = []Notice{}
	}
	return notices, nil
}

// --- Consents ---

func (s *PrivacyService) RecordConsent(ctx context.Context, params CreateConsentParams, tenantID, actorID string) (*Consent, error) {
	if tenantID == "" || params.PurposeID == "" || params.ActorID == "" {
		return nil, fmt.Errorf("%w: tenant_id, purpose_id, and actor_id are required", ErrInvalidConsent)
	}
	now := time.Now().UTC()
	c := &Consent{
		TenantID:    tenantID,
		ActorID:     params.ActorID,
		PurposeID:   params.PurposeID,
		NoticeID:    params.NoticeID,
		Status:      ConsentStatusActive,
		LawfulBasis: params.LawfulBasis,
		GrantedAt:   now,
		ExpiresAt:   params.ExpiresAt,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if c.LawfulBasis == "" {
		c.LawfulBasis = "consent"
	}
	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertConsent(ctx, tx, c); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeSecurity,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "privacy.consent.grant",
			ResourceType: "privacy_consent",
			ResourceID:   c.ID,
		})
	}); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *PrivacyService) WithdrawConsent(ctx context.Context, consentID, tenantID, actorID, reason string) (*Consent, error) {
	c, err := s.store.GetConsent(ctx, tenantID, consentID)
	if err != nil {
		return nil, err
	}
	if c.Status == ConsentStatusWithdrawn {
		return nil, fmt.Errorf("%w: consent %s already withdrawn", ErrConsentWithdrawn, consentID)
	}
	now := time.Now().UTC()
	c.Status = ConsentStatusWithdrawn
	c.WithdrawnAt = &now
	c.UpdatedAt = now
	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.UpdateConsent(ctx, tx, c); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeSecurity,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "privacy.consent.withdraw",
			ResourceType: "privacy_consent",
			ResourceID:   c.ID,
			Metadata:     mustJSON(map[string]string{"reason": reason}),
		})
	}); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *PrivacyService) GetConsent(ctx context.Context, tenantID, consentID string) (*Consent, error) {
	return s.store.GetConsent(ctx, tenantID, consentID)
}

func (s *PrivacyService) ListConsents(ctx context.Context, tenantID, actorID string) ([]Consent, error) {
	consents, err := s.store.ListConsents(ctx, tenantID, actorID)
	if err != nil {
		return nil, err
	}
	if consents == nil {
		consents = []Consent{}
	}
	return consents, nil
}

// --- Rights Requests (access, correction, withdrawal, grievance, erasure) ---

func validRightsRequestType(t string) bool {
	for _, v := range ValidRightsRequestTypes {
		if v == t {
			return true
		}
	}
	return false
}

func (s *PrivacyService) SubmitRightsRequest(ctx context.Context, params CreateRightsRequestParams, tenantID, actorID string) (*RightsRequest, error) {
	if tenantID == "" || !validRightsRequestType(params.RequestType) {
		return nil, fmt.Errorf("%w: invalid request_type %q", ErrInvalidRightsRequest, params.RequestType)
	}
	now := time.Now().UTC()
	rr := &RightsRequest{
		TenantID:       tenantID,
		ActorID:        params.ActorID,
		RequestType:    params.RequestType,
		Status:         RightsRequestStatusPending,
		Description:    params.Description,
		RelatedData:    params.RelatedData,
		CorrectionData: params.CorrectionData,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertRightsRequest(ctx, tx, rr); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeSecurity,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "privacy.rights_request.submit",
			ResourceType: "privacy_rights_request",
			ResourceID:   rr.ID,
		})
	}); err != nil {
		return nil, err
	}
	return rr, nil
}

// ProcessRightsRequest handles the review/execution of a rights request.
// For erasure requests, it checks retention records and blocks if applicable.
func (s *PrivacyService) ProcessRightsRequest(ctx context.Context, requestID, tenantID, reviewerID string, approved bool, responseData, blockReason string) (*RightsRequest, error) {
	rr, err := s.store.GetRightsRequest(ctx, tenantID, requestID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	rr.ReviewedBy = reviewerID
	rr.ReviewedAt = &now
	rr.UpdatedAt = now

	if !approved {
		rr.Status = RightsRequestStatusRejected
		rr.BlockReason = blockReason
		if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
			if err := s.store.UpdateRightsRequest(ctx, tx, rr); err != nil {
				return err
			}
			return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
				EventType:    audit.EventTypeSecurity,
				TenantID:     tenantID,
				ActorID:      reviewerID,
				Action:       "privacy.rights_request.reject",
				ResourceType: "privacy_rights_request",
				ResourceID:   rr.ID,
				Metadata:     mustJSON(map[string]string{"reason": blockReason}),
			})
		}); err != nil {
			return nil, err
		}
		return rr, nil
	}

	// For erasure, check if there's an active retention record blocking it
	if rr.RequestType == RightsRequestTypeErasure {
		retentions, err := s.store.ListActiveRetentionRecords(ctx, tenantID, rr.ActorID)
		if err != nil {
			return nil, fmt.Errorf("check retention records: %w", err)
		}
		if len(retentions) > 0 {
			rr.Status = RightsRequestStatusBlocked
			rr.BlockReason = fmt.Sprintf("erasure blocked: %d active retention record(s) exist. Reason: %s",
				len(retentions), retentions[0].Reason)
			if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
				if err := s.store.UpdateRightsRequest(ctx, tx, rr); err != nil {
					return err
				}
				return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
					EventType:    audit.EventTypeSecurity,
					TenantID:     tenantID,
					ActorID:      reviewerID,
					Action:       "privacy.erasure.blocked_by_retention",
					ResourceType: "privacy_rights_request",
					ResourceID:   rr.ID,
					Metadata:     mustJSON(map[string]any{"retention_count": len(retentions)}),
				})
			}); err != nil {
				return nil, err
			}
			return rr, nil
		}
	}

	rr.Status = RightsRequestStatusCompleted
	rr.ResponseData = responseData
	rr.CompletedAt = &now
	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.UpdateRightsRequest(ctx, tx, rr); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeSecurity,
			TenantID:     tenantID,
			ActorID:      reviewerID,
			Action:       "privacy.rights_request.complete",
			ResourceType: "privacy_rights_request",
			ResourceID:   rr.ID,
		})
	}); err != nil {
		return nil, err
	}
	return rr, nil
}

func (s *PrivacyService) GetRightsRequest(ctx context.Context, tenantID, requestID string) (*RightsRequest, error) {
	return s.store.GetRightsRequest(ctx, tenantID, requestID)
}

func (s *PrivacyService) ListRightsRequests(ctx context.Context, tenantID, actorID string) ([]RightsRequest, error) {
	reqs, err := s.store.ListRightsRequests(ctx, tenantID, actorID)
	if err != nil {
		return nil, err
	}
	if reqs == nil {
		reqs = []RightsRequest{}
	}
	return reqs, nil
}

// --- Retention Records ---

func (s *PrivacyService) CreateRetentionRecord(ctx context.Context, params CreateRetentionRecordParams, tenantID, actorID string) (*RetentionRecord, error) {
	if tenantID == "" || params.RecordType == "" || params.LawfulBasis == "" || params.RetainUntil.IsZero() {
		return nil, fmt.Errorf("%w: tenant_id, record_type, lawful_basis, and retain_until are required", ErrInvalidRetentionRecord)
	}
	now := time.Now().UTC()
	r := &RetentionRecord{
		TenantID:          tenantID,
		ActorID:           params.ActorID,
		RecordType:        params.RecordType,
		RecordDescription: params.RecordDescription,
		LawfulBasis:       params.LawfulBasis,
		RetainUntil:       params.RetainUntil,
		Status:            RetentionRecordStatusActive,
		Reason:            params.Reason,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertRetentionRecord(ctx, tx, r); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeSecurity,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "privacy.retention.create",
			ResourceType: "privacy_retention_record",
			ResourceID:   r.ID,
		})
	}); err != nil {
		return nil, err
	}
	return r, nil
}

func (s *PrivacyService) GetRetentionRecord(ctx context.Context, tenantID, recordID string) (*RetentionRecord, error) {
	return s.store.GetRetentionRecord(ctx, tenantID, recordID)
}

func (s *PrivacyService) ListActiveRetentionRecords(ctx context.Context, tenantID, actorID string) ([]RetentionRecord, error) {
	records, err := s.store.ListActiveRetentionRecords(ctx, tenantID, actorID)
	if err != nil {
		return nil, err
	}
	if records == nil {
		records = []RetentionRecord{}
	}
	return records, nil
}

// --- Processor Contracts ---

func (s *PrivacyService) RecordProcessorContract(ctx context.Context, params CreateProcessorContractParams, tenantID, actorID string) (*ProcessorContract, error) {
	if tenantID == "" || params.VendorName == "" {
		return nil, fmt.Errorf("%w: tenant_id and vendor_name are required", ErrInvalidProcessor)
	}
	now := time.Now().UTC()
	pc := &ProcessorContract{
		TenantID:             tenantID,
		VendorName:           params.VendorName,
		VendorContact:        params.VendorContact,
		ContractReference:    params.ContractReference,
		ProcessingScope:      params.ProcessingScope,
		DataCategories:       params.DataCategories,
		SecurityReviewStatus: ProcessorStatusPending,
		Status:               ProcessorStatusPending,
		ExpiresAt:            params.ExpiresAt,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if pc.DataCategories == nil {
		pc.DataCategories = []string{}
	}
	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertProcessorContract(ctx, tx, pc); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeSecurity,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "privacy.processor.create",
			ResourceType: "privacy_processor_contract",
			ResourceID:   pc.ID,
		})
	}); err != nil {
		return nil, err
	}
	return pc, nil
}

func (s *PrivacyService) ReviewProcessorContract(ctx context.Context, contractID, tenantID, reviewerID string, approved bool, reviewNotes string) (*ProcessorContract, error) {
	pc, err := s.store.GetProcessorContract(ctx, tenantID, contractID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if approved {
		pc.SecurityReviewStatus = ProcessorStatusApproved
		pc.Status = ProcessorStatusApproved
	} else {
		pc.SecurityReviewStatus = ProcessorStatusRevoked
		pc.Status = ProcessorStatusRevoked
	}
	pc.SecurityReviewDate = &now
	pc.ReviewerID = reviewerID
	pc.UpdatedAt = now
	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.UpdateProcessorContract(ctx, tx, pc); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeSecurity,
			TenantID:     tenantID,
			ActorID:      reviewerID,
			Action:       "privacy.processor.review",
			ResourceType: "privacy_processor_contract",
			ResourceID:   pc.ID,
			Metadata:     mustJSON(map[string]string{"notes": reviewNotes}),
		})
	}); err != nil {
		return nil, err
	}
	return pc, nil
}

func (s *PrivacyService) GetProcessorContract(ctx context.Context, tenantID, contractID string) (*ProcessorContract, error) {
	return s.store.GetProcessorContract(ctx, tenantID, contractID)
}

func (s *PrivacyService) ListProcessorContracts(ctx context.Context, tenantID string) ([]ProcessorContract, error) {
	contracts, err := s.store.ListProcessorContracts(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if contracts == nil {
		contracts = []ProcessorContract{}
	}
	return contracts, nil
}

// --- Security Log Settings ---

func (s *PrivacyService) SetSecurityLogRetention(ctx context.Context, tenantID, actorID, region string, retentionYears int, incidentProcess string) (*SecurityLogSetting, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidRetentionRecord)
	}
	if region == "" {
		region = SecurityLogRegionIndia
	}
	if retentionYears <= 0 {
		retentionYears = SecurityLogRetentionYearsDefault
	}
	now := time.Now().UTC()
	sls := &SecurityLogSetting{
		TenantID:              tenantID,
		Region:                region,
		RetentionYears:        retentionYears,
		IncidentReportProcess: incidentProcess,
		Active:                true,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.UpsertSecurityLogSetting(ctx, tx, sls); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeSecurity,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "privacy.security_log.set_retention",
			ResourceType: "privacy_security_log_setting",
			ResourceID:   sls.ID,
		})
	}); err != nil {
		return nil, err
	}
	return sls, nil
}

func (s *PrivacyService) GetSecurityLogSetting(ctx context.Context, tenantID, settingID string) (*SecurityLogSetting, error) {
	return s.store.GetSecurityLogSetting(ctx, tenantID, settingID)
}

func (s *PrivacyService) ListSecurityLogSettings(ctx context.Context, tenantID string) ([]SecurityLogSetting, error) {
	settings, err := s.store.ListSecurityLogSettings(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if settings == nil {
		settings = []SecurityLogSetting{}
	}
	return settings, nil
}

// --- Identity Alternatives (non-Aadhaar ID path) ---

func validIdentityType(t string) bool {
	for _, v := range ValidIdentityTypes {
		if v == t {
			return true
		}
	}
	return false
}

func (s *PrivacyService) RecordIdentityAlternative(ctx context.Context, params CreateIdentityAlternativeParams, tenantID, actorID string) (*IdentityAlternative, error) {
	if tenantID == "" || !validIdentityType(params.IdentityType) {
		return nil, fmt.Errorf("%w: invalid identity_type %q", ErrInvalidIdentityAlt, params.IdentityType)
	}
	if params.IdentityType == IdentityTypeAadhaar {
		return nil, ErrAadhaarRequired
	}
	now := time.Now().UTC()
	alt := &IdentityAlternative{
		TenantID:      tenantID,
		ActorID:       actorID,
		IdentityType:  params.IdentityType,
		IdentityValue: params.IdentityValue,
		MaskedValue:   params.MaskedValue,
		Verified:      false,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertIdentityAlternative(ctx, tx, alt); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeSecurity,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "privacy.identity_alternative.create",
			ResourceType: "privacy_identity_alternative",
			ResourceID:   alt.ID,
		})
	}); err != nil {
		return nil, err
	}
	return alt, nil
}

func (s *PrivacyService) GetIdentityAlternative(ctx context.Context, tenantID, altID string) (*IdentityAlternative, error) {
	return s.store.GetIdentityAlternative(ctx, tenantID, altID)
}

func (s *PrivacyService) ListIdentityAlternatives(ctx context.Context, tenantID, actorID string) ([]IdentityAlternative, error) {
	alts, err := s.store.ListIdentityAlternatives(ctx, tenantID, actorID)
	if err != nil {
		return nil, err
	}
	if alts == nil {
		alts = []IdentityAlternative{}
	}
	return alts, nil
}

// --- Aadhaar Preferences ---

func (s *PrivacyService) SetAadhaarPreference(ctx context.Context, actorID, tenantID string, aadhaarProvided bool, aadhaarMasked string, altIDType, altIDValue string) (*AadhaarPreference, error) {
	if tenantID == "" || actorID == "" {
		return nil, fmt.Errorf("%w: tenant_id and actor_id are required", ErrInvalidConsent)
	}
	now := time.Now().UTC()
	ap := &AadhaarPreference{
		TenantID:         tenantID,
		ActorID:          actorID,
		AadhaarProvided:  aadhaarProvided,
		AadhaarMasked:    aadhaarMasked,
		AlternateIDType:  altIDType,
		AlternateIDValue: altIDValue,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.UpsertAadhaarPreference(ctx, tx, ap); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeSecurity,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "privacy.aadhaar_preference.set",
			ResourceType: "privacy_aadhaar_preference",
			ResourceID:   ap.ID,
		})
	}); err != nil {
		return nil, err
	}
	return ap, nil
}

func (s *PrivacyService) GetAadhaarPreference(ctx context.Context, tenantID, actorID string) (*AadhaarPreference, error) {
	return s.store.GetAadhaarPreference(ctx, tenantID, actorID)
}

// --- Evaluation Exports ---

func (s *PrivacyService) RequestEvalExport(ctx context.Context, params CreateEvalExportParams, tenantID, actorID string) (*EvalExport, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidEvalExport)
	}
	now := time.Now().UTC()
	ee := &EvalExport{
		TenantID:               tenantID,
		ActorID:                actorID,
		DatasetName:            params.DatasetName,
		DatasetScope:           params.DatasetScope,
		IsDeidentified:         params.IsDeidentified,
		DeidentificationMethod: params.DeidentificationMethod,
		Status:                 EvalExportStatusCreated,
		CreatedAt:              now,
		UpdatedAt:              now,
	}

	// SEC-012: Production data cannot enter evaluation export by default.
	// If data is not de-identified, deny automatically.
	if !params.IsDeidentified {
		ee.Status = EvalExportStatusDenied
		ee.DenialReason = "production data cannot enter evaluation export without approved de-identification"
	}
	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertEvalExport(ctx, tx, ee); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeSecurity,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "privacy.eval_export.request",
			ResourceType: "privacy_evaluation_export",
			ResourceID:   ee.ID,
		})
	}); err != nil {
		return nil, err
	}
	if ee.Status == EvalExportStatusDenied {
		return ee, ErrProductionDataInEval
	}
	return ee, nil
}

func (s *PrivacyService) ApproveEvalExport(ctx context.Context, exportID, tenantID, reviewerID string) (*EvalExport, error) {
	ee, err := s.store.GetEvalExport(ctx, tenantID, exportID)
	if err != nil {
		return nil, err
	}
	if !ee.IsDeidentified {
		return nil, fmt.Errorf("%w: export %s is not de-identified", ErrProductionDataInEval, exportID)
	}
	now := time.Now().UTC()
	ee.Status = EvalExportStatusApproved
	ee.ApprovedBy = reviewerID
	ee.UpdatedAt = now
	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.UpdateEvalExport(ctx, tx, ee); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeSecurity,
			TenantID:     tenantID,
			ActorID:      reviewerID,
			Action:       "privacy.eval_export.approve",
			ResourceType: "privacy_evaluation_export",
			ResourceID:   ee.ID,
		})
	}); err != nil {
		return nil, err
	}
	return ee, nil
}

func (s *PrivacyService) DenyEvalExport(ctx context.Context, exportID, tenantID, reviewerID, reason string) (*EvalExport, error) {
	ee, err := s.store.GetEvalExport(ctx, tenantID, exportID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	ee.Status = EvalExportStatusDenied
	ee.DenialReason = reason
	ee.ApprovedBy = reviewerID
	ee.UpdatedAt = now
	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.UpdateEvalExport(ctx, tx, ee); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeSecurity,
			TenantID:     tenantID,
			ActorID:      reviewerID,
			Action:       "privacy.eval_export.deny",
			ResourceType: "privacy_evaluation_export",
			ResourceID:   ee.ID,
			Metadata:     mustJSON(map[string]string{"reason": reason}),
		})
	}); err != nil {
		return nil, err
	}
	return ee, nil
}

func (s *PrivacyService) GetEvalExport(ctx context.Context, tenantID, exportID string) (*EvalExport, error) {
	return s.store.GetEvalExport(ctx, tenantID, exportID)
}

func (s *PrivacyService) ListEvalExports(ctx context.Context, tenantID string) ([]EvalExport, error) {
	exports, err := s.store.ListEvalExports(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if exports == nil {
		exports = []EvalExport{}
	}
	return exports, nil
}

// --- Helpers ---

func (s *PrivacyService) appendAudit(ctx context.Context, evt audit.AuditEvent) {
	if s.auditStore == nil {
		return
	}
	if evt.ID == "" {
		evt.ID = newID("aud")
	}
	if err := s.auditStore.Append(ctx, evt); err != nil {
		return
	}
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}
