package inventory_test

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"comfort-curators-backend/internal/inventory"
	"comfort-curators-backend/internal/platform/audit"
	"comfort-curators-backend/internal/platform/testdb"

	"github.com/jackc/pgx/v5/pgxpool"
)

func invPostgresAvailable() bool {
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

func invDBConnString() string {
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

func invPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if !invPostgresAvailable() {
		t.Skip("PostgreSQL not available for inventory integration test")
	}
	pool, err := pgxpool.New(context.Background(), invDBConnString())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := inventory.EnsureSchema(context.Background(), pool); err != nil {
		t.Fatalf("ensure inventory schema: %v", err)
	}
	if err := audit.EnsureSchema(context.Background(), pool); err != nil {
		t.Fatalf("ensure audit schema: %v", err)
	}
	for _, table := range []string{
		"inventory_count_lines",
		"inventory_counts",
		"inventory_movements",
		"stock_locations",
		"audit_events",
	} {
		if _, err := pool.Exec(context.Background(), "TRUNCATE "+table+" CASCADE"); err != nil {
			t.Fatalf("truncate %s: %v", table, err)
		}
	}
	return pool
}

func newInvService(t *testing.T, pool *pgxpool.Pool) *inventory.Service {
	t.Helper()
	return inventory.NewService(pool).
		WithAudit(audit.NewAuditStore(pool))
}

func createLocation(t *testing.T, svc *inventory.Service, tenantID, name, locationType string) *inventory.StockLocation {
	t.Helper()
	loc, err := svc.CreateLocation(context.Background(), tenantID, inventory.CreateLocationParams{
		Name:         name,
		LocationType: locationType,
	}, "actor-1")
	if err != nil {
		t.Fatalf("create location: %v", err)
	}
	return loc
}

// Balance rebuilds from ledger: the balance for a location+item must equal
// the sum of all recorded movements.
func TestBalanceRebuildsFromLedger(t *testing.T) {
	tenantID := "tenant-bal-rebuild"
	pool := invPool(t)
	svc := newInvService(t, pool)
	ctx := context.Background()

	loc := createLocation(t, svc, tenantID, "Central Hub", inventory.LocationTypeCentral)
	itemID := "item-bal-1"

	// Record multiple movements
	steps := []struct {
		mtype    string
		quantity int64
	}{
		{inventory.MovementTypeReceive, 200},
		{inventory.MovementTypeReceive, 150},
		{inventory.MovementTypeIssue, -50},
		{inventory.MovementTypeIssue, -30},
		{inventory.MovementTypeReceive, 80},
	}

	for _, step := range steps {
		_, err := svc.RecordMovement(ctx, tenantID, loc.ID, inventory.RecordMovementParams{
			CatalogItemID: itemID,
			MovementType:  step.mtype,
			Quantity:      step.quantity,
			Reason:        "test movement",
		}, "actor-1")
		if err != nil {
			t.Fatalf("record movement %s %d: %v", step.mtype, step.quantity, err)
		}
	}

	// Rebuild balance from ledger
	balance, movements, err := svc.GetBalance(ctx, tenantID, loc.ID, itemID)
	if err != nil {
		t.Fatalf("get balance: %v", err)
	}

	expectedBalance := int64(200 + 150 - 50 - 30 + 80)
	if balance != expectedBalance {
		t.Fatalf("balance rebuild: expected %d, got %d", expectedBalance, balance)
	}
	if len(movements) != len(steps) {
		t.Fatalf("expected %d movements, got %d", len(steps), len(movements))
	}

	computed := inventory.ComputeBalance(movements)
	if computed != balance {
		t.Fatalf("ComputeBalance %d != GetBalance %d", computed, balance)
	}
}

// Unexplained negative stock fails: issuing more than available must be rejected
// unless it is a documented adjustment.
func TestUnexplainedNegativeStockFails(t *testing.T) {
	tenantID := "tenant-neg-stock"
	pool := invPool(t)
	svc := newInvService(t, pool)
	ctx := context.Background()

	loc := createLocation(t, svc, tenantID, "Property Stock", inventory.LocationTypeProperty)
	itemID := "item-neg-1"

	// Seed some stock
	_, err := svc.RecordMovement(ctx, tenantID, loc.ID, inventory.RecordMovementParams{
		CatalogItemID: itemID,
		MovementType:  inventory.MovementTypeReceive,
		Quantity:      50,
		Reason:        "seed stock",
	}, "actor-1")
	if err != nil {
		t.Fatalf("seed stock: %v", err)
	}

	// Attempt to issue more than available
	_, err = svc.RecordMovement(ctx, tenantID, loc.ID, inventory.RecordMovementParams{
		CatalogItemID: itemID,
		MovementType:  inventory.MovementTypeIssue,
		Quantity:      -100,
		Reason:        "over-issue attempt",
	}, "actor-1")
	if err == nil {
		t.Fatal("over-issue must be rejected")
	}
	if err.Error() == "" || !contains(err.Error(), "negative") {
		t.Fatalf("expected negative stock error, got: %v", err)
	}

	// Balance must still be 50 (the failed movement must not have been recorded)
	balance, _, err := svc.GetBalance(ctx, tenantID, loc.ID, itemID)
	if err != nil {
		t.Fatalf("get balance after reject: %v", err)
	}
	if balance != 50 {
		t.Fatalf("balance must be 50 after rejected over-issue, got %d", balance)
	}

	// An adjustment that goes negative IS allowed (attributable)
	_, err = svc.RecordMovement(ctx, tenantID, loc.ID, inventory.RecordMovementParams{
		CatalogItemID: itemID,
		MovementType:  inventory.MovementTypeAdjustment,
		Quantity:      -60,
		ReferenceType: "manual_correction",
		Reason:        "documented manual correction for damaged stock",
	}, "actor-1")
	if err != nil {
		t.Fatalf("attributable adjustment must be allowed: %v", err)
	}
}

// Concurrent movements remain consistent: parallel movement recordings
// must serialize safely without lost updates or duplicate effects.
func TestConcurrentMovementIsConsistent(t *testing.T) {
	tenantID := "tenant-concurrent"
	pool := invPool(t)
	svc := newInvService(t, pool)
	ctx := context.Background()

	loc := createLocation(t, svc, tenantID, "Concurrent Hub", inventory.LocationTypeCentral)
	itemID := "item-conc-1"

	// Seed initial stock
	_, err := svc.RecordMovement(ctx, tenantID, loc.ID, inventory.RecordMovementParams{
		CatalogItemID: itemID,
		MovementType:  inventory.MovementTypeReceive,
		Quantity:      1000,
		Reason:        "initial stock",
	}, "actor-1")
	if err != nil {
		t.Fatalf("seed stock: %v", err)
	}

	// Run concurrent issues
	var wg sync.WaitGroup
	errorsCh := make(chan error, 20)
	concurrency := 10

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := svc.RecordMovement(ctx, tenantID, loc.ID, inventory.RecordMovementParams{
				CatalogItemID: itemID,
				MovementType:  inventory.MovementTypeIssue,
				Quantity:      -10,
				Reason:        fmt.Sprintf("concurrent issue %d", idx),
			}, "actor-1")
			if err != nil {
				errorsCh <- err
			}
		}(i)
	}

	wg.Wait()
	close(errorsCh)

	failures := 0
	for e := range errorsCh {
		failures++
		t.Logf("concurrent movement error: %v", e)
	}

	balance, movements, err := svc.GetBalance(ctx, tenantID, loc.ID, itemID)
	if err != nil {
		t.Fatalf("get balance: %v", err)
	}

	// Each successful issue removes 10, plus the initial 1000 receive
	successfulIssues := concurrency - failures
	expectedBalance := int64(1000 - successfulIssues*10)
	if balance != expectedBalance {
		t.Fatalf("concurrent consistency: expected balance %d, got %d (failures=%d)", expectedBalance, balance, failures)
	}

	// Verify movement count
	if len(movements) != 1+successfulIssues {
		t.Fatalf("expected %d movements (1 seed + %d issues), got %d", 1+successfulIssues, successfulIssues, len(movements))
	}
}

// Movement ledger is append-only: movements cannot be updated or deleted.
func TestMovementLedgerIsAppendOnly(t *testing.T) {
	tenantID := "tenant-append-only"
	pool := invPool(t)
	svc := newInvService(t, pool)
	ctx := context.Background()

	loc := createLocation(t, svc, tenantID, "AppendOnly Hub", inventory.LocationTypeCentral)
	itemID := "item-append-1"

	mov, err := svc.RecordMovement(ctx, tenantID, loc.ID, inventory.RecordMovementParams{
		CatalogItemID: itemID,
		MovementType:  inventory.MovementTypeReceive,
		Quantity:      100,
		Reason:        "initial receive",
	}, "actor-1")
	if err != nil {
		t.Fatalf("record movement: %v", err)
	}

	// Attempt to update the movement directly
	_, err = pool.Exec(ctx, `UPDATE inventory_movements SET quantity = 999 WHERE id = $1`, mov.ID)
	if err != nil {
		t.Logf("update blocked (expected): %v", err)
	}

	// Verify the movement's quantity is unchanged in the ledger
	var storedQty int64
	err = pool.QueryRow(ctx,
		`SELECT quantity FROM inventory_movements WHERE id = $1`, mov.ID,
	).Scan(&storedQty)
	if err != nil {
		t.Fatalf("read movement: %v", err)
	}
	if storedQty != 100 {
		t.Fatalf("ledger quantity must remain 100, got %d", storedQty)
	}

	// Deleting a movement should be blocked or recoverable
	_, err = pool.Exec(ctx, `DELETE FROM inventory_movements WHERE id = $1`, mov.ID)
	if err != nil {
		t.Logf("delete blocked (expected): %v", err)
	}

	// Balance must still equal 100
	balance, _, err := svc.GetBalance(ctx, tenantID, loc.ID, itemID)
	if err != nil {
		t.Fatalf("get balance: %v", err)
	}
	if balance != 100 {
		t.Fatalf("balance must be 100 after blocked mutation, got %d", balance)
	}
}

// Reconciliation is attributable: count variance posts explicit adjustments
// with counter, reviewer, reason and source count.
func TestReconciliationIsAttributable(t *testing.T) {
	tenantID := "tenant-reconcile"
	pool := invPool(t)
	svc := newInvService(t, pool)
	ctx := context.Background()

	loc := createLocation(t, svc, tenantID, "Reconcile Hub", inventory.LocationTypeCentral)

	// Seed stock for two items
	items := map[string]int64{"item-rec-1": 100, "item-rec-2": 50}
	for itemID, qty := range items {
		_, err := svc.RecordMovement(ctx, tenantID, loc.ID, inventory.RecordMovementParams{
			CatalogItemID: itemID,
			MovementType:  inventory.MovementTypeReceive,
			Quantity:      qty,
			Reason:        "seed stock",
		}, "actor-1")
		if err != nil {
			t.Fatalf("seed item %s: %v", itemID, err)
		}
	}

	// Create a count
	count, err := svc.CreateCount(ctx, tenantID, inventory.CreateCountParams{
		LocationID: loc.ID,
		CountedBy:  "counter-1",
	}, "actor-1")
	if err != nil {
		t.Fatalf("create count: %v", err)
	}

	// Record count lines with different counted values
	_, err = svc.UpdateCountLine(ctx, tenantID, count.ID, inventory.UpdateCountLineParams{
		CatalogItemID:   "item-rec-1",
		CountedQuantity: 95, // shortage of 5
	}, "actor-1")
	if err != nil {
		t.Fatalf("update line item-rec-1: %v", err)
	}

	_, err = svc.UpdateCountLine(ctx, tenantID, count.ID, inventory.UpdateCountLineParams{
		CatalogItemID:   "item-rec-2",
		CountedQuantity: 60, // surplus of 10
	}, "actor-1")
	if err != nil {
		t.Fatalf("update line item-rec-2: %v", err)
	}

	// Review the count
	reviewed, err := svc.ReviewCount(ctx, tenantID, count.ID, inventory.ReviewCountParams{
		ReviewedBy: "reviewer-1",
	}, "actor-1")
	if err != nil {
		t.Fatalf("review count: %v", err)
	}
	if reviewed.Status != inventory.CountStatusReviewed {
		t.Fatalf("expected reviewed status, got %s", reviewed.Status)
	}

	// Reconcile
	reconciled, err := svc.ReconcileCount(ctx, tenantID, count.ID, inventory.ReconcileCountParams{
		ReviewedBy: "reviewer-1",
		Reason:     "monthly cycle count",
	}, "actor-1")
	if err != nil {
		t.Fatalf("reconcile count: %v", err)
	}
	if reconciled.Status != inventory.CountStatusReconciled {
		t.Fatalf("expected reconciled status, got %s", reconciled.Status)
	}

	// Verify adjustment movements were created
	bal1, movs1, _ := svc.GetBalance(ctx, tenantID, loc.ID, "item-rec-1")
	if bal1 != 95 {
		t.Fatalf("item-rec-1 balance after reconciliation must be 95, got %d", bal1)
	}
	hasAdjustment := false
	for _, m := range movs1 {
		if m.MovementType == inventory.MovementTypeAdjustment && m.ReferenceID == count.ID {
			hasAdjustment = true
			if m.Quantity != -5 {
				t.Fatalf("expected adjustment -5, got %d", m.Quantity)
			}
		}
	}
	if !hasAdjustment {
		t.Fatal("reconciliation must create attributable adjustment for item-rec-1")
	}

	bal2, movs2, _ := svc.GetBalance(ctx, tenantID, loc.ID, "item-rec-2")
	if bal2 != 60 {
		t.Fatalf("item-rec-2 balance after reconciliation must be 60, got %d", bal2)
	}
	hasAdjustment = false
	for _, m := range movs2 {
		if m.MovementType == inventory.MovementTypeAdjustment && m.ReferenceID == count.ID {
			hasAdjustment = true
			if m.Quantity != 10 {
				t.Fatalf("expected adjustment +10, got %d", m.Quantity)
			}
			if m.Reason == "" {
				t.Fatal("adjustment must include reason")
			}
		}
	}
	if !hasAdjustment {
		t.Fatal("reconciliation must create attributable adjustment for item-rec-2")
	}

	// Verify original ledger entries are preserved (not modified)
	_, fullMovs, _ := svc.GetBalance(ctx, tenantID, loc.ID, "item-rec-1")
	hasOriginalReceive := false
	for _, m := range fullMovs {
		if m.MovementType == inventory.MovementTypeReceive && m.Quantity == 100 {
			hasOriginalReceive = true
		}
	}
	if !hasOriginalReceive {
		t.Fatal("original receive movement must remain in ledger after reconciliation")
	}
}

// Expired stock cannot issue: stock received with a past expiry date
// must not be issuable. The expired portion must be written off via
// an explicit expiry movement before the remaining stock can be used.
func TestExpiredStockCannotIssue(t *testing.T) {
	tenantID := "tenant-expired"
	pool := invPool(t)
	svc := newInvService(t, pool)
	ctx := context.Background()

	loc := createLocation(t, svc, tenantID, "Expiry Hub", inventory.LocationTypeCentral)
	itemID := "item-exp-1"

	yesterday := time.Now().Add(-24 * time.Hour)
	future := time.Now().Add(30 * 24 * time.Hour)

	// Receive stock that expired yesterday
	_, err := svc.RecordMovement(ctx, tenantID, loc.ID, inventory.RecordMovementParams{
		CatalogItemID: itemID,
		MovementType:  inventory.MovementTypeReceive,
		Quantity:      100,
		Reason:        "expired stock",
		ExpiresAt:     &yesterday,
	}, "actor-1")
	if err != nil {
		t.Fatalf("receive expired stock: %v", err)
	}

	// Issue from expired stock must fail
	_, err = svc.RecordMovement(ctx, tenantID, loc.ID, inventory.RecordMovementParams{
		CatalogItemID: itemID,
		MovementType:  inventory.MovementTypeIssue,
		Quantity:      -50,
		Reason:        "issue attempt against expired stock",
	}, "actor-1")
	if err == nil {
		t.Fatal("issue from expired stock must be rejected")
	}
	if !contains(err.Error(), "expired") {
		t.Fatalf("expected expired stock error, got: %v", err)
	}

	// Receive fresh stock (future expiry) in the same location/item
	_, err = svc.RecordMovement(ctx, tenantID, loc.ID, inventory.RecordMovementParams{
		CatalogItemID: itemID,
		MovementType:  inventory.MovementTypeReceive,
		Quantity:      50,
		Reason:        "fresh stock",
		ExpiresAt:     &future,
	}, "actor-1")
	if err != nil {
		t.Fatalf("receive fresh stock: %v", err)
	}

	// Effective balance = fresh stock only (50). Total balance = 150.
	// Issue 30 from the fresh stock must succeed.
	_, err = svc.RecordMovement(ctx, tenantID, loc.ID, inventory.RecordMovementParams{
		CatalogItemID: itemID,
		MovementType:  inventory.MovementTypeIssue,
		Quantity:      -30,
		Reason:        "issue fresh stock",
	}, "actor-1")
	if err != nil {
		t.Fatalf("issue from fresh stock must succeed: %v", err)
	}

	// Issue 30 more (needs 20 of fresh remaining) should fail
	_, err = svc.RecordMovement(ctx, tenantID, loc.ID, inventory.RecordMovementParams{
		CatalogItemID: itemID,
		MovementType:  inventory.MovementTypeIssue,
		Quantity:      -30,
		Reason:        "too much issue",
	}, "actor-1")
	if err == nil {
		t.Fatal("issue exceeding effective balance must be rejected")
	}

	// Write off the expired portion via explicit expiry movement
	_, err = svc.RecordMovement(ctx, tenantID, loc.ID, inventory.RecordMovementParams{
		CatalogItemID: itemID,
		MovementType:  inventory.MovementTypeExpiry,
		Quantity:      -100,
		Reason:        "writing off expired stock",
	}, "actor-1")
	if err != nil {
		t.Fatalf("expiry write-off must succeed: %v", err)
	}

	// Now balance = 20 (150 - 30 issue - 100 expiry)
	balance, _, err := svc.GetBalance(ctx, tenantID, loc.ID, itemID)
	if err != nil {
		t.Fatalf("get balance after expiry write-off: %v", err)
	}
	if balance != 20 {
		t.Fatalf("balance after expiry write-off must be 20, got %d", balance)
	}

	// Remaining 20 can be issued
	_, err = svc.RecordMovement(ctx, tenantID, loc.ID, inventory.RecordMovementParams{
		CatalogItemID: itemID,
		MovementType:  inventory.MovementTypeIssue,
		Quantity:      -20,
		Reason:        "issue remaining stock",
	}, "actor-1")
	if err != nil {
		t.Fatalf("issue remaining 20 must succeed: %v", err)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
