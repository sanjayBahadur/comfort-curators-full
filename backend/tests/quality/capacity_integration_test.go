package quality_test

import (
	"context"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"comfort-curators-backend/internal/inventory"
	"comfort-curators-backend/internal/operations"
	"comfort-curators-backend/internal/platform/config"
	"comfort-curators-backend/internal/platform/database"
	"comfort-curators-backend/internal/platform/logging"
	"comfort-curators-backend/internal/property"
	"comfort-curators-backend/internal/quality"
	"comfort-curators-backend/internal/reservations"
	"comfort-curators-backend/internal/workforce"

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

func connectMigrated(t *testing.T) *database.DB {
	t.Helper()

	// The global slog logger is package state; initializing it before the
	// first log call avoids the nil-logger lazy-init path.
	logging.Init("error")

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

	cfg := config.Config{
		DBHost: host,
		DBPort: 5432,
		DBUser: user,
		DBPass: pass,
		DBName: name,
		DBSSL:  "disable",
	}

	db, err := database.Connect(context.Background(), cfg)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := database.RunMigrations(context.Background(), db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// The API process applies the module schemas at startup; the capacity
	// scenario exercises the same tables, so apply them here too.
	ctx := context.Background()
	for name, apply := range map[string]func(context.Context, *pgxpool.Pool) error{
		"property":     property.EnsureSchema,
		"reservations": reservations.EnsureSchema,
		"operations":   operations.EnsureSchema,
		"workforce":    workforce.EnsureSchema,
		"inventory":    inventory.EnsureSchema,
	} {
		if err := apply(ctx, db.Pool); err != nil {
			t.Fatalf("apply %s schema: %v", name, err)
		}
	}

	return db
}

func TestCapacityScenarioRunsOnExistingSchema(t *testing.T) {
	if !postgresAvailable() {
		t.Skip("PostgreSQL not available")
	}

	db := connectMigrated(t)
	ctx := context.Background()

	tenantID := fmt.Sprintf("tenant-quality-cap-%d", time.Now().UnixNano())
	target := quality.CapacityTarget{
		Properties:   2,
		Reservations: 5,
		Workers:      3,
		Tickets:      100,
		Movements:    200,
	}

	// Seeds must be visible and verify through the existing schema.
	seeds, err := quality.SeedCapacity(ctx, db.Pool, tenantID, target)
	if err != nil {
		t.Fatalf("seed capacity scenario: %v", err)
	}

	result, err := quality.VerifyCapacity(ctx, db.Pool, seeds, target)
	if err != nil {
		t.Fatalf("verify capacity scenario: %v", err)
	}
	if !result.Completed {
		t.Fatal("capacity verification did not complete")
	}
	if result.Counts["properties"] != 2 {
		t.Errorf("properties count = %d, want 2", result.Counts["properties"])
	}
	if result.Counts["reservations"] != 5 {
		t.Errorf("reservations count = %d, want 5", result.Counts["reservations"])
	}
	if result.Counts["workers"] != 3 {
		t.Errorf("workers count = %d, want 3", result.Counts["workers"])
	}
	if result.Counts["tickets"] != 100 {
		t.Errorf("tickets count = %d, want 100", result.Counts["tickets"])
	}
	if result.Counts["movements"] != 200 {
		t.Errorf("movements count = %d, want 200", result.Counts["movements"])
	}
	if len(result.Queries) < 4 {
		t.Errorf("expected representative core queries to run, got %d", len(result.Queries))
	}
}

func TestCapacityScenarioHonoursImmutableMovementLedger(t *testing.T) {
	if !postgresAvailable() {
		t.Skip("PostgreSQL not available")
	}

	db := connectMigrated(t)
	ctx := context.Background()

	tenantID := fmt.Sprintf("tenant-quality-cap-%d", time.Now().UnixNano())
	target := quality.CapacityTarget{Properties: 1, Reservations: 0, Workers: 1, Tickets: 0, Movements: 10}

	if _, err := quality.SeedCapacity(ctx, db.Pool, tenantID, target); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// The append-only inventory movement ledger must still reject mutation at
	// capacity volume without any schema redesign.
	if _, err := db.Pool.Exec(ctx,
		`UPDATE inventory_movements SET quantity = 99 WHERE tenant_id = $1`, tenantID); err == nil {
		t.Error("inventory_movements must reject UPDATE even at capacity volume")
	}
}
