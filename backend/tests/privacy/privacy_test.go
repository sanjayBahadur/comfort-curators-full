package privacy_test

import (
	"context"
	"errors"
	"net"
	"os"
	"testing"
	"time"

	"comfort-curators-backend/internal/platform/audit"
	"comfort-curators-backend/internal/platform/testdb"
	"comfort-curators-backend/internal/privacy"

	"github.com/jackc/pgx/v5/pgxpool"
)

func postgresAvailable() bool {
	host := os.Getenv("CC_DB_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("CC_DB_PORT")
	if port == "" {
		port = "5432"
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func dbConnString() string {
	host := os.Getenv("CC_DB_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("CC_DB_PORT")
	if port == "" {
		port = "5432"
	}
	user := os.Getenv("CC_DB_USER")
	if user == "" {
		user = "ccuser"
	}
	pass := os.Getenv("CC_DB_PASS")
	if pass == "" {
		pass = "ccpass"
	}
	name := testdb.MustName()
	return "postgres://" + user + ":" + pass + "@" + host + ":" + port + "/" + name + "?sslmode=disable"
}

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if !postgresAvailable() {
		t.Skip("PostgreSQL not available for privacy integration test")
	}
	pool, err := pgxpool.New(context.Background(), dbConnString())
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(context.Background()); err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}
	return pool
}

func ensureSchemas(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	if err := privacy.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure privacy schema: %v", err)
	}
	if err := audit.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure audit schema: %v", err)
	}
}

func cleanup(t *testing.T, pool *pgxpool.Pool, tables ...string) {
	t.Helper()
	ctx := context.Background()
	for _, table := range tables {
		_, _ = pool.Exec(ctx, "DELETE FROM "+table)
	}
}

// --- Purpose Tests ---

func TestCreatePurpose(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	ensureSchemas(t, pool)
	cleanup(t, pool, "privacy_purposes")

	tenantID := "tenant-prv-purpose"
	svc := privacy.NewPrivacyService(pool, audit.NewAuditStore(pool))

	p, err := svc.CreatePurpose(ctx, privacy.CreatePurposeParams{
		Name:                "Guest Identity Verification",
		Description:         "Collect identity documents to verify guest identity as required by law",
		DataCategories:      []string{"name", "contact", "government_id"},
		LawfulBasis:         "legal_obligation",
		RetentionPeriodDays: 365,
	}, tenantID, "staff-1")
	if err != nil {
		t.Fatalf("create purpose: %v", err)
	}
	if p.ID == "" {
		t.Error("expected non-empty ID")
	}
	if p.Name != "Guest Identity Verification" {
		t.Errorf("expected name, got %q", p.Name)
	}
	if p.LawfulBasis != "legal_obligation" {
		t.Errorf("expected lawful_basis legal_obligation, got %q", p.LawfulBasis)
	}
	if len(p.DataCategories) != 3 {
		t.Errorf("expected 3 data categories, got %d", len(p.DataCategories))
	}

	got, err := svc.GetPurpose(ctx, tenantID, p.ID)
	if err != nil {
		t.Fatalf("get purpose: %v", err)
	}
	if got.Name != p.Name {
		t.Errorf("got name %q, want %q", got.Name, p.Name)
	}

	purposes, err := svc.ListPurposes(ctx, tenantID)
	if err != nil {
		t.Fatalf("list purposes: %v", err)
	}
	if len(purposes) != 1 {
		t.Errorf("expected 1 purpose, got %d", len(purposes))
	}
}

func TestCreatePurposeValidation(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	ensureSchemas(t, pool)

	svc := privacy.NewPrivacyService(pool, audit.NewAuditStore(pool))

	_, err := svc.CreatePurpose(ctx, privacy.CreatePurposeParams{
		Name: "",
	}, "", "staff-1")
	if !errors.Is(err, privacy.ErrInvalidPurpose) {
		t.Errorf("empty tenant must fail, got %v", err)
	}
}

// --- Notice Tests ---

func TestRecordNotice(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	ensureSchemas(t, pool)
	cleanup(t, pool, "privacy_notices")

	tenantID := "tenant-prv-notice"
	svc := privacy.NewPrivacyService(pool, audit.NewAuditStore(pool))

	n, err := svc.RecordNotice(ctx, privacy.CreateNoticeParams{
		ActorID:    "user-1",
		PurposeID:  "pur-001",
		NoticeText: "Your data will be used for identity verification as required by law.",
		Version:    "1.0",
		Language:   "en",
	}, tenantID, "staff-1")
	if err != nil {
		t.Fatalf("record notice: %v", err)
	}
	if n.ID == "" {
		t.Error("expected non-empty ID")
	}
	if n.NoticeText != "Your data will be used for identity verification as required by law." {
		t.Errorf("unexpected notice text: %q", n.NoticeText)
	}

	got, err := svc.GetNotice(ctx, tenantID, n.ID)
	if err != nil {
		t.Fatalf("get notice: %v", err)
	}
	if got.ActorID != "user-1" {
		t.Errorf("expected actor user-1, got %q", got.ActorID)
	}
}

// --- Consent Tests ---

func TestRecordAndWithdrawConsent(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	ensureSchemas(t, pool)
	cleanup(t, pool, "privacy_consents")

	tenantID := "tenant-prv-consent"
	svc := privacy.NewPrivacyService(pool, audit.NewAuditStore(pool))

	c, err := svc.RecordConsent(ctx, privacy.CreateConsentParams{
		ActorID:     "user-1",
		PurposeID:   "pur-001",
		NoticeID:    "not-001",
		LawfulBasis: "consent",
	}, tenantID, "staff-1")
	if err != nil {
		t.Fatalf("record consent: %v", err)
	}
	if c.Status != privacy.ConsentStatusActive {
		t.Errorf("expected active consent, got %q", c.Status)
	}

	withdrawn, err := svc.WithdrawConsent(ctx, c.ID, tenantID, "staff-1", "user request")
	if err != nil {
		t.Fatalf("withdraw consent: %v", err)
	}
	if withdrawn.Status != privacy.ConsentStatusWithdrawn {
		t.Errorf("expected withdrawn status, got %q", withdrawn.Status)
	}
	if withdrawn.WithdrawnAt == nil {
		t.Error("expected withdrawn_at to be set")
	}

	_, err = svc.WithdrawConsent(ctx, c.ID, tenantID, "staff-1", "double")
	if !errors.Is(err, privacy.ErrConsentWithdrawn) {
		t.Errorf("double withdraw must fail, got %v", err)
	}
}

// --- Rights Request Tests ---

func TestSubmitAccessRequest(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	ensureSchemas(t, pool)
	cleanup(t, pool, "privacy_rights_requests")

	tenantID := "tenant-prv-access"
	svc := privacy.NewPrivacyService(pool, audit.NewAuditStore(pool))

	rr, err := svc.SubmitRightsRequest(ctx, privacy.CreateRightsRequestParams{
		ActorID:     "user-1",
		RequestType: privacy.RightsRequestTypeAccess,
		Description: "I would like a copy of my personal data",
	}, tenantID, "staff-1")
	if err != nil {
		t.Fatalf("submit access request: %v", err)
	}
	if rr.Status != privacy.RightsRequestStatusPending {
		t.Errorf("expected pending, got %q", rr.Status)
	}
	if rr.RequestType != privacy.RightsRequestTypeAccess {
		t.Errorf("expected access type, got %q", rr.RequestType)
	}

	processed, err := svc.ProcessRightsRequest(ctx, rr.ID, tenantID, "reviewer-1", true, "Your data: name=John, contact=+919999999999", "")
	if err != nil {
		t.Fatalf("process access request: %v", err)
	}
	if processed.Status != privacy.RightsRequestStatusCompleted {
		t.Errorf("expected completed, got %q", processed.Status)
	}
}

func TestSubmitGrievanceRequest(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	ensureSchemas(t, pool)
	cleanup(t, pool, "privacy_rights_requests")

	tenantID := "tenant-prv-griev"
	svc := privacy.NewPrivacyService(pool, audit.NewAuditStore(pool))

	rr, err := svc.SubmitRightsRequest(ctx, privacy.CreateRightsRequestParams{
		ActorID:     "user-1",
		RequestType: privacy.RightsRequestTypeGrievance,
		Description: "My data has been processed without consent",
	}, tenantID, "staff-1")
	if err != nil {
		t.Fatalf("submit grievance: %v", err)
	}
	if rr.RequestType != privacy.RightsRequestTypeGrievance {
		t.Errorf("expected grievance type, got %q", rr.RequestType)
	}
}

// --- Erasure Blocked by Retention ---

func TestErasureBlockedByRetention(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	ensureSchemas(t, pool)
	cleanup(t, pool, "privacy_rights_requests", "privacy_retention_records")

	tenantID := "tenant-prv-erasure"
	svc := privacy.NewPrivacyService(pool, audit.NewAuditStore(pool))

	futureDate := time.Now().UTC().Add(365 * 24 * time.Hour)
	_, err := svc.CreateRetentionRecord(ctx, privacy.CreateRetentionRecordParams{
		ActorID:           "user-1",
		RecordType:        "financial_transaction",
		RecordDescription: "Tax-related financial records for FY 2025-26",
		LawfulBasis:       "legal_obligation",
		RetainUntil:       futureDate,
		Reason:            "Income Tax Act requires 7-year retention",
	}, tenantID, "staff-1")
	if err != nil {
		t.Fatalf("create retention record: %v", err)
	}

	rr, err := svc.SubmitRightsRequest(ctx, privacy.CreateRightsRequestParams{
		ActorID:     "user-1",
		RequestType: privacy.RightsRequestTypeErasure,
		Description: "Please delete all my data",
	}, tenantID, "staff-1")
	if err != nil {
		t.Fatalf("submit erasure request: %v", err)
	}

	processed, err := svc.ProcessRightsRequest(ctx, rr.ID, tenantID, "reviewer-1", true, "", "")
	if err != nil {
		t.Fatalf("process erasure request: %v", err)
	}
	if processed.Status != privacy.RightsRequestStatusBlocked {
		t.Errorf("expected blocked status for erasure with active retention, got %q", processed.Status)
	}
	if processed.BlockReason == "" {
		t.Error("expected block reason to be non-empty")
	}
}

// --- Retention Record Tests ---

func TestRetentionRecordBlocksErasure(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	ensureSchemas(t, pool)
	cleanup(t, pool, "privacy_retention_records", "privacy_rights_requests")

	tenantID := "tenant-prv-retention"
	svc := privacy.NewPrivacyService(pool, audit.NewAuditStore(pool))

	futureDate := time.Now().UTC().Add(180 * 24 * time.Hour)
	record, err := svc.CreateRetentionRecord(ctx, privacy.CreateRetentionRecordParams{
		ActorID:           "user-2",
		RecordType:        "security_log",
		RecordDescription: "CERT-In required security logs",
		LawfulBasis:       "legal_obligation",
		RetainUntil:       futureDate,
		Reason:            "India CERT-In directions require 5-year security log retention",
	}, tenantID, "compliance-1")
	if err != nil {
		t.Fatalf("create retention record: %v", err)
	}
	if record.Reason == "" {
		t.Error("expected a non-empty reason for retention")
	}

	records, err := svc.ListActiveRetentionRecords(ctx, tenantID, "user-2")
	if err != nil {
		t.Fatalf("list retention records: %v", err)
	}
	if len(records) != 1 {
		t.Errorf("expected 1 retention record, got %d", len(records))
	}
}

// --- Processor Contract Tests ---

func TestProcessorContractReview(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	ensureSchemas(t, pool)
	cleanup(t, pool, "privacy_processor_contracts")

	tenantID := "tenant-prv-processor"
	svc := privacy.NewPrivacyService(pool, audit.NewAuditStore(pool))

	pc, err := svc.RecordProcessorContract(ctx, privacy.CreateProcessorContractParams{
		VendorName:        "CloudData Processing Pvt Ltd",
		VendorContact:     "dpo@clouddata.example",
		ContractReference: "DPA-2026-001",
		ProcessingScope:   "Storage and processing of guest identity documents",
		DataCategories:    []string{"name", "contact", "government_ids"},
	}, tenantID, "procurement-1")
	if err != nil {
		t.Fatalf("record processor: %v", err)
	}
	if pc.SecurityReviewStatus != privacy.ProcessorStatusPending {
		t.Errorf("expected pending review, got %q", pc.SecurityReviewStatus)
	}

	reviewed, err := svc.ReviewProcessorContract(ctx, pc.ID, tenantID, "dpo-1", true, "Security review passed. ISO 27001 certified.")
	if err != nil {
		t.Fatalf("review processor: %v", err)
	}
	if reviewed.SecurityReviewStatus != privacy.ProcessorStatusApproved {
		t.Errorf("expected approved, got %q", reviewed.SecurityReviewStatus)
	}

	contracts, err := svc.ListProcessorContracts(ctx, tenantID)
	if err != nil {
		t.Fatalf("list processor contracts: %v", err)
	}
	if len(contracts) != 1 {
		t.Errorf("expected 1 contract, got %d", len(contracts))
	}
}

// --- Security Log Retention Tests ---

func TestSecurityLogRetention(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	ensureSchemas(t, pool)
	cleanup(t, pool, "privacy_security_log_settings")

	tenantID := "tenant-prv-seclog"
	svc := privacy.NewPrivacyService(pool, audit.NewAuditStore(pool))

	sls, err := svc.SetSecurityLogRetention(ctx, tenantID, "admin-1", privacy.SecurityLogRegionIndia, 5, "CERT-In incident reporting via incident@comfortcurators.example")
	if err != nil {
		t.Fatalf("set security log retention: %v", err)
	}
	if sls.Region != privacy.SecurityLogRegionIndia {
		t.Errorf("expected region IN, got %q", sls.Region)
	}
	if sls.RetentionYears != 5 {
		t.Errorf("expected 5 years, got %d", sls.RetentionYears)
	}

	settings, err := svc.ListSecurityLogSettings(ctx, tenantID)
	if err != nil {
		t.Fatalf("list security log settings: %v", err)
	}
	if len(settings) != 1 {
		t.Errorf("expected 1 setting, got %d", len(settings))
	}
}

// --- Identity Alternative Tests (Aadhaar optional) ---

func TestIdentityAlternativeRejectsAadhaar(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	ensureSchemas(t, pool)
	cleanup(t, pool, "privacy_identity_alternatives")

	tenantID := "tenant-prv-altid"
	svc := privacy.NewPrivacyService(pool, audit.NewAuditStore(pool))

	_, err := svc.RecordIdentityAlternative(ctx, privacy.CreateIdentityAlternativeParams{
		IdentityType:  privacy.IdentityTypeAadhaar,
		IdentityValue: "1234-5678-9012",
		MaskedValue:   "XXXX-XXXX-9012",
	}, tenantID, "user-1")
	if !errors.Is(err, privacy.ErrAadhaarRequired) {
		t.Errorf("aadhaar must be rejected via identity alternative path, got %v", err)
	}

	alt, err := svc.RecordIdentityAlternative(ctx, privacy.CreateIdentityAlternativeParams{
		IdentityType:  privacy.IdentityTypePAN,
		IdentityValue: "ABCDE1234F",
		MaskedValue:   "XXXXX1234F",
	}, tenantID, "user-1")
	if err != nil {
		t.Fatalf("record identity alternative: %v", err)
	}
	if alt.IdentityType != privacy.IdentityTypePAN {
		t.Errorf("expected PAN, got %q", alt.IdentityType)
	}
	if alt.Verified {
		t.Error("new alternative must not be pre-verified")
	}
}

// --- Aadhaar Preference Tests ---

func TestAadhaarIsOptional(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	ensureSchemas(t, pool)
	cleanup(t, pool, "privacy_aadhaar_preferences")

	tenantID := "tenant-prv-aadhaar"
	svc := privacy.NewPrivacyService(pool, audit.NewAuditStore(pool))

	ap, err := svc.SetAadhaarPreference(ctx, "user-1", tenantID, false, "", "pan", "ABCDE1234F")
	if err != nil {
		t.Fatalf("set aadhaar preference: %v", err)
	}
	if ap.AadhaarProvided {
		t.Error("aadhaar must not be required")
	}
	if ap.AlternateIDType != "pan" {
		t.Errorf("expected alternate ID type PAN, got %q", ap.AlternateIDType)
	}

	got, err := svc.GetAadhaarPreference(ctx, tenantID, "user-1")
	if err != nil {
		t.Fatalf("get aadhaar preference: %v", err)
	}
	if got.AadhaarProvided {
		t.Error("retrieved preference must show aadhaar not provided")
	}
}

func TestAadhaarMaskedPreference(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	ensureSchemas(t, pool)
	cleanup(t, pool, "privacy_aadhaar_preferences")

	tenantID := "tenant-prv-masked"
	svc := privacy.NewPrivacyService(pool, audit.NewAuditStore(pool))

	ap, err := svc.SetAadhaarPreference(ctx, "user-2", tenantID, true, "XXXX-XXXX-9012", "", "")
	if err != nil {
		t.Fatalf("set aadhaar preference: %v", err)
	}
	if ap.AadhaarMasked != "XXXX-XXXX-9012" {
		t.Errorf("expected masked aadhaar, got %q", ap.AadhaarMasked)
	}
	if !ap.AadhaarProvided {
		t.Error("expected aadhaar_provided=true")
	}
}

// --- Evaluation Export Tests (production data cannot enter by default) ---

func TestEvaluationExportDeniesProductionData(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	ensureSchemas(t, pool)
	cleanup(t, pool, "privacy_evaluation_exports")

	tenantID := "tenant-prv-eval"
	svc := privacy.NewPrivacyService(pool, audit.NewAuditStore(pool))

	ee, err := svc.RequestEvalExport(ctx, privacy.CreateEvalExportParams{
		DatasetName:            "guest_identity_data",
		DatasetScope:           "all_tenants",
		IsDeidentified:         false,
		DeidentificationMethod: "",
	}, tenantID, "ml-engineer-1")
	if err == nil {
		t.Fatal("expected error for non-deidentified evaluation export")
	}
	if !errors.Is(err, privacy.ErrProductionDataInEval) {
		t.Errorf("expected ErrProductionDataInEval, got %v", err)
	}
	if ee.Status != privacy.EvalExportStatusDenied {
		t.Errorf("expected denied status, got %q", ee.Status)
	}
	if ee.DenialReason == "" {
		t.Error("expected non-empty denial reason")
	}
}

func TestEvaluationExportDeidentifiedAllowed(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	ensureSchemas(t, pool)
	cleanup(t, pool, "privacy_evaluation_exports")

	tenantID := "tenant-prv-deid"
	svc := privacy.NewPrivacyService(pool, audit.NewAuditStore(pool))

	ee, err := svc.RequestEvalExport(ctx, privacy.CreateEvalExportParams{
		DatasetName:            "anonymized_guest_preferences",
		DatasetScope:           "single_tenant",
		IsDeidentified:         true,
		DeidentificationMethod: "k-anonymity-v3",
	}, tenantID, "ml-engineer-1")
	if err != nil {
		t.Fatalf("request eval export: %v", err)
	}
	if ee.Status != privacy.EvalExportStatusCreated {
		t.Errorf("expected created status, got %q", ee.Status)
	}
	if !ee.IsDeidentified {
		t.Error("expected is_deidentified=true")
	}

	approved, err := svc.ApproveEvalExport(ctx, ee.ID, tenantID, "dpo-1")
	if err != nil {
		t.Fatalf("approve eval export: %v", err)
	}
	if approved.Status != privacy.EvalExportStatusApproved {
		t.Errorf("expected approved status, got %q", approved.Status)
	}
}

// --- Rights Request Validation ---

func TestRightsRequestValidation(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	ensureSchemas(t, pool)

	svc := privacy.NewPrivacyService(pool, audit.NewAuditStore(pool))

	_, err := svc.SubmitRightsRequest(ctx, privacy.CreateRightsRequestParams{
		ActorID:     "user-1",
		RequestType: "invalid_type",
	}, "tenant-1", "staff-1")
	if !errors.Is(err, privacy.ErrInvalidRightsRequest) {
		t.Errorf("invalid request type must fail, got %v", err)
	}
}

// --- Processor Contract Denied ---

func TestProcessorContractReviewDenied(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	ensureSchemas(t, pool)
	cleanup(t, pool, "privacy_processor_contracts")

	tenantID := "tenant-prv-proc-deny"
	svc := privacy.NewPrivacyService(pool, audit.NewAuditStore(pool))

	pc, err := svc.RecordProcessorContract(ctx, privacy.CreateProcessorContractParams{
		VendorName:      "UnsecureVendor Ltd",
		ProcessingScope: "Personal data processing",
		DataCategories:  []string{"health_data"},
	}, tenantID, "proc-1")
	if err != nil {
		t.Fatalf("record processor: %v", err)
	}

	reviewed, err := svc.ReviewProcessorContract(ctx, pc.ID, tenantID, "dpo-1", false, "Failed security audit: no encryption at rest")
	if err != nil {
		t.Fatalf("review processor: %v", err)
	}
	if reviewed.Status != privacy.ProcessorStatusRevoked {
		t.Errorf("expected revoked status, got %q", reviewed.Status)
	}
}

// --- Erasure Without Retention ---

func TestErasureWithoutRetentionSucceeds(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	ensureSchemas(t, pool)
	cleanup(t, pool, "privacy_rights_requests", "privacy_retention_records")

	tenantID := "tenant-prv-erasure-ok"
	svc := privacy.NewPrivacyService(pool, audit.NewAuditStore(pool))

	rr, err := svc.SubmitRightsRequest(ctx, privacy.CreateRightsRequestParams{
		ActorID:     "user-3",
		RequestType: privacy.RightsRequestTypeErasure,
		Description: "Delete my data, no retention applies",
	}, tenantID, "staff-1")
	if err != nil {
		t.Fatalf("submit erasure request: %v", err)
	}

	processed, err := svc.ProcessRightsRequest(ctx, rr.ID, tenantID, "reviewer-1", true, "Data has been deleted", "")
	if err != nil {
		t.Fatalf("process erasure: %v", err)
	}
	if processed.Status != privacy.RightsRequestStatusCompleted {
		t.Errorf("expected completed status (no retention blocks), got %q", processed.Status)
	}
}

// --- Correction Request ---

func TestCorrectionRequest(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	ensureSchemas(t, pool)
	cleanup(t, pool, "privacy_rights_requests")

	tenantID := "tenant-prv-correct"
	svc := privacy.NewPrivacyService(pool, audit.NewAuditStore(pool))

	rr, err := svc.SubmitRightsRequest(ctx, privacy.CreateRightsRequestParams{
		ActorID:        "user-1",
		RequestType:    privacy.RightsRequestTypeCorrection,
		Description:    "My name is spelled incorrectly",
		CorrectionData: `{"name":"corrected name"}`,
	}, tenantID, "staff-1")
	if err != nil {
		t.Fatalf("submit correction request: %v", err)
	}
	if rr.RequestType != privacy.RightsRequestTypeCorrection {
		t.Errorf("expected correction type, got %q", rr.RequestType)
	}
	if rr.CorrectionData != `{"name":"corrected name"}` {
		t.Errorf("expected correction data, got %q", rr.CorrectionData)
	}
}

// --- Withdrawal Request ---

func TestWithdrawalRequest(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	ensureSchemas(t, pool)
	cleanup(t, pool, "privacy_rights_requests")

	tenantID := "tenant-prv-withdraw-req"
	svc := privacy.NewPrivacyService(pool, audit.NewAuditStore(pool))

	rr, err := svc.SubmitRightsRequest(ctx, privacy.CreateRightsRequestParams{
		ActorID:     "user-1",
		RequestType: privacy.RightsRequestTypeWithdrawal,
		Description: "I withdraw consent for marketing communications",
	}, tenantID, "staff-1")
	if err != nil {
		t.Fatalf("submit withdrawal request: %v", err)
	}
	if rr.RequestType != privacy.RightsRequestTypeWithdrawal {
		t.Errorf("expected withdrawal type, got %q", rr.RequestType)
	}
}

// --- Rejected Rights Request ---

func TestRejectedRightsRequest(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	ensureSchemas(t, pool)
	cleanup(t, pool, "privacy_rights_requests")

	tenantID := "tenant-prv-reject"
	svc := privacy.NewPrivacyService(pool, audit.NewAuditStore(pool))

	rr, err := svc.SubmitRightsRequest(ctx, privacy.CreateRightsRequestParams{
		ActorID:     "user-1",
		RequestType: privacy.RightsRequestTypeAccess,
		Description: "Request for data",
	}, tenantID, "staff-1")
	if err != nil {
		t.Fatalf("submit request: %v", err)
	}

	processed, err := svc.ProcessRightsRequest(ctx, rr.ID, tenantID, "reviewer-1", false, "", "Identity not verified")
	if err != nil {
		t.Fatalf("process request: %v", err)
	}
	if processed.Status != privacy.RightsRequestStatusRejected {
		t.Errorf("expected rejected status, got %q", processed.Status)
	}
	if processed.BlockReason != "Identity not verified" {
		t.Errorf("expected block reason, got %q", processed.BlockReason)
	}
}

// --- Multiple Purposes List ---

func TestListPurposesMultiple(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	ensureSchemas(t, pool)
	cleanup(t, pool, "privacy_purposes")

	tenantID := "tenant-prv-purposes"
	svc := privacy.NewPrivacyService(pool, audit.NewAuditStore(pool))

	for i, purpose := range []struct{ name, basis string }{
		{"Guest Verification", "legal_obligation"},
		{"Marketing Communications", "consent"},
		{"Property Access Logging", "legitimate_interest"},
	} {
		_, err := svc.CreatePurpose(ctx, privacy.CreatePurposeParams{
			Name:                purpose.name,
			LawfulBasis:         purpose.basis,
			DataCategories:      []string{"identity"},
			RetentionPeriodDays: 365,
		}, tenantID, "staff-1")
		if err != nil {
			t.Fatalf("create purpose %d: %v", i, err)
		}
	}

	purposes, err := svc.ListPurposes(ctx, tenantID)
	if err != nil {
		t.Fatalf("list purposes: %v", err)
	}
	if len(purposes) != 3 {
		t.Errorf("expected 3 purposes, got %d", len(purposes))
	}
}

// --- Identity Alternative List ---

func TestIdentityAlternativeList(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	ensureSchemas(t, pool)
	cleanup(t, pool, "privacy_identity_alternatives")

	tenantID := "tenant-prv-altlist"
	svc := privacy.NewPrivacyService(pool, audit.NewAuditStore(pool))

	for _, idType := range []string{privacy.IdentityTypePAN, privacy.IdentityTypePassport, privacy.IdentityTypeVoterID} {
		_, err := svc.RecordIdentityAlternative(ctx, privacy.CreateIdentityAlternativeParams{
			IdentityType:  idType,
			IdentityValue: "VALUE-" + idType,
			MaskedValue:   "MASKED-" + idType,
		}, tenantID, "user-1")
		if err != nil {
			t.Fatalf("record %s: %v", idType, err)
		}
	}

	alts, err := svc.ListIdentityAlternatives(ctx, tenantID, "user-1")
	if err != nil {
		t.Fatalf("list identity alternatives: %v", err)
	}
	if len(alts) != 3 {
		t.Errorf("expected 3 alternatives, got %d", len(alts))
	}
}
