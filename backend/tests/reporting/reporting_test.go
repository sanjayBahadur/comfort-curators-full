package reporting_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"comfort-curators-backend/internal/billing"
	"comfort-curators-backend/internal/inventory"
	"comfort-curators-backend/internal/operations"
	"comfort-curators-backend/internal/reporting"

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
	name := os.Getenv("CC_DB_NAME")
	if name == "" {
		name = "comfort_curators"
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, pass, host, port, name)
}

func setupPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if !postgresAvailable() {
		t.Skip("PostgreSQL not available for reporting integration test")
	}
	pool, err := pgxpool.New(context.Background(), dbConnString())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	for _, setup := range []struct {
		name string
		fn   func(context.Context, *pgxpool.Pool) error
	}{
		{"billing", billing.EnsureSchema},
		{"operations", operations.EnsureSchema},
		{"inventory", inventory.EnsureSchema},
		{"reporting", reporting.EnsureSchema},
	} {
		if err := setup.fn(context.Background(), pool); err != nil {
			t.Fatalf("ensure %s schema: %v", setup.name, err)
		}
	}

	truncateTables(t, pool)
	return pool
}

func truncateTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	// inventory_movements is append-only via a DB trigger; drop the trigger so
	// test rows can be cleared, then recreate it via EnsureSchema below.
	if _, err := pool.Exec(ctx, `DROP TRIGGER IF EXISTS inventory_movements_no_update ON inventory_movements`); err != nil {
		t.Fatalf("drop inventory immutable trigger: %v", err)
	}
	for _, table := range []string{
		"metric_observations",
		"report_snapshots",
		"reconciliation_exceptions",
		"inventory_movements",
		"stock_locations",
		"service_recoveries",
		"ticket_state_events",
		"tickets",
		"credits",
		"charges",
	} {
		if _, err := pool.Exec(ctx, "DELETE FROM "+table); err != nil {
			t.Fatalf("truncate %s: %v", table, err)
		}
	}
	if err := inventory.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("recreate inventory schema: %v", err)
	}
}

func insertCharge(t *testing.T, pool *pgxpool.Pool, id, tenant, property, chargeType string, amount int64, status string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO charges (id, tenant_id, property_id, charge_type, amount_minor_units, currency, idempotency_key, status)
		VALUES ($1,$2,$3,$4,$5,'INR',$6,$7)
	`, id, tenant, property, chargeType, amount, "ik-"+id, status)
	if err != nil {
		t.Fatalf("insert charge %s: %v", id, err)
	}
}

func insertCredit(t *testing.T, pool *pgxpool.Pool, id, tenant, property, creditType string, amount int64, status string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO credits (id, tenant_id, property_id, credit_type, amount_minor_units, currency, original_entry_id, original_entry_type, idempotency_key, status)
		VALUES ($1,$2,$3,$4,$5,'INR','entry-1','charge',$6,$7)
	`, id, tenant, property, creditType, amount, "ik-"+id, status)
	if err != nil {
		t.Fatalf("insert credit %s: %v", id, err)
	}
}

func insertTicket(t *testing.T, pool *pgxpool.Pool, id, tenant, property, ticketType, status string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO tickets (id, tenant_id, property_id, type, status, reason)
		VALUES ($1,$2,$3,$4,$5,$6)
	`, id, tenant, property, ticketType, status, "reason-"+id)
	if err != nil {
		t.Fatalf("insert ticket %s: %v", id, err)
	}
}

func insertRecovery(t *testing.T, pool *pgxpool.Pool, id, tenant, property, incidentTicketID string, rework int64, status string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO service_recoveries (id, tenant_id, property_id, incident_ticket_id, severity, original_reason, responsibility, rework_cost_minor, currency, status)
		VALUES ($1,$2,$3,$4,'high','failure','internal',$5,'INR',$6)
	`, id, tenant, property, incidentTicketID, rework, status)
	if err != nil {
		t.Fatalf("insert recovery %s: %v", id, err)
	}
}

func insertFinancialException(t *testing.T, pool *pgxpool.Pool, id, tenant, property string, status string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO reconciliation_exceptions (id, tenant_id, property_id, entry_id, entry_type, exception_type, description, status, recorded_by)
		VALUES ($1,$2,$3,'entry-1','payment','amount_mismatch','bank statement mismatch',$4,'auditor-1')
	`, id, tenant, property, status)
	if err != nil {
		t.Fatalf("insert financial exception %s: %v", id, err)
	}
}

func insertStockLocation(t *testing.T, pool *pgxpool.Pool, id, tenant, property string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO stock_locations (id, tenant_id, property_id, name, location_type)
		VALUES ($1,$2,$3,'property closet','property')
	`, id, tenant, property)
	if err != nil {
		t.Fatalf("insert stock location %s: %v", id, err)
	}
}

func insertMovement(t *testing.T, pool *pgxpool.Pool, id, tenant, locationID string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO inventory_movements (id, tenant_id, location_id, catalog_item_id, movement_type, quantity, reason)
		VALUES ($1,$2,$3,'item-1','consume',1,'ticket work')
	`, id, tenant, locationID)
	if err != nil {
		t.Fatalf("insert movement %s: %v", id, err)
	}
}

// TestReportingProjectionRebuildMatchesSourceTransactions proves that a
// rebuildable read model is exactly reproducible from the source transactions:
// a fresh rebuild matches, a new source transaction makes the stored snapshot
// stale until it is rebuilt again, and the rebuilt snapshot matches again.
func TestReportingProjectionRebuildMatchesSourceTransactions(t *testing.T) {
	pool := setupPool(t)
	svc := reporting.NewReportingService(pool, nil)

	insertCharge(t, pool, "chg-1", "tenant-1", "prop-1", "management_fee", 10000, "applied")
	insertCharge(t, pool, "chg-2", "tenant-1", "prop-1", "task_service", 5000, "applied")
	insertCredit(t, pool, "crd-1", "tenant-1", "prop-1", "refund", 250, "issued")
	insertTicket(t, pool, "tkt-inc", "tenant-1", "prop-1", "incident", "in_progress")
	insertRecovery(t, pool, "rec-1", "tenant-1", "prop-1", "tkt-inc", 700, "open")

	snap, err := svc.RebuildSnapshot(context.Background(), "tenant-1", reporting.RebuildParams{
		Kind:       reporting.ProjectionPropertyContribution,
		PropertyID: "prop-1",
	})
	if err != nil {
		t.Fatalf("rebuild contribution snapshot: %v", err)
	}
	if snap.Version != 1 {
		t.Fatalf("expected version 1 on first rebuild, got %d", snap.Version)
	}
	if snap.SourceCount != 4 {
		t.Fatalf("expected 4 source rows (2 charges, 1 credit, 1 recovery), got %d", snap.SourceCount)
	}

	verification, err := svc.VerifySnapshot(context.Background(), "tenant-1", snap.ID)
	if err != nil {
		t.Fatalf("verify snapshot: %v", err)
	}
	if !verification.Match {
		t.Fatalf("fresh snapshot must match its source: %s", verification.MismatchReason)
	}

	var pc reporting.PropertyContribution
	if err := json.Unmarshal(snap.Data, &pc); err != nil {
		t.Fatalf("unmarshal contribution data: %v", err)
	}
	if pc.RevenueMinorUnits != 15000 || pc.RefundMinorUnits != 250 || pc.ExceptionCostMinorUnits != 700 {
		t.Fatalf("unexpected contribution values: %+v", pc)
	}

	// A new source transaction makes the stored projection stale.
	insertCharge(t, pool, "chg-3", "tenant-1", "prop-1", "vendor_fee", 800, "applied")

	stale, err := svc.VerifySnapshot(context.Background(), "tenant-1", snap.ID)
	if err != nil {
		t.Fatalf("verify stale snapshot: %v", err)
	}
	if stale.Match {
		t.Fatal("snapshot must not match after a new source transaction")
	}

	// Rebuilding restores the match.
	rebuilt, err := svc.RebuildSnapshot(context.Background(), "tenant-1", reporting.RebuildParams{
		Kind:       reporting.ProjectionPropertyContribution,
		PropertyID: "prop-1",
	})
	if err != nil {
		t.Fatalf("rebuild after source change: %v", err)
	}
	if rebuilt.Version != snap.Version+1 {
		t.Fatalf("expected version bump on rebuild, got %d", rebuilt.Version)
	}
	fresh, err := svc.VerifySnapshot(context.Background(), "tenant-1", rebuilt.ID)
	if err != nil {
		t.Fatalf("verify rebuilt snapshot: %v", err)
	}
	if !fresh.Match {
		t.Fatalf("rebuilt snapshot must match its source: %s", fresh.MismatchReason)
	}
}

// TestReportingOwnerSeesExceptionsWithoutInternalNoise proves that the owner
// exception feed contains only owner-visible exceptions and never internal
// operational noise (routine turnover work, closed records, alert queues).
func TestReportingOwnerSeesExceptionsWithoutInternalNoise(t *testing.T) {
	pool := setupPool(t)
	svc := reporting.NewReportingService(pool, nil)

	// Owner-visible: active incident, open service recovery, open financial
	// exception.
	insertTicket(t, pool, "tkt-inc", "tenant-1", "prop-1", "incident", "in_progress")
	insertRecovery(t, pool, "rec-1", "tenant-1", "prop-1", "tkt-inc", 700, "open")
	insertFinancialException(t, pool, "fin-1", "tenant-1", "prop-1", "open")

	// Internal noise: routine turnover work, closed incident, closed
	// recovery, resolved financial exception.
	insertTicket(t, pool, "tkt-turn", "tenant-1", "prop-1", "turnover", "in_progress")
	insertTicket(t, pool, "tkt-closed", "tenant-1", "prop-1", "incident", "closed")
	insertRecovery(t, pool, "rec-2", "tenant-1", "prop-1", "tkt-closed", 0, "closed")
	insertFinancialException(t, pool, "fin-2", "tenant-1", "prop-1", "resolved")

	// Noise from a different property must also stay out of the feed.
	insertTicket(t, pool, "tkt-inc-other", "tenant-1", "prop-2", "incident", "in_progress")

	feed, err := svc.ListOwnerExceptions(context.Background(), "tenant-1", "prop-1")
	if err != nil {
		t.Fatalf("list owner exceptions: %v", err)
	}

	bySource := map[string]int{}
	for _, ex := range feed {
		if !ex.OwnerVisible {
			t.Fatalf("feed must only carry owner-visible exceptions, got %+v", ex)
		}
		bySource[ex.Source]++
	}

	if bySource[reporting.ExceptionSourceIncident] != 1 {
		t.Fatalf("expected exactly 1 owner-visible incident, got %d", bySource[reporting.ExceptionSourceIncident])
	}
	if bySource[reporting.ExceptionSourceServiceRecovery] != 1 {
		t.Fatalf("expected exactly 1 owner-visible service recovery, got %d", bySource[reporting.ExceptionSourceServiceRecovery])
	}
	if bySource[reporting.ExceptionSourceFinancial] != 1 {
		t.Fatalf("expected exactly 1 owner-visible financial exception, got %d", bySource[reporting.ExceptionSourceFinancial])
	}
	if len(feed) != 3 {
		t.Fatalf("expected 3 owner-visible exceptions, got %d: %+v", len(feed), feed)
	}

	for _, ex := range feed {
		if ex.SourceID == "tkt-turn" || ex.SourceID == "tkt-closed" || ex.SourceID == "rec-2" || ex.SourceID == "fin-2" {
			t.Fatalf("internal noise record %s leaked into the owner feed", ex.SourceID)
		}
		if ex.PropertyID != "prop-1" {
			t.Fatalf("property-scoped feed leaked a record from property %s", ex.PropertyID)
		}
	}
}

// TestReportingWorkerMetricDoesNotBecomeDiscipline proves that worker metrics
// are recorded as development data: they are returned chronologically without
// any rank or discipline artifact, and the non-disciplinary guard holds.
func TestReportingWorkerMetricDoesNotBecomeDiscipline(t *testing.T) {
	pool := setupPool(t)
	svc := reporting.NewReportingService(pool, nil)

	_, err := svc.RecordWorkerMetric(context.Background(), "tenant-1", reporting.MetricObservationParams{
		WorkerID:   "worker-a",
		PropertyID: "prop-1",
		MetricKind: reporting.MetricKindTurnoverTimeMinutes,
		Value:      90,
		Unit:       "minutes",
		SourceRef:  "ticket/t-1",
	}, "ops-1")
	if err != nil {
		t.Fatalf("record metric 1: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
	_, err = svc.RecordWorkerMetric(context.Background(), "tenant-1", reporting.MetricObservationParams{
		WorkerID:   "worker-b",
		PropertyID: "prop-1",
		MetricKind: reporting.MetricKindTurnoverTimeMinutes,
		Value:      45,
		Unit:       "minutes",
		SourceRef:  "ticket/t-2",
	}, "ops-1")
	if err != nil {
		t.Fatalf("record metric 2: %v", err)
	}

	observations, err := svc.ListWorkerMetrics(context.Background(), "tenant-1", "prop-1", "", "")
	if err != nil {
		t.Fatalf("list worker metrics: %v", err)
	}
	if len(observations) != 2 {
		t.Fatalf("expected 2 observations, got %d", len(observations))
	}
	if err := reporting.GuardMetricsNonDisciplinary(observations); err != nil {
		t.Fatalf("metric guard must pass: %v", err)
	}

	// No rank or discipline artifact may ever be serialized.
	for _, o := range observations {
		raw, err := json.Marshal(o)
		if err != nil {
			t.Fatalf("marshal observation: %v", err)
		}
		for _, forbidden := range []string{"rank", "discipline", "leaderboard"} {
			if strings.Contains(string(raw), forbidden) {
				t.Fatalf("metric observation JSON must not expose %q, got %s", forbidden, string(raw))
			}
		}
	}

	// The list is chronological, not ranked by value (worker-a recorded
	// first with the higher value).
	if observations[0].WorkerID != "worker-a" {
		t.Fatalf("expected chronological order, first observation recorded by worker-a, got %s", observations[0].WorkerID)
	}
	if observations[0].HasRank() || observations[1].HasRank() {
		t.Fatal("observations must never carry a rank")
	}

	// A summary aggregates without producing a leaderboard position.
	summary, err := svc.WorkerMetricSummary(context.Background(), "tenant-1", "prop-1", "worker-a", reporting.MetricKindTurnoverTimeMinutes)
	if err != nil {
		t.Fatalf("worker metric summary: %v", err)
	}
	if summary.Count != 1 || summary.Average != 90 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}

// TestReportingOwnerMonthlyReportMatchesSource proves that the owner-facing
// monthly report is a rebuildable projection whose aggregates equal the
// source transactions it was built from, and that its exception section only
// carries owner-visible records.
func TestReportingOwnerMonthlyReportMatchesSource(t *testing.T) {
	pool := setupPool(t)
	svc := reporting.NewReportingService(pool, nil)

	insertCharge(t, pool, "chg-1", "tenant-1", "prop-1", "management_fee", 10000, "applied")
	insertCharge(t, pool, "chg-2", "tenant-1", "prop-1", "task_service", 5000, "applied")
	insertTicket(t, pool, "tkt-inc", "tenant-1", "prop-1", "incident", "in_progress")
	insertRecovery(t, pool, "rec-1", "tenant-1", "prop-1", "tkt-inc", 700, "open")
	insertTicket(t, pool, "tkt-done", "tenant-1", "prop-1", "turnover", "closed")
	insertTicket(t, pool, "tkt-noise", "tenant-1", "prop-1", "restock", "in_progress")
	insertStockLocation(t, pool, "loc-1", "tenant-1", "prop-1")
	insertMovement(t, pool, "mov-1", "tenant-1", "loc-1")
	insertMovement(t, pool, "mov-2", "tenant-1", "loc-1")

	snap, err := svc.RebuildSnapshot(context.Background(), "tenant-1", reporting.RebuildParams{
		Kind:       reporting.ProjectionOwnerMonthlyReport,
		PropertyID: "prop-1",
	})
	if err != nil {
		t.Fatalf("rebuild monthly report: %v", err)
	}

	verification, err := svc.VerifySnapshot(context.Background(), "tenant-1", snap.ID)
	if err != nil {
		t.Fatalf("verify monthly report: %v", err)
	}
	if !verification.Match {
		t.Fatalf("monthly report must match its source: %s", verification.MismatchReason)
	}

	var rpt reporting.OwnerMonthlyReport
	if err := json.Unmarshal(snap.Data, &rpt); err != nil {
		t.Fatalf("unmarshal monthly report: %v", err)
	}
	if rpt.Contribution.RevenueMinorUnits != 15000 {
		t.Fatalf("expected contribution revenue 15000, got %d", rpt.Contribution.RevenueMinorUnits)
	}
	if rpt.CompletedTickets != 1 {
		t.Fatalf("expected 1 completed ticket, got %d", rpt.CompletedTickets)
	}
	if rpt.OpenIncidents != 1 {
		t.Fatalf("expected 1 open incident, got %d", rpt.OpenIncidents)
	}
	if rpt.OpenRecoveries != 1 {
		t.Fatalf("expected 1 open recovery, got %d", rpt.OpenRecoveries)
	}
	if rpt.InventoryMovements != 2 {
		t.Fatalf("expected 2 inventory movements, got %d", rpt.InventoryMovements)
	}
	if len(rpt.OwnerExceptions) != 2 {
		t.Fatalf("expected 2 owner-visible exceptions in the report (incident + recovery), got %d: %+v", len(rpt.OwnerExceptions), rpt.OwnerExceptions)
	}
	for _, ex := range rpt.OwnerExceptions {
		if ex.SourceID == "tkt-noise" || ex.SourceID == "tkt-done" {
			t.Fatalf("internal noise leaked into the monthly report exceptions: %s", ex.SourceID)
		}
	}
}

// TestReportingCrossTenantDenied proves snapshots and metrics fail closed
// across tenant boundaries.
func TestReportingCrossTenantDenied(t *testing.T) {
	pool := setupPool(t)
	svc := reporting.NewReportingService(pool, nil)

	insertCharge(t, pool, "chg-1", "tenant-a", "prop-a", "management_fee", 1000, "applied")

	snap, err := svc.RebuildSnapshot(context.Background(), "tenant-a", reporting.RebuildParams{
		Kind:       reporting.ProjectionPropertyContribution,
		PropertyID: "prop-a",
	})
	if err != nil {
		t.Fatalf("rebuild for tenant-a: %v", err)
	}

	if _, err := svc.GetSnapshot(context.Background(), "tenant-b", snap.ID); !errors.Is(err, reporting.ErrSnapshotNotFound) {
		t.Fatalf("expected ErrSnapshotNotFound for cross-tenant snapshot read, got %v", err)
	}

	observations, err := svc.ListWorkerMetrics(context.Background(), "tenant-b", "", "", "")
	if err != nil {
		t.Fatalf("list metrics for tenant-b: %v", err)
	}
	if len(observations) != 0 {
		t.Fatalf("tenant-b must see no tenant-a metrics, got %d", len(observations))
	}
}
