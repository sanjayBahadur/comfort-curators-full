package privacy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newID(prefix string) string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(b[:])
}

type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type PrivacyStore struct {
	pool *pgxpool.Pool
}

func NewPrivacyStore(pool *pgxpool.Pool) *PrivacyStore {
	return &PrivacyStore{pool: pool}
}

// Purposes

func (s *PrivacyStore) InsertPurpose(ctx context.Context, q querier, p *Purpose) error {
	if p.ID == "" {
		p.ID = newID("prp")
	}
	catJSON, _ := json.Marshal(p.DataCategories)
	_, err := q.Exec(ctx, `
		INSERT INTO privacy_purposes (
			id, tenant_id, name, description, data_categories,
			lawful_basis, retention_period_days, active, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	`, p.ID, p.TenantID, p.Name, p.Description, catJSON,
		p.LawfulBasis, p.RetentionPeriodDays, p.Active, p.CreatedAt, p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert purpose: %w", err)
	}
	return nil
}

func (s *PrivacyStore) GetPurpose(ctx context.Context, tenantID, purposeID string) (*Purpose, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, name, description, data_categories,
			lawful_basis, retention_period_days, active, created_at, updated_at
		FROM privacy_purposes
		WHERE id = $1 AND tenant_id = $2
	`, purposeID, tenantID)
	return scanPurpose(row)
}

func (s *PrivacyStore) ListPurposes(ctx context.Context, tenantID string) ([]Purpose, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, name, description, data_categories,
			lawful_basis, retention_period_days, active, created_at, updated_at
		FROM privacy_purposes
		WHERE tenant_id = $1
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list purposes: %w", err)
	}
	defer rows.Close()
	var result []Purpose
	for rows.Next() {
		p, err := scanPurpose(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *p)
	}
	return result, rows.Err()
}

func (s *PrivacyStore) UpdatePurpose(ctx context.Context, q querier, p *Purpose) error {
	catJSON, _ := json.Marshal(p.DataCategories)
	tag, err := q.Exec(ctx, `
		UPDATE privacy_purposes
		SET name = $3, description = $4, data_categories = $5,
			lawful_basis = $6, retention_period_days = $7, active = $8,
			updated_at = $9
		WHERE id = $1 AND tenant_id = $2
	`, p.ID, p.TenantID, p.Name, p.Description, catJSON,
		p.LawfulBasis, p.RetentionPeriodDays, p.Active, p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("update purpose: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrPurposeNotFound
	}
	return nil
}

// Notices

func (s *PrivacyStore) InsertNotice(ctx context.Context, q querier, n *Notice) error {
	if n.ID == "" {
		n.ID = newID("pri")
	}
	_, err := q.Exec(ctx, `
		INSERT INTO privacy_notices (
			id, tenant_id, actor_id, purpose_id, notice_text,
			version, language, delivered_at, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`, n.ID, n.TenantID, n.ActorID, n.PurposeID, n.NoticeText,
		n.Version, n.Language, n.DeliveredAt, n.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert notice: %w", err)
	}
	return nil
}

func (s *PrivacyStore) GetNotice(ctx context.Context, tenantID, noticeID string) (*Notice, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, actor_id, purpose_id, notice_text,
			version, language, delivered_at, created_at
		FROM privacy_notices
		WHERE id = $1 AND tenant_id = $2
	`, noticeID, tenantID)
	return scanNotice(row)
}

func (s *PrivacyStore) ListNotices(ctx context.Context, tenantID, actorID string) ([]Notice, error) {
	var rows pgx.Rows
	var err error
	if actorID != "" {
		rows, err = s.pool.Query(ctx, `
			SELECT id, tenant_id, actor_id, purpose_id, notice_text,
				version, language, delivered_at, created_at
			FROM privacy_notices
			WHERE tenant_id = $1 AND actor_id = $2
			ORDER BY delivered_at DESC
		`, tenantID, actorID)
	} else {
		rows, err = s.pool.Query(ctx, `
			SELECT id, tenant_id, actor_id, purpose_id, notice_text,
				version, language, delivered_at, created_at
			FROM privacy_notices
			WHERE tenant_id = $1
			ORDER BY delivered_at DESC
		`, tenantID)
	}
	if err != nil {
		return nil, fmt.Errorf("list notices: %w", err)
	}
	defer rows.Close()
	var result []Notice
	for rows.Next() {
		n, err := scanNotice(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *n)
	}
	return result, rows.Err()
}

// Consents

func (s *PrivacyStore) InsertConsent(ctx context.Context, q querier, c *Consent) error {
	if c.ID == "" {
		c.ID = newID("prc")
	}
	_, err := q.Exec(ctx, `
		INSERT INTO privacy_consents (
			id, tenant_id, actor_id, purpose_id, notice_id,
			status, lawful_basis, granted_at, withdrawn_at, expires_at,
			created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	`, c.ID, c.TenantID, c.ActorID, c.PurposeID, c.NoticeID,
		c.Status, c.LawfulBasis, c.GrantedAt, c.WithdrawnAt, c.ExpiresAt,
		c.CreatedAt, c.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert consent: %w", err)
	}
	return nil
}

func (s *PrivacyStore) GetConsent(ctx context.Context, tenantID, consentID string) (*Consent, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, actor_id, purpose_id, notice_id,
			status, lawful_basis, granted_at, withdrawn_at, expires_at,
			created_at, updated_at
		FROM privacy_consents
		WHERE id = $1 AND tenant_id = $2
	`, consentID, tenantID)
	return scanConsent(row)
}

func (s *PrivacyStore) ListConsents(ctx context.Context, tenantID, actorID string) ([]Consent, error) {
	var rows pgx.Rows
	var err error
	if actorID != "" {
		rows, err = s.pool.Query(ctx, `
			SELECT id, tenant_id, actor_id, purpose_id, notice_id,
				status, lawful_basis, granted_at, withdrawn_at, expires_at,
				created_at, updated_at
			FROM privacy_consents
			WHERE tenant_id = $1 AND actor_id = $2
			ORDER BY granted_at DESC
		`, tenantID, actorID)
	} else {
		rows, err = s.pool.Query(ctx, `
			SELECT id, tenant_id, actor_id, purpose_id, notice_id,
				status, lawful_basis, granted_at, withdrawn_at, expires_at,
				created_at, updated_at
			FROM privacy_consents
			WHERE tenant_id = $1
			ORDER BY granted_at DESC
		`, tenantID)
	}
	if err != nil {
		return nil, fmt.Errorf("list consents: %w", err)
	}
	defer rows.Close()
	var result []Consent
	for rows.Next() {
		c, err := scanConsent(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *c)
	}
	return result, rows.Err()
}

func (s *PrivacyStore) UpdateConsent(ctx context.Context, q querier, c *Consent) error {
	tag, err := q.Exec(ctx, `
		UPDATE privacy_consents
		SET status = $3, withdrawn_at = $4, updated_at = $5
		WHERE id = $1 AND tenant_id = $2
	`, c.ID, c.TenantID, c.Status, c.WithdrawnAt, c.UpdatedAt)
	if err != nil {
		return fmt.Errorf("update consent: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrConsentNotFound
	}
	return nil
}

// Rights Requests

func (s *PrivacyStore) InsertRightsRequest(ctx context.Context, q querier, rr *RightsRequest) error {
	if rr.ID == "" {
		rr.ID = newID("prr")
	}
	_, err := q.Exec(ctx, `
		INSERT INTO privacy_rights_requests (
			id, tenant_id, actor_id, request_type, status,
			description, related_data, correction_data, response_data,
			block_reason, reviewed_by, reviewed_at, completed_at,
			created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
	`, rr.ID, rr.TenantID, rr.ActorID, rr.RequestType, rr.Status,
		rr.Description, rr.RelatedData, rr.CorrectionData, rr.ResponseData,
		rr.BlockReason, rr.ReviewedBy, rr.ReviewedAt, rr.CompletedAt,
		rr.CreatedAt, rr.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert rights request: %w", err)
	}
	return nil
}

func (s *PrivacyStore) GetRightsRequest(ctx context.Context, tenantID, requestID string) (*RightsRequest, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, actor_id, request_type, status,
			description, related_data, correction_data, response_data,
			block_reason, reviewed_by, reviewed_at, completed_at,
			created_at, updated_at
		FROM privacy_rights_requests
		WHERE id = $1 AND tenant_id = $2
	`, requestID, tenantID)
	return scanRightsRequest(row)
}

func (s *PrivacyStore) ListRightsRequests(ctx context.Context, tenantID, actorID string) ([]RightsRequest, error) {
	var rows pgx.Rows
	var err error
	if actorID != "" {
		rows, err = s.pool.Query(ctx, `
			SELECT id, tenant_id, actor_id, request_type, status,
				description, related_data, correction_data, response_data,
				block_reason, reviewed_by, reviewed_at, completed_at,
				created_at, updated_at
			FROM privacy_rights_requests
			WHERE tenant_id = $1 AND actor_id = $2
			ORDER BY created_at DESC
		`, tenantID, actorID)
	} else {
		rows, err = s.pool.Query(ctx, `
			SELECT id, tenant_id, actor_id, request_type, status,
				description, related_data, correction_data, response_data,
				block_reason, reviewed_by, reviewed_at, completed_at,
				created_at, updated_at
			FROM privacy_rights_requests
			WHERE tenant_id = $1
			ORDER BY created_at DESC
		`, tenantID)
	}
	if err != nil {
		return nil, fmt.Errorf("list rights requests: %w", err)
	}
	defer rows.Close()
	var result []RightsRequest
	for rows.Next() {
		rr, err := scanRightsRequest(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *rr)
	}
	return result, rows.Err()
}

func (s *PrivacyStore) UpdateRightsRequest(ctx context.Context, q querier, rr *RightsRequest) error {
	tag, err := q.Exec(ctx, `
		UPDATE privacy_rights_requests
		SET status = $3, response_data = $4, block_reason = $5,
			reviewed_by = $6, reviewed_at = $7, completed_at = $8,
			correction_data = $9, updated_at = $10
		WHERE id = $1 AND tenant_id = $2
	`, rr.ID, rr.TenantID, rr.Status, rr.ResponseData, rr.BlockReason,
		rr.ReviewedBy, rr.ReviewedAt, rr.CompletedAt,
		rr.CorrectionData, rr.UpdatedAt)
	if err != nil {
		return fmt.Errorf("update rights request: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrRightsRequestNotFound
	}
	return nil
}

// Retention Records

func (s *PrivacyStore) InsertRetentionRecord(ctx context.Context, q querier, r *RetentionRecord) error {
	if r.ID == "" {
		r.ID = newID("prt")
	}
	_, err := q.Exec(ctx, `
		INSERT INTO privacy_retention_records (
			id, tenant_id, actor_id, record_type, record_description,
			lawful_basis, retain_until, status, reason, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
	`, r.ID, r.TenantID, r.ActorID, r.RecordType, r.RecordDescription,
		r.LawfulBasis, r.RetainUntil, r.Status, r.Reason, r.CreatedAt, r.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert retention record: %w", err)
	}
	return nil
}

func (s *PrivacyStore) GetRetentionRecord(ctx context.Context, tenantID, recordID string) (*RetentionRecord, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, actor_id, record_type, record_description,
			lawful_basis, retain_until, status, reason, created_at, updated_at
		FROM privacy_retention_records
		WHERE id = $1 AND tenant_id = $2
	`, recordID, tenantID)
	return scanRetentionRecord(row)
}

func (s *PrivacyStore) ListActiveRetentionRecords(ctx context.Context, tenantID, actorID string) ([]RetentionRecord, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, actor_id, record_type, record_description,
			lawful_basis, retain_until, status, reason, created_at, updated_at
		FROM privacy_retention_records
		WHERE tenant_id = $1 AND actor_id = $2 AND status = $3
		ORDER BY retain_until ASC
	`, tenantID, actorID, RetentionRecordStatusActive)
	if err != nil {
		return nil, fmt.Errorf("list retention records: %w", err)
	}
	defer rows.Close()
	var result []RetentionRecord
	for rows.Next() {
		r, err := scanRetentionRecord(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *r)
	}
	return result, rows.Err()
}

// Processor Contracts

func (s *PrivacyStore) InsertProcessorContract(ctx context.Context, q querier, pc *ProcessorContract) error {
	if pc.ID == "" {
		pc.ID = newID("prp")
	}
	catJSON, _ := json.Marshal(pc.DataCategories)
	_, err := q.Exec(ctx, `
		INSERT INTO privacy_processor_contracts (
			id, tenant_id, vendor_name, vendor_contact, contract_reference,
			processing_scope, data_categories, security_review_status,
			security_review_date, reviewer_id, status, expires_at,
			created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
	`, pc.ID, pc.TenantID, pc.VendorName, pc.VendorContact,
		pc.ContractReference, pc.ProcessingScope, catJSON,
		pc.SecurityReviewStatus, pc.SecurityReviewDate, pc.ReviewerID,
		pc.Status, pc.ExpiresAt, pc.CreatedAt, pc.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert processor contract: %w", err)
	}
	return nil
}

func (s *PrivacyStore) GetProcessorContract(ctx context.Context, tenantID, contractID string) (*ProcessorContract, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, vendor_name, vendor_contact, contract_reference,
			processing_scope, data_categories, security_review_status,
			security_review_date, reviewer_id, status, expires_at,
			created_at, updated_at
		FROM privacy_processor_contracts
		WHERE id = $1 AND tenant_id = $2
	`, contractID, tenantID)
	return scanProcessorContract(row)
}

func (s *PrivacyStore) ListProcessorContracts(ctx context.Context, tenantID string) ([]ProcessorContract, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, vendor_name, vendor_contact, contract_reference,
			processing_scope, data_categories, security_review_status,
			security_review_date, reviewer_id, status, expires_at,
			created_at, updated_at
		FROM privacy_processor_contracts
		WHERE tenant_id = $1
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list processor contracts: %w", err)
	}
	defer rows.Close()
	var result []ProcessorContract
	for rows.Next() {
		pc, err := scanProcessorContract(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *pc)
	}
	return result, rows.Err()
}

func (s *PrivacyStore) UpdateProcessorContract(ctx context.Context, q querier, pc *ProcessorContract) error {
	catJSON, _ := json.Marshal(pc.DataCategories)
	tag, err := q.Exec(ctx, `
		UPDATE privacy_processor_contracts
		SET security_review_status = $3, security_review_date = $4,
			reviewer_id = $5, status = $6, updated_at = $7
		WHERE id = $1 AND tenant_id = $2
	`, pc.ID, pc.TenantID, pc.SecurityReviewStatus, pc.SecurityReviewDate,
		pc.ReviewerID, pc.Status, pc.UpdatedAt)
	if err != nil {
		return fmt.Errorf("update processor contract: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrProcessorNotFound
	}
	_ = catJSON
	return nil
}

// Security Log Settings

func (s *PrivacyStore) UpsertSecurityLogSetting(ctx context.Context, q querier, sls *SecurityLogSetting) error {
	if sls.ID == "" {
		sls.ID = newID("prs")
	}
	_, err := q.Exec(ctx, `
		INSERT INTO privacy_security_log_settings (
			id, tenant_id, region, retention_years,
			incident_report_process, active, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (id) DO UPDATE SET
			region = EXCLUDED.region,
			retention_years = EXCLUDED.retention_years,
			incident_report_process = EXCLUDED.incident_report_process,
			active = EXCLUDED.active,
			updated_at = EXCLUDED.updated_at
	`, sls.ID, sls.TenantID, sls.Region, sls.RetentionYears,
		sls.IncidentReportProcess, sls.Active, sls.CreatedAt, sls.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert security log setting: %w", err)
	}
	return nil
}

func (s *PrivacyStore) GetSecurityLogSetting(ctx context.Context, tenantID, settingID string) (*SecurityLogSetting, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, region, retention_years,
			incident_report_process, active, created_at, updated_at
		FROM privacy_security_log_settings
		WHERE id = $1 AND tenant_id = $2
	`, settingID, tenantID)
	return scanSecurityLogSetting(row)
}

func (s *PrivacyStore) ListSecurityLogSettings(ctx context.Context, tenantID string) ([]SecurityLogSetting, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, region, retention_years,
			incident_report_process, active, created_at, updated_at
		FROM privacy_security_log_settings
		WHERE tenant_id = $1
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list security log settings: %w", err)
	}
	defer rows.Close()
	var result []SecurityLogSetting
	for rows.Next() {
		sls, err := scanSecurityLogSetting(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *sls)
	}
	return result, rows.Err()
}

// Identity Alternatives

func (s *PrivacyStore) InsertIdentityAlternative(ctx context.Context, q querier, alt *IdentityAlternative) error {
	if alt.ID == "" {
		alt.ID = newID("pia")
	}
	_, err := q.Exec(ctx, `
		INSERT INTO privacy_identity_alternatives (
			id, tenant_id, actor_id, identity_type, identity_value,
			masked_value, verification_hash, verified, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	`, alt.ID, alt.TenantID, alt.ActorID, alt.IdentityType,
		alt.IdentityValue, alt.MaskedValue, alt.VerificationHash,
		alt.Verified, alt.CreatedAt, alt.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert identity alternative: %w", err)
	}
	return nil
}

func (s *PrivacyStore) GetIdentityAlternative(ctx context.Context, tenantID, altID string) (*IdentityAlternative, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, actor_id, identity_type, identity_value,
			masked_value, verification_hash, verified, created_at, updated_at
		FROM privacy_identity_alternatives
		WHERE id = $1 AND tenant_id = $2
	`, altID, tenantID)
	return scanIdentityAlternative(row)
}

func (s *PrivacyStore) ListIdentityAlternatives(ctx context.Context, tenantID, actorID string) ([]IdentityAlternative, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, actor_id, identity_type, identity_value,
			masked_value, verification_hash, verified, created_at, updated_at
		FROM privacy_identity_alternatives
		WHERE tenant_id = $1 AND actor_id = $2
		ORDER BY created_at DESC
	`, tenantID, actorID)
	if err != nil {
		return nil, fmt.Errorf("list identity alternatives: %w", err)
	}
	defer rows.Close()
	var result []IdentityAlternative
	for rows.Next() {
		alt, err := scanIdentityAlternative(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *alt)
	}
	return result, rows.Err()
}

// Aadhaar Preferences

func (s *PrivacyStore) UpsertAadhaarPreference(ctx context.Context, q querier, ap *AadhaarPreference) error {
	if ap.ID == "" {
		ap.ID = newID("paa")
	}
	_, err := q.Exec(ctx, `
		INSERT INTO privacy_aadhaar_preferences (
			id, tenant_id, actor_id, aadhaar_provided, aadhaar_masked,
			verification_result, alternate_id_type, alternate_id_value,
			created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (id) DO UPDATE SET
			aadhaar_provided = EXCLUDED.aadhaar_provided,
			aadhaar_masked = EXCLUDED.aadhaar_masked,
			verification_result = EXCLUDED.verification_result,
			alternate_id_type = EXCLUDED.alternate_id_type,
			alternate_id_value = EXCLUDED.alternate_id_value,
			updated_at = EXCLUDED.updated_at
	`, ap.ID, ap.TenantID, ap.ActorID, ap.AadhaarProvided,
		ap.AadhaarMasked, ap.VerificationResult, ap.AlternateIDType,
		ap.AlternateIDValue, ap.CreatedAt, ap.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert aadhaar preference: %w", err)
	}
	return nil
}

func (s *PrivacyStore) GetAadhaarPreference(ctx context.Context, tenantID, actorID string) (*AadhaarPreference, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, actor_id, aadhaar_provided, aadhaar_masked,
			verification_result, alternate_id_type, alternate_id_value,
			created_at, updated_at
		FROM privacy_aadhaar_preferences
		WHERE tenant_id = $1 AND actor_id = $2
		ORDER BY created_at DESC LIMIT 1
	`, tenantID, actorID)
	return scanAadhaarPreference(row)
}

// Evaluation Exports

func (s *PrivacyStore) InsertEvalExport(ctx context.Context, q querier, ee *EvalExport) error {
	if ee.ID == "" {
		ee.ID = newID("pee")
	}
	_, err := q.Exec(ctx, `
		INSERT INTO privacy_evaluation_exports (
			id, tenant_id, actor_id, dataset_name, dataset_scope,
			is_deidentified, deidentification_method, approved_by,
			status, denial_reason, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	`, ee.ID, ee.TenantID, ee.ActorID, ee.DatasetName, ee.DatasetScope,
		ee.IsDeidentified, ee.DeidentificationMethod, ee.ApprovedBy,
		ee.Status, ee.DenialReason, ee.CreatedAt, ee.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert eval export: %w", err)
	}
	return nil
}

func (s *PrivacyStore) GetEvalExport(ctx context.Context, tenantID, exportID string) (*EvalExport, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, actor_id, dataset_name, dataset_scope,
			is_deidentified, deidentification_method, approved_by,
			status, denial_reason, created_at, updated_at
		FROM privacy_evaluation_exports
		WHERE id = $1 AND tenant_id = $2
	`, exportID, tenantID)
	return scanEvalExport(row)
}

func (s *PrivacyStore) ListEvalExports(ctx context.Context, tenantID string) ([]EvalExport, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, actor_id, dataset_name, dataset_scope,
			is_deidentified, deidentification_method, approved_by,
			status, denial_reason, created_at, updated_at
		FROM privacy_evaluation_exports
		WHERE tenant_id = $1
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list eval exports: %w", err)
	}
	defer rows.Close()
	var result []EvalExport
	for rows.Next() {
		ee, err := scanEvalExport(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *ee)
	}
	return result, rows.Err()
}

func (s *PrivacyStore) UpdateEvalExport(ctx context.Context, q querier, ee *EvalExport) error {
	tag, err := q.Exec(ctx, `
		UPDATE privacy_evaluation_exports
		SET status = $3, approved_by = $4, denial_reason = $5,
			updated_at = $6
		WHERE id = $1 AND tenant_id = $2
	`, ee.ID, ee.TenantID, ee.Status, ee.ApprovedBy,
		ee.DenialReason, ee.UpdatedAt)
	if err != nil {
		return fmt.Errorf("update eval export: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrEvalExportNotFound
	}
	return nil
}

// --- Scanner helpers ---

type scanner interface {
	Scan(dest ...any) error
}

func scanPurpose(row scanner) (*Purpose, error) {
	var p Purpose
	var catJSON []byte
	err := row.Scan(
		&p.ID, &p.TenantID, &p.Name, &p.Description, &catJSON,
		&p.LawfulBasis, &p.RetentionPeriodDays, &p.Active,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrPurposeNotFound
		}
		return nil, fmt.Errorf("scan purpose: %w", err)
	}
	if err := json.Unmarshal(catJSON, &p.DataCategories); err != nil {
		p.DataCategories = []string{}
	}
	return &p, nil
}

func scanNotice(row scanner) (*Notice, error) {
	var n Notice
	err := row.Scan(
		&n.ID, &n.TenantID, &n.ActorID, &n.PurposeID, &n.NoticeText,
		&n.Version, &n.Language, &n.DeliveredAt, &n.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNoticeNotFound
		}
		return nil, fmt.Errorf("scan notice: %w", err)
	}
	return &n, nil
}

func scanConsent(row scanner) (*Consent, error) {
	var c Consent
	err := row.Scan(
		&c.ID, &c.TenantID, &c.ActorID, &c.PurposeID, &c.NoticeID,
		&c.Status, &c.LawfulBasis, &c.GrantedAt, &c.WithdrawnAt, &c.ExpiresAt,
		&c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrConsentNotFound
		}
		return nil, fmt.Errorf("scan consent: %w", err)
	}
	return &c, nil
}

func scanRightsRequest(row scanner) (*RightsRequest, error) {
	var rr RightsRequest
	err := row.Scan(
		&rr.ID, &rr.TenantID, &rr.ActorID, &rr.RequestType, &rr.Status,
		&rr.Description, &rr.RelatedData, &rr.CorrectionData, &rr.ResponseData,
		&rr.BlockReason, &rr.ReviewedBy, &rr.ReviewedAt, &rr.CompletedAt,
		&rr.CreatedAt, &rr.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrRightsRequestNotFound
		}
		return nil, fmt.Errorf("scan rights request: %w", err)
	}
	return &rr, nil
}

func scanRetentionRecord(row scanner) (*RetentionRecord, error) {
	var r RetentionRecord
	err := row.Scan(
		&r.ID, &r.TenantID, &r.ActorID, &r.RecordType, &r.RecordDescription,
		&r.LawfulBasis, &r.RetainUntil, &r.Status, &r.Reason,
		&r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrRetentionRecordNotFound
		}
		return nil, fmt.Errorf("scan retention record: %w", err)
	}
	return &r, nil
}

func scanProcessorContract(row scanner) (*ProcessorContract, error) {
	var pc ProcessorContract
	var catJSON []byte
	err := row.Scan(
		&pc.ID, &pc.TenantID, &pc.VendorName, &pc.VendorContact,
		&pc.ContractReference, &pc.ProcessingScope, &catJSON,
		&pc.SecurityReviewStatus, &pc.SecurityReviewDate, &pc.ReviewerID,
		&pc.Status, &pc.ExpiresAt, &pc.CreatedAt, &pc.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrProcessorNotFound
		}
		return nil, fmt.Errorf("scan processor contract: %w", err)
	}
	if err := json.Unmarshal(catJSON, &pc.DataCategories); err != nil {
		pc.DataCategories = []string{}
	}
	return &pc, nil
}

func scanSecurityLogSetting(row scanner) (*SecurityLogSetting, error) {
	var sls SecurityLogSetting
	err := row.Scan(
		&sls.ID, &sls.TenantID, &sls.Region, &sls.RetentionYears,
		&sls.IncidentReportProcess, &sls.Active,
		&sls.CreatedAt, &sls.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrSecurityLogSettingNotFound
		}
		return nil, fmt.Errorf("scan security log setting: %w", err)
	}
	return &sls, nil
}

func scanIdentityAlternative(row scanner) (*IdentityAlternative, error) {
	var alt IdentityAlternative
	err := row.Scan(
		&alt.ID, &alt.TenantID, &alt.ActorID, &alt.IdentityType,
		&alt.IdentityValue, &alt.MaskedValue, &alt.VerificationHash,
		&alt.Verified, &alt.CreatedAt, &alt.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrIdentityAlternativeNotFound
		}
		return nil, fmt.Errorf("scan identity alternative: %w", err)
	}
	return &alt, nil
}

func scanAadhaarPreference(row scanner) (*AadhaarPreference, error) {
	var ap AadhaarPreference
	err := row.Scan(
		&ap.ID, &ap.TenantID, &ap.ActorID, &ap.AadhaarProvided,
		&ap.AadhaarMasked, &ap.VerificationResult,
		&ap.AlternateIDType, &ap.AlternateIDValue,
		&ap.CreatedAt, &ap.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrAadhaarPreferenceNotFound
		}
		return nil, fmt.Errorf("scan aadhaar preference: %w", err)
	}
	return &ap, nil
}

func scanEvalExport(row scanner) (*EvalExport, error) {
	var ee EvalExport
	err := row.Scan(
		&ee.ID, &ee.TenantID, &ee.ActorID, &ee.DatasetName,
		&ee.DatasetScope, &ee.IsDeidentified, &ee.DeidentificationMethod,
		&ee.ApprovedBy, &ee.Status, &ee.DenialReason,
		&ee.CreatedAt, &ee.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrEvalExportNotFound
		}
		return nil, fmt.Errorf("scan eval export: %w", err)
	}
	return &ee, nil
}
