package consumer_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"comfort-curators-backend/internal/billing"
	"comfort-curators-backend/internal/catalog"
	"comfort-curators-backend/internal/consumer"
	"comfort-curators-backend/internal/operations"
	"comfort-curators-backend/internal/platform/audit"
	"comfort-curators-backend/internal/platform/testdb"

	"github.com/jackc/pgx/v5/pgxpool"
)

func consumerPostgresAvailable() bool {
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

func consumerDBConnString() string {
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

func consumerPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if !consumerPostgresAvailable() {
		t.Skip("PostgreSQL not available for consumer integration test")
	}
	pool, err := pgxpool.New(context.Background(), consumerDBConnString())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	for name, ensure := range map[string]func(context.Context, *pgxpool.Pool) error{
		"consumer":   consumer.EnsureSchema,
		"catalog":    catalog.EnsureSchema,
		"billing":    billing.EnsureSchema,
		"operations": operations.EnsureSchema,
		"audit":      audit.EnsureSchema,
	} {
		if err := ensure(context.Background(), pool); err != nil {
			t.Fatalf("ensure %s schema: %v", name, err)
		}
	}

	for _, table := range []string{
		"consumer_history_exports",
		"consumer_acceptances",
		"consumer_disclosures",
		"property_package_versions",
		"invoices",
		"charges",
		"tickets",
		"audit_events",
	} {
		if _, err := pool.Exec(context.Background(), "TRUNCATE "+table+" CASCADE"); err != nil {
			t.Fatalf("truncate %s: %v", table, err)
		}
	}
	return pool
}

func newConsumerService(t *testing.T, pool *pgxpool.Pool) *consumer.ConsumerService {
	t.Helper()
	return consumer.NewConsumerService(pool, audit.NewAuditStore(pool))
}

func int64Ptr(v int64) *int64 { return &v }

func monthlyDisclosureParams(resourceID string) consumer.DisclosureParams {
	return consumer.DisclosureParams{
		ResourceType:               consumer.ResourceTypePackage,
		ResourceID:                 resourceID,
		PropertyID:                 "prop-1",
		PriceMinorUnits:            10000,
		TaxMinorUnits:              1200,
		Currency:                   "INR",
		Recurrence:                 consumer.RecurrenceMonthly,
		RecurrenceAmountMinorUnits: int64Ptr(18000),
		SubstitutionPolicy:         catalog.SubstitutionOwnerApproval,
		CancellationPolicy:         "cancel_anytime",
		RefundPolicy:               "pro_rata",
		Seller:                     "curators-direct",
		CountryOfOrigin:            "India",
		GrievanceContact:           "support@curators.example",
	}
}

// --- Recurring cost is visible before acceptance (CON-001, CON-004) ---------

func TestConsumerAcceptanceRequiresDisclosureWithVisibleRecurringCost(t *testing.T) {
	ctx := context.Background()
	tenantID := "tenant-consumer-accept"
	pool := consumerPool(t)
	svc := newConsumerService(t, pool)

	// Acceptance without any prior disclosure is refused: recurring cost can
	// never be accepted unseen.
	if _, err := svc.Accept(ctx, tenantID, consumer.AcceptanceParams{
		DisclosureID: "missing-disclosure",
		ResourceType: consumer.ResourceTypePackage,
		ResourceID:   "pkg-1",
	}, "owner-1"); !errors.Is(err, consumer.ErrNoDisclosureBeforeAccept) {
		t.Fatalf("acceptance without a disclosure must be refused, got %v", err)
	}

	// A recurring disclosure must carry an explicit recurring cost.
	if _, err := svc.RecordDisclosure(ctx, tenantID, consumer.DisclosureParams{
		ResourceType:    consumer.ResourceTypePackage,
		ResourceID:      "pkg-hidden",
		PriceMinorUnits: 10000,
		Currency:        "INR",
		Recurrence:      consumer.RecurrenceMonthly,
	}, "operator-1"); !errors.Is(err, consumer.ErrHiddenRecurringCost) {
		t.Fatalf("a recurring disclosure without an explicit recurring cost must be rejected, got %v", err)
	}

	// A valid recurring disclosure makes the recurring cost visible, and the
	// same resource can then be accepted.
	disclosure, err := svc.RecordDisclosure(ctx, tenantID, monthlyDisclosureParams("pkg-1"), "operator-1")
	if err != nil {
		t.Fatalf("record disclosure: %v", err)
	}
	if !disclosure.RecurringCostVisible {
		t.Fatal("recorded disclosure must expose the recurring cost as visible")
	}
	if disclosure.RecurringCost() != 18000 {
		t.Fatalf("recurring cost must be 18000, got %d", disclosure.RecurringCost())
	}

	acceptance, err := svc.Accept(ctx, tenantID, consumer.AcceptanceParams{
		DisclosureID: disclosure.ID,
		ResourceType: consumer.ResourceTypePackage,
		ResourceID:   "pkg-1",
	}, "owner-1")
	if err != nil {
		t.Fatalf("accept after disclosure: %v", err)
	}
	if acceptance.DisclosureID != disclosure.ID {
		t.Fatalf("acceptance must reference its disclosure, got %q", acceptance.DisclosureID)
	}
	if acceptance.AcceptedBy != "owner-1" {
		t.Fatalf("acceptance must record the acceptor, got %q", acceptance.AcceptedBy)
	}

	// A one-time disclosure with a visible price is also acceptable.
	oneTime, err := svc.RecordDisclosure(ctx, tenantID, consumer.DisclosureParams{
		ResourceType:    consumer.ResourceTypeService,
		ResourceID:      "svc-1",
		PropertyID:      "prop-1",
		PriceMinorUnits: 5000,
		Currency:        "INR",
		Recurrence:      consumer.RecurrenceOneTime,
	}, "operator-1")
	if err != nil {
		t.Fatalf("record one-time disclosure: %v", err)
	}
	if _, err := svc.Accept(ctx, tenantID, consumer.AcceptanceParams{
		DisclosureID: oneTime.ID,
		ResourceType: consumer.ResourceTypeService,
		ResourceID:   "svc-1",
	}, "owner-1"); err != nil {
		t.Fatalf("accept after one-time disclosure: %v", err)
	}

	// A disclosure for a different resource cannot be used for acceptance.
	if _, err := svc.Accept(ctx, tenantID, consumer.AcceptanceParams{
		DisclosureID: disclosure.ID,
		ResourceType: consumer.ResourceTypePackage,
		ResourceID:   "pkg-other",
	}, "owner-1"); !errors.Is(err, consumer.ErrDisclosureResourceMismatch) {
		t.Fatalf("acceptance with a mismatched resource must be refused, got %v", err)
	}
}

func TestConsumerDisclosureIsTenantScoped(t *testing.T) {
	ctx := context.Background()
	tenantA := "tenant-consumer-scope-a"
	tenantB := "tenant-consumer-scope-b"
	pool := consumerPool(t)
	svc := newConsumerService(t, pool)

	dA, err := svc.RecordDisclosure(ctx, tenantA, monthlyDisclosureParams("pkg-a"), "operator-a")
	if err != nil {
		t.Fatalf("record tenant A disclosure: %v", err)
	}

	if _, err := svc.GetDisclosure(ctx, tenantB, dA.ID); !errors.Is(err, consumer.ErrDisclosureNotFound) {
		t.Fatalf("tenant B must not read tenant A disclosure, got %v", err)
	}
	if _, err := svc.Accept(ctx, tenantB, consumer.AcceptanceParams{
		DisclosureID: dA.ID,
		ResourceType: consumer.ResourceTypePackage,
		ResourceID:   "pkg-a",
	}, "owner-b"); !errors.Is(err, consumer.ErrNoDisclosureBeforeAccept) {
		t.Fatalf("tenant B must not accept on tenant A disclosure, got %v", err)
	}
}

// --- History exports stay within tenant scope (CON-006) ----------------------

func seedTenantHistory(t *testing.T, pool *pgxpool.Pool, tenantID, propertyID string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()

	if _, err := pool.Exec(ctx, `
		INSERT INTO property_package_versions (
			id, tenant_id, property_id, version_number, status, effective_date,
			currency, setup_cost_minor_units, monthly_cost_minor_units,
			review_summary, created_at, updated_at
		) VALUES ($1,$2,$3,1,'active',$4,'INR',25000,18000,'{}',$4,$4)
	`, "pkg-"+tenantID+"-"+propertyID, tenantID, propertyID, now); err != nil {
		t.Fatalf("seed package: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO invoices (
			id, tenant_id, property_id, period_start, period_end,
			total_minor_units, currency, status, idempotency_key,
			version, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,43000,'INR','issued',$6,1,$5,$5)
	`, "inv-"+tenantID+"-"+propertyID, tenantID, propertyID, now.AddDate(0, -1, 0), now, "idem-"+tenantID+"-"+propertyID); err != nil {
		t.Fatalf("seed invoice: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO charges (
			id, tenant_id, property_id, charge_type, amount_minor_units,
			currency, reason, order_id, idempotency_key, status,
			version, created_at, updated_at
		) VALUES ($1,$2,$3,'purchased_goods',12000,'INR','order',$4,$6,'applied',1,$5,$5)
	`, "chg-"+tenantID+"-"+propertyID, tenantID, propertyID, "ord-"+tenantID+"-"+propertyID, now, "cidem-"+tenantID+"-"+propertyID); err != nil {
		t.Fatalf("seed charge: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO tickets (
			id, tenant_id, property_id, type, status, version, created_at, updated_at
		) VALUES ($1,$2,$3,'cleaning','completed',1,$4,$4)
	`, "tkt-"+tenantID+"-"+propertyID, tenantID, propertyID, now); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
}

func TestConsumerHistoryExportIsTenantScoped(t *testing.T) {
	ctx := context.Background()
	tenantA := "tenant-consumer-export-a"
	tenantB := "tenant-consumer-export-b"
	pool := consumerPool(t)
	svc := newConsumerService(t, pool)

	seedTenantHistory(t, pool, tenantA, "prop-a")
	seedTenantHistory(t, pool, tenantB, "prop-b")

	exportA, err := svc.CreateHistoryExport(ctx, tenantA, "", "owner-a")
	if err != nil {
		t.Fatalf("create tenant A export: %v", err)
	}

	var dataA consumer.HistoryExportData
	if err := json.Unmarshal(exportA.Data, &dataA); err != nil {
		t.Fatalf("unmarshal tenant A export data: %v", err)
	}

	if len(dataA.Packages) != 1 || dataA.Packages[0].PropertyID != "prop-a" {
		t.Fatalf("tenant A export must contain only tenant A package history, got %+v", dataA.Packages)
	}
	if len(dataA.Invoices) != 1 || dataA.Invoices[0].PropertyID != "prop-a" {
		t.Fatalf("tenant A export must contain only tenant A invoice history, got %+v", dataA.Invoices)
	}
	if len(dataA.Orders) != 1 || dataA.Orders[0].OrderID != "ord-"+tenantA+"-prop-a" {
		t.Fatalf("tenant A export must contain only tenant A order history, got %+v", dataA.Orders)
	}
	if len(dataA.Services) != 1 || dataA.Services[0].PropertyID != "prop-a" {
		t.Fatalf("tenant A export must contain only tenant A service history, got %+v", dataA.Services)
	}

	// No tenant B row may leak into tenant A's export.
	for _, p := range dataA.Packages {
		if p.PropertyID == "prop-b" {
			t.Fatal("tenant A export must not contain tenant B package history")
		}
	}
	for _, inv := range dataA.Invoices {
		if inv.PropertyID == "prop-b" {
			t.Fatal("tenant A export must not contain tenant B invoice history")
		}
	}
	for _, o := range dataA.Orders {
		if o.OrderID == "ord-"+tenantB+"-prop-b" {
			t.Fatal("tenant A export must not contain tenant B order history")
		}
	}
	for _, s := range dataA.Services {
		if s.PropertyID == "prop-b" {
			t.Fatal("tenant A export must not contain tenant B service history")
		}
	}

	// Tenant B cannot read tenant A's export.
	if _, err := svc.GetHistoryExport(ctx, tenantB, exportA.ID); !errors.Is(err, consumer.ErrExportNotFound) {
		t.Fatalf("tenant B must not read tenant A export, got %v", err)
	}

	// Tenant B's own export stays inside tenant B's scope.
	exportB, err := svc.CreateHistoryExport(ctx, tenantB, "", "owner-b")
	if err != nil {
		t.Fatalf("create tenant B export: %v", err)
	}
	var dataB consumer.HistoryExportData
	if err := json.Unmarshal(exportB.Data, &dataB); err != nil {
		t.Fatalf("unmarshal tenant B export data: %v", err)
	}
	if len(dataB.Packages) != 1 || dataB.Packages[0].PropertyID != "prop-b" {
		t.Fatalf("tenant B export must contain only tenant B history, got %+v", dataB.Packages)
	}

	exportsA, err := svc.ListHistoryExports(ctx, tenantA)
	if err != nil {
		t.Fatalf("list tenant A exports: %v", err)
	}
	if len(exportsA) != 1 || exportsA[0].ID != exportA.ID {
		t.Fatalf("tenant A must list exactly its own export, got %d", len(exportsA))
	}
}

func TestConsumerHistoryExportPropertyFilter(t *testing.T) {
	ctx := context.Background()
	tenantID := "tenant-consumer-export-prop"
	pool := consumerPool(t)
	svc := newConsumerService(t, pool)

	seedTenantHistory(t, pool, tenantID, "prop-1")
	seedTenantHistory(t, pool, tenantID, "prop-2")

	export, err := svc.CreateHistoryExport(ctx, tenantID, "prop-2", "owner-1")
	if err != nil {
		t.Fatalf("create property-scoped export: %v", err)
	}
	var data consumer.HistoryExportData
	if err := json.Unmarshal(export.Data, &data); err != nil {
		t.Fatalf("unmarshal export data: %v", err)
	}
	if len(data.Packages) != 1 || data.Packages[0].PropertyID != "prop-2" {
		t.Fatalf("property-scoped export must contain only that property's packages, got %+v", data.Packages)
	}
}
