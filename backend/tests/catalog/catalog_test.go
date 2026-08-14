package catalog_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"comfort-curators-backend/internal/catalog"
	"comfort-curators-backend/internal/platform/audit"

	"github.com/jackc/pgx/v5/pgxpool"
)

func catalogPostgresAvailable() bool {
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

func catalogDBConnString() string {
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

type stubAuthorizer struct {
	tenantID string
}

func (a stubAuthorizer) RequireResourceAccess(ctx context.Context, resourceTenantID, resourceType, resourceID string) error {
	if a.tenantID == resourceTenantID {
		return nil
	}
	return errors.New("denied")
}

func catalogPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if !catalogPostgresAvailable() {
		t.Skip("PostgreSQL not available for catalog integration test")
	}
	pool, err := pgxpool.New(context.Background(), catalogDBConnString())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := catalog.EnsureSchema(context.Background(), pool); err != nil {
		t.Fatalf("ensure catalog schema: %v", err)
	}
	if err := audit.EnsureSchema(context.Background(), pool); err != nil {
		t.Fatalf("ensure audit schema: %v", err)
	}
	for _, table := range []string{
		"property_package_items",
		"property_package_versions",
		"package_templates",
		"catalog_claim_evidence",
		"catalog_items",
		"audit_events",
	} {
		if _, err := pool.Exec(context.Background(), "TRUNCATE "+table+" CASCADE"); err != nil {
			t.Fatalf("truncate %s: %v", table, err)
		}
	}
	return pool
}

func newCatalogServiceOnPool(t *testing.T, pool *pgxpool.Pool, tenantID string) *catalog.Service {
	t.Helper()
	return catalog.NewService(pool).
		WithAuthorizer(stubAuthorizer{tenantID: tenantID}).
		WithAudit(audit.NewAuditStore(pool))
}

func newCatalogService(t *testing.T, tenantID string) *catalog.Service {
	t.Helper()
	return newCatalogServiceOnPool(t, catalogPool(t), tenantID)
}

func int64Ptr(v int64) *int64 { return &v }

func createItem(t *testing.T, svc *catalog.Service, tenantID, sku string) *catalog.CatalogItem {
	t.Helper()
	item, err := svc.CreateCatalogItem(context.Background(), tenantID, catalog.CreateItemParams{
		SKU:                    sku,
		Name:                   "Toilet Paper " + sku,
		Category:               "paper",
		Brand:                  "BrandX",
		PackSize:               "12 rolls",
		UnitCostMinorUnits:     1200,
		UnitCostCurrency:       "INR",
		OwnerPriceMinorUnits:   2500,
		OwnerPriceCurrency:     "INR",
		TaxClass:               "gst12",
		Supplier:               "supplier-1",
		CountryOfOrigin:        "India",
		Status:                 catalog.ItemStatusActive,
		ShelfLifeRule:          "none",
		SubstitutionGroup:      "paper",
		OperationalSuitability: "indoor",
		Label:                  catalog.LabelCuratorsStandard,
	}, "actor-ops-1")
	if err != nil {
		t.Fatalf("create item %s: %v", sku, err)
	}
	return item
}

func createSoapItem(t *testing.T, svc *catalog.Service, tenantID, sku string) *catalog.CatalogItem {
	t.Helper()
	item, err := svc.CreateCatalogItem(context.Background(), tenantID, catalog.CreateItemParams{
		SKU:                    sku,
		Name:                   "Soap " + sku,
		Category:               "toiletries",
		Brand:                  "BrandY",
		PackSize:               "1 pc",
		UnitCostMinorUnits:     2000,
		UnitCostCurrency:       "INR",
		OwnerPriceMinorUnits:   4000,
		OwnerPriceCurrency:     "INR",
		TaxClass:               "gst18",
		Supplier:               "supplier-1",
		CountryOfOrigin:        "India",
		Status:                 catalog.ItemStatusActive,
		ShelfLifeRule:          "12 months",
		SubstitutionGroup:      "toiletries",
		OperationalSuitability: "indoor",
		Label:                  catalog.LabelOwnerPreferred,
	}, "actor-ops-1")
	if err != nil {
		t.Fatalf("create item %s: %v", sku, err)
	}
	return item
}

// --- Catalog item profile (CAT-001) -----------------------------------------

func TestCatalogItemProfileAndValidation(t *testing.T) {
	tenantID := "tenant-catalog-item"
	svc := newCatalogService(t, tenantID)
	ctx := context.Background()

	item, err := svc.CreateCatalogItem(ctx, tenantID, catalog.CreateItemParams{
		SKU:                    "SKU-FULL",
		Name:                   "Full profile item",
		Category:               "cleaning",
		Brand:                  "BrandZ",
		PackSize:               "500 ml",
		UnitCostMinorUnits:     900,
		UnitCostCurrency:       "INR",
		OwnerPriceMinorUnits:   1500,
		OwnerPriceCurrency:     "INR",
		TaxClass:               "gst18",
		Supplier:               "supplier-2",
		CountryOfOrigin:        "India",
		Status:                 catalog.ItemStatusActive,
		ShelfLifeRule:          "24 months",
		SubstitutionGroup:      "cleaning",
		OperationalSuitability: "kitchen",
		Label:                  catalog.LabelCuratorsStandard,
	}, "actor-ops-1")
	if err != nil {
		t.Fatalf("create item: %v", err)
	}
	if item.SKU != "SKU-FULL" || item.Name == "" || item.Category == "" || item.Brand == "" ||
		item.PackSize == "" || item.UnitCostMinorUnits != 900 || item.OwnerPriceMinorUnits != 1500 ||
		item.TaxClass == "" || item.Supplier == "" || item.CountryOfOrigin == "" ||
		item.Status != catalog.ItemStatusActive || item.ShelfLifeRule == "" ||
		item.SubstitutionGroup == "" || item.OperationalSuitability == "" ||
		item.Label != catalog.LabelCuratorsStandard {
		t.Fatalf("catalog item must carry the full CAT-001 profile, got %+v", item)
	}

	reloaded, err := svc.GetCatalogItem(ctx, tenantID, item.ID)
	if err != nil {
		t.Fatalf("get item: %v", err)
	}
	if reloaded.ID != item.ID || reloaded.SKU != item.SKU {
		t.Fatalf("reloaded item mismatch: %+v", reloaded)
	}

	listed, err := svc.ListCatalogItems(ctx, tenantID)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected one listed item, got %d", len(listed))
	}
}

func TestCatalogItemRejectsInvalidInputs(t *testing.T) {
	tenantID := "tenant-catalog-invalid"
	svc := newCatalogService(t, tenantID)
	ctx := context.Background()

	base := catalog.CreateItemParams{
		SKU:                  "SKU-V",
		Name:                 "Valid item",
		Category:             "cleaning",
		UnitCostMinorUnits:   900,
		UnitCostCurrency:     "INR",
		OwnerPriceMinorUnits: 1500,
		OwnerPriceCurrency:   "INR",
		Label:                catalog.LabelAlternative,
	}

	if _, err := svc.CreateCatalogItem(ctx, tenantID, base, "actor-ops-1"); err != nil {
		t.Fatalf("baseline item must be created: %v", err)
	}

	duplicate := base
	duplicate.SKU = "SKU-V"
	if _, err := svc.CreateCatalogItem(ctx, tenantID, duplicate, "actor-ops-1"); !errors.Is(err, catalog.ErrSKUAlreadyExists) {
		t.Fatalf("duplicate SKU must be rejected, got %v", err)
	}

	noName := base
	noName.SKU = "SKU-NONAME"
	noName.Name = ""
	if _, err := svc.CreateCatalogItem(ctx, tenantID, noName, "actor-ops-1"); !errors.Is(err, catalog.ErrInvalidItem) {
		t.Fatalf("missing name must be rejected, got %v", err)
	}

	negative := base
	negative.SKU = "SKU-NEG"
	negative.OwnerPriceMinorUnits = -1
	if _, err := svc.CreateCatalogItem(ctx, tenantID, negative, "actor-ops-1"); !errors.Is(err, catalog.ErrInvalidItem) {
		t.Fatalf("negative owner price must be rejected, got %v", err)
	}

	mixedCurrency := base
	mixedCurrency.SKU = "SKU-MIX"
	mixedCurrency.UnitCostCurrency = "USD"
	if _, err := svc.CreateCatalogItem(ctx, tenantID, mixedCurrency, "actor-ops-1"); !errors.Is(err, catalog.ErrInvalidItem) {
		t.Fatalf("mixed currencies must be rejected, got %v", err)
	}

	sponsored := base
	sponsored.SKU = "SKU-SPONSORED"
	sponsored.Label = catalog.LabelSponsored
	if _, err := svc.CreateCatalogItem(ctx, tenantID, sponsored, "actor-ops-1"); !errors.Is(err, catalog.ErrSponsoredDisabled) {
		t.Fatalf("sponsored label must be rejected while sponsored placement is disabled, got %v", err)
	}

	unknownLabel := base
	unknownLabel.SKU = "SKU-LABEL"
	unknownLabel.Label = "premium"
	if _, err := svc.CreateCatalogItem(ctx, tenantID, unknownLabel, "actor-ops-1"); !errors.Is(err, catalog.ErrInvalidLabel) {
		t.Fatalf("unknown label must be rejected, got %v", err)
	}
}

// --- Claim evidence retention (CAT-010) --------------------------------------

func TestCatalogClaimsRequireRetainedEvidence(t *testing.T) {
	tenantID := "tenant-catalog-claims"
	svc := newCatalogService(t, tenantID)
	ctx := context.Background()
	item := createItem(t, svc, tenantID, "SKU-CLAIM")

	if _, err := svc.AddClaimEvidence(ctx, tenantID, item.ID, catalog.ClaimEvidenceParams{
		ClaimType:      catalog.ClaimTypeQuality,
		ClaimStatement: "Long lasting paper",
	}, "actor-ops-1"); !errors.Is(err, catalog.ErrClaimEvidenceRequired) {
		t.Fatalf("a claim without retained evidence must be rejected, got %v", err)
	}

	if _, err := svc.AddClaimEvidence(ctx, tenantID, item.ID, catalog.ClaimEvidenceParams{
		ClaimType:   "free",
		EvidenceRef: "evidence:doc-1",
	}, "actor-ops-1"); !errors.Is(err, catalog.ErrInvalidClaimType) {
		t.Fatalf("an unknown claim type must be rejected, got %v", err)
	}

	if _, err := svc.AddClaimEvidence(ctx, tenantID, item.ID, catalog.ClaimEvidenceParams{
		ClaimType:   catalog.ClaimTypeOrigin,
		EvidenceRef: "evidence:origin-cert-001",
	}, "actor-ops-1"); err != nil {
		t.Fatalf("a claim with retained evidence must be recorded: %v", err)
	}

	evidence, err := svc.ListClaimEvidence(ctx, tenantID, item.ID)
	if err != nil {
		t.Fatalf("list claim evidence: %v", err)
	}
	if len(evidence) != 1 {
		t.Fatalf("expected one retained evidence record, got %d", len(evidence))
	}
	if evidence[0].EvidenceRef != "evidence:origin-cert-001" {
		t.Fatalf("evidence reference must be retained, got %q", evidence[0].EvidenceRef)
	}
	if evidence[0].EvidenceRetainedAt.IsZero() {
		t.Fatal("evidence retention time must be recorded")
	}
}

// --- Bundles expand into a property package (CAT-004) ------------------------

func TestCatalogBundleExpandsIntoPackageVersion(t *testing.T) {
	tenantID := "tenant-catalog-bundle"
	svc := newCatalogService(t, tenantID)
	ctx := context.Background()

	paper := createItem(t, svc, tenantID, "SKU-PAPER")
	soap := createSoapItem(t, svc, tenantID, "SKU-SOAP")

	tpl, err := svc.CreatePackageTemplate(ctx, tenantID, catalog.CreateTemplateParams{
		Name:        "Essentials starter",
		Description: "Starter bundle",
		Items: []catalog.PackageTemplateItem{
			{CatalogItemID: paper.ID, Quantity: 4},
			{CatalogItemID: soap.ID, Quantity: 2},
		},
	}, "actor-ops-1")
	if err != nil {
		t.Fatalf("create bundle template: %v", err)
	}
	if len(tpl.Items) != 2 {
		t.Fatalf("bundle must retain two items, got %d", len(tpl.Items))
	}

	version, err := svc.CreatePropertyPackageVersion(ctx, tenantID, "property-1", catalog.CreatePackageVersionParams{
		EffectiveDate:      time.Now().UTC().AddDate(0, 0, 7),
		SubstitutionPolicy: catalog.SubstitutionOwnerApproval,
		Bundles: []catalog.PackageBundleInput{
			{PackageTemplateID: tpl.ID},
		},
	}, "owner-1")
	if err != nil {
		t.Fatalf("create package version from bundle: %v", err)
	}

	if version.VersionNumber != 1 {
		t.Fatalf("first version number must be 1, got %d", version.VersionNumber)
	}
	if version.Status != catalog.PackageStatusDraft {
		t.Fatalf("new package version must be a draft, got %q", version.Status)
	}
	if len(version.Items) != 2 {
		t.Fatalf("bundle must expand into two package items, got %d", len(version.Items))
	}
	// Bundle expansion: 4 paper rolls + 2 soaps, baseline consumption equal to
	// bundle quantity.
	if version.ReviewSummary.MonthlyConsumptionUnits != 6 {
		t.Fatalf("estimated monthly consumption = %d, want 6", version.ReviewSummary.MonthlyConsumptionUnits)
	}
	if version.ReviewSummary.SetupCostMinorUnits != 4*2500+2*4000 {
		t.Fatalf("setup cost = %d, want %d", version.ReviewSummary.SetupCostMinorUnits, 4*2500+2*4000)
	}
	if version.ReviewSummary.MonthlyCostMinorUnits != 4*2500+2*4000 {
		t.Fatalf("monthly cost = %d, want %d", version.ReviewSummary.MonthlyCostMinorUnits, 4*2500+2*4000)
	}
}

// --- Owner sees cost and substitution before activation (CAT-005, CAT-009) ---

func TestCatalogOwnerSeesCostAndSubstitutionBeforeActivation(t *testing.T) {
	tenantID := "tenant-catalog-before-activation"
	svc := newCatalogService(t, tenantID)
	ctx := context.Background()

	paper := createItem(t, svc, tenantID, "SKU-A-PAPER")
	soap := createSoapItem(t, svc, tenantID, "SKU-A-SOAP")

	version, err := svc.CreatePropertyPackageVersion(ctx, tenantID, "property-a", catalog.CreatePackageVersionParams{
		EffectiveDate:                   time.Now().UTC().AddDate(0, 0, 7),
		SubstitutionPolicy:              catalog.SubstitutionOwnerApproval,
		RequireApprovalForPriceIncrease: true,
		RequireApprovalForNewSKU:        true,
		MonthlyBudgetLimitMinorUnits:    int64Ptr(2000000),
		Items: []catalog.PackageItemInput{
			{CatalogItemID: paper.ID, Quantity: 4, ExpectedMonthlyConsumption: 6},
			{CatalogItemID: soap.ID, Quantity: 2, ExpectedMonthlyConsumption: 2},
		},
	}, "owner-a")
	if err != nil {
		t.Fatalf("create package version: %v", err)
	}

	summary := version.ReviewSummary
	if summary.SetupCostMinorUnits != 4*2500+2*4000 {
		t.Fatalf("review summary must disclose one-time setup cost, got %d", summary.SetupCostMinorUnits)
	}
	if summary.MonthlyCostMinorUnits != 6*2500+2*4000 {
		t.Fatalf("review summary must disclose estimated monthly cost, got %d", summary.MonthlyCostMinorUnits)
	}
	if summary.MonthlyConsumptionUnits != 8 {
		t.Fatalf("review summary must disclose estimated monthly consumption, got %d", summary.MonthlyConsumptionUnits)
	}
	if summary.SubstitutionBehavior == "" {
		t.Fatal("review summary must describe substitution behavior")
	}
	if !summary.SubstitutionApprovalRequired {
		t.Fatal("owner_approval policy must require owner approval for substitution")
	}
	if !summary.PriceIncreaseRequiresApproval || !summary.NewSKURequiresApproval {
		t.Fatal("review summary must disclose approval policy for price increases and new SKUs")
	}
	if summary.MonthlyBudgetLimitMinorUnits == nil || *summary.MonthlyBudgetLimitMinorUnits != 2000000 {
		t.Fatal("review summary must disclose the monthly budget limit")
	}
	for _, it := range summary.Items {
		if it.Label == "" {
			t.Fatalf("review summary must expose the operational label of SKU %s", it.SKU)
		}
	}

	// An automatic package still presents a review summary before first
	// activation (CAT-009): activation is a separate call on a draft that has a
	// retained review summary, and it is not allowed without one.
	activated, err := svc.ActivatePropertyPackageVersion(ctx, tenantID, "property-a", version.ID, "owner-a")
	if err != nil {
		t.Fatalf("activate package version: %v", err)
	}
	if activated.Status != catalog.PackageStatusActive {
		t.Fatalf("activated version must be active, got %q", activated.Status)
	}
	if activated.ActivatedAt == nil {
		t.Fatal("activated version must record its activation time")
	}
	if activated.ReviewSummary.SetupCostMinorUnits != version.ReviewSummary.SetupCostMinorUnits {
		t.Fatal("the retained review summary must be preserved through activation")
	}

	if _, err := svc.ActivatePropertyPackageVersion(ctx, tenantID, "property-a", version.ID, "owner-a"); !errors.Is(err, catalog.ErrPackageVersionAlreadyActive) {
		t.Fatalf("activating an active version must be rejected, got %v", err)
	}
}

func TestCatalogPackageVersionRequiresEffectiveDateAndItems(t *testing.T) {
	tenantID := "tenant-catalog-validate-pkg"
	svc := newCatalogService(t, tenantID)
	ctx := context.Background()

	item := createItem(t, svc, tenantID, "SKU-VALIDATE")

	if _, err := svc.CreatePropertyPackageVersion(ctx, tenantID, "property-v", catalog.CreatePackageVersionParams{
		Items: []catalog.PackageItemInput{
			{CatalogItemID: item.ID, Quantity: 2, ExpectedMonthlyConsumption: 2},
		},
	}, "owner-v"); !errors.Is(err, catalog.ErrEffectiveDateRequired) {
		t.Fatalf("a package version without an effective date must be rejected, got %v", err)
	}

	if _, err := svc.CreatePropertyPackageVersion(ctx, tenantID, "property-v", catalog.CreatePackageVersionParams{
		EffectiveDate: time.Now().UTC(),
	}, "owner-v"); !errors.Is(err, catalog.ErrNoPackageItems) {
		t.Fatalf("a package version without items must be rejected, got %v", err)
	}

	if _, err := svc.CreatePropertyPackageVersion(ctx, tenantID, "property-v", catalog.CreatePackageVersionParams{
		EffectiveDate:      time.Now().UTC(),
		SubstitutionPolicy: "ask_first",
		Items: []catalog.PackageItemInput{
			{CatalogItemID: item.ID, Quantity: 2, ExpectedMonthlyConsumption: 2},
		},
	}, "owner-v"); !errors.Is(err, catalog.ErrInvalidSubstitutionPolicy) {
		t.Fatalf("an unknown substitution policy must be rejected, got %v", err)
	}

	if _, err := svc.CreatePropertyPackageVersion(ctx, tenantID, "property-v", catalog.CreatePackageVersionParams{
		EffectiveDate:                time.Now().UTC(),
		MonthlyBudgetLimitMinorUnits: int64Ptr(-1),
		Items: []catalog.PackageItemInput{
			{CatalogItemID: item.ID, Quantity: 2, ExpectedMonthlyConsumption: 2},
		},
	}, "owner-v"); !errors.Is(err, catalog.ErrInvalidPackageVersion) {
		t.Fatalf("a negative monthly budget must be rejected, got %v", err)
	}

	if _, err := svc.CreatePropertyPackageVersion(ctx, tenantID, "property-v", catalog.CreatePackageVersionParams{
		EffectiveDate: time.Now().UTC(),
		Items: []catalog.PackageItemInput{
			{CatalogItemID: "missing-item", Quantity: 2, ExpectedMonthlyConsumption: 2},
		},
	}, "owner-v"); !errors.Is(err, catalog.ErrPackageVersionItemNotFound) {
		t.Fatalf("a package version referencing an unknown item must be rejected, got %v", err)
	}
}

// --- Package changes are versioned with effective dates (CAT-008) ------------

func TestCatalogPackageChangesAreVersioned(t *testing.T) {
	tenantID := "tenant-catalog-versioned"
	svc := newCatalogService(t, tenantID)
	ctx := context.Background()

	paper := createItem(t, svc, tenantID, "SKU-V-PAPER")
	soap := createSoapItem(t, svc, tenantID, "SKU-V-SOAP")

	base := catalog.CreatePackageVersionParams{
		EffectiveDate:      time.Now().UTC().AddDate(0, 0, 7),
		SubstitutionPolicy: catalog.SubstitutionOwnerApproval,
		Items: []catalog.PackageItemInput{
			{CatalogItemID: paper.ID, Quantity: 4, ExpectedMonthlyConsumption: 6},
		},
	}

	v1, err := svc.CreatePropertyPackageVersion(ctx, tenantID, "property-v", base, "owner-v")
	if err != nil {
		t.Fatalf("create v1: %v", err)
	}
	if v1.VersionNumber != 1 {
		t.Fatalf("v1 must be version 1, got %d", v1.VersionNumber)
	}

	activatedV1, err := svc.ActivatePropertyPackageVersion(ctx, tenantID, "property-v", v1.ID, "owner-v")
	if err != nil {
		t.Fatalf("activate v1: %v", err)
	}
	if activatedV1.Status != catalog.PackageStatusActive {
		t.Fatalf("v1 must become active, got %q", activatedV1.Status)
	}

	v2, err := svc.CreatePropertyPackageVersion(ctx, tenantID, "property-v", catalog.CreatePackageVersionParams{
		EffectiveDate:      time.Now().UTC().AddDate(0, 0, 14),
		SubstitutionPolicy: catalog.SubstitutionAutomatic,
		Items: []catalog.PackageItemInput{
			{CatalogItemID: paper.ID, Quantity: 6, ExpectedMonthlyConsumption: 8},
			{CatalogItemID: soap.ID, Quantity: 2, ExpectedMonthlyConsumption: 2},
		},
	}, "owner-v")
	if err != nil {
		t.Fatalf("create v2: %v", err)
	}
	if v2.VersionNumber != 2 {
		t.Fatalf("v2 must be version 2, got %d", v2.VersionNumber)
	}
	if v2.Status != catalog.PackageStatusDraft || v2.EffectiveDate.IsZero() {
		t.Fatalf("v2 must be a draft with an effective date, got %+v", v2)
	}

	// The change (adding a soap line and changing substitution policy) is a new
	// version; the earlier active version is superseded, not deleted.
	activated, err := svc.ActivatePropertyPackageVersion(ctx, tenantID, "property-v", v2.ID, "owner-v")
	if err != nil {
		t.Fatalf("activate v2: %v", err)
	}
	if activated.Status != catalog.PackageStatusActive {
		t.Fatalf("v2 must become active, got %q", activated.Status)
	}

	all, err := svc.ListPropertyPackageVersions(ctx, tenantID, "property-v")
	if err != nil {
		t.Fatalf("list package versions: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("both versions must be retained, got %d", len(all))
	}
	byNumber := map[int]*catalog.PropertyPackageVersion{}
	for i := range all {
		byNumber[all[i].VersionNumber] = &all[i]
	}
	if byNumber[1].Status != catalog.PackageStatusSuperseded {
		t.Fatalf("prior active version must be superseded, got %q", byNumber[1].Status)
	}
	if byNumber[2].Status != catalog.PackageStatusActive {
		t.Fatalf("new version must be the active one, got %q", byNumber[2].Status)
	}

	// Rejected drafts stay on the record and cannot be activated.
	v3, err := svc.CreatePropertyPackageVersion(ctx, tenantID, "property-v", catalog.CreatePackageVersionParams{
		EffectiveDate:      time.Now().UTC().AddDate(0, 0, 21),
		SubstitutionPolicy: catalog.SubstitutionRestricted,
		Items: []catalog.PackageItemInput{
			{CatalogItemID: soap.ID, Quantity: 4, ExpectedMonthlyConsumption: 4},
		},
	}, "owner-v")
	if err != nil {
		t.Fatalf("create v3: %v", err)
	}
	if v3.VersionNumber != 3 {
		t.Fatalf("v3 must be version 3, got %d", v3.VersionNumber)
	}
	rejected, err := svc.RejectPropertyPackageVersion(ctx, tenantID, "property-v", v3.ID, "owner-v")
	if err != nil {
		t.Fatalf("reject v3: %v", err)
	}
	if rejected.Status != catalog.PackageStatusRejected {
		t.Fatalf("v3 must be rejected, got %q", rejected.Status)
	}
	if _, err := svc.ActivatePropertyPackageVersion(ctx, tenantID, "property-v", v3.ID, "owner-v"); !errors.Is(err, catalog.ErrPackageVersionNotDraft) {
		t.Fatalf("activating a rejected version must be rejected, got %v", err)
	}
}

func TestCatalogDuplicateSKUInPackageRejected(t *testing.T) {
	tenantID := "tenant-catalog-dup-sku"
	svc := newCatalogService(t, tenantID)
	ctx := context.Background()

	item := createItem(t, svc, tenantID, "SKU-DUP")

	if _, err := svc.CreatePropertyPackageVersion(ctx, tenantID, "property-d", catalog.CreatePackageVersionParams{
		EffectiveDate: time.Now().UTC(),
		Items: []catalog.PackageItemInput{
			{CatalogItemID: item.ID, Quantity: 1, ExpectedMonthlyConsumption: 1},
			{CatalogItemID: item.ID, Quantity: 2, ExpectedMonthlyConsumption: 2},
		},
	}, "owner-d"); !errors.Is(err, catalog.ErrDuplicatePackageSKU) {
		t.Fatalf("a package version with a duplicate SKU must be rejected, got %v", err)
	}
}

func TestCatalogDisabledItemCannotEnterPackage(t *testing.T) {
	tenantID := "tenant-catalog-disabled"
	svc := newCatalogService(t, tenantID)
	ctx := context.Background()

	item, err := svc.CreateCatalogItem(ctx, tenantID, catalog.CreateItemParams{
		SKU:                  "SKU-DISABLED",
		Name:                 "Disabled item",
		Category:             "cleaning",
		UnitCostMinorUnits:   100,
		UnitCostCurrency:     "INR",
		OwnerPriceMinorUnits: 200,
		OwnerPriceCurrency:   "INR",
		Status:               catalog.ItemStatusDisabled,
		Label:                catalog.LabelCuratorsStandard,
	}, "actor-ops-1")
	if err != nil {
		t.Fatalf("create disabled item: %v", err)
	}

	if _, err := svc.CreatePropertyPackageVersion(ctx, tenantID, "property-x", catalog.CreatePackageVersionParams{
		EffectiveDate: time.Now().UTC(),
		Items: []catalog.PackageItemInput{
			{CatalogItemID: item.ID, Quantity: 1, ExpectedMonthlyConsumption: 1},
		},
	}, "owner-x"); !errors.Is(err, catalog.ErrPackageItemDisabled) {
		t.Fatalf("a disabled catalog item must not enter a package, got %v", err)
	}
}

// --- Tenant and property scope fail closed -----------------------------------

func TestCatalogCrossTenantAndPropertyScopeFailsClosed(t *testing.T) {
	ctx := context.Background()
	tenantA := "tenant-catalog-cross-a"
	tenantB := "tenant-catalog-cross-b"
	pool := catalogPool(t)
	svcA := newCatalogServiceOnPool(t, pool, tenantA)
	svcB := newCatalogServiceOnPool(t, pool, tenantB)

	item := createItem(t, svcA, tenantA, "SKU-CROSS")

	// A service with no authorizer denies everything (fail closed).
	unauthenticated := catalog.NewService(pool)
	if _, err := unauthenticated.CreatePropertyPackageVersion(ctx, tenantA, "property-a-1", catalog.CreatePackageVersionParams{
		EffectiveDate: time.Now().UTC(),
		Items: []catalog.PackageItemInput{
			{CatalogItemID: item.ID, Quantity: 2, ExpectedMonthlyConsumption: 2},
		},
	}, "owner-a"); !errors.Is(err, catalog.ErrCrossTenantDenied) {
		t.Fatalf("a service without an authorizer must deny property-scoped writes, got %v", err)
	}
	if _, err := svcA.AddClaimEvidence(ctx, tenantA, item.ID, catalog.ClaimEvidenceParams{
		ClaimType:   catalog.ClaimTypePerformance,
		EvidenceRef: "evidence:perf-1",
	}, "actor-a"); err != nil {
		t.Fatalf("add claim evidence: %v", err)
	}

	if _, err := svcB.GetCatalogItem(ctx, tenantB, item.ID); !errors.Is(err, catalog.ErrItemNotFound) {
		t.Fatalf("cross-tenant item read must fail closed, got %v", err)
	}
	if _, err := svcB.ListClaimEvidence(ctx, tenantB, item.ID); !errors.Is(err, catalog.ErrItemNotFound) {
		t.Fatalf("cross-tenant claim evidence read must fail closed, got %v", err)
	}
	if _, err := svcB.AddClaimEvidence(ctx, tenantB, item.ID, catalog.ClaimEvidenceParams{
		ClaimType:   catalog.ClaimTypeQuality,
		EvidenceRef: "evidence:q-1",
	}, "actor-b"); !errors.Is(err, catalog.ErrItemNotFound) {
		t.Fatalf("cross-tenant claim write must fail closed, got %v", err)
	}

	version, err := svcA.CreatePropertyPackageVersion(ctx, tenantA, "property-a-1", catalog.CreatePackageVersionParams{
		EffectiveDate: time.Now().UTC(),
		Items: []catalog.PackageItemInput{
			{CatalogItemID: item.ID, Quantity: 2, ExpectedMonthlyConsumption: 2},
		},
	}, "owner-a")
	if err != nil {
		t.Fatalf("create package version: %v", err)
	}

	// The same tenant with a different property scope must fail closed.
	if _, err := svcA.GetPropertyPackageVersion(ctx, tenantA, "property-a-2", version.ID); !errors.Is(err, catalog.ErrPackageVersionNotFound) {
		t.Fatalf("wrong property scope must fail closed, got %v", err)
	}
	if _, err := svcA.ActivatePropertyPackageVersion(ctx, tenantA, "property-a-2", version.ID, "owner-a"); !errors.Is(err, catalog.ErrPackageVersionNotFound) {
		t.Fatalf("cross-property activation must fail closed, got %v", err)
	}
}
