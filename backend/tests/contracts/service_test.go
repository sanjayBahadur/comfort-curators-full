package contracts_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"comfort-curators-backend/internal/contracts"
	"comfort-curators-backend/internal/platform/audit"
	"comfort-curators-backend/internal/platform/testdb"

	"github.com/jackc/pgx/v5/pgxpool"
)

type testAuthorizer struct {
	tenant string
	deny   bool
}

func (a testAuthorizer) RequireResourceAccess(ctx context.Context, resourceTenantID, resourceType, resourceID string) error {
	if a.deny {
		return errors.New("denied")
	}
	if a.tenant != "" && a.tenant != resourceTenantID {
		return errors.New("cross-tenant access denied")
	}
	return nil
}

func contractsPostgresAvailable() bool {
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

func contractsDBConnString() string {
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
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, pass, host, port, name)
}

func contractsPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if !contractsPostgresAvailable() {
		t.Skip("PostgreSQL not available for contracts integration test")
	}
	pool, err := pgxpool.New(context.Background(), contractsDBConnString())
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(context.Background()); err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}
	return pool
}

func contractsService(pool *pgxpool.Pool, tenantID string) *contracts.Service {
	auditStore := audit.NewAuditStore(pool)
	return contracts.NewService(pool, auditStore).WithAuthorizer(testAuthorizer{tenant: tenantID})
}

func ensureContractSchema(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	if err := contracts.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure contracts schema: %v", err)
	}
	if err := audit.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure audit schema: %v", err)
	}
}

func contractTerms() []byte {
	return []byte(`{"scope":{"tier":"full_service","units":3},"fee":{"percentage_basis_points":1800,"minimum_monthly_minor_units":60000000}}`)
}

func sampleRule() contracts.FeeRule {
	return contracts.FeeRule{
		Version:                     "2026-07-01",
		Currency:                    "INR",
		ServiceTier:                 contracts.ServiceTierFullService,
		PercentageBasisPoints:       1800,
		MinimumMonthlyFeeMinorUnits: 600_000_00,
		SetupFeeMinorUnits:          250_000_00,
		EffectiveFrom:               "2026-07-01",
	}
}

func sampleRulePtr() *contracts.FeeRule {
	r := sampleRule()
	return &r
}

func sampleQuoteInputs(tenantID string) contracts.QuoteInputs {
	return contracts.QuoteInputs{
		TenantID:                       tenantID,
		PropertyID:                     "prop-1",
		ServiceTier:                    contracts.ServiceTierFullService,
		ManagedUnits:                   3,
		Currency:                       "INR",
		RevenuePeriod:                  "2026-07",
		AccommodationRevenueMinorUnits: 5_000_000_00,
		PassThroughs: []contracts.PassThroughAmount{
			{Category: contracts.PassThroughCategoryTaxes, MinorUnits: 100_000_00},
			{Category: contracts.PassThroughCategoryCleaning, MinorUnits: 50_000_00},
			{Category: contracts.PassThroughCategoryRefundableDeposits, MinorUnits: 20_000_00},
		},
	}
}

func TestContractsQuoteIsDeterministic(t *testing.T) {
	pool := contractsPool(t)
	ensureContractSchema(t, pool)
	ctx := context.Background()
	tenantID := "tenant-contracts-quote"
	svc := contractsService(pool, tenantID)

	if err := svc.SaveFeeRule(ctx, sampleRulePtr()); err != nil {
		t.Fatalf("save fee rule: %v", err)
	}

	inputs := sampleQuoteInputs(tenantID)
	first, err := svc.Quote(ctx, tenantID, inputs, "2026-07-01")
	if err != nil {
		t.Fatalf("quote first run: %v", err)
	}
	second, err := svc.Quote(ctx, tenantID, inputs, "2026-07-01")
	if err != nil {
		t.Fatalf("quote second run: %v", err)
	}

	if first.InputHash != second.InputHash {
		t.Errorf("same inputs must produce same quote hash: %s vs %s", first.InputHash, second.InputHash)
	}
	if first.ManagementFeeMinorUnits != second.ManagementFeeMinorUnits {
		t.Errorf("same inputs must produce same management fee: %d vs %d", first.ManagementFeeMinorUnits, second.ManagementFeeMinorUnits)
	}
	if first.RuleVersion != "2026-07-01" {
		t.Errorf("quote must expose the rule version, got %q", first.RuleVersion)
	}
}

func TestContractsQuoteFeeBaseExcludesProtectedPassThroughs(t *testing.T) {
	pool := contractsPool(t)
	ensureContractSchema(t, pool)
	ctx := context.Background()
	tenantID := "tenant-contracts-fee-base"
	svc := contractsService(pool, tenantID)

	if err := svc.SaveFeeRule(ctx, sampleRulePtr()); err != nil {
		t.Fatalf("save fee rule: %v", err)
	}

	quote, err := svc.Quote(ctx, tenantID, sampleQuoteInputs(tenantID), "2026-07-01")
	if err != nil {
		t.Fatalf("quote: %v", err)
	}
	var wantBase int64 = 5_000_000_00 - 100_000_00 - 50_000_00 - 20_000_00
	if quote.FeeBase.BaseMinorUnits != wantBase {
		t.Errorf("fee base = %d, want %d (protected pass-throughs excluded)", quote.FeeBase.BaseMinorUnits, wantBase)
	}
	if len(quote.FeeBase.Exclusions) != 3 {
		t.Errorf("all protected categories must be excluded, got %+v", quote.FeeBase.Exclusions)
	}
}

func TestContractsQuoteRuleVersionChangeIsVisible(t *testing.T) {
	pool := contractsPool(t)
	ensureContractSchema(t, pool)
	ctx := context.Background()
	tenantID := "tenant-contracts-rule-version"
	svc := contractsService(pool, tenantID)

	if err := svc.SaveFeeRule(ctx, sampleRulePtr()); err != nil {
		t.Fatalf("save v1 rule: %v", err)
	}
	v2 := sampleRule()
	v2.Version = "2026-08-01"
	v2.PercentageBasisPoints = 2000
	if err := svc.SaveFeeRule(ctx, &v2); err != nil {
		t.Fatalf("save v2 rule: %v", err)
	}

	inputs := sampleQuoteInputs(tenantID)
	v1, err := svc.Quote(ctx, tenantID, inputs, "2026-07-01")
	if err != nil {
		t.Fatalf("quote v1: %v", err)
	}
	v2q, err := svc.Quote(ctx, tenantID, inputs, "2026-08-01")
	if err != nil {
		t.Fatalf("quote v2: %v", err)
	}
	if v1.InputHash == v2q.InputHash {
		t.Error("changed rule version must change the quote hash")
	}
	if v1.ManagementFeeMinorUnits == v2q.ManagementFeeMinorUnits {
		t.Error("changed percentage must change the management fee")
	}
}

func TestContractsAgreementAcceptedCannotMutate(t *testing.T) {
	pool := contractsPool(t)
	ensureContractSchema(t, pool)
	ctx := context.Background()
	tenantID := "tenant-contracts-immutable"
	svc := contractsService(pool, tenantID)

	created, err := svc.CreateAgreement(ctx, contracts.CreateAgreementParams{
		TenantID: tenantID, PropertyID: "prop-1", Terms: contractTerms(),
	}, "ops-1")
	if err != nil {
		t.Fatalf("create agreement: %v", err)
	}
	if created.Status != contracts.AgreementStatusDraft {
		t.Errorf("new agreement must be draft, got %q", created.Status)
	}

	accepted, err := svc.Accept(ctx, tenantID, created.ID, "owner-1")
	if err != nil {
		t.Fatalf("accept agreement: %v", err)
	}
	if accepted.Status != contracts.AgreementStatusAccepted {
		t.Fatalf("agreement must be accepted, got %q", accepted.Status)
	}
	if accepted.Acceptance == nil {
		t.Fatal("acceptance record must be present")
	}

	// The accepted agreement cannot mutate through the service.
	if _, err := svc.AddVersion(ctx, tenantID, created.ID, []byte(`{"scope":{}}`), "ops-1"); !errors.Is(err, contracts.ErrAcceptedImmutable) {
		t.Errorf("adding a version to an accepted agreement must fail, got %v", err)
	}

	// The accepted agreement cannot mutate through direct SQL: the schema
	// trigger rejects a new version row for an accepted contract.
	_, err = pool.Exec(ctx, `
		INSERT INTO service_contract_versions (id, agreement_id, tenant_id, version_number, content_hash, terms, created_at)
		VALUES ('direct-insert', $1, $2, 99, 'sha256:' || repeat('a', 64), '{"scope":{}}'::jsonb, NOW())
	`, created.ID, tenantID)
	if err == nil {
		t.Error("direct insert of a version into an accepted agreement must be rejected by the database")
	}
}

func TestContractsAgreementVersionRecordsAreImmutable(t *testing.T) {
	pool := contractsPool(t)
	ensureContractSchema(t, pool)
	ctx := context.Background()
	tenantID := "tenant-contracts-version-immutable"
	svc := contractsService(pool, tenantID)

	created, err := svc.CreateAgreement(ctx, contracts.CreateAgreementParams{
		TenantID: tenantID, PropertyID: "prop-1", Terms: contractTerms(),
	}, "ops-1")
	if err != nil {
		t.Fatalf("create agreement: %v", err)
	}

	// Direct UPDATE and DELETE of a version row are rejected by the trigger.
	_, err = pool.Exec(ctx, `
		UPDATE service_contract_versions SET terms = '{"changed":true}'::jsonb
		WHERE agreement_id = $1
	`, created.ID)
	if err == nil {
		t.Error("updating a version row must be rejected by the database")
	}
	_, err = pool.Exec(ctx, `DELETE FROM service_contract_versions WHERE agreement_id = $1`, created.ID)
	if err == nil {
		t.Error("deleting a version row must be rejected by the database")
	}
}

func TestContractsAgreementCorrectionCreatesNewVersion(t *testing.T) {
	pool := contractsPool(t)
	ensureContractSchema(t, pool)
	ctx := context.Background()
	tenantID := "tenant-contracts-correction"
	svc := contractsService(pool, tenantID)

	created, err := svc.CreateAgreement(ctx, contracts.CreateAgreementParams{
		TenantID: tenantID, PropertyID: "prop-1", Terms: contractTerms(),
	}, "ops-1")
	if err != nil {
		t.Fatalf("create agreement: %v", err)
	}

	corrected := []byte(`{"scope":{"tier":"full_service","units":4},"fee":{"percentage_basis_points":1800,"minimum_monthly_minor_units":60000000}}`)
	updated, err := svc.AddVersion(ctx, tenantID, created.ID, corrected, "ops-1")
	if err != nil {
		t.Fatalf("add corrected version: %v", err)
	}
	if updated.CurrentVersion != 2 {
		t.Errorf("correction must create version 2, got %d", updated.CurrentVersion)
	}

	reloaded, err := svc.GetAgreement(ctx, tenantID, created.ID)
	if err != nil {
		t.Fatalf("get agreement: %v", err)
	}
	if len(reloaded.Versions) != 2 {
		t.Fatalf("both versions must be retained, got %d", len(reloaded.Versions))
	}
	if reloaded.Versions[0].VersionNumber != 1 || reloaded.Versions[1].VersionNumber != 2 {
		t.Errorf("versions must remain sequential, got %+v", reloaded.Versions)
	}
	if reloaded.Versions[0].ContentHash == reloaded.Versions[1].ContentHash {
		t.Error("different terms must have different content hashes")
	}
}

func TestContractsAgreementAcceptancePointsToExactHash(t *testing.T) {
	pool := contractsPool(t)
	ensureContractSchema(t, pool)
	ctx := context.Background()
	tenantID := "tenant-contracts-acceptance"
	svc := contractsService(pool, tenantID)

	created, err := svc.CreateAgreement(ctx, contracts.CreateAgreementParams{
		TenantID: tenantID, PropertyID: "prop-1", Terms: contractTerms(),
	}, "ops-1")
	if err != nil {
		t.Fatalf("create agreement: %v", err)
	}

	accepted, err := svc.Accept(ctx, tenantID, created.ID, "owner-1")
	if err != nil {
		t.Fatalf("accept agreement: %v", err)
	}
	last := accepted.Versions[len(accepted.Versions)-1]
	if accepted.Acceptance.ContentHash != last.ContentHash {
		t.Errorf("acceptance must point to the exact version content hash, got %s want %s", accepted.Acceptance.ContentHash, last.ContentHash)
	}
	if accepted.Acceptance.VersionNumber != last.VersionNumber {
		t.Errorf("acceptance must point to version %d, got %d", last.VersionNumber, accepted.Acceptance.VersionNumber)
	}

	// Reload and verify acceptance survives.
	reloaded, err := svc.GetAgreement(ctx, tenantID, created.ID)
	if err != nil {
		t.Fatalf("get agreement: %v", err)
	}
	if reloaded.Acceptance == nil || reloaded.Acceptance.AcceptedBy != "owner-1" {
		t.Errorf("acceptance must be reloaded with the actor, got %+v", reloaded.Acceptance)
	}
}

func TestContractsAgreementCrossTenantDenied(t *testing.T) {
	pool := contractsPool(t)
	ensureContractSchema(t, pool)
	ctx := context.Background()
	tenantID := "tenant-contracts-scope"
	svc := contractsService(pool, tenantID)

	created, err := svc.CreateAgreement(ctx, contracts.CreateAgreementParams{
		TenantID: tenantID, PropertyID: "prop-1", Terms: contractTerms(),
	}, "ops-1")
	if err != nil {
		t.Fatalf("create agreement: %v", err)
	}

	otherSvc := contractsService(pool, "tenant-other")
	if _, err := otherSvc.GetAgreement(ctx, "tenant-other", created.ID); !errors.Is(err, contracts.ErrCrossTenantDenied) {
		t.Errorf("cross-tenant read must be denied, got %v", err)
	}
	if _, err := otherSvc.Accept(ctx, "tenant-other", created.ID, "owner-2"); !errors.Is(err, contracts.ErrCrossTenantDenied) {
		t.Errorf("cross-tenant accept must be denied, got %v", err)
	}
}

func TestContractsAgreementRejectsEmptyTerms(t *testing.T) {
	pool := contractsPool(t)
	ensureContractSchema(t, pool)
	ctx := context.Background()
	tenantID := "tenant-contracts-empty"
	svc := contractsService(pool, tenantID)

	if _, err := svc.CreateAgreement(ctx, contracts.CreateAgreementParams{
		TenantID: tenantID, PropertyID: "prop-1", Terms: nil,
	}, "ops-1"); !errors.Is(err, contracts.ErrEmptyTerms) {
		t.Errorf("empty terms must be rejected, got %v", err)
	}
}
