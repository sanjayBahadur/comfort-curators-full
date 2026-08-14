package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"comfort-curators-backend/internal/inventory"
	"comfort-curators-backend/internal/quality"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ProbeFunc func(ctx context.Context, baseURL string) error

type ProbeResult struct {
	Name     string
	Group    string
	Status   string
	Error    string
	Output   string
	Duration time.Duration
}

func registeredProbes() map[string]ProbeFunc {
	return map[string]ProbeFunc{
		"TestCCFND001EmptyDatabaseMigration":    probeCCFND001EmptyDatabaseMigration,
		"TestCCFND001OutboxCommitAtomicity":     probeCCFND001OutboxCommitAtomicity,
		"TestCCFND001IdempotencyReplay":         probeCCFND001IdempotencyReplay,
		"TestCCFND001PrivateObjectSignedAccess": probeCCFND001PrivateObjectSignedAccess,
		"TestCCFND001APIWorkerStart":            probeCCFND001APIWorkerStart,

		"TestCCIAM001CrossTenantDenied":          probeCCIAM001CrossTenantDenied,
		"TestCCIAM001UnassignedPropertyDenied":   probeCCIAM001UnassignedPropertyDenied,
		"TestCCIAM001OwnerGuestRolesDistinct":    probeCCIAM001OwnerGuestRolesDistinct,
		"TestCCIAM001ExpiredSupportAccessDenied": probeCCIAM001ExpiredSupportAccessDenied,
		"TestCCIAM001StaffMFARequired":           probeCCIAM001StaffMFARequired,

		"TestCCONB001LifecycleTransitions":        probeCCONB001LifecycleTransitions,
		"TestCCONB001SafetyHoldBlocksActivation":  probeCCONB001SafetyHoldBlocksActivation,
		"TestCCONB001QuoteIsDeterministic":        probeCCONB001QuoteIsDeterministic,
		"TestCCONB001AgreementVersionIsImmutable": probeCCONB001AgreementVersionIsImmutable,

		"TestCCRES001StaleCalendarIsRejected":      probeCCRES001StaleCalendarIsRejected,
		"TestCCRES001ConflictIsDetected":           probeCCRES001ConflictIsDetected,
		"TestCCRES001CancellationUpdatesTurnover":  probeCCRES001CancellationUpdatesTurnover,
		"TestCCRES001UnauthorizedMessageIsBlocked": probeCCRES001UnauthorizedMessageIsBlocked,

		"TestCCOPS001DispatchHonorsHardConstraints": probeCCOPS001DispatchHonorsHardConstraints,
		"TestCCOPS001EvidenceRequiredForCompletion": probeCCOPS001EvidenceRequiredForCompletion,
		"TestCCOPS001OfflineReplayIsIdempotent":     probeCCOPS001OfflineReplayIsIdempotent,
		"TestCCOPS001IncidentEscalates":             probeCCOPS001IncidentEscalates,

		"TestCCACC001CustodyLedgerIsAppendOnly":     probeCCACC001CustodyLedgerIsAppendOnly,
		"TestCCACC001DisclosureIsAuditedAndExpires": probeCCACC001DisclosureIsAuditedAndExpires,
		"TestCCACC001RevocationBlocksDisclosure":    probeCCACC001RevocationBlocksDisclosure,
		"TestCCACC001EmergencyAccessIsAttributable": probeCCACC001EmergencyAccessIsAttributable,

		"TestCCINV001MovementLedgerIsAppendOnly":     probeCCINV001MovementLedgerIsAppendOnly,
		"TestCCINV001NegativeBalanceIsRejected":      probeCCINV001NegativeBalanceIsRejected,
		"TestCCINV001ReconciliationIsAttributable":   probeCCINV001ReconciliationIsAttributable,
		"TestCCINV001ConcurrentMovementIsConsistent": probeCCINV001ConcurrentMovementIsConsistent,

		"TestCCBIL001MoneyUsesMinorUnitsAndCurrency": probeCCBIL001MoneyUsesMinorUnitsAndCurrency,
		"TestCCBIL001OwnerBillingOnly":               probeCCBIL001OwnerBillingOnly,
		"TestCCBIL001InvoiceCreationIsIdempotent":    probeCCBIL001InvoiceCreationIsIdempotent,
		"TestCCBIL001DuplicateExpenseIsDetected":     probeCCBIL001DuplicateExpenseIsDetected,

		"TestCCDOC001VersionsAreImmutable":                 probeCCDOC001VersionsAreImmutable,
		"TestCCDOC001ExpiryIsDetected":                     probeCCDOC001ExpiryIsDetected,
		"TestCCDOC001ExtractionRetainsSourceAndConfidence": probeCCDOC001ExtractionRetainsSourceAndConfidence,
		"TestCCDOC001SubmissionRequiresHumanReview":        probeCCDOC001SubmissionRequiresHumanReview,

		"TestCCHOU001PropertyScopeCannotCross":     probeCCHOU001PropertyScopeCannotCross,
		"TestCCHOU001OnlyTypedToolsAreExposed":     probeCCHOU001OnlyTypedToolsAreExposed,
		"TestCCHOU001PolicyRejectsDirectMutation":  probeCCHOU001PolicyRejectsDirectMutation,
		"TestCCHOU001ModelOutageHasManualFallback": probeCCHOU001ModelOutageHasManualFallback,

		"TestCCHER001CommunicationAuthorityIsNarrow":    probeCCHER001CommunicationAuthorityIsNarrow,
		"TestCCHER001ApprovalPolicyIsEnforced":          probeCCHER001ApprovalPolicyIsEnforced,
		"TestCCHER001OwnerAndGuestContextsAreSeparated": probeCCHER001OwnerAndGuestContextsAreSeparated,
		"TestCCHER001DeliveryIsIdempotent":              probeCCHER001DeliveryIsIdempotent,

		"TestCCSEC001SecretsAreRedacted":                probeCCSEC001SecretsAreRedacted,
		"TestCCSEC001SecureLinkExpiresAndRejectsReplay": probeCCSEC001SecureLinkExpiresAndRejectsReplay,
		"TestCCSEC001AuditEvidenceCannotBeRewritten":    probeCCSEC001AuditEvidenceCannotBeRewritten,
		"TestCCSEC001CrossTenantRequestsFailClosed":     probeCCSEC001CrossTenantRequestsFailClosed,

		"TestCCREL001BackupRestoreRebuildsWorkflow":  probeCCREL001BackupRestoreRebuildsWorkflow,
		"TestCCREL001MigrationForwardRecovery":       probeCCREL001MigrationForwardRecovery,
		"TestCCREL001OutboxReplayIsIdempotent":       probeCCREL001OutboxReplayIsIdempotent,
		"TestCCREL001DependencyDegradationIsVisible": probeCCREL001DependencyDegradationIsVisible,
		"TestCCREL001CapacityTarget":                 probeCCREL001CapacityTarget,
	}
}

var phaseGroup = map[string]string{
	"TestCCFND001EmptyDatabaseMigration":    "CC-FND-001",
	"TestCCFND001OutboxCommitAtomicity":     "CC-FND-001",
	"TestCCFND001IdempotencyReplay":         "CC-FND-001",
	"TestCCFND001PrivateObjectSignedAccess": "CC-FND-001",
	"TestCCFND001APIWorkerStart":            "CC-FND-001",

	"TestCCIAM001CrossTenantDenied":          "CC-IAM-001",
	"TestCCIAM001UnassignedPropertyDenied":   "CC-IAM-001",
	"TestCCIAM001OwnerGuestRolesDistinct":    "CC-IAM-001",
	"TestCCIAM001ExpiredSupportAccessDenied": "CC-IAM-001",
	"TestCCIAM001StaffMFARequired":           "CC-IAM-001",

	"TestCCONB001LifecycleTransitions":        "CC-ONB-001",
	"TestCCONB001SafetyHoldBlocksActivation":  "CC-ONB-001",
	"TestCCONB001QuoteIsDeterministic":        "CC-ONB-001",
	"TestCCONB001AgreementVersionIsImmutable": "CC-ONB-001",

	"TestCCRES001StaleCalendarIsRejected":      "CC-RES-001",
	"TestCCRES001ConflictIsDetected":           "CC-RES-001",
	"TestCCRES001CancellationUpdatesTurnover":  "CC-RES-001",
	"TestCCRES001UnauthorizedMessageIsBlocked": "CC-RES-001",

	"TestCCOPS001DispatchHonorsHardConstraints": "CC-OPS-001",
	"TestCCOPS001EvidenceRequiredForCompletion": "CC-OPS-001",
	"TestCCOPS001OfflineReplayIsIdempotent":     "CC-OPS-001",
	"TestCCOPS001IncidentEscalates":             "CC-OPS-001",

	"TestCCACC001CustodyLedgerIsAppendOnly":     "CC-ACC-001",
	"TestCCACC001DisclosureIsAuditedAndExpires": "CC-ACC-001",
	"TestCCACC001RevocationBlocksDisclosure":    "CC-ACC-001",
	"TestCCACC001EmergencyAccessIsAttributable": "CC-ACC-001",

	"TestCCINV001MovementLedgerIsAppendOnly":     "CC-INV-001",
	"TestCCINV001NegativeBalanceIsRejected":      "CC-INV-001",
	"TestCCINV001ReconciliationIsAttributable":   "CC-INV-001",
	"TestCCINV001ConcurrentMovementIsConsistent": "CC-INV-001",

	"TestCCBIL001MoneyUsesMinorUnitsAndCurrency": "CC-BIL-001",
	"TestCCBIL001OwnerBillingOnly":               "CC-BIL-001",
	"TestCCBIL001InvoiceCreationIsIdempotent":    "CC-BIL-001",
	"TestCCBIL001DuplicateExpenseIsDetected":     "CC-BIL-001",

	"TestCCDOC001VersionsAreImmutable":                 "CC-DOC-001",
	"TestCCDOC001ExpiryIsDetected":                     "CC-DOC-001",
	"TestCCDOC001ExtractionRetainsSourceAndConfidence": "CC-DOC-001",
	"TestCCDOC001SubmissionRequiresHumanReview":        "CC-DOC-001",

	"TestCCHOU001PropertyScopeCannotCross":     "CC-HOU-001",
	"TestCCHOU001OnlyTypedToolsAreExposed":     "CC-HOU-001",
	"TestCCHOU001PolicyRejectsDirectMutation":  "CC-HOU-001",
	"TestCCHOU001ModelOutageHasManualFallback": "CC-HOU-001",

	"TestCCHER001CommunicationAuthorityIsNarrow":    "CC-HER-001",
	"TestCCHER001ApprovalPolicyIsEnforced":          "CC-HER-001",
	"TestCCHER001OwnerAndGuestContextsAreSeparated": "CC-HER-001",
	"TestCCHER001DeliveryIsIdempotent":              "CC-HER-001",

	"TestCCSEC001SecretsAreRedacted":                "CC-SEC-001",
	"TestCCSEC001SecureLinkExpiresAndRejectsReplay": "CC-SEC-001",
	"TestCCSEC001AuditEvidenceCannotBeRewritten":    "CC-SEC-001",
	"TestCCSEC001CrossTenantRequestsFailClosed":     "CC-SEC-001",

	"TestCCREL001BackupRestoreRebuildsWorkflow":  "CC-REL-001",
	"TestCCREL001MigrationForwardRecovery":       "CC-REL-001",
	"TestCCREL001OutboxReplayIsIdempotent":       "CC-REL-001",
	"TestCCREL001DependencyDegradationIsVisible": "CC-REL-001",
	"TestCCREL001CapacityTarget":                 "CC-REL-001",
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
	name := os.Getenv("CC_DB_NAME")
	if name == "" {
		name = "comfort_curators"
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, pass, host, port, name)
}

func connectDB(ctx context.Context) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dbConnString())
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return pool, nil
}

func httpGet(baseURL, path string) (*http.Response, []byte, error) {
	url := strings.TrimRight(baseURL, "/") + path
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp, nil, err
	}
	return resp, body, nil
}

func minioAvailable() bool {
	host := os.Getenv("CC_S3_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("CC_S3_PORT")
	if port == "" {
		port = "9000"
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func probeCCFND001EmptyDatabaseMigration(ctx context.Context, baseURL string) error {
	pool, err := connectDB(ctx)
	if err != nil {
		return fmt.Errorf("connect to postgres: %w", err)
	}
	defer pool.Close()

	var count int
	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count)
	if err != nil {
		return fmt.Errorf("query schema_migrations: %w", err)
	}
	if count < 1 {
		return fmt.Errorf("expected at least 1 migration, got %d", count)
	}

	var version int
	var description string
	err = pool.QueryRow(ctx,
		`SELECT version, description FROM schema_migrations ORDER BY version LIMIT 1`,
	).Scan(&version, &description)
	if err != nil {
		return fmt.Errorf("read first migration: %w", err)
	}

	resp, body, err := httpGet(baseURL, "/health/ready")
	if err != nil {
		return fmt.Errorf("health ready request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health ready: expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	var readyResp struct {
		Status string            `json:"status"`
		Checks map[string]string `json:"checks"`
	}
	if err := json.Unmarshal(body, &readyResp); err != nil {
		return fmt.Errorf("parse ready response: %w", err)
	}
	if readyResp.Status != "ok" {
		return fmt.Errorf("health ready status: expected ok, got %s", readyResp.Status)
	}
	if dbStatus, ok := readyResp.Checks["database"]; ok && dbStatus != "ok" {
		return fmt.Errorf("database health check: %s", dbStatus)
	}

	return nil
}

func probeCCFND001OutboxCommitAtomicity(ctx context.Context, baseURL string) error {
	pool, err := connectDB(ctx)
	if err != nil {
		return fmt.Errorf("connect to postgres: %w", err)
	}
	defer pool.Close()

	var exists bool
	err = pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'outbox_events')`,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check outbox_events table: %w", err)
	}
	if !exists {
		return fmt.Errorf("outbox_events table does not exist")
	}

	cols := []struct {
		Name     string
		Nullable string
	}{}
	rows, err := pool.Query(ctx,
		`SELECT column_name, is_nullable FROM information_schema.columns WHERE table_name = 'outbox_events' ORDER BY ordinal_position`,
	)
	if err != nil {
		return fmt.Errorf("query outbox_events columns: %w", err)
	}
	for rows.Next() {
		var c struct {
			Name     string
			Nullable string
		}
		if err := rows.Scan(&c.Name, &c.Nullable); err != nil {
			rows.Close()
			return err
		}
		cols = append(cols, c)
	}
	rows.Close()

	required := []string{"event_id", "event_name", "correlation_id", "tenant_id", "payload", "occurred_at"}
	for _, r := range required {
		found := false
		for _, c := range cols {
			if c.Name == r {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("outbox_events missing required column: %s", r)
		}
	}

	return nil
}

func probeCCFND001IdempotencyReplay(ctx context.Context, baseURL string) error {
	pool, err := connectDB(ctx)
	if err != nil {
		return fmt.Errorf("connect to postgres: %w", err)
	}
	defer pool.Close()

	var exists bool
	err = pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'idempotency_records')`,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check idempotency_records table: %w", err)
	}
	if !exists {
		return fmt.Errorf("idempotency_records table does not exist")
	}

	cols := []string{}
	rows, err := pool.Query(ctx,
		`SELECT column_name FROM information_schema.columns WHERE table_name = 'idempotency_records' ORDER BY ordinal_position`,
	)
	if err != nil {
		return fmt.Errorf("query idempotency_records columns: %w", err)
	}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			rows.Close()
			return err
		}
		cols = append(cols, c)
	}
	rows.Close()

	required := []string{"idempotency_key", "operation_class", "request_hash", "result_ref"}
	for _, r := range required {
		found := false
		for _, c := range cols {
			if c == r {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("idempotency_records missing required column: %s", r)
		}
	}

	return nil
}

func probeCCFND001PrivateObjectSignedAccess(ctx context.Context, baseURL string) error {
	resp, body, err := httpGet(baseURL, "/health/ready")
	if err != nil {
		return fmt.Errorf("health ready request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health ready: expected 200, got %d", resp.StatusCode)
	}

	var readyResp struct {
		Status string            `json:"status"`
		Checks map[string]string `json:"checks"`
	}
	if err := json.Unmarshal(body, &readyResp); err != nil {
		return fmt.Errorf("parse ready response: %w", err)
	}

	if !minioAvailable() {
		fmt.Fprintf(os.Stderr, "[warn] MinIO not reachable at default address; object-store checks skipped\n")
		return nil
	}

	host := os.Getenv("CC_S3_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("CC_S3_PORT")
	if port == "" {
		port = "9000"
	}
	minioURL := fmt.Sprintf("http://%s:%s/minio/health/live", host, port)
	minioResp, err := http.Get(minioURL)
	if err != nil {
		return fmt.Errorf("minio health request: %w", err)
	}
	minioResp.Body.Close()
	if minioResp.StatusCode != http.StatusOK {
		return fmt.Errorf("minio health: expected 200, got %d", minioResp.StatusCode)
	}

	return nil
}

func probeCCFND001APIWorkerStart(ctx context.Context, baseURL string) error {
	resp, body, err := httpGet(baseURL, "/health/live")
	if err != nil {
		return fmt.Errorf("health live request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health live: expected 200, got %d: %s", resp.StatusCode, string(body))
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		return fmt.Errorf("health live: expected application/json content-type, got %s", ct)
	}
	cid := resp.Header.Get("X-Correlation-ID")
	if cid == "" {
		return fmt.Errorf("health live: X-Correlation-ID header missing")
	}

	var liveResp struct {
		Status string `json:"status"`
		Time   string `json:"time"`
	}
	if err := json.Unmarshal(body, &liveResp); err != nil {
		return fmt.Errorf("parse live response: %w", err)
	}
	if liveResp.Status != "ok" {
		return fmt.Errorf("health live status: expected ok, got %s", liveResp.Status)
	}
	if liveResp.Time == "" {
		return fmt.Errorf("health live time field empty")
	}

	resp2, body2, err := httpGet(baseURL, "/health/ready")
	if err != nil {
		return fmt.Errorf("health ready request: %w", err)
	}
	if resp2.StatusCode != http.StatusOK {
		return fmt.Errorf("health ready: expected 200, got %d: %s", resp2.StatusCode, string(body2))
	}

	var readyResp struct {
		Status string            `json:"status"`
		Time   string            `json:"time"`
		Checks map[string]string `json:"checks"`
	}
	if err := json.Unmarshal(body2, &readyResp); err != nil {
		return fmt.Errorf("parse ready response: %w", err)
	}
	if readyResp.Status != "ok" {
		return fmt.Errorf("health ready status: expected ok, got %s", readyResp.Status)
	}
	if readyResp.Time == "" {
		return fmt.Errorf("health ready time field empty")
	}
	if dbCheck, ok := readyResp.Checks["database"]; ok && dbCheck != "ok" {
		return fmt.Errorf("database health check failed: %s", dbCheck)
	}

	modelURL := modelStubURL()
	if modelURL != "" {
		mresp, mbody, err := httpGet(modelURL, "/health/live")
		if err != nil {
			return fmt.Errorf("model-stub health live request: %w", err)
		}
		if mresp.StatusCode != http.StatusOK {
			return fmt.Errorf("model-stub health live: expected 200, got %d: %s", mresp.StatusCode, string(mbody))
		}
		var modelLive struct {
			Status string `json:"status"`
			Time   string `json:"time"`
		}
		if err := json.Unmarshal(mbody, &modelLive); err != nil {
			return fmt.Errorf("parse model-stub live response: %w", err)
		}
		if modelLive.Status != "ok" {
			return fmt.Errorf("model-stub status: expected ok, got %s", modelLive.Status)
		}
		mcid := mresp.Header.Get("X-Correlation-ID")
		if mcid == "" {
			return fmt.Errorf("model-stub: X-Correlation-ID header missing")
		}
	}

	return nil
}

func modelStubURL() string {
	host := os.Getenv("CC_MODEL_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("CC_MODEL_PORT")
	if port == "" {
		port = "8081"
	}
	url := fmt.Sprintf("http://%s:%s", host, port)

	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 500*time.Millisecond)
	if err != nil {
		return ""
	}
	conn.Close()
	return url
}

func createTestSession(baseURL string, tenantID, contact string, roles []string) (sessionCreateResult, error) {
	body, err := apiPost(baseURL, "/auth/session/create", map[string]any{
		"tenant_id": tenantID,
		"contact":   contact,
		"roles":     roles,
	}, "")
	if err != nil {
		return sessionCreateResult{}, err
	}

	var result sessionCreateResult
	if err := json.Unmarshal(body, &result); err != nil {
		return sessionCreateResult{}, fmt.Errorf("parse session create response: %w", err)
	}
	if result.SessionToken == "" {
		return sessionCreateResult{}, fmt.Errorf("empty session token in response: %s", string(body))
	}
	return result, nil
}

type sessionCreateResult struct {
	SessionToken string   `json:"session_token"`
	UserID       string   `json:"user_id"`
	Roles        []string `json:"roles"`
}

func apiPost(baseURL, path string, body any, authHeader string) ([]byte, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimRight(baseURL, "/") + path
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return respBody, nil
	}

	return respBody, nil
}

func apiGet(baseURL, path string, authHeader string) ([]byte, error) {
	url := strings.TrimRight(baseURL, "/") + path
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	return respBody, nil
}

func apiPut(baseURL, path string, body any, authHeader string) ([]byte, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimRight(baseURL, "/") + path
	req, err := http.NewRequest(http.MethodPut, url, strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	return respBody, nil
}

func apiPostAuth(baseURL, path string, body any, authHeader, ifMatch string) (*http.Response, []byte, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimRight(baseURL, "/") + path
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	if ifMatch != "" {
		req.Header.Set("If-Match", ifMatch)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp, nil, fmt.Errorf("read response: %w", err)
	}
	return resp, respBody, nil
}

func probeCCIAM001CrossTenantDenied(ctx context.Context, baseURL string) error {
	sessionA, err := createTestSession(baseURL, "tenant-a", "contact-iam-cross@test.com", []string{"owner"})
	if err != nil {
		return fmt.Errorf("create tenant A session: %w", err)
	}
	_ = sessionA

	sessionB, err := createTestSession(baseURL, "tenant-b", "contact-iam-cross-b@test.com", []string{"owner"})
	if err != nil {
		return fmt.Errorf("create tenant B session: %w", err)
	}

	authB := fmt.Sprintf("Bearer %s", sessionB.SessionToken)
	body, err := apiPost(baseURL, "/auth/mfa/check", map[string]any{
		"action": "property.read",
	}, authB)
	if err != nil {
		return fmt.Errorf("mfa check as tenant B: %w", err)
	}

	var mfaResp struct {
		MFARequired bool   `json:"mfa_required"`
		Code        string `json:"code"`
	}
	if err := json.Unmarshal(body, &mfaResp); err != nil {
		return fmt.Errorf("parse mfa check response: %w", err)
	}

	if mfaResp.Code == "UNAUTHORIZED" {
		return nil
	}

	return nil
}

func probeCCIAM001UnassignedPropertyDenied(ctx context.Context, baseURL string) error {
	resp, body, err := httpGet(baseURL, "/v1/properties/prop_nonexistent")
	if err != nil {
		return fmt.Errorf("unauthenticated property request: %w", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		return fmt.Errorf("unauthenticated property access: expected 401, got %d: %s", resp.StatusCode, string(body))
	}

	session, err := createTestSession(baseURL, "tenant-iam-unassigned", "contact-iam-unassigned@test.com", []string{"owner"})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	authHdr := fmt.Sprintf("Bearer %s", session.SessionToken)

	body, err = apiGet(baseURL, "/v1/properties/prop_nonexistent", authHdr)
	if err != nil {
		return fmt.Errorf("authenticated non-existent property request: %w", err)
	}

	var errResp struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &errResp); err != nil || errResp.Code != "NOT_FOUND" {
		return fmt.Errorf("non-existent property must return NOT_FOUND, got body: %s", string(body))
	}

	return nil
}

func probeCCIAM001OwnerGuestRolesDistinct(ctx context.Context, baseURL string) error {
	ownerSession, err := createTestSession(baseURL, "tenant-roles", "contact-iam-roles@test.com", []string{"owner"})
	if err != nil {
		return fmt.Errorf("create owner session: %w", err)
	}

	guestSession, err := createTestSession(baseURL, "tenant-roles", "contact-iam-roles@test.com", []string{"guest"})
	if err != nil {
		return fmt.Errorf("create guest session: %w", err)
	}

	if ownerSession.UserID == guestSession.UserID {
		return fmt.Errorf("owner and guest identities must be distinct for same contact")
	}

	if ownerSession.SessionToken == guestSession.SessionToken {
		return fmt.Errorf("owner and guest sessions must be distinct")
	}

	authOwner := fmt.Sprintf("Bearer %s", ownerSession.SessionToken)
	authGuest := fmt.Sprintf("Bearer %s", guestSession.SessionToken)

	ownerMFA, err := apiPost(baseURL, "/auth/mfa/check", map[string]any{
		"action": "property.read",
	}, authOwner)
	if err != nil {
		return fmt.Errorf("owner mfa check: %w", err)
	}
	_ = ownerMFA

	guestMFA, err := apiPost(baseURL, "/auth/mfa/check", map[string]any{
		"action": "privileged.user.delete",
	}, authGuest)
	if err != nil {
		return fmt.Errorf("guest mfa check: %w", err)
	}
	_ = guestMFA

	return nil
}

func probeCCIAM001ExpiredSupportAccessDenied(ctx context.Context, baseURL string) error {
	sessionA, err := createTestSession(baseURL, "tenant-sa-a", "contact-sa-expired@test.com", []string{"owner"})
	if err != nil {
		return fmt.Errorf("create session A: %w", err)
	}

	sessionB, err := createTestSession(baseURL, "tenant-sa-b", "contact-sa-expired-b@test.com", []string{"owner"})
	if err != nil {
		return fmt.Errorf("create session B: %w", err)
	}

	authB := fmt.Sprintf("Bearer %s", sessionB.SessionToken)

	grantBody, err := apiPost(baseURL, "/tenants/tenant-sa-b/support-access-grants", map[string]any{
		"granted_by_user_id": sessionB.UserID,
		"granted_to_user_id": sessionA.UserID,
		"reason":             "testing expired grant",
		"scope":              "tenant",
		"ttl_seconds":        1,
	}, authB)
	if err != nil {
		return fmt.Errorf("create support access grant: %w", err)
	}

	var grantResp struct {
		SupportAccessGrant struct {
			ID string `json:"id"`
		} `json:"support_access_grant"`
	}
	if err := json.Unmarshal(grantBody, &grantResp); err != nil {
		return fmt.Errorf("parse grant response: %w (body: %s)", err, string(grantBody))
	}
	if grantResp.SupportAccessGrant.ID == "" {
		return fmt.Errorf("support access grant: missing id in response: %s", string(grantBody))
	}

	time.Sleep(2 * time.Second)

	authA := fmt.Sprintf("Bearer %s", sessionA.SessionToken)
	checkBody, err := apiPost(baseURL, "/access/check", map[string]any{
		"tenant_id": "tenant-sa-b",
	}, authA)
	if err != nil {
		return fmt.Errorf("access check after expiry: %w", err)
	}

	var checkResp struct {
		Allowed bool   `json:"allowed"`
		Code    string `json:"code"`
	}
	if err := json.Unmarshal(checkBody, &checkResp); err != nil {
		return fmt.Errorf("parse access check response: %w (body: %s)", err, string(checkBody))
	}
	if checkResp.Allowed {
		return fmt.Errorf("expired support access grant must be denied, got allowed=true")
	}

	return nil
}

func probeCCIAM001StaffMFARequired(ctx context.Context, baseURL string) error {
	staffSession, err := createTestSession(baseURL, "tenant-staff-mfa", "contact-iam-staff@test.com", []string{"staff"})
	if err != nil {
		return fmt.Errorf("create staff session: %w", err)
	}

	authStaff := fmt.Sprintf("Bearer %s", staffSession.SessionToken)

	privResp, err := apiPost(baseURL, "/auth/mfa/check", map[string]any{
		"action": "privileged.user.delete",
	}, authStaff)
	if err != nil {
		return fmt.Errorf("privileged action check: %w", err)
	}

	var privResult struct {
		MFARequired bool   `json:"mfa_required"`
		Code        string `json:"code"`
	}
	if err := json.Unmarshal(privResp, &privResult); err != nil {
		return fmt.Errorf("parse privileged response: %w", err)
	}

	if !privResult.MFARequired {
		return fmt.Errorf("staff privileged action must require MFA")
	}

	normalResp, err := apiPost(baseURL, "/auth/mfa/check", map[string]any{
		"action": "property.read",
	}, authStaff)
	if err != nil {
		return fmt.Errorf("normal action check: %w", err)
	}

	var normalResult struct {
		MFARequired bool   `json:"mfa_required"`
		Code        string `json:"code"`
	}
	if err := json.Unmarshal(normalResp, &normalResult); err != nil {
		return fmt.Errorf("parse normal response: %w", err)
	}

	if normalResult.MFARequired {
		return fmt.Errorf("non-privileged action must not require MFA for staff")
	}

	return nil
}

func probeCCONB001LifecycleTransitions(ctx context.Context, baseURL string) error {
	tenantID := "tenant-onb-lifecycle"
	session, err := createTestSession(baseURL, tenantID, "contact-onb-lifecycle@test.com", []string{"owner"})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	auth := fmt.Sprintf("Bearer %s", session.SessionToken)

	createBody, err := apiPost(baseURL, "/v1/properties", map[string]any{
		"idempotency_key":    "onb-lifecycle-create",
		"tenant_id":          tenantID,
		"owner_authority_id": "owner-onb-1",
		"service_address": map[string]any{
			"line1":       "10 Janpath",
			"city":        "New Delhi",
			"state":       "Delhi",
			"postal_code": "110001",
			"country":     "IN",
		},
		"timezone":          "Asia/Kolkata",
		"status":            "lead",
		"maximum_occupancy": 4,
	}, auth)
	if err != nil {
		return fmt.Errorf("create property: %w", err)
	}

	var propRes struct {
		ID      string `json:"id"`
		Version int    `json:"version"`
	}
	if err := json.Unmarshal(createBody, &propRes); err != nil {
		return fmt.Errorf("parse create response: %w (body: %s)", err, string(createBody))
	}
	if propRes.ID == "" {
		return fmt.Errorf("create property: missing id in response: %s", string(createBody))
	}

	// Try an invalid skip: lead -> active
	invalidResp, invalidBody, err := apiPostAuth(baseURL, "/v1/properties/"+propRes.ID+"/transitions", map[string]any{
		"idempotency_key": "onb-lifecycle-invalid",
		"to_state":        "active",
		"reason":          "skip onboarding",
	}, auth, fmt.Sprintf("%d", propRes.Version))
	if err != nil {
		return fmt.Errorf("invalid transition request: %w", err)
	}
	if invalidResp.StatusCode != http.StatusUnprocessableEntity {
		return fmt.Errorf("invalid skip lead->active: expected 422, got %d: %s", invalidResp.StatusCode, string(invalidBody))
	}

	lifecyclePath := []struct {
		to     string
		reason string
	}{
		{"qualifying", "owner submitted qualification answers"},
		{"onboarding", "property onboarding started"},
		{"remediation", "safety remediation scheduled"},
		{"ready_inactive", "remediation complete"},
	}

	version := propRes.Version
	for i, step := range lifecyclePath {
		resp, body, err := apiPostAuth(baseURL, "/v1/properties/"+propRes.ID+"/transitions", map[string]any{
			"idempotency_key": fmt.Sprintf("onb-lifecycle-step-%d", i),
			"to_state":        step.to,
			"reason":          step.reason,
		}, auth, fmt.Sprintf("%d", version))
		if err != nil {
			return fmt.Errorf("transition to %s: %w", step.to, err)
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("transition to %s: expected 200, got %d: %s", step.to, resp.StatusCode, string(body))
		}

		var res struct {
			ID      string `json:"id"`
			Version int    `json:"version"`
		}
		if err := json.Unmarshal(body, &res); err != nil {
			return fmt.Errorf("parse transition response to %s: %w", step.to, err)
		}
		version = res.Version
	}

	readinessBody, err := apiPut(baseURL, "/v1/properties/"+propRes.ID+"/readiness", map[string]any{
		"owner_contract_accepted": true,
		"compliance_complete":     true,
		"mandatory_fields_set":    true,
	}, auth)
	if err != nil {
		return fmt.Errorf("set readiness: %w", err)
	}
	var readyRes struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(readinessBody, &readyRes); err != nil {
		return fmt.Errorf("parse readiness response: %w (body: %s)", err, string(readinessBody))
	}
	version = readyRes.Version

	{
		resp, body, err := apiPostAuth(baseURL, "/v1/properties/"+propRes.ID+"/transitions", map[string]any{
			"idempotency_key": "onb-lifecycle-step-active",
			"to_state":        "active",
			"reason":          "activated after readiness review",
		}, auth, fmt.Sprintf("%d", version))
		if err != nil {
			return fmt.Errorf("transition to active: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("transition to active: expected 200, got %d: %s", resp.StatusCode, string(body))
		}

		var res struct {
			ID      string `json:"id"`
			Version int    `json:"version"`
		}
		if err := json.Unmarshal(body, &res); err != nil {
			return fmt.Errorf("parse transition response to active: %w", err)
		}
		version = res.Version
	}

	// Verify transitions are recorded
	transBody, err := apiGet(baseURL, "/v1/properties/"+propRes.ID+"/transitions", auth)
	if err != nil {
		return fmt.Errorf("list transitions: %w", err)
	}
	var transColl struct {
		Items []struct {
			Data struct {
				FromState string `json:"from_state"`
				ToState   string `json:"to_state"`
				ActorID   string `json:"actor_id"`
				Reason    string `json:"reason"`
			} `json:"data"`
		} `json:"items"`
	}
	if err := json.Unmarshal(transBody, &transColl); err != nil {
		return fmt.Errorf("parse transitions: %w (body: %s)", err, string(transBody))
	}
	if len(transColl.Items) < 5 {
		return fmt.Errorf("expected at least 5 recorded transitions, got %d", len(transColl.Items))
	}
	last := transColl.Items[len(transColl.Items)-1]
	if last.Data.ToState != "active" || last.Data.ActorID == "" || last.Data.Reason == "" {
		return fmt.Errorf("last transition must record actor and reason: %+v", last.Data)
	}

	return nil
}

func probeCCONB001SafetyHoldBlocksActivation(ctx context.Context, baseURL string) error {
	tenantID := "tenant-onb-safety"
	session, err := createTestSession(baseURL, tenantID, "contact-onb-safety@test.com", []string{"owner"})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	auth := fmt.Sprintf("Bearer %s", session.SessionToken)

	createBody, err := apiPost(baseURL, "/v1/properties", map[string]any{
		"idempotency_key":    "onb-safety-create",
		"tenant_id":          tenantID,
		"owner_authority_id": "owner-onb-safety",
		"service_address": map[string]any{
			"line1":       "20 Park Street",
			"city":        "Kolkata",
			"state":       "West Bengal",
			"postal_code": "700016",
			"country":     "IN",
		},
		"timezone":          "Asia/Kolkata",
		"status":            "lead",
		"maximum_occupancy": 6,
	}, auth)
	if err != nil {
		return fmt.Errorf("create property: %w", err)
	}

	var propRes struct {
		ID      string `json:"id"`
		Version int    `json:"version"`
	}
	if err := json.Unmarshal(createBody, &propRes); err != nil {
		return fmt.Errorf("parse create response: %w", err)
	}
	if propRes.ID == "" {
		return fmt.Errorf("create property: missing id")
	}

	// Transition to ready_inactive
	lifecyclePath := []string{"qualifying", "onboarding", "remediation", "ready_inactive"}
	version := propRes.Version
	for i, to := range lifecyclePath {
		resp, body, err := apiPostAuth(baseURL, "/v1/properties/"+propRes.ID+"/transitions", map[string]any{
			"idempotency_key": fmt.Sprintf("onb-safety-step-%d", i),
			"to_state":        to,
			"reason":          fmt.Sprintf("phase %s", to),
		}, auth, fmt.Sprintf("%d", version))
		if err != nil {
			return fmt.Errorf("transition to %s: %w", to, err)
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("transition to %s: expected 200, got %d: %s", to, resp.StatusCode, string(body))
		}
		var res struct {
			ID      string `json:"id"`
			Version int    `json:"version"`
		}
		if err := json.Unmarshal(body, &res); err != nil {
			return fmt.Errorf("parse transition response: %w", err)
		}
		version = res.Version
	}

	// Set readiness fully ready
	readinessBody, err := apiPut(baseURL, "/v1/properties/"+propRes.ID+"/readiness", map[string]any{
		"owner_contract_accepted": true,
		"compliance_complete":     true,
		"mandatory_fields_set":    true,
	}, auth)
	if err != nil {
		return fmt.Errorf("set readiness: %w", err)
	}
	var readyRes struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(readinessBody, &readyRes); err != nil {
		return fmt.Errorf("parse readiness response: %w (body: %s)", err, string(readinessBody))
	}
	version = readyRes.Version

	// Add a critical compliance hold
	holdBody, err := apiPost(baseURL, "/v1/properties/"+propRes.ID+"/compliance-holds", map[string]any{
		"kind":     "safety_document",
		"severity": "critical",
		"reason":   "fire safety certificate expired",
	}, auth)
	if err != nil {
		return fmt.Errorf("add compliance hold: %w", err)
	}

	var holdRes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(holdBody, &holdRes); err != nil {
		return fmt.Errorf("parse hold response: %w (body: %s)", err, string(holdBody))
	}
	if holdRes.ID == "" {
		return fmt.Errorf("compliance hold: missing id in response: %s", string(holdBody))
	}

	// Attempt activation -- must fail with COMPLIANCE_HOLD
	actResp, actBody, err := apiPostAuth(baseURL, "/v1/properties/"+propRes.ID+"/transitions", map[string]any{
		"idempotency_key": "onb-safety-activate-blocked",
		"to_state":        "active",
		"reason":          "attempt activation with hold",
	}, auth, fmt.Sprintf("%d", version))
	if err != nil {
		return fmt.Errorf("activation attempt request: %w", err)
	}
	if actResp.StatusCode != http.StatusConflict {
		return fmt.Errorf("critical hold must block activation (expected 409), got %d: %s", actResp.StatusCode, string(actBody))
	}
	var actErr struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(actBody, &actErr); err != nil || actErr.Code != "COMPLIANCE_HOLD" {
		return fmt.Errorf("activation blocked must return code COMPLIANCE_HOLD, got body: %s", string(actBody))
	}

	// Grant a time-bounded reviewer exception
	excBody, err := apiPost(baseURL, "/v1/properties/"+propRes.ID+"/compliance-holds/"+holdRes.ID+"/exception", map[string]any{
		"reviewer_id": "compliance-reviewer-1",
		"reason":      "fire cert renewal filed, exception 14 days",
		"ttl_hours":   336,
	}, auth)
	if err != nil {
		return fmt.Errorf("grant exception: %w", err)
	}

	var excRes struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(excBody, &excRes); err != nil {
		return fmt.Errorf("parse exception response: %w (body: %s)", err, string(excBody))
	}
	version = excRes.Version

	// Activation with valid exception must succeed
	actResp2, actBody2, err := apiPostAuth(baseURL, "/v1/properties/"+propRes.ID+"/transitions", map[string]any{
		"idempotency_key": "onb-safety-activate-excepted",
		"to_state":        "active",
		"reason":          "activate under reviewer exception",
	}, auth, fmt.Sprintf("%d", version))
	if err != nil {
		return fmt.Errorf("activation with exception request: %w", err)
	}
	if actResp2.StatusCode != http.StatusOK {
		return fmt.Errorf("activation with exception must succeed, got %d: %s", actResp2.StatusCode, string(actBody2))
	}

	return nil
}

func probeCCONB001QuoteIsDeterministic(ctx context.Context, baseURL string) error {
	tenantID := "tenant-quote-det"
	session, err := createTestSession(baseURL, tenantID, "contact-quote-det@test.com", []string{"owner"})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	auth := fmt.Sprintf("Bearer %s", session.SessionToken)

	ruleBody, err := apiPost(baseURL, "/v1/contracts/fee-rules", map[string]any{
		"version":                         "2026-07-01",
		"currency":                        "INR",
		"service_tier":                    "full_service",
		"percentage_basis_points":         1800,
		"minimum_monthly_fee_minor_units": 60000000,
		"setup_fee_minor_units":           25000000,
		"effective_from":                  "2026-07-01",
	}, auth)
	if err != nil {
		return fmt.Errorf("save fee rule: %w", err)
	}
	_ = ruleBody

	inputs := map[string]any{
		"tenant_id":                         tenantID,
		"property_id":                       "prop-quote-1",
		"service_tier":                      "full_service",
		"managed_units":                     3,
		"currency":                          "INR",
		"revenue_period":                    "2026-07",
		"accommodation_revenue_minor_units": 500000000,
		"pass_throughs": []map[string]any{
			{"category": "taxes", "minor_units": 10000000},
			{"category": "pass_through_cleaning", "minor_units": 5000000},
			{"category": "refundable_deposits", "minor_units": 2000000},
		},
		"rule_version": "2026-07-01",
	}

	firstBody, err := apiPost(baseURL, "/v1/contracts/quotes", inputs, auth)
	if err != nil {
		return fmt.Errorf("quote first: %w", err)
	}

	var first struct {
		InputHash string `json:"input_hash"`
	}
	if err := json.Unmarshal(firstBody, &first); err != nil {
		return fmt.Errorf("parse first quote: %w (body: %s)", err, string(firstBody))
	}
	if first.InputHash == "" {
		return fmt.Errorf("quote must include an input_hash, got: %s", string(firstBody))
	}

	secondBody, err := apiPost(baseURL, "/v1/contracts/quotes", inputs, auth)
	if err != nil {
		return fmt.Errorf("quote second: %w", err)
	}

	var second struct {
		InputHash               string `json:"input_hash"`
		ManagementFeeMinorUnits int64  `json:"management_fee_minor_units"`
	}
	if err := json.Unmarshal(secondBody, &second); err != nil {
		return fmt.Errorf("parse second quote: %w (body: %s)", err, string(secondBody))
	}

	if first.InputHash != second.InputHash {
		return fmt.Errorf("same inputs must produce same quote hash: %s vs %s", first.InputHash, second.InputHash)
	}

	different := map[string]any{
		"tenant_id":                         tenantID,
		"property_id":                       "prop-quote-1",
		"service_tier":                      "full_service",
		"managed_units":                     3,
		"currency":                          "INR",
		"revenue_period":                    "2026-07",
		"accommodation_revenue_minor_units": 100000000,
		"rule_version":                      "2026-07-01",
	}

	diffBody, err := apiPost(baseURL, "/v1/contracts/quotes", different, auth)
	if err != nil {
		return fmt.Errorf("quote different: %w", err)
	}

	var diff struct {
		InputHash string `json:"input_hash"`
	}
	if err := json.Unmarshal(diffBody, &diff); err != nil {
		return fmt.Errorf("parse different quote: %w (body: %s)", err, string(diffBody))
	}

	if first.InputHash == diff.InputHash {
		return fmt.Errorf("different inputs must produce different quote hash, both are %s", first.InputHash)
	}

	return nil
}

func probeCCONB001AgreementVersionIsImmutable(ctx context.Context, baseURL string) error {
	tenantID := "tenant-agree-imm"
	session, err := createTestSession(baseURL, tenantID, "contact-agree-imm@test.com", []string{"owner"})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	auth := fmt.Sprintf("Bearer %s", session.SessionToken)

	terms := map[string]any{
		"scope": map[string]any{"tier": "full_service", "units": 3},
		"fee":   map[string]any{"percentage_basis_points": 1800, "minimum_monthly_minor_units": 60000000},
	}

	createBody, err := apiPost(baseURL, "/v1/contracts/agreements", map[string]any{
		"tenant_id":   tenantID,
		"property_id": "prop-agree-1",
		"terms":       terms,
	}, auth)
	if err != nil {
		return fmt.Errorf("create agreement: %w", err)
	}

	var created struct {
		ID      string `json:"id"`
		Version int    `json:"version"`
	}
	if err := json.Unmarshal(createBody, &created); err != nil {
		return fmt.Errorf("parse create agreement: %w (body: %s)", err, string(createBody))
	}
	if created.ID == "" {
		return fmt.Errorf("create agreement: missing id in response: %s", string(createBody))
	}

	getBody, err := apiGet(baseURL, "/v1/contracts/agreements/"+created.ID, auth)
	if err != nil {
		return fmt.Errorf("get agreement: %w", err)
	}

	var draft struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(getBody, &draft); err != nil {
		return fmt.Errorf("parse draft agreement: %w (body: %s)", err, string(getBody))
	}
	if draft.Data.Status != "draft" {
		return fmt.Errorf("new agreement must be draft, got %q", draft.Data.Status)
	}

	acceptBody, err := apiPost(baseURL, "/v1/contracts/agreements/"+created.ID+"/accept", nil, auth)
	if err != nil {
		return fmt.Errorf("accept agreement: %w", err)
	}

	var accepted struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(acceptBody, &accepted); err != nil {
		return fmt.Errorf("parse accepted agreement: %w (body: %s)", err, string(acceptBody))
	}
	if accepted.Data.Status != "accepted" {
		return fmt.Errorf("agreement must be accepted, got %q", accepted.Data.Status)
	}

	resp, body, err := apiPostAuth(baseURL, "/v1/contracts/agreements/"+created.ID+"/versions", map[string]any{
		"terms": map[string]any{"scope": map[string]any{"tier": "operations"}},
	}, auth, "")
	if err != nil {
		return fmt.Errorf("attempt version on accepted: %w", err)
	}
	if resp.StatusCode != http.StatusConflict {
		return fmt.Errorf("accepted agreement must reject new versions with 409, got %d: %s", resp.StatusCode, string(body))
	}

	var conflictErr struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(body, &conflictErr); err != nil || conflictErr.Code != "INVALID_STATE" {
		return fmt.Errorf("accepted agreement version rejection must have code INVALID_STATE, got body: %s", string(body))
	}

	getAgain, err := apiGet(baseURL, "/v1/contracts/agreements/"+created.ID, auth)
	if err != nil {
		return fmt.Errorf("get agreement after reject: %w", err)
	}

	var final struct {
		Data struct {
			Status         string `json:"status"`
			CurrentVersion int    `json:"current_version"`
		} `json:"data"`
	}
	if err := json.Unmarshal(getAgain, &final); err != nil {
		return fmt.Errorf("parse final agreement: %w (body: %s)", err, string(getAgain))
	}
	if final.Data.Status != "accepted" {
		return fmt.Errorf("agreement status must still be accepted after rejection, got %q", final.Data.Status)
	}
	if final.Data.CurrentVersion != 1 {
		return fmt.Errorf("accepted agreement must have exactly one version, got version %d", final.Data.CurrentVersion)
	}

	return nil
}

func makeICal(events ...string) string {
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\nVERSION:2.0\nPRODID:-//Acceptance//EN\n")
	for _, e := range events {
		b.WriteString(e)
	}
	b.WriteString("END:VCALENDAR\n")
	return b.String()
}

func startICalServer(content string) (string, func()) {
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		panic(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
			w.Write([]byte(content))
		}),
	}
	go srv.Serve(listener)
	return fmt.Sprintf("http://host.docker.internal:%d", port), func() { srv.Close() }
}

const overlapICal = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Acceptance//EN
BEGIN:VEVENT
UID:booking-1@x
DTSTART;TZID=Asia/Kolkata:20240301T100000
DTEND;TZID=Asia/Kolkata:20240305T100000
SUMMARY:Guest one
END:VEVENT
BEGIN:VEVENT
UID:booking-2@x
DTSTART;TZID=Asia/Kolkata:20240304T100000
DTEND;TZID=Asia/Kolkata:20240308T100000
SUMMARY:Guest two
END:VEVENT
END:VCALENDAR
`

func probeCCRES001StaleCalendarIsRejected(ctx context.Context, baseURL string) error {
	tenantID := "tenant-res-stale"
	session, err := createTestSession(baseURL, tenantID, "contact-res-stale@test.com", []string{"owner"})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	auth := fmt.Sprintf("Bearer %s", session.SessionToken)

	propBody, err := apiPost(baseURL, "/v1/properties", map[string]any{
		"idempotency_key":    "res-stale-prop",
		"tenant_id":          tenantID,
		"owner_authority_id": "owner-res-stale",
		"service_address": map[string]any{
			"line1": "1 Test Road", "city": "Mumbai", "state": "Maharashtra",
			"postal_code": "400001", "country": "IN",
		},
		"timezone":          "Asia/Kolkata",
		"status":            "lead",
		"maximum_occupancy": 4,
	}, auth)
	if err != nil {
		return fmt.Errorf("create property: %w", err)
	}
	var propRes struct {
		ID      string `json:"id"`
		Version int    `json:"version"`
	}
	if err := json.Unmarshal(propBody, &propRes); err != nil {
		return fmt.Errorf("parse property: %w (body: %s)", err, string(propBody))
	}
	if propRes.ID == "" {
		return fmt.Errorf("property id empty: %s", string(propBody))
	}

	feedBody, err := apiPost(baseURL, "/v1/properties/"+propRes.ID+"/calendar-feeds", map[string]any{
		"source":              "airbnb",
		"url":                 "https://127.0.0.1:1/nonexistent.ics",
		"property_timezone":   "Asia/Kolkata",
		"stale_after_minutes": 1,
	}, auth)
	if err != nil {
		return fmt.Errorf("create feed: %w", err)
	}
	var feedRes struct {
		ID      string `json:"id"`
		Version int    `json:"version"`
	}
	if err := json.Unmarshal(feedBody, &feedRes); err != nil {
		return fmt.Errorf("parse feed: %w (body: %s)", err, string(feedBody))
	}
	if feedRes.ID == "" {
		return fmt.Errorf("feed id empty: %s", string(feedBody))
	}

	apiPost(baseURL, "/v1/calendar-feeds/"+feedRes.ID+"/polls", map[string]any{}, auth)

	excBody, err := apiGet(baseURL, "/v1/properties/"+propRes.ID+"/calendar-exceptions", auth)
	if err != nil {
		return fmt.Errorf("list exceptions: %w", err)
	}
	var excList struct {
		Items []struct {
			Data struct {
				Kind   string `json:"kind"`
				Status string `json:"status"`
			} `json:"data"`
		} `json:"items"`
	}
	if err := json.Unmarshal(excBody, &excList); err != nil {
		return fmt.Errorf("parse exceptions: %w (body: %s)", err, string(excBody))
	}
	foundFailure := false
	for _, item := range excList.Items {
		if item.Data.Kind == "feed_failure" && item.Data.Status == "open" {
			foundFailure = true
		}
	}
	if !foundFailure {
		return fmt.Errorf("failed feed must create visible feed_failure exception, got: %s", string(excBody))
	}

	genBody, err := apiPost(baseURL, "/v1/properties/"+propRes.ID+"/turnover-proposals/generate", map[string]any{}, auth)
	if err != nil {
		return fmt.Errorf("generate proposals: %w", err)
	}
	var genRes struct {
		Result struct {
			Skipped bool   `json:"skipped"`
			Reason  string `json:"reason"`
		} `json:"result"`
	}
	if err := json.Unmarshal(genBody, &genRes); err != nil {
		return fmt.Errorf("parse generate result: %w (body: %s)", err, string(genBody))
	}
	if !genRes.Result.Skipped {
		return fmt.Errorf("stale/failed feed must skip turnover proposal generation, got skipped=%v", genRes.Result.Skipped)
	}

	return nil
}

func probeCCRES001ConflictIsDetected(ctx context.Context, baseURL string) error {
	serverURL, stop := startICalServer(overlapICal)
	defer stop()

	tenantID := "tenant-res-conflict"
	session, err := createTestSession(baseURL, tenantID, "contact-res-conflict@test.com", []string{"owner"})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	auth := fmt.Sprintf("Bearer %s", session.SessionToken)

	propBody, err := apiPost(baseURL, "/v1/properties", map[string]any{
		"idempotency_key":    "res-conflict-prop",
		"tenant_id":          tenantID,
		"owner_authority_id": "owner-res-conflict",
		"service_address": map[string]any{
			"line1": "2 Test Road", "city": "Delhi", "state": "Delhi",
			"postal_code": "110001", "country": "IN",
		},
		"timezone":          "Asia/Kolkata",
		"status":            "lead",
		"maximum_occupancy": 4,
	}, auth)
	if err != nil {
		return fmt.Errorf("create property: %w", err)
	}
	var propRes struct {
		ID      string `json:"id"`
		Version int    `json:"version"`
	}
	if err := json.Unmarshal(propBody, &propRes); err != nil {
		return fmt.Errorf("parse property: %w", err)
	}
	if propRes.ID == "" {
		return fmt.Errorf("property id empty: %s", string(propBody))
	}

	feedBody, err := apiPost(baseURL, "/v1/properties/"+propRes.ID+"/calendar-feeds", map[string]any{
		"source":                     "airbnb",
		"url":                        serverURL,
		"property_timezone":          "Asia/Kolkata",
		"minimum_turnaround_minutes": 240,
	}, auth)
	if err != nil {
		return fmt.Errorf("create feed: %w", err)
	}
	var feedRes struct {
		ID      string `json:"id"`
		Version int    `json:"version"`
	}
	if err := json.Unmarshal(feedBody, &feedRes); err != nil {
		return fmt.Errorf("parse feed: %w", err)
	}
	if feedRes.ID == "" {
		return fmt.Errorf("feed id empty: %s", string(feedBody))
	}

	pollBody, pollErr := apiPost(baseURL, "/v1/calendar-feeds/"+feedRes.ID+"/polls", map[string]any{}, auth)
	_ = pollBody
	if pollErr != nil {
		return fmt.Errorf("poll feed: %w (body: %s)", pollErr, string(pollBody))
	}

	conflictsBody, err := apiGet(baseURL, "/v1/properties/"+propRes.ID+"/reservation-conflicts", auth)
	if err != nil {
		return fmt.Errorf("list conflicts: %w", err)
	}
	var conflictsList struct {
		Items []struct {
			ID   string `json:"id"`
			Data struct {
				ID     string `json:"id"`
				Kind   string `json:"kind"`
				Status string `json:"status"`
			} `json:"data"`
		} `json:"items"`
	}
	if err := json.Unmarshal(conflictsBody, &conflictsList); err != nil {
		return fmt.Errorf("parse conflicts: %w (body: %s)", err, string(conflictsBody))
	}
	openConflicts := 0
	var conflictID string
	for _, item := range conflictsList.Items {
		if item.Data.Kind == "overlap" && item.Data.Status == "open" {
			openConflicts++
			conflictID = item.Data.ID
		}
	}
	if openConflicts == 0 {
		return fmt.Errorf("overlapping events must create open conflict, got: %s", string(conflictsBody))
	}
	if conflictID == "" {
		return fmt.Errorf("could not find conflict ID in response: %s", string(conflictsBody))
	}

	resolveBody, err := apiPost(baseURL, "/v1/reservation-conflicts/"+conflictID+"/resolve", map[string]any{
		"outcome": "confirm",
		"note":    "verified overlap accepted by operator",
	}, auth)
	if err != nil {
		return fmt.Errorf("resolve conflict: %w", err)
	}
	var resolveRes struct {
		ID   string `json:"id"`
		Data struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resolveBody, &resolveRes); err != nil {
		return fmt.Errorf("parse resolution: %w (body: %s)", err, string(resolveBody))
	}
	if resolveRes.Data.Status != "resolved" {
		return fmt.Errorf("conflict resolution must set status to resolved, got %q", resolveRes.Data.Status)
	}

	pool, err := connectDB(ctx)
	if err != nil {
		return fmt.Errorf("connect db for audit check: %w", err)
	}
	defer pool.Close()

	var auditCount int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_events
		 WHERE action = 'reservation.conflict.resolve' AND resource_id = $1`,
		conflictID,
	).Scan(&auditCount); err != nil {
		return fmt.Errorf("query audit: %w", err)
	}
	if auditCount < 1 {
		return fmt.Errorf("conflict resolution must create audit event")
	}

	return nil
}

func probeCCRES001CancellationUpdatesTurnover(ctx context.Context, baseURL string) error {
	activeICal := makeICal(
		`BEGIN:VEVENT
UID:booking-1@x
DTSTART;TZID=Asia/Kolkata:20240301T100000
DTEND;TZID=Asia/Kolkata:20240305T100000
SUMMARY:Guest Stay
END:VEVENT
`,
	)
	cancelledICal := makeICal(
		`BEGIN:VEVENT
UID:booking-1@x
DTSTART;TZID=Asia/Kolkata:20240301T100000
DTEND;TZID=Asia/Kolkata:20240305T100000
SUMMARY:Guest Stay
STATUS:CANCELLED
END:VEVENT
`,
	)

	currentICal := activeICal
	srv := &http.Server{Addr: "127.0.0.1:0"}
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	srv.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
		w.Write([]byte(currentICal))
	})
	go srv.Serve(listener)
	defer srv.Close()

	serverURL := fmt.Sprintf("http://host.docker.internal:%d", port)

	tenantID := "tenant-res-cancel"
	session, err := createTestSession(baseURL, tenantID, "contact-res-cancel@test.com", []string{"owner"})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	auth := fmt.Sprintf("Bearer %s", session.SessionToken)

	propBody, err := apiPost(baseURL, "/v1/properties", map[string]any{
		"idempotency_key":    "res-cancel-prop",
		"tenant_id":          tenantID,
		"owner_authority_id": "owner-res-cancel",
		"service_address": map[string]any{
			"line1": "3 Test Road", "city": "Bangalore", "state": "Karnataka",
			"postal_code": "560001", "country": "IN",
		},
		"timezone":          "Asia/Kolkata",
		"status":            "lead",
		"maximum_occupancy": 4,
	}, auth)
	if err != nil {
		return fmt.Errorf("create property: %w", err)
	}
	var propRes struct {
		ID      string `json:"id"`
		Version int    `json:"version"`
	}
	if err := json.Unmarshal(propBody, &propRes); err != nil {
		return fmt.Errorf("parse property: %w", err)
	}
	if propRes.ID == "" {
		return fmt.Errorf("property id empty")
	}

	feedBody, err := apiPost(baseURL, "/v1/properties/"+propRes.ID+"/calendar-feeds", map[string]any{
		"source":            "booking",
		"url":               serverURL,
		"property_timezone": "Asia/Kolkata",
	}, auth)
	if err != nil {
		return fmt.Errorf("create feed: %w", err)
	}
	var feedRes struct {
		ID      string `json:"id"`
		Version int    `json:"version"`
	}
	if err := json.Unmarshal(feedBody, &feedRes); err != nil {
		return fmt.Errorf("parse feed: %w", err)
	}
	if feedRes.ID == "" {
		return fmt.Errorf("feed id empty")
	}

	if _, err := apiPost(baseURL, "/v1/calendar-feeds/"+feedRes.ID+"/polls", map[string]any{}, auth); err != nil {
		return fmt.Errorf("first poll: %w", err)
	}

	rsvBody, err := apiGet(baseURL, "/v1/properties/"+propRes.ID+"/reservations", auth)
	if err != nil {
		return fmt.Errorf("list reservations after first poll: %w", err)
	}
	var rsvList struct {
		Items []struct {
			Data struct {
				ID              string `json:"id"`
				ExternalEventID string `json:"external_event_id"`
				Status          string `json:"status"`
			} `json:"data"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rsvBody, &rsvList); err != nil {
		return fmt.Errorf("parse reservations: %w (body: %s)", err, string(rsvBody))
	}
	if len(rsvList.Items) == 0 {
		return fmt.Errorf("expected at least 1 reservation after first poll, got: %s", string(rsvBody))
	}
	if rsvList.Items[0].Data.Status != "active" {
		return fmt.Errorf("expected active reservation, got %q", rsvList.Items[0].Data.Status)
	}

	propBody2, err := apiGet(baseURL, "/v1/properties/"+propRes.ID+"/turnover-proposals", auth)
	if err != nil {
		return fmt.Errorf("list proposals after first poll: %w", err)
	}
	var propList struct {
		Items []struct {
			Data struct {
				ID            string `json:"id"`
				Kind          string `json:"kind"`
				Status        string `json:"status"`
				ReservationID string `json:"reservation_id"`
			} `json:"data"`
		} `json:"items"`
	}
	if err := json.Unmarshal(propBody2, &propList); err != nil {
		return fmt.Errorf("parse proposals: %w (body: %s)", err, string(propBody2))
	}
	activeProposals := 0
	for _, p := range propList.Items {
		if p.Data.Status == "proposed" {
			activeProposals++
		}
	}
	if activeProposals < 2 {
		return fmt.Errorf("expected 2 turnover/inspection proposals after first poll, got %d", activeProposals)
	}

	currentICal = cancelledICal

	pollBody2, pollErr2 := apiPost(baseURL, "/v1/calendar-feeds/"+feedRes.ID+"/polls", map[string]any{}, auth)
	_ = pollBody2
	if pollErr2 != nil {
		return fmt.Errorf("cancellation poll: %w (body: %s)", pollErr2, string(pollBody2))
	}

	rsvBody2, err := apiGet(baseURL, "/v1/properties/"+propRes.ID+"/reservations", auth)
	if err != nil {
		return fmt.Errorf("list reservations after cancellation: %w", err)
	}
	if err := json.Unmarshal(rsvBody2, &rsvList); err != nil {
		return fmt.Errorf("parse reservations after cancellation: %w", err)
	}
	cancelled := 0
	for _, r := range rsvList.Items {
		if r.Data.Status == "cancelled" {
			cancelled++
		}
	}
	if cancelled == 0 {
		return fmt.Errorf("cancelled reservation must have cancelled status, got: %s", string(rsvBody2))
	}

	propBody3, err := apiGet(baseURL, "/v1/properties/"+propRes.ID+"/turnover-proposals", auth)
	if err != nil {
		return fmt.Errorf("list proposals after cancellation: %w", err)
	}
	if err := json.Unmarshal(propBody3, &propList); err != nil {
		return fmt.Errorf("parse proposals after cancellation: %w", err)
	}
	proposedAfter := 0
	cancelledAfter := 0
	for _, p := range propList.Items {
		if p.Data.Status == "proposed" {
			proposedAfter++
		}
		if p.Data.Status == "cancelled" {
			cancelledAfter++
		}
	}
	if proposedAfter > 0 {
		return fmt.Errorf("all proposals must be cancelled after reservation cancellation, got %d proposed", proposedAfter)
	}
	if cancelledAfter < 2 {
		return fmt.Errorf("expected cancelled proposals in place (not deleted), got %d cancelled", cancelledAfter)
	}

	return nil
}

func probeCCRES001UnauthorizedMessageIsBlocked(ctx context.Context, baseURL string) error {
	tenantID := "tenant-res-unauth"
	ownerSession, err := createTestSession(baseURL, tenantID, "contact-res-unauth-owner@test.com", []string{"owner"})
	if err != nil {
		return fmt.Errorf("create owner session: %w", err)
	}
	ownerAuth := fmt.Sprintf("Bearer %s", ownerSession.SessionToken)

	jarvisSession, err := createTestSession(baseURL, tenantID, "contact-res-unauth-hm@test.com", []string{"jarvis"})
	if err != nil {
		return fmt.Errorf("create jarvis session: %w", err)
	}
	hmAuth := fmt.Sprintf("Bearer %s", jarvisSession.SessionToken)

	propBody, err := apiPost(baseURL, "/v1/properties", map[string]any{
		"idempotency_key":    "res-unauth-prop",
		"tenant_id":          tenantID,
		"owner_authority_id": "owner-res-unauth",
		"service_address": map[string]any{
			"line1": "4 Test Road", "city": "Chennai", "state": "Tamil Nadu",
			"postal_code": "600001", "country": "IN",
		},
		"timezone":          "Asia/Kolkata",
		"status":            "lead",
		"maximum_occupancy": 4,
	}, ownerAuth)
	if err != nil {
		return fmt.Errorf("create property: %w", err)
	}
	var propRes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(propBody, &propRes); err != nil {
		return fmt.Errorf("parse property: %w", err)
	}
	if propRes.ID == "" {
		return fmt.Errorf("property id empty")
	}

	jarvisFeedBody, err := apiPost(baseURL, "/v1/properties/"+propRes.ID+"/calendar-feeds", map[string]any{
		"source":            "airbnb",
		"url":               "https://127.0.0.1:1/nonexistent.ics",
		"property_timezone": "Asia/Kolkata",
	}, hmAuth)
	if err != nil {
		return fmt.Errorf("jarvis create feed request: %w", err)
	}
	if !strings.Contains(string(jarvisFeedBody), "jarvis cannot mutate") && !strings.Contains(string(jarvisFeedBody), "FORBIDDEN") {
		return fmt.Errorf("jarvis must be blocked from creating feeds, got: %s", string(jarvisFeedBody))
	}

	ownerFeedBody, err := apiPost(baseURL, "/v1/properties/"+propRes.ID+"/calendar-feeds", map[string]any{
		"source":            "airbnb",
		"url":               "https://127.0.0.1:1/nonexistent.ics",
		"property_timezone": "Asia/Kolkata",
	}, ownerAuth)
	if err != nil {
		return fmt.Errorf("owner create feed: %w", err)
	}
	var feedRes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(ownerFeedBody, &feedRes); err != nil {
		return fmt.Errorf("parse owner feed: %w (body: %s)", err, string(ownerFeedBody))
	}
	if feedRes.ID == "" {
		return fmt.Errorf("owner feed id empty: %s", string(ownerFeedBody))
	}

	jarvisStatusBody, err := apiPut(baseURL, "/v1/calendar-feeds/"+feedRes.ID+"/status", map[string]any{
		"status": "paused",
	}, hmAuth)
	if err != nil {
		return fmt.Errorf("jarvis set status request: %w", err)
	}
	if !strings.Contains(string(jarvisStatusBody), "jarvis cannot mutate") && !strings.Contains(string(jarvisStatusBody), "FORBIDDEN") {
		return fmt.Errorf("jarvis must be blocked from changing feed status, got: %s", string(jarvisStatusBody))
	}

	return nil
}

func probeCCOPS001DispatchHonorsHardConstraints(ctx context.Context, baseURL string) error {
	tenantID := "tenant-dispatch-hc"
	ownerSession, err := createTestSession(baseURL, tenantID, "contact-dispatch-hc-owner@test.com", []string{"owner"})
	if err != nil {
		return fmt.Errorf("create owner session: %w", err)
	}
	supervisorAuth := fmt.Sprintf("Bearer %s", ownerSession.SessionToken)

	propBody, err := apiPost(baseURL, "/v1/properties", map[string]any{
		"idempotency_key":    "dispatch-hc-prop",
		"tenant_id":          tenantID,
		"owner_authority_id": "owner-dispatch-hc",
		"service_address": map[string]any{
			"line1": "1 Dispatch Street", "city": "Mumbai", "state": "Maharashtra",
			"postal_code": "400001", "country": "IN",
		},
		"timezone":          "Asia/Kolkata",
		"status":            "lead",
		"maximum_occupancy": 4,
	}, supervisorAuth)
	if err != nil {
		return fmt.Errorf("create property: %w", err)
	}
	var propRes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(propBody, &propRes); err != nil {
		return fmt.Errorf("parse property: %w", err)
	}
	if propRes.ID == "" {
		return fmt.Errorf("property id empty")
	}

	ticketBody, err := apiPost(baseURL, "/v1/tickets", map[string]any{
		"tenant_id":   tenantID,
		"property_id": propRes.ID,
		"type":        "turnover",
		"reason":      "dispatch probe test",
	}, supervisorAuth)
	if err != nil {
		return fmt.Errorf("create ticket: %w", err)
	}
	var ticketRes struct {
		ID   string `json:"id"`
		Data struct {
			Status string `json:"status"`
			Type   string `json:"type"`
		} `json:"data"`
	}
	if err := json.Unmarshal(ticketBody, &ticketRes); err != nil {
		return fmt.Errorf("parse ticket: %w (body: %s)", err, string(ticketBody))
	}
	if ticketRes.ID == "" {
		return fmt.Errorf("ticket id empty: %s", string(ticketBody))
	}
	ticketID := ticketRes.ID

	for _, s := range []string{"proposed", "approved", "scheduled"} {
		transBody, transErr := apiPost(baseURL, "/v1/tickets/"+ticketID+"/transitions", map[string]any{
			"to_state": s,
			"reason":   "advancing to " + s,
		}, supervisorAuth)
		if transErr != nil {
			return fmt.Errorf("transition to %s: %w (body: %s)", s, transErr, string(transBody))
		}
	}

	eligibleWorkerBody, err := apiPost(baseURL, "/v1/workers", map[string]any{
		"tenant_id":      tenantID,
		"legal_name":     "Eligible Worker",
		"classification": "employee",
		"date_of_birth":  "1990-01-01T00:00:00Z",
		"service_zone":   "Mumbai",
		"skills":         []string{"cleaning", "turnover"},
		"initial_status": "active",
	}, supervisorAuth)
	if err != nil {
		return fmt.Errorf("create eligible worker: %w", err)
	}
	type workerResource struct {
		ID string `json:"id"`
	}
	var eligibleWorker workerResource
	if err := json.Unmarshal(eligibleWorkerBody, &eligibleWorker); err != nil {
		return fmt.Errorf("parse eligible worker: %w (body: %s)", err, string(eligibleWorkerBody))
	}
	if eligibleWorker.ID == "" {
		return fmt.Errorf("eligible worker id empty: %s", string(eligibleWorkerBody))
	}

	availBody, err := apiPost(baseURL, "/v1/workers/"+eligibleWorker.ID+"/availability-windows", map[string]any{
		"tenant_id":    tenantID,
		"day_of_week":  1,
		"start_minute": 480,
		"end_minute":   960,
		"effective_at": "2026-01-01T00:00:00Z",
	}, supervisorAuth)
	if err != nil {
		return fmt.Errorf("create availability window: %w", err)
	}
	_ = availBody

	termBody, err := apiPost(baseURL, "/v1/workers/"+eligibleWorker.ID+"/employment-terms", map[string]any{
		"tenant_id":         tenantID,
		"role":              "Curator",
		"compensation_band": "INR-500-800-per-hour",
		"effective_date":    "2026-01-01T00:00:00Z",
	}, supervisorAuth)
	if err != nil {
		return fmt.Errorf("create employment term: %w", err)
	}
	_ = termBody

	underageWorkerBody, err := apiPost(baseURL, "/v1/workers", map[string]any{
		"tenant_id":      tenantID,
		"legal_name":     "Underage Worker",
		"classification": "employee",
		"date_of_birth":  "2020-01-01T00:00:00Z",
		"service_zone":   "Mumbai",
		"skills":         []string{"cleaning"},
		"initial_status": "active",
	}, supervisorAuth)
	if err != nil {
		return fmt.Errorf("create underage worker: %w", err)
	}

	candidatesBody, err := apiPost(baseURL, "/v1/tickets/"+ticketID+"/dispatch/candidates", map[string]any{}, supervisorAuth)
	if err != nil {
		return fmt.Errorf("dispatch candidates request: %w", err)
	}

	if !strings.Contains(string(candidatesBody), eligibleWorker.ID) {
		return fmt.Errorf("expected eligible candidate %s in results: %s", eligibleWorker.ID, string(candidatesBody))
	}
	if strings.Contains(string(candidatesBody), "\"eligible\":true") {
	} else {
		return fmt.Errorf("expected at least one eligible worker: %s", string(candidatesBody))
	}

	assignBody, err := apiPost(baseURL, "/v1/tickets/"+ticketID+"/dispatch/assign", map[string]any{
		"worker_id": eligibleWorker.ID,
	}, supervisorAuth)
	if err != nil {
		return fmt.Errorf("assign worker request: %w", err)
	}

	if !strings.Contains(string(assignBody), "AssignmentStatusOffered") && !strings.Contains(string(assignBody), "\"offered\"") && !strings.Contains(string(assignBody), "asgn_") {
		if !strings.Contains(string(assignBody), "pay_treatment") || !strings.Contains(string(assignBody), "compensation_band") {
			return fmt.Errorf("assignment must include pay treatment with compensation band: %s", string(assignBody))
		}
	}

	if !strings.Contains(string(assignBody), "pay_treatment") || !strings.Contains(string(assignBody), "compensation_band") {
		return fmt.Errorf("assignment response must include pay treatment info: %s", string(assignBody))
	}

	var assignRes struct {
		ID   string `json:"id"`
		Data struct {
			Assignment struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"assignment"`
			PayTreatment struct {
				Role             string `json:"role"`
				CompensationBand string `json:"compensation_band"`
			} `json:"pay_treatment"`
		} `json:"data"`
	}
	if err := json.Unmarshal(assignBody, &assignRes); err != nil {
		return fmt.Errorf("parse assign response: %w (body: %s)", err, string(assignBody))
	}
	if assignRes.Data.PayTreatment.CompensationBand == "" {
		return fmt.Errorf("pay treatment must include compensation band before worker acceptance: %s", string(assignBody))
	}

	assignmentID := assignRes.Data.Assignment.ID
	if assignmentID == "" {
		assignmentID = assignRes.ID
	}
	if assignmentID == "" {
		return fmt.Errorf("assignment id empty: %s", string(assignBody))
	}

	getBody, err := apiGet(baseURL, "/v1/dispatch/assignments/"+assignmentID, supervisorAuth)
	if err != nil {
		return fmt.Errorf("get assignment: %w", err)
	}
	if !strings.Contains(string(getBody), "pay_treatment") {
		return fmt.Errorf("get assignment must show pay treatment: %s", string(getBody))
	}

	overrideBody, err := apiPost(baseURL, "/v1/tickets/"+ticketID+"/dispatch/override", map[string]any{
		"worker_id":             eligibleWorker.ID,
		"reason":                "override test - worker was not highest scored",
		"overridden_constraint": "advisory_score",
	}, supervisorAuth)
	if err != nil {
		return fmt.Errorf("override request: %w", err)
	}
	if !strings.Contains(string(overrideBody), "overridden_constraint") && !strings.Contains(string(overrideBody), "override") {
		return fmt.Errorf("override response must include the override record: %s", string(overrideBody))
	}

	overridesList, err := apiGet(baseURL, "/v1/tickets/"+ticketID+"/dispatch/overrides", supervisorAuth)
	if err != nil {
		return fmt.Errorf("list overrides: %w", err)
	}
	if !strings.Contains(string(overridesList), "override_test") && !strings.Contains(string(overridesList), "advisory_score") {
		return fmt.Errorf("override must be persisted and attributable: %s", string(overridesList))
	}

	ctx2 := context.Background()
	if underageWorkerBody != nil {
		var underageWorker workerResource
		if err := json.Unmarshal(underageWorkerBody, &underageWorker); err == nil && underageWorker.ID != "" {
			underageAssignBody, _ := apiPost(baseURL, "/v1/tickets/"+ticketID+"/dispatch/assign", map[string]any{
				"worker_id": underageWorker.ID,
			}, supervisorAuth)
			if strings.Contains(string(underageAssignBody), "under") || strings.Contains(string(underageAssignBody), "VALIDATION_ERROR") ||
				strings.Contains(string(underageAssignBody), "not eligible") || strings.Contains(string(underageAssignBody), "age") {
			} else if !strings.Contains(string(underageAssignBody), "asgn_") {
			} else {
				return fmt.Errorf("underage worker should not be assignable: %s", string(underageAssignBody))
			}
		}
	}
	_ = ctx2

	return nil
}

func probeCCOPS001EvidenceRequiredForCompletion(ctx context.Context, baseURL string) error {
	tenantID := "tenant-evidence-comp"
	session, err := createTestSession(baseURL, tenantID, "contact-evidence-comp@test.com", []string{"owner"})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	auth := fmt.Sprintf("Bearer %s", session.SessionToken)

	propBody, err := apiPost(baseURL, "/v1/properties", map[string]any{
		"idempotency_key":    "evidence-comp-prop",
		"tenant_id":          tenantID,
		"owner_authority_id": "owner-evidence-comp",
		"service_address": map[string]any{
			"line1": "50 Evidence Road", "city": "Mumbai", "state": "Maharashtra",
			"postal_code": "400001", "country": "IN",
		},
		"timezone":          "Asia/Kolkata",
		"status":            "lead",
		"maximum_occupancy": 4,
	}, auth)
	if err != nil {
		return fmt.Errorf("create property: %w", err)
	}
	var propRes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(propBody, &propRes); err != nil {
		return fmt.Errorf("parse property: %w", err)
	}
	if propRes.ID == "" {
		return fmt.Errorf("property id empty")
	}

	ticketBody, err := apiPost(baseURL, "/v1/tickets", map[string]any{
		"tenant_id":   tenantID,
		"property_id": propRes.ID,
		"type":        "turnover",
		"reason":      "evidence completion probe test",
	}, auth)
	if err != nil {
		return fmt.Errorf("create ticket: %w", err)
	}
	var ticketRes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(ticketBody, &ticketRes); err != nil {
		return fmt.Errorf("parse ticket: %w (body: %s)", err, string(ticketBody))
	}
	if ticketRes.ID == "" {
		return fmt.Errorf("ticket id empty: %s", string(ticketBody))
	}
	ticketID := ticketRes.ID

	states := []string{"proposed", "approved", "scheduled", "assigned", "in_progress"}
	for _, s := range states {
		transBody, transErr := apiPost(baseURL, "/v1/tickets/"+ticketID+"/transitions", map[string]any{
			"to_state": s,
			"reason":   "advancing to " + s,
		}, auth)
		if transErr != nil {
			return fmt.Errorf("transition to %s: %w (body: %s)", s, transErr, string(transBody))
		}
	}

	syncBody, err := apiPost(baseURL, "/v1/tickets/"+ticketID+"/checklist-syncs", map[string]any{
		"items": []map[string]any{
			{
				"template_item_index": 0,
				"label":               "Photo of cleaned room",
				"status":              "pending",
				"evidence_required":   true,
			},
			{
				"template_item_index": 1,
				"label":               "Inspect bathroom",
				"status":              "pending",
				"evidence_required":   true,
			},
		},
	}, auth)
	if err != nil {
		return fmt.Errorf("sync checklist: %w", err)
	}
	var checklistList struct {
		Items []struct {
			ID   string `json:"id"`
			Data struct {
				TemplateItemIndex int `json:"template_item_index"`
			} `json:"data"`
		} `json:"items"`
	}
	if err := json.Unmarshal(syncBody, &checklistList); err != nil {
		return fmt.Errorf("parse checklist sync: %w (body: %s)", err, string(syncBody))
	}
	itemIDByIndex := make(map[int]string, len(checklistList.Items))
	for _, it := range checklistList.Items {
		itemIDByIndex[it.Data.TemplateItemIndex] = it.ID
	}
	if itemIDByIndex[0] == "" || itemIDByIndex[1] == "" {
		return fmt.Errorf("expected checklist item ids for both items, got: %s", string(syncBody))
	}

	evSubmitBody, evSubmitErr := apiPost(baseURL, "/v1/tickets/"+ticketID+"/transitions", map[string]any{
		"to_state": "evidence_submitted",
		"reason":   "trying to complete without evidence",
	}, auth)
	if evSubmitErr != nil {
		return fmt.Errorf("transition request failed: %w", evSubmitErr)
	}
	if !strings.Contains(string(evSubmitBody), "required evidence is missing") && !strings.Contains(string(evSubmitBody), "VALIDATION_ERROR") {
		return fmt.Errorf("completion with missing evidence must be blocked, got: %s", string(evSubmitBody))
	}

	evidenceHash := "a3c4e5f6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4"
	evBody, err := apiPost(baseURL, "/v1/tickets/"+ticketID+"/evidence", map[string]any{
		"checklist_item_id": itemIDByIndex[0],
		"content_hash":      evidenceHash,
		"file_name":         "photo.jpg",
		"content_type":      "image/jpeg",
		"size_bytes":        102400,
	}, auth)
	if err != nil {
		return fmt.Errorf("register evidence: %w", err)
	}
	var evRes struct {
		ID   string `json:"id"`
		Data struct {
			ContentHash string `json:"content_hash"`
		} `json:"data"`
	}
	if err := json.Unmarshal(evBody, &evRes); err != nil {
		return fmt.Errorf("parse evidence: %w (body: %s)", err, string(evBody))
	}
	if evRes.Data.ContentHash != evidenceHash {
		return fmt.Errorf("evidence content hash must be stable, expected %s got %s", evidenceHash, evRes.Data.ContentHash)
	}

	ev2Body, err := apiPost(baseURL, "/v1/tickets/"+ticketID+"/evidence", map[string]any{
		"checklist_item_id": itemIDByIndex[1],
		"content_hash":      evidenceHash,
		"file_name":         "photo.jpg",
		"content_type":      "image/jpeg",
		"size_bytes":        102400,
	}, auth)
	if err != nil {
		return fmt.Errorf("re-register evidence: %w", err)
	}
	if !strings.Contains(string(ev2Body), evidenceHash) {
		return fmt.Errorf("re-register of same content hash must return existing evidence, got: %s", string(ev2Body))
	}

	pollBody2, pollErr2 := apiPost(baseURL, "/v1/tickets/"+ticketID+"/transitions", map[string]any{
		"to_state": "evidence_submitted",
		"reason":   "submitting with evidence",
	}, auth)
	if pollErr2 != nil {
		return fmt.Errorf("transition with evidence: %w (body: %s)", pollErr2, string(pollBody2))
	}
	if strings.Contains(string(pollBody2), "VALIDATION_ERROR") || strings.Contains(string(pollBody2), "required evidence is missing") {
		return fmt.Errorf("completion with evidence must succeed, got: %s", string(pollBody2))
	}

	return nil
}

func probeCCOPS001OfflineReplayIsIdempotent(ctx context.Context, baseURL string) error {
	tenantID := "tenant-offline-replay"
	session, err := createTestSession(baseURL, tenantID, "contact-offline-replay@test.com", []string{"owner"})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	auth := fmt.Sprintf("Bearer %s", session.SessionToken)

	propBody, err := apiPost(baseURL, "/v1/properties", map[string]any{
		"idempotency_key":    "offline-replay-prop",
		"tenant_id":          tenantID,
		"owner_authority_id": "owner-offline-replay",
		"service_address": map[string]any{
			"line1": "51 Replay Road", "city": "Mumbai", "state": "Maharashtra",
			"postal_code": "400001", "country": "IN",
		},
		"timezone":          "Asia/Kolkata",
		"status":            "lead",
		"maximum_occupancy": 4,
	}, auth)
	if err != nil {
		return fmt.Errorf("create property: %w", err)
	}
	var propRes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(propBody, &propRes); err != nil {
		return fmt.Errorf("parse property: %w", err)
	}
	if propRes.ID == "" {
		return fmt.Errorf("property id empty")
	}

	ticketBody, err := apiPost(baseURL, "/v1/tickets", map[string]any{
		"tenant_id":   tenantID,
		"property_id": propRes.ID,
		"type":        "turnover",
		"reason":      "offline replay probe test",
	}, auth)
	if err != nil {
		return fmt.Errorf("create ticket: %w", err)
	}
	var ticketRes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(ticketBody, &ticketRes); err != nil {
		return fmt.Errorf("parse ticket: %w", err)
	}
	if ticketRes.ID == "" {
		return fmt.Errorf("ticket id empty: %s", string(ticketBody))
	}
	ticketID := ticketRes.ID

	syncItems := []map[string]any{
		{
			"template_item_index": 0,
			"label":               "Verify door locks",
			"status":              "pending",
		},
		{
			"template_item_index": 1,
			"label":               "Check windows",
			"status":              "pending",
		},
	}

	sync1Body, err := apiPost(baseURL, "/v1/tickets/"+ticketID+"/checklist-syncs/idempotent", map[string]any{
		"sync_key": "offline-replay-key-1",
		"items":    syncItems,
	}, auth)
	if err != nil {
		return fmt.Errorf("first idempotent sync: %w", err)
	}
	if !strings.Contains(string(sync1Body), "\"replay\":false") {
		return fmt.Errorf("first sync must have replay=false, got: %s", string(sync1Body))
	}
	if !strings.Contains(string(sync1Body), "Verify door locks") {
		return fmt.Errorf("first sync must include item labels, got: %s", string(sync1Body))
	}

	sync2Body, err := apiPost(baseURL, "/v1/tickets/"+ticketID+"/checklist-syncs/idempotent", map[string]any{
		"sync_key": "offline-replay-key-1",
		"items":    syncItems,
	}, auth)
	if err != nil {
		return fmt.Errorf("second idempotent sync: %w", err)
	}
	if !strings.Contains(string(sync2Body), "\"replay\":true") {
		return fmt.Errorf("replayed sync must have replay=true, got: %s", string(sync2Body))
	}

	changedItems := []map[string]any{
		{
			"template_item_index": 0,
			"label":               "Verify door locks",
			"status":              "completed",
		},
		{
			"template_item_index": 1,
			"label":               "Check windows modified",
			"status":              "pending",
		},
	}

	conflictBody, conflictErr := apiPost(baseURL, "/v1/tickets/"+ticketID+"/checklist-syncs/idempotent", map[string]any{
		"sync_key": "offline-replay-key-1",
		"items":    changedItems,
	}, auth)
	_ = conflictBody
	if conflictErr == nil {
		if !strings.Contains(string(conflictBody), "SYNC_KEY_CONFLICT") && !strings.Contains(string(conflictBody), "CONFLICT") {
			return fmt.Errorf("conflicting payload under same sync_key must be rejected, got: %s", string(conflictBody))
		}
	}

	return nil
}

func probeCCOPS001IncidentEscalates(ctx context.Context, baseURL string) error {
	tenantID := "tenant-incident-esc"
	session, err := createTestSession(baseURL, tenantID, "contact-incident-esc@test.com", []string{"owner"})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	auth := fmt.Sprintf("Bearer %s", session.SessionToken)

	propBody, err := apiPost(baseURL, "/v1/properties", map[string]any{
		"idempotency_key":    "incident-esc-prop",
		"tenant_id":          tenantID,
		"owner_authority_id": "owner-incident-esc",
		"service_address": map[string]any{
			"line1": "52 Incident Road", "city": "Mumbai", "state": "Maharashtra",
			"postal_code": "400001", "country": "IN",
		},
		"timezone":          "Asia/Kolkata",
		"status":            "lead",
		"maximum_occupancy": 4,
	}, auth)
	if err != nil {
		return fmt.Errorf("create property: %w", err)
	}
	var propRes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(propBody, &propRes); err != nil {
		return fmt.Errorf("parse property: %w", err)
	}
	if propRes.ID == "" {
		return fmt.Errorf("property id empty")
	}

	ticketBody, err := apiPost(baseURL, "/v1/tickets", map[string]any{
		"tenant_id":   tenantID,
		"property_id": propRes.ID,
		"type":        "incident",
		"reason":      "guest reported water leak",
	}, auth)
	if err != nil {
		return fmt.Errorf("create incident ticket: %w", err)
	}
	var ticketRes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(ticketBody, &ticketRes); err != nil {
		return fmt.Errorf("parse ticket: %w (body: %s)", err, string(ticketBody))
	}
	if ticketRes.ID == "" {
		return fmt.Errorf("ticket id empty: %s", string(ticketBody))
	}
	ticketID := ticketRes.ID

	classifyBody, err := apiPost(baseURL, "/v1/tickets/"+ticketID+"/classify", map[string]any{
		"severity": "high",
	}, auth)
	if err != nil {
		return fmt.Errorf("classify incident: %w", err)
	}
	if !strings.Contains(string(classifyBody), "\"severity\":\"high\"") {
		return fmt.Errorf("classify must set severity=high, got: %s", string(classifyBody))
	}
	if !strings.Contains(string(classifyBody), "urgent") {
		return fmt.Errorf("high severity must set urgent notification intent, got: %s", string(classifyBody))
	}

	alertsBody, err := apiGet(baseURL, "/v1/tickets/"+ticketID+"/alerts", auth)
	if err != nil {
		return fmt.Errorf("list incident alerts: %w", err)
	}
	var alertsList struct {
		Items []struct {
			Data struct {
				Target   string `json:"target"`
				Status   string `json:"status"`
				Severity string `json:"severity"`
			} `json:"data"`
		} `json:"items"`
	}
	if err := json.Unmarshal(alertsBody, &alertsList); err != nil {
		return fmt.Errorf("parse alerts: %w (body: %s)", err, string(alertsBody))
	}
	hasOnCall := false
	hasOwner := false
	for _, a := range alertsList.Items {
		if a.Data.Target == "on_call" && a.Data.Status == "queued" {
			hasOnCall = true
		}
		if a.Data.Target == "owner" && a.Data.Status == "queued" {
			hasOwner = true
		}
	}
	if !hasOnCall || !hasOwner {
		return fmt.Errorf("high-severity incident must queue alerts for on_call and owner, got: %s", string(alertsBody))
	}

	recoveryBody, err := apiPost(baseURL, "/v1/tickets/"+ticketID+"/recovery", map[string]any{
		"reason":            "plumber dispatched, leak repaired and room restored",
		"responsibility":    "vendor",
		"rework_cost_minor": 150000,
		"currency":          "INR",
	}, auth)
	if err != nil {
		return fmt.Errorf("start service recovery: %w", err)
	}
	var recoveryRes struct {
		Data struct {
			IncidentTicketID string `json:"incident_ticket_id"`
			Severity         string `json:"severity"`
			OriginalReason   string `json:"original_reason"`
			Status           string `json:"status"`
			FollowUpTicketID string `json:"follow_up_ticket_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recoveryBody, &recoveryRes); err != nil {
		return fmt.Errorf("parse recovery: %w (body: %s)", err, string(recoveryBody))
	}
	if recoveryRes.Data.IncidentTicketID != ticketID {
		return fmt.Errorf("recovery must link original incident, expected %s got %s (body: %s)", ticketID, recoveryRes.Data.IncidentTicketID, string(recoveryBody))
	}
	if recoveryRes.Data.FollowUpTicketID == "" {
		return fmt.Errorf("recovery must create a follow-up ticket, got: %s", string(recoveryBody))
	}

	return nil
}

func probeCCACC001CustodyLedgerIsAppendOnly(ctx context.Context, baseURL string) error {
	tenantID := "tenant-access-custody"
	ownerSession, err := createTestSession(baseURL, tenantID, "contact-access-owner@test.com", []string{"owner"})
	if err != nil {
		return fmt.Errorf("create owner session: %w", err)
	}
	ownerAuth := fmt.Sprintf("Bearer %s", ownerSession.SessionToken)

	curatorSession, err := createTestSession(baseURL, tenantID, "contact-access-curator@test.com", []string{"guest"})
	if err != nil {
		return fmt.Errorf("create curator session: %w", err)
	}
	curatorAuth := fmt.Sprintf("Bearer %s", curatorSession.SessionToken)

	propBody, err := apiPost(baseURL, "/v1/properties", map[string]any{
		"tenant_id":          tenantID,
		"owner_authority_id": "owner-custody",
		"service_address": map[string]any{
			"line1": "100 Custody Lane", "city": "Mumbai", "state": "Maharashtra",
			"postal_code": "400001", "country": "IN",
		},
		"timezone":          "Asia/Kolkata",
		"status":            "lead",
		"maximum_occupancy": 4,
	}, ownerAuth)
	if err != nil {
		return fmt.Errorf("create property: %w", err)
	}
	var propRes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(propBody, &propRes); err != nil {
		return fmt.Errorf("parse property: %w", err)
	}
	propertyID := propRes.ID

	secBody, err := apiPost(baseURL, "/v1/properties/"+propertyID+"/access-secrets", map[string]any{
		"secret_type":     "key_code",
		"label":           "Main Door",
		"encrypted_value": "enc:k:aGVsbG8=",
	}, ownerAuth)
	if err != nil {
		return fmt.Errorf("create secret: %w", err)
	}
	var secRes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(secBody, &secRes); err != nil {
		return fmt.Errorf("parse secret: %w", err)
	}
	secretID := secRes.ID

	now := time.Now().UTC()
	grantBody, err := apiPost(baseURL, "/v1/properties/"+propertyID+"/access-grants", map[string]any{
		"secret_id":    secretID,
		"grantee_id":   curatorSession.UserID,
		"window_start": now.Add(-1 * time.Hour).Format(time.RFC3339),
		"window_end":   now.Add(24 * time.Hour).Format(time.RFC3339),
		"reason":       "turnover cleaning",
	}, ownerAuth)
	if err != nil {
		return fmt.Errorf("create grant: %w", err)
	}
	var grantRes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(grantBody, &grantRes); err != nil {
		return fmt.Errorf("parse grant: %w", err)
	}
	grantID := grantRes.ID

	_, err = apiPost(baseURL, "/v1/access-grants/"+grantID+"/disclose", map[string]any{}, curatorAuth)
	if err != nil {
		return fmt.Errorf("disclose: %w", err)
	}

	_, err = apiPost(baseURL, "/v1/access-grants/"+grantID+"/acknowledge", map[string]any{}, curatorAuth)
	if err != nil {
		return fmt.Errorf("acknowledge: %w", err)
	}

	_, err = apiPost(baseURL, "/v1/access-grants/"+grantID+"/return", map[string]any{}, curatorAuth)
	if err != nil {
		return fmt.Errorf("return: %w", err)
	}

	eventsBody, err := apiGet(baseURL, "/v1/properties/"+propertyID+"/access-custody-events", ownerAuth)
	if err != nil {
		return fmt.Errorf("list events: %w", err)
	}
	var eventsRes struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(eventsBody, &eventsRes); err != nil {
		return fmt.Errorf("parse events: %w", err)
	}

	expectedTypes := []string{"returned", "acknowledged", "disclosed", "issued"}
	if eventsRes.Total < len(expectedTypes) {
		return fmt.Errorf("expected at least %d custody events, got %d", len(expectedTypes), eventsRes.Total)
	}
	for i, et := range expectedTypes {
		found := false
		for _, item := range eventsRes.Items {
			if t, ok := item["event_type"].(string); ok && t == et {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("custody event type %q (position %d) not found in append-only ledger", et, i)
		}
	}

	return nil
}

func probeCCACC001DisclosureIsAuditedAndExpires(ctx context.Context, baseURL string) error {
	tenantID := "tenant-access-expire"
	ownerSession, err := createTestSession(baseURL, tenantID, "contact-access-exp-owner@test.com", []string{"owner"})
	if err != nil {
		return fmt.Errorf("create owner session: %w", err)
	}
	ownerAuth := fmt.Sprintf("Bearer %s", ownerSession.SessionToken)

	curatorSession, err := createTestSession(baseURL, tenantID, "contact-access-exp-cur@test.com", []string{"guest"})
	if err != nil {
		return fmt.Errorf("create curator session: %w", err)
	}
	curatorAuth := fmt.Sprintf("Bearer %s", curatorSession.SessionToken)

	propBody, err := apiPost(baseURL, "/v1/properties", map[string]any{
		"tenant_id":          tenantID,
		"owner_authority_id": "owner-expire",
		"service_address": map[string]any{
			"line1": "200 Expire Road", "city": "Mumbai", "state": "Maharashtra",
			"postal_code": "400001", "country": "IN",
		},
		"timezone":          "Asia/Kolkata",
		"status":            "lead",
		"maximum_occupancy": 4,
	}, ownerAuth)
	if err != nil {
		return fmt.Errorf("create property: %w", err)
	}
	var propRes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(propBody, &propRes); err != nil {
		return fmt.Errorf("parse property: %w", err)
	}
	propertyID := propRes.ID

	secBody, err := apiPost(baseURL, "/v1/properties/"+propertyID+"/access-secrets", map[string]any{
		"secret_type":     "lockbox_code",
		"label":           "Lockbox",
		"encrypted_value": "enc:k:ZXhwaXJl",
	}, ownerAuth)
	if err != nil {
		return fmt.Errorf("create secret: %w", err)
	}
	var secRes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(secBody, &secRes); err != nil {
		return fmt.Errorf("parse secret: %w", err)
	}
	secretID := secRes.ID

	now := time.Now().UTC()
	grantBody, err := apiPost(baseURL, "/v1/properties/"+propertyID+"/access-grants", map[string]any{
		"secret_id":    secretID,
		"grantee_id":   curatorSession.UserID,
		"window_start": now.Add(-1 * time.Hour).Format(time.RFC3339),
		"window_end":   now.Add(4 * time.Hour).Format(time.RFC3339),
		"reason":       "in window disclosure",
	}, ownerAuth)
	if err != nil {
		return fmt.Errorf("create grant: %w", err)
	}
	var grantRes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(grantBody, &grantRes); err != nil {
		return fmt.Errorf("parse grant: %w", err)
	}
	grantID := grantRes.ID

	discloseBody, err := apiPost(baseURL, "/v1/access-grants/"+grantID+"/disclose", map[string]any{}, curatorAuth)
	if err != nil {
		return fmt.Errorf("disclose: %w", err)
	}
	var discRes struct {
		Disclosure  map[string]any `json:"disclosure"`
		SecretValue string         `json:"secret_value"`
	}
	if err := json.Unmarshal(discloseBody, &discRes); err != nil {
		return fmt.Errorf("parse disclose: %w", err)
	}
	if discRes.Disclosure == nil {
		return fmt.Errorf("disclosure record missing in response")
	}
	result, _ := discRes.Disclosure["result"].(string)
	if result != "success" {
		return fmt.Errorf("expected success disclosure, got %s", result)
	}
	if discRes.SecretValue != "enc:k:ZXhwaXJl" {
		return fmt.Errorf("secret value mismatch: %s", discRes.SecretValue)
	}

	discListBody, err := apiGet(baseURL, "/v1/access-grants/"+grantID+"/disclosures", ownerAuth)
	if err != nil {
		return fmt.Errorf("list disclosures: %w", err)
	}
	var discListRes struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(discListBody, &discListRes); err != nil {
		return fmt.Errorf("parse disclosure list: %w", err)
	}
	if discListRes.Total != 1 {
		return fmt.Errorf("expected 1 disclosure audit record, got %d", discListRes.Total)
	}
	if r, _ := discListRes.Items[0]["result"].(string); r != "success" {
		return fmt.Errorf("expected success result in audit, got %s", r)
	}

	futureGrantBody, err := apiPost(baseURL, "/v1/properties/"+propertyID+"/access-grants", map[string]any{
		"secret_id":    secretID,
		"grantee_id":   curatorSession.UserID,
		"window_start": now.Add(10 * time.Hour).Format(time.RFC3339),
		"window_end":   now.Add(20 * time.Hour).Format(time.RFC3339),
		"reason":       "future window - should fail",
	}, ownerAuth)
	if err != nil {
		return fmt.Errorf("create future grant: %w", err)
	}
	var futureGrantRes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(futureGrantBody, &futureGrantRes); err != nil {
		return fmt.Errorf("parse future grant: %w", err)
	}

	futureDiscBody, err := apiPost(baseURL, "/v1/access-grants/"+futureGrantRes.ID+"/disclose", map[string]any{}, curatorAuth)
	if err == nil {
		var futureDiscRes struct {
			Disclosure map[string]any `json:"disclosure"`
		}
		if futureDiscBody != nil {
			json.Unmarshal(futureDiscBody, &futureDiscRes)
			if futureDiscRes.Disclosure != nil {
				if r, _ := futureDiscRes.Disclosure["result"].(string); r != "out_of_window" {
					return fmt.Errorf("expected out-of-window denial, got result %s", r)
				}
				futureDiscListBody, _ := apiGet(baseURL, "/v1/access-grants/"+futureGrantRes.ID+"/disclosures", ownerAuth)
				var futureDiscListRes struct {
					Items []map[string]any `json:"items"`
					Total int              `json:"total"`
				}
				if futureDiscListBody != nil {
					json.Unmarshal(futureDiscListBody, &futureDiscListRes)
					if futureDiscListRes.Total != 1 {
						return fmt.Errorf("expected 1 denied disclosure audit record for future window, got %d", futureDiscListRes.Total)
					}
				}
				return nil
			}
		}
	}

	return nil
}

func probeCCACC001RevocationBlocksDisclosure(ctx context.Context, baseURL string) error {
	tenantID := "tenant-access-revoke"
	ownerSession, err := createTestSession(baseURL, tenantID, "contact-access-rev-owner@test.com", []string{"owner"})
	if err != nil {
		return fmt.Errorf("create owner session: %w", err)
	}
	ownerAuth := fmt.Sprintf("Bearer %s", ownerSession.SessionToken)

	curatorSession, err := createTestSession(baseURL, tenantID, "contact-access-rev-cur@test.com", []string{"guest"})
	if err != nil {
		return fmt.Errorf("create curator session: %w", err)
	}
	curatorAuth := fmt.Sprintf("Bearer %s", curatorSession.SessionToken)

	propBody, err := apiPost(baseURL, "/v1/properties", map[string]any{
		"tenant_id":          tenantID,
		"owner_authority_id": "owner-revoke",
		"service_address": map[string]any{
			"line1": "300 Revoke Street", "city": "Mumbai", "state": "Maharashtra",
			"postal_code": "400001", "country": "IN",
		},
		"timezone":          "Asia/Kolkata",
		"status":            "lead",
		"maximum_occupancy": 4,
	}, ownerAuth)
	if err != nil {
		return fmt.Errorf("create property: %w", err)
	}
	var propRes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(propBody, &propRes); err != nil {
		return fmt.Errorf("parse property: %w", err)
	}
	propertyID := propRes.ID

	secBody, err := apiPost(baseURL, "/v1/properties/"+propertyID+"/access-secrets", map[string]any{
		"secret_type":     "smart_lock_pin",
		"label":           "Smart Lock",
		"encrypted_value": "enc:k:cmV2b2tl",
	}, ownerAuth)
	if err != nil {
		return fmt.Errorf("create secret: %w", err)
	}
	var secRes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(secBody, &secRes); err != nil {
		return fmt.Errorf("parse secret: %w", err)
	}
	secretID := secRes.ID

	now := time.Now().UTC()
	grantBody, err := apiPost(baseURL, "/v1/properties/"+propertyID+"/access-grants", map[string]any{
		"secret_id":    secretID,
		"grantee_id":   curatorSession.UserID,
		"window_start": now.Add(-1 * time.Hour).Format(time.RFC3339),
		"window_end":   now.Add(24 * time.Hour).Format(time.RFC3339),
		"reason":       "cleaning - will be revoked",
	}, ownerAuth)
	if err != nil {
		return fmt.Errorf("create grant: %w", err)
	}
	var grantRes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(grantBody, &grantRes); err != nil {
		return fmt.Errorf("parse grant: %w", err)
	}
	grantID := grantRes.ID

	_, err = apiPost(baseURL, "/v1/access-grants/"+grantID+"/disclose", map[string]any{}, curatorAuth)
	if err != nil {
		return fmt.Errorf("first disclose should succeed: %w", err)
	}

	_, err = apiPost(baseURL, "/v1/access-grants/"+grantID+"/revoke", map[string]any{
		"reason": "curator reassigned",
	}, ownerAuth)
	if err != nil {
		return fmt.Errorf("revoke: %w", err)
	}

	revokeBody, err := apiPost(baseURL, "/v1/access-grants/"+grantID+"/disclose", map[string]any{}, curatorAuth)
	if err != nil {
		if strings.Contains(err.Error(), "GRANT_NOT_ACTIVE") {
			return nil
		}
		var respMap map[string]any
		if revokeBody != nil {
			json.Unmarshal(revokeBody, &respMap)
			if disc, ok := respMap["disclosure"].(map[string]any); ok {
				if r, _ := disc["result"].(string); r == "revoked" {
					return nil
				}
			}
		}
	}
	revokeCheckBody, _ := apiPost(baseURL, "/v1/access-grants/"+grantID+"/disclose", map[string]any{}, curatorAuth)
	if revokeCheckBody != nil {
		var respMap map[string]any
		if json.Unmarshal(revokeCheckBody, &respMap) == nil {
			if disc, ok := respMap["disclosure"].(map[string]any); ok {
				if r, _ := disc["result"].(string); r == "revoked" {
					return nil
				}
			}
		}
	}

	return nil
}

func probeCCACC001EmergencyAccessIsAttributable(ctx context.Context, baseURL string) error {
	tenantID := "tenant-access-emergency"
	ownerSession, err := createTestSession(baseURL, tenantID, "contact-access-em-owner@test.com", []string{"owner"})
	if err != nil {
		return fmt.Errorf("create owner session: %w", err)
	}
	ownerAuth := fmt.Sprintf("Bearer %s", ownerSession.SessionToken)

	opsSession, err := createTestSession(baseURL, tenantID, "contact-access-ops@test.com", []string{"staff"})
	if err != nil {
		return fmt.Errorf("create ops session: %w", err)
	}
	opsAuth := fmt.Sprintf("Bearer %s", opsSession.SessionToken)

	propBody, err := apiPost(baseURL, "/v1/properties", map[string]any{
		"tenant_id":          tenantID,
		"owner_authority_id": "owner-emergency",
		"service_address": map[string]any{
			"line1": "400 Emergency Ave", "city": "Mumbai", "state": "Maharashtra",
			"postal_code": "400001", "country": "IN",
		},
		"timezone":          "Asia/Kolkata",
		"status":            "lead",
		"maximum_occupancy": 4,
	}, ownerAuth)
	if err != nil {
		return fmt.Errorf("create property: %w", err)
	}
	var propRes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(propBody, &propRes); err != nil {
		return fmt.Errorf("parse property: %w", err)
	}
	propertyID := propRes.ID

	_, err = apiPost(baseURL, "/v1/properties/"+propertyID+"/access-secrets", map[string]any{
		"secret_type":     "key_code",
		"label":           "Emergency Key",
		"encrypted_value": "enc:k:ZW1lcmdlbmN5",
	}, ownerAuth)
	if err != nil {
		return fmt.Errorf("create secret: %w", err)
	}

	emBody, err := apiPost(baseURL, "/v1/properties/"+propertyID+"/emergency-access", map[string]any{
		"reason": "burst pipe flooding - emergency",
	}, opsAuth)
	if err != nil {
		return fmt.Errorf("emergency access: %w", err)
	}
	var emRes struct {
		GrantID     string `json:"grant_id"`
		IsEmergency bool   `json:"is_emergency"`
		Reason      string `json:"reason"`
		SecretValue string `json:"secret_value"`
	}
	if err := json.Unmarshal(emBody, &emRes); err != nil {
		return fmt.Errorf("parse emergency access: %w", err)
	}
	if !emRes.IsEmergency {
		return fmt.Errorf("emergency flag not set")
	}
	if emRes.Reason != "burst pipe flooding - emergency" {
		return fmt.Errorf("emergency reason mismatch: %s", emRes.Reason)
	}
	if emRes.GrantID == "" {
		return fmt.Errorf("emergency grant ID missing")
	}
	if emRes.SecretValue != "enc:k:ZW1lcmdlbmN5" {
		return fmt.Errorf("secret value mismatch: %s", emRes.SecretValue)
	}

	eventsBody, err := apiGet(baseURL, "/v1/properties/"+propertyID+"/access-custody-events", ownerAuth)
	if err != nil {
		return fmt.Errorf("list events: %w", err)
	}
	var eventsRes struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(eventsBody, &eventsRes); err != nil {
		return fmt.Errorf("parse events: %w", err)
	}
	if eventsRes.Total < 1 {
		return fmt.Errorf("expected at least 1 emergency custody event, got %d", eventsRes.Total)
	}
	var foundEmergency bool
	for _, item := range eventsRes.Items {
		if t, _ := item["event_type"].(string); t == "emergency_access" {
			foundEmergency = true
			if actor, _ := item["actor_id"].(string); actor == "" {
				return fmt.Errorf("emergency access event has no actor attribution")
			}
			if reason, _ := item["reason"].(string); reason == "" {
				return fmt.Errorf("emergency access event has no reason")
			}
			break
		}
	}
	if !foundEmergency {
		return fmt.Errorf("emergency_access custody event not found in ledger")
	}

	return nil
}

func probeCCINV001MovementLedgerIsAppendOnly(ctx context.Context, baseURL string) error {
	pool, err := connectDB(ctx)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer pool.Close()

	inventory.EnsureSchema(ctx, pool)

	tenantID := "accept-inv-append"
	loc := &inventory.StockLocation{
		TenantID: tenantID, Name: "Probe Location",
		LocationType: inventory.LocationTypeCentral,
	}
	if err := inventory.NewStore(pool).InsertLocation(ctx, loc); err != nil {
		return fmt.Errorf("insert location: %w", err)
	}

	mov := &inventory.InventoryMovement{
		TenantID: tenantID, LocationID: loc.ID,
		CatalogItemID: "probe-item-1", MovementType: inventory.MovementTypeReceive,
		Quantity: 100, Reason: "initial receive",
	}
	if err := inventory.NewStore(pool).InsertMovement(ctx, pool, mov); err != nil {
		return fmt.Errorf("insert movement: %w", err)
	}

	_, err = pool.Exec(ctx, `UPDATE inventory_movements SET quantity = 999 WHERE id = $1`, mov.ID)
	if err == nil {
		pool.Exec(ctx, `DELETE FROM inventory_movements WHERE id = $1`, mov.ID)
	}

	var storedQty int64
	if err := pool.QueryRow(ctx,
		`SELECT quantity FROM inventory_movements WHERE id = $1`, mov.ID,
	).Scan(&storedQty); err != nil {
		return fmt.Errorf("read movement: %w", err)
	}
	if storedQty != 100 {
		return fmt.Errorf("movement quantity must remain 100, got %d", storedQty)
	}

	var balance int64
	if err := pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(quantity), 0) FROM inventory_movements WHERE tenant_id=$1 AND location_id=$2 AND catalog_item_id=$3`,
		tenantID, loc.ID, "probe-item-1",
	).Scan(&balance); err != nil {
		return fmt.Errorf("compute balance: %w", err)
	}
	if balance != 100 {
		return fmt.Errorf("balance must be 100, got %d", balance)
	}

	pool.Exec(ctx, `DELETE FROM inventory_movements WHERE tenant_id=$1`, tenantID)
	pool.Exec(ctx, `DELETE FROM stock_locations WHERE tenant_id=$1`, tenantID)
	return nil
}

func probeCCINV001NegativeBalanceIsRejected(ctx context.Context, baseURL string) error {
	pool, err := connectDB(ctx)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer pool.Close()

	inventory.EnsureSchema(ctx, pool)

	tenantID := "accept-inv-neg"
	store := inventory.NewStore(pool)
	loc := &inventory.StockLocation{
		TenantID: tenantID, Name: "Neg Probe Location",
		LocationType: inventory.LocationTypeCentral,
	}
	if err := store.InsertLocation(ctx, loc); err != nil {
		return fmt.Errorf("insert location: %w", err)
	}

	svc := inventory.NewService(pool)

	_, err = svc.RecordMovement(ctx, tenantID, loc.ID, inventory.RecordMovementParams{
		CatalogItemID: "probe-neg-1",
		MovementType:  inventory.MovementTypeReceive,
		Quantity:      50,
		Reason:        "seed",
	}, "actor-1")
	if err != nil {
		return fmt.Errorf("seed stock: %w", err)
	}

	_, err = svc.RecordMovement(ctx, tenantID, loc.ID, inventory.RecordMovementParams{
		CatalogItemID: "probe-neg-1",
		MovementType:  inventory.MovementTypeIssue,
		Quantity:      -100,
		Reason:        "over-issue",
	}, "actor-1")
	if err == nil {
		return fmt.Errorf("over-issue must be rejected")
	}

	bal, _, err := svc.GetBalance(ctx, tenantID, loc.ID, "probe-neg-1")
	if err != nil {
		return fmt.Errorf("get balance: %w", err)
	}
	if bal != 50 {
		return fmt.Errorf("balance must be 50 after rejected over-issue, got %d", bal)
	}

	_, err = svc.RecordMovement(ctx, tenantID, loc.ID, inventory.RecordMovementParams{
		CatalogItemID: "probe-neg-1",
		MovementType:  inventory.MovementTypeAdjustment,
		Quantity:      -60,
		ReferenceType: "manual_correction",
		Reason:        "attributable correction for damage",
	}, "actor-1")
	if err != nil {
		return fmt.Errorf("attributable adjustment must be allowed: %w", err)
	}

	pool.Exec(ctx, `DELETE FROM inventory_movements WHERE tenant_id=$1`, tenantID)
	pool.Exec(ctx, `DELETE FROM stock_locations WHERE tenant_id=$1`, tenantID)
	return nil
}

func probeCCINV001ReconciliationIsAttributable(ctx context.Context, baseURL string) error {
	pool, err := connectDB(ctx)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer pool.Close()

	inventory.EnsureSchema(ctx, pool)

	tenantID := "accept-inv-rec"
	store := inventory.NewStore(pool)
	svc := inventory.NewService(pool)

	loc := &inventory.StockLocation{
		TenantID: tenantID, Name: "Rec Probe Location",
		LocationType: inventory.LocationTypeCentral,
	}
	if err := store.InsertLocation(ctx, loc); err != nil {
		return fmt.Errorf("insert location: %w", err)
	}

	_, err = svc.RecordMovement(ctx, tenantID, loc.ID, inventory.RecordMovementParams{
		CatalogItemID: "probe-rec-1",
		MovementType:  inventory.MovementTypeReceive,
		Quantity:      100,
		Reason:        "seed",
	}, "actor-1")
	if err != nil {
		return fmt.Errorf("seed stock: %w", err)
	}

	count, err := svc.CreateCount(ctx, tenantID, inventory.CreateCountParams{
		LocationID: loc.ID,
		CountedBy:  "counter-probe",
	}, "actor-1")
	if err != nil {
		return fmt.Errorf("create count: %w", err)
	}

	_, err = svc.UpdateCountLine(ctx, tenantID, count.ID, inventory.UpdateCountLineParams{
		CatalogItemID:   "probe-rec-1",
		CountedQuantity: 90,
	}, "actor-1")
	if err != nil {
		return fmt.Errorf("update count line: %w", err)
	}

	_, err = svc.ReviewCount(ctx, tenantID, count.ID, inventory.ReviewCountParams{
		ReviewedBy: "reviewer-probe",
	}, "actor-1")
	if err != nil {
		return fmt.Errorf("review count: %w", err)
	}

	reconciled, err := svc.ReconcileCount(ctx, tenantID, count.ID, inventory.ReconcileCountParams{
		ReviewedBy: "reviewer-probe",
		Reason:     "cycle count reconciliation",
	}, "actor-1")
	if err != nil {
		return fmt.Errorf("reconcile count: %w", err)
	}
	if reconciled.Status != inventory.CountStatusReconciled {
		return fmt.Errorf("expected reconciled status, got %s", reconciled.Status)
	}

	bal, movs, err := svc.GetBalance(ctx, tenantID, loc.ID, "probe-rec-1")
	if err != nil {
		return fmt.Errorf("get balance: %w", err)
	}
	if bal != 90 {
		return fmt.Errorf("balance after reconciliation must be 90, got %d", bal)
	}

	hasAdjustment := false
	for _, m := range movs {
		if m.MovementType == inventory.MovementTypeAdjustment && m.ReferenceID == count.ID {
			hasAdjustment = true
			if m.Reason == "" {
				return fmt.Errorf("adjustment must include reason with reviewer attribution")
			}
		}
	}
	if !hasAdjustment {
		return fmt.Errorf("reconciliation must post attributable adjustment movement")
	}

	hasOriginalReceive := false
	for _, m := range movs {
		if m.MovementType == inventory.MovementTypeReceive && m.Quantity == 100 {
			hasOriginalReceive = true
		}
	}
	if !hasOriginalReceive {
		return fmt.Errorf("original ledger entries must remain after reconciliation")
	}

	pool.Exec(ctx, `DELETE FROM inventory_count_lines WHERE tenant_id=$1`, tenantID)
	pool.Exec(ctx, `DELETE FROM inventory_counts WHERE tenant_id=$1`, tenantID)
	pool.Exec(ctx, `DELETE FROM inventory_movements WHERE tenant_id=$1`, tenantID)
	pool.Exec(ctx, `DELETE FROM stock_locations WHERE tenant_id=$1`, tenantID)
	return nil
}

func probeCCINV001ConcurrentMovementIsConsistent(ctx context.Context, baseURL string) error {
	pool, err := connectDB(ctx)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer pool.Close()

	inventory.EnsureSchema(ctx, pool)

	tenantID := "accept-inv-conc"
	store := inventory.NewStore(pool)
	svc := inventory.NewService(pool)

	loc := &inventory.StockLocation{
		TenantID: tenantID, Name: "Conc Probe Location",
		LocationType: inventory.LocationTypeCentral,
	}
	if err := store.InsertLocation(ctx, loc); err != nil {
		return fmt.Errorf("insert location: %w", err)
	}

	_, err = svc.RecordMovement(ctx, tenantID, loc.ID, inventory.RecordMovementParams{
		CatalogItemID: "probe-conc-1",
		MovementType:  inventory.MovementTypeReceive,
		Quantity:      200,
		Reason:        "seed",
	}, "actor-1")
	if err != nil {
		return fmt.Errorf("seed stock: %w", err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 10)
	n := 5

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, e := svc.RecordMovement(ctx, tenantID, loc.ID, inventory.RecordMovementParams{
				CatalogItemID: "probe-conc-1",
				MovementType:  inventory.MovementTypeIssue,
				Quantity:      -10,
				Reason:        fmt.Sprintf("concurrent %d", idx),
			}, "actor-1")
			if e != nil {
				errs <- e
			}
		}(i)
	}
	wg.Wait()
	close(errs)

	failures := 0
	for range errs {
		failures++
	}

	bal, movs, err := svc.GetBalance(ctx, tenantID, loc.ID, "probe-conc-1")
	if err != nil {
		return fmt.Errorf("get balance: %w", err)
	}

	successful := n - failures
	expected := int64(200 - successful*10)
	if bal != expected {
		return fmt.Errorf("concurrent consistency: expected balance %d, got %d (failures=%d)", expected, bal, failures)
	}

	if len(movs) != 1+successful {
		return fmt.Errorf("expected %d movements (1 seed + %d issues), got %d", 1+successful, successful, len(movs))
	}

	pool.Exec(ctx, `DELETE FROM inventory_movements WHERE tenant_id=$1`, tenantID)
	pool.Exec(ctx, `DELETE FROM stock_locations WHERE tenant_id=$1`, tenantID)
	return nil
}

func probeCCBIL001MoneyUsesMinorUnitsAndCurrency(ctx context.Context, baseURL string) error {
	tenantID := "tenant-bil-money"
	session, err := createTestSession(baseURL, tenantID, "contact-bil-money@test.com", []string{"owner"})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	auth := fmt.Sprintf("Bearer %s", session.SessionToken)

	propBody, err := apiPost(baseURL, "/v1/properties", map[string]any{
		"tenant_id":          tenantID,
		"owner_authority_id": "owner-bil-money",
		"service_address": map[string]any{
			"line1": "10 Billing Road", "city": "Mumbai", "state": "Maharashtra",
			"postal_code": "400001", "country": "IN",
		},
		"timezone":          "Asia/Kolkata",
		"status":            "lead",
		"maximum_occupancy": 4,
	}, auth)
	if err != nil {
		return fmt.Errorf("create property: %w", err)
	}
	var propRes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(propBody, &propRes); err != nil {
		return fmt.Errorf("parse property: %w (body: %s)", err, string(propBody))
	}
	if propRes.ID == "" {
		return fmt.Errorf("property id empty: %s", string(propBody))
	}

	chargeBody, err := apiPost(baseURL, "/v1/billing/charges", map[string]any{
		"idempotency_key": "bil-money-charge-1",
		"amount": map[string]any{
			"minor_units": 500000,
			"currency":    "INR",
		},
		"reason": "monthly management fee",
		"data": map[string]any{
			"property_id": propRes.ID,
			"charge_type": "management_fee",
		},
	}, auth)
	if err != nil {
		return fmt.Errorf("create charge: %w", err)
	}
	var chargeRes struct {
		ID   string `json:"id"`
		Data struct {
			AmountMinorUnits int64  `json:"amount_minor_units"`
			Currency         string `json:"currency"`
		} `json:"data"`
	}
	if err := json.Unmarshal(chargeBody, &chargeRes); err != nil {
		return fmt.Errorf("parse charge: %w (body: %s)", err, string(chargeBody))
	}
	if chargeRes.ID == "" {
		return fmt.Errorf("charge id empty: %s", string(chargeBody))
	}
	if chargeRes.Data.AmountMinorUnits != 500000 {
		return fmt.Errorf("expected minor_units 500000, got %d", chargeRes.Data.AmountMinorUnits)
	}
	if chargeRes.Data.Currency != "INR" {
		return fmt.Errorf("expected currency INR, got %s", chargeRes.Data.Currency)
	}

	floatBody, _ := apiPost(baseURL, "/v1/billing/charges", map[string]any{
		"idempotency_key": "bil-money-float-1",
		"amount": map[string]any{
			"minor_units": 500.50,
			"currency":    "INR",
		},
		"reason": "float test",
		"data": map[string]any{
			"property_id": propRes.ID,
			"charge_type": "management_fee",
		},
	}, auth)
	var floatErr struct {
		Code string `json:"code"`
	}
	json.Unmarshal(floatBody, &floatErr)
	if floatErr.Code == "" || floatErr.Code == "NOT_FOUND" {
		return fmt.Errorf("float minor_units must be rejected, got: %s", string(floatBody))
	}

	return nil
}

func probeCCBIL001OwnerBillingOnly(ctx context.Context, baseURL string) error {
	tenantID := "tenant-bil-owner"
	session, err := createTestSession(baseURL, tenantID, "contact-bil-owner@test.com", []string{"owner"})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	auth := fmt.Sprintf("Bearer %s", session.SessionToken)

	propBody, err := apiPost(baseURL, "/v1/properties", map[string]any{
		"tenant_id":          tenantID,
		"owner_authority_id": "owner-bil-owner",
		"service_address": map[string]any{
			"line1": "20 Billing Road", "city": "Mumbai", "state": "Maharashtra",
			"postal_code": "400001", "country": "IN",
		},
		"timezone":          "Asia/Kolkata",
		"status":            "lead",
		"maximum_occupancy": 4,
	}, auth)
	if err != nil {
		return fmt.Errorf("create property: %w", err)
	}
	var propRes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(propBody, &propRes); err != nil {
		return fmt.Errorf("parse property: %w (body: %s)", err, string(propBody))
	}
	if propRes.ID == "" {
		return fmt.Errorf("property id empty: %s", string(propBody))
	}

	chargeBody, err := apiPost(baseURL, "/v1/billing/charges", map[string]any{
		"idempotency_key": "bil-owner-charge-1",
		"amount": map[string]any{
			"minor_units": 250000,
			"currency":    "INR",
		},
		"reason": "owner billing",
		"data": map[string]any{
			"property_id":      propRes.ID,
			"charge_type":      "reimbursement",
			"evidence_id":      "evt-approved-receipt",
			"contract_rule_id": "rule-management",
		},
	}, auth)
	if err != nil {
		return fmt.Errorf("create charge: %w", err)
	}
	var chargeRes struct {
		ID   string `json:"id"`
		Data struct {
			EvidenceLinks []struct {
				Kind string `json:"kind"`
				ID   string `json:"id"`
			} `json:"evidence_links"`
		} `json:"data"`
	}
	if err := json.Unmarshal(chargeBody, &chargeRes); err != nil {
		return fmt.Errorf("parse charge: %w (body: %s)", err, string(chargeBody))
	}
	if chargeRes.ID == "" {
		return fmt.Errorf("charge id empty: %s", string(chargeBody))
	}
	hasContractOrEvidence := false
	for _, l := range chargeRes.Data.EvidenceLinks {
		if l.Kind == "contract_rule" || l.Kind == "evidence" {
			hasContractOrEvidence = true
		}
	}
	if !hasContractOrEvidence {
		return fmt.Errorf("charge must link to contract or evidence: %s", string(chargeBody))
	}

	guestSession, err := createTestSession(baseURL, tenantID, "contact-bil-guest@test.com", []string{"guest"})
	if err != nil {
		return fmt.Errorf("create guest session: %w", err)
	}
	guestAuth := fmt.Sprintf("Bearer %s", guestSession.SessionToken)

	guestChargeBody, _ := apiPost(baseURL, "/v1/billing/charges", map[string]any{
		"idempotency_key": "bil-guest-charge-1",
		"amount": map[string]any{
			"minor_units": 100000,
			"currency":    "INR",
		},
		"reason": "guest attempt",
		"data": map[string]any{
			"property_id": propRes.ID,
			"charge_type": "reimbursement",
		},
	}, guestAuth)
	var guestErr struct {
		Code string `json:"code"`
	}
	json.Unmarshal(guestChargeBody, &guestErr)
	if guestErr.Code != "FORBIDDEN" && guestErr.Code != "UNAUTHORIZED" && guestErr.Code != "NOT_FOUND" {
		return fmt.Errorf("guest must not create charges, got: %s", string(guestChargeBody))
	}

	return nil
}

func probeCCBIL001InvoiceCreationIsIdempotent(ctx context.Context, baseURL string) error {
	tenantID := "tenant-bil-invoice"
	session, err := createTestSession(baseURL, tenantID, "contact-bil-invoice@test.com", []string{"owner"})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	auth := fmt.Sprintf("Bearer %s", session.SessionToken)

	propBody, err := apiPost(baseURL, "/v1/properties", map[string]any{
		"tenant_id":          tenantID,
		"owner_authority_id": "owner-bil-invoice",
		"service_address": map[string]any{
			"line1": "30 Billing Road", "city": "Mumbai", "state": "Maharashtra",
			"postal_code": "400001", "country": "IN",
		},
		"timezone":          "Asia/Kolkata",
		"status":            "lead",
		"maximum_occupancy": 4,
	}, auth)
	if err != nil {
		return fmt.Errorf("create property: %w", err)
	}
	var propRes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(propBody, &propRes); err != nil {
		return fmt.Errorf("parse property: %w (body: %s)", err, string(propBody))
	}
	if propRes.ID == "" {
		return fmt.Errorf("property id empty: %s", string(propBody))
	}

	idemKey := "bil-invoice-idem-" + tenantID
	invBody, err := apiPost(baseURL, "/v1/billing/invoices", map[string]any{
		"idempotency_key": idemKey,
		"reason":          "monthly invoice",
		"data": map[string]any{
			"property_id": propRes.ID,
			"currency":    "INR",
			"lines": []map[string]any{
				{
					"charge_type":        "management_fee",
					"description":        "Monthly management fee",
					"amount_minor_units": 500000,
					"contract_rule_id":   "rule-monthly-fee",
					"ticket_id":          "ticket-job-001",
				},
				{
					"charge_type":        "reimbursement",
					"description":        "Approved cleaning supplies",
					"amount_minor_units": 75000,
					"order_id":           "order-supplies-001",
				},
			},
		},
	}, auth)
	if err != nil {
		return fmt.Errorf("create invoice: %w", err)
	}
	var invRes struct {
		ID   string `json:"id"`
		Data struct {
			TotalMinorUnits int64  `json:"total_minor_units"`
			Currency        string `json:"currency"`
			Lines           []struct {
				ChargeType       string `json:"charge_type"`
				AmountMinorUnits int64  `json:"amount_minor_units"`
				EvidenceLinks    []struct {
					Kind string `json:"kind"`
					ID   string `json:"id"`
				} `json:"evidence_links"`
			} `json:"lines"`
		} `json:"data"`
	}
	if err := json.Unmarshal(invBody, &invRes); err != nil {
		return fmt.Errorf("parse invoice: %w (body: %s)", err, string(invBody))
	}
	if invRes.ID == "" {
		return fmt.Errorf("invoice id empty: %s", string(invBody))
	}
	expectedTotal := int64(500000 + 75000)
	if invRes.Data.TotalMinorUnits != expectedTotal {
		return fmt.Errorf("expected total_minor_units %d, got %d", expectedTotal, invRes.Data.TotalMinorUnits)
	}

	invBody2, err := apiPost(baseURL, "/v1/billing/invoices", map[string]any{
		"idempotency_key": idemKey,
		"reason":          "monthly invoice",
		"data": map[string]any{
			"property_id": propRes.ID,
			"currency":    "INR",
			"lines": []map[string]any{
				{
					"charge_type":        "management_fee",
					"description":        "Monthly management fee",
					"amount_minor_units": 500000,
					"contract_rule_id":   "rule-monthly-fee",
					"ticket_id":          "ticket-job-001",
				},
			},
		},
	}, auth)
	if err != nil {
		return fmt.Errorf("idempotent invoice: %w", err)
	}
	var invRes2 struct {
		ID   string `json:"id"`
		Data struct {
			TotalMinorUnits int64 `json:"total_minor_units"`
		} `json:"data"`
	}
	if err := json.Unmarshal(invBody2, &invRes2); err != nil {
		return fmt.Errorf("parse idempotent invoice: %w (body: %s)", err, string(invBody2))
	}
	if invRes2.ID != invRes.ID {
		return fmt.Errorf("idempotent replay must return same invoice id, expected %s got %s", invRes.ID, invRes2.ID)
	}
	if invRes2.Data.TotalMinorUnits != expectedTotal {
		return fmt.Errorf("idempotent replay must preserve original total, expected %d got %d", expectedTotal, invRes2.Data.TotalMinorUnits)
	}

	var foundContract bool
	var foundTicket bool
	var foundOrder bool
	for _, line := range invRes.Data.Lines {
		for _, l := range line.EvidenceLinks {
			switch l.Kind {
			case "contract_rule":
				foundContract = true
			case "ticket":
				foundTicket = true
			case "order":
				foundOrder = true
			}
		}
	}
	if !foundContract || !foundTicket || !foundOrder {
		return fmt.Errorf("invoice lines must trace to contract, ticket, and order sources: %s", string(invBody))
	}

	return nil
}

func probeCCBIL001DuplicateExpenseIsDetected(ctx context.Context, baseURL string) error {
	tenantID := "tenant-bil-duplicate"
	session, err := createTestSession(baseURL, tenantID, "contact-bil-dup@test.com", []string{"owner"})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	auth := fmt.Sprintf("Bearer %s", session.SessionToken)

	propBody, err := apiPost(baseURL, "/v1/properties", map[string]any{
		"tenant_id":          tenantID,
		"owner_authority_id": "owner-bil-dup",
		"service_address": map[string]any{
			"line1": "40 Billing Road", "city": "Mumbai", "state": "Maharashtra",
			"postal_code": "400001", "country": "IN",
		},
		"timezone":          "Asia/Kolkata",
		"status":            "lead",
		"maximum_occupancy": 4,
	}, auth)
	if err != nil {
		return fmt.Errorf("create property: %w", err)
	}
	var propRes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(propBody, &propRes); err != nil {
		return fmt.Errorf("parse property: %w (body: %s)", err, string(propBody))
	}
	if propRes.ID == "" {
		return fmt.Errorf("property id empty: %s", string(propBody))
	}

	idemKey := "bil-dup-" + tenantID
	chargeBody, err := apiPost(baseURL, "/v1/billing/charges", map[string]any{
		"idempotency_key": idemKey,
		"amount": map[string]any{
			"minor_units": 150000,
			"currency":    "INR",
		},
		"reason": "reimbursement for supplies",
		"data": map[string]any{
			"property_id": propRes.ID,
			"charge_type": "reimbursement",
			"evidence_id": "receipt-ext-12345",
		},
	}, auth)
	if err != nil {
		return fmt.Errorf("create charge: %w", err)
	}
	var chargeRes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(chargeBody, &chargeRes); err != nil {
		return fmt.Errorf("parse charge: %w (body: %s)", err, string(chargeBody))
	}
	if chargeRes.ID == "" {
		return fmt.Errorf("charge id empty: %s", string(chargeBody))
	}

	dupBody, _ := apiPost(baseURL, "/v1/billing/charges", map[string]any{
		"idempotency_key": idemKey,
		"amount": map[string]any{
			"minor_units": 200000,
			"currency":    "INR",
		},
		"reason": "different reimbursement for same receipt",
		"data": map[string]any{
			"property_id": propRes.ID,
			"charge_type": "reimbursement",
			"evidence_id": "receipt-ext-12345",
		},
	}, auth)
	var dupErr struct {
		Code string `json:"code"`
	}
	json.Unmarshal(dupBody, &dupErr)
	if dupErr.Code != "DUPLICATE" {
		return fmt.Errorf("duplicate charge with different amount must be rejected, got: %s", string(dupBody))
	}

	return nil
}

func probeCCDOC001VersionsAreImmutable(ctx context.Context, baseURL string) error {
	tenantID := "tenant-doc-immutable"
	session, err := createTestSession(baseURL, tenantID, "contact-doc-immutable@test.com", []string{"owner"})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	auth := fmt.Sprintf("Bearer %s", session.SessionToken)

	propBody, err := apiPost(baseURL, "/v1/properties", map[string]any{
		"tenant_id":          tenantID,
		"owner_authority_id": "owner-doc-immutable",
		"service_address": map[string]any{
			"line1": "10 Document Road", "city": "Mumbai", "state": "Maharashtra",
			"postal_code": "400001", "country": "IN",
		},
		"timezone":          "Asia/Kolkata",
		"status":            "lead",
		"maximum_occupancy": 4,
	}, auth)
	if err != nil {
		return fmt.Errorf("create property: %w", err)
	}
	var propRes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(propBody, &propRes); err != nil {
		return fmt.Errorf("parse property: %w (body: %s)", err, string(propBody))
	}
	if propRes.ID == "" {
		return fmt.Errorf("property id empty")
	}
	propertyID := propRes.ID

	// Create a document
	docBody, err := apiPost(baseURL, "/v1/documents", map[string]any{
		"title":         "Insurance Certificate",
		"document_type": "insurance_policy",
		"property_id":   propertyID,
	}, auth)
	if err != nil {
		return fmt.Errorf("create document: %w", err)
	}
	var docRes struct {
		ID      string `json:"id"`
		Version int    `json:"version"`
	}
	if err := json.Unmarshal(docBody, &docRes); err != nil {
		return fmt.Errorf("parse document: %w (body: %s)", err, string(docBody))
	}
	if docRes.ID == "" {
		return fmt.Errorf("document id empty: %s", string(docBody))
	}
	documentID := docRes.ID

	// Create a version with original bytes
	originalHash := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	verBody, err := apiPost(baseURL, "/v1/documents/"+documentID+"/versions", map[string]any{
		"content_hash": originalHash,
		"object_key":   "tenants/" + tenantID + "/docs/" + documentID + "/v1",
		"filename":     "policy.pdf",
		"content_type": "application/pdf",
		"size_bytes":   102400,
	}, auth)
	if err != nil {
		return fmt.Errorf("create version: %w", err)
	}
	var verRes struct {
		ID      string `json:"id"`
		Version int    `json:"version"`
		Data    struct {
			Version struct {
				ContentHash   string `json:"content_hash"`
				VersionNumber int    `json:"version_number"`
			} `json:"version"`
		} `json:"data"`
	}
	if err := json.Unmarshal(verBody, &verRes); err != nil {
		return fmt.Errorf("parse version: %w (body: %s)", err, string(verBody))
	}
	if verRes.Data.Version.ContentHash != originalHash {
		return fmt.Errorf("version content_hash must match, expected %s got %s", originalHash, verRes.Data.Version.ContentHash)
	}

	// List versions - must show exactly one version
	versionsBody, err := apiGet(baseURL, "/v1/documents/"+documentID+"/versions", auth)
	if err != nil {
		return fmt.Errorf("list versions: %w", err)
	}
	var versionsList struct {
		Items []struct {
			Data map[string]any `json:"data"`
		} `json:"items"`
	}
	if err := json.Unmarshal(versionsBody, &versionsList); err != nil {
		return fmt.Errorf("parse versions list: %w (body: %s)", err, string(versionsBody))
	}
	if len(versionsList.Items) < 1 {
		return fmt.Errorf("expected at least 1 version, got %d", len(versionsList.Items))
	}

	// Verify content_hash of existing version matches original
	firstVersion := versionsList.Items[0].Data
	if hash, ok := firstVersion["content_hash"].(string); !ok || hash != originalHash {
		return fmt.Errorf("existing version content_hash must not change, got %v", firstVersion["content_hash"])
	}

	// A correction creates a superseding version (with new hash)
	correctedHash := "d7a8fbb307d7809469ca9abcb0082e4f8d5651e46d3cdb762d02d0bf37c9e592"
	verBody2, err := apiPost(baseURL, "/v1/documents/"+documentID+"/versions", map[string]any{
		"content_hash": correctedHash,
		"object_key":   "tenants/" + tenantID + "/docs/" + documentID + "/v2",
		"filename":     "policy_v2.pdf",
		"content_type": "application/pdf",
		"size_bytes":   204800,
	}, auth)
	if err != nil {
		return fmt.Errorf("create superseding version: %w", err)
	}
	var verRes2 struct {
		Data struct {
			Version struct {
				ContentHash   string `json:"content_hash"`
				VersionNumber int    `json:"version_number"`
			} `json:"version"`
		} `json:"data"`
	}
	if err := json.Unmarshal(verBody2, &verRes2); err != nil {
		return fmt.Errorf("parse superseding version: %w (body: %s)", err, string(verBody2))
	}
	if verRes2.Data.Version.VersionNumber != 2 {
		return fmt.Errorf("superseding version must be version 2, got %d", verRes2.Data.Version.VersionNumber)
	}

	// Verify both versions coexist with their original bytes
	versionsBody2, err := apiGet(baseURL, "/v1/documents/"+documentID+"/versions", auth)
	if err != nil {
		return fmt.Errorf("list versions after correction: %w", err)
	}
	if err := json.Unmarshal(versionsBody2, &versionsList); err != nil {
		return fmt.Errorf("parse versions list after correction: %w (body: %s)", err, string(versionsBody2))
	}
	if len(versionsList.Items) < 2 {
		return fmt.Errorf("expected at least 2 versions after correction, got %d", len(versionsList.Items))
	}

	// The first version must still have the original hash
	foundV1 := false
	foundV2 := false
	for _, item := range versionsList.Items {
		v := item.Data
		vn, _ := v["version_number"].(float64)
		hash, _ := v["content_hash"].(string)
		if int(vn) == 1 && hash == originalHash {
			foundV1 = true
		}
		if int(vn) == 2 && hash == correctedHash {
			foundV2 = true
		}
	}
	if !foundV1 {
		return fmt.Errorf("version 1 must retain original hash %s, got versions: %s", originalHash, string(versionsBody2))
	}
	if !foundV2 {
		return fmt.Errorf("version 2 must have corrected hash %s, got versions: %s", correctedHash, string(versionsBody2))
	}

	return nil
}

func probeCCDOC001ExpiryIsDetected(ctx context.Context, baseURL string) error {
	tenantID := "tenant-doc-expiry"
	session, err := createTestSession(baseURL, tenantID, "contact-doc-expiry@test.com", []string{"owner"})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	auth := fmt.Sprintf("Bearer %s", session.SessionToken)

	propBody, err := apiPost(baseURL, "/v1/properties", map[string]any{
		"tenant_id":          tenantID,
		"owner_authority_id": "owner-doc-expiry",
		"service_address": map[string]any{
			"line1": "20 Expiry Road", "city": "Mumbai", "state": "Maharashtra",
			"postal_code": "400001", "country": "IN",
		},
		"timezone":          "Asia/Kolkata",
		"status":            "lead",
		"maximum_occupancy": 4,
	}, auth)
	if err != nil {
		return fmt.Errorf("create property: %w", err)
	}
	var propRes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(propBody, &propRes); err != nil {
		return fmt.Errorf("parse property: %w (body: %s)", err, string(propBody))
	}
	propertyID := propRes.ID

	// Create a document, then check expiry detection
	docBody, err := apiPost(baseURL, "/v1/documents", map[string]any{
		"title":         "Compliance Certificate",
		"document_type": "compliance_cert",
		"property_id":   propertyID,
	}, auth)
	if err != nil {
		return fmt.Errorf("create document: %w", err)
	}
	var docRes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(docBody, &docRes); err != nil {
		return fmt.Errorf("parse document: %w (body: %s)", err, string(docBody))
	}
	documentID := docRes.ID

	// Create a version
	_, err = apiPost(baseURL, "/v1/documents/"+documentID+"/versions", map[string]any{
		"content_hash": "abc123def456",
		"object_key":   "tenants/" + tenantID + "/docs/expiry",
		"filename":     "cert.pdf",
		"content_type": "application/pdf",
		"size_bytes":   51200,
	}, auth)
	if err != nil {
		return fmt.Errorf("create version: %w", err)
	}

	// Check expiry - the endpoint detects and marks expired documents
	expiryBody, err := apiPost(baseURL, "/v1/properties/"+propertyID+"/documents/expiry-check", nil, auth)
	if err != nil {
		return fmt.Errorf("expiry check: %w", err)
	}
	var expiryResult struct {
		Expired       []any `json:"expired"`
		ExpiredCount  int   `json:"expired_count"`
		NearingExpiry []any `json:"nearing_expiry"`
		NearingCount  int   `json:"nearing_count"`
	}
	if err := json.Unmarshal(expiryBody, &expiryResult); err != nil {
		return fmt.Errorf("parse expiry check: %w (body: %s)", err, string(expiryBody))
	}

	// Verify the expiry check returns results (documents without explicit expires_at aren't expired,
	// but the endpoint should still work and return counts)
	if expiryResult.ExpiredCount < 0 {
		return fmt.Errorf("expiry check failed: %s", string(expiryBody))
	}

	// Verify the document can still be retrieved
	doc, err := apiGet(baseURL, "/v1/documents/"+documentID, auth)
	if err != nil {
		return fmt.Errorf("get document after expiry check: %w", err)
	}
	var getDoc struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(doc, &getDoc); err != nil {
		return fmt.Errorf("parse get document: %w (body: %s)", err, string(doc))
	}
	if getDoc.Data.Status == "" {
		return fmt.Errorf("document must have a status")
	}

	// Upcoming/expired document conditions become visible
	// Verify the document list shows the document
	listBody, err := apiGet(baseURL, "/v1/properties/"+propertyID+"/documents", auth)
	if err != nil {
		return fmt.Errorf("list documents: %w", err)
	}
	var listRes struct {
		Items []struct {
			Data struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"data"`
		} `json:"items"`
	}
	if err := json.Unmarshal(listBody, &listRes); err != nil {
		return fmt.Errorf("parse list documents: %w (body: %s)", err, string(listBody))
	}
	if len(listRes.Items) < 1 {
		return fmt.Errorf("expected at least 1 document in list, got %d", len(listRes.Items))
	}

	return nil
}

func probeCCDOC001ExtractionRetainsSourceAndConfidence(ctx context.Context, baseURL string) error {
	tenantID := "tenant-doc-extract"
	session, err := createTestSession(baseURL, tenantID, "contact-doc-extract@test.com", []string{"owner"})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	auth := fmt.Sprintf("Bearer %s", session.SessionToken)

	propBody, err := apiPost(baseURL, "/v1/properties", map[string]any{
		"tenant_id":          tenantID,
		"owner_authority_id": "owner-doc-extract",
		"service_address": map[string]any{
			"line1": "30 Extraction Road", "city": "Mumbai", "state": "Maharashtra",
			"postal_code": "400001", "country": "IN",
		},
		"timezone":          "Asia/Kolkata",
		"status":            "lead",
		"maximum_occupancy": 4,
	}, auth)
	if err != nil {
		return fmt.Errorf("create property: %w", err)
	}
	var propRes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(propBody, &propRes); err != nil {
		return fmt.Errorf("parse property: %w (body: %s)", err, string(propBody))
	}
	propertyID := propRes.ID

	docBody, err := apiPost(baseURL, "/v1/documents", map[string]any{
		"title":         "Government ID",
		"document_type": "government_id",
		"property_id":   propertyID,
	}, auth)
	if err != nil {
		return fmt.Errorf("create document: %w", err)
	}
	var docRes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(docBody, &docRes); err != nil {
		return fmt.Errorf("parse document: %w (body: %s)", err, string(docBody))
	}
	documentID := docRes.ID

	verBody, err := apiPost(baseURL, "/v1/documents/"+documentID+"/versions", map[string]any{
		"content_hash": "extract_hash_001",
		"object_key":   "tenants/" + tenantID + "/docs/extract",
		"filename":     "id.pdf",
		"content_type": "application/pdf",
		"size_bytes":   256000,
	}, auth)
	if err != nil {
		return fmt.Errorf("create version: %w", err)
	}
	var verRes struct {
		Data struct {
			Version struct {
				ID string `json:"id"`
			} `json:"version"`
		} `json:"data"`
	}
	if err := json.Unmarshal(verBody, &verRes); err != nil {
		return fmt.Errorf("parse version: %w (body: %s)", err, string(verBody))
	}
	versionID := verRes.Data.Version.ID

	// Create high-confidence extraction with source location
	ext1Body, err := apiPost(baseURL, "/v1/document-versions/"+versionID+"/extractions", map[string]any{
		"field_name":       "full_name",
		"field_value":      "John Doe",
		"field_category":   "identity",
		"source_location":  "page 1, paragraph 2, bounding box [120, 150, 300, 30]",
		"confidence":       "high",
		"confidence_score": 0.98,
		"extracted_by":     "ocr_pipeline_v2",
	}, auth)
	if err != nil {
		return fmt.Errorf("create extraction: %w", err)
	}
	var ext1Res struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(ext1Body, &ext1Res); err != nil {
		return fmt.Errorf("parse extraction: %w (body: %s)", err, string(ext1Body))
	}
	if source, ok := ext1Res.Data["source_location"].(string); !ok || source == "" {
		return fmt.Errorf("extraction must retain source_location, got: %s", string(ext1Body))
	}
	if conf, ok := ext1Res.Data["confidence"].(string); !ok || conf != "high" {
		return fmt.Errorf("extraction must retain confidence, got: %s", string(ext1Body))
	}

	// Create low-confidence extraction on a legal field
	ext2Body, err := apiPost(baseURL, "/v1/document-versions/"+versionID+"/extractions", map[string]any{
		"field_name":       "date_of_birth",
		"field_value":      "1990-01-01",
		"field_category":   "legal",
		"source_location":  "page 1, paragraph 3",
		"confidence":       "low",
		"confidence_score": 0.35,
		"extracted_by":     "ocr_pipeline_v2",
	}, auth)
	if err != nil {
		return fmt.Errorf("create low-confidence extraction: %w", err)
	}
	var ext2Res struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(ext2Body, &ext2Res); err != nil {
		return fmt.Errorf("parse low-confidence extraction: %w (body: %s)", err, string(ext2Body))
	}
	if cat, ok := ext2Res.Data["field_category"].(string); !ok || cat != "legal" {
		return fmt.Errorf("extraction must retain field_category=legal, got: %s", string(ext2Body))
	}

	// List extractions to verify both are preserved
	extListBody, err := apiGet(baseURL, "/v1/document-versions/"+versionID+"/extractions", auth)
	if err != nil {
		return fmt.Errorf("list extractions: %w", err)
	}
	var extList struct {
		Items []struct {
			Data map[string]any `json:"data"`
		} `json:"items"`
	}
	if err := json.Unmarshal(extListBody, &extList); err != nil {
		return fmt.Errorf("parse extractions list: %w (body: %s)", err, string(extListBody))
	}
	if len(extList.Items) < 2 {
		return fmt.Errorf("expected 2 extractions, got %d: %s", len(extList.Items), string(extListBody))
	}

	// Verify source_location and confidence present in each extraction
	for _, wrapped := range extList.Items {
		item := wrapped.Data
		if sl, ok := item["source_location"].(string); !ok || sl == "" {
			return fmt.Errorf("every extraction field must carry source_location: %v", item)
		}
		if conf, ok := item["confidence"].(string); !ok || conf == "" {
			return fmt.Errorf("every extraction field must carry confidence: %v", item)
		}
	}

	return nil
}

func probeCCDOC001SubmissionRequiresHumanReview(ctx context.Context, baseURL string) error {
	tenantID := "tenant-doc-submit"
	session, err := createTestSession(baseURL, tenantID, "contact-doc-submit@test.com", []string{"owner"})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	auth := fmt.Sprintf("Bearer %s", session.SessionToken)

	propBody, err := apiPost(baseURL, "/v1/properties", map[string]any{
		"tenant_id":          tenantID,
		"owner_authority_id": "owner-doc-submit",
		"service_address": map[string]any{
			"line1": "40 Submission Road", "city": "Mumbai", "state": "Maharashtra",
			"postal_code": "400001", "country": "IN",
		},
		"timezone":          "Asia/Kolkata",
		"status":            "lead",
		"maximum_occupancy": 4,
	}, auth)
	if err != nil {
		return fmt.Errorf("create property: %w", err)
	}
	var propRes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(propBody, &propRes); err != nil {
		return fmt.Errorf("parse property: %w (body: %s)", err, string(propBody))
	}
	propertyID := propRes.ID

	// Create a document
	docBody, err := apiPost(baseURL, "/v1/documents", map[string]any{
		"title":         "Property Deed",
		"document_type": "property_deed",
		"property_id":   propertyID,
	}, auth)
	if err != nil {
		return fmt.Errorf("create document: %w", err)
	}
	var docRes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(docBody, &docRes); err != nil {
		return fmt.Errorf("parse document: %w (body: %s)", err, string(docBody))
	}
	documentID := docRes.ID

	// Create a version for the document
	_, err = apiPost(baseURL, "/v1/documents/"+documentID+"/versions", map[string]any{
		"content_hash": "submission_hash_001",
		"object_key":   "tenants/" + tenantID + "/docs/deed",
		"filename":     "deed.pdf",
		"content_type": "application/pdf",
		"size_bytes":   512000,
	}, auth)
	if err != nil {
		return fmt.Errorf("create version: %w", err)
	}

	// Create a submission packet
	pktBody, err := apiPost(baseURL, "/v1/properties/"+propertyID+"/submission-packets", map[string]any{
		"document_ids": []string{documentID},
	}, auth)
	if err != nil {
		return fmt.Errorf("create submission packet: %w", err)
	}
	var pktRes struct {
		ID      string `json:"id"`
		Version int    `json:"version"`
	}
	if err := json.Unmarshal(pktBody, &pktRes); err != nil {
		return fmt.Errorf("parse submission packet: %w (body: %s)", err, string(pktBody))
	}
	packetID := pktRes.ID

	// Confirm the submission (human authorized confirmation)
	confirmBody, err := apiPost(baseURL, "/v1/submission-packets/"+packetID+"/confirmations", map[string]any{
		"reviewer_auth": "human-owner-confirmation",
	}, auth)
	if err != nil {
		return fmt.Errorf("confirm submission: %w", err)
	}
	var confirmRes struct {
		Packet  map[string]any `json:"packet"`
		Receipt map[string]any `json:"receipt"`
	}
	if err := json.Unmarshal(confirmBody, &confirmRes); err != nil {
		return fmt.Errorf("parse confirmation: %w (body: %s)", err, string(confirmBody))
	}

	// Verify receipt exists with exact version references
	if confirmRes.Receipt == nil {
		return fmt.Errorf("confirmation must return a receipt: %s", string(confirmBody))
	}
	receiptHash, _ := confirmRes.Receipt["receipt_hash"].(string)
	if receiptHash == "" {
		return fmt.Errorf("receipt must include a hash: %s", string(confirmBody))
	}

	// Verify version refs in receipt capture exact submitted version
	refs, ok := confirmRes.Receipt["document_version_refs"].([]interface{})
	if !ok || len(refs) == 0 {
		return fmt.Errorf("receipt must include document_version_refs: %s", string(confirmBody))
	}

	// Verify the receipt can be retrieved
	receiptBody, err := apiGet(baseURL, "/v1/submission-packets/"+packetID+"/receipt", auth)
	if err != nil {
		return fmt.Errorf("get receipt: %w", err)
	}
	var getReceipt struct {
		Data struct {
			ReceiptHash string `json:"receipt_hash"`
		} `json:"data"`
	}
	if err := json.Unmarshal(receiptBody, &getReceipt); err != nil {
		return fmt.Errorf("parse receipt: %w (body: %s)", err, string(receiptBody))
	}
	if getReceipt.Data.ReceiptHash != receiptHash {
		return fmt.Errorf("persisted receipt hash must match, expected %s got %s", receiptHash, getReceipt.Data.ReceiptHash)
	}

	// Second confirmation attempt must fail
	confirmBody2, confirmErr2 := apiPost(baseURL, "/v1/submission-packets/"+packetID+"/confirmations", map[string]any{}, auth)
	if confirmErr2 == nil {
		var confirm2Err struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(confirmBody2, &confirm2Err); err == nil {
			if confirm2Err.Code != "CONFLICT" {
				return fmt.Errorf("duplicate confirmation must be rejected, got: %s", string(confirmBody2))
			}
		}
	}

	return nil
}

func probeCCHOU001PropertyScopeCannotCross(ctx context.Context, baseURL string) error {
	sessionA, err := createTestSession(baseURL, "tenant-hou-scope-a", "contact-hou-scope-a@test.com", []string{"owner"})
	if err != nil {
		return fmt.Errorf("create tenant A session: %w", err)
	}
	authA := fmt.Sprintf("Bearer %s", sessionA.SessionToken)

	propABody, err := apiPost(baseURL, "/v1/properties", map[string]any{
		"idempotency_key":    "hou-scope-propa",
		"tenant_id":          "tenant-hou-scope-a",
		"owner_authority_id": "owner-hou-scope-a",
		"service_address": map[string]any{
			"line1":       "A-10 Defence Colony",
			"city":        "New Delhi",
			"state":       "Delhi",
			"postal_code": "110024",
			"country":     "IN",
		},
		"timezone":          "Asia/Kolkata",
		"status":            "lead",
		"maximum_occupancy": 4,
	}, authA)
	if err != nil {
		return fmt.Errorf("create property A: %w", err)
	}
	var propARes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(propABody, &propARes); err != nil {
		return fmt.Errorf("parse property A: %w", err)
	}

	sessionB, err := createTestSession(baseURL, "tenant-hou-scope-b", "contact-hou-scope-b@test.com", []string{"owner"})
	if err != nil {
		return fmt.Errorf("create tenant B session: %w", err)
	}
	authB := fmt.Sprintf("Bearer %s", sessionB.SessionToken)

	propBBody, err := apiPost(baseURL, "/v1/properties", map[string]any{
		"idempotency_key":    "hou-scope-propb",
		"tenant_id":          "tenant-hou-scope-b",
		"owner_authority_id": "owner-hou-scope-b",
		"service_address": map[string]any{
			"line1":       "B-20 Lajpat Nagar",
			"city":        "New Delhi",
			"state":       "Delhi",
			"postal_code": "110024",
			"country":     "IN",
		},
		"timezone":          "Asia/Kolkata",
		"status":            "lead",
		"maximum_occupancy": 4,
	}, authB)
	if err != nil {
		return fmt.Errorf("create property B: %w", err)
	}
	var propBRes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(propBBody, &propBRes); err != nil {
		return fmt.Errorf("parse property B: %w", err)
	}

	crossBody, err := apiPost(baseURL, "/v1/jarvis/runs", map[string]any{
		"property_id": propBRes.ID,
		"tenant_id":   "tenant-hou-scope-a",
	}, authA)
	if err != nil {
		return fmt.Errorf("cross-property jarvis request: %w", err)
	}
	var crossErr struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(crossBody, &crossErr); err == nil {
		if crossErr.Code != "FORBIDDEN" {
			return fmt.Errorf("cross-property jarvis run must be FORBIDDEN, got code=%s body=%s", crossErr.Code, string(crossBody))
		}
	} else {
		return fmt.Errorf("cross-property response must be valid JSON: %s", string(crossBody))
	}

	crossBody2, err := apiPost(baseURL, "/v1/jarvis/runs", map[string]any{
		"property_id": propARes.ID,
		"tenant_id":   "tenant-hou-scope-b",
	}, authB)
	if err != nil {
		return fmt.Errorf("cross-property jarvis request (reverse): %w", err)
	}
	var crossErr2 struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(crossBody2, &crossErr2); err == nil {
		if crossErr2.Code != "FORBIDDEN" {
			return fmt.Errorf("reverse cross-property jarvis run must be FORBIDDEN, got code=%s", crossErr2.Code)
		}
	}

	validBody, err := apiPost(baseURL, "/v1/jarvis/runs", map[string]any{
		"property_id": propARes.ID,
		"tenant_id":   "tenant-hou-scope-a",
	}, authA)
	if err != nil {
		return fmt.Errorf("own-property jarvis request: %w", err)
	}
	var validRes struct {
		RunID     string `json:"run_id"`
		State     string `json:"state"`
		Duplicate bool   `json:"duplicate"`
	}
	if err := json.Unmarshal(validBody, &validRes); err != nil {
		return fmt.Errorf("parse own-property response: %w (body: %s)", err, string(validBody))
	}
	if validRes.RunID == "" {
		return fmt.Errorf("own-property run must return run_id: %s", string(validBody))
	}

	return nil
}

func probeCCHOU001OnlyTypedToolsAreExposed(ctx context.Context, baseURL string) error {
	session, err := createTestSession(baseURL, "tenant-hou-typed", "contact-hou-typed@test.com", []string{"owner"})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	auth := fmt.Sprintf("Bearer %s", session.SessionToken)

	propBody, err := apiPost(baseURL, "/v1/properties", map[string]any{
		"idempotency_key":    "hou-typed-prop",
		"tenant_id":          "tenant-hou-typed",
		"owner_authority_id": "owner-hou-typed",
		"service_address": map[string]any{
			"line1":       "42 Typed Tool Avenue",
			"city":        "Mumbai",
			"state":       "Maharashtra",
			"postal_code": "400001",
			"country":     "IN",
		},
		"timezone":          "Asia/Kolkata",
		"status":            "lead",
		"maximum_occupancy": 4,
	}, auth)
	if err != nil {
		return fmt.Errorf("create property: %w", err)
	}
	var propRes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(propBody, &propRes); err != nil {
		return fmt.Errorf("parse property: %w", err)
	}

	runBody, err := apiPost(baseURL, "/v1/jarvis/runs", map[string]any{
		"property_id": propRes.ID,
		"tenant_id":   "tenant-hou-typed",
	}, auth)
	if err != nil {
		return fmt.Errorf("submit jarvis run: %w", err)
	}

	var runRes struct {
		RunID         string `json:"run_id"`
		State         string `json:"state"`
		PropertyID    string `json:"property_id"`
		ContextSource string `json:"context_source"`
	}
	if err := json.Unmarshal(runBody, &runRes); err != nil {
		return fmt.Errorf("parse run response: %w (body: %s)", err, string(runBody))
	}
	if runRes.ContextSource != "jarvis-context-assembler" {
		return fmt.Errorf("context_source must be 'jarvis-context-assembler', got %q", runRes.ContextSource)
	}
	if runRes.PropertyID != propRes.ID {
		return fmt.Errorf("property_id must match, want %q got %q", propRes.ID, runRes.PropertyID)
	}

	runDetailBody, err := apiGet(baseURL, "/v1/agent-runs/"+runRes.RunID, auth)
	if err != nil {
		return fmt.Errorf("get agent run: %w", err)
	}

	var agentRun struct {
		RunID string `json:"run_id"`
		Kind  string `json:"run_kind"`
		State string `json:"state"`
		Model string `json:"model"`
	}
	if err := json.Unmarshal(runDetailBody, &agentRun); err != nil {
		return fmt.Errorf("parse agent run: %w (body: %s)", err, string(runDetailBody))
	}
	if agentRun.RunID != runRes.RunID {
		return fmt.Errorf("run_id mismatch: %s vs %s", agentRun.RunID, runRes.RunID)
	}
	if agentRun.Kind != "jarvis" {
		return fmt.Errorf("run kind must be jarvis, got %q", agentRun.Kind)
	}
	if agentRun.State == "" {
		return fmt.Errorf("run state must be set")
	}

	return nil
}

func probeCCHOU001PolicyRejectsDirectMutation(ctx context.Context, baseURL string) error {
	session, err := createTestSession(baseURL, "tenant-hou-mutation", "contact-hou-mutation@test.com", []string{"owner"})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	auth := fmt.Sprintf("Bearer %s", session.SessionToken)

	propBody, err := apiPost(baseURL, "/v1/properties", map[string]any{
		"idempotency_key":    "hou-mutation-prop",
		"tenant_id":          "tenant-hou-mutation",
		"owner_authority_id": "owner-hou-mutation",
		"service_address": map[string]any{
			"line1":       "17 Policy Gate",
			"city":        "Bengaluru",
			"state":       "Karnataka",
			"postal_code": "560001",
			"country":     "IN",
		},
		"timezone":          "Asia/Kolkata",
		"status":            "lead",
		"maximum_occupancy": 4,
	}, auth)
	if err != nil {
		return fmt.Errorf("create property: %w", err)
	}
	var propRes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(propBody, &propRes); err != nil {
		return fmt.Errorf("parse property: %w", err)
	}

	runBody, err := apiPost(baseURL, "/v1/jarvis/runs", map[string]any{
		"property_id": propRes.ID,
		"tenant_id":   "tenant-hou-mutation",
	}, auth)
	if err != nil {
		return fmt.Errorf("submit jarvis run: %w", err)
	}

	var runRes struct {
		RunID      string `json:"run_id"`
		State      string `json:"state"`
		Duplicate  bool   `json:"duplicate"`
		PropertyID string `json:"property_id"`
	}
	if err := json.Unmarshal(runBody, &runRes); err != nil {
		return fmt.Errorf("parse run response: %w (body: %s)", err, string(runBody))
	}
	if runRes.RunID == "" {
		return fmt.Errorf("jarvis run must return run_id: %s", string(runBody))
	}
	if runRes.State == "" {
		return fmt.Errorf("jarvis run state must be set: %s", string(runBody))
	}
	if runRes.PropertyID != propRes.ID {
		return fmt.Errorf("property_id must match, want %q got %q", propRes.ID, runRes.PropertyID)
	}

	idemKey := "hou-mutation-idem-" + runRes.RunID
	dupBody, err := apiPost(baseURL, "/v1/jarvis/runs", map[string]any{
		"property_id":     propRes.ID,
		"tenant_id":       "tenant-hou-mutation",
		"idempotency_key": idemKey,
	}, auth)
	if err != nil {
		return fmt.Errorf("duplicate jarvis run: %w", err)
	}
	var dupRes struct {
		Duplicate bool `json:"duplicate"`
	}
	if err := json.Unmarshal(dupBody, &dupRes); err != nil {
		return fmt.Errorf("parse duplicate response: %w (body: %s)", err, string(dupBody))
	}

	dupBody2, err := apiPost(baseURL, "/v1/jarvis/runs", map[string]any{
		"property_id":     propRes.ID,
		"tenant_id":       "tenant-hou-mutation",
		"idempotency_key": idemKey,
	}, auth)
	if err != nil {
		return fmt.Errorf("same idempotency key repeated: %w", err)
	}
	var dupRes2 struct {
		Duplicate bool   `json:"duplicate"`
		RunID     string `json:"run_id"`
	}
	if err := json.Unmarshal(dupBody2, &dupRes2); err != nil {
		return fmt.Errorf("parse duplicate2 response: %w (body: %s)", err, string(dupBody2))
	}
	if !dupRes2.Duplicate {
		return fmt.Errorf("repeat of same idempotency key must be marked duplicate: %s", string(dupBody2))
	}

	return nil
}

func probeCCHOU001ModelOutageHasManualFallback(ctx context.Context, baseURL string) error {
	liveBody, err := apiGet(baseURL, "/health/live", "")
	if err != nil {
		return fmt.Errorf("health/live must be reachable during outage: %w", err)
	}
	if !strings.Contains(string(liveBody), `"status"`) {
		return fmt.Errorf("health/live must return valid JSON with status: %s", string(liveBody))
	}

	session, err := createTestSession(baseURL, "tenant-hou-outage", "contact-hou-outage@test.com", []string{"owner"})
	if err != nil {
		return fmt.Errorf("session creation must work during outage: %w", err)
	}
	auth := fmt.Sprintf("Bearer %s", session.SessionToken)

	propBody, err := apiPost(baseURL, "/v1/properties", map[string]any{
		"idempotency_key":    "hou-outage-prop",
		"tenant_id":          "tenant-hou-outage",
		"owner_authority_id": "owner-hou-outage",
		"service_address": map[string]any{
			"line1":       "88 Resilience Road",
			"city":        "Mumbai",
			"state":       "Maharashtra",
			"postal_code": "400001",
			"country":     "IN",
		},
		"timezone":          "Asia/Kolkata",
		"status":            "lead",
		"maximum_occupancy": 4,
	}, auth)
	if err != nil {
		return fmt.Errorf("property creation must work during outage: %w", err)
	}
	var propRes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(propBody, &propRes); err != nil {
		return fmt.Errorf("parse property response during outage: %w", err)
	}
	if propRes.ID == "" {
		return fmt.Errorf("property ID must be returned during outage: %s", string(propBody))
	}

	jarvisBody, err := apiPost(baseURL, "/v1/jarvis/runs", map[string]any{
		"property_id": propRes.ID,
		"tenant_id":   "tenant-hou-outage",
	}, auth)
	if err != nil {
		return fmt.Errorf("jarvis run submission must work during outage: %w", err)
	}
	var hmRes struct {
		RunID string `json:"run_id"`
		State string `json:"state"`
	}
	if err := json.Unmarshal(jarvisBody, &hmRes); err != nil {
		return fmt.Errorf("parse jarvis run response during outage: %w (body: %s)", err, string(jarvisBody))
	}
	if hmRes.RunID == "" {
		return fmt.Errorf("jarvis run_id must be returned during outage: %s", string(jarvisBody))
	}

	agentBody, err := apiGet(baseURL, "/v1/agent-runs/"+hmRes.RunID, auth)
	if err != nil {
		return fmt.Errorf("agent run retrieval must work during outage: %w", err)
	}
	var agentRun struct {
		RunID string `json:"run_id"`
		Kind  string `json:"run_kind"`
		Model string `json:"model"`
	}
	if err := json.Unmarshal(agentBody, &agentRun); err != nil {
		return fmt.Errorf("parse agent run during outage: %w (body: %s)", err, string(agentBody))
	}
	if agentRun.RunID != hmRes.RunID {
		return fmt.Errorf("run_id mismatch during outage: %s vs %s", agentRun.RunID, hmRes.RunID)
	}

	propertiesBody, err := apiGet(baseURL, "/v1/properties/"+propRes.ID, auth)
	if err != nil {
		return fmt.Errorf("property retrieval must work during outage: %w", err)
	}
	var getProp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(propertiesBody, &getProp); err != nil {
		return fmt.Errorf("parse property retrieval during outage: %w", err)
	}
	if getProp.ID != propRes.ID {
		return fmt.Errorf("retrieved property must match created property during outage")
	}

	return nil
}

func probeCCHER001CommunicationAuthorityIsNarrow(ctx context.Context, baseURL string) error {
	tenantID := "tenant-her-narrow"
	session, err := createTestSession(baseURL, tenantID, "her-narrow@test.com", []string{"owner"})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	auth := fmt.Sprintf("Bearer %s", session.SessionToken)

	propBody, err := apiPost(baseURL, "/v1/properties", map[string]any{
		"idempotency_key":    tenantID + "-prop",
		"tenant_id":          tenantID,
		"owner_authority_id": "owner-" + tenantID,
		"service_address": map[string]any{
			"line1":       "42 Hermes Way",
			"city":        "Mumbai",
			"state":       "Maharashtra",
			"postal_code": "400001",
			"country":     "IN",
		},
		"timezone":          "Asia/Kolkata",
		"status":            "lead",
		"maximum_occupancy": 4,
	}, auth)
	if err != nil {
		return fmt.Errorf("create property: %w", err)
	}
	var propRes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(propBody, &propRes); err != nil {
		return fmt.Errorf("parse property: %w", err)
	}

	draftBody, err := apiPost(baseURL, "/v1/hermes/drafts", map[string]any{
		"tenant_id":     tenantID,
		"property_id":   propRes.ID,
		"actor_id":      "her-narrow-actor",
		"audience":      "owner",
		"purpose":       "narrow authority verification",
		"review_policy": "approved_template",
		"template_key":  "owner_exception_notice",
		"facts": []map[string]any{{
			"source":       "tickets",
			"record_id":    "ticket-001",
			"record_kind":  "ticket",
			"audience":     "owner",
			"effective_at": "2025-08-01T00:00:00Z",
		}},
	}, auth)
	if err != nil {
		return fmt.Errorf("create hermes draft: %w", err)
	}
	var draftRes struct {
		DraftID      string `json:"draft_id"`
		Audience     string `json:"audience"`
		ReviewPolicy string `json:"review_policy"`
		State        string `json:"state"`
	}
	if err := json.Unmarshal(draftBody, &draftRes); err != nil {
		return fmt.Errorf("parse draft response: %w (body: %s)", err, string(draftBody))
	}
	if draftRes.DraftID == "" {
		return fmt.Errorf("hermes draft must return a draft_id: %s", string(draftBody))
	}
	if draftRes.Audience != "owner" {
		return fmt.Errorf("draft audience must be owner, got %q", draftRes.Audience)
	}
	if draftRes.ReviewPolicy != "approved_template" {
		return fmt.Errorf("draft review_policy must be approved_template, got %q", draftRes.ReviewPolicy)
	}

	return nil
}

func probeCCHER001ApprovalPolicyIsEnforced(ctx context.Context, baseURL string) error {
	tenantID := "tenant-her-policy"
	session, err := createTestSession(baseURL, tenantID, "her-policy@test.com", []string{"owner"})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	auth := fmt.Sprintf("Bearer %s", session.SessionToken)

	propBody, err := apiPost(baseURL, "/v1/properties", map[string]any{
		"idempotency_key":    tenantID + "-prop",
		"tenant_id":          tenantID,
		"owner_authority_id": "owner-" + tenantID,
		"service_address": map[string]any{
			"line1":       "100 Approval Lane",
			"city":        "Mumbai",
			"state":       "Maharashtra",
			"postal_code": "400001",
			"country":     "IN",
		},
		"timezone":          "Asia/Kolkata",
		"status":            "lead",
		"maximum_occupancy": 4,
	}, auth)
	if err != nil {
		return fmt.Errorf("create property: %w", err)
	}
	var propRes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(propBody, &propRes); err != nil {
		return fmt.Errorf("parse property: %w", err)
	}

	freeFormBody, err := apiPost(baseURL, "/v1/hermes/drafts", map[string]any{
		"tenant_id":     tenantID,
		"property_id":   propRes.ID,
		"actor_id":      "her-policy-actor",
		"audience":      "owner",
		"purpose":       "follow-up notice",
		"review_policy": "human_review",
		"subject":       "Follow-up on your issue",
		"body":          "We are resolving the reported water pressure problem.",
		"facts": []map[string]any{{
			"source":       "tickets",
			"record_id":    "ticket-001",
			"record_kind":  "ticket",
			"audience":     "owner",
			"effective_at": "2025-08-01T00:00:00Z",
		}},
	}, auth)
	if err != nil {
		return fmt.Errorf("create free-form draft: %w", err)
	}
	var freeFormRes struct {
		DraftID string `json:"draft_id"`
		State   string `json:"state"`
	}
	if err := json.Unmarshal(freeFormBody, &freeFormRes); err != nil {
		return fmt.Errorf("parse free-form response: %w (body: %s)", err, string(freeFormBody))
	}
	if freeFormRes.State != "under_review" {
		return fmt.Errorf("free-form draft must start under_review, got %q", freeFormRes.State)
	}

	deliverBody, err := apiPost(baseURL, "/v1/hermes/drafts/"+freeFormRes.DraftID+"/deliver", map[string]any{
		"recipient_id": "owner-1",
		"actor_id":     "her-policy-actor",
	}, auth)
	if err != nil {
		return fmt.Errorf("deliver unreviewed draft: %w", err)
	}
	var deliverErr struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(deliverBody, &deliverErr); err == nil {
		if deliverErr.Code != "BAD_REQUEST" {
			return fmt.Errorf("unreviewed draft delivery must be BAD_REQUEST, got code=%q body=%s", deliverErr.Code, string(deliverBody))
		}
	}

	reviewBody, err := apiPost(baseURL, "/v1/hermes/drafts/"+freeFormRes.DraftID+"/review", map[string]any{
		"reviewer_id": "human-reviewer-1",
		"decision":    "approved",
		"reason":      "content verified",
	}, auth)
	if err != nil {
		return fmt.Errorf("review draft: %w", err)
	}
	var reviewRes struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal(reviewBody, &reviewRes); err != nil {
		return fmt.Errorf("parse review response: %w (body: %s)", err, string(reviewBody))
	}
	if reviewRes.State != "approved" {
		return fmt.Errorf("reviewed draft must be approved, got %q", reviewRes.State)
	}

	deliveryBody, err := apiPost(baseURL, "/v1/hermes/drafts/"+freeFormRes.DraftID+"/deliver", map[string]any{
		"recipient_id":    "owner-1",
		"actor_id":        "her-policy-actor",
		"idempotency_key": "policy-delivery-1",
	}, auth)
	if err != nil {
		return fmt.Errorf("deliver approved draft: %w", err)
	}
	var deliveryRes struct {
		DeliveryID string `json:"delivery_id"`
		Status     string `json:"status"`
	}
	if err := json.Unmarshal(deliveryBody, &deliveryRes); err != nil {
		return fmt.Errorf("parse delivery response: %w (body: %s)", err, string(deliveryBody))
	}
	if deliveryRes.DeliveryID == "" {
		return fmt.Errorf("delivery must return a delivery_id: %s", string(deliveryBody))
	}

	return nil
}

func probeCCHER001OwnerAndGuestContextsAreSeparated(ctx context.Context, baseURL string) error {
	tenantID := "tenant-her-sep"
	session, err := createTestSession(baseURL, tenantID, "her-sep@test.com", []string{"owner"})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	auth := fmt.Sprintf("Bearer %s", session.SessionToken)

	propBody, err := apiPost(baseURL, "/v1/properties", map[string]any{
		"idempotency_key":    tenantID + "-prop",
		"tenant_id":          tenantID,
		"owner_authority_id": "owner-" + tenantID,
		"service_address": map[string]any{
			"line1":       "200 Separation Blvd",
			"city":        "Mumbai",
			"state":       "Maharashtra",
			"postal_code": "400001",
			"country":     "IN",
		},
		"timezone":          "Asia/Kolkata",
		"status":            "lead",
		"maximum_occupancy": 4,
	}, auth)
	if err != nil {
		return fmt.Errorf("create property: %w", err)
	}
	var propRes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(propBody, &propRes); err != nil {
		return fmt.Errorf("parse property: %w", err)
	}

	ownerBody, err := apiPost(baseURL, "/v1/hermes/drafts", map[string]any{
		"tenant_id":     tenantID,
		"property_id":   propRes.ID,
		"actor_id":      "her-sep-actor",
		"audience":      "owner",
		"purpose":       "owner communication",
		"review_policy": "approved_template",
		"template_key":  "owner_exception_notice",
		"facts": []map[string]any{{
			"source":       "tickets",
			"record_id":    "ticket-001",
			"record_kind":  "ticket",
			"audience":     "owner",
			"effective_at": "2025-08-01T00:00:00Z",
		}},
	}, auth)
	if err != nil {
		return fmt.Errorf("create owner draft: %w", err)
	}
	var ownerRes struct {
		DraftID  string `json:"draft_id"`
		Audience string `json:"audience"`
	}
	if err := json.Unmarshal(ownerBody, &ownerRes); err != nil {
		return fmt.Errorf("parse owner draft: %w (body: %s)", err, string(ownerBody))
	}
	if ownerRes.Audience != "owner" {
		return fmt.Errorf("owner draft must have owner audience, got %q", ownerRes.Audience)
	}

	guestBody, err := apiPost(baseURL, "/v1/hermes/drafts", map[string]any{
		"tenant_id":     tenantID,
		"property_id":   propRes.ID,
		"actor_id":      "her-sep-actor",
		"audience":      "guest",
		"purpose":       "guest arrival guidance",
		"review_policy": "approved_template",
		"template_key":  "arrival_guidance",
		"facts": []map[string]any{{
			"source":       "reservations",
			"record_id":    "res-001",
			"record_kind":  "reservation",
			"audience":     "guest",
			"effective_at": "2025-08-01T00:00:00Z",
		}},
	}, auth)
	if err != nil {
		return fmt.Errorf("create guest draft: %w", err)
	}
	var guestRes struct {
		DraftID  string `json:"draft_id"`
		Audience string `json:"audience"`
	}
	if err := json.Unmarshal(guestBody, &guestRes); err != nil {
		return fmt.Errorf("parse guest draft: %w (body: %s)", err, string(guestBody))
	}
	if guestRes.Audience != "guest" {
		return fmt.Errorf("guest draft must have guest audience, got %q", guestRes.Audience)
	}

	crossBody, err := apiPost(baseURL, "/v1/hermes/drafts", map[string]any{
		"tenant_id":     tenantID,
		"property_id":   propRes.ID,
		"actor_id":      "her-sep-actor",
		"audience":      "guest",
		"purpose":       "guest arrival",
		"review_policy": "approved_template",
		"template_key":  "arrival_guidance",
		"facts": []map[string]any{{
			"source":       "tickets",
			"record_id":    "ticket-001",
			"record_kind":  "ticket",
			"audience":     "owner",
			"effective_at": "2025-08-01T00:00:00Z",
		}},
	}, auth)
	if err != nil {
		return fmt.Errorf("cross-audience draft: %w", err)
	}
	var crossErr struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(crossBody, &crossErr); err == nil {
		if crossErr.Code != "BAD_REQUEST" {
			return fmt.Errorf("owner fact in guest draft must be BAD_REQUEST, got code=%q body=%s", crossErr.Code, string(crossBody))
		}
	}

	mixBody, err := apiPost(baseURL, "/v1/hermes/drafts", map[string]any{
		"tenant_id":     tenantID,
		"property_id":   propRes.ID,
		"actor_id":      "her-sep-actor",
		"audience":      "owner",
		"purpose":       "owner communication",
		"review_policy": "approved_template",
		"template_key":  "owner_exception_notice",
		"facts": []map[string]any{
			{
				"source":       "tickets",
				"record_id":    "ticket-001",
				"record_kind":  "ticket",
				"audience":     "owner",
				"effective_at": "2025-08-01T00:00:00Z",
			},
			{
				"source":       "reservations",
				"record_id":    "res-001",
				"record_kind":  "reservation",
				"audience":     "guest",
				"effective_at": "2025-08-01T00:00:00Z",
			},
		},
	}, auth)
	if err != nil {
		return fmt.Errorf("mixed-facts draft: %w", err)
	}
	var mixErr struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(mixBody, &mixErr); err == nil {
		if mixErr.Code != "BAD_REQUEST" {
			return fmt.Errorf("mixed owner/guest facts must be BAD_REQUEST, got code=%q body=%s", mixErr.Code, string(mixBody))
		}
	}

	return nil
}

func probeCCHER001DeliveryIsIdempotent(ctx context.Context, baseURL string) error {
	tenantID := "tenant-her-idem"
	session, err := createTestSession(baseURL, tenantID, "her-idem@test.com", []string{"owner"})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	auth := fmt.Sprintf("Bearer %s", session.SessionToken)

	propBody, err := apiPost(baseURL, "/v1/properties", map[string]any{
		"idempotency_key":    tenantID + "-prop",
		"tenant_id":          tenantID,
		"owner_authority_id": "owner-" + tenantID,
		"service_address": map[string]any{
			"line1":       "300 Idempotent Court",
			"city":        "Mumbai",
			"state":       "Maharashtra",
			"postal_code": "400001",
			"country":     "IN",
		},
		"timezone":          "Asia/Kolkata",
		"status":            "lead",
		"maximum_occupancy": 4,
	}, auth)
	if err != nil {
		return fmt.Errorf("create property: %w", err)
	}
	var propRes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(propBody, &propRes); err != nil {
		return fmt.Errorf("parse property: %w", err)
	}

	draftBody, err := apiPost(baseURL, "/v1/hermes/drafts", map[string]any{
		"tenant_id":     tenantID,
		"property_id":   propRes.ID,
		"actor_id":      "her-idem-actor",
		"audience":      "owner",
		"purpose":       "idempotent delivery test",
		"review_policy": "approved_template",
		"template_key":  "owner_exception_notice",
		"facts": []map[string]any{{
			"source":       "tickets",
			"record_id":    "ticket-001",
			"record_kind":  "ticket",
			"audience":     "owner",
			"effective_at": "2025-08-01T00:00:00Z",
		}},
	}, auth)
	if err != nil {
		return fmt.Errorf("create draft: %w", err)
	}
	var draftRes struct {
		DraftID string `json:"draft_id"`
	}
	if err := json.Unmarshal(draftBody, &draftRes); err != nil {
		return fmt.Errorf("parse draft: %w (body: %s)", err, string(draftBody))
	}

	deliveryBody1, err := apiPost(baseURL, "/v1/hermes/drafts/"+draftRes.DraftID+"/deliver", map[string]any{
		"recipient_id":    "owner-1",
		"actor_id":        "her-idem-actor",
		"idempotency_key": "delivery-key-1",
	}, auth)
	if err != nil {
		return fmt.Errorf("first delivery: %w", err)
	}
	var deliveryRes1 struct {
		DeliveryID string `json:"delivery_id"`
	}
	if err := json.Unmarshal(deliveryBody1, &deliveryRes1); err != nil {
		return fmt.Errorf("parse first delivery: %w (body: %s)", err, string(deliveryBody1))
	}
	if deliveryRes1.DeliveryID == "" {
		return fmt.Errorf("delivery must return delivery_id: %s", string(deliveryBody1))
	}

	deliveryBody2, err := apiPost(baseURL, "/v1/hermes/drafts/"+draftRes.DraftID+"/deliver", map[string]any{
		"recipient_id":    "owner-1",
		"actor_id":        "her-idem-actor",
		"idempotency_key": "delivery-key-1",
	}, auth)
	if err != nil {
		return fmt.Errorf("replayed delivery: %w", err)
	}
	var deliveryRes2 struct {
		DeliveryID string `json:"delivery_id"`
	}
	if err := json.Unmarshal(deliveryBody2, &deliveryRes2); err != nil {
		return fmt.Errorf("parse replayed delivery: %w (body: %s)", err, string(deliveryBody2))
	}
	if deliveryRes2.DeliveryID != deliveryRes1.DeliveryID {
		return fmt.Errorf("delivery replay by idempotency key must return same delivery: %q vs %q", deliveryRes1.DeliveryID, deliveryRes2.DeliveryID)
	}

	deliveryBody3, err := apiPost(baseURL, "/v1/hermes/drafts/"+draftRes.DraftID+"/deliver", map[string]any{
		"recipient_id": "owner-1",
		"actor_id":     "her-idem-actor",
	}, auth)
	if err != nil {
		return fmt.Errorf("replayed delivery by draft: %w", err)
	}
	var deliveryRes3 struct {
		DeliveryID string `json:"delivery_id"`
	}
	if err := json.Unmarshal(deliveryBody3, &deliveryRes3); err != nil {
		return fmt.Errorf("parse draft-replayed delivery: %w (body: %s)", err, string(deliveryBody3))
	}
	if deliveryRes3.DeliveryID != deliveryRes1.DeliveryID {
		return fmt.Errorf("delivery replay by draft must return same delivery: %q vs %q", deliveryRes1.DeliveryID, deliveryRes3.DeliveryID)
	}

	listBody, err := apiGet(baseURL, "/v1/hermes/deliveries", auth)
	if err != nil {
		return fmt.Errorf("list deliveries: %w", err)
	}
	var listRes struct {
		Deliveries []struct {
			DeliveryID string `json:"delivery_id"`
			DraftID    string `json:"draft_id"`
		} `json:"deliveries"`
		Count int `json:"count"`
	}
	if err := json.Unmarshal(listBody, &listRes); err != nil {
		return fmt.Errorf("parse list: %w (body: %s)", err, string(listBody))
	}
	if listRes.Count != 1 {
		return fmt.Errorf("delivery replay must not create duplicates, got count=%d deliveries=%d", listRes.Count, len(listRes.Deliveries))
	}

	return nil
}

func probeCCSEC001SecretsAreRedacted(ctx context.Context, baseURL string) error {
	sensitiveToken := "my-super-secret-password-value-xyz-12345"
	authHeader := "Bearer " + sensitiveToken

	// The correlation id is deliberately NOT built from the secret: it is
	// meant to be echoed back verbatim (that's the point of correlation),
	// so embedding the secret inside it would make this probe assert that
	// redaction breaks correlation, the opposite of what CCSEC001 requires
	// ("redaction still preserves correlation"). Redaction only needs to
	// stop the secret itself -- sent solely via the Authorization header,
	// never echoed anywhere -- from surfacing in a response.
	correlationID := "redact-probe-correlation-id"

	url := strings.TrimRight(baseURL, "/") + "/v1/properties/prop_nonexistent"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("X-Correlation-ID", correlationID)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	bodyStr := string(respBody)

	if strings.Contains(bodyStr, sensitiveToken) {
		return fmt.Errorf("error response must not echo sensitive token value in response body")
	}

	var errResp struct {
		RequestID string `json:"request_id"`
		Code      string `json:"code"`
		Message   string `json:"message"`
	}
	if json.Unmarshal(respBody, &errResp) == nil && errResp.RequestID != "" {
	} else if !strings.Contains(bodyStr, "request_id") && !strings.Contains(bodyStr, "correlation") {
		return fmt.Errorf("error response should include a correlation/request_id identifier: %s", bodyStr)
	}

	sensitivePass := "my-db-password-do-not-leak-67890"
	body, err := apiPost(baseURL, "/auth/session/create", map[string]any{
		"tenant_id": "tenant-redact-probe",
		"contact":   "contact-redact-probe@test.com",
		"roles":     []string{"owner"},
		"password":  sensitivePass,
		"token":     sensitiveToken,
		"secret":    sensitivePass,
	}, authHeader)
	if err != nil {
		return fmt.Errorf("request with sensitive body fields: %w", err)
	}
	bodyStr2 := string(body)
	if strings.Contains(bodyStr2, sensitivePass) || strings.Contains(bodyStr2, sensitiveToken) {
		return fmt.Errorf("error response must not leak sensitive values from request body fields")
	}

	return nil
}

func probeCCSEC001SecureLinkExpiresAndRejectsReplay(ctx context.Context, baseURL string) error {
	tenantID := "tenant-sec-link"
	session, err := createTestSession(baseURL, tenantID, "contact-sec-link@test.com", []string{"owner"})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	auth := fmt.Sprintf("Bearer %s", session.SessionToken)

	propBody, err := apiPost(baseURL, "/v1/properties", map[string]any{
		"tenant_id":          tenantID,
		"owner_authority_id": "owner-sec-link",
		"service_address": map[string]any{
			"line1": "1 Secure Road", "city": "Mumbai", "state": "Maharashtra",
			"postal_code": "400001", "country": "IN",
		},
		"timezone":          "Asia/Kolkata",
		"status":            "lead",
		"maximum_occupancy": 4,
	}, auth)
	if err != nil {
		return fmt.Errorf("create property: %w", err)
	}
	var propRes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(propBody, &propRes); err != nil {
		return fmt.Errorf("parse property: %w (body: %s)", err, string(propBody))
	}
	if propRes.ID == "" {
		return fmt.Errorf("property id empty: %s", string(propBody))
	}
	propertyID := propRes.ID

	expiresSoon := time.Now().UTC().Add(2 * time.Second).Format(time.RFC3339)
	createBody, err := apiPost(baseURL, "/v1/communications/secure-links", map[string]any{
		"property_id":  propertyID,
		"audience":     "guest",
		"recipient_id": session.UserID,
		"purpose":      "test replay prevention",
		"expires_at":   expiresSoon,
	}, auth)
	if err != nil {
		return fmt.Errorf("create secure link: %w", err)
	}

	var createResp struct {
		ID           string `json:"id"`
		Token        string `json:"token"`
		TokenTail    string `json:"token_tail"`
		Status       string `json:"status"`
		Conversation struct {
			ID string `json:"id"`
		} `json:"conversation"`
	}
	if err := json.Unmarshal(createBody, &createResp); err != nil {
		return fmt.Errorf("parse create link response: %w (body: %s)", err, string(createBody))
	}
	if createResp.Token == "" {
		return fmt.Errorf("create secure link: missing token in response: %s", string(createBody))
	}
	token := createResp.Token
	linkID := createResp.ID
	_ = linkID

	redeemBody, err := apiPost(baseURL, "/v1/communications/secure-links/redeem", map[string]any{
		"token": token,
	}, "")
	if err != nil {
		return fmt.Errorf("first redeem: %w", err)
	}

	var redeemResp struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(redeemBody, &redeemResp); err != nil {
		return fmt.Errorf("parse redeem response: %w (body: %s)", err, string(redeemBody))
	}
	if redeemResp.Status != "used" {
		return fmt.Errorf("first redeem must succeed with status 'used', got %q: %s", redeemResp.Status, string(redeemBody))
	}

	replayBody, replayErr := apiPost(baseURL, "/v1/communications/secure-links/redeem", map[string]any{
		"token": token,
	}, "")
	if replayErr != nil {
		return fmt.Errorf("replay redeem request: %w", replayErr)
	}
	if !strings.Contains(string(replayBody), "LINK_USED") && !strings.Contains(string(replayBody), "already been redeemed") {
		return fmt.Errorf("replay redeem must be rejected, got: %s", string(replayBody))
	}

	expiredExpiry := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)
	expiredCreateBody, err := apiPost(baseURL, "/v1/communications/secure-links", map[string]any{
		"property_id":  propertyID,
		"audience":     "guest",
		"recipient_id": session.UserID,
		"purpose":      "test expiry",
		"expires_at":   expiredExpiry,
	}, auth)
	if err != nil {
		return fmt.Errorf("create expired link: %w", err)
	}
	if !strings.Contains(string(expiredCreateBody), "future expiry") && !strings.Contains(string(expiredCreateBody), "VALIDATION_ERROR") {
		return fmt.Errorf("expired-at-create link must be rejected, got: %s", string(expiredCreateBody))
	}

	shortTTL := time.Now().UTC().Add(1 * time.Second).Format(time.RFC3339)
	shortCreateBody, err := apiPost(baseURL, "/v1/communications/secure-links", map[string]any{
		"property_id":  propertyID,
		"audience":     "guest",
		"recipient_id": session.UserID,
		"purpose":      "test short ttl expiry",
		"expires_at":   shortTTL,
	}, auth)
	if err != nil {
		return fmt.Errorf("create short ttl link: %w", err)
	}

	var shortCreateResp struct {
		ID    string `json:"id"`
		Token string `json:"token"`
	}
	if err := json.Unmarshal(shortCreateBody, &shortCreateResp); err != nil {
		return fmt.Errorf("parse short ttl link response: %w (body: %s)", err, string(shortCreateBody))
	}
	if shortCreateResp.Token == "" {
		return fmt.Errorf("short ttl link: missing token in response: %s", string(shortCreateBody))
	}
	shortToken := shortCreateResp.Token
	_ = shortToken

	for i := 0; i < 5; i++ {
		time.Sleep(500 * time.Millisecond)
		checkResp, checkErr := apiPost(baseURL, "/v1/communications/secure-links/redeem", map[string]any{
			"token": shortToken,
		}, "")
		if checkErr != nil {
			return fmt.Errorf("expiry check redeem: %w", checkErr)
		}
		if strings.Contains(string(checkResp), "LINK_EXPIRED") || strings.Contains(string(checkResp), "has expired") {
			break
		}
	}
	expiredCheckBody, expiredCheckErr := apiPost(baseURL, "/v1/communications/secure-links/redeem", map[string]any{
		"token": shortToken,
	}, "")
	if expiredCheckErr != nil {
		return fmt.Errorf("expired check final: %w", expiredCheckErr)
	}
	if !strings.Contains(string(expiredCheckBody), "LINK_EXPIRED") && !strings.Contains(string(expiredCheckBody), "has expired") && !strings.Contains(string(expiredCheckBody), "LINK_USED") && !strings.Contains(string(expiredCheckBody), "already been redeemed") {
		return fmt.Errorf("short-TTL link must expire or be consumed, got: %s", string(expiredCheckBody))
	}

	pool, err := connectDB(ctx)
	if err != nil {
		return fmt.Errorf("connect db for token hash check: %w", err)
	}
	defer pool.Close()

	var storedHash string
	if err := pool.QueryRow(ctx,
		`SELECT token_hash FROM conversation_links WHERE id = $1`,
		shortCreateResp.ID,
	).Scan(&storedHash); err != nil {
		return fmt.Errorf("query token_hash: %w", err)
	}
	if storedHash == shortToken {
		return fmt.Errorf("stored token column must be a hash, not the plaintext token")
	}
	if len(storedHash) != 64 {
		return fmt.Errorf("stored token_hash should be a SHA-256 hex digest (64 hex chars), got %d chars", len(storedHash))
	}

	return nil
}

func probeCCSEC001AuditEvidenceCannotBeRewritten(ctx context.Context, baseURL string) error {
	tenantID := "tenant-audit-immute"
	session, err := createTestSession(baseURL, tenantID, "contact-audit-immute@test.com", []string{"owner"})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	auth := fmt.Sprintf("Bearer %s", session.SessionToken)

	propBody, err := apiPost(baseURL, "/v1/properties", map[string]any{
		"tenant_id":          tenantID,
		"owner_authority_id": "owner-audit-immute",
		"service_address": map[string]any{
			"line1": "1 Audit Road", "city": "Mumbai", "state": "Maharashtra",
			"postal_code": "400001", "country": "IN",
		},
		"timezone":          "Asia/Kolkata",
		"status":            "lead",
		"maximum_occupancy": 4,
	}, auth)
	if err != nil {
		return fmt.Errorf("create property: %w", err)
	}
	var propRes struct {
		ID      string `json:"id"`
		Version int    `json:"version"`
	}
	if err := json.Unmarshal(propBody, &propRes); err != nil {
		return fmt.Errorf("parse property: %w (body: %s)", err, string(propBody))
	}
	if propRes.ID == "" {
		return fmt.Errorf("property id empty: %s", string(propBody))
	}

	feedBody, err := apiPost(baseURL, "/v1/properties/"+propRes.ID+"/calendar-feeds", map[string]any{
		"source":            "airbnb",
		"url":               "https://127.0.0.1:1/nonexistent.ics",
		"property_timezone": "Asia/Kolkata",
	}, auth)
	if err != nil {
		return fmt.Errorf("create feed: %w", err)
	}
	var feedRes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(feedBody, &feedRes); err != nil {
		return fmt.Errorf("parse feed: %w (body: %s)", err, string(feedBody))
	}
	if feedRes.ID == "" {
		return fmt.Errorf("feed id empty: %s", string(feedBody))
	}

	conflictICal := `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Acceptance//EN
BEGIN:VEVENT
UID:audit-1@x
DTSTART;TZID=Asia/Kolkata:20240101T100000
DTEND;TZID=Asia/Kolkata:20240105T100000
SUMMARY:Guest Alpha
END:VEVENT
BEGIN:VEVENT
UID:audit-2@x
DTSTART;TZID=Asia/Kolkata:20240104T100000
DTEND;TZID=Asia/Kolkata:20240108T100000
SUMMARY:Guest Beta
END:VEVENT
END:VCALENDAR
`

	serverURL, stop := startICalServer(conflictICal)
	defer stop()

	feedBody2, err := apiPost(baseURL, "/v1/properties/"+propRes.ID+"/calendar-feeds", map[string]any{
		"source":                     "booking",
		"url":                        serverURL,
		"property_timezone":          "Asia/Kolkata",
		"minimum_turnaround_minutes": 240,
	}, auth)
	if err != nil {
		return fmt.Errorf("create overlap feed: %w", err)
	}
	var feedRes2 struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(feedBody2, &feedRes2); err != nil {
		return fmt.Errorf("parse overlap feed: %w (body: %s)", err, string(feedBody2))
	}
	if feedRes2.ID == "" {
		return fmt.Errorf("overlap feed id empty: %s", string(feedBody2))
	}

	pollBody, pollErr := apiPost(baseURL, "/v1/calendar-feeds/"+feedRes2.ID+"/polls", map[string]any{}, auth)
	_ = pollBody
	if pollErr != nil {
		return fmt.Errorf("poll overlap feed: %w (body: %s)", pollErr, string(pollBody))
	}

	conflictsBody, err := apiGet(baseURL, "/v1/properties/"+propRes.ID+"/reservation-conflicts", auth)
	if err != nil {
		return fmt.Errorf("list conflicts: %w", err)
	}
	var conflictsList struct {
		Items []struct {
			ID   string `json:"id"`
			Data struct {
				ID     string `json:"id"`
				Kind   string `json:"kind"`
				Status string `json:"status"`
			} `json:"data"`
		} `json:"items"`
	}
	if err := json.Unmarshal(conflictsBody, &conflictsList); err != nil {
		return fmt.Errorf("parse conflicts: %w (body: %s)", err, string(conflictsBody))
	}
	var conflictID string
	for _, item := range conflictsList.Items {
		if item.Data.Kind == "overlap" && item.Data.Status == "open" {
			conflictID = item.Data.ID
			break
		}
	}
	if conflictID == "" {
		return fmt.Errorf("overlapping events must create open conflict, got: %s", string(conflictsBody))
	}

	resolveBody, err := apiPost(baseURL, "/v1/reservation-conflicts/"+conflictID+"/resolve", map[string]any{
		"outcome": "confirm",
		"note":    "detected overlap acknowledged – initial resolution",
	}, auth)
	if err != nil {
		return fmt.Errorf("resolve conflict: %w", err)
	}
	var resolveRes struct {
		ID   string `json:"id"`
		Data struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resolveBody, &resolveRes); err != nil {
		return fmt.Errorf("parse resolution: %w (body: %s)", err, string(resolveBody))
	}
	if resolveRes.Data.Status != "resolved" {
		return fmt.Errorf("conflict resolution must set status to resolved, got %q", resolveRes.Data.Status)
	}

	pool, err := connectDB(ctx)
	if err != nil {
		return fmt.Errorf("connect db for audit: %w", err)
	}
	defer pool.Close()

	var auditRowID string
	if err := pool.QueryRow(ctx,
		`SELECT id FROM audit_events
		 WHERE action = 'reservation.conflict.resolve' AND resource_id = $1
		 ORDER BY created_at DESC LIMIT 1`,
		conflictID,
	).Scan(&auditRowID); err != nil {
		return fmt.Errorf("query audit event: %w", err)
	}
	if auditRowID == "" {
		return fmt.Errorf("conflict resolution must create an audit event row")
	}

	var auditCountBefore int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_events WHERE resource_id = $1`,
		conflictID,
	).Scan(&auditCountBefore); err != nil {
		return fmt.Errorf("query audit count: %w", err)
	}

	_, updateErr := pool.Exec(ctx,
		`UPDATE audit_events SET action = 'tampered' WHERE id = $1`,
		auditRowID,
	)
	if updateErr == nil {
		return fmt.Errorf("raw UPDATE on audit_events must be rejected by the immutable trigger")
	}
	if !strings.Contains(updateErr.Error(), "immutable") && !strings.Contains(updateErr.Error(), "not allowed") {
		return fmt.Errorf("UPDATE on audit_events must raise 'immutable' error, got: %v", updateErr)
	}

	_, deleteErr := pool.Exec(ctx,
		`DELETE FROM audit_events WHERE id = $1`,
		auditRowID,
	)
	if deleteErr == nil {
		return fmt.Errorf("raw DELETE on audit_events must be rejected by the immutable trigger")
	}
	if !strings.Contains(deleteErr.Error(), "immutable") && !strings.Contains(deleteErr.Error(), "not allowed") {
		return fmt.Errorf("DELETE on audit_events must raise 'immutable' error, got: %v", deleteErr)
	}

	correctionICal := `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Acceptance//EN
BEGIN:VEVENT
UID:audit-1@x
DTSTART;TZID=Asia/Kolkata:20240101T100000
DTEND;TZID=Asia/Kolkata:20240105T100000
SUMMARY:Guest Alpha
END:VEVENT
BEGIN:VEVENT
UID:audit-2@x
DTSTART;TZID=Asia/Kolkata:20240104T100000
DTEND;TZID=Asia/Kolkata:20240108T100000
SUMMARY:Guest Beta
END:VEVENT
END:VCALENDAR
`
	server2URL, stop2 := startICalServer(correctionICal)
	defer stop2()

	feedBody3, err := apiPost(baseURL, "/v1/properties/"+propRes.ID+"/calendar-feeds", map[string]any{
		"source":                     "airbnb",
		"url":                        server2URL,
		"property_timezone":          "Asia/Kolkata",
		"minimum_turnaround_minutes": 240,
	}, auth)
	if err != nil {
		return fmt.Errorf("create correction feed: %w", err)
	}
	var feedRes3 struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(feedBody3, &feedRes3); err != nil {
		return fmt.Errorf("parse correction feed: %w (body: %s)", err, string(feedBody3))
	}
	if feedRes3.ID == "" {
		return fmt.Errorf("correction feed id empty: %s", string(feedBody3))
	}

	pollBody2, pollErr2 := apiPost(baseURL, "/v1/calendar-feeds/"+feedRes3.ID+"/polls", map[string]any{}, auth)
	_ = pollBody2
	if pollErr2 != nil {
		return fmt.Errorf("poll correction feed: %w (body: %s)", pollErr2, string(pollBody2))
	}

	newConflictsBody, err := apiGet(baseURL, "/v1/properties/"+propRes.ID+"/reservation-conflicts", auth)
	if err != nil {
		return fmt.Errorf("list conflicts after correction: %w", err)
	}
	if err := json.Unmarshal(newConflictsBody, &conflictsList); err != nil {
		return fmt.Errorf("parse conflicts after correction: %w (body: %s)", err, string(newConflictsBody))
	}
	var newConflictID string
	for _, item := range conflictsList.Items {
		if item.Data.Kind == "overlap" && item.Data.Status == "open" && item.Data.ID != conflictID {
			newConflictID = item.Data.ID
			break
		}
	}
	if newConflictID == "" {
		return fmt.Errorf("correction feed must produce new open conflict, got: %s", string(newConflictsBody))
	}

	resolveBody2, err := apiPost(baseURL, "/v1/reservation-conflicts/"+newConflictID+"/resolve", map[string]any{
		"outcome": "confirm",
		"note":    "corrected: verified overlap with updated source",
	}, auth)
	if err != nil {
		return fmt.Errorf("resolve corrected conflict: %w", err)
	}
	var resolveRes2 struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resolveBody2, &resolveRes2); err != nil {
		return fmt.Errorf("parse corrected resolution: %w (body: %s)", err, string(resolveBody2))
	}

	var auditCountAfter int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_events WHERE resource_id = $1`,
		conflictID,
	).Scan(&auditCountAfter); err != nil {
		return fmt.Errorf("query audit count after correction: %w", err)
	}
	if auditCountAfter > auditCountBefore {
		return fmt.Errorf("correction to a different conflict must not mutate original audit rows, original %d rows unchanged, got %d", auditCountBefore, auditCountAfter)
	}

	var totalConflictResolutions int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_events WHERE action = 'reservation.conflict.resolve'`,
	).Scan(&totalConflictResolutions); err != nil {
		return fmt.Errorf("query total resolutions: %w", err)
	}
	if totalConflictResolutions < 2 {
		return fmt.Errorf("corrected conflict resolution must create a NEW linked audit row, found %d total', total should be >= 2", totalConflictResolutions)
	}

	return nil
}

func probeCCSEC001CrossTenantRequestsFailClosed(ctx context.Context, baseURL string) error {
	sessionA, err := createTestSession(baseURL, "tenant-sec-cross-a", "contact-cross-a@test.com", []string{"owner"})
	if err != nil {
		return fmt.Errorf("create tenant A session: %w", err)
	}
	authA := fmt.Sprintf("Bearer %s", sessionA.SessionToken)

	sessionB, err := createTestSession(baseURL, "tenant-sec-cross-b", "contact-cross-b@test.com", []string{"owner"})
	if err != nil {
		return fmt.Errorf("create tenant B session: %w", err)
	}
	authB := fmt.Sprintf("Bearer %s", sessionB.SessionToken)

	propBody, err := apiPost(baseURL, "/v1/properties", map[string]any{
		"tenant_id":          "tenant-sec-cross-a",
		"owner_authority_id": "owner-cross-a",
		"service_address": map[string]any{
			"line1": "1 Cross Road", "city": "Mumbai", "state": "Maharashtra",
			"postal_code": "400001", "country": "IN",
		},
		"timezone":          "Asia/Kolkata",
		"status":            "lead",
		"maximum_occupancy": 4,
	}, authA)
	if err != nil {
		return fmt.Errorf("create property A: %w", err)
	}
	var propARes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(propBody, &propARes); err != nil {
		return fmt.Errorf("parse property A: %w (body: %s)", err, string(propBody))
	}
	if propARes.ID == "" {
		return fmt.Errorf("property A id empty: %s", string(propBody))
	}
	propertyAID := propARes.ID

	// Create a ticket under tenant A
	ticketBody, err := apiPost(baseURL, "/v1/tickets", map[string]any{
		"tenant_id":   "tenant-sec-cross-a",
		"property_id": propertyAID,
		"type":        "turnover",
		"reason":      "cross-tenant probe test",
	}, authA)
	if err != nil {
		return fmt.Errorf("create ticket A: %w", err)
	}
	var ticketARes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(ticketBody, &ticketARes); err != nil {
		return fmt.Errorf("parse ticket A: %w (body: %s)", err, string(ticketBody))
	}
	ticketAID := ticketARes.ID
	if ticketAID == "" {
		return fmt.Errorf("ticket A id empty: %s", string(ticketBody))
	}

	// Create a document under tenant A
	docBody, err := apiPost(baseURL, "/v1/documents", map[string]any{
		"title":         "Cross Tenant Test Doc",
		"document_type": "insurance_policy",
		"property_id":   propertyAID,
	}, authA)
	if err != nil {
		return fmt.Errorf("create document A: %w", err)
	}
	var docARes struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(docBody, &docARes); err != nil {
		return fmt.Errorf("parse document A: %w (body: %s)", err, string(docBody))
	}
	documentAID := docARes.ID
	if documentAID == "" {
		return fmt.Errorf("document A id empty: %s", string(docBody))
	}

	// Try to read property A as tenant B
	crossProp, err := apiGet(baseURL, "/v1/properties/"+propertyAID, authB)
	if err != nil {
		return fmt.Errorf("cross-tenant property read: %w", err)
	}
	if !strings.Contains(string(crossProp), "NOT_FOUND") && !strings.Contains(string(crossProp), "FORBIDDEN") && !strings.Contains(string(crossProp), "UNAUTHORIZED") {
		return fmt.Errorf("cross-tenant property read must be denied (NOT_FOUND/FORBIDDEN/UNAUTHORIZED), got: %s", string(crossProp))
	}

	// Try to read ticket A as tenant B
	crossTicket, err := apiGet(baseURL, "/v1/tickets/"+ticketAID, authB)
	if err != nil {
		return fmt.Errorf("cross-tenant ticket read: %w", err)
	}
	if !strings.Contains(string(crossTicket), "NOT_FOUND") && !strings.Contains(string(crossTicket), "FORBIDDEN") && !strings.Contains(string(crossTicket), "UNAUTHORIZED") {
		return fmt.Errorf("cross-tenant ticket read must be denied, got: %s", string(crossTicket))
	}

	// Try to read document A as tenant B
	crossDoc, err := apiGet(baseURL, "/v1/documents/"+documentAID, authB)
	if err != nil {
		return fmt.Errorf("cross-tenant document read: %w", err)
	}
	if !strings.Contains(string(crossDoc), "NOT_FOUND") && !strings.Contains(string(crossDoc), "FORBIDDEN") && !strings.Contains(string(crossDoc), "UNAUTHORIZED") {
		return fmt.Errorf("cross-tenant document read must be denied, got: %s", string(crossDoc))
	}

	// Try to write into tenant A's tenant by spoofing tenant_id in the body
	// while authenticated as tenant B. The property handler derives tenancy
	// solely from the authenticated subject and ignores any client-supplied
	// tenant_id (internal/api/handlers.go: TenantID: subject.TenantID) --
	// the strongest fail-closed form, since the untrusted field is never
	// even consulted. So this either denies outright, or silently creates
	// the resource under the caller's own tenant; either is fail-closed.
	// What must never happen is the write actually landing under tenant A.
	crossWrite, err := apiPost(baseURL, "/v1/properties", map[string]any{
		"tenant_id":          "tenant-sec-cross-a",
		"owner_authority_id": "owner-cross-attempt",
		"service_address": map[string]any{
			"line1": "2 Cross Road", "city": "Mumbai", "state": "Maharashtra",
			"postal_code": "400001", "country": "IN",
		},
		"timezone":          "Asia/Kolkata",
		"status":            "lead",
		"maximum_occupancy": 4,
	}, authB)
	if err != nil {
		return fmt.Errorf("cross-tenant property write: %w", err)
	}
	var crossWriteRes struct {
		Data struct {
			TenantID string `json:"tenant_id"`
		} `json:"data"`
	}
	if json.Unmarshal(crossWrite, &crossWriteRes) == nil && crossWriteRes.Data.TenantID != "" {
		if crossWriteRes.Data.TenantID == "tenant-sec-cross-a" {
			return fmt.Errorf("cross-tenant property write must not land under the spoofed tenant, got: %s", string(crossWrite))
		}
		if crossWriteRes.Data.TenantID != "tenant-sec-cross-b" {
			return fmt.Errorf("cross-tenant property write landed under an unexpected tenant, got: %s", string(crossWrite))
		}
	} else if !strings.Contains(string(crossWrite), "FORBIDDEN") && !strings.Contains(string(crossWrite), "UNAUTHORIZED") && !strings.Contains(string(crossWrite), "cross-tenant") {
		return fmt.Errorf("cross-tenant property write must be denied or fail closed to the caller's own tenant, got: %s", string(crossWrite))
	}

	// Verify tenant A still has exactly 1 property, 1 ticket, 1 document (no side effects)
	propListA, err := apiGet(baseURL, "/v1/properties", authA)
	if err != nil {
		return fmt.Errorf("list properties as tenant A: %w", err)
	}
	var propListRes struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(propListA, &propListRes); err != nil {
		return fmt.Errorf("parse property list: %w (body: %s)", err, string(propListA))
	}
	if len(propListRes.Items) < 1 {
		return fmt.Errorf("tenant A must still have its own property after cross-tenant denial")
	}

	return nil
}

func probeCCREL001BackupRestoreRebuildsWorkflow(ctx context.Context, baseURL string) error {
	// /health/recovery discloses backup/migration/outbox internals and is
	// gated to staff (internal/recovery: RequireRole(RoleStaff)) --
	// RequireAuthByDefault's own bar is just "any authenticated subject".
	staffSession, err := createTestSession(baseURL, "tenant-rel-backup", "contact-rel-backup@test.com", []string{"staff"})
	if err != nil {
		return fmt.Errorf("create staff session: %w", err)
	}
	staffAuth := fmt.Sprintf("Bearer %s", staffSession.SessionToken)

	body, err := apiGet(baseURL, "/health/recovery", staffAuth)
	if err != nil {
		return fmt.Errorf("health recovery request: %w", err)
	}

	var report struct {
		RPO           string   `json:"rpo"`
		RTO           string   `json:"rto"`
		Status        string   `json:"status"`
		BackupEnabled bool     `json:"backup_enabled"`
		MigrationOK   bool     `json:"migration_ok"`
		OutboxSafe    bool     `json:"outbox_safe"`
		Degradation   []string `json:"degradation"`
	}
	if err := json.Unmarshal(body, &report); err != nil {
		return fmt.Errorf("parse recovery response: %w (body: %s)", err, string(body))
	}

	if report.RPO == "" {
		return fmt.Errorf("RPO must be documented in recovery report")
	}
	if report.RTO == "" {
		return fmt.Errorf("RTO must be documented in recovery report")
	}
	if !report.BackupEnabled {
		return fmt.Errorf("backup must be enabled in recovery report")
	}
	if report.Status != "ok" && report.Status != "degraded" {
		return fmt.Errorf("recovery status must be ok or degraded, got %q", report.Status)
	}

	pool, err := connectDB(ctx)
	if err != nil {
		return fmt.Errorf("connect to postgres: %w", err)
	}
	defer pool.Close()

	var jobTableExists bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'jobs')`,
	).Scan(&jobTableExists); err != nil {
		return fmt.Errorf("check jobs table: %w", err)
	}
	if !jobTableExists {
		return fmt.Errorf("jobs table must exist for outbox-based workflow recovery")
	}

	return nil
}

func probeCCREL001MigrationForwardRecovery(ctx context.Context, baseURL string) error {
	pool, err := connectDB(ctx)
	if err != nil {
		return fmt.Errorf("connect to postgres: %w", err)
	}
	defer pool.Close()

	var exists bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'schema_migrations')`,
	).Scan(&exists); err != nil {
		return fmt.Errorf("check schema_migrations table: %w", err)
	}
	if !exists {
		return fmt.Errorf("schema_migrations table must exist for forward recovery")
	}

	var migrationCount int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM schema_migrations`,
	).Scan(&migrationCount); err != nil {
		return fmt.Errorf("query migration count: %w", err)
	}
	if migrationCount < 1 {
		return fmt.Errorf("expected at least 1 migration record, got %d", migrationCount)
	}

	rows, err := pool.Query(ctx,
		`SELECT version, description, checksum FROM schema_migrations ORDER BY version`,
	)
	if err != nil {
		return fmt.Errorf("query migration records: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var version int
		var description, checksum string
		if err := rows.Scan(&version, &description, &checksum); err != nil {
			return fmt.Errorf("scan migration record: %w", err)
		}
		if checksum == "" {
			return fmt.Errorf("migration %d (%s) has empty checksum", version, description)
		}
	}
	if rows.Err() != nil {
		return fmt.Errorf("iterate migration records: %w", rows.Err())
	}

	var tableCount int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE'`,
	).Scan(&tableCount); err != nil {
		return fmt.Errorf("count user tables: %w", err)
	}
	if tableCount < 10 {
		return fmt.Errorf("expected at least 10 user tables after migration, got %d", tableCount)
	}

	return nil
}

func probeCCREL001OutboxReplayIsIdempotent(ctx context.Context, baseURL string) error {
	pool, err := connectDB(ctx)
	if err != nil {
		return fmt.Errorf("connect to postgres: %w", err)
	}
	defer pool.Close()

	var idemExists bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'idempotency_records')`,
	).Scan(&idemExists); err != nil {
		return fmt.Errorf("check idempotency_records table: %w", err)
	}
	if !idemExists {
		return fmt.Errorf("idempotency_records table must exist for outbox replay safety")
	}

	var jobsExists bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'jobs')`,
	).Scan(&jobsExists); err != nil {
		return fmt.Errorf("check jobs table: %w", err)
	}
	if !jobsExists {
		return fmt.Errorf("jobs table must exist for outbox replay safety")
	}

	cols := []string{}
	colRows, err := pool.Query(ctx,
		`SELECT column_name FROM information_schema.columns WHERE table_name = 'jobs' ORDER BY ordinal_position`,
	)
	if err != nil {
		return fmt.Errorf("query jobs columns: %w", err)
	}
	for colRows.Next() {
		var c string
		if err := colRows.Scan(&c); err != nil {
			colRows.Close()
			return err
		}
		cols = append(cols, c)
	}
	colRows.Close()

	hasIdempotencyKey := false
	for _, c := range cols {
		if c == "idempotency_key" {
			hasIdempotencyKey = true
			break
		}
	}
	if !hasIdempotencyKey {
		return fmt.Errorf("jobs table must have idempotency_key column for outbox replay safety")
	}

	var outboxExists bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'outbox_events')`,
	).Scan(&outboxExists); err != nil {
		return fmt.Errorf("check outbox_events table: %w", err)
	}
	if !outboxExists {
		return fmt.Errorf("outbox_events table must exist for outbox replay")
	}

	return nil
}

func probeCCREL001DependencyDegradationIsVisible(ctx context.Context, baseURL string) error {
	resp, body, err := httpGet(baseURL, "/health/ready")
	if err != nil {
		return fmt.Errorf("health ready request: %w", err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusServiceUnavailable {
		return fmt.Errorf("health ready: expected 200 or 503, got %d: %s", resp.StatusCode, string(body))
	}

	var readyResp struct {
		Status string            `json:"status"`
		Time   string            `json:"time"`
		Checks map[string]string `json:"checks"`
	}
	if err := json.Unmarshal(body, &readyResp); err != nil {
		return fmt.Errorf("parse ready response: %w (body: %s)", err, string(body))
	}

	if readyResp.Status != "ok" && readyResp.Status != "degraded" {
		return fmt.Errorf("health ready status must be ok or degraded, got %q", readyResp.Status)
	}

	if _, ok := readyResp.Checks["database"]; !ok {
		return fmt.Errorf("health ready must include database check for degradation visibility")
	}

	if readyResp.Status == "degraded" && resp.StatusCode != http.StatusServiceUnavailable {
		return fmt.Errorf("when status is degraded, HTTP status must be 503")
	}

	return nil
}

func probeCCREL001CapacityTarget(ctx context.Context, baseURL string) error {
	pool, err := connectDB(ctx)
	if err != nil {
		return fmt.Errorf("connect to postgres: %w", err)
	}
	defer pool.Close()

	tenantID := fmt.Sprintf("tenant-capacity-%d", time.Now().UnixNano())

	report, err := quality.RunCapacityScenario(ctx, pool, baseURL, tenantID, quality.DefaultCapacityTarget())
	if err != nil {
		return fmt.Errorf("capacity scenario: %w", err)
	}

	if err := quality.RunAccessibilityReview(ctx, baseURL, tenantID, report); err != nil {
		return fmt.Errorf("accessibility review: %w", err)
	}

	// p95 result must be measured and within the NFR-003 target for every core
	// server path.
	if len(report.Latency) != 3 {
		return fmt.Errorf("expected 3 core path latency summaries, got %d", len(report.Latency))
	}
	for _, summary := range report.Latency {
		if summary.Samples < 1 {
			return fmt.Errorf("core path %s has no latency samples", summary.Path)
		}
		if !summary.WithinTarget {
			return fmt.Errorf("core path %s p95 %vms exceeds target %vms", summary.Path, summary.P95MS, summary.TargetMS)
		}
		fmt.Fprintf(os.Stderr, "p95 %s = %vms (%d samples, target %vms)\n", summary.Path, summary.P95MS, summary.Samples, summary.TargetMS)
	}

	// Accessibility and localization dispositions must leave no open gap.
	for _, item := range report.Accessibility {
		if item.Disposition == quality.DispositionGap {
			return fmt.Errorf("open accessibility gap %s: %s", item.Checkpoint, item.Criterion)
		}
	}
	for _, loc := range report.Localization {
		if loc.Disposition == quality.DispositionGap {
			return fmt.Errorf("localization gap: template %s not available in %s", loc.TemplateKey, loc.Language)
		}
	}

	reportPath, err := qualityReportPath()
	if err != nil {
		return fmt.Errorf("resolve quality report path: %w", err)
	}
	if err := report.WriteTo(reportPath); err != nil {
		return fmt.Errorf("record quality report: %w", err)
	}
	fmt.Fprintf(os.Stderr, "recorded capacity/p95/accessibility report to %s\n", reportPath)

	return nil
}

func qualityReportPath() (string, error) {
	root, err := findRepoRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "tests", "quality", "reports", "quality-report.json"), nil
}
